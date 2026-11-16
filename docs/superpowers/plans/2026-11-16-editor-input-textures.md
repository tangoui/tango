# Warm Input Texture Reuse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement inline. User already delegates execution and routine implementation choices.

**Goal:** Reduce repeated native allocation when minimap strip widths recur across input frames.
**Architecture:** Metal retains a tiny exact-size pool of detached textures after prior submissions finish; frame-local handoff remains first choice. Optional device trimming reaches it from the existing renderer trim path.
**Tech Stack:** Go, purego Metal, native identity/pixel tests and real offscreen editor workload.
**Spec:** docs/superpowers/specs/2026-11-16-editor-input-textures-design.md

## Global Constraints
- Preserve UI/features/pixels, immutable images/lists, logical IDs, complete upload order, production budgets and failure policy.
- Spare pool: maximum16 objects /1<<20 nominal RGBA bytes, FIFO eviction, expire after120 subsequent applyImages calls; clear on no-upload frame, native failures, invalid applyImages and Close.
- Existing same-frame32-object/8MiB transient handoff stays bounded separately; exact-size RGBA8 ShaderRead only after wait, never alias active textures.
- Renderer.Trim calls optional TrimImages; native trim immediately clears spares and suppresses refilling on next applyImages. No cross-backend protocol/shader/public UI API changes.

## Review Focus
- A freed texture still referenced by previous GPU work must never be mutated before wait; multiple simultaneously live IDs remain distinct.
- Reused widths/heights/alpha/stride must fully replace old pixels; mismatched dimensions never reuse.
- Native errors, Close and low-memory trim must release both local candidates and persistent spares without double release; repeated lifecycle calls safe.
- Adversarial unique sizes and long upload sequences stay within count/bytes/age caps; no-upload frames release idle resources.
- Measurements must accept all keystrokes with all features enabled; warm profiles exclude setup, component gains do not substitute for full-editor results.

### Task 1: Bound and trim unused Metal textures
**Files:** modify internal/render/gpu/metal/images_darwin.go, metal_darwin.go, internal/render/gpu/renderer.go; create internal/render/gpu/metal/image_pool_darwin.go and image_pool_darwin_test.go.
**Interfaces:** Device retains imageTexturePool; pool owns only unused imageCandidate values. Device.TrimImages() is an optional capability consumed by gpu.Renderer.Trim.
- [x] Write native alternating-size identity/pixel test, observe baseline allocation failure; add workload benchmarks before compiling baseline binaries.
- [x] Add tests through Device/Renderer for mismatch, distinct/live IDs, no-upload release,121-age workload,16 count/1MiB bounds, invalid return/failure/Close and low-memory trim. Retain old native objects to eliminate allocator-address ambiguity.
- [x] Implement bounded FIFO/age pool, integrate claim/deposit and every release path; TrimImages drops it and suppresses one refill. Unknown devices keep existing Trim behavior.
- [x] Focused identity/pixel/lifecycle tests and full Metal/renderer trim regression PASS; variable-size benchmark demonstrates reuse without changed pixels.

### Task 2: Measure and deliver
**Files:** ui/editor/perf_input_profile_test.go (warm-only opt-in observation); docs/specs/editor-input-performance-2026-11-16.md; plan/spec; external measurement artifacts and execution ledger.
**Interfaces:** real GPU renderer/devices from Task1, unchanged editor benchmark fixtures. Device wrappers in diagnostic test preserve retention and cache trimming capability where used.
- [x] Validate actual warm-profile input workload, preserve baseline/candidate binaries; alternating native and full/partial TS20k editor comparisons, warm-only profile and sustained memory, report mixed results and1MiB nominal residency tradeoff.
- [x] Full Metal/editor/subpackages/host/demo regressions, root short suite, meaningful changed-path race, API/vet/build/gofmt/diff and fresh whole-branch review. Address substantive findings with RED→GREEN.
- [x] Verify tango identity and clean local main; prepare report and completed acceptance evidence for delivery.
- Delivery: commit and fast-forward local main; build clean merged tree and safely update/start app with unsaved-content protection. Record commit, SHA, PID and limits in external delivery.json; archive only this plan's scratch ledger and retain the host-owned worktree.
