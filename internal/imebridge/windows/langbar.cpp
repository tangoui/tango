#include "langbar.h"
#include "pipe.h"
#include <oleauto.h>
#include <olectl.h>

namespace tango::ime {

LangBar::LangBar(const Config& config, Pipe* pipe) : config_(config), pipe_(pipe) { ++references; }
LangBar::~LangBar() { --references; }

void LangBar::update(Frame frame) {
    if (frame.type == Type::mode) {
        mode_ = std::move(frame);
        if (sink_) sink_->OnUpdate(0x1 | 0x2 | 0x4); // icon, text and tooltip
    } else if (frame.type == Type::menu) {
        menu_ = std::move(frame);
    }
}

HRESULT LangBar::QueryInterface(REFIID iid, void** out) {
    if (!out) return E_POINTER;
    *out = nullptr;
    if (iid == IID_IUnknown || iid == button_iid || iid == __uuidof(ITfLangBarItem))
        *out = static_cast<TipLangBarButton*>(this);
    else if (iid == __uuidof(ITfSource))
        *out = static_cast<ITfSource*>(this);
    if (!*out) return E_NOINTERFACE;
    AddRef();
    return S_OK;
}
ULONG LangBar::AddRef() { return ++refs_; }
ULONG LangBar::Release() {
    const auto count = --refs_;
    if (!count) delete this;
    return count;
}

HRESULT LangBar::GetInfo(TF_LANGBARITEMINFO* info) {
    if (!info) return E_POINTER;
    *info = {};
    info->clsidService = config_.clsid;
    info->guidItem = input_mode_item;
    info->dwStyle = 0x00020000 | 0x2; // menu button in the system tray
    const size_t size = std::min(config_.name.size(), size_t(ARRAYSIZE(info->szDescription) - 1));
    config_.name.copy(info->szDescription, size);
    return S_OK;
}
HRESULT LangBar::GetStatus(DWORD* status) {
    if (!status) return E_POINTER;
    *status = status_;
    return S_OK;
}
HRESULT LangBar::Show(BOOL show) {
    status_ = show ? 0 : 1;
    if (sink_) sink_->OnUpdate(0x00010000); // status
    return S_OK;
}
HRESULT LangBar::GetTooltipString(BSTR* text) {
    if (!text) return E_POINTER;
    *text = SysAllocString(config_.name.c_str());
    return *text ? S_OK : E_OUTOFMEMORY;
}
HRESULT LangBar::GetText(BSTR* text) {
    if (!text) return E_POINTER;
    try {
        *text = SysAllocString(wide(mode_.short_text).c_str());
        return *text ? S_OK : E_OUTOFMEMORY;
    } catch (...) {
        *text = nullptr;
        return E_FAIL;
    }
}
HRESULT LangBar::OnClick(int, POINT, const RECT*) { return S_OK; }

HRESULT LangBar::InitMenu(TipMenu* menu) {
    if (!menu) return E_POINTER;
    try {
        if (pipe_) {
            Frame open;
            open.type = Type::menu_open;
            open.session = session_;
            pipe_->post(std::move(open));
        }
        visible_ = menu_.items;
        for (size_t i = 0; i < visible_.size(); ++i) {
            const auto& entry = visible_[i];
            DWORD flags = 0;
            if (entry.flags & checked) flags |= 0x1;
            if (entry.flags & separator) flags |= 0x4;
            if (entry.flags & disabled) flags |= 0x10;
            const auto title = wide(entry.title);
            const auto hr = menu->AddMenuItem(static_cast<UINT>(i + 1), flags, nullptr, nullptr,
                                             title.data(), static_cast<ULONG>(title.size()), nullptr);
            if (FAILED(hr)) return hr;
        }
        return S_OK;
    } catch (...) {
        return E_FAIL;
    }
}

HRESULT LangBar::OnMenuSelect(UINT id) {
    if (!pipe_ || !id || id > visible_.size()) return S_FALSE;
    const auto& item = visible_[id - 1];
    if (item.flags & (disabled | separator)) return S_FALSE;
    try {
        Frame pick;
        pick.type = Type::menu_pick;
        pick.id = item.id;
        return pipe_->post(std::move(pick)) ? S_OK : S_FALSE;
    } catch (...) {
        return E_FAIL;
    }
}

HRESULT LangBar::GetIcon(HICON* icon) {
    if (!icon) return E_POINTER;
    *icon = nullptr;
    try {
        constexpr int size = 16;
        if (!mode_.icon.empty()) {
            const auto path = wide(mode_.icon);
            *icon = static_cast<HICON>(LoadImageW(nullptr, path.c_str(), IMAGE_ICON, size, size, LR_LOADFROMFILE));
            if (*icon) return S_OK;
        }
        const auto text = wide(mode_.short_text.empty() ? std::u16string_view(u"A") : std::u16string_view(mode_.short_text));
        HDC dc = CreateCompatibleDC(nullptr);
        if (!dc) return E_FAIL;
        BITMAPINFO info{};
        info.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
        info.bmiHeader.biWidth = size;
        info.bmiHeader.biHeight = -size;
        info.bmiHeader.biPlanes = 1;
        info.bmiHeader.biBitCount = 32;
        void* pixels = nullptr;
        HBITMAP color = CreateDIBSection(dc, &info, DIB_RGB_COLORS, &pixels, nullptr, 0);
        HBITMAP mask = CreateBitmap(size, size, 1, 1, nullptr);
        if (color && mask) {
            auto previous = SelectObject(dc, color);
            auto* argb = static_cast<uint32_t*>(pixels);
            std::fill_n(argb, size * size, 0u);
            auto font = CreateFontW(-14, 0, 0, 0, FW_BOLD, FALSE, FALSE, FALSE,
                                    DEFAULT_CHARSET, 0, 0, ANTIALIASED_QUALITY, 0, L"Microsoft YaHei UI");
            const auto oldFont = font ? SelectObject(dc, font) : nullptr;
            SetBkMode(dc, TRANSPARENT);
            SetTextColor(dc, RGB(255, 255, 255));
            RECT rect{0, 0, size, size};
            DrawTextW(dc, text.data(), static_cast<int>(text.size()), &rect,
                      DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
            GdiFlush();
            for (int i = 0; i < size * size; ++i) {
                const uint32_t alpha = argb[i] & 255;
                argb[i] |= alpha << 24;
            }
            if (oldFont) SelectObject(dc, oldFont);
            if (font) DeleteObject(font);
            SelectObject(dc, previous);
            ICONINFO iconInfo{TRUE, 0, 0, mask, color};
            *icon = CreateIconIndirect(&iconInfo);
        }
        if (mask) DeleteObject(mask);
        if (color) DeleteObject(color);
        DeleteDC(dc);
        return *icon ? S_OK : E_FAIL;
    } catch (...) {
        return E_FAIL;
    }
}

HRESULT LangBar::AdviseSink(REFIID iid, IUnknown* sink, DWORD* cookie) {
    if (!sink || !cookie) return E_POINTER;
    if (iid != __uuidof(ITfLangBarItemSink)) return CONNECT_E_CANNOTCONNECT;
    if (sink_) return CONNECT_E_ADVISELIMIT;
    const auto hr = sink->QueryInterface(iid, reinterpret_cast<void**>(sink_.put()));
    if (SUCCEEDED(hr)) *cookie = 1;
    return hr;
}
HRESULT LangBar::UnadviseSink(DWORD cookie) {
    if (cookie != 1 || !sink_) return CONNECT_E_NOCONNECTION;
    sink_.reset();
    return S_OK;
}

} // namespace tango::ime
