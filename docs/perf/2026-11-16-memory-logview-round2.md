# Resident memory and LogView allocations — round two

Measured on 2026-11-16, Apple M4/macOS, Go 1.27.3. Measurement baseline:
`dcaf77f`, which includes the first-round optimization. This round preserves
the public APIs, default GPU choice and rendered pixels.

## Persistent Metal upload buffers

The staging and instance buffers previously only grew. After temporary large
uploads, the patch texture could retire while its upload buffer still retained
peak capacity. Each buffer now records the maximum bytes needed during
consecutive committed frames using at most one quarter of its capacity. After
120 such frames, the next valid frame releases a wholly unused large buffer
or shrinks it to fit that maximum/current demand, with a 4 KiB minimum for
nonempty buffers. Small buffers keep their floor allocation; recurring large
frames reset the grace period rather than causing shrink/grow churn.

Retirement uses the existing wait for previous render/present submissions.
Replacement occurs after validation, mandatory allocations and command-buffer
creation, before encoding references. It copies the current packed bytes and
allocates a replacement before releasing the old buffer. Optional allocation
failure retains usable storage. Empty/rejected frames and Wait-only calls do
not advance the committed-frame usage window. There is no new GPU wait,
per-frame collection or idle redraw timer. The window must continue executing
frames before this policy can retire its peak allocation.

One paired native measurement per scene, identical source/probe programs and
build flags, sequential 17-second processes. The table uses median process
physical footprint from seconds 10–15, before the probe's first forced GC at
18 seconds. Decimal MB:

| Scene | First-round baseline | Round two | Interpretation |
| --- | ---: | ---: | --- |
| Large transient content → ordinary drawing | 188.43 MB | 180.21 MB | −8.22 MB in this single run |
| Static counter control | 52.95 MB | 52.64 MB | No Metal resource change; treat difference as noise |

Independent transient resource counters show staging allocatedSize
7,766,016 → 0 bytes, and instance allocatedSize 32,768 → 4,096 bytes.
The device's currentAllocatedSize changes from 40,828,928 to 33,161,216 bytes;
this and OS footprint are different accounting views and must not be summed.
The control's owned Metal allocation is identical before/after. This is
transient-capacity reclamation, not a general claim of lower static-window
baseline memory or elimination of driver-associated fixed costs.

## LogView glyph clipping

The post-round-one profile still shows temporary alpha masks created for
cached glyphs inside rounded clips. The CPU renderer now checks whether the
glyph's whole destination has coverage 255. If so, it draws directly from
the cached glyph mask. Partial coverage, out-of-mask areas and fractional
corners retain the existing mask multiplication. The glyph cache is never
modified and no new persistent cache is introduced.

The unchanged million-line log benchmark appends one line and renders a CPU
frame. Three alternating-order fixed-iteration rounds, GOMAXPROCS=2,
1,000 iterations per process, medians. Fixed iteration counts keep the
number of appended lines identical in both revisions:

| Metric per operation | First-round baseline | Round two |
| --- | ---: | ---: |
| Allocations | 2,495 | 1,004 |
| Allocated bytes | 777,648 | 651,209 |
| Wall time | 0.876 ms | 0.873 ms |

Allocation count falls 59.8%, and allocated bytes fall 16.3%. These resource
measurements were consistent across the three fixed-iteration rounds. Timing
ranges overlap (baseline 0.793–0.916 ms, optimized 0.705–0.948 ms), so this
round does **not** establish an additional CPU-time speedup. Earlier
time-based calibration runs were especially noisy as workstation load rose
above 20 on this 10-logical-CPU machine; their raw logs remain intact but are
not used to claim a timing gain. These are CPU rasterization benchmarks, not
GPU presented FPS or input latency.

## Regression evidence and limits

The new glyph allocation regression fails before the fix: three cached glyphs
inside an opaque clip allocate eight times versus two for ordinary unclipped
drawing. It passes after the change. Independent multiplied-mask reference
pixels cover opaque/nonopaque parent masks, clipped/off-mask glyphs,
translucent ink/background, and preservation of cached source pixels.

Native Metal regressions fail on the former grow-only buffers and pass after
retirement. They cover release/rebuild, changed current upload pixels through
nonzero shrink, periodic demand spikes, invalid/empty frames, expired grace
periods, and optional replacement failure. The usage window records CPU-side
successful commits; later GPU completion failures are not represented as
successful pixel measurements. As before, idle windows without executed
frames retain resources until work resumes.

Local raw data, profiles, RED/GREEN logs, archived benchmark sources/binaries,
and reproduction helpers are under
`/Users/admin/min/outputs/tango-memory-log-opt-round2-20261116/`.
`logview/summary.json` preserves both timing protocols; `memory/REPORT.md`
and `memory/summary.json` detail native counters and collection limits.

The measured source snapshots precede subsequent upstream feature-profile and
theme changes. The final changes are integrated with those updates and checked
separately; the controlled before/after numbers above are for the pinned
measurement baseline and this patch, not a claim about every later revision.

Final verification passed production and development root suites, the two
gallery packages in CI short mode, CLI tests, focused race regressions, root
and CLI vet, and Windows/Linux root vet in both default/development modes.
Independent review found no material correctness issues. Four newly integrated
Darwin profile constant stubs received formatting-only corrections. Native
resource/footprint measurements are macOS-only; cross-platform checks establish
compilation/static analysis rather than runtime memory improvements.
