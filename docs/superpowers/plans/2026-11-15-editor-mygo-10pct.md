# Editor MyGo 10% Performance Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make each selected Tango editor core benchmark at least 10% faster than the equivalent MyGo benchmark on the same machine and input, or document a proven blocker.

**Architecture:** Profile existing `ui/editor` core paths first. Add focused regression/benchmark coverage, then optimize only confirmed hotspots. Keep renderer and platform code out of scope.

**Tech Stack:** Go 1.27, `testing` benchmarks, `pprof`, Tango Monarch lexer, editor buffer.

**Spec:** `docs/superpowers/specs/2026-11-15-editor-mygo-10pct-design.md`

## Global Constraints

- Acceptance is per metric: Tango median time must be at most 90% of MyGo median.
- Same machine, Go version, generated input and benchmark repetitions are required.
- Preserve tokenizer output, state transitions, UTF-16 columns, timeout degradation and public APIs.
- Do not copy MyGo source or grammar data.
- Product changes stay under `ui/editor/**`; no renderer/platform changes.

## Review Focus

- Multiline lexer state must survive strings/comments and embedded regions.
- UTF-16 columns and non-ASCII input must remain exact after fast paths.
- Timeout/degraded-line behavior must still stop pathological grammars.
- Reused token slices must not retain stale tokens across short and long lines.
- Large-file edit/index paths must not turn a local edit into a full document scan.

### Task 1: Establish benchmark and profile baseline

**Files:**
- Read: `ui/editor/monarch/bench_test.go`, `ui/editor/monarch/lexer.go`, `ui/editor/buffer/bench_test.go`
- Create: `.superpowers/sdd/2026-11-15-editor-mygo-10pct/`

- [ ] Run the focused Tango and MyGo benchmarks three times and save raw output.
- [ ] Capture a CPU profile for Tango's tokenizer benchmark.
- [ ] Inspect the top hotspots and write a short evidence note before editing code.

### Task 2: Add regression coverage for the confirmed hotspot

**Files:**
- Test: `ui/editor/monarch/*_test.go` or `ui/editor/buffer/*_test.go`

- [ ] Add a focused correctness test for the exact behavior the profile targets.
- [ ] Run the focused test and confirm it passes on the baseline.
- [ ] Add/adjust a benchmark that keeps the MyGo comparison input fixed.

### Task 3: Implement the smallest core optimization

**Files:**
- Modify only the file identified by Task 1, under `ui/editor/**`.

- [ ] Implement the minimal allocation/matching/state change.
- [ ] Run focused correctness tests and the benchmark.
- [ ] Reject the change if the median improvement is below noise or any semantic test regresses.

### Task 4: Re-measure and verify the 10% gate

**Files:**
- Read: saved evidence and profiles.
- Update: `comparison.md` in the evidence directory.

- [ ] Re-run Tango and MyGo benchmark pairs with fresh processes.
- [ ] Run `go test ./ui/editor/... -count=1`.
- [ ] Run `gofmt -l` on modified files and `git diff --check`.
- [ ] Mark each metric pass, fail or statistically unresolved; do not claim overall success if any selected metric fails.
