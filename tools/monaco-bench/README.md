# monaco-bench — Monaco 0.56 same-scenario baseline

Runs the spec 6.6 performance scenarios of the Tango editor
(`ui/editor/perf_test.go`) against Monaco 0.56 in headless Microsoft Edge, on
the same generated documents, so the two can be put in one table.
No npm packages, no network, no browser download: Node's built-in `http`
and `WebSocket` drive the locally installed Edge over the Chrome DevTools
Protocol (CDP).

| file | role |
|---|---|
| `server.mjs` | static server on `127.0.0.1` (random port): `page.html`, Monaco's AMD build `<monaco>/min/vs` at `/monaco/vs`, `package.json` at `/monaco/package.json`, the perf documents at `/data/<name>`. Sends COOP/COEP so the page is cross-origin isolated (`performance.now()` at 5 µs instead of 100 µs). |
| `page.html` | loads `vs/loader.js` + `vs/editor/editor.main` (which installs its own `MonacoEnvironment.getWorker`, same-origin blob workers) and exposes `window.bench.*` (open, prepareType, scroll, forceTokenizeDetached, waitBgDone, …). |
| `bench.mjs` | starts the server and one Edge, runs scenarios, gates runs on load, writes JSON. |

## Usage

```sh
node tools/monaco-bench/bench.mjs --self-test          # exit 0 when Monaco 0.56.0 loads and renders
node tools/monaco-bench/bench.mjs --scenario all --runs 5 --out results.json
node tools/monaco-bench/bench.mjs --variant nolfo --scenario open,type,scroll,memory --runs 5 --out nolfo.json
```

Options: `--monaco <monaco-editor dir>` (default
`/Users/admin/min/mincode/vue-app/node_modules/monaco-editor`), `--edge <binary>`
(default `/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge`),
`--data <dir>` (default `$TMPDIR/tango-editor-perf`), `--scenario
open|type|scroll|ts20k|memory|all` (comma list allowed), `--runs N` (accepted
runs per scenario, default 5), `--variant default|nolfo`, `--max-load 6`,
`--load-wait-min 30`, `--capped` (keep Chromium's 60 Hz frame cap),
`--keys 300`, `--frames 300`, `--screenshot out.png`, `-v`.

The documents are the Tango benchmark cache: `gen1000000-15d1551d.ts`
(100.5 MB, 1 000 000 lines) and `gen20000-15d1551d.ts` (the first 20 000
lines of it, byte-identical). If missing, generate them with the Tango
generator (a child test, no benchmark run):

```sh
D=$TMPDIR/tango-editor-perf; mkdir -p $D
TANGO_EDITOR_PERF_GEN=1000000:$D/gen1000000-15d1551d.ts go test ./ui/editor -run '^TestPerfGenFile$' -count=1
TANGO_EDITOR_PERF_GEN=20000:$D/gen20000-15d1551d.ts     go test ./ui/editor -run '^TestPerfGenFile$' -count=1
```

(The hash in the name is FNV-32a of `perfTemplates`; if the templates change, use the new name with `--file`.)

## Browser

One Edge at a time, relaunched for every run (fresh `--user-data-dir` in `$TMPDIR/monaco-bench-*`, so every run gets a new renderer and V8 isolate),
killed (process group SIGKILL) and removed on exit, error, SIGINT/SIGTERM/SIGHUP.
Flags:

```
--headless=new --remote-debugging-port=0 --window-size=1920,1080 --force-device-scale-factor=2
--no-first-run --no-default-browser-check --disable-extensions
--disable-background-timer-throttling --disable-renderer-backgrounding --disable-backgrounding-occluded-windows
--disable-background-networking --disable-component-update --disable-sync --disable-default-apps --no-pings --metrics-recording-only
--disable-frame-rate-limit --disable-gpu-vsync          (omitted with --capped)
--user-data-dir=<fresh tmp dir>
```

The DevTools port is read from `<user-data-dir>/DevToolsActivePort`. The
headless window's viewport is smaller than `--window-size` (1912×996 seen),
so the page is pinned to 1920×1080 @2x with `Emulation.setDeviceMetricsOverride`;
`Emulation.setFocusEmulationEnabled` keeps the page focused for keyboard input.

## Scenarios (Tango counterpart in parentheses)

All times are `performance.now()` in the page. "End of frame" = the next
`requestAnimationFrame` callback, then a `MessageChannel` task posted from it:
the frame's main-thread work (Monaco's render, style, layout, paint record,
commit) is done; **raster/GPU composition run on other threads/processes and
are not included** (Tango's numbers include waiting for the GPU).

- **open** (`BenchmarkOpen100MB`): the 100 MB text is fetched first (`fetchMs`,
  reported, not counted); then `monaco.editor.createModel` + `monaco.editor.create`
  → `editableMs` at the end of the first frame with view lines in the DOM;
  `tokensMs` at the end of the first frame whose visible lines all have accurate
  tokens (`model.tokenization.hasAccurateTokensForLine`) and the DOM shows more
  than one token class. With Monaco's defaults the 100 MB model is "too large for
  tokenization" (`largeFileOptimizations`: > 20 MB or > 300 k lines), so
  `tokensMs` is null — the degraded state is recorded per run (`degrade`).
- **type** (`BenchmarkTypeAt1MLines/mid`): caret at line 500 001 (Tango's
  0-based 500 000), editor focused; 300 keys through CDP
  `Input.dispatchKeyEvent` (keyDown with text + keyUp), `x` except every 40th
  key Enter (`keyAt`). Sample = window capture-phase `keydown` → end of the
  first frame after the model's content change (`p50/p95`: the latency a user
  gets, including the wait for the next frame tick — Chromium starts frames on
  its ~16.7 ms BeginFrame clock when the page was idle, even uncapped);
  `workP50/workP95` = keydown → model change, plus frame start (or the change,
  if later) → end of frame: the editor's own main-thread cost without the tick
  wait, the closer analogue of Tango's event→frame time. p50/p95/max with Tango's
  percentile rule (`sorted[ceil(p·n)−1]`). Line count is checked (+7).
- **scroll** (`BenchmarkScroll/scale2`): caret and top at line 100 001; for
  300 frames each rAF callback does `editor.setScrollTop(top + 2·lineHeight)`.
  fps = 1000 / mean rAF-to-rAF interval, p99 of the intervals. With the frame
  cap removed, rAF runs as soon as the previous frame is produced, so this is
  throughput (no display vsync) like Tango's. `idleIntervalMedian` is the rAF
  interval of the idle editor (≈16.7 ms: idle frames are not spun).
- **ts20k** (`BenchmarkHighlightTS20kBackground` / `…Alone`): open the 20 000-line
  file (Monarch TypeScript tokenizer; Monaco default, below every large-file
  limit; `maxTokenizationLineLength` 20 000 does not apply, lines are ~100 chars).
  `bgFromCreateMs` = from createModel until the background tokenizer finished
  (`backgroundTokenizationState === Completed` or the last line accurate);
  Monaco tokenizes in `requestIdleCallback` slices on the main thread.
  `forceMs` = synchronous `model.tokenization.forceTokenization(lineCount)` on a
  model with no editor attached (so the background tokenizer never ran).
  Then a fresh open with typing at line 11 one key per ≥ 16 ms until
  tokenization finished (`duringP50/P95`), plus 100 keys after (`afterP95`).
  Note: the TypeScript language worker (tsMode) also starts for this model.
- **memory** (`BenchmarkMemory100MB`): (fresh Edge per sample, as every run.) After GC
  (`HeapProfiler.collectGarbage` ×2) before fetching and after opening + 10
  frames: `Runtime.getHeapUsage` `usedSize` delta (`heapOpenMB`) and
  `backingStorageSize` delta (`backingOpenMB` — V8 keeps the big text and
  Monaco's chunks as external strings, so they are here, not in usedSize),
  `jsAllOpenMB` = used + embedder + backing delta; the renderer process (the
  largest-RSS `--type=renderer` child of our profile) `ps` RSS and
  `footprint(1)` phys_footprint deltas. With tokenization on (`--variant nolfo`)
  also after the background tokenizer finished (`full`).

`--variant nolfo` creates the model with `largeFileOptimizations: false`
(set through a throwaway `editor.create` because the standalone model service
reads it from configuration), i.e. Monaco with highlighting on like Tango.

## Load

Before every run the 1-minute load average must be < `--max-load` (6); the
tool waits in 15 s steps, at most `--load-wait-min` minutes per invocation, then
stops and marks the output `pending: true`. A run is accepted only if the load
was below the limit before and after it; rejected runs are kept in the JSON
with their load and `uptime` lines.
