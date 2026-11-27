#pragma once
#include "proto.h"
#include <algorithm>
#include <chrono>
#include <condition_variable>
#include <mutex>
#include <optional>

namespace tango::ime {

inline constexpr auto key_timeout = std::chrono::milliseconds(150);

class Reconnect {
    uint64_t ready_at_ = 0, delay_ = 1000;
public:
    bool ready(uint64_t now) const { return now >= ready_at_; }
    void fail(uint64_t now) {
        ready_at_ = now + delay_;
        delay_ = std::min<uint64_t>(delay_ * 2, 30000);
    }
    void connected() { ready_at_ = 0; delay_ = 1000; }
};

struct KeyOutcome {
    bool eaten = false;
    std::vector<Frame> edits;
};

class KeyReply {
    std::mutex mutex_;
    std::condition_variable cv_;
    uint64_t seq_ = 0, session_ = 0;
    bool completed_ = false;
    KeyOutcome result_;
public:
    bool begin(uint64_t seq, uint64_t session) {
        std::lock_guard<std::mutex> lock(mutex_);
        if (!seq || seq_) return false;
        seq_ = seq;
        session_ = session;
        completed_ = false;
        result_ = {};
        return true;
    }

    // Capture edits associated with a synchronous decision. A late edit
    // from an abandoned key is dropped along with its reply.
    bool accept(Frame frame) {
        std::lock_guard<std::mutex> lock(mutex_);
        if (!seq_ || completed_ || frame.seq != seq_ || frame.session != session_) return false;
        if (frame.type == Type::key_result) {
            result_.eaten = frame.eaten;
            completed_ = true;
            cv_.notify_one();
        } else if ((frame.type == Type::compose || frame.type == Type::commit) && result_.edits.size() < 32) {
            result_.edits.push_back(std::move(frame));
        } else {
            return false;
        }
        return true;
    }

    std::optional<KeyOutcome> wait(std::chrono::steady_clock::time_point deadline) {
        std::unique_lock<std::mutex> lock(mutex_);
        cv_.wait_until(lock, deadline, [this] { return completed_ || !seq_; });
        const bool received = seq_ && completed_;
        seq_ = 0;
        return received ? std::optional<KeyOutcome>(std::move(result_)) : std::nullopt;
    }

    void disconnect() {
        std::lock_guard<std::mutex> lock(mutex_);
        seq_ = 0;
        cv_.notify_one();
    }
};

} // namespace tango::ime
