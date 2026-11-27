#pragma once
#include <cstdint>
#include <string>
#include <string_view>

namespace tango::ime {

struct ConfigIni {
    std::u16string clsid, profile, connection, name, host;
    uint16_t langid = 0x0804;
};

inline bool hex_digit(char16_t ch) {
    return (ch >= u'0' && ch <= u'9') || (ch >= u'a' && ch <= u'f') || (ch >= u'A' && ch <= u'F');
}

inline bool valid_guid(std::u16string_view guid) {
    if (guid.size() != 38 || guid.front() != u'{' || guid.back() != u'}') return false;
    for (size_t i = 1; i + 1 < guid.size(); ++i) {
        const bool dash = i == 9 || i == 14 || i == 19 || i == 24;
        if (dash ? guid[i] != u'-' : !hex_digit(guid[i])) return false;
    }
    return true;
}

inline bool valid_connection(std::u16string_view name) {
    if (name.empty() || name.size() > 120) return false;
    for (const auto ch : name)
        if (!(ch >= u'a' && ch <= u'z') && !(ch >= u'A' && ch <= u'Z') &&
            !(ch >= u'0' && ch <= u'9') && ch != u'_' && ch != u'.' && ch != u'-') return false;
    return true;
}

inline bool parse_ini(std::u16string_view data, ConfigIni& config) {
    ConfigIni result;
    bool ime = false;
    while (!data.empty()) {
        const auto end = data.find(u'\n');
        auto line = data.substr(0, end);
        if (!line.empty() && line.back() == u'\r') line.remove_suffix(1);
        data.remove_prefix(end == data.npos ? data.size() : end + 1);
        if (line.empty() || line.front() == u';') continue;
        if (line.front() == u'[') {
            ime = line == u"[IME]";
            continue;
        }
        const auto split = line.find(u'=');
        if (!ime || split == line.npos) continue;
        const auto key = line.substr(0, split);
        const auto value = line.substr(split + 1);
        if (key == u"clsid") result.clsid = value;
        else if (key == u"profile") result.profile = value;
        else if (key == u"connection") result.connection = value;
        else if (key == u"name") result.name = value;
        else if (key == u"host") result.host = value;
        else if (key == u"langid") {
            if (value.size() != 4) return false;
            result.langid = 0;
            for (char16_t ch : value) {
                if (!hex_digit(ch)) return false;
                const unsigned digit = ch <= u'9' ? ch - u'0' : (ch | 32) - u'a' + 10;
                result.langid = static_cast<uint16_t>(result.langid * 16 + digit);
            }
        }
    }
    if (!valid_guid(result.clsid) || !valid_guid(result.profile) ||
        !valid_connection(result.connection) || !result.langid || result.name.empty()) return false;
    config = std::move(result);
    return true;
}

} // namespace tango::ime
