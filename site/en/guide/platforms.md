---
title: Platform support
---

# Platform support

All five platforms share the `App` / `Surface` API, but each one uses different system technologies and offers different capabilities. The Chinese page [平台支持](/guide/platforms) has more detail.

| Platform | Stack | Interfaces | Packages | Verification |
| --- | --- | --- | --- | --- |
| macOS | Cocoa · WKWebView · Metal · CoreText | WebView · Go-drawn · hybrid | .dmg (signing, notarization) | Verified on macOS hardware |
| Windows | Win32 · WebView2 · Direct3D 11 · DirectWrite | WebView · Go-drawn · hybrid | NSIS, MSIX (Authenticode) | Cross-compiled + CI smoke tests; no systematic hardware validation yet |
| Linux | GTK 3 · WebKitGTK 4.1 · OpenGL 3.3 · FreeType/fontconfig | WebView · Go-drawn · hybrid | tar.gz, deb, rpm, AppImage, Flatpak | Cross-compiled + CI smoke tests; OpenGL tested in containers only |
| iOS | UIKit · WKWebView · Metal · CoreText | WebView · Go-drawn | .app, .ipa | Experimental: simulators, plus one run on an iPhone 12 Pro (iOS 27, 2026-11-14) |
| Android | View · WebView · OpenGL ES 3.0 · pure-Go text | WebView · Go-drawn | .apk, .aab | Experimental: emulator (API 36) only |

Go-drawn rendering falls back to the CPU when no GPU device is available (`TANGO_RENDERER=cpu` forces it on desktop). Before relying on a feature, check [WebView capabilities](https://github.com/tangoui/tango/blob/main/docs/webview.md), [mobile](https://github.com/tangoui/tango/blob/main/docs/mobile.md) and [runtime APIs](https://github.com/tangoui/tango/blob/main/docs/runtime-api.md).
