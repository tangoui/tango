# uibench: out-of-process cross-framework UI benchmark

Measures Tango against MyGo and NSGo on the same scenes from outside the
programs, as the design (docs/superpowers/specs/2026-11-16-ui-component-performance-design.md
§6.3) asks. Every directory is its own Go module and builds with
`GOWORK=off`; nothing here is part of Tango's product module or API golden.

| Directory | Module | What |
|---|---|---|
| `proto/` | `tango.local/uibench/proto` | the line protocol the scene programs print |
| `runner/` | `tango.local/uibench/runner` | starts the programs, samples, judges, writes JSON |
| `tango/` | `tango.local/uibench/tango` | Tango scenes (`replace github.com/tangoui/tango => ../../..`) |
| `mygo/` | `tango.local/uibench/mygo` | MyGo v0.9.0 scenes, self-drawn `ui` mode |
| `nsgo/` | `tango.local/uibench/nsgo` | NSGo scenes (`replace github.com/nsgo-dev/nsgo => ../third_party/nsgo`) |

Scenes and their per-frame work: [scenes.md](scenes.md) (the base scenes
counter, list and grid, and the component scenes: buttons, inputs, textarea,
checkboxes, radios, switches, sliders, progress, tabs, menu, dialog, toast,
tree, datepicker, colorpicker).

## Use

```sh
cd tools/uibench
./setup.sh /path/to/nsgo/source   # links third_party/nsgo, writes third_party/nsgo.version
./build.sh                        # bin/tango, bin/mygo, bin/nsgo, bin/runner, bin/build-info.txt
cd runner                         # the runner is its own module: run it from here
GOWORK=off go test ./... -count=1
GOWORK=off go run . -bin ../bin -scenes counter,list,grid -frameworks tango,mygo,nsgo \
	-rounds 5 -settle 3s -seconds 10 -out ../results/$(date +%F).json
# smoke (opens windows, about 2 minutes; not a performance result):
GOWORK=off go run . -bin ../bin -scenes counter,list,grid -rounds 1 -seconds 3 -out ../results/smoke.json
# component scenes smoke:
GOWORK=off go run . -bin ../bin -rounds 1 -seconds 3 -out ../results/smoke-components.json \
	-scenes buttons,inputs,textarea,checkboxes,radios,switches,sliders,progress,tabs,menu,dialog,toast,tree,datepicker,colorpicker
```

`go run ./runner` from tools/uibench does not work: tools/uibench has no
go.mod, so the go command resolves the parent Tango module instead.

The runner opens real windows and needs an unlocked screen: behind a
locked screen the windows are occluded and Tango and MyGo (which draw on the
display's frame clock) draw nothing. The runner refuses to start when the
session screen is locked, and if the screen locks during a run it marks the
rounds `screen_locked`, which the judge does not read. `bin/`, `third_party/` and
`results/` are git-ignored.

## Protocol

A scene program is started as `<bin> -scene <name>`; an unknown scene exits
with status 2. It prints on standard output:

```
bench renderer <name>
bench ready
bench frames <fps>          once a second
bench work <work/s>         once a second
bench intervals_us <a,b,…>  once a second
```

## Measurement

Per scene, `-rounds` rounds; in each round every framework's program is
started afresh, in an order that rotates by one each round. For each:

1. Load gate before the round (1-minute load < 6, waiting up to 10 min,
   as uiperf's gate).
2. Start, wait for `bench ready` (30 s timeout; startup = exec to ready),
   bring the window to the front with `osascript`, wait `-settle`.
3. Sample `-seconds`: process CPU (`proc_pid_rusage` flavor 2,
   user+system, Mach time converted with `mach_timebase_info`);
   WindowServer CPU (`ps -o time=` before and after); fps and work/s
   (means of the per-second reports); frame-interval p50/p99 (nearest rank
   over all intervals reported); phys_footprint at the end and its peak.
4. Load ≥ 6 before or after sampling marks the attempt `disturbed`; it is
   measured again, up to 3 more times. Every attempt stays in the file.

Per-second reports arrive as groups (frames, work, intervals). A group is
taken whole or not at all, and the first group completed after sampling
starts is dropped, since part of it lies before the start. Ctrl-C or
SIGTERM stops the running scene program's process group before the runner
exits (status 130), so no orphaned scene keeps drawing.

## Judge

`judge(tango, opponent, minRounds)` reads only rounds that are ready,
without error, not disturbed, not screen-locked and with a known renderer
(not empty or `unknown`), and returns:

- `Invalid` when either side has fewer such rounds than `-rounds` (never
  fewer than 3: with one round Q1 = Q3 = the median and the interquartile
  condition would hold by itself), a framework changed its
  renderer between rounds, or the work per frame (work/s ÷ fps) differs by
  more than 5%;
- `Faster` when Tango's median (process + WindowServer) CPU is at most half
  the opponent's, Tango's third quartile is below the opponent's first,
  median fps is at least the opponent's and median frame-interval p99 at
  most the opponent's (counter: footprint median at most the opponent's in
  place of fps and p99);
- `NotFaster` otherwise.

## Methodology choices

- **MyGo frame driving** follows MyGo's documented API: the view calls
  `c.AnimationFrame()` every frame ("asks for another frame as soon as the
  display can show it", ui/api_gen.go), as MyGo's own benchmark app does.
  Tango uses `c.WakeAt(c.Now())`, NSGo chains `cx.RequestAnimationFrame`.
- **Tango list uses `ui.Listy`**, with Virtual, ItemHeight(40), Split(false),
  RowHoverable(false) and ScrollTo(px). `ui.VirtualList` scrolls only by
  whole rows (ScrollToIndex / ScrollIntoView), so it cannot scroll 8 px a frame.
- **The row's third text is `"<i*13%100> ms"`**, identical in all three
  programs, after `"Row <i>"` and `"<i*7%1000>"`.
- **The 5% work check compares work per frame** (work/s ÷ fps). Work per
  second would mix the frame rate into the work check; fps is judged on its own.
- **The counter scene compares median memory** (phys_footprint, Tango ≤
  opponent) instead of fps and p99, alongside the CPU conditions.

- **Counter CPU is mostly noise.** At rest the process uses almost no CPU,
  and WindowServer's `ps` time has 10 ms resolution, so the counter's
  (process + WindowServer) sum is dominated by compositor noise. Process
  CPU (`cpu_s`) and WindowServer CPU (`windowserver_s`) are recorded
  separately in every round so the two can be read apart.

## Build modes

`build.sh` builds every program with `GOWORK=off CGO_ENABLED=0 go build
-trimpath -ldflags "-s -w"` and one toolchain (`GOTOOLCHAIN=go1.27.3`). A
framework that fails without cgo is rebuilt with `CGO_ENABLED=1`; the
mode goes to `bin/build-info.txt` and into every result's `build` field.

As of 2026-11-17 all three build with `CGO_ENABLED=0`.

## Written independently

MyGo and NSGo ship their own benchmark suites. This one borrows their ideas
(external rusage sampling, a ready line, frame intervals) and none of their
code. The scene programs in `mygo/` and `nsgo/` were written independently
from each framework's public API and documentation, without working from
the opponents' benchmark sources: the widget keys, the counter's text and
layout, the code organization and the grid's cell formula
(`(cell*131 + frame*17) % 10000`, shared by all three programs) are this
suite's own. Where the code resembles an opponent's, it is because the
framework's API allows only that shape.
The component scenes (Task 10) were written the same way: their texts,
counts, starting values and formulas (`components_data.go`, shared by all
three programs) are this suite's own.
