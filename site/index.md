---
layout: home
title: Tango
titleTemplate: 用 Go 写桌面与手机应用

hero:
  name: Tango
  text: 用 Go 写桌面与手机应用
  tagline: 界面可以交给系统 WebView，也可以用 Go 自绘，两者能在同一个窗口里混用。WebView 用系统自带的，安装包里没有浏览器引擎。
  image:
    src: /logo.svg
    alt: Tango
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/getting-started
    - theme: alt
      text: 在线体验组件
      link: /components/
    - theme: alt
      text: GitHub
      link: https://github.com/tangoui/tango
---

<HomeStats />

::: warning 当前状态：0.x，API 仍会变化
桌面端只在 **macOS 实机**上验证过；Windows、Linux 做了交叉编译和 CI 冒烟测试，还没有系统的实机验证。移动端是**实验性**的：iOS 在模拟器上测过，另外在一台 iPhone 12 Pro（iOS 27）上做过一次真机运行（2026-11-14）；Android 只在模拟器上测过。各平台的具体情况见 [平台支持](/guide/platforms)。
:::

<div class="home-head">

## 围绕什么构建

</div>

<p class="home-lead">Tango 的取舍，以及每一项目前做到哪一步。</p>

<div class="home-grid">
<div class="home-card">

### 一套 Go 代码，桌面加手机

`App` 和 `Surface` 在五个平台上是同一套 API。桌面上 Surface 是窗口，手机上是页面栈里的一页。平台之间的能力差异写在能力表里，不会悄悄失效。

</div>
<div class="home-card">

### 系统 WebView，不带浏览器引擎

用的是 WKWebView、WebView2、WebKitGTK 和 Android WebView。前端框架随你选；Go 和页面之间有方法调用、事件，以及带背压的流。

</div>
<div class="home-card">

### Go 自绘 GUI

组件用 Go 写状态和逻辑，由 Tango 负责布局和绘制。默认走 GPU 渲染（Metal、Direct3D 11、OpenGL、OpenGL ES），设备起不来时回退到 CPU。

</div>
<div class="home-card">

### 三套设计语言

内置 Ant Design、Element Plus、shadcn 三套主题，共享同一套语义令牌，都有深色和浅色。切换主题不需要改组件代码。

</div>
<div class="home-card">

### 原生系统能力

`sys/` 下按能力分包：桌面有菜单、托盘、通知、剪贴板、快捷键等，移动端有权限、分享、生物识别、定位、推送等。不支持时返回 `ErrUnsupported`。

</div>
<div class="home-card">

### 从模板到安装包

`tan` 一个命令行工具负责模板、热重载、构建、打包（dmg、NSIS、MSIX、deb、rpm、AppImage、Flatpak、apk、aab、ipa）、签名和签名的自动更新。

</div>
</div>

<div class="home-head">

## 实测体积，可复现

[体积测量详情 →](/guide/size)

</div>

<p class="home-lead">2026-11-14 实测：Go 1.27.2，在 Apple M4 上交叉编译，<code>CGO_ENABLED=0</code>，<code>-trimpath -ldflags='-s -w -buildid='</code>。1 MiB = 1,048,576 字节。只统计体积，没有测运行时的内存或 CPU。</p>

<div class="home-grid cols-2">
<div class="home-card">
<span class="metric">1.80 <small>MiB</small></span>

### 最小安装包（macOS .dmg）

一个只加载网页的最小 WebView 应用，`tan package` 默认打出的 .dmg（LZMA 压缩，未正式签名）。同一个应用的 Windows NSIS 安装程序是 1.97 MiB，也是默认产物。Linux 安装包这次没有测。

</div>
<div class="home-card">
<span class="metric">5.56–5.70 <small>MiB</small></span>

### 最小 WebView 应用的可执行文件

去掉符号后的可执行文件（darwin/arm64、linux/amd64、windows/amd64）。其中大部分是 Go 运行时、`net/http` 和 `crypto/tls`。用 xz -9e 压缩后是 1.75–1.92 MiB。

</div>
<div class="home-card">
<span class="metric">9.68–10.09 <small>MiB</small></span>

### 最小自绘 GUI 应用

`uihost.Open` 加一个 `ui.Button`，同样是去掉符号的三平台可执行文件，多出来的是自绘组件和渲染器的代码。gzip -9 后是 3.87–4.08 MiB。

</div>
<div class="home-card">
<span class="metric">2.78–5.11 <small>MB</small></span>

### 默认模板的安装包

另一次测量（2026-11-13，十进制 MB）：`tan init` 生成的默认 WebView 和自绘模板，在三个桌面平台上打出的 11 种安装包。详见 [包体积记录](https://github.com/tangoui/tango/blob/main/docs/package-size.md)，其中 dmg 是改用 LZMA 之前测的。

</div>
</div>

<div class="home-head">

## 各平台现状

[平台支持详情 →](/guide/platforms)

</div>

<div class="home-grid">
<div class="home-card">

### macOS <span class="home-badge ok">实机验证</span>

<span class="stack">Cocoa · WKWebView · Metal · CoreText</span>

支持 WebView、自绘和混合界面。打包为 .dmg，可签名和公证。最小 WebView 应用的 .dmg 为 1.80 MiB。

</div>
<div class="home-card">

### Windows <span class="home-badge">交叉编译 + CI</span>

<span class="stack">Win32 · WebView2 · Direct3D 11 · DirectWrite</span>

支持 WebView、自绘和混合界面。打包为 NSIS 或 MSIX，可做 Authenticode 签名。最小 WebView 应用的 NSIS 安装程序为 1.97 MiB。Direct3D 11 渲染还没有在实机上验收。

</div>
<div class="home-card">

### Linux <span class="home-badge">交叉编译 + CI</span>

<span class="stack">GTK 3 · WebKitGTK 4.1 · OpenGL 3.3 · FreeType/fontconfig</span>

支持 WebView、自绘和混合界面。打包为 tar.gz、deb、rpm、AppImage 或 Flatpak。OpenGL 渲染只在容器里（Mesa llvmpipe）测过。

</div>
<div class="home-card">

### iOS <span class="home-badge warn">实验性</span>

<span class="stack">UIKit · WKWebView · Metal · CoreText</span>

支持 WebView 页和自绘页，产物为 .app 和 .ipa。在模拟器上测过，另在一台 iPhone 12 Pro 上做过一次真机运行。

</div>
<div class="home-card">

### Android <span class="home-badge warn">实验性</span>

<span class="stack">View · WebView · OpenGL ES 3.0 · 纯 Go 文字光栅化</span>

支持 WebView 页和自绘页，产物为 .apk 和 .aab。只在模拟器（API 36）上测过，还没有上过真机。

</div>
</div>

<div class="home-head">

## 真实界面

[在线操作组件 →](/components/)

</div>

<div class="gui-showcase">

![Tango Go GUI：导航、统计卡片、筛选表单和数据表格](/screenshots/gui-preview.png)

这张截图由 `examples/ui-gallery` 实际渲染生成，包含导航、统计卡片、筛选表单和数据表格。

</div>

<div class="home-head">

## 一分钟上手

[快速开始 →](/guide/getting-started)

</div>

```bash
go install github.com/tangoui/tango/cmd/tan@latest
tan init -template ui hello  # Go 自绘 GUI 模板；WebView 模板见快速开始
cd hello
tan dev                      # 打开窗口，Go 代码改动后自动重建
tan build                    # 发布构建，产物在 build/
tan package                  # 生成本机平台的安装包
```

<div class="home-footer">
<div>
<strong>Tango</strong>
<p>Go 应用框架，覆盖桌面和手机，可用系统 WebView，也可用 Go 自绘界面。MIT 许可。</p>
</div>
<div>
<strong>文档</strong>
<ul>
<li><a href="./guide/getting-started">快速开始</a></li>
<li><a href="./guide/concepts/app-surface">核心概念</a></li>
<li><a href="./guide/ui">自绘 UI</a></li>
<li><a href="./guide/packaging">打包与发布</a></li>
</ul>
</div>
<div>
<strong>参考</strong>
<ul>
<li><a href="./components/">组件</a></li>
<li><a href="./reference/cli">命令行 tan</a></li>
<li><a href="./reference/api/">Go API</a></li>
<li><a href="./reference/js-api">前端运行时 API</a></li>
</ul>
</div>
<div>
<strong>更多</strong>
<ul>
<li><a href="./guide/platforms">平台支持</a></li>
<li><a href="./guide/size">性能与体积</a></li>
<li><a href="https://github.com/tangoui/tango">GitHub</a></li>
<li><a href="https://github.com/tangoui/tango/blob/main/CHANGELOG.md">更新日志</a></li>
</ul>
</div>
</div>
