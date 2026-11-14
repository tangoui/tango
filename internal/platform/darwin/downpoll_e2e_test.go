//go:build darwin && !ios

package darwin_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/automation"
)

// End-to-end tests of the down:poll downstream transport on the real
// WKWebView (docs/superpowers/specs/2026-11-14-down-poll-design.md §5, §6):
// examples/duplex with TANGO_IPC_LANES_DARWIN=1 (both lanes), its test hooks
// (TANGO_SCHEME_PROBE=1: Debug.Stats with the scheme handler's poll request
// counters, and the Debug.* hooks of examples/duplex/debughooks.go, which
// Send and Eval on the calling page's Surface from Go).
//
//	TANGO_E2E=1 go test -timeout 900s ./internal/platform/darwin -run 'Poll|EvalOrdered' -v
//
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the waits (not the bounds the
// design sets: 2 s for an occluded page, 1 s for a reload to settle).

// pollEnv is the environment of the down:poll tests.
var pollEnv = []string{"TANGO_SCHEME_PROBE=1", "TANGO_IPC_LANES_DARWIN=1", "TANGO_IPC_LANES=", "TANGO_IPC_LANE_FAULT="}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1).
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}

// launchPoll starts examples/duplex with pollEnv plus extra (later values
// win) and waits until the page has down:poll and up:post.
func launchPoll(t *testing.T, extra ...string) (*automation.App, *automation.Window) {
	t.Helper()
	return launchPollArgs(t, nil, extra...)
}

func launchPollArgs(t *testing.T, args []string, extra ...string) (*automation.App, *automation.Window) {
	t.Helper()
	app, win := launchLanesArgs(t, args, append(append([]string{}, pollEnv...), extra...)...)
	app.SetTimeout(scaled(120 * time.Second))
	return app, win
}

// waitPolling waits until the page's down lane is down:poll.
func waitPolling(t *testing.T, win *automation.Window) {
	t.Helper()
	win.WaitJS(`tango.lanes().down === "down:poll"`)
}

// jsErr is how the page scripts report a stream error.
type jsErr struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

// pullJS reads a Source stream of frames byte frames of size bytes from
// Go and checks them (Source writes byte i = i % 256 in every frame; every
// 251st byte and the last are checked). hook(k), if given, runs (sync)
// after frame k arrived. Defines window.__pull.
const pullJS = `window.__pull = window.__pull || (async (frames, size, hook) => {
  const r = { frames: 0, bytes: 0, bad: 0, err: null, ms: 0 };
  const t0 = performance.now();
  try {
    for await (const b of tango.stream("Source", frames, size)) {
      if (!(b instanceof Uint8Array) || b.length !== size) r.bad++;
      else {
        for (let i = 0; i < size; i += 251) if (b[i] !== (i & 255)) { r.bad++; break; }
        if (b[size - 1] !== ((size - 1) & 255)) r.bad++;
      }
      r.bytes += b.length;
      if (hook) hook(r.frames);
      r.frames++;
    }
  } catch (e) {
    r.err = { message: String((e && e.message) || e), code: (e && e.code) || "" };
  }
  r.ms = Math.round(performance.now() - t0);
  return r;
});`

type pullResult struct {
	Frames int    `json:"frames"`
	Bytes  int    `json:"bytes"`
	Bad    int    `json:"bad"`
	Err    *jsErr `json:"err"`
	MS     int    `json:"ms"`
}

func (r pullResult) ok(frames, size int) bool {
	return r.Err == nil && r.Bad == 0 && r.Frames == frames && r.Bytes == frames*size
}

// readPollStats is readStats plus the poll counters it does not check.
func readPollStats(t *testing.T, win *automation.Window) map[string]int64 {
	t.Helper()
	m := readStats(t, win)
	if _, ok := m["downRefused"]; !ok {
		t.Fatalf("Debug.Stats has no downRefused: %v", m)
	}
	return m
}

func statDelta(a, b map[string]int64, k string) int64 { return b[k] - a[k] }

// TestPollAdvertised: TANGO_IPC_LANES_DARWIN=1 gives the page down:poll
// and up:post.
func TestPollAdvertised(t *testing.T) {
	app, win := launchPoll(t)
	waitPolling(t, win)
	if l := settledLanes(t, win); l != (lanes{"down:poll", "up:post"}) {
		t.Fatalf("lanes %+v, want {down:poll up:post}", l)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestEvalOrderedWithSend: Go (an ipc handler's goroutine, not the main
// thread) makes 200 rounds of Send("a:i"), Eval(script pushing "b:i"),
// Send("c:i"); the script of round 50 throws after it pushed. The page sees
// exactly a0 b0 c0 a1 b1 c1 ...: every Eval ran at its place between the
// messages (design 5.9 barriers), and the throw held nothing up. The Evals
// were barriers of the poll lane: poll requests were answered meanwhile.
func TestEvalOrderedWithSend(t *testing.T) {
	app, win := launchPoll(t)
	waitPolling(t, win)
	settledLanes(t, win)
	before := readPollStats(t, win)
	const rounds, throwAt = 200, 50
	var log []string
	win.EvalInto(`(async () => {
	  window.__dbgLog = [];
	  __tango.onMessage((m) => { if (/^[ac]:/.test(m)) __dbgLog.push(m); });
	  await tango.call("Debug.SendEval", `+strconv.Itoa(rounds)+`, `+strconv.Itoa(throwAt)+`);
	  for (const t0 = Date.now(); __dbgLog.length < `+strconv.Itoa(3*rounds)+` && Date.now() - t0 < `+
		strconv.Itoa(int(scaled(30*time.Second)/time.Millisecond))+`; ) await new Promise((r) => setTimeout(r, 20));
	  return __dbgLog;
	})()`, &log)
	after := readPollStats(t, win)
	var want []string
	for i := range rounds {
		n := strconv.Itoa(i)
		want = append(want, "a:"+n, "b:"+n, "c:"+n)
	}
	if len(log) != len(want) {
		t.Fatalf("page saw %d entries, want %d; first %v", len(log), len(want), log[:min(len(log), 12)])
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("entry %d is %q, want %q; around it: %v", i, log[i], want[i], log[max(0, i-4):min(len(log), i+5)])
		}
	}
	if d := statDelta(before, after, "downConns"); d < 1 {
		t.Errorf("no poll request answered during the rounds (downConns +%d): the Evals did not ride the poll lane", d)
	}
	t.Logf("poll requests answered during the rounds: %d", statDelta(before, after, "downConns"))
	if got := win.EvalString(`window.duplex.echoOnce("after")`); got != "after" {
		t.Errorf("echo after the rounds: %q", got)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestPollBurstNoLoss: 64 MiB downstream (Source, 1024 frames of 64 KiB)
// five times in a row on one page: every run complete and intact, no
// stream error, the frames mostly riding poll responses (at least 8 batches
// answered per run: a batch holds at most 4 MiB), down:poll still the lane
// at the end.
func TestPollBurstNoLoss(t *testing.T) {
	app, win := launchPoll(t)
	waitPolling(t, win)
	settledLanes(t, win)
	win.Eval(pullJS)
	const frames, size = 1024, 64 << 10
	for run := 1; run <= 5; run++ {
		before := readPollStats(t, win)
		var r pullResult
		win.EvalInto(fmt.Sprintf(`__pull(%d, %d)`, frames, size), &r)
		after := readPollStats(t, win)
		conns, writes := statDelta(before, after, "downConns"), statDelta(before, after, "downWrites")
		t.Logf("run %d: %+v; poll responses %d, data blocks %d", run, r, conns, writes)
		if !r.ok(frames, size) {
			t.Fatalf("run %d: %+v", run, r)
		}
		if conns < 8 {
			t.Errorf("run %d: only %d poll responses for 64 MiB: the frames did not ride down:poll", run, conns)
		}
	}
	if l := win.EvalString(`tango.lanes().down`); l != "down:poll" {
		t.Errorf("down lane after the bursts: %q", l)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestPollStall: during a 16 MiB download (Source, 256 x 64 KiB) the
// page's main thread busy-loops for 6 s after frame 8. Go keeps writing
// meanwhile (the watchdog hands queued items to Eval, design 5.8). The
// stream completes intact with no transport error, down:poll is still the
// lane, and poll requests are answered again after the stall.
func TestPollStall(t *testing.T) {
	app, win := launchPoll(t)
	waitPolling(t, win)
	settledLanes(t, win)
	win.Eval(pullJS)
	const frames, size = 256, 64 << 10
	before := readPollStats(t, win)
	var r struct {
		pullResult
		BusyMS int   `json:"busyMs"`
		Lanes  lanes `json:"lanes"`
	}
	win.EvalInto(fmt.Sprintf(`(async () => {
	  let busy = 0;
	  const r = await __pull(%d, %d, (k) => {
	    if (k !== 8) return;
	    const t0 = Date.now();
	    while (Date.now() - t0 < 6000) {}
	    busy = Date.now() - t0;
	  });
	  r.busyMs = busy;
	  r.lanes = tango.lanes();
	  return r;
	})()`, frames, size), &r)
	mid := readPollStats(t, win)
	t.Logf("stall: %+v; poll responses %d", r, statDelta(before, mid, "downConns"))
	if !r.ok(frames, size) || r.BusyMS < 6000 {
		t.Fatalf("stall: %+v", r)
	}
	if r.Lanes.Down != "down:poll" {
		t.Errorf("down lane after the stall: %q", r.Lanes.Down)
	}
	// Polling resumed: a further download rides poll responses.
	var r2 pullResult
	win.EvalInto(fmt.Sprintf(`__pull(64, %d)`, size), &r2)
	after := readPollStats(t, win)
	if !r2.ok(64, size) {
		t.Errorf("download after the stall: %+v", r2)
	}
	if d := statDelta(mid, after, "downConns"); d < 1 {
		t.Errorf("no poll request answered after the stall (downConns +%d)", d)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestPollOccluded: Go minimizes the window, waits 1.5 s, sends 1000
// messages and restores the window 4 s later. All 1000 reach the page in
// order within 2 s of the first Send (through poll responses or, when the
// page does not poll, the watchdog's Eval), and after the restore messages
// ride poll responses again.
func TestPollOccluded(t *testing.T) {
	app, win := launchPoll(t)
	waitPolling(t, win)
	settledLanes(t, win)
	if m := win.EvalString(`tango.call("Debug.WindowState").then(s => String(s.minimized))`); m != "false" {
		t.Fatalf("window minimized before the test: %s", m)
	}
	win.Eval(`(() => {
	  window.__occ = []; window.__vis = [];
	  __tango.onMessage((m) => {
	    if (!m.startsWith("o:")) return;
	    if (!__occ.length) __vis.push(document.visibilityState);
	    __occ.push([m, Date.now()]);
	  });
	  return true;
	})()`)
	before := readPollStats(t, win)
	const n = 1000
	var res struct {
		T0 int64 `json:"t0"`
		T1 int64 `json:"t1"`
	}
	win.EvalInto(`tango.call("Debug.OccludedBurst", `+strconv.Itoa(n)+`, 1500, 4000)`, &res)
	var got struct {
		Msgs [][2]json.RawMessage `json:"msgs"`
		Vis  []string             `json:"vis"`
		Now  string               `json:"now"`
		Min  bool                 `json:"min"`
	}
	win.EvalInto(`tango.call("Debug.WindowState").then(s => ({ msgs: __occ, vis: __vis, now: document.visibilityState, min: s.minimized }))`, &got)
	during := readPollStats(t, win)
	t.Logf("occluded: sent %d in %d ms; page visibility at the first message %v, now %s; poll responses meanwhile %d",
		n, res.T1-res.T0, got.Vis, got.Now, statDelta(before, during, "downConns"))
	if !got.Min {
		t.Errorf("the window was not minimized after the burst")
	}
	if len(got.Msgs) != n {
		t.Fatalf("page got %d of %d messages", len(got.Msgs), n)
	}
	var last int64
	for i, m := range got.Msgs {
		var s string
		var at int64
		if json.Unmarshal(m[0], &s) != nil || json.Unmarshal(m[1], &at) != nil {
			t.Fatalf("message %d: %s %s", i, m[0], m[1])
		}
		if s != "o:"+strconv.Itoa(i) {
			t.Fatalf("message %d is %q", i, s)
		}
		last = max(last, at)
	}
	t.Logf("occluded: last message %d ms after the first Send", last-res.T0)
	if d := last - res.T0; d > 2000 {
		t.Errorf("the last message arrived %d ms after the first Send, want <= 2000", d)
	}
	// After the restore (4 s after the first Send): messages ride poll
	// responses again.
	for deadline := time.Now().Add(scaled(15 * time.Second)); ; time.Sleep(200 * time.Millisecond) {
		if win.EvalString(`tango.call("Debug.WindowState").then(s => String(s.minimized))`) == "false" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the window was not restored")
		}
	}
	mid := readPollStats(t, win)
	var cnt int
	win.EvalInto(`(async () => {
	  let k = 0;
	  const off = __tango.onMessage((m) => { if (m.startsWith("r:")) k++; });
	  await tango.call("Debug.Burst", 200, "r:");
	  for (const t0 = Date.now(); k < 200 && Date.now() - t0 < 10000; ) await new Promise((r) => setTimeout(r, 20));
	  off();
	  return k;
	})()`, &cnt)
	after := readPollStats(t, win)
	if cnt != 200 {
		t.Errorf("after the restore the page got %d of 200 messages", cnt)
	}
	if d := statDelta(mid, after, "downConns"); d < 1 {
		t.Errorf("no poll request answered after the restore (downConns +%d)", d)
	}
	if l := win.EvalString(`tango.lanes().down`); l != "down:poll" {
		t.Errorf("down lane after the restore: %q", l)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestPollReloadNoLeak reloads the page while a download (Source, not
// ending on its own) rides poll responses. Go samples its goroutines and
// the open poll requests every 10 ms across the reload (Debug.Sample, which
// outlives the page): from 1 s after the reload on, the goroutines are
// within ±5 of the idle baseline and at most one poll request is open (the
// old generation's held request and the stream are gone; the new page's
// own request may be open, and is at some point). The new page's lanes are
// down:poll and up:post again and its downstream works.
func TestPollReloadNoLeak(t *testing.T) {
	app, win := launchPoll(t)
	waitPolling(t, win)
	settledLanes(t, win)
	win.Eval(`tango.call("Debug.Sample", ` + strconv.Itoa(int(scaled(30*time.Second)/time.Millisecond)) + `, 10)`)
	// Idle baseline (the sampler's goroutine included): one poll request
	// held, nothing else going.
	time.Sleep(500 * time.Millisecond)
	base := readPollStats(t, win)
	win.Eval(`(() => {
	  window.__got = 0;
	  (async () => {
	    try { for await (const b of tango.stream("Source", 1 << 30, 65536)) window.__got++; } catch (e) {}
	  })();
	  return true;
	})()`)
	win.WaitJS(`window.__got >= 64`)
	going := readPollStats(t, win)
	t.Logf("before the reload: goroutines %d (idle %d), poll responses so far %d", going["goroutines"], base["goroutines"], statDelta(base, going, "downConns"))
	if statDelta(base, going, "downConns") < 1 {
		t.Fatalf("the download does not ride poll responses: %v -> %v", base, going)
	}
	var reloadAt int64
	win.EvalInto(`(() => { window.__oldPage = true; const t = Date.now(); setTimeout(() => location.reload(), 0); return t; })()`, &reloadAt)
	// An Eval made before the old page goes is lost with it (its barrier
	// belongs to the old generation, design 5.9; before down:poll it ran
	// in the leaving page): give the reload time to start.
	time.Sleep(scaled(1500 * time.Millisecond))
	win.WaitJS(`!window.__oldPage && document.readyState === "complete" && !!window.tango && !!window.duplex`)
	waitPolling(t, win)
	if l := settledLanes(t, win); l != (lanes{"down:poll", "up:post"}) {
		t.Errorf("new page lanes %+v", l)
	}
	// A while without Evals (each is a barrier, which pauses polling until
	// its release): the new page's poll request is held meanwhile.
	time.Sleep(time.Second)
	type sample struct {
		At         int64 `json:"at"`
		Goroutines int64 `json:"goroutines"`
		DownOpen   int64 `json:"downOpen"`
	}
	var ss []sample
	win.EvalInto(`tango.call("Debug.Samples")`, &ss)
	var after []sample
	peakG, peakOpen := int64(0), int64(0)
	for _, s := range ss {
		if s.At >= reloadAt {
			peakG, peakOpen = max(peakG, s.Goroutines), max(peakOpen, s.DownOpen)
		}
		if s.At >= reloadAt+1000 {
			after = append(after, s)
		}
	}
	t.Logf("samples: %d, %d of them from 1 s after the reload; after the reload goroutines peaked at %d, open poll requests at %d",
		len(ss), len(after), peakG, peakOpen)
	if len(after) == 0 {
		t.Fatalf("no samples from 1 s after the reload on (%d samples)", len(ss))
	}
	polled := false
	for _, s := range after {
		if d := s.Goroutines - base["goroutines"]; d > 5 || d < -5 || s.DownOpen > 1 {
			t.Errorf("%d ms after the reload: goroutines %d (idle %d), open poll requests %d", s.At-reloadAt, s.Goroutines, base["goroutines"], s.DownOpen)
			break
		}
		polled = polled || s.DownOpen == 1
	}
	if !polled {
		t.Errorf("the new page never had a poll request open")
	}
	win.Eval(pullJS)
	var p pullResult
	win.EvalInto(`__pull(64, 65536)`, &p)
	if !p.ok(64, 64<<10) {
		t.Errorf("download on the new page: %+v", p)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestPollCSPBlocked: a page whose CSP connect-src excludes the asset
// origin (/csp.html) cannot poll. The page reports the failed fetch, the
// core keeps the generation on Eval, and downstream works as before: a
// call, a 16 x 64 KiB echo and a 4 MiB download complete; no poll request
// ever reached the scheme handler.
func TestPollCSPBlocked(t *testing.T) {
	app, win := launchPollArgs(t, []string{"-page", "csp.html"})
	if p := win.EvalString(`location.pathname`); p != "/csp.html" {
		t.Fatalf("page %q, want /csp.html", p)
	}
	if got := win.EvalString(`fetch("/index.html", {cache: "no-store"}).then(() => "allowed", e => "refused: " + e.name)`); !strings.HasPrefix(got, "refused") {
		t.Fatalf("connect-src not enforced on /csp.html: fetch %s", got)
	}
	before := readPollStats(t, win)
	l := settledLanes(t, win)
	t.Logf("lanes under the CSP: %+v", l)
	if l.Up != "fallback" {
		t.Errorf("up lane under a blocking CSP: %q", l.Up)
	}
	if got := win.EvalString(`window.duplex.echoOnce("csp")`); got != "csp" {
		t.Errorf("call under the CSP: %q", got)
	}
	var e echoResult
	win.EvalInto(laneEcho, &e)
	if e.Err != "" || e.Frames != 16 || e.Bytes != 16<<16 || e.Bad != 0 {
		t.Errorf("echo under the CSP: %+v", e)
	}
	win.Eval(pullJS)
	var p pullResult
	win.EvalInto(`__pull(64, 65536)`, &p)
	if !p.ok(64, 64<<10) {
		t.Errorf("download under the CSP: %+v", p)
	}
	after := readPollStats(t, win)
	t.Logf("CSP: echo %+v, download %+v, poll requests %d answered %d", e, p,
		statDelta(before, after, "downRequests"), statDelta(before, after, "downConns"))
	if after["downRequests"] != 0 || after["downConns"] != 0 {
		t.Errorf("poll requests reached the scheme handler under the CSP: %v", after)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestPollFaultNoLoss: tango_dev build (automation adds the tag) with
// TANGO_IPC_LANE_FAULT=down:poll:3: the poll response carrying the 4th
// byte packet is cut off mid-item (internal/downq fault.go). The page
// cannot decode it, reports the failure, and the core sends every
// unacknowledged item again by Eval. Two streams run across the cut: an
// Echo of 32 distinct 64 KiB frames (each checked byte for byte at its
// place: a lost, repeated or reordered frame fails) and a 64 x 64 KiB
// download. Both complete intact with no error, and the page keeps working.
func TestPollFaultNoLoss(t *testing.T) {
	app, win := launchPoll(t, "TANGO_IPC_LANE_FAULT=down:poll:3")
	waitPolling(t, win)
	settledLanes(t, win)
	win.Eval(pullJS)
	var r struct {
		A struct {
			Frames, Bytes, Bad int
			FirstBad           int
			Err                *jsErr
		}
		B pullResult
	}
	win.EvalInto(`(async () => {
	  const size = 64 * 1024, n = 32;
	  const echo = (async () => {
	    const r = { frames: 0, bytes: 0, bad: 0, firstBad: -1, err: null };
	    try {
	      const s = tango.stream("Echo");
	      await s.accepted;
	      const send = (async () => {
	        for (let k = 0; k < n; k++) {
	          const b = new Uint8Array(size);
	          for (let i = 0; i < size; i++) b[i] = (i + 7 * k) % 251;
	          await s.sendBytes(b);
	        }
	        s.end();
	      })();
	      for await (const m of s) {
	        const k = r.frames++;
	        let ok = m instanceof Uint8Array && m.length === size;
	        for (let i = 0; ok && i < size; i++) if (m[i] !== (i + 7 * k) % 251) ok = false;
	        if (!ok) { r.bad++; if (r.firstBad < 0) r.firstBad = k; }
	        r.bytes += m.length;
	      }
	      await send;
	    } catch (e) {
	      r.err = { message: String((e && e.message) || e), code: (e && e.code) || "" };
	    }
	    return r;
	  })();
	  const [a, b] = await Promise.all([echo, __pull(64, size)]);
	  return { a, b };
	})()`, &r)
	t.Logf("fault: echo %+v, download %+v", r.A, r.B)
	if r.A.Err != nil || r.A.Frames != 32 || r.A.Bytes != 32<<16 || r.A.Bad != 0 {
		t.Errorf("echo across the fault: %+v", r.A)
	}
	if !r.B.ok(64, 64<<10) {
		t.Errorf("download across the fault: %+v", r.B)
	}
	if !strings.Contains(app.Output(), "injected fault: poll response cut off") {
		t.Errorf("the fault was not injected (no log line); output:\n%s", app.Output())
	}
	if got := win.EvalString(`window.duplex.echoOnce("after")`); got != "after" {
		t.Errorf("echo after the fault: %q", got)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestPollSubframeNoToken: frames without the token (it is only sent to
// the main frame, by Eval) cannot read the main page's messages through
// /__tango/lane/poll: a same-origin frame and two opaque-origin frames
// guess generations 0..5 with no or made-up tokens (attackScript). Every
// request of the same-origin frame is answered 403 (it can read its own
// statuses) and the scheme handler counted the refusals; no opaque frame
// gets a readable 200. The main page receives every message sent around
// the attacks, in order.
func TestPollSubframeNoToken(t *testing.T) {
	app, win := launchPoll(t)
	waitPolling(t, win)
	settledLanes(t, win)
	burst := func(when string) {
		t.Helper()
		var bad string
		win.EvalInto(`(async () => {
		  const got = [];
		  const off = __tango.onMessage((m) => { if (m.startsWith("s:")) got.push(m); });
		  await tango.call("Debug.Burst", 300, "s:");
		  for (const t0 = Date.now(); got.length < 300 && Date.now() - t0 < 10000; ) await new Promise((r) => setTimeout(r, 20));
		  off();
		  for (let i = 0; i < 300; i++) if (got[i] !== "s:" + i) return "got " + got.length + ", entry " + i + " is " + got[i];
		  return got.length === 300 ? "" : "got " + got.length;
		})()`, &bad)
		if bad != "" {
			t.Errorf("main page messages %s: %s", when, bad)
		}
	}
	burst("before the frames")
	frames := []struct{ tag, mk string }{
		{"sandboxed", `(code) => {
		  const f = document.createElement("iframe");
		  f.setAttribute("sandbox", "allow-scripts");
		  f.srcdoc = "<!doctype html><script>" + code + "<\/script>";
		  return f;
		}`},
		{"data", `(code) => {
		  const f = document.createElement("iframe");
		  f.src = "data:text/html;charset=utf-8," + encodeURIComponent("<!doctype html><script>" + code + "<\/script>");
		  return f;
		}`},
		{"same", `(code) => {
		  const f = document.createElement("iframe");
		  f.src = "/iframe.html#" + encodeURIComponent(code);
		  return f;
		}`},
	}
	for _, f := range frames {
		s0 := readPollStats(t, win)
		rep := runFrame(t, win, "poll-"+f.tag, f.mk)
		win.EvalInto(`new Promise(r => setTimeout(() => r(true), 1000))`, new(bool))
		s1 := readPollStats(t, win)
		statuses := map[string]int{}
		polls := 0
		for _, r := range rep.Results {
			if !strings.Contains(r.URL, "/__tango/lane/poll") {
				continue
			}
			polls++
			k := r.Mode + " " + r.Type + " " + r.Err
			if r.Err == "" {
				k += " " + strconv.Itoa(r.Status)
			}
			statuses[k]++
			if r.Status == 200 {
				t.Errorf("%s frame: poll %s (%s) answered 200 with %d bytes", f.tag, r.URL, r.Mode, r.N)
			}
			if f.tag == "same" && (r.Err != "" || r.Status != 403) {
				t.Errorf("same-origin frame: poll %s (%s): status %d err %q, want 403", r.URL, r.Mode, r.Status, r.Err)
			}
		}
		refused := statDelta(s0, s1, "downRefused")
		t.Logf("%s frame: tango=%s parent.tango=%s; %d poll requests: %v; scheme handler: %d poll requests, %d refused",
			f.tag, rep.Tango, rep.Parent, polls, statuses, statDelta(s0, s1, "downRequests"), refused)
		if polls != frameRequests/2 {
			t.Errorf("%s frame reported %d poll requests, want %d (its script did not run)", f.tag, polls, frameRequests/2)
		}
		if rep.Tango != "undefined" || rep.Native != "undefined" {
			t.Errorf("%s frame: the runtime is in the frame: tango=%s native=%s", f.tag, rep.Tango, rep.Native)
		}
		if f.tag == "same" && refused < frameRequests/2 {
			t.Errorf("same-origin frame: the scheme handler refused %d poll requests, want >= %d", refused, frameRequests/2)
		}
	}
	burst("after the frames")
	if l := win.EvalString(`tango.lanes().down`); l != "down:poll" {
		t.Errorf("main page down lane after the frames: %q", l)
	}
	noWebKitException(t, app)
	app.Quit()
}
