# Editor Metal Texture Reuse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement inline.

**Goal:** Reduce repeated native image texture creation/release without added cross-frame residency or pixel changes.
**Architecture:** Bounded frame-local exact-size texture handoff after previous submissions complete. Maintain existing logical IDs and full uploads.
**Tech Stack:** Go, purego Metal, native retain/release, offscreen GPU pixel and editor benchmarks.
**Spec:** docs/superpowers/specs/2026-11-16-editor-metal-reuse-design.md

## Global Constraints
- Preserve pixels, functionality, immutable image contracts, error behavior and all production budgets.
- Only after Device.wait; RGBA8 ShaderRead resources of exactly equal size; maximum32 candidates and8MiB during one applyImages.
- Release unused resources on success/error, clear size metadata on frees/Close; no cross-frame GPU pool.

## Review Focus
- Previous in-flight rendering is finished before physical textures change owner.
- Several same-size live images never share a texture; duplicate frees never double-release.
- Unmatched/oversized/error candidates are released; same-ID reupload keeps valid ownership.
- Full overwrite respects row stride, pixel alpha and replacement dimensions.
- No change to partial rendering, late GPU failures, device loss, CPU/GL/D3D backends or cache budgets.

### Task 1: Frame-local reuse
- [x] Write native object identity and pixel regression tests for reuse, distinct/live images, mismatch, same-ID, error/close; watch reuse test fail against baseline.
- [x] Add native texture-churn benchmark and compile baseline editor/Metal binaries before implementation.
- [x] Move applyImages into focused images_darwin.go, add live-ID size metadata and bounded local candidates; release leftovers on all returns.
- [x] Run focused and full Metal conformance/damage/golden tests; expect PASS and reused native texture identity.

### Task 2: Retire discarded minimap images
- [x] Add integration tests proving edited/disabled minimap strips leave no obsolete GPU image IDs and host GPU/CPU/oversize caches refresh pixels; observe RED.
- [x] Wire optional Context.InvalidateImage to Runner renderer, forward through uihost fallback wrapper, retire discarded private strip images on every drop path. Document UI-thread/Damage constraints and update API listing.
- [x] Run minimap/damage/host fallback tests and compare final editor to native-only/baseline paths; images remain immutable and IDs valid.

### Task 3: Acceptance and delivery
- [x] Compare matched native churn and warm editor frames before/after; inspect updated profile and process CPU, retain only demonstrated improvements.
- [x] Full editor/examples and root short regressions, meaningful changed-package race, vet/build/formatting and independent fresh-context review.
- [x] Report reproducible results/limits; verify tango identity and clean local main before delivery.
- Delivery: commit as tango, fast-forward local main, build the clean merged tree and safely update the app under standing authorization. Record the resulting commit, binary and live process checks in the external artifact directory's delivery.json and archive this plan's execution ledger there.
