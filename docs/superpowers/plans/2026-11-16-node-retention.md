# UI Node Retention Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Release abandoned widgets during scrolling without invalidating retained widget pointers.

**Architecture:** Give node, Label and Box independent allocations and preserve ID-based reuse. Remove frame-block allocator state and its per-build reset.

**Tech Stack:** Go 1.27, existing UI runner/driver, weak references in tests, macOS native-window probes.

**Spec:** `docs/superpowers/specs/2026-11-16-node-retention-design.md`

## Global Constraints

- No public API, theme or default renderer changes.
- Never clear/recycle objects that external widget pointers may retain.
- Keep unrelated work and the existing analysis artifacts intact.
- Run implementation inline; use the isolated `fix/ui-node-retention` checkout.

## Review Focus

- An abandoned node must be collectible while an allocation peer remains live.
- The same requirement applies to Label and Box objects.
- Scrolling away must release the initial row's widget while the runner stays live.
- Holding an older widget pointer must not cause its storage to be overwritten.
- Stable widget IDs must continue to reuse nodes and preserve input behavior.

## Task 1: Widget allocation ownership

**Files:** `ui/frame_alloc.go`, `ui/context.go`, `ui/runner.go`, `ui/frame_alloc_test.go`, `ui/more_virtual_test.go`.

**Interfaces:** Preserve `allocNode() *node`, `allocLabel() *Label`, `allocBox() *Box`; remove allocator-only state/reset.

- [x] Add `TestFrameAllocationsReleaseUnreferencedPeers` for node, Label and Box using weak references and live-peer KeepAlive.
- [x] Add `TestMoreVirtualListReleasesOffscreenWidgets` using real build/layout/afterFrame and an initial-row weak Label reference.
- [x] Run both tests against unchanged code and confirm retention failures.
- [x] Allocate each object independently and remove frame-block state/reset. Replace the allocation-count implementation test with the lifetime regressions; retain the old-pointer independence test.
- [x] Run focused tests, the full repository Go suite and native/development build checks; report any baseline failures separately.
- [x] Compare identical build/scroll benchmarks and the one-minute GPU scene with the fixed main baseline; record exact commands and evidence.
- [x] Review the final diff and commit the verified change with tests and measured results.
