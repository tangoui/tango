#pragma once
#include "../common/pipe_logic.h"
#include <deque>

namespace tango::ime {

inline bool isSocketHello(const Frame& frame, uint32_t pid) {
    return frame.type == Type::hello && frame.seq == 0 && frame.pid == pid;
}


// A decision records its receive watermark. The Fcitx event thread drains
// pushes before that watermark before applying the decision's edits.
class SocketInbox {
    using Item = std::pair<uint64_t, Frame>;
    mutable std::mutex mutex_;
    std::deque<Item> frames_;
    uint64_t serial_ = 0;
public:
    bool push(Frame frame) {
        std::lock_guard<std::mutex> lock(mutex_);
        if (frames_.size() >= 64) return false;
        frames_.emplace_back(++serial_, std::move(frame));
        return true;
    }

    uint64_t watermark() const {
        std::lock_guard<std::mutex> lock(mutex_);
        return serial_;
    }

    std::deque<Frame> take(uint64_t through = UINT64_MAX) {
        std::lock_guard<std::mutex> lock(mutex_);
        std::deque<Frame> result;
        while (!frames_.empty() && frames_.front().first <= through) {
            result.push_back(std::move(frames_.front().second));
            frames_.pop_front();
        }
        return result;
    }

    void clear() {
        std::lock_guard<std::mutex> lock(mutex_);
        frames_.clear();
    }
};

} // namespace tango::ime
