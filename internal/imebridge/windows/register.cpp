#include "bridge.h"
#include "tsf_compat.h"

namespace tango::ime {

HMODULE module;
std::atomic<long> references{0};

namespace {
class Factory final : public IClassFactory {
    ULONG refs_ = 1;
    Config config_;
public:
    explicit Factory(Config config) : config_(std::move(config)) { ++references; }
    ~Factory() { --references; }
    HRESULT STDMETHODCALLTYPE QueryInterface(REFIID iid, void** out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (iid != IID_IUnknown && iid != IID_IClassFactory) return E_NOINTERFACE;
        *out = static_cast<IClassFactory*>(this);
        AddRef();
        return S_OK;
    }
    ULONG STDMETHODCALLTYPE AddRef() override { return ++refs_; }
    ULONG STDMETHODCALLTYPE Release() override {
        const auto count = --refs_;
        if (!count) delete this;
        return count;
    }
    HRESULT STDMETHODCALLTYPE CreateInstance(IUnknown* outer, REFIID iid, void** out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (outer) return CLASS_E_NOAGGREGATION;
        return create_tip(config_, iid, out);
    }
    HRESULT STDMETHODCALLTYPE LockServer(BOOL lock) override {
        if (lock) ++references;
        else --references;
        return S_OK;
    }
};

class COM {
public:
    HRESULT result = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    ~COM() { if (SUCCEEDED(result)) CoUninitialize(); }
    bool usable() const { return SUCCEEDED(result) || result == RPC_E_CHANGED_MODE; }
};

std::wstring class_key(const CLSID& clsid) {
    wchar_t id[40]{};
    StringFromGUID2(clsid, id, ARRAYSIZE(id));
    return std::wstring(L"Software\\Classes\\CLSID\\") + id;
}

HRESULT registry_string(HKEY key, const wchar_t* name, const std::wstring& value) {
    return HRESULT_FROM_WIN32(RegSetValueExW(key, name, 0, REG_SZ,
        reinterpret_cast<const BYTE*>(value.c_str()), DWORD((value.size() + 1) * sizeof(wchar_t))));
}

HRESULT register_class(const Config& config, const std::wstring& path) {
    const auto relative = class_key(config.clsid) + L"\\InprocServer32";
    HKEY key{};
    auto hr = HRESULT_FROM_WIN32(RegCreateKeyExW(HKEY_LOCAL_MACHINE, relative.c_str(), 0,
        nullptr, 0, KEY_WRITE, nullptr, &key, nullptr));
    if (FAILED(hr)) return hr;
    hr = registry_string(key, nullptr, path);
    if (SUCCEEDED(hr)) hr = registry_string(key, L"ThreadingModel", L"Apartment");
    RegCloseKey(key);
    return hr;
}

HRESULT open_profiles(ComPtr<ITfInputProcessorProfiles>& profiles) {
    return CoCreateInstance(CLSID_TF_InputProcessorProfiles, nullptr, CLSCTX_INPROC_SERVER,
        __uuidof(ITfInputProcessorProfiles), reinterpret_cast<void**>(profiles.put()));
}

HRESULT open_categories(ComPtr<ITfCategoryMgr>& categories) {
    return CoCreateInstance(CLSID_TF_CategoryMgr, nullptr, CLSCTX_INPROC_SERVER,
        __uuidof(ITfCategoryMgr), reinterpret_cast<void**>(categories.put()));
}

const GUID* const categories_to_register[] = {
    &GUID_TFCAT_TIP_KEYBOARD, &immersive_support, &systray_support, &ui_element_enabled,
};

HRESULT remove_registration(const Config& config) {
    ComPtr<ITfCategoryMgr> categories;
    auto result = open_categories(categories);
    if (SUCCEEDED(result)) {
        for (const auto* category : categories_to_register)
            categories->UnregisterCategory(config.clsid, *category, config.clsid);
    }
    ComPtr<ITfInputProcessorProfiles> profiles;
    const auto profile_result = open_profiles(profiles);
    if (SUCCEEDED(profile_result)) {
        profiles->RemoveLanguageProfile(config.clsid, config.langid, config.profile);
        result = profiles->Unregister(config.clsid);
    } else result = profile_result;
    const LSTATUS status = RegDeleteTreeW(HKEY_LOCAL_MACHINE, class_key(config.clsid).c_str());
    if (status != ERROR_SUCCESS && status != ERROR_FILE_NOT_FOUND) return HRESULT_FROM_WIN32(status);
    return result;
}

HRESULT register_server(const Config& config, const std::wstring& path) {
    auto hr = register_class(config, path);
    ComPtr<ITfInputProcessorProfiles> profiles;
    if (SUCCEEDED(hr)) hr = open_profiles(profiles);
    if (SUCCEEDED(hr)) hr = profiles->Register(config.clsid);
    if (SUCCEEDED(hr)) hr = profiles->AddLanguageProfile(config.clsid, config.langid, config.profile,
        config.name.c_str(), static_cast<ULONG>(config.name.size()), path.c_str(),
        static_cast<ULONG>(path.size()), 0);
    ComPtr<ITfCategoryMgr> categories;
    if (SUCCEEDED(hr)) hr = open_categories(categories);
    for (const auto* category : categories_to_register) {
        if (FAILED(hr)) break;
        hr = categories->RegisterCategory(config.clsid, *category, config.clsid);
    }
    if (FAILED(hr)) remove_registration(config);
    return hr;
}
} // namespace
} // namespace tango::ime

using namespace tango::ime;

extern "C" BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) module = instance;
    return TRUE;
}

extern "C" __declspec(dllexport) HRESULT WINAPI DllCanUnloadNow() {
    return references == 0 ? S_OK : S_FALSE;
}

extern "C" __declspec(dllexport) HRESULT WINAPI DllGetClassObject(REFCLSID clsid, REFIID iid, void** out) {
    if (!out) return E_POINTER;
    *out = nullptr;
    try {
        Config config;
        if (!load_config(config) || clsid != config.clsid) return CLASS_E_CLASSNOTAVAILABLE;
        auto* factory = new Factory(std::move(config));
        const auto hr = factory->QueryInterface(iid, out);
        factory->Release();
        return hr;
    } catch (...) { return E_FAIL; }
}

extern "C" __declspec(dllexport) HRESULT WINAPI DllRegisterServer() {
    try {
        Config config;
        if (!load_config(config)) return E_FAIL;
        COM com;
        if (!com.usable()) return com.result;
        const auto path = module_path();
        return path.empty() ? E_FAIL : register_server(config, path);
    } catch (...) { return E_FAIL; }
}

extern "C" __declspec(dllexport) HRESULT WINAPI DllUnregisterServer() {
    try {
        Config config;
        if (!load_config(config)) return E_FAIL;
        COM com;
        return com.usable() ? remove_registration(config) : com.result;
    } catch (...) { return E_FAIL; }
}
