# macOS Native Lite Host Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an independent macOS-native lightweight host that can show a small CoreText-rendered window without loading Tango App, WebView, Runner, Flex, accessibility, or the Tango GPU presenter, with a measured idle physical footprint below 31 MiB.

**Architecture:** Add a Darwin-only `macoslite` package that owns NSApplication/NSWindow/NSView and a CPU bitmap drawing loop through purego; use CoreGraphics for simple fills and CoreText for short text. Keep a non-Darwin unsupported stub so the module remains cross-platform compilable. The first vertical slice is a counter window and benchmark; it is not wired into `uihost.Open` or the default renderer policy until the footprint and lifecycle are verified.

**Tech Stack:** Go 1.27; `github.com/ebitengine/purego`; AppKit, CoreGraphics, CoreText; Go `image.RGBA`; no cgo, WebKit, Metal, `tango.App`, `uihost`, `ui`, `paint`, or `text` imports in the lite package.

**Spec:** `docs/specs/macos-native-lite-design.md`

## Global Constraints

- The lite package must not import `github.com/tangoui/tango` root, `uihost`, `ui`, `ui/paint`, `ui/text`, `internal/corehook`, or `internal/platform`.
- macOS implementation uses Go + purego and `CGO_ENABLED=0`; no cgo.
- Non-macOS builds expose the same public names and return `ErrUnsupported`.
- `Run` is called from the main goroutine; native callbacks and drawing run on the AppKit main thread.
- Native ownership uses explicit release plus `runtime.AddCleanup` whose cleanup argument does not reference the owner, view, or callback closure.
- The lite host must not create WebKit/WebView, Metal, Tango GPU presenters, `ui.Runner`, Flex nodes, or accessibility trees.
- Default Tango and `uihost.Open` behavior must remain unchanged.
- First-stage window content is 360×260 with title `Lite Counter`.
- Memory metric is `proc_pid_rusage` physical footprint (not RSS), measured with the screen unlocked; target is a median below 31 MiB over three rounds.
- Commits use author `tango <903300326@qq.com>` with no `Co-Authored-By` trailer; never force-push.

## Review Focus

- **Main-thread violations:** `Run`, window creation, invalidation, draw callbacks, and close must all obey AppKit main-thread rules; test with callbacks queued from a worker goroutine in Task 3.
- **Native handle leaks/double releases:** NSApplication, NSWindow, NSView, CoreGraphics contexts, CFStrings, and CTFonts must be released exactly once; fake release counters cover this in Task 2.
- **Re-entrant drawing:** invalidating during `Draw` must schedule a later frame, not recurse; Task 3 pins this.
- **Resize/scale changes:** a zero-size or scale change must rebuild the bitmap with correct pixel dimensions and not draw into a stale buffer; Task 3 covers it.
- **Text encoding/fallback:** UTF-8 Latin, CJK, missing-glyph fallback, and an empty string must not panic and must produce a stable frame; Task 4 covers it.

---

### Task 1: Package Boundary and Unsupported Stub

**Files:**
- Create: `macoslite/api.go`
- Create: `macoslite/api_other.go`
- Create: `macoslite/api_test.go` (validation, all platforms)
- Create: `macoslite/api_other_test.go` (`//go:build !darwin`)
- Modify: `docs/specs/macos-native-lite-design.md` only if the final API names differ

**Interfaces:**
- Produce `type Options struct { Name, Title string; Width, Height float64 }`.
- Produce `type Frame struct { Width, Height, Scale float64 }`.
- Produce `type View interface { Draw(*Frame, *Canvas); Mouse(MouseEvent); Key(KeyEvent); Close() }`.
- Produce `type App struct` with `func New(Options, View) (*App, error)`, `func (*App) Run() error`, `func (*App) Stop()`, and `func (*App) Invalidate()`.
- Produce `type Canvas` with `Fill(Rect, Color)`, `RoundRect(Rect, float64, Color)` and `Text(string, Point, float64, Color)`.
- Produce value types: `Point{X, Y float64}`, `Rect{X, Y, W, H float64}` with `Contains(Point) bool`, `Color{R, G, B, A uint8}` (non-premultiplied).
- Produce `MouseEvent{Kind MouseKind; Pos Point; Button int}` with `MouseKind` constants `MouseDown`, `MouseUp`, `MouseMove`; and `KeyEvent{Key string; Code uint16; Down bool}` where Escape is `Key == "Escape"`.
- Produce `func (*App) TrimCaches()` (Task 4 fills the body).
- Produce `ErrUnsupported` on non-Darwin.
- `New` only validates and records options; it never touches native APIs (those start in `Run`, Task 3). Validation errors are `ErrInvalidOptions` (wrapped with the field name) and `ErrNilView`.

- [ ] **Step 1: Write failing API/unsupported tests**
  - `TestNewValidates` (all platforms): empty `Title`, `Width <= 0`, `Height <= 0` return `errors.Is(err, ErrInvalidOptions)`; nil view returns `ErrNilView`; valid options on Darwin return a non-nil `*App`.
  - `TestRectContains`: `Rect{10,10,20,20}` contains `{10,10}` and `{29.9,29.9}`, not `{30,30}`.
  - `TestNewUnsupported` (`api_other_test.go`, non-Darwin only): valid options return `ErrUnsupported`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./macoslite -count=1`
Expected: FAIL because the package and API do not exist.

- [ ] **Step 3: Implement the public boundary and non-Darwin stub**
  - Keep the public types platform-neutral and small.
  - Validate `Width > 0`, `Height > 0`, and non-nil `View` in `New`.
  - Make the non-Darwin implementation return `ErrUnsupported` without importing any platform package.

- [ ] **Step 4: Run tests and cross-platform vet**

Run: `go test ./macoslite -count=1`, `GOOS=linux GOARCH=amd64 go vet ./macoslite` and `GOOS=linux GOARCH=amd64 go test -c -o /dev/null ./macoslite`.
Expected: PASS on the host and PASS for Linux vet.

- [ ] **Step 5: Commit**

```bash
git add macoslite docs/specs/macos-native-lite-design.md
git commit -m "feat(macoslite): define native lite host boundary"
```

---

### Task 2: Purego Native Bindings and Ownership

**Files:**
- Create: `macoslite/native_darwin.go`
- Create: `macoslite/ownership_darwin.go`
- Create: `macoslite/ownership_darwin_test.go`
- Modify: `macoslite/api.go` only to connect Darwin internals to `App`

**Interfaces:**
- Produce internal `darwinApp`, `darwinWindow`, and `darwinView` state owned by `App`.
- Produce `newNativeOwner(handle uintptr, release func(uintptr))`, `(*nativeOwner).Release()`, and `(*nativeOwner).KeepAlive()` equivalents for ObjC/CF handles.
- Produce internal binding initialization that registers only the AppKit/CoreGraphics/CoreText symbols used by the first slice.

- [ ] **Step 1: Write failing ownership tests**
  - Use a fake release counter to prove an owner releases one retained object exactly once.
  - Cover explicit release followed by cleanup, cleanup without explicit release, zero handles, and two handles that happen to have the same numeric address but represent two retains.
  - Assert the cleanup state does not retain the owner or callback closure.

- [ ] **Step 2: Run the ownership tests to verify failure**

Run: `go test ./macoslite -run 'TestNativeOwner' -count=1`.
Expected: FAIL because ownership helpers do not exist.

- [ ] **Step 3: Implement Darwin symbol loading and ownership**
  - Load AppKit, CoreFoundation, CoreGraphics, and CoreText with purego.
  - Register only symbols needed for application lifecycle, NSWindow/NSView, bitmap contexts, CoreText line/run APIs, event-loop invalidation, and release.
  - Use explicit ownership for handles returned retained or copied; use autorelease pools around frame/draw operations.
  - Keep cleanup arguments independent of owner state and use `runtime.KeepAlive` around native calls.

- [ ] **Step 4: Run ownership and build tests**

Run: `go test ./macoslite -run 'TestNativeOwner' -count=1`, `go test ./macoslite -count=1`, and `CGO_ENABLED=0 go test ./macoslite -count=1`.
Expected: PASS; no cgo dependency.

- [ ] **Step 5: Commit**

```bash
git add macoslite
git commit -m "feat(macoslite): add purego AppKit ownership bindings"
```

---

### Task 3: Window Lifecycle, Main-Thread Run Loop, and Bitmap Canvas

**Files:**
- Modify: `macoslite/native_darwin.go`
- Create: `macoslite/canvas_darwin.go`
- Create: `macoslite/lifecycle_darwin_test.go`

**Interfaces:**
- Implement `Run`, `Stop`, and `Invalidate` from Task 1.
- Internal seam `type nativeBackend interface { open(o Options) error; setNeedsDisplay(); dispatchMain(func()); present(img *image.RGBA); close() }`; the real backend is purego AppKit, tests use a recording fake.
- `(*App).drawFrame(w, h, scale float64)` builds the bitmap, calls `View.Draw` and `present`; it is the only caller of `View.Draw`.
- Implement Darwin `Canvas.Fill` and `Canvas.RoundRect` directly on the `image.RGBA` (source-over blend, non-premultiplied `Color` input); `Text` is a no-op until Task 4.
- Window view subclass is registered with `purego/objc.RegisterClass` and forwards `drawRect:`, mouse, key, resize and `windowWillClose:` to the App.

- [ ] **Step 1: Write failing lifecycle tests**
  - Test a fake native backend state machine: `New` creates one app/window/view; `Invalidate` coalesces; `Stop` is idempotent; close calls `View.Close` once.
  - `TestDrawFrameNoReentry`: a view that calls `Invalidate` inside `Draw` is drawn once per `drawFrame`, and exactly one further `setNeedsDisplay` is recorded.
  - `TestInvalidateFromGoroutine`: 100 concurrent `Invalidate` calls from worker goroutines go through `dispatchMain`, coalesce to ≤ 1 pending `setNeedsDisplay`, and pass `-race`.
  - `TestRoundRectFill`: a 10×10 red `RoundRect` with radius 3 colors the center pixel and leaves the corner pixel transparent.
  - Test resize rebuilds the bitmap at `ceil(logicalSize * scale)` dimensions and zero-size frames skip drawing.

- [ ] **Step 2: Run the tests to verify failure**

Run: `go test ./macoslite -run 'Test(Lite|Canvas|Lifecycle)' -count=1`.
Expected: FAIL because the lifecycle and canvas are not implemented.

- [ ] **Step 3: Implement the lifecycle and CPU bitmap path**
  - Create NSApplication/NSWindow/NSView on the main thread.
  - Use an NSView subclass callback for `drawRect:` and event methods.
  - Maintain one premultiplied RGBA bitmap sized to device pixels; recreate it only on size/scale change.
  - Coalesce invalidations and dispatch them through AppKit without recursive draws.
  - `Stop` removes callbacks, releases the window/view/application-owned objects, and is safe when called twice.

- [ ] **Step 4: Run lifecycle tests and an early memory floor gate**

Run: `go test ./macoslite -run 'Test(Lite|Canvas|Lifecycle)' -count=1` and `CGO_ENABLED=0 go test ./macoslite -count=1`.
Expected: PASS.

Then build a throwaway probe under `/tmp/fwbench/macoslite-floor` (not committed) that opens the 360×260 window and fills it with one rectangle, wait 7 s, and read the physical footprint with `footprint -p <pid>`.
Expected: below 31 MiB. If the floor is already ≥ 31 MiB, stop and report the footprint breakdown before Task 4 — the remaining tasks cannot reach the target.

- [ ] **Step 5: Commit**

```bash
git add macoslite
git commit -m "feat(macoslite): add native window lifecycle and bitmap canvas"
```

---

### Task 4: CoreText Short-Text Drawing

**Files:**
- Modify: `macoslite/canvas_darwin.go`
- Create: `macoslite/coretext_darwin.go`
- Create: `macoslite/coretext_darwin_test.go`
- Modify: `macoslite/api_other.go` only if the stub needs new method declarations

**Interfaces:**
- Implement `Canvas.Text(s string, origin Point, size float64, color Color)`; `origin` is the baseline-left point in logical coordinates.
- Internal CoreText helper returns a retained/owned CFAttributedString/CTLine only for the duration of a draw call.
- CTFonts are cached by size (system UI font, regular weight) with at most 8 entries, least-recently-used evicted and released.
- Implement `App.TrimCaches()`: releases every cached CTFont; the current bitmap is kept until the next frame.

- [ ] **Step 1: Write failing CoreText tests**
  - Assert Latin text draws non-empty pixels.
  - Assert Chinese text draws non-empty pixels when the system fallback supplies it.
  - Assert empty text and invalid/empty color do not panic.
  - Assert a glyph-run failure falls back to no-op without invalidating the frame.
  - Compare repeated draws for deterministic bitmap output.
  - Drawing at 9 distinct sizes leaves exactly 8 cached fonts and releases the evicted one once; `TrimCaches` releases all of them and a following draw still succeeds.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./macoslite -run 'TestCoreText' -count=1`.
Expected: FAIL because CoreText drawing is not implemented.

- [ ] **Step 3: Implement CoreText text drawing**
  - Build a CTLine from UTF-8 text and a CTFont attribute; let CoreText select fallback runs.
  - Draw into the current RGBA bitmap through a CGBitmapContext with premultiplied alpha and the requested color.
  - Release transient CoreText objects before returning; never store a CTLine or CFAttributedString in the view.
  - Keep this independent from `ui/text`, `paint`, and the existing Tango CoreText path.

- [ ] **Step 4: Run text tests and a screenshot smoke**

Run: `go test ./macoslite -run 'TestCoreText' -count=1` and build/run the counter example, then capture a screenshot and inspect that Latin and CJK text are visible.
Expected: PASS and a non-blank screenshot.

- [ ] **Step 5: Commit**

```bash
git add macoslite
git commit -m "feat(macoslite): draw short text with CoreText"
```

---

### Task 5: Counter Example, Footprint Benchmark, and Documentation

**Files:**
- Create: `examples/macoslite-counter/main.go`
- Create: `examples/macoslite-counter/README.md`
- Create: `tools/bench/macoslite-memory.sh`
- Modify: `docs/specs/macos-native-lite-design.md`
- Modify: `docs/dev-builds.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Example uses only `macoslite`, standard library, and purego; it must not import Tango root, `uihost`, `ui`, `paint`, or `text`.
- Benchmark records startup-to-first-draw, process physical footprint after 7 seconds, and binary size.

- [ ] **Step 1: Write failing example/build smoke**
  - Add a shell smoke that builds `examples/macoslite-counter` with `CGO_ENABLED=0 -trimpath -ldflags '-s -w'` and asserts the binary exists.
  - Add a Go-level counter state test proving `+`, `-`, reset and Escape-close callbacks update state.

- [ ] **Step 2: Run the smoke to verify failure**

Run: `CGO_ENABLED=0 go test ./examples/macoslite-counter -count=1` and the build command.
Expected: FAIL until the example and package are wired.

- [ ] **Step 3: Implement the counter example and benchmark**
  - Window content 360×260, title `Lite Counter`.
  - Draw a number and three buttons using direct `Canvas` calls; handle mouse hit rectangles and keyboard Escape.
  - Benchmark the same machine and process metric used for earlier Tango/NSGo comparisons: `proc_pid_rusage` physical footprint, not RSS.
  - Exit nonzero if the process is not unlocked/visible or if no first-draw marker arrives; never record a locked-screen result.

- [ ] **Step 4: Run the real app and measure**

Run:
```bash
CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /tmp/macoslite-counter ./examples/macoslite-counter
/tmp/macoslite-counter
```
Then run `tools/bench/macoslite-memory.sh` for three rounds. Inspect a screenshot and record the median footprint, startup, binary size and zero WebKit child processes.
Expected first-stage target: idle physical footprint below 31 MiB; if it is not met, report the measured fixed costs before expanding the API.

- [ ] **Step 5: Update docs and commit**

Record the measured result and known limitations in the design/README; document that this is an independent macOS host and not a replacement for `uihost.Open`.

```bash
git add examples/macoslite-counter tools/bench/macoslite-memory.sh docs/specs/macos-native-lite-design.md docs/dev-builds.md CHANGELOG.md
git commit -m "feat(macoslite): add native counter host and memory benchmark"
```

---

### Task 6: Final Verification and Integration Gate

**Files:**
- Modify: none unless verification exposes a defect

- [ ] **Step 1: Run package and full platform checks**

Run:
```bash
go test -p 2 ./macoslite ./examples/macoslite-counter ./uihost
CGO_ENABLED=0 go test ./macoslite ./examples/macoslite-counter
GOOS=linux GOARCH=amd64 go vet ./macoslite
GOOS=windows GOARCH=amd64 go vet ./macoslite
```
Expected: PASS; non-Darwin stub compiles.

- [ ] **Step 2: Run native ownership race checks**

Run: `go test -race ./macoslite -count=1`.
Expected: PASS on macOS.

- [ ] **Step 3: Re-run the memory benchmark after the final build**

Run: `tools/bench/macoslite-memory.sh --rounds 3` with the screen unlocked and no competing benchmark processes. Record the actual median; do not claim the 31 MiB target unless the result proves it.

- [ ] **Step 4: Review the diff and integration scope**

Run: `git diff --check`, inspect imports to confirm no lite package imports forbidden Tango/UI packages, and compare the counter screenshot against the expected title, number, buttons and CJK text.

- [ ] **Step 5: Commit/push only after the user’s integration decision**

Keep the native-lite work isolated until the memory target and review are accepted; do not change `uihost.Open` defaults or merge it into the default rendering path as part of this plan.
