---
title: 平台支持
outline: [2, 3]
---

# 平台支持

Tango 面向 macOS、Windows、Linux、iOS 和 Android 五个平台，五个平台共用 `App` 和 `Surface` 这套 API。各平台的系统接口不同，能用的功能也不完全一样。下面写清楚每个平台用了哪些系统技术、能打出什么包，以及**目前实际验证到哪一步**。

::: warning 0.x
只有 macOS 做过实机验证。其他平台写着“已实现”，意思是代码写好了、单元测试和冒烟测试通过，不代表在真机上系统地测过。依赖某项功能之前，请先查 [WebView 能力表](https://github.com/tangoui/tango/blob/main/docs/webview.md)、[移动端文档](https://github.com/tangoui/tango/blob/main/docs/mobile.md) 和 [运行时 API](https://github.com/tangoui/tango/blob/main/docs/runtime-api.md)。
:::

## 验证状态

| 平台 | 界面 | 验证情况 |
| --- | --- | --- |
| macOS | WebView · 自绘 · 混合 | 在 macOS 实机上验证 |
| Windows | WebView · 自绘 · 混合 | 已实现；交叉编译，CI 跑冒烟测试；还没有系统的实机验证，Direct3D 11 渲染待实机验收 |
| Linux | WebView · 自绘 · 混合 | 已实现；交叉编译，CI 跑冒烟测试；OpenGL 渲染只在容器里测过（Mesa llvmpipe、Xvfb），真实 GPU、Wayland 和高分屏待验收 |
| iOS | WebView · 自绘 | 实验性；在模拟器上测过，另在一台 iPhone 12 Pro（iOS 27.0.1）上做过一次真机运行（2026-11-14） |
| Android | WebView · 自绘 | 实验性；只在模拟器（AVD API 36）上测过，还没有上过真机 |

桌面上可以在同一个窗口里混用 WebView 和自绘内容（见 `examples/webview-overlay`）。移动端可以把自绘层叠在 WebView 页的上方或下方，叠放层只走 CPU 渲染。

## macOS

- **系统技术**：Cocoa（AppKit）窗口、WKWebView、Metal（`CAMetalLayer`）、CoreText。Cocoa 和 Metal 通过 purego 和 objc 运行时调用，不需要 cgo。
- **自绘渲染**：默认用 Metal，起不来时回退到 CPU；设置 `TANGO_RENDERER=cpu` 可强制走 CPU。
- **打包**：`tan package` 生成 `.dmg`，里面是应用和指向“应用程序”文件夹的链接。支持 `darwin/universal`。用 `tan sign` 签名和公证。最小 WebView 应用的默认 .dmg（LZMA）为 1.80 MiB（见 [性能与体积](./size)）。
- **状态**：实机验证。

## Windows

- **系统技术**：Win32 窗口、WebView2、Direct3D 11（DXGI flip-model 交换链）、DirectWrite。通过 `syscall` 调 COM，不需要 cgo。
- **自绘渲染**：默认用 Direct3D 11，失败时回退到 CPU（`StretchDIBits`）。
- **打包**：NSIS 安装器（按用户安装）；加 `-format msix` 打 MSIX。支持 Authenticode 签名。最小 WebView 应用的 NSIS 安装程序为 1.97 MiB（见 [性能与体积](./size)）。
- **系统依赖**：目标机器上要有 WebView2 运行时，它不计入应用体积。
- **状态**：交叉编译，CI 跑冒烟测试；安装器只验证过能生成、结构正确，没有在实机上安装运行过。

## Linux

- **系统技术**：GTK 3、WebKitGTK 4.1、OpenGL 3.3 core（通过 `GtkGLArea`）、FreeType 加 fontconfig（用 purego 运行时加载，缺这两个库时改用纯 Go 文字引擎）。
- **自绘渲染**：默认用 OpenGL。检测到 Mesa 软件光栅化（llvmpipe、softpipe、swrast）时按没有 GPU 处理，改走 CPU。
- **打包**：`tar.gz`、`deb`、`rpm`、`AppImage`、`Flatpak`。deb 和 rpm 由 tan 自己写出，并声明对 GTK 3 和 WebKitGTK 4.1 的依赖。AppImage 需要 PATH 里有 `appimagetool`，没有时只生成 `.AppDir`。
- **状态**：交叉编译，CI 跑冒烟测试。在 Debian 12 arm64 容器里装过 deb、跑过 AppImage（在 Xvfb 下运行，没有 GPU）。

## iOS（实验性）

- **系统技术**：UIKit 页面栈、WKWebView、Metal、CoreText。
- **自绘页**：用 `TGCanvasView` 加 `CAMetalLayer`，CPU 作回退；支持 `UITextInput` 输入法和 VoiceOver 无障碍桥接。
- **打包**：`tan build -platform ios-simulator` 生成模拟器用的 `.app`；`tan build -platform ios` 生成真机用的 `.app` 和 `.ipa`，需要签名身份和描述文件。也可以用 `tan build -xcode` 生成 Xcode 工程。
- **状态**：在模拟器上测过。真机只在一台 iPhone 12 Pro 上跑过一次，覆盖 WebView 和自绘示例、触摸、输入法、Metal 和 CPU 渲染以及无障碍。其他机型、iPad、推送、后台任务、小组件和 App Store 发布流程都还没验证。只支持 arm64。

## Android（实验性）

- **系统技术**：Activity 加 View 栈、`android.webkit.WebView`、OpenGL ES 3.0 加 EGL（自绘页）。文字用纯 Go 光栅化，字体通过解析系统的 `fonts.xml` 找到，不打包 FreeType。
- **自绘页**：`SurfaceView` 整页显示，默认用 GPU，失败或检测到软件光栅化时回退到 CPU。
- **打包**：调试版产物为 `<Exe>-<version>-debug.apk`，发布版为 `.apk` 和 `.aab`。
- **状态**：只在模拟器（API 36）上测过，GPU 上下文丢失和真机 GPU 都还没验收。推送需要应用自己接入 FCM。

## 相关

- [性能与体积](./size)：最小应用的实测体积和复现方法
- [打包、签名与自动更新](./packaging)
- [移动端](./mobile)
- [CI 与冒烟测试](./ci)
