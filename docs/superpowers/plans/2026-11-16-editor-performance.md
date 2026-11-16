# Editor performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement inline.

**Goal:** Reduce repeated editor frame allocations while preserving behavior and appearance.
**Architecture:** Retain only bounded per-view temporary buffers, with persistent cached results copied separately. Existing lexer and renderer algorithms remain unchanged.
**Tech Stack:** Go, Tango editor, Go allocation benchmarks, offscreen Metal and pprof.
**Spec:** docs/superpowers/specs/2026-11-16-editor-performance-design.md

## Global Constraints
- No disabled display features or public API/grammar changes.
- Existing running editor sessions remain untouched.
- Use same workload and environment for before/after comparisons.

## Review Focus
- Scratch slices cannot alias tokens stored in persistent minimap cache.
- Style buffers cannot overwrite colors needed by another row in the same frame.
- Alternate themes/lexers, wrapping, tabs and non-ASCII glyphs preserve existing pixels.
- Memory retention remains tied to view/viewport workload, not entire file length.
- Benchmark setup or GPU wait must not be described as real input latency.

### Task 1: Minimap allocations
- [x] Add allocation tests and microbenchmarks for miniature glyph generation and warmed minimap preparation; watch allocation thresholds fail.
- [x] Reuse minimap token-query/keep-set buffers and use bounded local cell scratch for <=128 glyph cells; keep all cached tokens/bitmaps owned.
- [x] Run minimap/navigation/golden tests and benchmark before/after.

### Task 2: Shaping temporaries
- [x] Add warm style-run allocation test; watch baseline fail.
- [x] Reuse shaping input run and override-boundary scratch while keeping returned paint colors independent.
- [x] Run editor damage/wrapping/inline-style/line-cache regressions and repeat full-frame input/scroll benchmarks.

### Task 3: Verify and report
- [x] Run full suite, changed-package race tests, vet/build/formatting checks and fresh review.
- [x] Record reproducible before/after measurements and limits in a performance report, with no premature FPS/latency claims.
