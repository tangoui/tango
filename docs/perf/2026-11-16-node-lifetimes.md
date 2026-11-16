# UI node lifetime regression and fix

Measured on 2026-11-16 with Apple M4, 16 GiB, macOS 26.6, arm64,
Go 1.27.3, CGO_ENABLED=0. Baseline: `a4e7c7802cc8e647befe8bdd2c27cd6cd903be59`.
The fixed build uses independent node, Label and Box allocations. Both builds
retain the same ID-based reuse, renderer, atlas settings and benchmark code.

## Cause and change

A pointer to one element of a `[16]T` allocation keeps the entire array alive.
The initial root and reused widgets can therefore retain abandoned neighbors,
including their parent/child and callback references. Repeated scrolling ties
successive old trees to live allocations. Replacing those blocks with separate
objects lets the collector follow each widget's actual lifetime. Objects held by
callers are neither cleared nor recycled.

Weak-reference regressions first failed on the baseline for all three object
types and for an initial VirtualList row after it left the viewport. They passed
after the change. Existing ID reuse, pointer independence and interaction tests
remain enabled. The former batching allocation-count assertion was removed
because it required the allocation strategy responsible for the bug.

## Native-window validation

Identical 480×800 windows at scale 2, 10,000 rows, one-row scroll per update.
Each build ran for 65 seconds. MB = 1,000,000 bytes.

| Metric | Baseline | Fixed |
| --- | ---: | ---: |
| Process physical footprint, median seconds 10–17 | 160.5 MB | 142.4 MB |
| Process physical footprint, median seconds 55–65 | 271.0 MB | 143.7 MB |
| Go HeapAlloc, median seconds 55–65 | 121.4 MB | 12.9 MB |
| Go HeapAlloc immediately after GC at 18 seconds | 34.2 MB | 8.2 MB |
| Go HeapAlloc after FreeOSMemory at 22 seconds | 40.6 MB | 8.2 MB |
| Median update callbacks per second | 60.0 | 60.0 |

The fixed footprint varied by approximately 1.3 MB between the two windows of
measurement, rather than continuing to grow with scrolling. Heap profiles no
longer show the retained frame-block arrays among the main allocation sites.
One round per native configuration establishes the lifetime improvement, not a
statistical performance ranking or a claim about infinite-duration behavior.

Counter footprint at seconds 10–17 was 52.4/52.1 MB (baseline/fixed). The warmed
quotes scene at seconds 24–29 was 159.9/160.8 MB. Earlier quote samples differed
by about one drawable's memory; use the warmed phase rather than calling that a
widget-allocation regression.

## Headless build/layout measurements

Existing benchmarks, three alternating-order rounds, 1 second per benchmark,
medians shown. These exclude CPU pixel rasterization.

| Benchmark / metric | Baseline | Fixed |
| --- | ---: | ---: |
| ContextGrid600 time | 3.074 ms | 3.104 ms |
| ContextGrid600 allocated bytes | 580,248 | 580,261 |
| ContextGrid600 allocations | 74,080 | 74,081 |
| TableVirtualScrollNoRaster time | 0.320 ms | 0.295 ms |
| TableVirtualScrollNoRaster allocated bytes | 397,012 | 388,625 |
| TableVirtualScrollNoRaster allocations | 6,097 | 6,149 |

The table allocation count rises by less than 1%; allocated bytes decrease.
The grid timing difference is about 1%, within the observed workstation noise.
These samples do not justify a general CPU speedup claim.

Commands:

```sh
GOTOOLCHAIN=go1.27.3 go test ./ui -run '^$' \
  -bench '^(BenchmarkContextGrid600|BenchmarkTableVirtualScrollNoRaster)$' \
  -benchmem -benchtime=1s -count=1 -timeout=3m
GOTOOLCHAIN=go1.27.3 go test ./ui \
  -run 'TestFrameAllocations|TestMoreVirtualList|TestNodeReuse' -count=10
GOTOOLCHAIN=go1.27.3 go test -race ./ui \
  -run 'TestFrameAllocations|TestMoreVirtualListReleasesOffscreenWidgets|TestNodeReuse' -count=3
GOTOOLCHAIN=go1.27.3 go test -p 1 -timeout 40m ./...
```

## Separate Metal fixed cost

The approximately 70 MB `owned unmapped (graphics)` account survives both a
four-buffer command-queue control and a control that waits after every GPU
submission. Synchronous submission changes drawable residency but does not
remove that account. Those experimental changes are not in the product fix.
This patch fixes widget retention; it does not claim that Tango's steady-state
Metal footprint now matches AppKit.

## Evidence and limits

Local experiment artifacts are under
`/Users/admin/min/outputs/tango-memory-fix-20261116/`: source/binary revisions,
`retention-red.log`, `retention-green.log`, `race-tests.log`, per-round headless
logs, native process/Go samples, heap profiles, VM maps, `summary.json`, and the
measurement/build scripts. The separate earlier investigation remains intact.

Application physical footprint is not total system/WindowServer memory. Go heap,
Metal counters and VM categories overlap and must not be summed. GC, profile
collection and VM inspection cause pauses, so callback medians are a guardrail,
not presented FPS, latency or a renderer throughput benchmark. Other workstation
programs and tests remained running during collection.

The original parallel full-suite baseline hit `TestMatchManyCursorsBounded` in
`ui/editor` (10.94 ms vs its 2.75 ms threshold). Its unchanged baseline passed an
isolated recheck. The fixed full suite uses `-p 1`, following CI's serialization
of CPU-heavy render/editor tests; no timing threshold was modified.
