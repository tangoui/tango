//go:build darwin && !ios

package darwin_test

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/automation"
	"github.com/tangoui/tango/ipc"
)

// End-to-end tests of the IPC fast lanes on macOS (lanes.go): which lanes
// examples/duplex's page gets under the TANGO_IPC_LANES_DARWIN switch (on by
// default: up:post), whether a subframe can use them, and that a page whose
// CSP blocks the asset origin keeps the message channel. Every test sets the switches explicitly
// through automation.Options.Env (the app inherits the test's own
// environment otherwise).
//
//	TANGO_E2E=1 go test -timeout 600s ./internal/platform/darwin -run 'Lanes|Lane' -v

type lanes struct {
	Down string `json:"down"`
	Up   string `json:"up"`
}

// launchLanes starts examples/duplex with env and waits for its runtime.
func launchLanes(t *testing.T, env ...string) (*automation.App, *automation.Window) {
	t.Helper()
	return launchLanesArgs(t, nil, env...)
}

// launchLanesArgs is launchLanes with program arguments.
func launchLanesArgs(t *testing.T, args []string, env ...string) (*automation.App, *automation.Window) {
	t.Helper()
	app := automation.LaunchOptions(t, duplexApp(t), automation.Options{
		Args:    args,
		Env:     env,
		Timeout: 60 * time.Second,
	})
	win := app.Window("Tango Duplex")
	win.WaitJS(`document.readyState === "complete" && !!window.tango && !!window.duplex`)
	return app, win
}

// settledLanes waits until the hello reply surely arrived (an Echo round
// trip) and any offered lane had time to start, then reads tango.lanes().
func settledLanes(t *testing.T, win *automation.Window) lanes {
	t.Helper()
	if got := win.EvalString(`window.duplex.echoOnce("ping")`); got != "ping" {
		t.Fatalf("echo %q", got)
	}
	var l lanes
	win.EvalInto(`new Promise(r => setTimeout(() => r(tango.lanes()), 1500))`, &l)
	return l
}

// TestLanesAdvertised: with an explicit TANGO_IPC_LANES_DARWIN=1 (the same
// as the default) the page gets up:post only (WebKit holds back large down:scheme bursts, spec §6.2, so downstream
// frames stay on the message channel).
func TestLanesAdvertised(t *testing.T) {
	app, win := launchLanes(t, "TANGO_IPC_LANES_DARWIN=1", "TANGO_IPC_LANES=")
	win.WaitJS(`tango.lanes().up === "up:post"`)
	if l := settledLanes(t, win); l != (lanes{"fallback", "up:post"}) {
		t.Fatalf("lanes %+v, want {fallback up:post}", l)
	}
	app.Quit()
}

// TestLanesAdvertisedAll: the experimental TANGO_IPC_LANES_DARWIN=all also
// offers down:scheme. Advertisement only: its data flow is not reliable on
// WebKit (spec §6.2) and is not asserted here.
func TestLanesAdvertisedAll(t *testing.T) {
	app, win := launchLanes(t, "TANGO_IPC_LANES_DARWIN=all", "TANGO_IPC_LANES=")
	win.WaitJS(`(l => l.down === "down:scheme" && l.up === "up:post")(tango.lanes())`)
	app.Quit()
}

// TestLanesOnByDefault: with the switch unset (empty) macOS offers up:post.
func TestLanesOnByDefault(t *testing.T) {
	app, win := launchLanes(t, "TANGO_IPC_LANES_DARWIN=", "TANGO_IPC_LANES=")
	win.WaitJS(`tango.lanes().up === "up:post"`)
	if l := settledLanes(t, win); l != (lanes{"fallback", "up:post"}) {
		t.Fatalf("lanes %+v, want {fallback up:post} by default", l)
	}
	app.Quit()
}

// TestLanesSwitchedOff: TANGO_IPC_LANES_DARWIN=0 (or false/off) turns the
// default off.
func TestLanesSwitchedOff(t *testing.T) {
	app, win := launchLanes(t, "TANGO_IPC_LANES_DARWIN=0", "TANGO_IPC_LANES=")
	if l := settledLanes(t, win); l != (lanes{"fallback", "fallback"}) {
		t.Fatalf("lanes %+v, want fallback with TANGO_IPC_LANES_DARWIN=0", l)
	}
	app.Quit()
}

// TestLaneCSPBlocked: a page whose CSP connect-src excludes the asset
// origin cannot reach Go over up:post. runtime-js starts up:post with a
// probe POST (the empty packet), CSP refuses it, the lane fails at start
// and every frame rides the message channel: a 16 x 64 KiB echo completes
// without a transport error, nothing reaches OnBinary, lanes() says
// fallback. examples/duplex serves /csp.html with that header.
func TestLaneCSPBlocked(t *testing.T) {
	app, win := launchLanesArgs(t, []string{"-page", "csp.html"}, "TANGO_SCHEME_PROBE=1", "TANGO_IPC_LANES_DARWIN=", "TANGO_IPC_LANES=")
	if p := win.EvalString(`location.pathname`); p != "/csp.html" {
		t.Fatalf("page %q, want /csp.html", p)
	}
	// The CSP is in force: a fetch to the asset origin is refused.
	if got := win.EvalString(`fetch("/index.html", {cache: "no-store"}).then(() => "allowed", e => "refused: " + e.name)`); !strings.HasPrefix(got, "refused") {
		t.Fatalf("connect-src not enforced on /csp.html: fetch %s", got)
	}
	l := settledLanes(t, win)
	if l != (lanes{"fallback", "fallback"}) {
		t.Fatalf("lanes %+v under a blocking CSP, want fallback", l)
	}
	before := readStats(t, win)
	var r echoResult
	win.EvalInto(laneEcho, &r)
	after := readStats(t, win)
	d := delta(before, after)
	t.Logf("echo under CSP: %+v, counters %v", r, d)
	if r.Err != "" || r.Frames != 16 || r.Bytes != 16<<16 || r.Bad != 0 {
		t.Fatalf("echo under CSP: %+v", r)
	}
	if d["upFrames"] != 0 || d["upPackets"] != 0 || d["upRequests"] != 0 {
		t.Errorf("frames rode a lane under a blocking CSP: %v", d)
	}
	if l := win.EvalString(`tango.lanes().up`); l != "fallback" {
		t.Errorf("tango.lanes().up = %q after the echo, want fallback", l)
	}
	if out := app.Output(); strings.Contains(out, "NSInternalInconsistencyException") {
		t.Fatalf("WebKit exception:\n%s", out)
	}
	app.Quit()
}

func TestLanesKillSwitch(t *testing.T) {
	app, win := launchLanes(t, "TANGO_IPC_LANES_DARWIN=1", "TANGO_IPC_LANES=0")
	if l := settledLanes(t, win); l != (lanes{"fallback", "fallback"}) {
		t.Fatalf("lanes %+v, want fallback with TANGO_IPC_LANES=0", l)
	}
	app.Quit()
}

// laneCounters are the Debug.Stats fields this test reads
// (examples/duplex/services: Go-side counters the page cannot see).
var laneCounterNames = []string{
	"hellos",       // runtime hellos that reached Go's OnMessage
	"upPackets",    // packets delivered to OnBinary
	"upFrames",     // frames in them
	"downRequests", // /__tango/lane/down requests seen by the scheme handler
	"downConns",    // ... answered 200: accepted down connections
	"downActive",   // accepted ones still open
	"downWrites",   // data blocks handed to WebKit on accepted ones
	"upRequests",   // /__tango/lane/up requests seen by the scheme handler
	"upAccepted",   // ... answered 2xx
}

func readStats(t *testing.T, win *automation.Window) map[string]int64 {
	t.Helper()
	var m map[string]int64
	win.EvalInto(`tango.call("Debug.Stats")`, &m)
	for _, k := range laneCounterNames {
		if _, ok := m[k]; !ok {
			t.Fatalf("Debug.Stats has no %q: %v", k, m)
		}
	}
	return m
}

// attackScript is the code of an attacking frame. It is given the asset
// origin (BASE), a well-formed lane packet (PKT, base64) and the runtime
// protocol version, tries everything a frame without the token could, and
// posts what it saw to its parent as {tag, report}.
const attackScript = `(async () => {
  const BASE = %BASE%, PKT = Uint8Array.from(atob(%PKT%), c => c.charCodeAt(0)), TAG = %TAG%;
  const rep = { tango: typeof window.tango, native: typeof window.__tangoNative, origin: String(self.origin),
    parent: "", handler: "", results: [] };
  try { rep.parent = typeof parent.tango; } catch (e) { rep.parent = "throws: " + e.name; }
  try {
    webkit.messageHandlers.tango.postMessage(JSON.stringify({ tango: "hello", proto: %PROTO% }));
    rep.handler = "posted";
  } catch (e) { rep.handler = "throws: " + e.name; }
  const toks = ["", "0", "00000000000000000000000000000000"];
  const attempt = async (url, init) => {
    const ac = new AbortController();
    const timer = setTimeout(() => ac.abort(), 1500);
    try {
      const r = await fetch(url, Object.assign({ cache: "no-store", signal: ac.signal }, init));
      let n = -1;
      try { n = (await r.arrayBuffer()).byteLength; } catch (e) {}
      rep.results.push({ url, mode: init.mode || "cors", type: r.type, status: r.status, n });
    } catch (e) {
      rep.results.push({ url, mode: init.mode || "cors", err: e.name });
    } finally { clearTimeout(timer); }
  };
  const jobs = [];
  for (let gen = 0; gen <= 5; gen++) {
    for (const tok of toks) {
      const q = "?gen=" + gen + "&tok=" + encodeURIComponent(tok);
      for (const mode of ["cors", "no-cors"]) {
        jobs.push(attempt(BASE + "/__tango/lane/up" + q, { method: "POST", body: PKT.slice().buffer, mode }));
        jobs.push(attempt(BASE + "/__tango/lane/down" + q, { mode }));
      }
    }
  }
  await Promise.all(jobs);
  parent.postMessage({ tag: TAG, report: rep }, "*");
})();`

type attackReport struct {
	Tango   string `json:"tango"`
	Native  string `json:"native"`
	Origin  string `json:"origin"`
	Parent  string `json:"parent"`
	Handler string `json:"handler"`
	Results []struct {
		URL    string `json:"url"`
		Mode   string `json:"mode"`
		Type   string `json:"type"`
		Status int    `json:"status"`
		N      int    `json:"n"`
		Err    string `json:"err"`
	} `json:"results"`
}

func attackCode(base, tag string) string {
	pkt := ipc.AppendPacket(nil, ipc.PacketFrame{Stream: 1, Seq: 0, Payload: make([]byte, 64<<10)})
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	return strings.NewReplacer(
		"%BASE%", q(base), "%PKT%", q(base64.StdEncoding.EncodeToString(pkt)),
		"%TAG%", q(tag), "%PROTO%", "2",
	).Replace(attackScript)
}

// runFrame adds an iframe built by mk (JS: (code) => HTMLIFrameElement)
// carrying attackCode and waits for its report.
func runFrame(t *testing.T, win *automation.Window, tag, mk string) attackReport {
	t.Helper()
	base := win.EvalString(`location.protocol + "//" + location.host`)
	code := attackCode(base, tag)
	cq, _ := json.Marshal(code)
	var rep attackReport
	win.EvalInto(`new Promise((resolve, reject) => {
	  const tag = `+string(mustJSON(tag))+`;
	  const done = setTimeout(() => reject(new Error("no report from frame " + tag)), 30000);
	  addEventListener("message", function on(e) {
	    if (!e.data || e.data.tag !== tag) return;
	    removeEventListener("message", on);
	    clearTimeout(done);
	    resolve(e.data.report);
	  });
	  const f = (`+mk+`)(`+string(cq)+`);
	  f.dataset.tag = tag;
	  document.body.appendChild(f);
	})`, &rep)
	return rep
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// frameRequests is how many lane requests attackScript makes: 6
// generations x 3 tokens x 2 modes, each up and down.
const frameRequests = 6 * 3 * 2 * 2

// laneEcho waits for Go's accept (earlier byte frames take the message
// channel), then sends n byte frames of 64 KiB (byte i of frame k = (i+k) % 251)
// through Echo and checks every byte that comes back.
const laneEcho = `(async (n) => {
  const t0 = performance.now();
  const s = tango.stream("Echo");
  // Byte frames sent before Go accepted the stream go as messages.
  await s.accepted;
  const size = 64 * 1024;
  let k = 0, bad = 0, bytes = 0;
  try {
    for (let j = 0; j < n; j++) {
      const b = new Uint8Array(size);
      for (let i = 0; i < size; i++) b[i] = (i + j) % 251;
      await s.sendBytes(b);
    }
    s.end();
    for await (const m of s) {
      if (!(m instanceof Uint8Array) || m.length !== size) { bad++; k++; continue; }
      for (let i = 0; i < size; i++) if (m[i] !== (i + k) % 251) { bad++; break; }
      bytes += m.length;
      k++;
    }
  } catch (e) {
    return { frames: k, bytes, bad, err: String(e && e.message || e), ms: Math.round(performance.now() - t0) };
  }
  return { frames: k, bytes, bad, err: "", ms: Math.round(performance.now() - t0) };
})(16)`

type echoResult struct {
	Frames int    `json:"frames"`
	Bytes  int    `json:"bytes"`
	Bad    int    `json:"bad"`
	Err    string `json:"err"`
	MS     int    `json:"ms"`
}

func delta(a, b map[string]int64) map[string]int64 {
	d := map[string]int64{}
	for _, k := range laneCounterNames {
		d[k] = b[k] - a[k]
	}
	return d
}

// TestLaneSubframeAccess checks that only the main frame can use the
// lanes. Facts it relies on: the runtime is injected into the main frame
// only and the backend drops subframe script messages, so the lane token
// reaches the main frame's runtime only (in the hello reply). The lane
// requests themselves cannot be told apart by frame (WKURLSchemeTask has no
// frame), so the token is the only gate; the assertions are made on the Go
// side, since a frame cannot see cross-origin response statuses.
//
//  1. Opaque-origin frames (sandbox="allow-scripts" srcdoc; a data: URL):
//     guessed generations 0..5 with an empty or made-up token, up and down,
//     cors and no-cors. Nothing may reach OnBinary, no down connection may
//     be accepted, no hello may reach Go.
//  2. A same-origin frame (/iframe.html): parent.tango is reachable — by
//     design, it is application code. Its own hello, posted straight to
//     webkit.messageHandlers, must get nowhere (no new hello in Go, the main
//     page's lane generation unchanged: no new down connection), and its own
//     token-less lane requests are refused like the opaque frames'.
//  3. The main page's lane keeps working throughout (1 MiB echo, up through
//     up:post, before and after).
//
// Run with the shipped default (TANGO_IPC_LANES_DARWIN empty: up:post
// only). The down requests are still made: the core serves
// /__tango/lane/down whatever was offered, and refuses them for want of a
// token (and of a down:scheme offer).
//
// Every frame makes 6 generations x 3 tokens x 2 modes = 36 up and 36 down
// requests. The test asserts that they happened (the frame's report lists
// them; for the same-origin frame the scheme handler counted them), so
// that the "nothing reached Go" checks cannot pass because nothing ran.
func TestLaneSubframeAccess(t *testing.T) {
	app, win := launchLanes(t, "TANGO_SCHEME_PROBE=1", "TANGO_IPC_LANES_DARWIN=", "TANGO_IPC_LANES=")
	win.WaitJS(`tango.lanes().up === "up:post"`)
	// The main page's own hello reached Go (the counter works).
	if h := readStats(t, win)["hellos"]; h < 1 {
		t.Fatalf("main page hellos = %d, want >= 1", h)
	}

	echo := func(when string) {
		t.Helper()
		before := readStats(t, win)
		var r echoResult
		win.EvalInto(laneEcho, &r)
		after := readStats(t, win)
		d := delta(before, after)
		t.Logf("echo %s: %+v, counters %v", when, r, d)
		// Errorf, not Fatalf: the frame checks below must still run.
		if r.Err != "" || r.Frames != 16 || r.Bytes != 16<<16 || r.Bad != 0 {
			t.Errorf("echo %s: %+v", when, r)
		}
		// The upstream bytes went through up:post, not the message channel
		// (downstream has no lane: the echo comes back as messages).
		if d["upFrames"] < 8 {
			t.Errorf("echo %s did not use up:post: %v", when, d)
		}
	}
	echo("before")

	check := func(name string, rep attackReport, before map[string]int64) map[string]int64 {
		t.Helper()
		// Let late requests land before reading the counters.
		win.EvalInto(`new Promise(r => setTimeout(() => r(true), 1000))`, new(bool))
		after := readStats(t, win)
		d := delta(before, after)
		statuses := map[string]int{}
		for _, r := range rep.Results {
			k := r.Mode + " " + r.Type + " " + r.Err
			if r.Err == "" {
				k += " " + strconv.Itoa(r.Status)
			}
			statuses[k]++
		}
		t.Logf("%s: frame saw tango=%s native=%s origin=%s parent.tango=%s handler=%s; %d requests: %v",
			name, rep.Tango, rep.Native, rep.Origin, rep.Parent, rep.Handler, len(rep.Results), statuses)
		t.Logf("%s: Go-side counter deltas %v", name, d)
		for _, k := range []string{"hellos", "upPackets", "upFrames", "downConns", "downActive", "upAccepted"} {
			if d[k] != 0 {
				t.Errorf("%s: %s changed by %d (frame reached Go): %v", name, k, d[k], d)
			}
		}
		if rep.Tango != "undefined" || rep.Native != "undefined" {
			t.Errorf("%s: the runtime is in the frame: tango=%s native=%s", name, rep.Tango, rep.Native)
		}
		// The frame's script ran to the end: it made every request.
		if len(rep.Results) < frameRequests {
			t.Errorf("%s: the frame reported %d requests, want %d (its script did not run)", name, len(rep.Results), frameRequests)
		}
		return d
	}

	// 1. Opaque origins.
	s0 := readStats(t, win)
	rep := runFrame(t, win, "sandboxed", `(code) => {
	  const f = document.createElement("iframe");
	  f.setAttribute("sandbox", "allow-scripts");
	  f.srcdoc = "<!doctype html><script>" + code + "<\/script>";
	  return f;
	}`)
	d := check("sandboxed srcdoc", rep, s0)
	if rep.Parent == "object" {
		t.Errorf("sandboxed frame reached parent.tango")
	}
	t.Logf("sandboxed srcdoc: %d of its lane requests reached the scheme handler", d["downRequests"]+d["upRequests"])

	s0 = readStats(t, win)
	rep = runFrame(t, win, "data", `(code) => {
	  const f = document.createElement("iframe");
	  f.src = "data:text/html;charset=utf-8," + encodeURIComponent("<!doctype html><script>" + code + "<\/script>");
	  return f;
	}`)
	d = check("data: URL", rep, s0)
	if rep.Parent == "object" {
		t.Errorf("data: frame reached parent.tango")
	}
	t.Logf("data: URL: %d of its lane requests reached the scheme handler", d["downRequests"]+d["upRequests"])

	// 2. Same origin: the frame page runs the same attack (its hello goes
	// straight to webkit.messageHandlers).
	s0 = readStats(t, win)
	rep = runFrame(t, win, "same", `(code) => {
	  const f = document.createElement("iframe");
	  f.src = "/iframe.html#" + encodeURIComponent(code);
	  return f;
	}`)
	d = check("same-origin /iframe.html", rep, s0)
	if rep.Parent != "object" {
		t.Logf("same-origin frame: parent.tango is %q (expected reachable, by design)", rep.Parent)
	}
	// Its hello really was posted (and got nowhere: hellos unchanged above).
	if rep.Handler != "posted" {
		t.Errorf("same-origin frame: posting to the message handler: %s, want posted", rep.Handler)
	}
	// Its lane requests reached the scheme handler, so the token check is
	// what refused them.
	if d["upRequests"] < frameRequests/2 || d["downRequests"] < frameRequests/2 {
		t.Errorf("same-origin frame: the scheme handler saw %d up and %d down requests, want >= %d each",
			d["upRequests"], d["downRequests"], frameRequests/2)
	}

	// 3. The main page is unaffected.
	var l lanes
	win.EvalInto(`tango.lanes()`, &l)
	if l != (lanes{"fallback", "up:post"}) {
		t.Fatalf("main page lanes after the frames: %+v", l)
	}
	echo("after")
	if out := app.Output(); strings.Contains(out, "NSInternalInconsistencyException") {
		t.Fatalf("WebKit exception:\n%s", out)
	}
	app.Quit()
}
