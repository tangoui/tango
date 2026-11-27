#pragma once
#include "bridge.h"
#include <ctfutb.h>

namespace tango::ime {

// These TSF interfaces are missing from some mingw-w64 SDKs. The vtables
// and IIDs match the Windows SDK msctf.h and ctfutb.h declarations.
inline constexpr IID processor_ex_iid = {0x6e4e2102, 0xf9cd, 0x433d, {0xb4,0x96,0x30,0x3c,0xe0,0x3a,0x65,0x07}};
inline constexpr IID button_iid = {0x28c7f1d0, 0xde25, 0x11d2, {0xaf,0xdd,0x00,0x10,0x5a,0x27,0x99,0xb5}};
inline constexpr GUID input_mode_item = {0x2c77a81e, 0x41cc, 0x4178, {0xa3,0xa7,0x5f,0x8a,0x98,0x75,0x68,0xe6}};
inline constexpr GUID immersive_support = {0x13a016df, 0x560b, 0x46cd, {0x94,0x7a,0x4c,0x3a,0xf1,0xe0,0xe3,0x5d}};
inline constexpr GUID systray_support = {0x25504fb4, 0x7bab, 0x4bc1, {0x9c,0x69,0xcf,0x81,0x89,0x0f,0x0e,0xf5}};
inline constexpr GUID ui_element_enabled = {0x49d2f9cf, 0x1f5e, 0x11d7, {0xa6,0xd3,0x00,0x06,0x5b,0x84,0x43,0x5c}};

struct TipProcessorEx : ITfTextInputProcessor {
    virtual HRESULT STDMETHODCALLTYPE ActivateEx(ITfThreadMgr*, TfClientId, DWORD) = 0;
};

struct TipMenu : IUnknown {
    virtual HRESULT STDMETHODCALLTYPE AddMenuItem(UINT, DWORD, HBITMAP, HBITMAP,
                                                    const WCHAR*, ULONG, TipMenu**) = 0;
};

struct TipLangBarButton : ITfLangBarItem {
    virtual HRESULT STDMETHODCALLTYPE OnClick(int, POINT, const RECT*) = 0;
    virtual HRESULT STDMETHODCALLTYPE InitMenu(TipMenu*) = 0;
    virtual HRESULT STDMETHODCALLTYPE OnMenuSelect(UINT) = 0;
    virtual HRESULT STDMETHODCALLTYPE GetIcon(HICON*) = 0;
    virtual HRESULT STDMETHODCALLTYPE GetText(BSTR*) = 0;
};

} // namespace tango::ime
