#include "pipe.h"

#include <cerrno>
#include <cstring>
#include <fcntl.h>
#include <poll.h>
#include <spawn.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <unistd.h>

extern char** environ;

namespace tango::ime {
namespace {
using Clock = std::chrono::steady_clock;

uint64_t milliseconds() {
    return static_cast<uint64_t>(std::chrono::duration_cast<std::chrono::milliseconds>(
        Clock::now().time_since_epoch()).count());
}

bool transfer(int fd, bool writing, uint8_t* bytes, size_t length, const std::atomic<bool>& stop) {
    const auto deadline = Clock::now() + std::chrono::milliseconds(100);
    while (length && !stop && Clock::now() < deadline) {
        const auto size = writing ? send(fd, bytes, length, MSG_NOSIGNAL)
                                  : recv(fd, bytes, length, 0);
        if (size > 0) {
            bytes += size;
            length -= size;
            continue;
        }
        if (size == 0) return false;
        if (errno == EINTR) continue;
        if (errno != EAGAIN && errno != EWOULDBLOCK) return false;
        pollfd wait{fd, static_cast<short>(writing ? POLLOUT : POLLIN), 0};
        poll(&wait, 1, 5);
    }
    return length == 0;
}

bool readFrame(int fd, Bytes& bytes, Frame& frame, const std::atomic<bool>& stop) {
    bytes.resize(4);
    if (!transfer(fd, false, bytes.data(), 4, stop)) return false;
    const auto length = uint32_t(bytes[0]) | uint32_t(bytes[1]) << 8 |
                        uint32_t(bytes[2]) << 16 | uint32_t(bytes[3]) << 24;
    if (length < header_size - 4 || length > max_frame - 4) return false;
    bytes.resize(length + 4);
    return transfer(fd, false, bytes.data() + 4, length, stop) && decode(bytes, frame);
}

int connect_host(const std::string& path) {
    sockaddr_un addr{};
    addr.sun_family = AF_UNIX;
    if (path.empty() || path.size() >= sizeof(addr.sun_path)) return -1;
    std::memcpy(addr.sun_path, path.c_str(), path.size() + 1);
    const int fd = socket(AF_UNIX, SOCK_STREAM | SOCK_NONBLOCK | SOCK_CLOEXEC, 0);
    if (fd < 0) return -1;
    if (connect(fd, reinterpret_cast<const sockaddr*>(&addr), sizeof(addr)) != 0) {
        const auto error = errno;
        close(fd);
        errno = error;
        return -1;
    }
    ucred peer{};
    socklen_t length = sizeof(peer);
    if (getsockopt(fd, SOL_SOCKET, SO_PEERCRED, &peer, &length) != 0 || peer.uid != getuid()) {
        close(fd);
        errno = EPERM;
        return -1;
    }
    return fd;
}
} // namespace

void SocketPipe::start(std::string path, std::string host, std::function<void()> notify) {
    path_ = std::move(path);
    host_ = std::move(host);
    notify_ = std::move(notify);
    if (pipe2(signal_, O_NONBLOCK | O_CLOEXEC) != 0) return;
    worker_ = std::thread([this] { run(); });
}

SocketPipe::~SocketPipe() {
    stop_ = true;
    wake();
    if (worker_.joinable()) worker_.join();
    for (int fd : signal_)
        if (fd >= 0) close(fd);
}

void SocketPipe::wake() {
    const uint8_t byte = 1;
    if (signal_[1] >= 0) (void)write(signal_[1], &byte, 1);
}

bool SocketPipe::post(Frame frame) {
    if (!connected()) return false;
    Bytes encoded;
    if (!encode(frame, encoded)) return false;
    {
        std::lock_guard<std::mutex> lock(mutex_);
        if (!connected() || sent_.size() >= 64) return false;
        sent_.push_back(std::move(encoded));
    }
    wake();
    return true;
}

std::optional<KeyOutcome> SocketPipe::request(Frame frame, std::chrono::milliseconds timeout) {
    if (!connected()) return std::nullopt;
    const auto deadline = Clock::now() + timeout;
    frame.seq = ++sequence_;
    if (!reply_.begin(frame.seq, frame.session)) return std::nullopt;
    if (!post(std::move(frame))) reply_.disconnect();
    auto result = reply_.wait(deadline);
    if (!result) {
        connection_.timed_out();
        wake();
    }
    return result;
}

void SocketPipe::run() {
    Reconnect backoff;
    bool spawned = false;
    pid_t child = 0;
    while (!stop_) {
        if (child > 0 && waitpid(child, nullptr, WNOHANG) != 0) child = 0;
        if (backoff.ready(milliseconds())) {
            const int fd = connect_host(path_);
            if (fd >= 0) {
                try { serve(fd, backoff); }
                catch (...) {}
                close(fd);
            } else if (!spawned && (errno == ENOENT || errno == ECONNREFUSED)) {
                spawned = true;
                char* args[] = {host_.data(), nullptr};
                if (posix_spawn(&child, host_.c_str(), nullptr, nullptr, args, environ) != 0) child = 0;
            }
            connection_.disconnected();
            reply_.disconnect();
            {
                std::lock_guard<std::mutex> lock(mutex_);
                sent_.clear();
            }
            inbox_.clear();
            if (!stop_) {
                try { notify_(); }
                catch (...) {}
            }
            backoff.fail(milliseconds());
        }
        pollfd w{signal_[0], POLLIN, 0};
        poll(&w, 1, 50);
        if (w.revents & POLLIN) {
            uint8_t buffer[64];
            while (read(signal_[0], buffer, sizeof(buffer)) > 0) {}
        }
    }
}

void SocketPipe::serve(int fd, Reconnect& backoff) {
    Frame hello;
    hello.type = Type::hello;
    hello.pid = static_cast<uint32_t>(getpid());
    hello.exe = u"fcitx5";
    Bytes bytes;
    if (!encode(hello, bytes) || !transfer(fd, true, bytes.data(), bytes.size(), stop_)) return;
    Frame acknowledgment;
    if (!readFrame(fd, bytes, acknowledgment, stop_) || !isSocketHello(acknowledgment, hello.pid)) return;
    ++generation_;
    connection_.connected();
    backoff.connected();
    notify_();
    while (!stop_ && !connection_.reconnect()) {
        std::deque<Bytes> outgoing;
        {
            std::lock_guard<std::mutex> lock(mutex_);
            sent_.swap(outgoing);
        }
        for (auto& frame : outgoing)
            if (!transfer(fd, true, frame.data(), frame.size(), stop_)) return;
        pollfd ready[] = {{fd, POLLIN, 0}, {signal_[0], POLLIN, 0}};
        if (poll(ready, 2, 10) < 0) {
            if (errno == EINTR) continue;
            return;
        }
        if (ready[1].revents & POLLIN) {
            uint8_t buffer[64];
            while (read(signal_[0], buffer, sizeof(buffer)) > 0) {}
        }
        if (ready[0].revents & POLLIN) {
            Frame frame;
            if (!readFrame(fd, bytes, frame, stop_)) return;
            deliver(std::move(frame), backoff);
        } else if (ready[0].revents & (POLLHUP | POLLERR | POLLNVAL)) {
            return;
        }
    }
}

void SocketPipe::deliver(Frame frame, Reconnect& backoff) {
    switch (frame.type) {
    case Type::key_result:
        if (reply_.accept(std::move(frame), inbox_.watermark())) backoff.connected();
        return;
    case Type::compose:
    case Type::commit:
    case Type::candidates:
        if (frame.seq) {
            reply_.accept(std::move(frame));
            return;
        }
        break;
    case Type::mode:
    case Type::menu:
        break;
    default:
        return;
    }
    if (!inbox_.push(std::move(frame))) {
        connection_.timed_out();
        return;
    }
    notify_();
}

} // namespace tango::ime
