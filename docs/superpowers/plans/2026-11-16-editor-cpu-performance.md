# Editor CPU Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement inline.

**Goal:** Remove measured redundant CPU/memory work while preserving editor behavior and pixels.
**Architecture:** Eliminate duplicate image conversion/visible-text reads; reuse bounded temporaries at lexer and view scope.
**Tech Stack:** Go, offscreen Metal, CPU rendering, pprof, allocation tests.
**Spec:** docs/superpowers/specs/2026-11-16-editor-cpu-performance-design.md

## Global Constraints
- No feature removal, grammar/API changes, relaxed budgets or mutable recorded images.
- New caches/scratch must be bounded by viewport/one line; clear stale row references.
- Keep the running editor until the verified replacement is ready.

## Review Focus
- Premultiplication preserves exact rounding, transparent RGB, tall rows and fractional display scales.
- Guide source seeding ignores view zones and uses whole model-line text on wrapped rows.
- Rune reuse handles invalid UTF-8, astral text, embedded languages, empty/end-of-line matches and captures beyond matches.
- Scratch lifetimes do not alias persistent cache entries, image pixels or old paint lists.
- Measurements isolate GPU wait and setup; sustained workloads cannot silently turn off functionality.

### Task 1: Minimap pipeline
- [x] Add strip conversion allocation/pixel tests, warmed strip-request allocation check and microbench; observe RED budgets.
- [x] Return premultiplied RGBA strips directly, preserving immutable images; retain cleared strip-request scratch.
- [x] Run minimap/strict-damage regressions and before/after microbench; expect PASS and fewer allocation bytes.

### Task 2: Visible indentation guides
- [x] Add visible guide work allocation budget/microbench and wrapped/zone correctness coverage; observe RED allocation budget.
- [x] Seed per-frame guide indents with existing visible whole-line text, avoiding rereads; leave offscreen searches unchanged.
- [x] Run guide/wrap/zone regressions and microbench; expect PASS and less work.

### Task 3: Lexer temporary work
- [x] Add short backtracking line benchmark/budget and ASCII offset budget with explicit Unicode/invalid/capture boundary expectations; observe RED budget.
- [x] Lazily share line runes for candidate backtracking matches; skip ASCII byte-offset arrays.
- [x] Run full regex/monarch fixtures, embedded/budget/timeouts, race and microbench; retain only measured improvements.

### Task 4: Acceptance
- [x] Interleave before/after full and partial frame typing/scroll benchmarks plus 1M-line/open regression checks, and inspect updated profiles.
- [x] Verify full editor/example regressions, root short suite, changed-package race, vet/build, independent fresh-context review.
- [x] Record reproducible results including sustained/idle limits and commit with tango identity. Delivery uses the user’s standing authorization to merge clean main and restart after checking unsaved edits.
