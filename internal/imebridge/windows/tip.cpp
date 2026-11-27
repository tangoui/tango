#include "bridge.h"
#include "langbar.h"
#include "pipe.h"

#include <algorithm>

namespace tango::ime {
namespace {

class Tip final : public TipProcessorEx, public ITfKeyEventSink,
                  public ITfThreadMgrEventSink, public ITfCompositionSink {
    ULONG refs_ = 1;
    Config config_;
    TfClientId client_ = 0;
    DWORD cookie_ = TF_INVALID_COOKIE;
    bool key_sink_ = false;
    bool foreground_ = true;
    ComPtr<ITfThreadMgr> manager_;
    ComPtr<ITfContext> context_;
    ComPtr<ITfComposition> composition_;
    ComPtr<ITfLangBarItemMgr> bar_manager_;
    ComPtr<LangBar> bar_;
    Pipe pipe_;
    HWND window_ = nullptr;
    uint64_t session_ = 0, next_session_ = 0, opened_generation_ = 0;
    std::optional<KeyOutcome> pending_;
    WPARAM tested_key_ = 0;
    LPARAM tested_scan_ = 0;
    bool tested_down_ = false;

    class Edit final : public ITfEditSession {
        ULONG refs_ = 1;
        Tip* tip_;
        ComPtr<ITfContext> context_;
        uint64_t session_;
        std::vector<Frame> edits_;
        Frame* caret_;
        bool finish_;
    public:
        Edit(Tip* tip, ITfContext* context, uint64_t session, std::vector<Frame> edits,
             bool finish, Frame* caret)
            : tip_(tip), session_(session), edits_(std::move(edits)), caret_(caret), finish_(finish) {
            tip_->AddRef();
            context_.retain(context);
        }
        ~Edit() { tip_->Release(); }
        HRESULT STDMETHODCALLTYPE QueryInterface(REFIID iid, void** out) override {
            if (!out) return E_POINTER;
            *out = nullptr;
            if (iid != IID_IUnknown && iid != __uuidof(ITfEditSession)) return E_NOINTERFACE;
            *out = static_cast<ITfEditSession*>(this);
            AddRef();
            return S_OK;
        }
        ULONG STDMETHODCALLTYPE AddRef() override { return ++refs_; }
        ULONG STDMETHODCALLTYPE Release() override {
            const auto count = --refs_;
            if (!count) delete this;
            return count;
        }
        HRESULT STDMETHODCALLTYPE DoEditSession(TfEditCookie cookie) override {
            try {
                if (tip_->session_ != session_ || tip_->context_.get() != context_.get()) return S_FALSE;
                if (caret_) {
                    Tip::add_caret(context_.get(), cookie, *caret_);
                    return S_OK;
                }
                for (auto& frame : edits_) {
                    const auto hr = frame.type == Type::compose
                        ? tip_->set_marked(context_.get(), cookie, frame)
                        : tip_->commit(context_.get(), cookie, frame);
                    if (FAILED(hr)) return hr;
                }
                return finish_ ? tip_->finish(cookie) : S_OK;
            } catch (...) {
                return E_FAIL;
            }
        }
    };

    static bool selection(ITfContext* context, TfEditCookie cookie, ComPtr<ITfRange>& range) {
        TF_SELECTION value{};
        ULONG fetched = 0;
        if (FAILED(context->GetSelection(cookie, TF_DEFAULT_SELECTION, 1, &value, &fetched)) ||
            fetched != 1) return false;
        *range.put() = value.range;
        return true;
    }

    static void add_caret(ITfContext* context, TfEditCookie cookie, Frame& frame) {
        ComPtr<ITfRange> range;
        ComPtr<ITfContextView> view;
        if (!selection(context, cookie, range) || FAILED(context->GetActiveView(view.put()))) return;
        if (FAILED(range->Collapse(cookie, TF_ANCHOR_END))) return;
        RECT rect{};
        BOOL clipped = FALSE;
        if (FAILED(view->GetTextExt(cookie, range.get(), &rect, &clipped)) || clipped) return;
        frame.caret = {rect.left, rect.top, rect.right - rect.left, rect.bottom - rect.top};
    }

    static HRESULT place_cursor(ITfContext* context, TfEditCookie cookie, ITfRange* range, LONG offset) {
        auto hr = range->Collapse(cookie, TF_ANCHOR_START);
        LONG moved = 0;
        if (SUCCEEDED(hr)) hr = range->ShiftEnd(cookie, offset, &moved, nullptr);
        if (SUCCEEDED(hr)) hr = range->Collapse(cookie, TF_ANCHOR_END);
        if (FAILED(hr)) return hr;
        TF_SELECTION value{range, {TF_AE_NONE, FALSE}};
        return context->SetSelection(cookie, 1, &value);
    }

    HRESULT set_marked(ITfContext* context, TfEditCookie cookie, const Frame& frame) {
        ComPtr<ITfRange> range;
        const auto text = wide(frame.text);
        if (!composition_) {
            if (text.empty()) return S_OK;
            ComPtr<ITfInsertAtSelection> insert;
            auto hr = context->QueryInterface(__uuidof(ITfInsertAtSelection),
                                              reinterpret_cast<void**>(insert.put()));
            if (FAILED(hr)) return hr;
            hr = insert->InsertTextAtSelection(cookie, TF_IAS_QUERYONLY, nullptr, 0, range.put());
            if (FAILED(hr) || !range) return FAILED(hr) ? hr : E_FAIL;
            ComPtr<ITfContextComposition> owner;
            hr = context->QueryInterface(__uuidof(ITfContextComposition), reinterpret_cast<void**>(owner.put()));
            if (FAILED(hr)) return hr;
            hr = owner->StartComposition(cookie, range.get(), this, composition_.put());
            if (FAILED(hr) || !composition_) return FAILED(hr) ? hr : E_FAIL;
        } else {
            const auto hr = composition_->GetRange(range.put());
            if (FAILED(hr) || !range) return FAILED(hr) ? hr : E_FAIL;
        }
        const auto hr = range->SetText(cookie, 0, text.data(), static_cast<LONG>(text.size()));
        if (FAILED(hr)) return hr;
        if (text.empty()) return finish(cookie);
        size_t offset = 0;
        for (uint32_t rune = 0; offset < text.size() && rune < frame.cursor; ++rune) {
            const auto ch = text[offset++];
            if (ch >= 0xd800 && ch <= 0xdbff && offset < text.size()) ++offset;
        }
        return place_cursor(context, cookie, range.get(), static_cast<LONG>(offset));
    }

    HRESULT finish(TfEditCookie cookie) {
        if (!composition_) return S_OK;
        ComPtr<ITfComposition> value;
        value.retain(composition_.get());
        composition_.reset();
        return value->EndComposition(cookie);
    }

    HRESULT commit(ITfContext* context, TfEditCookie cookie, const Frame& frame) {
        const auto text = wide(frame.text);
        if (composition_) {
            ComPtr<ITfRange> range;
            auto hr = composition_->GetRange(range.put());
            if (FAILED(hr) || !range) return FAILED(hr) ? hr : E_FAIL;
            hr = range->SetText(cookie, 0, text.data(), static_cast<LONG>(text.size()));
            if (FAILED(hr)) return hr;
            place_cursor(context, cookie, range.get(), static_cast<LONG>(text.size()));
            return finish(cookie);
        }
        if (text.empty()) return S_OK;
        ComPtr<ITfInsertAtSelection> insert;
        const auto hr = context->QueryInterface(__uuidof(ITfInsertAtSelection),
                                                reinterpret_cast<void**>(insert.put()));
        if (FAILED(hr)) return hr;
        return insert->InsertTextAtSelection(cookie, TF_IAS_NOQUERY, text.data(),
                                             static_cast<LONG>(text.size()), nullptr);
    }

    HRESULT edit(std::vector<Frame> frames, DWORD flags, bool finish_composition = false,
                 Frame* caret = nullptr) {
        if (!context_) return S_FALSE;
        auto* request = new Edit(this, context_.get(), session_, std::move(frames), finish_composition, caret);
        HRESULT result = E_FAIL;
        const auto hr = context_->RequestEditSession(client_, request, flags, &result);
        request->Release();
        return FAILED(hr) ? hr : result;
    }

    void post_session(Type type, bool on = false) {
        Frame frame;
        frame.type = type;
        frame.session = session_;
        frame.focused = on;
        pipe_.post(std::move(frame));
    }

    bool ensure_session() {
        if (!session_ || !pipe_.connected()) return false;
        const auto generation = pipe_.generation();
        if (opened_generation_ == generation) return true;
        Frame frame;
        frame.type = Type::open;
        frame.session = session_;
        if (!pipe_.post(std::move(frame))) return false;
        opened_generation_ = generation;
        post_session(Type::focus, true);
        post_session(Type::menu_open);
        return true;
    }

    void end_session() {
        pending_.reset();
        if (!session_) return;
        if (opened_generation_ == pipe_.generation()) {
            post_session(Type::focus, false);
            post_session(Type::close);
        }
        if (composition_) edit({}, TF_ES_SYNC | TF_ES_READWRITE, true);
        composition_.reset();
        context_.reset();
        session_ = opened_generation_ = 0;
        if (bar_) bar_->session(0);
    }

    void use_context(ITfContext* context) {
        if (context_.get() == context) return;
        end_session();
        if (!context || !foreground_) return;
        context_.retain(context);
        session_ = ++next_session_;
        if (bar_) bar_->session(session_);
        ensure_session();
    }

    static LRESULT CALLBACK window_proc(HWND window, UINT message, WPARAM w, LPARAM l) {
        if (message == WM_NCCREATE) {
            const auto* create = reinterpret_cast<CREATESTRUCTW*>(l);
            SetWindowLongPtrW(window, GWLP_USERDATA, reinterpret_cast<LONG_PTR>(create->lpCreateParams));
        }
        if (message == bridge_message) {
            auto* tip = reinterpret_cast<Tip*>(GetWindowLongPtrW(window, GWLP_USERDATA));
            if (tip) {
                try { tip->receive(); }
                catch (...) {}
            }
            return 0;
        }
        return DefWindowProcW(window, message, w, l);
    }

    void receive() {
        if (!pipe_.connected()) {
            opened_generation_ = 0;
            pending_.reset();
            if (composition_) edit({}, TF_ES_ASYNCDONTCARE | TF_ES_READWRITE, true);
            return;
        }
        ensure_session();
        for (auto& frame : pipe_.received()) {
            if ((frame.type == Type::mode || frame.type == Type::menu) && bar_) bar_->update(std::move(frame));
            else if (frame.session == session_ && (frame.type == Type::compose || frame.type == Type::commit)) {
                std::vector<Frame> edits;
                edits.push_back(std::move(frame));
                edit(std::move(edits), TF_ES_ASYNCDONTCARE | TF_ES_READWRITE);
            }
        }
    }

    static void translate(Frame& frame, WPARAM key, LPARAM scan, bool down_event) {
        frame.vk = static_cast<uint16_t>(key);
        const auto bits = static_cast<uint32_t>(scan);
        frame.scan = (bits >> 16) & 0xff;
        frame.flags = down_event ? down : up;
        if (bits & (1u << 24)) frame.flags |= extended;
        if (down_event && bits & (1u << 30)) frame.flags |= repeat;
        BYTE keys[256]{};
        if (!GetKeyboardState(keys)) return;
        if (keys[VK_SHIFT] & 0x80) frame.modifiers |= shift;
        if (keys[VK_CONTROL] & 0x80) frame.modifiers |= ctrl;
        if (keys[VK_MENU] & 0x80) frame.modifiers |= alt;
        if ((keys[VK_LWIN] | keys[VK_RWIN]) & 0x80) frame.modifiers |= meta;
        if (keys[VK_CAPITAL] & 1) frame.modifiers |= caps_lock;
        if (!down_event) {
            // GetKeyboardState may still include the key TSF is releasing.
            const bool right = (frame.flags & extended) != 0;
            if (key == VK_SHIFT || key == VK_LSHIFT || key == VK_RSHIFT) {
                const bool right_shift = key == VK_RSHIFT || frame.scan == 0x36;
                if (!(keys[right_shift ? VK_LSHIFT : VK_RSHIFT] & 0x80)) frame.modifiers &= ~shift;
            } else if (key == VK_CONTROL || key == VK_LCONTROL || key == VK_RCONTROL) {
                if (!(keys[right || key == VK_RCONTROL ? VK_LCONTROL : VK_RCONTROL] & 0x80))
                    frame.modifiers &= ~ctrl;
            } else if (key == VK_MENU || key == VK_LMENU || key == VK_RMENU) {
                if (!(keys[right || key == VK_RMENU ? VK_LMENU : VK_RMENU] & 0x80))
                    frame.modifiers &= ~alt;
            } else if (key == VK_LWIN || key == VK_RWIN) {
                if (!(keys[key == VK_RWIN ? VK_LWIN : VK_RWIN] & 0x80)) frame.modifiers &= ~meta;
            }
            return;
        }
        wchar_t text[16]{};
        const int n = ToUnicodeEx(static_cast<UINT>(key), frame.scan, keys,
                                  text, ARRAYSIZE(text), 4, GetKeyboardLayout(0));
        if (n > 0 && n <= int(ARRAYSIZE(text))) frame.text = utf16(std::wstring_view(text, n));
    }

    HRESULT test_key(ITfContext* context, WPARAM key, LPARAM scan, BOOL* eaten, bool pressed) {
        if (!eaten) return E_POINTER;
        *eaten = FALSE;
        try {
            pending_.reset();
            if (!context || key == VK_PACKET || !pipe_.connected()) return S_OK;
            if (!foreground_) foreground_ = true;
            use_context(context);
            if (!ensure_session()) return S_OK;
            Frame frame;
            frame.session = session_;
            translate(frame, key, scan, pressed);
            edit({}, TF_ES_SYNC | TF_ES_READ, false, &frame);
            auto result = pipe_.test(std::move(frame), std::chrono::steady_clock::now() + key_timeout);
            if (!result) return S_OK;
            if (!result->eaten) {
                if (!result->edits.empty())
                    edit(std::move(result->edits), TF_ES_ASYNCDONTCARE | TF_ES_READWRITE);
                return S_OK;
            }
            tested_key_ = key;
            tested_scan_ = scan;
            tested_down_ = pressed;
            pending_ = std::move(result);
            *eaten = TRUE;
        } catch (...) {
            pending_.reset();
        }
        return S_OK;
    }

    HRESULT key_event(ITfContext* context, WPARAM key, LPARAM scan, BOOL* eaten, bool pressed) {
        if (!eaten) return E_POINTER;
        *eaten = FALSE;
        try {
            auto result = std::move(pending_);
            pending_.reset();
            if (context != context_.get() || !result || key != tested_key_ ||
                scan != tested_scan_ || pressed != tested_down_) return S_OK;
            if (result->edits.empty() || edit(std::move(result->edits), TF_ES_SYNC | TF_ES_READWRITE) == S_OK)
                *eaten = TRUE;
        } catch (...) {}
        return S_OK;
    }

public:
    explicit Tip(const Config& config) : config_(config) { ++references; }
    ~Tip() { pipe_.stop(); --references; }

    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID iid, void** out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (iid == IID_IUnknown || iid == processor_ex_iid || iid == __uuidof(ITfTextInputProcessor))
            *out = static_cast<TipProcessorEx*>(this);
        else if (iid == __uuidof(ITfKeyEventSink)) *out = static_cast<ITfKeyEventSink*>(this);
        else if (iid == __uuidof(ITfThreadMgrEventSink)) *out = static_cast<ITfThreadMgrEventSink*>(this);
        else if (iid == __uuidof(ITfCompositionSink)) *out = static_cast<ITfCompositionSink*>(this);
        if (!*out) return E_NOINTERFACE;
        AddRef();
        return S_OK;
    }

    ULONG STDMETHODCALLTYPE AddRef() override { return ++refs_; }
    ULONG STDMETHODCALLTYPE Release() override {
        const auto count = --refs_;
        if (!count) delete this;
        return count;
    }

    HRESULT STDMETHODCALLTYPE Activate(ITfThreadMgr* manager, TfClientId client) override {
        return ActivateEx(manager, client, 0);
    }
    HRESULT STDMETHODCALLTYPE ActivateEx(ITfThreadMgr* manager, TfClientId client, DWORD) override {
        if (!manager) return E_INVALIDARG;
        try {
            manager_.retain(manager);
            client_ = client;
            ComPtr<ITfKeystrokeMgr> keys;
            auto hr = manager->QueryInterface(__uuidof(ITfKeystrokeMgr), reinterpret_cast<void**>(keys.put()));
            if (SUCCEEDED(hr)) hr = keys->AdviseKeyEventSink(client_, this, TRUE);
            key_sink_ = SUCCEEDED(hr);
            ComPtr<ITfSource> source;
            if (SUCCEEDED(hr)) hr = manager->QueryInterface(__uuidof(ITfSource),
                                                            reinterpret_cast<void**>(source.put()));
            if (SUCCEEDED(hr)) hr = source->AdviseSink(__uuidof(ITfThreadMgrEventSink),
                static_cast<ITfThreadMgrEventSink*>(this), &cookie_);
            if (FAILED(hr)) {
                Deactivate();
                return hr;
            }
            const wchar_t window_class[] = L"TangoIMEBridgeMessage";
            WNDCLASSW wc{};
            wc.lpfnWndProc = window_proc;
            wc.hInstance = module;
            wc.lpszClassName = window_class;
            RegisterClassW(&wc);
            window_ = CreateWindowExW(0, window_class, nullptr, 0, 0, 0, 0, 0,
                                      HWND_MESSAGE, nullptr, module, this);
            if (window_) pipe_.start(config_, window_);
            if (SUCCEEDED(manager->QueryInterface(__uuidof(ITfLangBarItemMgr),
                                                   reinterpret_cast<void**>(bar_manager_.put())))) {
                *bar_.put() = new LangBar(config_, &pipe_);
                if (FAILED(bar_manager_->AddItem(bar_.get()))) bar_.reset();
            }
            ComPtr<ITfDocumentMgr> focus;
            if (SUCCEEDED(manager->GetFocus(focus.put())) && focus) OnSetFocus(focus.get(), nullptr);
            return S_OK;
        } catch (...) {
            Deactivate();
            return E_FAIL;
        }
    }

    HRESULT STDMETHODCALLTYPE Deactivate() override {
        try { end_session(); }
        catch (...) {}
        pipe_.stop();
        if (window_) DestroyWindow(window_);
        window_ = nullptr;
        if (bar_) {
            bar_->detach();
            bar_manager_->RemoveItem(bar_.get());
            bar_.reset();
        }
        bar_manager_.reset();
        if (manager_) {
            ComPtr<ITfKeystrokeMgr> keys;
            if (key_sink_ && SUCCEEDED(manager_->QueryInterface(__uuidof(ITfKeystrokeMgr),
                                                                reinterpret_cast<void**>(keys.put()))))
                keys->UnadviseKeyEventSink(client_);
            ComPtr<ITfSource> source;
            if (cookie_ != TF_INVALID_COOKIE && SUCCEEDED(manager_->QueryInterface(__uuidof(ITfSource),
                                                                                   reinterpret_cast<void**>(source.put()))))
                source->UnadviseSink(cookie_);
        }
        key_sink_ = false;
        cookie_ = TF_INVALID_COOKIE;
        client_ = 0;
        manager_.reset();
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE OnSetFocus(BOOL foreground) override {
        foreground_ = foreground != FALSE;
        try {
            if (!foreground_) end_session();
            else {
                ComPtr<ITfDocumentMgr> focus;
                if (manager_ && SUCCEEDED(manager_->GetFocus(focus.put()))) OnSetFocus(focus.get(), nullptr);
            }
        } catch (...) {}
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnInitDocumentMgr(ITfDocumentMgr*) override { return S_OK; }
    HRESULT STDMETHODCALLTYPE OnUninitDocumentMgr(ITfDocumentMgr*) override { return S_OK; }
    HRESULT STDMETHODCALLTYPE OnSetFocus(ITfDocumentMgr* focused, ITfDocumentMgr*) override {
        ComPtr<ITfContext> context;
        if (focused) focused->GetTop(context.put());
        try { use_context(context.get()); }
        catch (...) {}
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnPushContext(ITfContext* context) override {
        try { use_context(context); }
        catch (...) {}
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnPopContext(ITfContext* context) override {
        try { if (context == context_.get()) end_session(); }
        catch (...) {}
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnCompositionTerminated(TfEditCookie, ITfComposition* composition) override {
        if (composition == composition_.get()) {
            composition_.reset();
            try {
                end_session();
            } catch (...) {}
        }
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE OnTestKeyDown(ITfContext* context, WPARAM key, LPARAM scan, BOOL* eaten) override {
        return test_key(context, key, scan, eaten, true);
    }
    HRESULT STDMETHODCALLTYPE OnTestKeyUp(ITfContext* context, WPARAM key, LPARAM scan, BOOL* eaten) override {
        return test_key(context, key, scan, eaten, false);
    }
    HRESULT STDMETHODCALLTYPE OnKeyDown(ITfContext* context, WPARAM key, LPARAM scan, BOOL* eaten) override {
        return key_event(context, key, scan, eaten, true);
    }
    HRESULT STDMETHODCALLTYPE OnKeyUp(ITfContext* context, WPARAM key, LPARAM scan, BOOL* eaten) override {
        return key_event(context, key, scan, eaten, false);
    }
    HRESULT STDMETHODCALLTYPE OnPreservedKey(ITfContext*, REFGUID, BOOL* eaten) override {
        if (!eaten) return E_POINTER;
        *eaten = FALSE;
        return S_OK;
    }
};

} // namespace

HRESULT create_tip(const Config& config, REFIID iid, void** out) noexcept {
    try {
        auto* tip = new Tip(config);
        const auto hr = tip->QueryInterface(iid, out);
        tip->Release();
        return hr;
    } catch (...) {
        return E_OUTOFMEMORY;
    }
}

} // namespace tango::ime
