//go:build darwin && !ios

package darwin_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/automation"
)

// End-to-end lane scenarios on macOS: examples/duplex with
// TANGO_IPC_LANES_DARWIN=1 (explicit; the same as the default: up:post
// only, spec §6.2) and the test hooks
// (TANGO_SCHEME_PROBE=1: Debug.Stats, /__test/lanetests.js). The scenarios
// themselves are examples/duplex/services/lanetests.js, which the iOS
// self-test (examples/duplex-ios) runs too; each returns its numbers, ok
// and why (the failed conditions), and the tests here assert on those and
// on the scheme handler's counters.
//
//	TANGO_E2E=1 go test -timeout 900s ./internal/platform/darwin -run 'LaneBig|LaneMixed|LaneReload|LaneStall|LaneFault' -v

// laneFlowEnv is the environment of the lane scenarios.
var laneFlowEnv = []string{"TANGO_SCHEME_PROBE=1", "TANGO_IPC_LANES_DARWIN=1", "TANGO_IPC_LANES=", "TANGO_IPC_LANE_FAULT="}

// launchLaneFlow starts examples/duplex with laneFlowEnv plus extra, waits
// for up:post to be ready and loads the scenarios.
func launchLaneFlow(t *testing.T, extra ...string) (*automation.App, *automation.Window) {
	t.Helper()
	app, win := launchLanes(t, append(append([]string{}, laneFlowEnv...), extra...)...)
	app.SetTimeout(120 * time.Second)
	loadLaneTests(t, win)
	return app, win
}

// loadLaneTests waits for up:post and loads /__test/lanetests.js.
func loadLaneTests(t *testing.T, win *automation.Window) {
	t.Helper()
	win.WaitJS(`tango.lanes().up === "up:post"`)
	win.Eval(`new Promise((res, rej) => {
	  if (window.laneTests) return res(true);
	  const s = document.createElement("script");
	  s.src = "/__test/lanetests.js";
	  s.onload = () => res(true);
	  s.onerror = () => rej(new Error("cannot load /__test/lanetests.js"));
	  document.head.appendChild(s);
	})`)
}

// laneResult is the common part of a scenario's result.
type laneResult struct {
	OK  bool     `json:"ok"`
	Why []string `json:"why"`
}

// runScenario evaluates laneTests.<call>, logs the raw result, decodes it
// into v (which embeds laneResult) and returns the raw JSON.
func runScenario(t *testing.T, win *automation.Window, call string, v any) json.RawMessage {
	t.Helper()
	raw := win.Eval(`laneTests.` + call)
	t.Logf("%s: %s", call, raw)
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("%s: result %s: %v", call, raw, err)
	}
	return raw
}

func checkScenario(t *testing.T, name string, r laneResult) {
	t.Helper()
	if !r.OK {
		t.Errorf("%s failed: %s", name, strings.Join(r.Why, "; "))
	}
}

func noWebKitException(t *testing.T, app *automation.App) {
	t.Helper()
	if out := app.Output(); strings.Contains(out, "NSInternalInconsistencyException") {
		t.Fatalf("WebKit exception:\n%s", out)
	}
}

// TestLaneBigBothWays: 8 MiB through Echo in 64 KiB frames, byte-exact;
// at least 64 of the 128 frames reached Go on up:post. The echo comes back
// on the message channel (no down lane on Apple platforms), complete and in
// order.
func TestLaneBigBothWays(t *testing.T) {
	app, win := launchLaneFlow(t)
	var r struct {
		laneResult
		Frames, Bytes, Bad, UpFrames, MS int
	}
	runScenario(t, win, "bigBothWays()", &r)
	checkScenario(t, "bigBothWays", r.laneResult)
	if r.Frames != 128 || r.Bytes != 8<<20 || r.Bad != 0 || r.UpFrames < 64 {
		t.Errorf("bigBothWays: frames %d bytes %d bad %d upFrames %d", r.Frames, r.Bytes, r.Bad, r.UpFrames)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestLaneMixedOrder: 40 frames alternating 100 B / 64 KiB on one stream
// come back in send order; exactly the 20 large ones rode up:post.
func TestLaneMixedOrder(t *testing.T) {
	app, win := launchLaneFlow(t)
	var r struct {
		laneResult
		Frames, Bad, UpFrames int
	}
	runScenario(t, win, "mixedOrder()", &r)
	checkScenario(t, "mixedOrder", r.laneResult)
	if r.Frames != 40 || r.Bad != 0 || r.UpFrames != 20 {
		t.Errorf("mixedOrder: frames %d bad %d upFrames %d", r.Frames, r.Bad, r.UpFrames)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestLaneStall: the page's main thread busy-loops for 6 s right after an
// up:post request of a 4 MiB Echo started; the stream still completes,
// byte-exact, with no transport error, and up:post is still the upstream
// lane after the stall (it did not fail and fall back silently).
func TestLaneStall(t *testing.T) {
	app, win := launchLaneFlow(t)
	var r struct {
		laneResult
		Frames, Bytes, Bad, UpFrames, BusyMS int
		Err                                  *struct{ Message, Code string }
		Lanes                                lanes
	}
	runScenario(t, win, "stall(6000)", &r)
	checkScenario(t, "stall", r.laneResult)
	if r.Err != nil || r.Frames != 64 || r.Bytes != 4<<20 || r.Bad != 0 || r.BusyMS < 6000 {
		t.Errorf("stall: %+v", r)
	}
	if r.Lanes.Up != "up:post" {
		t.Errorf("stall: up lane after the stall is %q, want up:post", r.Lanes.Up)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestLaneFault: tango_dev build (automation adds the tag) with
// TANGO_IPC_LANE_FAULT=up:post:3. Go refuses the fourth up:post data packet
// (the empty start probe is not counted);
// the page falls back to the message channel for later frames, the stream
// whose frame was lost fails with code transport, and a parallel stream is
// unaffected (laneTests.fault).
func TestLaneFault(t *testing.T) {
	app, win := launchLaneFlow(t, "TANGO_IPC_LANE_FAULT=up:post:3")
	var r struct {
		laneResult
		UpFrames int
		A, B     struct {
			Frames, Bad int
			Err         *struct{ Message, Code string }
		}
		LanesAfter lanes
	}
	runScenario(t, win, "fault()", &r)
	checkScenario(t, "fault", r.laneResult)
	if r.UpFrames != 3 || r.A.Err == nil || r.A.Err.Code != "transport" || r.B.Err != nil || r.B.Frames != 6 || r.LanesAfter.Up != "fallback" {
		t.Errorf("fault: %+v", r)
	}
	// The page keeps working on the message channel.
	if got := win.EvalString(`window.duplex.echoOnce("after")`); got != "after" {
		t.Errorf("echo after the fault: %q", got)
	}
	noWebKitException(t, app)
	app.Quit()
}

// TestLaneReloadNoLeak reloads the page while an upload (64 KiB frames, up
// to 1 GiB, not ending on its own) is going through up:post, with at least
// one up:post request open in the scheme handler at the reload moment
// (upActive >= 1). The new page gets {down: fallback, up: up:post} again,
// the Go goroutines return to the old page's idle baseline (±5), no lane
// request of the old page stays open in the scheme handler (upActive,
// downActive), and the new page's up:post carries a 1 MiB Echo.
func TestLaneReloadNoLeak(t *testing.T) {
	app, win := launchLaneFlow(t)
	var start struct {
		Base  map[string]int64 `json:"base"`
		Mid   map[string]int64 `json:"mid"`
		Going bool             `json:"going"`
		Lanes lanes            `json:"lanes"`
	}
	runScenario(t, win, "reloadStart()", &start)
	if !start.Going {
		t.Fatalf("the upload did not start using up:post: %+v", start)
	}
	// Reload while the upload is in flight (it does not end on its own).
	// The mark tells the old page from the new one.
	var at struct {
		Uploaded int              `json:"uploaded"`
		Stats    map[string]int64 `json:"stats"`
	}
	win.EvalInto(`(async () => {
	  // Reload only once another up:post request has started since
	  // reloadStart returned and one is open now: the upload is going.
	  let st;
	  for (const t0 = Date.now(); Date.now() - t0 < 3000; await new Promise(r => setTimeout(r, 2))) {
	    st = await tango.call("Debug.Stats");
	    if (st.upRequests > `+strconv.FormatInt(start.Mid["upRequests"], 10)+` && st.upActive >= 1) break;
	  }
	  window.__oldPage = true;
	  setTimeout(() => location.reload(), 0);
	  return { uploaded: laneTests.uploaded, stats: st };
	})()`, &at)
	t.Logf("at reload: %d frames sent, %+v", at.Uploaded, at.Stats)
	if at.Stats["upRequests"] <= start.Mid["upRequests"] {
		t.Errorf("the upload was not going any more at the reload: upRequests %d, %d before", at.Stats["upRequests"], start.Mid["upRequests"])
	}
	// Otherwise "no request left open after the reload" proves nothing.
	if at.Stats["upActive"] < 1 {
		t.Errorf("no up:post request was open at the reload: upActive %d", at.Stats["upActive"])
	}
	time.Sleep(1500 * time.Millisecond)
	win.WaitJS(`!window.__oldPage && document.readyState === "complete" && !!window.tango && !!window.duplex`)
	loadLaneTests(t, win)
	base, _ := json.Marshal(start.Base)
	var r struct {
		laneResult
		Lanes                      lanes
		Goroutines, BaseGoroutines int
		UpActive, DownActive       int
		SettleMS                   int
	}
	runScenario(t, win, "reloadCheck("+string(base)+", 30000)", &r)
	checkScenario(t, "reloadCheck", r.laneResult)
	if r.Lanes != (lanes{"fallback", "up:post"}) || r.UpActive != 0 || r.DownActive != 0 {
		t.Errorf("after reload: %+v", r)
	}
	noWebKitException(t, app)
	app.Quit()
}
