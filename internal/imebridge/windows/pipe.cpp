#include "pipe.h"
#include <sddl.h>

namespace tango::ime {

namespace {

bool process_info(DWORD& login, std::wstring& pipe, std::wstring_view connection, Frame& hello) {
    if (!ProcessIdToSessionId(GetCurrentProcessId(), &login)) return false;
    HANDLE token;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token)) return false;
    alignas(TOKEN_USER) BYTE user[sizeof(TOKEN_USER) + SECURITY_MAX_SID_SIZE];
    alignas(TOKEN_MANDATORY_LABEL) BYTE label[sizeof(TOKEN_MANDATORY_LABEL) + SECURITY_MAX_SID_SIZE];
    DWORD n = 0, container = 0;
    const bool valid = GetTokenInformation(token, TokenUser, user, sizeof(user), &n) &&
        GetTokenInformation(token, TokenIntegrityLevel, label, sizeof(label), &n);
    GetTokenInformation(token, TokenIsAppContainer, &container, sizeof(container), &n);
    CloseHandle(token);
    if (!valid) return false;
    const auto label_sid = reinterpret_cast<TOKEN_MANDATORY_LABEL*>(label)->Label.Sid;
    hello.integrity = *GetSidSubAuthority(label_sid, *GetSidSubAuthorityCount(label_sid) - 1);
    wchar_t* sid = nullptr;
    if (!ConvertSidToStringSidW(reinterpret_cast<TOKEN_USER*>(user)->User.Sid, &sid)) return false;
    try {
        pipe = std::wstring(L"\\\\.\\pipe\\TangoIME-v2-") + sid + L"-" +
               std::to_wstring(login) + L"-" + std::wstring(connection);
    } catch (...) {
        LocalFree(sid);
        throw;
    }
    LocalFree(sid);
    hello.type = Type::hello;
    hello.pid = GetCurrentProcessId();
    hello.appcontainer = container != 0;
    wchar_t path[MAX_PATH];
    const DWORD length = GetModuleFileNameW(nullptr, path, ARRAYSIZE(path));
    if (length && length < ARRAYSIZE(path)) {
        auto name = std::wstring_view(path, length);
        const auto slash = name.find_last_of(L'\\');
        hello.exe = utf16(name.substr(slash == name.npos ? 0 : slash + 1));
    }
    return true;
}

bool expected_host(HANDLE pipe, DWORD login, const std::wstring& expected, bool appcontainer) {
    ULONG process_id = 0, session_id = 0;
    if (!GetNamedPipeServerProcessId(pipe, &process_id) ||
        !GetNamedPipeServerSessionId(pipe, &session_id) || session_id != login) return false;
    HANDLE process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, process_id);
    // AppContainer can be denied access to the medium-integrity process
    // object. The server still needs to be local and in this login session.
    if (!process) return appcontainer && GetLastError() == ERROR_ACCESS_DENIED;
    wchar_t path[32768];
    DWORD length = ARRAYSIZE(path);
    const bool matches = QueryFullProcessImageNameW(process, 0, path, &length) &&
        CompareStringOrdinal(path, length, expected.c_str(), -1, TRUE) == CSTR_EQUAL;
    CloseHandle(process);
    return matches;
}

} // namespace

void Pipe::start(const Config& config, HWND window) {
    host_ = config.host;
    if (!process_info(login_, name_, config.connection, hello_)) return;
    window_ = window;
    wake_ = CreateEventW(nullptr, FALSE, FALSE, nullptr);
    io_ = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    if (!wake_ || !io_) {
        stop();
        return;
    }
    stopping_ = false;
    try {
        worker_ = std::thread([this] { run(); });
    } catch (...) {
        stop();
    }
}

void Pipe::stop() {
    stopping_ = true;
    if (wake_) SetEvent(wake_);
    if (worker_.joinable()) worker_.join();
    if (wake_) CloseHandle(wake_);
    if (io_) CloseHandle(io_);
    wake_ = io_ = nullptr;
    window_ = nullptr;
    std::lock_guard<std::mutex> lock(mutex_);
    outgoing_.clear();
    incoming_.clear();
}

bool Pipe::post(Frame frame) {
    if (!connected()) return false;
    Bytes bytes;
    if (!encode(frame, bytes)) return false;
    {
        std::lock_guard<std::mutex> lock(mutex_);
        if (!connected() || outgoing_.size() >= 64) return false;
        outgoing_.push_back(std::move(bytes));
    }
    SetEvent(wake_);
    return true;
}

std::optional<KeyOutcome> Pipe::test(Frame frame, std::chrono::steady_clock::time_point deadline) {
    if (!connected()) return std::nullopt;
    frame.type = Type::test_key;
    frame.seq = ++seq_;
    if (!reply_.begin(frame.seq, frame.session)) return std::nullopt;
    if (!post(std::move(frame))) reply_.disconnect();
    return reply_.wait(deadline);
}

std::deque<Frame> Pipe::received() {
    std::deque<Frame> frames;
    std::lock_guard<std::mutex> lock(mutex_);
    frames.swap(incoming_);
    return frames;
}

void Pipe::run() {
    Reconnect retry;
    while (!stopping_) {
        const auto now = GetTickCount64();
        if (!retry.ready(now)) {
            WaitForSingleObject(wake_, 50);
            continue;
        }
        const HANDLE pipe = CreateFileW(name_.c_str(), FILE_READ_DATA | FILE_WRITE_DATA | FILE_READ_ATTRIBUTES | SYNCHRONIZE,
            0, nullptr, OPEN_EXISTING, FILE_FLAG_OVERLAPPED | SECURITY_SQOS_PRESENT |
            SECURITY_IDENTIFICATION, nullptr);
        if (pipe != INVALID_HANDLE_VALUE) {
            try {
                if (expected_host(pipe, login_, host_, hello_.appcontainer)) {
                    retry.connected();
                    session(pipe);
                }
            } catch (...) {
                // An allocation or transport error leaves keys untouched.
            }
            CloseHandle(pipe);
        }
        {
            std::lock_guard<std::mutex> lock(mutex_);
            connected_ = false;
            outgoing_.clear();
            incoming_.clear();
        }
        reply_.disconnect();
        if (!stopping_) PostMessageW(window_, bridge_message, 0, 0);
        retry.fail(GetTickCount64());
    }
}

void Pipe::session(HANDLE pipe) {
    Bytes hello;
    if (!encode(hello_, hello) || !transfer(pipe, true, hello.data(), hello.size())) return;
    ++generation_;
    connected_ = true;
    PostMessageW(window_, bridge_message, 0, 0);
    while (!stopping_) {
        std::deque<Bytes> outgoing;
        {
            std::lock_guard<std::mutex> lock(mutex_);
            outgoing.swap(outgoing_);
        }
        for (auto& bytes : outgoing)
            if (!transfer(pipe, true, bytes.data(), bytes.size())) return;

        for (unsigned i = 0; i < 64; ++i) {
            DWORD available = 0;
            if (!PeekNamedPipe(pipe, nullptr, 0, nullptr, &available, nullptr)) return;
            if (available < 4) break;
            Bytes bytes(4);
            if (!transfer(pipe, false, bytes.data(), bytes.size())) return;
            const auto n = uint32_t(bytes[0]) | uint32_t(bytes[1]) << 8 |
                           uint32_t(bytes[2]) << 16 | uint32_t(bytes[3]) << 24;
            if (n < header_size - 4 || n > max_frame - 4) return;
            bytes.resize(4 + n);
            if (!transfer(pipe, false, bytes.data() + 4, n)) return;
            Frame frame;
            if (!decode(bytes, frame)) return;
            deliver(std::move(frame));
        }
        WaitForSingleObject(wake_, 5);
    }
}

bool Pipe::transfer(HANDLE pipe, bool write, void* ptr, size_t size) {
    auto* data = static_cast<uint8_t*>(ptr);
    const auto deadline = GetTickCount64() + 100;
    while (size > 0 && !stopping_) {
        OVERLAPPED op{};
        ResetEvent(io_);
        op.hEvent = io_;
        DWORD done = 0;
        BOOL ok = write ? WriteFile(pipe, data, static_cast<DWORD>(size), &done, &op)
                        : ReadFile(pipe, data, static_cast<DWORD>(size), &done, &op);
        if (!ok && GetLastError() == ERROR_IO_PENDING) {
            while (GetTickCount64() < deadline && !stopping_) {
                if (WaitForSingleObject(io_, 5) == WAIT_OBJECT_0) {
                    ok = GetOverlappedResult(pipe, &op, &done, FALSE);
                    break;
                }
            }
            if (!ok) {
                CancelIoEx(pipe, &op);
                GetOverlappedResult(pipe, &op, &done, TRUE);
            }
        }
        if (!ok || !done) return false;
        size -= done;
        data += done;
    }
    return size == 0;
}

void Pipe::deliver(Frame frame) {
    if (frame.seq && (frame.type == Type::key_result || frame.type == Type::compose ||
                      frame.type == Type::commit)) {
        reply_.accept(std::move(frame));
        return;
    }
    if (frame.type != Type::compose && frame.type != Type::commit &&
        frame.type != Type::mode && frame.type != Type::menu) return;
    {
        std::lock_guard<std::mutex> lock(mutex_);
        if (incoming_.size() >= 64) return;
        incoming_.push_back(std::move(frame));
    }
    PostMessageW(window_, bridge_message, 0, 0);
}

} // namespace tango::ime
