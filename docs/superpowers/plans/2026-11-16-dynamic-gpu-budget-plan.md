# 动态 GPU 资源预算化实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans (execute inline). Steps use checkbox syntax.

**Goal:** Reduce Tango default 800×600 dynamic list/grid GPU memory below the matched mygo baseline without worsening P99 update intervals by more than 10%.

**Architecture:** First add internal diagnostics and fake-device counters to identify atlas, image-pool, render-target and heap residency. Then add bounded, frame-boundary-only eviction for rebuildable GPU resources, keeping current-frame resources pinned and preserving device-loss/scale-change recovery. No public API is added unless an internal control cannot be tested or safely configured otherwise; if one is required, expose the smallest experimental API and update `api/go`/`api/tiers.txt`.

**Tech Stack:** Go 1.27; Tango GPU renderer; Metal/CoreGraphics on macOS; existing atlas and paint renderer abstractions; `proc_pid_rusage` paired benchmark.

**Spec:** `docs/superpowers/specs/2026-11-16-dynamic-gpu-budget-design.md`

## Global Constraints

- Do not change default renderer selection in `uihost/gpu.go`.
- Do not use forced GC, `FreeOSMemory`, heap profiles, or outlier deletion in comparison measurements.
- Do not evict resources referenced by the current frame, command buffer, or present target.
- Evict only at frame boundaries; release atlas/index references before native textures.
- Device loss, close, scale changes and Trim must redraw from an empty rebuildable-resource state.
- Keep a single rollback switch; disabled behavior must match current main.
- Dynamic list/grid must use the shared words/cell-value data and 800×600 content size.
- Existing main and non-Darwin builds must remain compatible.
- Commits use author `tango <903300326@qq.com>` without Claude trailer.

## Review Focus

- **In-flight resource eviction:** a texture used by the current command buffer must survive until completion; fake-device tests pin this.
- **Atlas re-upload correctness:** an evicted glyph/path must render identically after re-upload; pixel tests pin this.
- **Scale/device reset:** a 1x→2x change must invalidate stale budgets and rebuild resources without stale handles.
- **Budget thrashing:** a list/grid that repeatedly crosses the budget must not evict and re-upload every frame; upload/eviction counters and P99 checks pin this.
- **CPU fallback parity:** `TANGO_RENDERER=cpu` and GPU-unavailable paths must not receive GPU-only state or fail to draw.

---

### Task 1: Baseline Diagnostics and Resource Counters

**Files:**
- Modify: `internal/render/gpu/renderer.go`
- Modify: `internal/render/gpu/atlas/atlas.go`
- Modify: `internal/render/gpu/metal/image_pool_darwin.go`
- Create: `internal/render/gpu/resource_stats.go`
- Create: `internal/render/gpu/resource_stats_test.go`

**Interfaces:**
- Produce internal `ResourceStats` with atlas page/byte counts, image-pool bytes/count, render-target bytes, uploads, evictions and reuploads.
- Produce a frame-boundary stats snapshot method used only by tests/diagnostics; no public package API.

- [ ] **Step 1: Write failing counter tests**
  - Fake atlas/device allocates pages and uploads; assert stats count bytes and uploads.
  - Fake image pool puts/takes/releases textures; assert current bytes and count.
  - Assert stats reset only at an explicit frame boundary and never mutate while a frame is in flight.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/render/gpu/... -run 'Test(ResourceStats|AtlasStats|ImagePoolStats)' -count=1`.
Expected: FAIL because counters do not exist.

- [ ] **Step 3: Implement internal counters**
  - Add counters at existing allocation/upload/release points.
  - Keep accounting byte-based and saturating at zero on release.
  - Exclude shared WindowServer/IOSurface pages that Tango does not own.

- [ ] **Step 4: Verify**

Run the focused tests, `go test -race ./internal/render/gpu/...`, and `GOOS=linux GOARCH=amd64 go test ./internal/render/gpu/...`.
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/render/gpu
git commit -m "perf(gpu): add resource residency diagnostics"
```

---

### Task 2: Atlas Frame-Boundary Eviction

**Files:**
- Modify: `internal/render/gpu/atlas/atlas.go`
- Modify: `internal/render/gpu/renderer.go`
- Create: `internal/render/gpu/atlas/eviction_test.go`
- Create: `internal/render/gpu/atlas/pixel_eviction_test.go`

**Interfaces:**
- Produce internal `AtlasBudget{MaxPages int; IdleFrames uint32}`.
- Produce `Atlas.SetBudget(AtlasBudget)`, `Atlas.BeginFrame(frame uint64)`, `Atlas.EndFrame()`, and `Atlas.Evict(frame uint64) int`.
- Eviction must return only entries not referenced by the current frame; renderer calls it after submission preparation and before the next frame begins.

- [ ] **Step 1: Write failing eviction tests**
  - Fill more pages than budget; mark current-frame entries; assert only old unpinned entries are evicted.
  - Re-request an evicted glyph/path and assert it uploads again and pixel output equals the non-evicted output.
  - Assert a budget of zero is rejected in production path and accepted only by test constructor if needed.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/render/gpu/atlas -run 'Test(Evict|Pixel)' -count=1`.
Expected: FAIL because budget and eviction APIs do not exist.

- [ ] **Step 3: Implement LRU/idle eviction**
  - Track last-used frame and current-frame pins on glyph/path entries.
  - Evict whole pages only when all entries on the page are unpinned and older than `IdleFrames`; otherwise leave the page resident.
  - Invalidate lookup entries before freeing page storage.
  - Keep default budget disabled until Task 4 wires the renderer policy.

- [ ] **Step 4: Verify**

Run focused atlas tests, GPU renderer pixel tests, race tests and `go test ./ui/paint ./ui/text`.
Expected: PASS with unchanged pixels.

- [ ] **Step 5: Commit**

```bash
git add internal/render/gpu/atlas internal/render/gpu/renderer.go
git commit -m "perf(gpu): evict idle atlas pages at frame boundaries"
```

---

### Task 3: Metal Image-Pool and Render-Target Budget

**Files:**
- Modify: `internal/render/gpu/metal/image_pool_darwin.go`
- Modify: `internal/render/gpu/metal/metal_darwin.go`
- Modify: `internal/render/gpu/renderer.go`
- Create: `internal/render/gpu/metal/image_pool_budget_test.go`
- Create/modify: platform-neutral fake pool tests as needed

**Interfaces:**
- Produce internal `ImageBudget{MaxBytes int64; IdleFrames uint32}`.
- Add frame-boundary `BeginFrame`, `EndFrame`, and `EvictIdle` hooks to the image pool/device abstraction; non-Darwin implementations compile and no-op safely.
- Renderer stats report image-pool bytes/count and render-target bytes.

- [ ] **Step 1: Write failing image-pool tests**
  - Add textures until over budget; assert only idle, unreferenced textures are released.
  - Assert a texture held by the active frame is not released even when over budget.
  - Assert scale/device reset clears pool accounting and later allocation works.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/render/gpu/metal -run 'Test(ImagePool|Budget|Scale)' -count=1`.
Expected: FAIL because budget hooks do not exist.

- [ ] **Step 3: Implement bounded pool eviction**
  - Track texture byte size, last-used frame and active references.
  - Release only at frame boundary and only after command-buffer safety conditions.
  - Integrate existing `TrimImages` with the same release path.
  - Keep render target and drawable lifetime separate from idle image-pool textures.

- [ ] **Step 4: Verify**

Run Darwin GPU package tests, race tests and cross-platform GPU package compilation.
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/render/gpu/metal internal/render/gpu
 git commit -m "perf(gpu): budget idle Metal image resources"
```

---

### Task 4: Renderer Policy and Rollback Switch

**Files:**
- Modify: `internal/render/gpu/renderer.go`
- Modify: `uihost/gpu.go` only for internal policy wiring, not renderer choice
- Modify: `docs/dev-builds.md`
- Create: `internal/render/gpu/budget_policy_test.go`

**Interfaces:**
- Produce internal `BudgetPolicy` derived from logical area, scale, observed upload rate and frame cadence.
- Add one rollback environment switch, e.g. `TANGO_GPU_BUDGET=off`; default is the new budget policy, `off` restores pre-budget residency.
- No public API unless an implementation blocker is proven; if added, classify experimental and update API golden files.

- [ ] **Step 1: Write failing policy tests**
  - Large 800×600 list/grid receives positive atlas/image budgets.
  - Small utility windows receive smaller budgets without changing renderer choice.
  - `TANGO_GPU_BUDGET=off` disables all eviction while preserving output.
  - High upload churn raises the budget within a bounded ceiling and then stabilizes.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./internal/render/gpu ./uihost -run 'Test(BudgetPolicy|RendererBudget)' -count=1`.
Expected: FAIL because policy and switch do not exist.

- [ ] **Step 3: Implement policy**
  - Configure defaults from window area/scale with hard byte/page ceilings.
  - Apply budgets only after renderer initialization and at frame boundaries.
  - Avoid scene-name checks and benchmark-only branches.
  - Document the rollback switch and diagnostic meaning.

- [ ] **Step 4: Verify**

Run focused tests, `go test ./uihost ./internal/render/gpu/...`, race tests and `go run ./internal/cmd/apitxt -check`.
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/render/gpu uihost/gpu.go docs/dev-builds.md
 git commit -m "perf(gpu): apply bounded resource budgets"
```

---

### Task 5: Dynamic List/Grid Matched Benchmark

**Files:**
- Modify: none in production code unless a verified defect appears
- Create: `outputs/...` only outside the repository, not committed

**Interfaces:**
- Use the existing paired runner with the shared list/grid data and exact `Page.ProcessID()` WebContent attribution.
- Record Tango renderer, physical footprint, CPU, frame interval diagnostics, resource counters, lock/visibility state and all failed attempts.

- [ ] **Step 1: Build both frameworks**

Use Go 1.27.3, `CGO_ENABLED=0`, `-trimpath -buildvcs=false -ldflags='-s -w'`, Tango current main and mygo v0.9.0; verify binary provenance and scene fixtures.

- [ ] **Step 2: Run valid paired list/grid trials**

Run 5 AB/BA pairs per scene, 800×600, 10 one-second samples after warmup, plus 65-second runs. Reject only locked/occluded/uncounted-WebContent attempts; preserve every rejected attempt separately.

- [ ] **Step 3: Apply target gate**

Require per scene: Tango median < mygo median, Tango maximum valid sample < mygo minimum valid sample, no sustained 65-second increase, and P99 interval no worse than 10% from baseline.

- [ ] **Step 4: Write measured report**

Record actual values, renderer mode, resource stats and limitations. If target misses, report the dominant resource category and stop rather than broadening scope silently.

---

### Task 6: Final Verification and Integration Gate

**Files:**
- Modify: none unless a verified defect appears

- [ ] **Step 1: Run suites**

Run `go test -p 2 ./internal/render/gpu/... ./uihost ./ui/paint ./ui/text`, `go test -race` on GPU/atlas/pool packages, Linux/Windows compile checks, and `go run ./internal/cmd/apitxt -check`.

- [ ] **Step 2: Run dynamic benchmark after final build**

Repeat Task 5 with screen unlocked, no competing GUI capture, all samples retained, and current main provenance.

- [ ] **Step 3: Review diff and rollback behavior**

Check `git diff --check`, verify `TANGO_GPU_BUDGET=off` reproduces pre-budget resource residency/output, and ensure no public API was added unnecessarily.

- [ ] **Step 4: Commit/push/merge only after user asks**

Keep implementation isolated until the target gate and review pass; do not change WebView or default CPU/native selection as a fallback in this plan.
