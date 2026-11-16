# Resident Memory and LogView CPU — Round Two

**Goal:** Improve the remaining costs measured after `dcaf77f`, preserving APIs, pixels and default rendering.

**Approach:** Use the existing Metal synchronization and committed-frame lifecycle to retire persistently oversized upload/instance buffers after sustained low demand. Reuse cached CPU glyph masks when their complete destination is covered by an opaque clip. No new persistent CPU mask cache or text-layout change.

**Constraints:** No extra GPU waits, no per-frame GC, no shrink churn for periodic high-demand frames, no state aging on invalid or skipped frames, and unchanged glyph blending/partial-clip behavior. Keep the user's main checkout and other worktrees intact.

**Artifacts:** `/Users/admin/min/outputs/tango-memory-log-opt-round2-20261116/`.

- [x] Establish the post-round-one LogView profile and Metal buffer retention baseline.
- [x] Observe failing real-renderer resource/allocation regressions, then implement the bounded fixes.
- [x] Compare glyph pixels against independently multiplied masks; verify upload and instance data survive shrink/rebuild and failed optional allocations.
- [x] Run alternating before/after LogView benchmarks and sequential native memory samples without concurrent heavy benchmarks.
- [x] Review changes independently, run root/default, root/dev, CLI, race, formatting and vet checks, and record measurement limits.
- [x] Commit and push the tested changes under the user's existing delivery preference.

Verification: 12 checks passed, repository formatting clean, independent review clear. Timings overlap; report only the confirmed allocation reduction and workload-specific native memory reclamation. Raw manifest: `/Users/admin/min/outputs/tango-memory-log-opt-round2-20261116/verification.json`.
