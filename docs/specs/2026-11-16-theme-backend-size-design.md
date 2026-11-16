# Theme and Backend Size Profiles Design

**Date:** 2026-11-16
**Status:** Approved scope after self-check (2026-11-16)
**Base:** `b7a963c` (`origin/main`)

## Intent

Continue reducing Tango application installed and download size after the first compatible optimization round. The next target is the code that is currently pulled into self-drawn applications by theme reconstruction and the code pulled into desktop applications by a platform backend that supports both WebView and self-drawn windows.

The design deliberately separates two kinds of optimization:

1. An internal theme-construction refactor that preserves all public APIs and behavior.
2. Explicit compile-time desktop backend profiles, selected by `tan`, that can omit an entire unused UI mode. Existing projects remain fully compatible when no profile is selected.

## Goals

- Preserve the behavior of `theme.AntDesign`, `theme.ElementPlus`, `theme.Shadcn`, aliases, dynamic primary colors, high contrast, and system theme following.
- Reduce the static reachability of icon maps and component derivation during internal theme color reconstruction.
- Let a WebView-only macOS build omit self-drawn backend code and let a canvas-only macOS build omit WebView backend code.
- Keep a mixed `webview,canvas` macOS build equivalent to today's default behavior.
- On Windows and Linux, keep `all` fully supported and reject non-`all` profiles before compilation until their backend boundaries are separately implemented.
- Make profile selection reproducible from `tan.json` and overridable from `tan build` / `tan package`.
- Produce a three-profile macOS size matrix (WebView, canvas, mixed) and an all-profile cross-platform baseline.
- Fail clearly when an application asks for a mode excluded from its build profile.

## Non-goals

- No UPX, executable packer, or proprietary compression.
- No removal of any existing capability from the default profile.
- No removal of the WebView engine from platform packages; system WebView dependencies remain external.
- No mobile profile change in this project; iOS and Android keep their existing build paths.
- No removal of `bind.Register`, service reflection, editor languages, fonts, or accessibility behavior.
- No promise that every format or every template becomes smaller; every result is reported by sample and artifact.

## Constraints

- Existing `tango.New(opts)` and `uihost.Open(...)` source compatibility remains for the default profile.
- No `NewWithBackend` API is introduced in this phase; build tags and the existing `native.NewBackend()` selection remain the backend boundary.
- A missing profile in existing `tan.json` means `all` (the current behavior).
- `webview,canvas` means mixed support and is the compatibility-equivalent profile.
- A feature profile is a build-time choice; a runtime `Options` field alone is insufficient because it cannot remove code from a Go binary.
- On Windows and Linux, `webview` and `canvas` are rejected by CLI validation in this phase; only `all` is supported there.
- The current design baseline remains Ant Design 6.7 + Pro v6, Element Plus 2.14.8, and shadcn/ui v4.
- A profile that excludes a mode must return the existing `tango.ErrUnsupported`-compatible error at the public boundary, not panic or silently create a blank window.
- API changes are allowed when required, but the default profile must not require an application migration. New profile APIs are experimental until a later stability review.
- v0.1.0 remains immutable; this work targets the next unreleased version and must be documented in `CHANGELOG.md`.

## Approaches considered

### A. Internal-only lazy loading

Split theme constructors and platform registration internally, but keep one all-capability backend. This is lowest risk and is the right first step for themes, but cannot remove a backend mode from a binary when the platform package's static call graph still includes both modes.

### B. Explicit compile-time profiles (selected)

Add a `features` configuration to `tan.json` and a `-features` override to `tan build` / `tan package`. The exact CLI spelling is `-features webview,canvas`; it is a single comma-separated value and cannot be repeated. `tan package` passes the normalized value to its internal build. The Go build receives stable profile tags and platform source files select the smallest graph. The default is `all`, so existing applications keep their behavior. This is the selected approach because installed-size reduction requires a static dependency boundary; a runtime feature switch would only change behavior, not link reachability.

The profile owns its build tags. User `GOFLAGS` tags are preserved unless they name a Tango backend profile tag; conflicting or duplicate profile tags fail with a clear error instead of silently producing a different graph.

### C. Delete automatic backend selection and require a new backend constructor

Make every application import and pass a specific backend. This could produce the smallest graph, but would force all existing applications to migrate and makes mixed windows more cumbersome. It is rejected for the first implementation; explicit profiles provide the same build-time boundary while retaining `tango.New` in the default profile.

## Architecture

### 1. Theme core construction

Keep the public constructors as full constructors:

```go
theme.AntDesign(opts ...theme.Option) *theme.Theme
theme.ElementPlus(opts ...theme.Option) *theme.Theme
theme.Shadcn(args ...theme.ShadcnArg) *theme.Theme
```

Factor each into an internal core constructor that can omit `Icons` and `Components` while constructing a temporary stock theme used only by `Theme.adoptPrimary`. The public constructor continues to create the complete icon map and derived component tokens. `stockTheme` uses the core path because it compares color fields and does not need either public payload. The tests compare all token colors, alias behavior, icon availability, component defaults, dark mode, compact mode, and shadcn base/radius before and after.

This is an internal call-graph optimization, not a new public API.

### 2. Profile model and CLI

Add a configuration field with a stable serialized spelling:

```json
{
  "features": ["webview", "canvas"]
}
```

Accepted values are `webview` and `canvas`; duplicates are rejected, an empty list is rejected, and omitted means `all`. The normalized profile is one of `all`, `webview`, `canvas`, or `webview,canvas`. `tan build -features` and `tan package -features` override the file for that invocation; `all` is accepted explicitly. Invalid combinations fail before compilation and explain the allowed values.

The CLI passes profile build tags through `buildTags` while preserving user `GOFLAGS` tags:

- `all`: current build graph and compatibility behavior.
- `webview`: `tango_backend_webview`.
- `canvas`: `tango_backend_canvas`.
- `webview,canvas`: `tango_backend_webview,tango_backend_canvas`.

The default build remains `all` when no profile is configured.

### 3. Backend capability boundary

Extend the internal backend capability description so the root can distinguish a backend that cannot host WebView surfaces from one that cannot host canvas surfaces. Keep the existing public surface methods and return `tango.ErrUnsupported` at the existing operation boundary.

Desktop platform implementations are split by build tags at the static dependency boundary:

- Common event-loop, surface identity, lifecycle and window primitives stay in the platform backend core.
- WebView scheme, navigation, page policy, downloads, IPC WebView bridge and WebView-specific delegate methods are behind the WebView profile.
- Canvas/GPU/overlay attachment, Canvas callbacks and self-drawn presentation are behind the canvas profile.
- The all profile composes both paths and preserves the current mixed-window behavior.
- Profile-specific stubs implement the same internal interfaces and return unsupported errors where a mode is absent, allowing `tango.New` and `uihost.Open` to give deterministic diagnostics.

The implementation is scoped to Darwin in this phase because it is the measured primary platform and has the clearest common delegate coupling. Windows and Linux retain `all` only; `tan build -features webview` or `canvas` fails during target/profile validation with an explicit "profile not implemented on <platform>" error. This is intentional, not a partial binary. A later phase can split those backends after the Darwin boundary and size benefit are proven.

Self-check result: the first Darwin profile implementation provides deterministic capability errors, but the minimal probe matrix showed that Go's linker already removes almost all unused mode code. The `webview` raw delta was only 16–48 bytes and canvas/mixed were unchanged. Therefore this phase does not advertise profile-based size reduction and does not perform a high-risk full-file Darwin split; the profile boundary remains an experimental foundation for a later measured split.

### 4. Measurement and release behavior

`tan.json` profile selection affects `tan build` and `tan package`; package metadata and artifacts include the selected profile for non-`all` outputs so they cannot overwrite each other. Existing artifact names remain unchanged for `all`.

The size report adds minimal WebView, minimal canvas and minimal mixed samples for Darwin arm64, plus the all-profile cross-platform baseline for Linux amd64 and Windows amd64. It records:

- raw executable bytes,
- gzip-9 estimate,
- actual macOS ULMO dmg where available,
- actual Windows NSIS setup where available,
- Linux tar.gz bytes and uncompressed payload bytes,
- capability smoke results for each profile.

A profile is accepted only if its capability smoke passes and its Darwin target's raw executable or actual download decreases. Windows/Linux non-`all` profiles are not in this phase's size acceptance matrix because CLI validation rejects them. A profile with no measurable benefit is documented and not advertised as an optimization.

## Compatibility and failure behavior

- Existing project without `features`: unchanged all-capability build.
- Existing project explicitly using mixed WebView + canvas: set `features` to both; no API migration.
- WebView-only application calling `uihost.Open`: returns unsupported with a message naming `canvas` and the profile.
- Canvas-only application creating a surface with a URL: returns unsupported with a message naming `webview` and the profile.
- On Windows/Linux, any non-`all` profile fails before compilation with the platform/profile validation error.
- Calling APIs unrelated to the missing mode remains valid.
- The profile is compile-time and cannot be changed after the binary is built.
- `features: null`, `features: []`, unknown values, repeated values, and repeated `-features` flags are errors; explicit `features: ["webview", "canvas"]` is the mixed profile.

## Verification plan

1. Theme token and icon/component parity tests; size comparison for an alias-heavy self-drawn sample.
2. CLI config, override, normalization, invalid values, platform rejection, and generated Schema tests.
3. Darwin profile matrix compile tests for all/default/webview/canvas; Windows/Linux tests that non-`all` profiles fail validation before compilation.
4. Runtime boundary tests for unsupported WebView/canvas operations in Darwin profiles.
5. Real macOS WebView-only, canvas-only, and mixed smoke applications; existing automation and accessibility smoke must remain green in `all`.
6. API snapshot, root short-mode tests, CLI tests, docs links, package archive verification, and exact before/after size report.

## Rollout

- Implement and measure in this isolated branch.
- Do not change `v0.1.0` or publish during implementation.
- If the profile boundary is too invasive for one release, land the theme core optimization and CLI/profile schema behind an experimental tag only after the profile matrix is complete; do not ship a profile that compiles but silently loses a capability.
