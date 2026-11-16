# Native macOS Renderer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Reduce native-UI process memory toward NSGo without dropping drawing operations or trading away sustained frame performance.

**Architecture:** Default eligible macOS/arm64 surfaces use retained Core Animation content layers and CoreText batches. Correct CPU patches/fallback preserve remaining display-list semantics; existing Metal remains forced/automatic for sustained expensive frames.

**Tech Stack:** Go 1.27, purego 0.11.1, CALayer/CoreGraphics/CoreText, existing paint.CPU and Metal.

**Spec:** `docs/superpowers/specs/2026-11-16-native-renderer-design.md`

## Global Constraints

- Preserve UI component APIs and Ant Design 6.7 / Pro v6 baseline.
- No helper-process memory shifting; source snapshots/probes remain separate from production.
- No weakening existing CPU/Metal image tests or timing thresholds.
- Internal glyph bridge only; no public Face/paint API additions.
- Native eligibility is macOS/arm64; preserve Intel macOS and other platforms.
- Explicit gpu/cpu/auto semantics preserved; add native force mode, report native accurately.
- Work inline in the isolated perf/macos-native-renderer checkout, based on 10fcb8d.

## Review Focus

- Correct visible text/ink bounds at fractional positions/scales, including custom/color-font fallback.
- Mutated shared glyph slices must not mutate retained content unnoticed.
- Clip stacks, missing/unclosed Pop, fallback operations and alpha must render completely.
- Repeated resize/scale/trim/close must release CF/layer/font ownership and stale callbacks.
- Promotion failure must leave a visible CPU/native frame and preserve input/IME scheduling.

## Task 1: Internal native glyph bridge

**Files:** create `internal/nativeglyph/coregraphics.go`, `ui/text/nativeglyph_darwin.go`, and `ui/text/nativeglyph_darwin_test.go`.

**Interfaces:** `nativeglyph.Run { Source paint.GlyphSource; Size, Skew float32; IDs []uint16; Positions []geom.Point }`, positions in device-pixel CoreGraphics y-up coordinates. Global immutable `CoreGraphics Provider` registered by ui/text on Darwin. Provider has `Available(paint.GlyphSource) bool`, `Key(paint.GlyphSource) uintptr`, `Bounds(Run) (image.Rectangle,bool)`, `Draw(uintptr,Run) bool`. Calls borrow the context and slices synchronously, preserve source/owner liveness, use existing font/variation/hinting, and decline unsupported sources.

- [x] Write real CoreText tests for visible ink, Latin/CJK/RTL, quarter-pixel placement, size/skew, and native bounds containing the existing RasterGlyph ink; observe RED before provider implementation.
- [x] Implement stateless registration and synchronized per-raster bounded native metrics/position scratch. Do not alter public Face methods or existing raster behavior.
- [x] Test bridge and existing font ownership/variation/layout suites, then commit.

## Task 2: Retained native display-list rendering

**Files:** create `internal/render/quartz/{plan.go,key.go,renderer_darwin_arm64.go,native_darwin_arm64.go}` and focused planner/native tests.

**Interfaces:** `NewPresenter(uintptr) (paint.Renderer, func(image.Point,float32) error,error)`; renderer supports Close, Trim, InvalidateImage and `FrameMetrics() (time.Duration,bool)` (work and full-software-fallback). Plan normalizes glyph positions and immutable content keys independently of integer layer movement; native registry/layer owners are private.

- [x] Write RED tests for identical content reuse, moving text without repaint, changed/mutated glyphs, complete fallbacks, clip semantics and bounded retirement.
- [x] Implement retained layers: native square fills; bounded cached CPU patches for rounded fills/borders/shadows/paths/gradients/checker/images; bounded text content layers with native batches. Reuse matching content across order changes, not only by slice index.
- [x] Normalize pointer ops safely. Use correct full-frame CPU fallback for transforms, opacity groups, rounded clips, backdrop blur or unsupported native glyph runs; never omit an op.
- [x] Deep-own changing glyph data, retain only current layers/resources, and invalidate image snapshots when notified. Avoid rerasterizing unchanged text/patches or rebuilding native colors/paths each frame.
- [x] Run real offscreen layer rendering tests at scales 1,1.5,2; text ink/bounds, clipping, alpha, patch pixel parity, reused buffers, close/trim and all existing renderer conformance scenes; commit.

## Task 3: Canvas lifecycle and renderer selection

**Files:** add internal CanvasCoreGraphics capability and `internal/platform/darwin/canvas_quartz_darwin_arm64.go`; update canvas scheduling/present/resize/destroy and Metal switching; add `uihost/native.go`, platform factory file and selection/promotion tests.

**Interfaces:** `CanvasCoreGraphics.UseCoreGraphicsLayer() (uintptr,error)` returns a distinct hosted CALayer, UI thread only. Keep native and Metal identities separate. Eligibility/factory failure uses existing GPU/CPU paths. Native default is overridable; explicit native does not promote.

- [x] Write RED tests for idempotent acquisition, no frame before ready, Present(nil), resize/scale, close and separate layer identities.
- [x] Implement native canvas hosting using existing display-link/coalescing/input paths; ensure references/delegates are released and Metal switching clears obsolete native identity.
- [x] Write RED policy tests: six >8ms frames within 100ms gaps promote; spaced/cold/fast native frames do not; six rapid full-fallback frames may promote; force-native never promotes; promotion failure is visible and sticky.
- [x] Integrate default-native eligible surfaces, preserve explicit modes and caller renderers, report native correctly and forward image invalidation. Exercise existing GPU/fallback/IME/input tests; commit.

## Task 4: Verification and measured acceptance

**Files:** add renderer policy documentation and measured report under `docs/perf/`.

- [x] Run native window image/resize/input smoke checks; compare matched GPU/native and pinned current NSGo scenes, three alternating-order rounds plus minute scrolling, no diagnostic GC in headline results.
- [x] Verify all text/rows are actually drawn before accepting memory numbers. Record CPU, WindowServer increment, process footprint, callbacks and tails; reject a default that substantially regresses sustained CPU/frame behavior.
- [x] Run complete Go suite with CPU-heavy packages serialized as CI, development builds, vet, relevant race tests, and Linux/Windows/Intel-mac cross builds; preserve existing thresholds.
- [x] Independent whole-branch review, fix important findings with RED/GREEN, document limitations and reproducible results, then integrate only verified code into main.
