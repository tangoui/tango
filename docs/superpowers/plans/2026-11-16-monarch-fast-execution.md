# Monarch Fast Execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add conservative compiled fast paths that reduce built-in HTML tokenizer overhead while retaining the generic Monarch fallback.

**Architecture:** Add rule candidate metadata first, then add assertion-aware RE2 plans and fixed embedded-boundary scanning. Each stage is independently gated by differential tests and can return to the generic path.

**Tech Stack:** Go 1.27.3, `regexp`, existing `ui/editor/regex`, Monarch lexer.

**Spec:** `docs/superpowers/specs/2026-11-16-monarch-fast-execution-design.md`

## Global Constraints

- Preserve custom grammar semantics and existing timeout/work budgets.
- Do not fork regexp2 in this phase.
- Use Go1.27.3 for tests and benchmarks.
- Require differential output equality before publishing performance results.

## Review Focus

- Rules with captures, `cases`, `@rematch`, `goBack`, and dynamic states must use the generic path unless proven safe.
- Inline flags, Unicode, invalid UTF-8, and multiline state must not enter byte-only fast paths.
- Embedded `script/style` boundaries split at exact byte offsets and preserve child state.
- A rejected candidate must not change timeout/degraded accounting.
- Concurrent calls on one Lexer must not share mutable scratch state.

### Task 1: Candidate metadata

**Files:** Modify `ui/editor/monarch/compile.go`, `ui/editor/lexer.go`; test `ui/editor/monarch/fastpath_equivalence_test.go`.

- [ ] Add immutable per-rule ASCII candidate metadata for simple literal/class prefixes.
- [ ] Add tests for nested alternatives, Unicode, ignoreCase, captures, dynamic rules, and fallback equality.
- [ ] Run related Monarch tests and the differential stream.

### Task 2: Assertion-aware RE2 plans

**Files:** Modify `ui/editor/regex/translate.go`, `ui/editor/regex/regex.go`; add focused tests under `ui/editor/regex`.

- [ ] Recognize only anchored consuming atoms followed by a simple positive/negative ASCII lookahead.
- [ ] Return the consuming span and evaluate the assertion without regexp2 allocation.
- [ ] Reject nullable, Unicode-sensitive, capture-dependent, nested, and unsupported assertions.
- [ ] Compare captures, spans, invalid UTF-8, and timeout behavior with the existing path.

### Task 3: Embedded boundary scanner

**Files:** Modify `ui/editor/monarch/lexer.go`; add HTML boundary tests and benchmark counters.

- [ ] Use direct case-aware byte search for fixed `</script`/`</style` boundaries only when the embedded state is built-in and state-compatible.
- [ ] Preserve child tokenizer state, offsets, line budgets, and fallback on malformed boundaries.
- [ ] Run HTML differential tests and race tests.

### Task 4: Measurement and integration

**Files:** Modify `ui/editor/monarch/html_bench_test.go`, report docs.

- [ ] Run five low-load paired samples for warm HTML, cold compile, and MyGo.
- [ ] Run full related/race tests, repository tests, vet, and source/hash checks.
- [ ] Record gains and remaining MyGo gap; commit without automatic push.
