// Windows Text Services Framework bridge for Tango's input method example.
// The DLL runs in the editor's process. The Go application owns the engine.
#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <msctf.h>
#include <sddl.h>
#include <algorithm>
#include <atomic>
#include <cwchar>
#include <new>
#include <string>

#ifndef TANGO_IME_CONNECTION
#define TANGO_IME_CONNECTION L"com.tangoui.inputmethod.example_Connection"
#endif
#ifndef TANGO_IME_HOST
#define TANGO_IME_HOST L"TangoPinyin.exe"
#endif

namespace {
const CLSID kClass = {0x6c765db9, 0x4b36, 0x466b, {0xac, 0x9c, 0x9f, 0x19, 0x59, 0x5e, 0x76, 0x70}};
const GUID kProfile = {0xc224e24b, 0x7e89, 0x468f, {0xb4, 0x4a, 0xc9, 0x90, 0xa2, 0x90, 0x7c, 0x14}};
constexpr wchar_t kName[] = L"Tango Pinyin";
constexpr LANGID kLanguage = MAKELANGID(LANG_CHINESE, SUBLANG_CHINESE_SIMPLIFIED);
constexpr DWORD kVersion = 0x31454d49;
enum Action : DWORD { ActivateSession = 1, Key = 2, Reset = 3, End = 4, Poll = 5 };
// More: committed text is still queued; poll for it before applying marked text.
enum Flags : DWORD { Handled = 1, Marked = 2, Commit = 4, More = 8 };
// The host queues at most 64K UTF-16 units of committed text per session.
constexpr int kMaxParts = 65;
// Poll quickly while composing or just after activity, slower when idle.
constexpr UINT kFastPoll = 160, kSlowPoll = 1280;

struct Request {
    DWORD version{kVersion}, action{};
    GUID token{};
    DWORD pid{GetCurrentProcessId()};
    LONG x{}, y{}, w{}, h{};
    DWORD key{}, modifiers{};
    wchar_t text[8]{};
};
struct Response {
    DWORD version{}, flags{}, cursor{}, markedLength{}, committedLength{};
    wchar_t marked[1024]{}, committed[1024]{};
};
static_assert(sizeof(Request) == 68);
static_assert(sizeof(Response) == 4116);
// Result joins the parts of one exchange.
struct Result {
    DWORD flags{}, cursor{};
    std::wstring marked, committed;
};

HMODULE module;
std::atomic<long> references{0};

template <typename T> class ComPtr {
    T* value_{};
public:
    ComPtr() = default;
    ~ComPtr() { reset(); }
    ComPtr(const ComPtr&) = delete;
    ComPtr& operator=(const ComPtr&) = delete;
    T* get() const { return value_; }
    T* operator->() const { return value_; }
    explicit operator bool() const { return value_ != nullptr; }
    void reset() { if (auto* prev = value_) { value_ = nullptr; prev->Release(); } }
    T** put() { reset(); return &value_; }
    void retain(T* value) {
        if (value) value->AddRef();
        reset();
        value_ = value;
    }
};

std::wstring ModulePath() {
    wchar_t path[MAX_PATH]{};
    const DWORD length = GetModuleFileNameW(module, path, ARRAYSIZE(path));
    return length && length < ARRAYSIZE(path) ? std::wstring(path, length) : std::wstring();
}

DWORD IntegrityLevel(HANDLE process);

std::wstring PipeName() {
    HANDLE token;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) return {};
    alignas(TOKEN_USER) BYTE storage[SECURITY_MAX_SID_SIZE + sizeof(TOKEN_USER)]{};
    DWORD size;
    const BOOL ok = GetTokenInformation(token, TokenUser, storage, sizeof(storage), &size);
    CloseHandle(token);
    if (!ok) return {};
    wchar_t* sid{};
    if (!ConvertSidToStringSidW(reinterpret_cast<TOKEN_USER*>(storage)->User.Sid, &sid)) return {};
    DWORD session{};
    ProcessIdToSessionId(GetCurrentProcessId(), &session);
    std::wstring path = L"\\\\.\\pipe\\TangoIME-";
    path += sid;
    LocalFree(sid);
    return path + L"-" + std::to_wstring(session) + L"-" +
           std::to_wstring(IntegrityLevel(GetCurrentProcess())) + L"-" TANGO_IME_CONNECTION;
}

bool Transfer(HANDLE pipe, bool write, void* data, DWORD size, DWORD timeout) {
    OVERLAPPED async{};
    async.hEvent = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    if (!async.hEvent) return false;
    DWORD done{};
    BOOL ok = write ? WriteFile(pipe, data, size, &done, &async)
                    : ReadFile(pipe, data, size, &done, &async);
    if (!ok && GetLastError() == ERROR_IO_PENDING) {
        if (WaitForSingleObject(async.hEvent, timeout) == WAIT_OBJECT_0) {
            ok = GetOverlappedResult(pipe, &async, &done, FALSE);
        } else {
            CancelIoEx(pipe, &async);
            GetOverlappedResult(pipe, &async, &done, TRUE);
        }
    }
    CloseHandle(async.hEvent);
    return ok && done == size;
}

std::wstring HostPath() {
    auto path = ModulePath();
    const auto slash = path.find_last_of(L'\\');
    if (slash == std::wstring::npos) return {};
    path.resize(slash + 1);
    return path + TANGO_IME_HOST;
}

DWORD IntegrityLevel(HANDLE process) {
    HANDLE token;
    if (!OpenProcessToken(process, TOKEN_QUERY, &token)) return 0;
    alignas(TOKEN_MANDATORY_LABEL) BYTE label[sizeof(TOKEN_MANDATORY_LABEL) + SECURITY_MAX_SID_SIZE]{};
    DWORD bytes;
    const BOOL ok = GetTokenInformation(token, TokenIntegrityLevel, label, sizeof(label), &bytes);
    CloseHandle(token);
    if (!ok) return 0;
    const PSID sid = reinterpret_cast<TOKEN_MANDATORY_LABEL*>(label)->Label.Sid;
    return *GetSidSubAuthority(sid, *GetSidSubAuthorityCount(sid) - 1);
}

bool VerifyHost(HANDLE pipe) {
    ULONG pid{};
    if (!GetNamedPipeServerProcessId(pipe, &pid)) return false;
    HANDLE process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid);
    if (!process) return false;
    wchar_t path[MAX_PATH]{};
    DWORD length = ARRAYSIZE(path);
    const bool matches = QueryFullProcessImageNameW(process, 0, path, &length) &&
        CompareStringOrdinal(path, length, HostPath().c_str(), -1, TRUE) == CSTR_EQUAL;
    const DWORD integrity = IntegrityLevel(process);
    CloseHandle(process);
    return matches && integrity && integrity == IntegrityLevel(GetCurrentProcess());
}

bool ExchangeOnce(const std::wstring& pipeName, const Request& request, Response& response) {
    if (pipeName.empty()) return false;
    HANDLE pipe = INVALID_HANDLE_VALUE;
    const auto deadline = GetTickCount64() + 15;
    do {
        pipe = CreateFileW(pipeName.c_str(), GENERIC_READ | GENERIC_WRITE, 0, nullptr,
                           OPEN_EXISTING, FILE_FLAG_OVERLAPPED | SECURITY_SQOS_PRESENT |
                           SECURITY_IDENTIFICATION, nullptr);
        if (pipe != INVALID_HANDLE_VALUE) break;
        if (GetLastError() != ERROR_PIPE_BUSY) return false;
        WaitNamedPipeW(pipeName.c_str(), 4);
    } while (GetTickCount64() < deadline);
    if (pipe == INVALID_HANDLE_VALUE) return false;
    const bool done = VerifyHost(pipe) &&
                      Transfer(pipe, true, const_cast<Request*>(&request), sizeof(request), 20) &&
                      Transfer(pipe, false, &response, sizeof(response), 100);
    CloseHandle(pipe);
    return done && response.version == kVersion && response.markedLength <= ARRAYSIZE(response.marked) &&
           response.committedLength <= ARRAYSIZE(response.committed) && response.cursor <= response.markedLength;
}

// Exchange sends request and polls for any parts of committed text that did
// not fit in the first response. Parts that fail to arrive stay queued.
bool Exchange(const std::wstring& pipeName, Request request, Result& result) {
    result = Result{};
    for (int part = 0; part < kMaxParts; ++part) {
        Response response;
        if (!ExchangeOnce(pipeName, request, response)) return part > 0;
        if (part == 0) result.flags |= response.flags & Handled;
        if (response.flags & Commit) {
            result.flags |= Commit;
            result.committed.append(response.committed, response.committedLength);
        }
        if (!(response.flags & More)) {
            if (response.flags & Marked) {
                result.flags |= Marked;
                result.marked.assign(response.marked, response.markedLength);
                result.cursor = response.cursor;
            }
            return true;
        }
        request.action = Poll;
    }
    return true;
}

void StartHost() {
    const auto path = HostPath();
    if (path.empty()) return;
    STARTUPINFOW startup{};
    startup.cb = sizeof(startup);
    PROCESS_INFORMATION process{};
    // Start the adjacent companion as the current user, without a focus change.
    if (CreateProcessW(path.c_str(), nullptr, nullptr, nullptr, FALSE, CREATE_NO_WINDOW,
                       nullptr, nullptr, &startup, &process)) {
        CloseHandle(process.hThread);
        CloseHandle(process.hProcess);
    }
}

class Tip final : public ITfTextInputProcessor, public ITfKeyEventSink,
                  public ITfThreadMgrEventSink, public ITfCompositionSink {
    ULONG refs_{1};
    TfClientId id_{0};
    DWORD sinkCookie_{TF_INVALID_COOKIE};
    ComPtr<ITfThreadMgr> mgr_;
    ComPtr<ITfContext> context_;
    ComPtr<ITfComposition> composition_;
    GUID token_{};
    std::wstring pipeName_;
    ULONGLONG startAfter_{};
    UINT_PTR timer_{};
    UINT interval_{kFastPoll};
    static thread_local Tip* polling_;

    class Edit final : public ITfEditSession {
        ULONG refs_{1};
        Tip* owner_;
        ITfContext* context_;
        Request request_;
        BOOL* handled_;
    public:
        Edit(Tip* owner, ITfContext* context, Request request, BOOL* handled)
            : owner_(owner), context_(context), request_(request), handled_(handled) {
            owner_->AddRef();
            context_->AddRef();
        }
        ~Edit() { context_->Release(); owner_->Release(); }
        HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id, void** out) override {
            if (!out) return E_POINTER;
            *out = nullptr;
            if (id != IID_IUnknown && id != __uuidof(ITfEditSession)) return E_NOINTERFACE;
            *out = static_cast<ITfEditSession*>(this);
            AddRef();
            return S_OK;
        }
        ULONG STDMETHODCALLTYPE AddRef() override { return ++refs_; }
        ULONG STDMETHODCALLTYPE Release() override {
            const auto n = --refs_;
            if (!n) delete this;
            return n;
        }
        HRESULT STDMETHODCALLTYPE DoEditSession(TfEditCookie cookie) override {
            return owner_->RunEdit(context_, cookie, request_, handled_);
        }
    };

    static bool Selection(ITfContext* context, TfEditCookie cookie, ComPtr<ITfRange>& range) {
        TF_SELECTION selection{};
        ULONG fetched{};
        if (FAILED(context->GetSelection(cookie, TF_DEFAULT_SELECTION, 1, &selection, &fetched)) ||
            fetched != 1) return false;
        *range.put() = selection.range;
        return true;
    }

    static void AddCaret(ITfContext* context, TfEditCookie cookie, Request& request) {
        ComPtr<ITfRange> range;
        ComPtr<ITfContextView> view;
        if (!Selection(context, cookie, range) || FAILED(context->GetActiveView(view.put()))) return;
        range->Collapse(cookie, TF_ANCHOR_END);
        RECT rect{};
        BOOL clipped{};
        if (FAILED(view->GetTextExt(cookie, range.get(), &rect, &clipped)) || clipped) return;
        HWND hwnd{};
        if (SUCCEEDED(view->GetWnd(&hwnd)) && hwnd) {
            POINT start{rect.left, rect.top}, end{rect.right, rect.bottom};
            if (PhysicalToLogicalPointForPerMonitorDPI(hwnd, &start) &&
                PhysicalToLogicalPointForPerMonitorDPI(hwnd, &end)) {
                rect = RECT{start.x, start.y, end.x, end.y};
            }
        }
        request.x = rect.left;
        request.y = rect.top;
        request.w = rect.right - rect.left;
        request.h = rect.bottom - rect.top;
    }

    HRESULT SetMarked(ITfContext* context, TfEditCookie cookie, const Result& r) {
        const auto markedLength = static_cast<LONG>(r.marked.size());
        ComPtr<ITfRange> range;
        if (!composition_) {
            if (!markedLength) return S_OK;
            ComPtr<ITfInsertAtSelection> insert;
            HRESULT hr = context->QueryInterface(__uuidof(ITfInsertAtSelection),
                                                 reinterpret_cast<void**>(insert.put()));
            if (FAILED(hr)) return hr;
            hr = insert->InsertTextAtSelection(cookie, TF_IAS_QUERYONLY, nullptr, 0, range.put());
            if (FAILED(hr)) return hr;
            ComPtr<ITfContextComposition> own;
            hr = context->QueryInterface(__uuidof(ITfContextComposition),
                                         reinterpret_cast<void**>(own.put()));
            if (FAILED(hr)) return hr;
            hr = own->StartComposition(cookie, range.get(), this, composition_.put());
            if (FAILED(hr) || !composition_) return FAILED(hr) ? hr : E_FAIL;
        } else {
            HRESULT hr = composition_->GetRange(range.put());
            if (FAILED(hr)) return hr;
        }
        HRESULT hr = range->SetText(cookie, 0, r.marked.data(), markedLength);
        if (FAILED(hr)) return hr;
        if (!markedLength) return FinishComposition(cookie);
        hr = range->Collapse(cookie, TF_ANCHOR_START);
        if (FAILED(hr)) return hr;
        LONG moved;
        hr = range->ShiftEnd(cookie, static_cast<LONG>(r.cursor), &moved, nullptr);
        if (FAILED(hr)) return hr;
        hr = range->Collapse(cookie, TF_ANCHOR_END);
        if (FAILED(hr)) return hr;
        TF_SELECTION selection{range.get(), {TF_AE_NONE, FALSE}};
        return context->SetSelection(cookie, 1, &selection);
    }

    HRESULT FinishComposition(TfEditCookie cookie) {
        if (!composition_) return S_OK;
        ComPtr<ITfComposition> current;
        current.retain(composition_.get());
        composition_.reset();
        return current->EndComposition(cookie);
    }

    HRESULT CommitText(ITfContext* context, TfEditCookie cookie, const Result& r) {
        const auto committedLength = static_cast<LONG>(r.committed.size());
        if (composition_) {
            ComPtr<ITfRange> range;
            HRESULT hr = composition_->GetRange(range.put());
            if (SUCCEEDED(hr)) hr = range->SetText(cookie, 0, r.committed.data(), committedLength);
            if (FAILED(hr)) return hr;
            range->Collapse(cookie, TF_ANCHOR_END);
            TF_SELECTION selection{range.get(), {TF_AE_NONE, FALSE}};
            context->SetSelection(cookie, 1, &selection);
            return FinishComposition(cookie);
        }
        if (!committedLength) return S_OK;
        ComPtr<ITfInsertAtSelection> insert;
        HRESULT hr = context->QueryInterface(__uuidof(ITfInsertAtSelection),
                                             reinterpret_cast<void**>(insert.put()));
        if (FAILED(hr)) return hr;
        return insert->InsertTextAtSelection(cookie, TF_IAS_NOQUERY,
                                             r.committed.data(), committedLength, nullptr);
    }

    HRESULT RunEdit(ITfContext* context, TfEditCookie cookie, Request request, BOOL* handled) {
        AddCaret(context, cookie, request);
        Result response;
        const bool exchanged = Exchange(pipeName_, request, response);
        SchedulePoll(request.action != Poll || response.flags & (Marked | Commit));
        if (!exchanged) {
            if (request.action == End) FinishComposition(cookie);
            return S_OK;
        }
        HRESULT hr = S_OK;
        if (response.flags & Commit) hr = CommitText(context, cookie, response);
        if (SUCCEEDED(hr) && response.flags & Marked) hr = SetMarked(context, cookie, response);
        if (request.action == End) FinishComposition(cookie);
        if (handled && SUCCEEDED(hr)) *handled = (response.flags & Handled) != 0;
        return hr;
    }

    // SchedulePoll resets the poll interval after activity or while composing,
    // and otherwise doubles it up to kSlowPoll.
    void SchedulePoll(bool active) {
        if (!timer_) return;
        const UINT next = active || composition_ ? kFastPoll : std::min(interval_ * 2, kSlowPoll);
        if (next == interval_) return;
        if (const UINT_PTR id = SetTimer(nullptr, timer_, next, Timer)) {
            timer_ = id;
            interval_ = next;
        }
    }

    void EditContext(ITfContext* context, Request request, BOOL* handled = nullptr) {
        if (!context || IsEqualGUID(request.token, GUID{})) return;
        if (auto* edit = new(std::nothrow) Edit(this, context, request, handled)) {
            HRESULT result;
            context->RequestEditSession(id_, edit, TF_ES_SYNC | TF_ES_READWRITE, &result);
            edit->Release();
        }
    }

    void EndSession() {
        if (!context_) return;
        Request request;
        request.action = End;
        request.token = token_;
        EditContext(context_.get(), request);
        composition_.reset();
        context_.reset();
        token_ = GUID{};
    }

    void UseContext(ITfContext* context) {
        if (context_.get() == context) return;
        EndSession();
        if (!context || FAILED(CoCreateGuid(&token_))) return;
        context_.retain(context);
        Request request;
        request.action = ActivateSession;
        request.token = token_;
        Result response;
        if (!Exchange(pipeName_, request, response) && GetTickCount64() >= startAfter_) {
            startAfter_ = GetTickCount64() + 1000;
            StartHost();
        }
        SchedulePoll(true);
    }

    static void CALLBACK Timer(HWND, UINT, UINT_PTR, DWORD) {
        if (auto* tip = polling_; tip && tip->context_) {
            Request request;
            request.action = Poll;
            request.token = tip->token_;
            tip->EditContext(tip->context_.get(), request);
        }
    }

    static void Translate(Request& request, WPARAM vk, LPARAM scan) {
        request.key = static_cast<DWORD>(vk);
        BYTE keys[256]{};
        if (!GetKeyboardState(keys)) return;
        if (keys[VK_SHIFT] & 0x80) request.modifiers |= 1;
        if (keys[VK_CONTROL] & 0x80) request.modifiers |= 2;
        if (keys[VK_MENU] & 0x80) request.modifiers |= 4;
        if ((keys[VK_LWIN] | keys[VK_RWIN]) & 0x80) request.modifiers |= 8;
        if (scan & (1L << 24)) request.modifiers |= 16;
        // Flag 4 prevents ToUnicodeEx from changing the keyboard's dead-key state.
        const int n = ToUnicodeEx(static_cast<UINT>(vk), (scan >> 16) & 0xff, keys,
                                  request.text, ARRAYSIZE(request.text) - 1, 4, GetKeyboardLayout(0));
        if (n <= 0) request.text[0] = L'\0';
    }

public:
    Tip() : pipeName_(PipeName()) { ++references; }
    ~Tip() { --references; }
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id, void** out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (id == IID_IUnknown || id == __uuidof(ITfTextInputProcessor))
            *out = static_cast<ITfTextInputProcessor*>(this);
        else if (id == __uuidof(ITfKeyEventSink)) *out = static_cast<ITfKeyEventSink*>(this);
        else if (id == __uuidof(ITfThreadMgrEventSink)) *out = static_cast<ITfThreadMgrEventSink*>(this);
        else if (id == __uuidof(ITfCompositionSink)) *out = static_cast<ITfCompositionSink*>(this);
        else return E_NOINTERFACE;
        AddRef();
        return S_OK;
    }
    ULONG STDMETHODCALLTYPE AddRef() override { return ++refs_; }
    ULONG STDMETHODCALLTYPE Release() override {
        const auto n = --refs_;
        if (!n) delete this;
        return n;
    }

    HRESULT STDMETHODCALLTYPE Activate(ITfThreadMgr* manager, TfClientId id) override {
        id_ = id;
        mgr_.retain(manager);
        ComPtr<ITfKeystrokeMgr> keys;
        HRESULT hr = mgr_->QueryInterface(__uuidof(ITfKeystrokeMgr),
                                          reinterpret_cast<void**>(keys.put()));
        if (SUCCEEDED(hr)) hr = keys->AdviseKeyEventSink(id_, this, TRUE);
        ComPtr<ITfSource> source;
        if (SUCCEEDED(hr)) hr = mgr_->QueryInterface(__uuidof(ITfSource),
                                                    reinterpret_cast<void**>(source.put()));
        if (SUCCEEDED(hr)) hr = source->AdviseSink(__uuidof(ITfThreadMgrEventSink),
                                                 static_cast<ITfThreadMgrEventSink*>(this), &sinkCookie_);
        if (FAILED(hr)) {
            Deactivate();
            return hr;
        }
        ComPtr<ITfDocumentMgr> focused;
        if (SUCCEEDED(mgr_->GetFocus(focused.put())) && focused) OnSetFocus(focused.get(), nullptr);
        polling_ = this;
        interval_ = kFastPoll;
        timer_ = SetTimer(nullptr, 0, interval_, Timer);
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE Deactivate() override {
        if (timer_) KillTimer(nullptr, timer_);
        timer_ = 0;
        if (polling_ == this) polling_ = nullptr;
        EndSession();
        if (mgr_) {
            ComPtr<ITfKeystrokeMgr> keys;
            if (SUCCEEDED(mgr_->QueryInterface(__uuidof(ITfKeystrokeMgr),
                                               reinterpret_cast<void**>(keys.put()))))
                keys->UnadviseKeyEventSink(id_);
            ComPtr<ITfSource> source;
            if (sinkCookie_ != TF_INVALID_COOKIE &&
                SUCCEEDED(mgr_->QueryInterface(__uuidof(ITfSource),
                                               reinterpret_cast<void**>(source.put()))))
                source->UnadviseSink(sinkCookie_);
        }
        sinkCookie_ = TF_INVALID_COOKIE;
        id_ = 0;
        mgr_.reset();
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE OnSetFocus(BOOL foreground) override {
        if (!foreground) {
            EndSession();
            return S_OK;
        }
        ComPtr<ITfDocumentMgr> focused;
        if (mgr_ && SUCCEEDED(mgr_->GetFocus(focused.put())) && focused)
            OnSetFocus(focused.get(), nullptr);
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnInitDocumentMgr(ITfDocumentMgr*) override { return S_OK; }
    HRESULT STDMETHODCALLTYPE OnUninitDocumentMgr(ITfDocumentMgr*) override { return S_OK; }
    HRESULT STDMETHODCALLTYPE OnSetFocus(ITfDocumentMgr* focused, ITfDocumentMgr*) override {
        ComPtr<ITfContext> context;
        if (focused) focused->GetTop(context.put());
        UseContext(context.get());
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnPushContext(ITfContext* context) override {
        UseContext(context);
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnPopContext(ITfContext* context) override {
        if (context == context_.get()) EndSession();
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnCompositionTerminated(TfEditCookie cookie, ITfComposition* composition) override {
        if (composition != composition_.get()) return S_OK;
        ComPtr<ITfRange> range;
        composition->GetRange(range.put());
        composition_.reset();
        Request request;
        request.action = Reset;
        request.token = token_;
        Result response;
        if (range && Exchange(pipeName_, request, response) && response.flags & Commit)
            return range->SetText(cookie, 0, response.committed.data(),
                                  static_cast<LONG>(response.committed.size()));
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnTestKeyDown(ITfContext* context, WPARAM key, LPARAM, BOOL* eaten) override {
        *eaten = context && key != VK_PACKET && key != VK_SHIFT && key != VK_CONTROL &&
                 key != VK_MENU && key != VK_LWIN && key != VK_RWIN &&
                 !(key >= VK_LSHIFT && key <= VK_RMENU);
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnKeyDown(ITfContext* context, WPARAM key, LPARAM scan, BOOL* eaten) override {
        *eaten = FALSE;
        UseContext(context);
        Request request;
        request.action = Key;
        request.token = token_;
        Translate(request, key, scan);
        EditContext(context, request, eaten);
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnTestKeyUp(ITfContext*, WPARAM, LPARAM, BOOL* eaten) override {
        *eaten = FALSE;
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnKeyUp(ITfContext*, WPARAM, LPARAM, BOOL* eaten) override {
        *eaten = FALSE;
        return S_OK;
    }
    HRESULT STDMETHODCALLTYPE OnPreservedKey(ITfContext*, REFGUID, BOOL* eaten) override {
        *eaten = FALSE;
        return S_OK;
    }
};
thread_local Tip* Tip::polling_{};

class Factory final : public IClassFactory {
    ULONG refs_{1};
public:
    Factory() { ++references; }
    ~Factory() { --references; }
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID id, void** out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (id != IID_IUnknown && id != IID_IClassFactory) return E_NOINTERFACE;
        *out = static_cast<IClassFactory*>(this);
        AddRef();
        return S_OK;
    }
    ULONG STDMETHODCALLTYPE AddRef() override { return ++refs_; }
    ULONG STDMETHODCALLTYPE Release() override {
        const auto n = --refs_;
        if (!n) delete this;
        return n;
    }
    HRESULT STDMETHODCALLTYPE CreateInstance(IUnknown* outer, REFIID id, void** out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (outer) return CLASS_E_NOAGGREGATION;
        auto* tip = new(std::nothrow) Tip();
        if (!tip) return E_OUTOFMEMORY;
        const HRESULT hr = tip->QueryInterface(id, out);
        tip->Release();
        return hr;
    }
    HRESULT STDMETHODCALLTYPE LockServer(BOOL lock) override {
        if (lock) ++references;
        else --references;
        return S_OK;
    }
};

std::wstring ClassKey() {
    wchar_t id[40]{};
    StringFromGUID2(kClass, id, ARRAYSIZE(id));
    return std::wstring(L"Software\\Classes\\CLSID\\") + id;
}

HRESULT RegistrySet(HKEY key, const wchar_t* name, const std::wstring& value) {
    return HRESULT_FROM_WIN32(RegSetValueExW(key, name, 0, REG_SZ,
        reinterpret_cast<const BYTE*>(value.c_str()), DWORD((value.size()+1)*sizeof(wchar_t))));
}

HRESULT RegisterServer(const std::wstring& path) {
    HKEY key{};
    HRESULT hr = HRESULT_FROM_WIN32(RegCreateKeyExW(HKEY_LOCAL_MACHINE, ClassKey().c_str(), 0,
        nullptr, 0, KEY_WRITE, nullptr, &key, nullptr));
    if (FAILED(hr)) return hr;
    hr = RegistrySet(key, nullptr, kName);
    HKEY inproc{};
    if (SUCCEEDED(hr)) hr = HRESULT_FROM_WIN32(RegCreateKeyExW(key, L"InprocServer32", 0,
        nullptr, 0, KEY_WRITE, nullptr, &inproc, nullptr));
    RegCloseKey(key);
    if (!inproc) return hr;
    hr = RegistrySet(inproc, nullptr, path);
    if (SUCCEEDED(hr)) hr = RegistrySet(inproc, L"ThreadingModel", L"Apartment");
    RegCloseKey(inproc);
    return hr;
}

HRESULT OpenProfiles(ComPtr<ITfInputProcessorProfiles>& profiles) {
    return CoCreateInstance(CLSID_TF_InputProcessorProfiles, nullptr, CLSCTX_INPROC_SERVER,
                            __uuidof(ITfInputProcessorProfiles), reinterpret_cast<void**>(profiles.put()));
}

HRESULT OpenCategories(ComPtr<ITfCategoryMgr>& categories) {
    return CoCreateInstance(CLSID_TF_CategoryMgr, nullptr, CLSCTX_INPROC_SERVER,
                            __uuidof(ITfCategoryMgr), reinterpret_cast<void**>(categories.put()));
}

void RemoveRegistration() {
    ComPtr<ITfCategoryMgr> categories;
    if (SUCCEEDED(OpenCategories(categories)))
        categories->UnregisterCategory(kClass, GUID_TFCAT_TIP_KEYBOARD, kClass);
    ComPtr<ITfInputProcessorProfiles> profiles;
    if (SUCCEEDED(OpenProfiles(profiles))) {
        profiles->RemoveLanguageProfile(kClass, kLanguage, kProfile);
        profiles->Unregister(kClass);
    }
    RegDeleteTreeW(HKEY_LOCAL_MACHINE, ClassKey().c_str());
}
}

extern "C" BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) module = instance;
    return TRUE;
}
extern "C" __declspec(dllexport) HRESULT WINAPI DllCanUnloadNow() {
    return references == 0 ? S_OK : S_FALSE;
}
extern "C" __declspec(dllexport) HRESULT WINAPI DllGetClassObject(REFCLSID id, REFIID iid, void** out) {
    if (!out) return E_POINTER;
    *out = nullptr;
    if (id != kClass) return CLASS_E_CLASSNOTAVAILABLE;
    auto* factory = new(std::nothrow) Factory();
    if (!factory) return E_OUTOFMEMORY;
    const HRESULT hr = factory->QueryInterface(iid, out);
    factory->Release();
    return hr;
}
extern "C" __declspec(dllexport) HRESULT WINAPI DllRegisterServer() {
    const HRESULT init = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(init) && init != RPC_E_CHANGED_MODE) return init;
    const auto path = ModulePath();
    HRESULT hr = path.empty() ? E_FAIL : RegisterServer(path);
    ComPtr<ITfInputProcessorProfiles> profiles;
    if (SUCCEEDED(hr)) hr = OpenProfiles(profiles);
    if (SUCCEEDED(hr)) hr = profiles->Register(kClass);
    if (SUCCEEDED(hr)) hr = profiles->AddLanguageProfile(
        kClass, kLanguage, kProfile, kName, ARRAYSIZE(kName)-1, nullptr, 0, 0);
    ComPtr<ITfCategoryMgr> categories;
    if (SUCCEEDED(hr)) hr = OpenCategories(categories);
    if (SUCCEEDED(hr)) hr = categories->RegisterCategory(kClass, GUID_TFCAT_TIP_KEYBOARD, kClass);
    if (FAILED(hr)) RemoveRegistration();
    categories.reset();
    profiles.reset();
    if (SUCCEEDED(init)) CoUninitialize();
    return hr;
}
extern "C" __declspec(dllexport) HRESULT WINAPI DllUnregisterServer() {
    const HRESULT init = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(init) && init != RPC_E_CHANGED_MODE) return init;
    RemoveRegistration();
    if (SUCCEEDED(init)) CoUninitialize();
    return S_OK;
}
