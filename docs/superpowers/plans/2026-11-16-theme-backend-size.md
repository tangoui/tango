# Theme and Darwin Size Profiles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a behavior-preserving theme-core optimization and explicit compile-time `all`/`webview`/`canvas` profiles for Darwin, with CLI validation and measurable installed-size reduction.

**Architecture:** First split temporary theme reconstruction from full public theme construction so aliases do not recreate icons/components. Then add a normalized profile model to `cmd/tan`, pass profile tags through release builds, and split Darwin backend source/dependencies at the profile boundary. The default `all` profile remains unchanged; Windows/Linux reject non-`all` before compilation in this phase.

**Tech Stack:** Go 1.27, build tags, existing `cmd/tan`, Darwin purego/AppKit/WKWebView, existing `platform.Backend` and `corehook` contracts.

**Spec:** `docs/specs/2026-11-16-theme-backend-size-design.md`

## Global Constraints

- Existing `tango.New`, `uihost.Open`, public theme constructors, and default behavior remain source-compatible.
- No `NewWithBackend` API in this phase.
- Omitted `features` means `all`; explicit `features: ["webview", "canvas"]` means mixed.
- CLI syntax is one `-features webview,canvas` value; repeated flags are rejected.
- `webview`/`canvas` profiles are implemented only for Darwin in this phase; Windows/Linux non-`all` fail before compilation.
- Invalid/null/empty/unknown/duplicate feature values fail clearly; conflicting Tango backend tags in user `GOFLAGS` fail.
- No mobile profile changes, UPX, capability deletion, or `v0.1.0` tag changes.
- Ant Design 6.7 + Pro v6, Element Plus 2.14.8, shadcn/ui v4 remain the design baselines.

## Review Focus

- Alias/primary reconstruction must preserve every token, icon map, component default, dark/compact mode and shadcn base/radius; `TestThemeCoreParity` owns this.
- The first WebView/canvas use must fail deterministically with `ErrUnsupported` when excluded, not panic or create a blank window; profile boundary tests own this.
- `all` must preserve mixed WebView plus canvas attachment and existing platform smoke; Darwin all-profile smoke owns this.
- Windows/Linux non-all validation must happen before Go compilation and must not produce a partial artifact; CLI profile tests own this.
- User `GOFLAGS` cannot silently override the selected backend graph; tag-normalization tests own this.

---

### Task 1: Theme core reconstruction

**Files:**
- Modify: `ui/theme/theme.go` (`AntDesign`, `ElementPlus` construction flow)
- Modify: `ui/theme/shadcn.go` (`Shadcn` construction flow)
- Modify: `ui/theme/components.go` (`stockTheme`, internal constructor helpers)
- Test: `ui/theme/theme_core_test.go`

**Interfaces:**
- Consumes: existing public constructors and `Theme.adoptPrimary`.
- Produces: unexported core constructors used only by temporary stock themes; public constructors still return fully populated `Theme` values.

- [ ] **Step 1: Write failing parity tests** named `TestThemeCoreParity`, `TestThemeAliasParity`, and `TestThemeCoreDoesNotInstallIcons` that compare public output, alias/primary reconstruction, component defaults, icons, dark/compact options, and shadcn base/radius against independently constructed expected themes.
- [ ] **Step 2: Run `GOCACHE=/tmp/p4-gocache go test ./ui/theme -run 'TestThemeCoreParity|TestThemeAliasParity|TestThemeCoreDoesNotInstallIcons'` and confirm the new tests fail because the core path does not exist.
- [ ] **Step 3: Implement unexported `antDesignCore`, `elementPlusCore`, and `shadcnCore` construction paths with an explicit `full bool`/equivalent internal choice; public constructors use full mode, while `stockTheme` uses core mode and never creates icon maps or component derivation.
- [ ] **Step 4: Run the focused tests and then `GOCACHE=/tmp/p4-gocache go test ./ui/theme ./ui`.
- [ ] **Step 5: Measure minimal/default self-drawn samples before/after; retain the refactor even if byte savings are zero only if the parity and construction-work benchmark improves, and document the measured result.
- [ ] **Step 6: Commit `perf(theme): avoid full reconstruction during aliases`.

### Task 2: Profile model and `tan` CLI/config

**Files:**
- Modify: `cmd/tan/config.go`, `cmd/tan/build.go`, `cmd/tan/package.go`, `cmd/tan/build_tags.go`, `cmd/tan/tan.schema.json`
- Test: `cmd/tan/profile_test.go`, `cmd/tan/cli_golden_test.go` if help changes
- Modify: `docs/release.md`, `docs/specs/2026-11-16-theme-backend-size-design.md` if implementation details need recording

**Interfaces:**
- Produces: unexported normalized `buildFeatures` value and `parseFeatures(string) (buildFeatures, error)`; `Config.Features []string` serialized as `features`; `buildOptions.features` passed from both build and package.
- Consumes: existing `LoadConfig`, `goBuildArgs`, `buildTags`, `parseTargets`.

- [ ] **Step 1: Write failing tests for omitted features → all, explicit array normalization, explicit mixed profile, null/empty/unknown/duplicate rejection, CLI `-features webview,canvas`, repeated flag rejection, GOFLAGS backend-tag conflict, and Windows/Linux non-all rejection before build.
- [ ] **Step 2: Run `GOWORK=off GOCACHE=/tmp/p4-gocache go test ./cmd/tan -run 'Test.*Feature|Test.*Profile'` and confirm expected missing parser/field failures.
- [ ] **Step 3: Add `Config.Features []string` and generated schema metadata; implement normalization and validation; add one-value `-features` flags to build/package and pass normalized features through `buildOptions` into `goBuildArgs`.
- [ ] **Step 4: Implement deterministic profile tags: `all` has no backend profile tag; Darwin `webview` gets `tango_backend_webview`, `canvas` gets `tango_backend_canvas`, mixed gets both; reject conflicting Tango backend tags in GOFLAGS; reject Darwin-unsupported profiles on Windows/Linux before `go build`.
- [ ] **Step 5: Update CLI help/golden and docs; run focused tests, standalone `GOWORK=off go build`, `go vet`, and `apitxt -check`.
- [ ] **Step 6: Commit `feat(tan): add compile-time backend feature profiles`.

### Task 3: Darwin profile boundary

**Files:**
- Modify/split: `internal/platform/darwin/backend.go`, `surface.go`, `surface_ext.go`, `webview_ext.go`, `scheme.go`, `webembed.go`, `canvas*.go`, `objc.go`, `internal/platform/native/backend_darwin.go`
- Modify: `hooks.go`, `corehook/*.go` only where the existing internal contract needs profile-safe stubs
- Test: Darwin profile build tests and focused backend tests under `internal/platform/darwin`

**Interfaces:**
- Consumes: build tags produced by Task 2; unchanged `platform.Backend`, `platform.Surface`, `platform.CanvasCreator`, and `corehook` contracts.
- Produces: `all`, `webview`, `canvas` Darwin build graphs; excluded-mode operations return errors wrapping `tango.ErrUnsupported` at root boundaries.

- [ ] **Step 1: Add failing compile/profile tests proving the intended three tag sets select distinct implementations and that canvas-only WebView calls / webview-only canvas calls return unsupported.
- [ ] **Step 2: Run Darwin tagged package builds/tests and confirm failures identify the current common files that still require the excluded mode.
- [ ] **Step 3: Split common window/event-loop code from WebView scheme/delegate code and canvas/GPU/overlay code using `tango_backend_webview` / `tango_backend_canvas` tags; keep an all-profile composition file that preserves current mixed behavior.
- [ ] **Step 4: Add profile-specific stubs for mandatory platform interfaces and core hooks; make unsupported errors deterministic and preserve `errors.Is(tango.ErrUnsupported)` behavior.
- [ ] **Step 5: Run Darwin `go test` for all, webview, canvas tags; run existing `internal/platform/darwin` and `uihost` tests; run real macOS WebView-only, canvas-only and mixed smoke/automation checks.
- [ ] **Step 6: Build three minimal Darwin samples and inspect `go tool nm`/raw bytes to prove the excluded mode is not statically reachable; reject the split if it does not reduce the relevant profile.
- [ ] **Step 7: Commit `feat(darwin): split webview and canvas backend profiles`.

### Task 4: Cross-platform validation, measurements and docs

**Files:**
- Modify: `scripts/size-report.sh`, `docs/package-size-optimization.md`, `docs/package-size-optimization-data.json`, `CHANGELOG.md`, `docs/release.md`
- Test/fixtures: profile sample sources and package/archive verification scripts as needed

**Interfaces:**
- Consumes: normalized features and Darwin profiles from Tasks 2–3.
- Produces: reproducible profile matrix and explicit platform rejection report.

- [ ] **Step 1: Write failing measurement/validation checks for three Darwin profiles, all-profile Windows/Linux samples, non-all platform rejection, exact archive payloads and artifact naming.
- [ ] **Step 2: Run the checks before implementation wiring and confirm the profile matrix is not yet available.
- [ ] **Step 3: Update the report to measure minimal WebView/canvas/mixed on Darwin arm64 plus all-profile Linux amd64 and Windows amd64; record raw executable, gzip, actual ULMO dmg, NSIS and Linux tar payload bytes.
- [ ] **Step 4: Run `go test ./...`/project CI-equivalent, API snapshots, CLI tests, Darwin smoke, `node site/tools/checklinks.mjs`, package verification and the size script.
- [ ] **Step 5: Record any profile with no measured reduction as unsupported for advertisement; document that Windows/Linux non-all is intentionally rejected in this phase.
- [ ] **Step 6: Commit `docs(size): report backend profile matrix`.

## Final verification

Run from the final branch:

```sh
GOCACHE=/tmp/p4-gocache go run ./internal/cmd/apitxt -check
TANGO_SMOKE_TIMEOUT_SCALE=3 GOCACHE=/tmp/p4-gocache go test -short -p 1 -timeout 40m ./...
GOWORK=off GOCACHE=/tmp/p4-gocache go test ./cmd/tan/...
node site/tools/checklinks.mjs
```

Then perform the fresh whole-branch code review against the design document before deciding whether to merge or push.
