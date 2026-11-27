#pragma once
#include "tsf_compat.h"

namespace tango::ime {

class Pipe;

class LangBar final : public TipLangBarButton, public ITfSource {
    ULONG refs_ = 1;
    Config config_;
    Pipe* pipe_ = nullptr;
    uint64_t session_ = 0;
    DWORD status_ = 0;
    ComPtr<ITfLangBarItemSink> sink_;
    Frame mode_, menu_;
    std::vector<MenuItem> visible_;
public:
    LangBar(const Config&, Pipe*);
    ~LangBar();
    void session(uint64_t id) { session_ = id; }
    void detach() { pipe_ = nullptr; sink_.reset(); }
    void update(Frame frame);
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID, void**) override;
    ULONG STDMETHODCALLTYPE AddRef() override;
    ULONG STDMETHODCALLTYPE Release() override;
    HRESULT STDMETHODCALLTYPE GetInfo(TF_LANGBARITEMINFO*) override;
    HRESULT STDMETHODCALLTYPE GetStatus(DWORD*) override;
    HRESULT STDMETHODCALLTYPE Show(BOOL) override;
    HRESULT STDMETHODCALLTYPE GetTooltipString(BSTR*) override;
    HRESULT STDMETHODCALLTYPE OnClick(int, POINT, const RECT*) override;
    HRESULT STDMETHODCALLTYPE InitMenu(TipMenu*) override;
    HRESULT STDMETHODCALLTYPE OnMenuSelect(UINT) override;
    HRESULT STDMETHODCALLTYPE GetIcon(HICON*) override;
    HRESULT STDMETHODCALLTYPE GetText(BSTR*) override;
    HRESULT STDMETHODCALLTYPE AdviseSink(REFIID, IUnknown*, DWORD*) override;
    HRESULT STDMETHODCALLTYPE UnadviseSink(DWORD) override;
};

} // namespace tango::ime
