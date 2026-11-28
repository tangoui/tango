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
    Frame compose = commit;
    compose.type = Type::compose;
    compose.text = u"ha";
    assert(queue.push(commit) && queue.push(compose));
    auto before = queue.watermark();
    assert(queue.push(commit));
    auto earlier = queue.take(before);
    assert(earlier.size() == 2 && earlier.front().text == u"你" && earlier.back().text == u"ha");
    assert(queue.take().size() == 1);

    KeyReply reply;
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

    Reconnect retry;
    uint64_t now = 0;
    for (auto backoff : {1000, 2000, 4000, 8000, 16000, 30000, 30000}) {
        retry.fail(now);
        assert(!retry.ready(now + backoff - 1));
        now += backoff;
        assert(retry.ready(now));
    }
}
