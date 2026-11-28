#pragma once

#include "pipe_logic.h"
#include <functional>
#include <thread>

namespace tango::ime {

class SocketPipe {
public:
    SocketPipe() = default;
    ~SocketPipe();
    void start(std::string path, std::string host, std::function<void()> notify);
    bool post(Frame frame, uint64_t expectedGeneration = 0);
    std::optional<KeyOutcome> request(Frame frame, std::chrono::milliseconds timeout = key_timeout,
                                      uint64_t expectedGeneration = 0);
    std::deque<Frame> received(uint64_t through = UINT64_MAX) { return inbox_.take(through); }
    bool connected() const { return connection_.ready(); }
    uint64_t generation() const { return generation_.load(); }
private:
    void wake();
    void run();
    void serve(int fd, Reconnect& backoff);
    void deliver(Frame frame, Reconnect& backoff);

    KeyConnection connection_;
    KeyReply reply_;
    SocketInbox inbox_;
    std::atomic<bool> stop_{false};
    std::atomic<uint64_t> generation_{0};
    uint64_t sequence_ = 0;
    int signal_[2] = {-1, -1};
    std::thread worker_;
    std::string path_, host_;
    std::function<void()> notify_;
    std::mutex mutex_;
    std::deque<Bytes> sent_;
};

} // namespace tango::ime
