---
title: Platform support
---

# Platform support

All five platforms share the `App` / `Surface` API, but each one uses different system technologies and offers different capabilities. The Chinese page [平台支持](/guide/platforms) has more detail.

| Platform | Stack | Interfaces | Packages | Verification |
| --- | --- | --- | --- | --- |
| macOS | Cocoa · WKWebView · Core Animation / Metal · CoreText | WebView · Go-drawn · hybrid | .dmg (signing, notarization) | Verified on macOS hardware |
| Windows | Win32 · WebView2 · Direct3D 11 · DirectWrite | WebView · Go-drawn · hybrid | NSIS, MSIX (Authenticode) | Cross-compiled + CI smoke tests; no systematic hardware validation yet |
| Linux | GTK 3 · WebKitGTK 4.1 · OpenGL 3.3 · FreeType/fontconfig | WebView · Go-drawn · hybrid | tar.gz, deb, rpm, AppImage, Flatpak | Cross-compiled + CI smoke tests; OpenGL tested with Mesa llvmpipe; headless Weston smoke tests; GPU hardware not yet validated |
| iOS | UIKit · WKWebView · Metal · CoreText | WebView · Go-drawn | .app, .ipa | Experimental: simulators and iPhone 12 Pro (iOS 27.0.1), including IPC reload recovery and fault fallback; `up:post` on by default |
| Android | View · WebView · OpenGL ES 3.0 · pure-Go text | WebView · Go-drawn | .apk, .aab | Experimental: emulator (API 36) only |

On Apple Silicon macOS, Go-drawn windows start on native Core Animation layers. The default `ui.ContentGeneral` switches once to Metal on keyboard input or sustained high frame cost. `ui.ContentData` stays on native layers for keyboard browsing and switches only for sustained high frame cost. Intel Mac uses Metal by default. Desktop overrides include `TANGO_RENDERER=cpu` and `gpu`, with `native` also available on Apple Silicon macOS. macOS compiles only CoreText by default. Build with `-tags tango_gotext` to restore go-text.

Go-drawn windows and pages fall back to the CPU when the GPU is unavailable. Mobile WebView overlays use CPU rendering. Before relying on a feature, check [WebView capabilities](https://github.com/tangoui/tango/blob/main/docs/webview.md), [mobile](https://github.com/tangoui/tango/blob/main/docs/mobile.md) and [runtime APIs](https://github.com/tangoui/tango/blob/main/docs/runtime-api.md).
