---
layout: home
title: Tango
titleTemplate: Desktop and mobile apps in Go

hero:
  name: Tango
  text: Desktop and mobile apps in Go
  tagline: Use the system WebView, draw the interface in Go, or mix both in one window. The WebView is the one the OS already ships, so no browser engine is bundled.
  image:
    src: /logo.svg
    alt: Tango
  actions:
    - theme: brand
      text: Get started
      link: /en/guide/getting-started
    - theme: alt
      text: Try the components
      link: /components/
    - theme: alt
      text: GitHub
      link: https://github.com/tangoui/tango
---

<HomeStats lang="en" />

::: warning Status: 0.x, the API still changes
Desktop is verified on **macOS hardware only**. Windows and Linux are cross-compiled and covered by CI smoke tests, but have not been validated systematically on hardware. Mobile is **experimental**: iOS has run in simulators and once on a real iPhone 12 Pro (iOS 27, 2026-11-14); Android has run in emulators only. See [Platform support](/en/guide/platforms).
:::

<div class="home-head">

## What it's built around

</div>

<div class="home-grid">
<div class="home-card">

### One Go codebase, desktop and mobile

`App` and `Surface` are the same API on all five platforms. Differences between platforms are listed in capability tables; nothing fails without telling you.

</div>
<div class="home-card">

### System WebView, no bundled engine

Tango uses WKWebView, WebView2, WebKitGTK and Android WebView. Bring any frontend. Go and the page talk through calls, events and streams with backpressure.

</div>
<div class="home-card">

### Go-drawn GUI

Components keep their state and logic in Go; Tango does layout and drawing. Rendering defaults to the GPU (Metal, Direct3D 11, OpenGL, OpenGL ES) and falls back to the CPU.

</div>
<div class="home-card">

### Three design languages

Ant Design, Element Plus and shadcn themes share one set of semantic tokens, each in light and dark.

</div>
<div class="home-card">

### Native capabilities

`sys/` packages cover menus, tray, notifications, clipboard and hotkeys on desktop, and permissions, share, biometrics, location and push on mobile. Unsupported calls return `ErrUnsupported`.

</div>
<div class="home-card">

### From template to installer

`tan` handles templates, hot reload, builds, packaging (dmg, NSIS, MSIX, deb, rpm, AppImage, Flatpak, apk, aab, ipa), signing and signed auto-updates.

</div>
</div>

<div class="home-head">

## Measured sizes, reproducible

[Size details →](/en/guide/size)

</div>

<p class="home-lead">Measured 2026-11-14 with Go 1.27.2, cross-compiled on an Apple M4, <code>CGO_ENABLED=0</code>, <code>-trimpath -ldflags='-s -w -buildid='</code>. 1 MiB = 1,048,576 bytes. These are file sizes only; runtime memory and CPU were not measured.</p>

<div class="home-grid cols-2">
<div class="home-card">
<span class="metric">1.80 <small>MiB</small></span>

### Smallest installer (macOS .dmg)

A minimal WebView app that loads one page, as the default `tan package` .dmg (LZMA, not formally signed). The Windows NSIS installer of the same app, also a default output, is 1.97 MiB. No Linux package was measured.

</div>
<div class="home-card">
<span class="metric">5.56–5.70 <small>MiB</small></span>

### Minimal WebView executable

Stripped executables for darwin/arm64, linux/amd64 and windows/amd64. Most of it is the Go runtime, `net/http` and `crypto/tls`. With xz -9e they come to 1.75–1.92 MiB.

</div>
<div class="home-card">
<span class="metric">9.68–10.09 <small>MiB</small></span>

### Minimal Go-drawn app

`uihost.Open` with one `ui.Button`. The extra size is the component and renderer code. With gzip -9 they come to 3.87–4.08 MiB.

</div>
<div class="home-card">
<span class="metric">2.78–5.11 <small>MB</small></span>

### Default-template installers

From a separate run (2026-11-13, decimal MB): 11 installer formats built from the default `tan init` templates. See the [package size record](https://github.com/tangoui/tango/blob/main/docs/package-size.md); its dmg sizes predate the LZMA switch.

</div>
</div>

<div class="home-head">

## Where it runs today

[Platform details →](/en/guide/platforms)

</div>

<div class="home-grid">
<div class="home-card">

### macOS <span class="home-badge ok">hardware-verified</span>

<span class="stack">Cocoa · WKWebView · Metal · CoreText</span>

WebView, Go-drawn and hybrid. Ships as .dmg, with signing and notarization.

</div>
<div class="home-card">

### Windows <span class="home-badge">cross-compiled + CI</span>

<span class="stack">Win32 · WebView2 · Direct3D 11 · DirectWrite</span>

WebView, Go-drawn and hybrid. Ships as NSIS or MSIX. Direct3D 11 rendering has not been checked on hardware yet.

</div>
<div class="home-card">

### Linux <span class="home-badge">cross-compiled + CI</span>

<span class="stack">GTK 3 · WebKitGTK 4.1 · OpenGL 3.3 · FreeType/fontconfig</span>

WebView, Go-drawn and hybrid. Ships as tar.gz, deb, rpm, AppImage or Flatpak. OpenGL has only been tested in containers.

</div>
<div class="home-card">

### iOS <span class="home-badge warn">experimental</span>

<span class="stack">UIKit · WKWebView · Metal · CoreText</span>

WebView and Go-drawn pages; builds .app and .ipa. Tested in simulators, plus one run on a real iPhone 12 Pro.

</div>
<div class="home-card">

### Android <span class="home-badge warn">experimental</span>

<span class="stack">View · WebView · OpenGL ES 3.0 · pure-Go text</span>

WebView and Go-drawn pages; builds .apk and .aab. Tested on the emulator (API 36) only.

</div>
</div>

<div class="home-head">

## A real interface

[Try the components →](/components/)

</div>

<div class="gui-showcase">

![Tango Go GUI: navigation, statistics, filters and a data table](/screenshots/gui-preview.png)

Rendered by `examples/ui-gallery`: navigation, statistics, filters and a data table.

</div>

<div class="home-head">

## In one minute

[Getting started →](/en/guide/getting-started)

</div>

```bash
go install github.com/tangoui/tango/cmd/tan@latest
tan init -template ui hello  # Go-drawn GUI template
cd hello
tan dev                      # opens the window, rebuilds on Go changes
tan build                    # release build in build/
tan package                  # installer for this machine
```

::: info Documentation language
Chinese is the primary documentation. Some English pages are still being translated; untranslated pages link to the Chinese original.
:::

<div class="home-footer">
<div>
<strong>Tango</strong>
<p>A Go application framework for desktop and mobile, with system WebView and Go-drawn UI. MIT licensed.</p>
</div>
<div>
<strong>Docs</strong>
<ul>
<li><a href="./guide/getting-started">Getting started</a></li>
<li><a href="./guide/concepts">Core concepts</a></li>
<li><a href="./guide/ui">Go-drawn UI</a></li>
<li><a href="./guide/packaging">Packaging</a></li>
</ul>
</div>
<div>
<strong>Reference</strong>
<ul>
<li><a href="../components/">Components</a></li>
<li><a href="./reference/cli">tan CLI</a></li>
<li><a href="./reference/api">Go API</a></li>
<li><a href="./reference/js-api">JS runtime API</a></li>
</ul>
</div>
<div>
<strong>More</strong>
<ul>
<li><a href="./guide/platforms">Platform support</a></li>
<li><a href="./guide/size">Performance &amp; size</a></li>
<li><a href="https://github.com/tangoui/tango">GitHub</a></li>
<li><a href="https://github.com/tangoui/tango/blob/main/CHANGELOG.md">Changelog</a></li>
</ul>
</div>
</div>
