#pragma once
#include "bridge.h"
#include "../common/pipe_logic.h"

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
    bool connected() const { return connection_.ready(); }
    uint64_t generation() const { return generation_.load(); }

    bool post(Frame frame);
    std::optional<KeyOutcome> request(Frame frame, std::chrono::steady_clock::time_point deadline);
    std::deque<Frame> received(uint64_t through = UINT64_MAX);
private:
    void run();
    void session(HANDLE pipe, Reconnect& retry);
    bool transfer(HANDLE pipe, bool write, void* ptr, size_t size);
    bool deliver(Frame frame);

    KeyConnection connection_;
    std::atomic<bool> stopping_{false};
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
    std::deque<std::pair<uint64_t, Frame>> incoming_;
    uint64_t received_ = 0; // pipe reader thread
};

} // namespace tango::ime
