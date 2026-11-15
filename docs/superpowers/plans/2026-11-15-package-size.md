# Compatible Package Size Reduction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce desktop download and installed application bytes without dropping capabilities or changing public APIs.

**Architecture:** Preserve Go dead-code elimination by installing global feedback renderers on the owning UI Context only when their public opening API is called. Keep fixed existing rendering order, frame timing, animations and scope behavior. Use best gzip compression for existing Linux tar.gz format; retain only optimizations whose before/after measurements show a benefit.

**Tech Stack:** Go 1.27.2, Tango UI, gzip/tar, macOS hdiutil ULMO, NSIS solid LZMA.

**Spec:** User selected compatibility-first option A and instructed direct execution. No optional capability-removal build mode, no UPX and no release/tag changes.

## Global Constraints
- Public Go/JS APIs and visible component behavior unchanged; an application that calls a feature still gets it automatically.
- Ant Design 6.7, Pro v6, Element Plus 2.14.8, shadcn/ui v4 baselines unchanged.
- No removal of runtime, TLS, browser, font, script or accessibility capabilities.
- Existing macOS minimum 11.0 and existing Linux/Windows archive formats unchanged.
- Four samples: minimal WebView, default vanilla template, minimal self-drawn, default UI template. Measure three targets darwin/arm64, linux/amd64, windows/amd64 with identical compiler flags.
- Installed size means logical file bytes, not RSS or virtual address space. Download size means actual installer/archive where produced.

## Review Focus
- First use in the page callback must display in the same frame.
- Layer ordering must not depend on which feature was opened first.
- Two concurrent windows/drivers must not share renderer registration/state.
- Closing handles and exit animations must continue on later frames without reopening APIs.
- Linux tar payload bytes, executable modes and names must survive compression unchanged.

### Task 1: On-demand feedback renderer reachability
**Files:** `ui/globals.go`, `ui/context.go` (Context declaration location verified during execution), `ui/more_confirm.go`, `ui/nav_contextmenu.go`, `ui/nav_notify.go`; create `ui/globals_test.go`.
**Interfaces:** internal fixed layer identifiers and `(*Context).useGlobalLayer(kind globalLayer, render func(*Context))`; existing public entry points unchanged.
- [ ] Add failing tests for empty initial registry, same-frame install/render, ordering, repeated install and per-Context isolation.
- [ ] Add regression coverage through real Confirm, Message, Notification and context-menu APIs; reuse existing theme/scope/animation tests.
- [ ] Remove four init registrations; install the renderer at each feature's open/create entry point; store slots on Context.
- [ ] Run ui tests and race tests for registration/isolation; run apitxt -check.
- [ ] Rebuild four samples with the exact baseline flags; reject change if it causes regressions or no useful installed-size benefit.

### Task 2: Best gzip for existing Linux tarball
**Files:** `cmd/tan/package.go`, `cmd/tan/package_test.go`.
**Interfaces:** existing `writeTarGz(out, prefix string, files map[string]fileEntry) error` unchanged.
- [ ] Test gzip/tar round-trip and compressed-byte benefit on a deterministic repeated payload against default gzip.
- [ ] Change to gzip.BestCompression, preserve error handling and tar metadata.
- [ ] Run cmd/tan tests; measure actual Linux tar package before/after.

### Task 3: Reproducible result report and acceptance
**Files:** `scripts/size-report.sh`, size documentation/report under `docs/`, `CHANGELOG.md`.
- [ ] Align measurement flags with actual tan release flags (-trimpath -s -w), record baseline commit/compiler/platforms and exact byte counts.
- [ ] Measure raw executables, gzip estimates and actual macOS dmg/Windows NSIS/default-template installed files. Report unsupported platform installation tests honestly.
- [ ] Check gofmt, API snapshot, root short-mode suite, CLI suite and native self-drawn/feedback smoke. Compare generated installers and archive contents.
- [ ] Record deltas separately for installed files and compressed downloads; do not present gzip as an actual Linux installer or old minimal figures as current results.

## Baseline
Source b42c75b, Go 1.27.2, CGO_ENABLED=0 -trimpath -ldflags='-s -w'. Raw and gzip-9 exact bytes are in the execution scratch directory. Clearing build ID did not change darwin sample file length and is not an accepted optimization.
