#include "bridge.h"
#include <vector>

namespace tango::ime {

std::wstring module_path() {
    std::vector<wchar_t> buffer(32768);
    const DWORD n = GetModuleFileNameW(module, buffer.data(), static_cast<DWORD>(buffer.size()));
    return n && n < buffer.size() ? std::wstring(buffer.data(), n) : std::wstring();
}

bool load_config(Config& config) {
    auto path = module_path();
    if (path.empty()) return false;
    const auto slash = path.find_last_of(L'\\');
    const auto dot = path.find_last_of(L'.');
    if (dot == path.npos || slash == path.npos || dot < slash) return false;
    const auto directory = path.substr(0, slash + 1);
    path.replace(dot, path.npos, L".ini");
    const HANDLE file = CreateFileW(path.c_str(), GENERIC_READ, FILE_SHARE_READ | FILE_SHARE_DELETE,
                                    nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (file == INVALID_HANDLE_VALUE) return false;
    const DWORD size = GetFileSize(file, nullptr);
    if (size < 2 || size > 16384 || size % 2 != 0) {
        CloseHandle(file);
        return false;
    }
    // Keep the file handle out of allocation's exception path.
    uint8_t bytes[16384];
    DWORD n = 0;
    const bool read = ReadFile(file, bytes, size, &n, nullptr) && n == size;
    CloseHandle(file);
    if (!read || bytes[0] != 0xff || bytes[1] != 0xfe) return false;
    std::u16string text;
    for (DWORD i = 2; i < size; i += 2)
        text.push_back(static_cast<char16_t>(bytes[i] | (bytes[i + 1] << 8)));
    ConfigIni ini;
    if (!valid_utf16(text) || text.find(u'\0') != text.npos || !parse_ini(text, ini)) return false;
    if (FAILED(CLSIDFromString(wide(ini.clsid).c_str(), &config.clsid)) ||
        FAILED(CLSIDFromString(wide(ini.profile).c_str(), &config.profile))) return false;
    if (ini.host.empty() || ini.host.find_first_of(u"\\/:") != ini.host.npos ||
        ini.host == u"." || ini.host == u"..") return false;
    config.langid = ini.langid;
    config.name = wide(ini.name);
    config.connection = wide(ini.connection);
    config.host = directory + wide(ini.host);
    return true;
}

} // namespace tango::ime
