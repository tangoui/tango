#pragma once

#include <cstdint>
#include <limits>
#include <string>
#include <type_traits>
#include <utility>
#include <vector>

namespace tango::ime {

using Bytes = std::vector<uint8_t>;

inline constexpr uint16_t version = 2;
inline constexpr size_t header_size = 16;
inline constexpr size_t max_frame = 64 * 1024;

enum class Type : uint16_t {
    hello = 1, open, close, key, test_key, focus, menu_open, menu_pick, mode_pick,
    key_result, compose, commit, mode, menu, candidates, reset, candidate_pick, candidate_list,
};

enum KeyFlag : uint8_t { down = 1, up = 2, repeat = 4, extended = 8 };
enum Modifier : uint8_t { shift = 1, ctrl = 2, alt = 4, meta = 8, caps_lock = 16 };
enum MenuFlag : uint8_t { checked = 1, disabled = 2, separator = 4 };

struct Rect {
    int32_t x = 0, y = 0, width = 0, height = 0;
};

struct MenuItem {
    uint8_t flags = 0;
    std::u16string id, title;
};

struct Candidate {
    std::u16string text, comment;
};

// The message type determines which fields are encoded.
struct Frame {
    Type type{};
    uint64_t seq = 0, session = 0;
    uint32_t pid = 0, integrity = 0, cursor = 0;
    uint16_t vk = 0, scan = 0;
    uint8_t flags = 0, modifiers = 0;
    bool appcontainer = false, focused = false, eaten = false;
    Rect caret;
    std::u16string exe, text, id, label, short_text, icon;
    std::vector<MenuItem> items;
    std::vector<Candidate> candidates;
    int32_t selected = -1;
    uint32_t candidate_index = 0;
    uint64_t candidate_revision = 0;
    uint64_t socket_generation = 0; // Local Fcitx5 metadata, never serialized.
};

inline bool valid_utf16(const std::u16string& text) {
    for (size_t i = 0; i < text.size(); ++i) {
        const auto ch = text[i];
        if (ch >= 0xd800 && ch <= 0xdbff) {
            if (++i == text.size() || text[i] < 0xdc00 || text[i] > 0xdfff) return false;
        } else if (ch >= 0xdc00 && ch <= 0xdfff) {
            return false;
        }
    }
    return true;
}

class Encoder {
public:
    explicit Encoder(Bytes& out) : out_(out) {}

    template<class T>
    bool operator()(T value) {
        using U = std::make_unsigned_t<T>;
        const auto number = static_cast<U>(value);
        if (out_.size() > max_frame - sizeof(T)) return false;
        for (size_t i = 0; i < sizeof(T); ++i)
            out_.push_back(static_cast<uint8_t>(number >> (8 * i)));
        return true;
    }

    bool operator()(bool value) { return (*this)(static_cast<uint8_t>(value)); }

    bool operator()(const std::u16string& text) {
        if (text.size() > std::numeric_limits<uint16_t>::max() || !valid_utf16(text) ||
            !(*this)(static_cast<uint16_t>(text.size()))) return false;
        for (auto ch : text)
            if (!(*this)(static_cast<uint16_t>(ch))) return false;
        return true;
    }

    bool operator()(const std::vector<MenuItem>& items) {
        if (items.size() > (max_frame - header_size - 2) / 5 ||
            !(*this)(static_cast<uint16_t>(items.size()))) return false;
        for (const auto& item : items) {
            if ((item.flags & ~7) || !(*this)(item.flags) ||
                !(*this)(item.id) || !(*this)(item.title)) return false;
        }
        return true;
    }

    bool operator()(const std::vector<Candidate>& items) {
        if (items.size() > (max_frame - header_size - 14) / 4 ||
            !(*this)(static_cast<uint16_t>(items.size()))) return false;
        for (const auto& item : items)
            if (!(*this)(item.text) || !(*this)(item.comment)) return false;
        return true;
    }

private:
    Bytes& out_;
};

class Decoder {
public:
    explicit Decoder(const Bytes& input) : input_(input) {}

    size_t remaining() const { return input_.size() - offset_; }

    template<class T>
    bool operator()(T& value) {
        using U = std::make_unsigned_t<T>;
        if (remaining() < sizeof(T)) return false;
        U number = 0;
        for (size_t i = 0; i < sizeof(T); ++i)
            number |= static_cast<U>(input_[offset_++]) << (8 * i);
        value = static_cast<T>(number);
        return true;
    }

    bool operator()(bool& value) {
        uint8_t byte;
        if (!(*this)(byte) || byte > 1) return false;
        value = byte != 0;
        return true;
    }

    bool operator()(std::u16string& text) {
        uint16_t size;
        if (!(*this)(size) || size > remaining() / 2) return false;
        text.resize(size);
        for (auto& character : text) {
            uint16_t ch;
            if (!(*this)(ch)) return false;
            character = static_cast<char16_t>(ch);
        }
        return valid_utf16(text);
    }

    bool operator()(std::vector<MenuItem>& items) {
        uint16_t count;
        if (!(*this)(count) || count > remaining() / 5) return false;
        items.resize(count);
        for (auto& item : items) {
            if (!(*this)(item.flags) || (item.flags & ~7) ||
                !(*this)(item.id) || !(*this)(item.title)) return false;
        }
        return true;
    }

    bool operator()(std::vector<Candidate>& items) {
        uint16_t count;
        if (!(*this)(count) || count > remaining() / 4) return false;
        items.resize(count);
        for (auto& item : items)
            if (!(*this)(item.text) || !(*this)(item.comment)) return false;
        return true;
    }

private:
    const Bytes& input_;
    size_t offset_ = 0;
};

template<class Codec, class Message>
bool code_body(Codec& c, Message& f) {
    switch (f.type) {
    case Type::hello:
        return c(f.pid) && c(f.appcontainer) && c(f.integrity) && c(f.exe);
    case Type::open:
    case Type::close:
    case Type::menu_open:
    case Type::reset:
        return c(f.session);
    case Type::candidate_pick:
        return c(f.session) && c(f.candidate_revision) && c(f.candidate_index);
    case Type::candidate_list:
        return c(f.session) && c(f.candidate_revision) && c(f.selected) && c(f.candidates);
    case Type::candidates:
        return c(f.session) && c(f.selected) && c(f.candidates);
    case Type::key:
    case Type::test_key:
        return c(f.session) && c(f.vk) && c(f.scan) && c(f.flags) && c(f.modifiers) &&
            c(f.caret.x) && c(f.caret.y) && c(f.caret.width) && c(f.caret.height) && c(f.text);
    case Type::focus:
        return c(f.session) && c(f.focused);
    case Type::menu_pick:
    case Type::mode_pick:
        return c(f.id);
    case Type::key_result:
        return c(f.session) && c(f.eaten);
    case Type::compose:
        return c(f.session) && c(f.cursor) && c(f.text);
    case Type::commit:
        return c(f.session) && c(f.text);
    case Type::mode:
        return c(f.id) && c(f.label) && c(f.short_text) && c(f.icon);
    case Type::menu:
        return c(f.items);
    }
    return false;
}

inline bool encode(const Frame& frame, Bytes& out) {
    Bytes result(4);
    Encoder e(result);
    if (!e(static_cast<uint16_t>(frame.type)) || !e(version) || !e(frame.seq) || !code_body(e, frame))
        return false;
    const auto size = result.size() - 4;
    for (size_t i = 0; i < 4; ++i) result[i] = static_cast<uint8_t>(size >> (8 * i));
    out = std::move(result);
    return true;
}

inline bool decode(const Bytes& data, Frame& frame) {
    if (data.size() < header_size || data.size() > max_frame) return false;
    Decoder d(data);
    uint32_t size;
    uint16_t kind, wire_version;
    Frame result;
    if (!d(size) || size != d.remaining() || !d(kind) || !d(wire_version) ||
        wire_version != version || !d(result.seq)) return false;
    result.type = static_cast<Type>(kind);
    if (!code_body(d, result) || d.remaining()) return false;
    frame = std::move(result);
    return true;
}

} // namespace tango::ime
