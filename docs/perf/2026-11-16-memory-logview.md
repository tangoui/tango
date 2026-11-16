# Resident memory and LogView CPU — 2026-11-16

Baseline: `1d76834`. Apple M4, macOS 26.6, arm64, Go 1.27.3.
The public API, default GPU selection and UI appearance remain the same.

## LogView CPU

The unchanged `BenchmarkLogViewFrameMillion` starts with a million ANSI log
lines, appends one line per operation, and runs the real UI driver frame. It
includes CPU pixel rasterization. The baseline profile attributed about 45%
of samples to rounded-clip mask generation, including its coverage function;
it did not show a full-log scan on every frame.

Rounded clip masks now compute coverage once across each non-corner row band
and copy the resulting byte across it. Corner pixels and nested parent-mask
modulation retain the scalar coverage definition. An intersection wholly in
an opaque part of a rounded clip reuses the parent mask without allocating.
No additional persistent mask cache is introduced.

Three alternating-order rounds per revision, 1 second per benchmark, with
the same test inputs and Go toolchain. The baseline uses an overlay containing
the unchanged `ui/paint/cpu.go`; the optimized run uses the new implementation.
Both revisions use the same benchmark code. Medians:

| Benchmark | Baseline | Optimized | Time reduction |
| --- | ---: | ---: | ---: |
| Million-line log append + CPU frame | 3.472 ms/op | 0.723 ms/op | 79.2% |
| 1000×600 rounded clip mask | 2.928 ms/op | 0.074 ms/op | 97.5% |

The log benchmark ranges were 3.449–3.625 and 0.701–0.734 ms/op. Allocated
bytes stayed near 780/776 kB per operation; this change primarily reduces
CPU computation. Allocation counts varied with the benchmark's different
iteration counts and are not presented as an improvement. These timings are
not native GPU frame times, presented FPS, or input latency. Earlier short
review samples used a different measurement interval and are not the baseline
for the percentage above.

## Resident GPU resources

Unused gradients previously allocated a 524,288-byte CPU ramp mirror, uploaded
the full ramp texture, and enlarged the Metal upload staging buffer in every
window. These resources are now created when the first gradient is drawn.
Native-only content also leaves the optional patch texture unallocated.

Patch pages used by transient complex content were previously retained at
their largest size. They now remain available for 120 patch-free executed
frames, then release their optional texture and restart future growth from
the existing minimum. This avoids immediate allocation churn. Completely
idle windows do not start a timer or render extra frames to reclaim them.
`Trim` resets the optional resource state; device resources and references
held by the previous frame's uploads are reclaimed on a later executed frame,
consistent with the existing atlas/image lifecycle. A skipped partial frame
does not execute that cleanup.

Paired native processes used identical scene programs and diagnostic probes,
Go 1.27.3 and `CGO_ENABLED=0`. Each scene ran 17 seconds; the table uses median
process physical footprint from seconds 10–15, before forced collection or
VM inspection. One run per revision and scene:

| Scene | Baseline | Optimized | Difference |
| --- | ---: | ---: | ---: |
| Counter, 360×260 at scale 2 | 56.44 MB | 54.46 MB | −1.98 MB |
| Quotes, 560×860 at scale 2 | 159.75 MB | 157.29 MB | −2.46 MB |
| List, 480×800 at scale 2 | 142.48 MB | 141.53 MB | −0.95 MB |
| Transient complex content → ordinary content | 202.57 MB | 188.04 MB | −14.53 MB |

MB = 1,000,000 bytes. These single-run footprint differences have workstation
noise; they are not confidence intervals or a framework ranking. Direct Metal
counters independently show 1,146,880 fewer allocated device bytes in ordinary
scenes. The transient scene releases a 16,908,288-byte patch texture to its
128-byte empty binding, reducing total reported Metal resources by 17,973,248
bytes. The 524,288-byte Go mirror is separate and must not be added to OS or
Metal measurements as if their accounting did not overlap.

The existing driver-associated fixed cost and much of the previously grown
upload staging buffer remain. This change does not close the whole memory gap
with native AppKit or claim that all idle memory has been eliminated.

## CPU layer lifetime

Popping a CPU clip/opacity layer shortened the stack slice but left pointers
to its targets and masks in the backing array. After a window resized or used
fewer layers, those entries could retain obsolete large images. Popped slots
are now cleared. A weak-reference regression fails on the old implementation
and passes after the fix while keeping the renderer alive; existing
compositing tests verify that the drawing result is preserved.

## Verification and artifacts

New regressions cover allocation-free opaque clip intersections, exact scalar
coverage over fractional bounds/scales/asymmetric radii and parent masks,
releasing an obsolete render target, unused gradient/patch allocation,
first later gradient upload, patch eviction/reconstruction, and Trim rebuild.
The behavior regressions were observed failing before their production fixes.

Performance artifacts, RED/GREEN logs, profiles, source overlays, native
probes and raw samples are retained locally at
`/Users/admin/min/outputs/tango-memory-log-opt-20261116/`. Its `logview/summary.json`
contains the three-round timing aggregates; `memory/REPORT.md` and
`memory/summary.json` contain native measurement details and limits.

The unchanged log benchmark can be rerun on either revision with:

```sh
GOTOOLCHAIN=go1.27.3 go test ./ui -run '^$' \
  -bench '^BenchmarkLogViewFrameMillion$' -benchmem -benchtime=1s -count=3
```

Final integrated verification passed the production root suites (167 packages),
the two gallery packages in CI short mode, the development root suites, the
CLI module, focused race regressions, and root/default, root/development and
CLI vet. Repository formatting and `git diff --check` are clean. The final
independent code review found no material correctness issues. Native process
measurements above are macOS-only; Windows/Linux backends are not presented
as physically measured here.
