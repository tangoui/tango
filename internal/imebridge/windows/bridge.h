#pragma once
#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>
#include <msctf.h>
#include <atomic>
#include <string>

#include "config_ini.h"
#include "../common/proto.h"

namespace tango::ime {

extern HMODULE module;
extern std::atomic<long> references;

template<class T> class ComPtr {
    T* value_ = nullptr;
public:
    ComPtr() = default;
    ~ComPtr() { reset(); }
    ComPtr(const ComPtr&) = delete;
    ComPtr& operator=(const ComPtr&) = delete;
    T* get() const { return value_; }
    T* operator->() const { return value_; }
    explicit operator bool() const { return value_ != nullptr; }
    void reset() { if (auto* prev = value_) { value_ = nullptr; prev->Release(); } }
    T** put() { reset(); return &value_; }
    void retain(T* value) {
        if (value) value->AddRef();
        reset();
        value_ = value;
    }
};

inline std::wstring wide(std::u16string_view text) { return {text.begin(), text.end()}; }
inline std::u16string utf16(std::wstring_view text) { return {text.begin(), text.end()}; }

struct Config {
    CLSID clsid{};
    GUID profile{};
    LANGID langid = 0;
    std::wstring name, connection, host;
};

std::wstring module_path();
bool load_config(Config& config);
HRESULT create_tip(const Config& config, REFIID iid, void** out) noexcept;

} // namespace tango::ime
