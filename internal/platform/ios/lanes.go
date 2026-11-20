package ios

// IPC fast lanes, the pure-Go part (no build tag, so its tests run on the
// host too); the surface methods are in lanes_ios.go.

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tangoui/tango/internal/lane"
)

// laneCapsFromEnv returns the lanes a new surface reports, from the
// environment read through getenv:
//
//   - TANGO_IPC_LANES=0: none (the kill switch wins).
//   - TANGO_IPC_LANES_DARWIN "0", "false" or "off" (any case, trimmed): none.
//   - Unset, empty or any other value: up:post only. iOS does not offer down:poll.
//
// hooks is testHooks (testhooks_on.go). It no longer changes anything:
// with the hooks "all" used to add the removed down:scheme lane.
func laneCapsFromEnv(getenv func(string) string, hooks bool) []string {
	if getenv("TANGO_IPC_LANES") == "0" {
		return nil
	}
	switch v := strings.ToLower(strings.TrimSpace(getenv("TANGO_IPC_LANES_DARWIN"))); {
	case v == "0" || v == "false" || v == "off":
		return nil
	default:
		return []string{lane.UpPost}
	}
}

// ---------------------------------------------------------------------------
// Lane counters (test hook). With TANGO_SCHEME_PROBE=1 the scheme handler
// counts the lane requests it sees and how they were answered, for the
// platform tests (examples/duplex-mobile's Debug.Stats reads LaneDownStats).
// Without the variable nothing is counted.
//
// These hooks, the streaming probe in bridge.m and TANGO_IPC_LANES_DARWIN=all
// exist only in builds with the self-test hooks (testHooks,
// testhooks_on.go: -tags tango_dev or tango_selftest), as on macOS
// (internal/platform/darwin's lanes_dev.go, tango_dev only: tan build drops
// tango_dev, so the simulator selftest, scripts/smoke.sh ios-selftest,
// builds with GOFLAGS=-tags=tango_selftest).

// LaneStats are the scheme handler's lane counters.
type LaneStats struct {
	DownRequests int64 // GET /__tango/lane/down requests
	DownConns    int64 // of them, answered 200: accepted down connections
	DownActive   int64 // accepted down connections not yet finished or stopped
	DownWrites   int64 // data blocks handed to WebKit on accepted down connections
	UpRequests   int64 // POST /__tango/lane/up requests
	UpAccepted   int64 // of them, answered 2xx
	UpActive     int64 // POST /__tango/lane/up requests not yet finished or stopped
}

// LaneDownStats returns the lane counters (all zero unless
// TANGO_SCHEME_PROBE=1). Any goroutine.
func LaneDownStats() LaneStats { return laneProbe.stats() }

var laneProbe laneCounters

type laneCounters struct {
	downReq, downConns, downActive, downWrites, upReq, upOK, upActive atomic.Int64
}

func (c *laneCounters) stats() LaneStats {
	return LaneStats{
		DownRequests: c.downReq.Load(), DownConns: c.downConns.Load(), DownActive: c.downActive.Load(),
		DownWrites: c.downWrites.Load(), UpRequests: c.upReq.Load(), UpAccepted: c.upOK.Load(),
		UpActive: c.upActive.Load(),
	}
}

// track starts counting a request for path; nil if it is not a lane
// request.
func (c *laneCounters) track(path string) *laneTrack {
	switch path {
	case "/__tango/lane/down":
		c.downReq.Add(1)
		return &laneTrack{c: c, down: true}
	case "/__tango/lane/up":
		c.upReq.Add(1)
		c.upActive.Add(1)
		return &laneTrack{c: c, open: true}
	}
	return nil
}

// laneTrack follows one lane request. Its methods are called on the main
// thread (the writer's drain, tangoIOSTaskStop) and accept a nil receiver.
type laneTrack struct {
	c        *laneCounters
	down     bool
	accepted bool // a down connection answered 200, until done
	open     bool // an up request, until done
}

func (t *laneTrack) respond(status int) {
	switch {
	case t == nil:
	case t.down && status == 200:
		t.accepted = true
		t.c.downConns.Add(1)
		t.c.downActive.Add(1)
	case !t.down && status >= 200 && status < 300:
		t.c.upOK.Add(1)
	}
}

func (t *laneTrack) data() {
	if t != nil && t.accepted {
		t.c.downWrites.Add(1)
	}
}

func (t *laneTrack) done() {
	if t != nil && t.accepted {
		t.accepted = false
		t.c.downActive.Add(-1)
	}
	if t != nil && t.open {
		t.open = false
		t.c.upActive.Add(-1)
	}
}

// laneProbeOn reports whether the test hook is on (TANGO_SCHEME_PROBE=1;
// bridge.m reads the same variable for its streaming probe).
var laneProbeOn = sync.OnceValue(func() bool { return testHooks && os.Getenv("TANGO_SCHEME_PROBE") == "1" })

// laneTrackFor returns the counter of a request for path, nil unless the
// test hook is on.
func laneTrackFor(path string) *laneTrack {
	if !laneProbeOn() {
		return nil
	}
	return laneProbe.track(path)
}
