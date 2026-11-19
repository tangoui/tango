---
title: Performance & size
---

# Performance & size

This page records **file sizes that you can reproduce**. Runtime memory, CPU and startup time were not measured for these samples, so none are given. The Chinese page [性能与体积](/guide/size) has the full method and the inline sources.

## Environment

- 2026-11-14, TanGo `main` @ `be5c620+`, Apple M4 host, Go 1.27.2.
- `CGO_ENABLED=0` cross-compilation, `-trimpath -ldflags='-s -w -buildid='`. 1 MiB = 1,048,576 bytes.
- WebView sample: one window loading `https://example.com`. Go-drawn sample: `uihost.Open` with one `ui.Button`.

## Results

| Sample | Target | Stripped executable | gzip -9 | xz -9e |
| --- | --- | ---: | ---: | ---: |
| WebView | darwin/arm64 | 5.59 MiB (5,861,026 B) | 2.32 MiB | 1.75 MiB |
| WebView | linux/amd64 | 5.70 MiB (5,972,195 B) | 2.40 MiB | 1.92 MiB |
| WebView | windows/amd64 | 5.56 MiB (5,828,096 B) | 2.35 MiB | 1.88 MiB |
| Go-drawn | darwin/arm64 | 9.68 MiB (10,152,386 B) | 3.87 MiB | — |
| Go-drawn | linux/amd64 | 10.09 MiB (10,584,291 B) | 4.08 MiB | — |
| Go-drawn | windows/amd64 | 9.84 MiB (10,317,824 B) | 3.97 MiB | — |

macOS packages of the WebView sample (`tan build` + `tan package`, not formally signed):

| Artifact | Size |
| --- | ---: |
| `Min-0.1.0-darwin-arm64.dmg`, `tan package` default, LZMA (`ULMO`) | **1.80 MiB** (1,884,576 B) |
| `Min.app` | 5.59 MiB |
| `.zip` of `Min.app` | 2.32 MiB |
| `.dmg`, LZFSE | 2.39 MiB |
| `.dmg`, zlib (`UDZO`), used by earlier `tan package` versions | 2.32 MiB |

Windows installer of the same sample (`tan package -platform windows/amd64`, NSIS `SetCompressor /SOLID lzma`, unsigned): `Min-0.1.0-windows-amd64-setup.exe` is **1.97 MiB**; the `Min.exe` inside is 5.60 MiB (zip -9: 2.36 MiB).

- Both installers are the default `tan package` output; no extra flags are needed. The LZMA dmg default arrived in `e2de8be`.
- No Linux package was built in this run, so the Linux gzip and xz figures are compressed executables, not installers.
- About 5.6 MiB of the WebView executable is the Go runtime plus `net/http` and `crypto/tls` (the asset server is an `http.Handler`). It will not get much smaller without changing that design.
- No browser engine is bundled: the system WebView (WKWebView, WebView2, WebKitGTK) is used, and its runtime is not counted.

## Reproduce

Run the size script from a Tango checkout. It writes both samples and cross-compiles them for three desktop targets.

```bash
bash scripts/size-report.sh
```

To reproduce the installer measurements, put the minimal WebView program in its own Go module with a `replace` pointing to the Tango checkout. Put `{"name":"Min","id":"com.example.min","version":"0.1.0"}` in its `tan.json` and run these commands from the sample's directory.

```bash
tan package -platform darwin/arm64   # on macOS; requires hdiutil
tan package -platform windows/amd64  # requires makensis
```

The source revision, compiler, signing, icons and embedded resources can change the results. [Sample source](/guide/size#复现)
