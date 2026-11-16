# Resident Memory and LogView CPU Optimization Plan

> **For agentic workers:** Execute the two independently scoped fixes, with systematic debugging, regression tests before production changes, and fresh verification before completion.

**Goal:** Reduce proven unused renderer resources and the CPU cost of LogView frames without changing public APIs, default rendering modes or visible output.

**Architecture:** Keep the current GPU and CPU renderers. Allocate gradient resources only when used, reclaim patch resources after a bounded period without patches, and avoid repeated rounded-rectangle coverage calculations in CPU clip interiors.

**Tech Stack:** Go 1.27.3, existing TanGo UI benchmarks, Go pprof, native macOS process footprint sampling.

**Spec:** The user requested resident-memory and LogView CPU optimization following the performance review. Baseline is commit `1d76834`; measurement artifacts belong under `/Users/admin/min/outputs/tango-memory-log-opt-20261116/`.

## Global Constraints

- Preserve public APIs, default GPU selection, text, interaction and pixel coverage.
- Do not add dependencies or force garbage collection in frame processing.
- Keep caches bounded and resources reproducible after eviction.
- Preserve unrelated work in the main checkout.
- Do not run heavy CPU benchmarks during native before/after memory collection.

## Review Focus

- Rounded clips must preserve fractional edges, asymmetric/clamped radii, clipping offsets and nested-mask multiplication.
- Fully opaque clip intersections must preserve the existing parent mask.
- Delayed gradient allocation must survive first use, trim and future reuse.
- Large patch pages must be released only after they are no longer needed and must rebuild correctly on later use.
- Headless CPU benchmarks do not establish native GPU FPS; Go heap and process physical footprint must be reported separately.

### Task 1: Reduce unnecessary renderer resource residency

**Files:** `internal/render/gpu/` resource/cache code and focused tests, with `uihost/` changes only if demonstrated necessary.

- [x] Reproduce eager unused-gradient allocation and retained oversized patch pages with failing tests.
- [x] Defer unused gradient resources; reclaim patch pages after bounded patch-free rendered frames, preserving later reconstruction.
- [x] Run resource tests, race checks and sequential native before/after samples with fixed scenes; record measured benefits and fixed driver-cost limits.

### Task 2: Reduce CPU rounded clipping cost in LogView

**Files:** `ui/paint/cpu.go`, `ui/paint/cpu_clip_mask_test.go`; existing `ui/logview_test.go` benchmark remains unchanged.

- [x] Record LogView baseline and CPU profile. Current profile puts approximately 45% of samples in rounded-clip mask generation.
- [x] Add an allocation regression for clips wholly inside an opaque rounded-rectangle region and pixel comparisons against the scalar coverage definition.
- [x] Compute coverage once across non-corner row bands, copy/fill constant coverage, and preserve parent-mask modulation.
- [x] Run paint and LogView interaction tests, race checks, and alternating baseline/optimized benchmark rounds. Require unchanged pixels and a measured LogView CPU reduction.

### Integration and delivery

- [x] Review both changes together and run repository-required checks appropriate to renderer/UI changes.
- [x] Obtain an independent code review and address material findings.
- [x] Save before/after data and a concise performance report, commit only task files, and integrate/push using the user's existing instruction to push completed work.

## Completion evidence

Production root suites (167 packages plus the two gallery packages in CI short mode), development root suites, CLI module, focused race regressions, and all three vet checks passed on the integrated tree. Independent review found no material correctness defects. The platform-stub formatting adjustment after integration is whitespace only. Raw results: `/Users/admin/min/outputs/tango-memory-log-opt-20261116/verification.json`.
