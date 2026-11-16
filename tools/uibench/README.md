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

Scenes and their per-frame work: [scenes.md](scenes.md).

## Use

```sh
cd tools/uibench
./setup.sh /path/to/nsgo/source   # links third_party/nsgo, writes third_party/nsgo.version
./build.sh                        # bin/tango, bin/mygo, bin/nsgo, bin/runner, bin/build-info.txt
cd runner
GOWORK=off go test ./... -count=1
../bin/runner -bin ../bin -scenes counter,list,grid -frameworks tango,mygo,nsgo \
	-rounds 5 -settle 3s -seconds 10 -out ../results/$(date +%F).json
```

The runner opens real windows and needs an unlocked screen: behind a
locked screen the windows are occluded, Tango and MyGo (which draw on the
display's frame clock) draw nothing, and the runner marks every round
`screen_locked`, which the judge does not read. `bin/`, `third_party/` and
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

## Judge

`judge(tango, opponent)` reads only rounds that are ready, without error,
not disturbed and not screen-locked, and returns:

- `Invalid` when either side has no such round, a framework changed its
  renderer between rounds, or the work per frame (work/s ÷ fps) differs by
  more than 5%;
- `Faster` when Tango's median (process + WindowServer) CPU is at most half
  the opponent's, Tango's third quartile is below the opponent's first,
  median fps is at least the opponent's and median frame-interval p99 at
  most the opponent's (counter: footprint median at most the opponent's in
  place of fps and p99);
- `NotFaster` otherwise.

## Build modes

`build.sh` builds every program with `GOWORK=off CGO_ENABLED=0 go build
-trimpath -ldflags "-s -w"` and one toolchain (`GOTOOLCHAIN=go1.27.3`). A
framework that fails without cgo is rebuilt with `CGO_ENABLED=1`; the
mode goes to `bin/build-info.txt` and into every result's `build` field.

As of 2026-11-17 all three build with `CGO_ENABLED=0`.

## Not copied

MyGo and NSGo ship their own benchmark suites. This one borrows their ideas
(external rusage sampling, a ready line, frame intervals) and none of their
code; the scene programs use each framework's ordinary public API.
