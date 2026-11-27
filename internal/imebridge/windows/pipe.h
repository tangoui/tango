#pragma once
#include "bridge.h"
#include "pipe_logic.h"

#include <deque>
#include <thread>

namespace tango::ime {

inline constexpr UINT bridge_message = WM_APP + 19;

class Pipe {
public:
    Pipe() = default;
    ~Pipe() { stop(); }
    void start(const Config& config, HWND window);
    void stop();
    bool connected() const { return connected_.load(); }
    uint64_t generation() const { return generation_.load(); }

    bool post(Frame frame);
    std::optional<KeyOutcome> test(Frame frame, std::chrono::steady_clock::time_point deadline);
    std::deque<Frame> received();
private:
    void run();
    void session(HANDLE pipe);
    bool transfer(HANDLE pipe, bool write, void* ptr, size_t size);
    void deliver(Frame frame);

    std::atomic<bool> connected_{false}, stopping_{false};
    std::atomic<uint64_t> generation_{0};
    uint64_t seq_ = 0; // TSF thread
    HWND window_ = nullptr;
    HANDLE wake_ = nullptr;
    HANDLE io_ = nullptr;
    std::thread worker_;
    std::wstring name_, host_;
    DWORD login_ = 0;
    Frame hello_;
    KeyReply reply_;
    std::mutex mutex_;
    std::deque<Bytes> outgoing_;
    std::deque<Frame> incoming_;
};

} // namespace tango::ime
