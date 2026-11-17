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
counter, list and grid; the component scenes: buttons, inputs, textarea,
checkboxes, radios, switches, sliders, progress, tabs, menu, dialog, toast,
tree, datepicker, colorpicker; and the nearest-counterpart scenes: select,
tooltip, table, pagination, cascader, transfer, treeselect, calendar,
steps, timeline, descriptions, form, upload, carousel, collapse, drawer,
tour, protable, segmented, rate, tags, badges, cards, statistics). The
same file maps every Tango fixture entry to the scene it is compared in.

## Use

```sh
cd tools/uibench
./setup.sh /path/to/nsgo/source   # links third_party/nsgo, writes third_party/nsgo.version
./build.sh                        # bin/tango, bin/mygo, bin/nsgo, bin/runner, bin/manifest.json
cd runner                         # the runner is its own module: test it from here
GOWORK=off go test ./... -count=1
../bin/runner -bin ../bin -verify # check the manifest, open no window
../bin/runner -bin ../bin -scenes counter,list,grid -frameworks tango,mygo,nsgo \
	-rounds 5 -settle 3s -seconds 10 -out ../results/$(date +%F).json
# smoke (opens windows, about 2 minutes; not a performance result):
../bin/runner -bin ../bin -scenes counter,list,grid -rounds 1 -seconds 3 -out ../results/smoke.json
# component scenes smoke:
../bin/runner -bin ../bin -rounds 1 -seconds 3 -out ../results/smoke-components.json \
	-scenes buttons,inputs,textarea,checkboxes,radios,switches,sliders,progress,tabs,menu,dialog,toast,tree,datepicker,colorpicker
# nearest-counterpart scenes smoke:
../bin/runner -bin ../bin -rounds 1 -seconds 3 -out ../results/smoke-nearest.json \
	-scenes select,tooltip,table,pagination,cascader,transfer,treeselect,calendar,steps,timeline,descriptions,form,upload,carousel,collapse,drawer,tour,protable,segmented,rate,tags,badges,cards,statistics
```

Each scene program's tests (`GOWORK=off go test ./...` in tango/, mygo/,
nsgo/) also run the nearest-counterpart scenes headless and check what
they show, and check every component scene's report cadence against
`sceneWork`; they open no window.

Measure with `../bin/runner`, the runner build.sh built: the runner
refuses to measure when its own SHA-256 is not the manifest's, so `go run .`
(another binary, maybe another toolchain) is refused. `go run ./runner`
from tools/uibench does not work either: tools/uibench has no go.mod, so
the go command resolves the parent Tango module instead.

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
bench scene <name> <w>x<h>  at start: the scene and its window in points
bench ready
bench renderer <name> <ns>  at ready, after every per-second report, and
                            as soon as the path changes between frames
bench frames <fps>          once a second
bench work <work/s>         once a second
bench intervals_us <a,b,…>  once a second
```

`<ns>` is the program's clock in Unix nanoseconds. Each program gives
proto the renderer every frame (Tango polls `uihost.Renderer(runner)` on
the UI thread; MyGo its painter's path once the first frame was painted;
NSGo `appkit`), and proto prints it at the points above. The runner still
reads the old once-only `bench renderer <name>` line, timing it by its own
clock (`runner_clock` in the timeline).

## Renderer recording

The runner removes every inherited `TANGO_*` variable from every scene
program's environment before starting it, says which on startup and lists
them in `meta.cleared_env`: no Tango-only diagnostics (`TANGO_FRAME_TRACE`
adds per-frame cost), and with `TANGO_RENDERER` gone Tango runs its default policy, which may start
native or cpu and move to gpu (or fall back) during a run, so the window
size does not tell the path. Each round keeps:

- `renderer_timeline`: every renderer line, (t_ns, name);
- `renderer_switches`: each change, with its time and `in_sample`;
- `sample_start_ns`, `sample_end_ns`: the window the timeline is read against;
- `renderer`: the one path seen in the sample window (the path in effect
  at its start plus any reported inside it), or `mixed`, with
  `renderer_sequence` (e.g. `native, gpu`) and `renderer_switches_in_sample`.

MyGo and NSGo report a fixed path: MyGo the painter's path probed on its
first painted frame, NSGo always `appkit`. A later fallback inside them
(say GPU to software in MyGo) would not show in their timelines; only
Tango's renderer is polled every frame.

A renderer time earlier than the program's start, or more than 1 s later
than the runner read it, adds the round note `clock step`: the program's
and the runner's wall clocks disagreed, so the timeline may be misplaced
against the sample window.

A mixed round is usable: its CPU includes the switch, which the main
result keeps. The judge's renderer check compares rounds by this summary,
so a mixed round matches only rounds that switched in the same sequence
(`mixed[native>gpu]`); verdicts name each side's path with its switch
count and never call a mixed side by one path. Round lines print
`switches=`, verdict lines `switches=<tango>/<opponent>`.

## Measurement

Per scene, `-rounds` rounds; in each round every framework's program is
started afresh, in an order that rotates by one each round. For each:

0. Before anything, the build manifest must verify (see Build), or the
   runner exits non-zero. Every round records its `input` (scene, window,
   work definition and period, settle and sampling time) and its `build`
   line; `meta.manifest` holds the whole manifest.
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
5. Idle compositor (ruling R44). Before each scene round's launches, and
   again after them, the runner waits `-settle` (so a closing window's
   animation and the focus change are not counted) and samples WindowServer
   for `-seconds` with no scene program running (it refuses to sample while
   one runs). Each attempt's idle rate is the two samples interpolated
   linearly at its sample midpoint (the mean when it has none); every round
   records the raw `windowserver_s`, `windowserver_idle_s` (idle rate ×
   sampled seconds), `windowserver_net_s` = max(0, raw − idle) and the
   samples (`windowserver_idle`). An idle sample above 0.6 cores, before
   and after differing by more than max(0.02, 0.10 × the larger) cores, or
   a failed sample marks the whole round `disturbed` (background
   WindowServer: e.g. a remote viewer connecting), and the round is
   measured again after the load gate, up to 3 more times.
   `meta.screen_sharing_processes` (and `_finish`) list the known
   screen-sharing / remote-desktop programs running (UURemoteServer,
   screensharingd, Screen Sharing, AnyDesk, TeamViewer, ToDesk, …); the
   runner never touches them. Adoptability follows the measured idle
   compositor only (R46): when a scene's median idle over its usable
   (calm) rounds is above 0.1 cores, its verdict lines end with
   "(compositor busy: …)" and `meta.adoptable` is false
   (`meta.not_adoptable`: "compositor busy"): the data is recorded, not
   adoptable. A screen-share program running (a daemon can stay with no
   viewer connected) only adds "(screen-share app running: <names>, idle X
   cores)" to the verdicts and `meta.compositor_notes`; a failed `ps` adds
   "screen-share state unknown".
   A round's attempts are written to the results file only once its
   second idle sample is in, so Ctrl-C loses the scene round in progress.

Per-second reports arrive as groups (frames, work, intervals). A group is
taken whole or not at all, and the first group completed after sampling
starts is dropped, since part of it lies before the start. Ctrl-C or
SIGTERM stops the running scene program's process group before the runner
exits (status 130), so no orphaned scene keeps drawing; once stopping,
the runner starts no further program (the start runs under the same lock
as the stop, so none can slip in between).

### Quiet-machine check

`runner -check-idle [-settle 3s] [-seconds 10]` opens no window and
launches nothing: it waits `-settle`, samples the idle WindowServer for
`-seconds`, and prints the idle cores, the 0.1-core threshold, PASS or
FAIL and the screen-share programs found; it exits 0 on pass, 1 on fail.
Run it (e.g. after disconnecting a remote viewer) before a long
measurement. It runs before, and without, manifest verification: it
starts no scene program, so it needs no build and proves nothing about
one.

## Judge

`judge(tango, opponent, minRounds)` reads only rounds that are ready,
without error, not disturbed, not screen-locked and with a known renderer
(not empty or `unknown`; `mixed` is known), and returns:

- `Invalid` when either side has fewer such rounds than `-rounds` (never
  fewer than 3: with one round Q1 = Q3 = the median and the interquartile
  condition would hold by itself), a framework changed its
  renderer between rounds (by the per-round summary above), or the work per frame (work/s ÷ fps) differs by
  more than the work tolerance: 5%, or for a scene that changes only every
  `period` frames (tabs 10; dialog, toast, tooltip, carousel, collapse,
  drawer and tour 30) max(5%, 1/expected changes), the expected changes
  being the frames sampled ÷ `period` (R35: one change more or less at a
  report's edge is not a difference in work); with less than one expected
  change the rounds are `Invalid` ("too few sampled frames");
- `Info` when the opponent's median (process + net WindowServer) CPU is
  below 0.05 cores: "too cheap to compare" (timer and compositor noise);
- `Info` for a valid comparison of a scene whose default motion differs
  between the frameworks (tabs, dialog, toast, progress, tooltip,
  carousel, collapse, drawer, segmented, steps, upload; scenes.md
  "Default motion", ruling R37): CPU, fps and frame-interval p99 are reported, not judged;
- `Faster` when Tango's median (process + net WindowServer) CPU is at most half
  the opponent's and at least 0.02 cores below it, Tango's third quartile is below the opponent's first,
  median fps is at least the opponent's and median frame-interval p99 at
  most the opponent's (counter: footprint median at most the opponent's in
  place of fps and p99);
- `NotFaster` otherwise.

Every "cpu" the judge, the verdict line (`cpu (proc + ws_net)`) and the
round line print is process CPU plus `windowserver_net_s`, the
WindowServer delta less the round's idle compositor, never the raw delta:
on a machine whose compositor idles at ~0.38 cores (a remote-desktop
server running) the raw sum carries that constant for every framework and
no framework could be half of another.

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

- **Built-in texts are English in all three** (from Task 11): Tango
  `ui.SetDefaultLocale(en)` with `uihost.LocaleFixed`, MyGo
  `MYGO_LOCALE=en` set by the program for its own run, NSGo
  `Scope.SetLocale("en")`, so that a zh-Hans system does not give one
  framework Chinese labels and month names and another English ones
  (scenes.md). Side effects: none persist. Tango's default locale is
  in-memory; MyGo's `App.SetLocale` would save the language for later
  launches (in the app's defaults on macOS), which is why the program sets
  `MYGO_LOCALE` instead, which MyGo never saves; NSGo's `SetLocale` pins
  the language in the app's defaults only for a bundled app (it needs a
  bundle identifier), and the scene programs run unbundled.

- **Counter CPU is mostly noise.** At rest the process uses almost no CPU,
  and WindowServer's `ps` time has 10 ms resolution, so the counter's
  (process + net WindowServer) sum is dominated by compositor noise. Process
  CPU (`cpu_s`) and WindowServer CPU (`windowserver_s`) are recorded
  separately in every round so the two can be read apart.

## Build

`build.sh` runs `go run . -build` in runner/ with exactly
`GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 GOWORK=off
GOFLAGS=` (no `+auto`: a module needing a newer go fails instead of
switching toolchains for one framework). The builder (runner/manifest.go)
refuses to run on another toolchain, builds the three scene programs with
`go build -trimpath -ldflags "-s -w"` and the runner with `-trimpath`,
stops at the first failure (there is no CGO_ENABLED=1 fallback any more),
checks each binary's own build info (`go1.27.3`, CGO_ENABLED, GOOS, GOARCH,
`-trimpath`; and, because Go leaves `-ldflags` out of the build info under
`-trimpath`, that the scene programs carry no defined symbols and no
`__DWARF` segment), and writes `bin/manifest.json`. A failed build leaves
no manifest. For each binary it records:

- the source: Tango and the runner, this repository's commit and whether
  the work tree was dirty (with the files); MyGo, the module version
  mygo/go.mod requires and its go.sum hash, which must equal the
  `dep github.com/egoist/mygo` the binary links (its build info and
  `go version -m`); NSGo, third_party/nsgo.version (CHANGELOG heading,
  source path, Go tree hash), whose hash the builder recomputes and refuses
  when stale (rerun setup.sh), plus the SHA-256 of its go.mod and go.sum,
  which that hash leaves out;
- the build command as run and its environment;
- its SHA-256, build settings and full `go version -m` output.

At startup the runner refuses to measure (exit 1) unless bin/manifest.json
exists, every binary's SHA-256 and its own match it, every binary was built
with the pinned toolchain and target, and the three scene programs were
built alike (toolchain, environment, flags, settings, stripping).
`runner -verify` runs just this check. `-unverified-runner` skips only the
runner's own SHA-256 (to allow `go run .`): `meta.adoptable` is false and
every verdict starts `UNVERIFIED RUNNER (not adoptable)`. A dirty build
(`meta.dirty_build`) is allowed but every verdict ends `(dirty build)`.

After verifying, the runner copies the four binaries into a private
directory of its own (0700, under the temp directory), hashing while
copying against the manifest, and launches only those copies, with the
same file names: a build.sh run during a long measurement cannot reach
later rounds. Each launch hashes its copy again and records that SHA-256
in the round's `build`; a copy that differs fails the round. The directory
is removed on exit, also after Ctrl-C or SIGTERM (once the scene programs
are stopped) and after a fatal error. Raising the toolchain means changing
`pinnedToolchain` in runner/manifest.go and build.sh together, then
rebuilding and re-measuring all three frameworks.

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
The nearest-counterpart scenes (Task 11) likewise: where an opponent has
no such component, its composition follows its own documentation (MyGo's
drawer is the one docs/ui/overlays.md shows), not any benchmark source.
