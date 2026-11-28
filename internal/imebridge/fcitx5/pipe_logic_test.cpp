#include "pipe_logic.h"

#include <cassert>

int main() {
    using namespace tango::ime;
    using namespace std::chrono;
    SocketInbox queue;
    Frame commit;
    commit.type = Type::commit;
    commit.session = 2;
    commit.text = u"你";
    commit.socket_generation = 3;
    Frame compose = commit;
    compose.type = Type::compose;
    compose.text = u"ha";
    assert(queue.push(commit) && queue.push(compose));
    auto before = queue.watermark();
    assert(queue.push(commit));
    auto earlier = queue.take(before);
    assert(earlier.size() == 2 && earlier.front().text == u"你" && earlier.back().text == u"ha");
    assert(earlier.front().socket_generation == 3);
    assert(queue.take().size() == 1);

    KeyReply reply;
    assert(reply.begin(6, 2));
    commit.seq = 6;
    assert(reply.accept(commit));
    Frame finished;
    finished.type = Type::key_result;
    finished.seq = 6;
    finished.session = 2;
    assert(reply.accept(finished));
    auto held = reply.wait(steady_clock::now());
    assert(held && held->edits.size() == 1 && held->edits.front().socket_generation == 3);
    assert(reply.begin(7, 2));
    Frame response;
    response.type = Type::key_result;
    response.seq = 8;
    response.session = 2;
    assert(!reply.accept(response));
    auto start = steady_clock::now();
    assert(!reply.wait(start + key_timeout));
    assert(steady_clock::now() - start >= key_timeout);
    KeyConnection state;
    state.connected();
    state.timed_out();
    assert(!state.ready() && state.reconnect());

    Frame hello;
    hello.type = Type::hello;
    hello.pid = 22;
    assert(!isSocketHello(hello, 23));
    assert(isSocketHello(hello, 22));
    hello.seq = 1;
    assert(!isSocketHello(hello, 22));

    Reconnect retry;
    uint64_t now = 0;
    for (auto backoff : {1000, 2000, 4000, 8000, 16000, 30000, 30000}) {
        retry.fail(now);
        assert(!retry.ready(now + backoff - 1));
        now += backoff;
        assert(retry.ready(now));
    }
}
