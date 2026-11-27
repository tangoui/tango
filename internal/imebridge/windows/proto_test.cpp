#include "proto.h"

#include <cassert>
#include <filesystem>
#include <fstream>
#include <iterator>

int main(int argc, char** argv) {
    using namespace tango::ime;
    assert(argc == 2);
    int count = 0;
    for (const auto& input : std::filesystem::directory_iterator(argv[1])) {
        if (input.path().extension() != ".bin") continue;
        ++count;
        std::ifstream file(input.path(), std::ios::binary);
        assert(file);
        const Bytes golden(std::istreambuf_iterator<char>(file), {});
        Frame frame;
        assert(decode(golden, frame));
        assert(frame.seq == 17);
        Bytes encoded;
        assert(encode(frame, encoded));
        assert(encoded == golden);
        for (const auto& bad_values : {std::pair{4, 255}, std::pair{6, 1}}) {
            auto bad = golden;
            bad[bad_values.first] = bad_values.second;
            assert(!decode(bad, frame));
        }
        auto bad = golden;
        bad.pop_back();
        assert(!decode(bad, frame));
        bad.resize(65537);
        assert(!decode(bad, frame));
    }
    assert(count == 14);
}
