#include "pipe_logic.h"
#include "config_ini.h"

#include <cassert>
#include <chrono>

int main() {
    using namespace tango::ime;
    using namespace std::chrono;
    KeyReply reply;
    Frame result;
    result.type = Type::key_result;
    result.session = 4;
    result.seq = 9;
    result.eaten = true;
    assert(reply.begin(10, 4));
    assert(!reply.accept(result));
    auto start = steady_clock::now();
    assert(!reply.wait(start + key_timeout));
    assert(steady_clock::now() - start >= key_timeout);
    assert(reply.begin(10, 4));
    result.seq = 10;
    assert(reply.accept(result));
    assert(reply.wait(steady_clock::now() + key_timeout)->eaten);
    assert(reply.begin(11, 4));
    reply.disconnect();
    assert(!reply.wait(steady_clock::now() + key_timeout));

    Reconnect backoff;
    assert(backoff.ready(0));
    backoff.fail(0);
    assert(!backoff.ready(999) && backoff.ready(1000));
    backoff.fail(1000);
    assert(!backoff.ready(2999) && backoff.ready(3000));
    backoff.fail(3000);
    assert(!backoff.ready(6999) && backoff.ready(7000));
    backoff.connected();
    backoff.fail(7000);
    assert(!backoff.ready(7999) && backoff.ready(8000));

    ConfigIni ini;
    assert(!parse_ini(u"[IME]\nprofile={12345678-1234-1234-1234-123456789abc}\n", ini));
    assert(parse_ini(u"[IME]\r\nclsid={12345678-1234-1234-1234-123456789abc}\r\n"
                     u"profile={87654321-4321-4321-4321-abcdefabcdef}\r\n"
                     u"connection=com.example.Test_Connection\r\nlangid=0804\r\n"
                     u"name=Tango 中文\r\n", ini));
    assert(ini.langid == 0x0804 && ini.name == u"Tango 中文");
}
