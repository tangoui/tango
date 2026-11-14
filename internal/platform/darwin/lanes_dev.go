//go:build darwin && !ios && tango_dev

package darwin

// Development-build test hooks of the lanes (see lanes.go): the
// TANGO_IPC_LANES_DARWIN=all experiment and the lane counters.

import (
	"sync/atomic"
	"time"
)

// devTestHooks is true in development builds: laneCapsFromEnv honours
// TANGO_IPC_LANES_DARWIN=all and the scheme handler honours
// TANGO_SCHEME_PROBE=1 (lane counters, the streaming probe route).
const devTestHooks = true

// LaneDownStats returns the lane counters (all zero unless
// TANGO_SCHEME_PROBE=1, and always zero in production builds). Any
// goroutine.
func LaneDownStats() LaneStats { return laneProbe.stats() }

var laneProbe laneCounters

type laneCounters struct {
	downReq, downConns, downActive, downWrites, upReq, upOK, upActive atomic.Int64
	upMainNanos, upMainMax, upBodyBytes                               atomic.Int64
}

func (c *laneCounters) stats() LaneStats {
	return LaneStats{
		DownRequests: c.downReq.Load(), DownConns: c.downConns.Load(), DownActive: c.downActive.Load(),
		DownWrites: c.downWrites.Load(), UpRequests: c.upReq.Load(), UpAccepted: c.upOK.Load(),
		UpActive: c.upActive.Load(), UpMainNanos: c.upMainNanos.Load(), UpMainMaxNanos: c.upMainMax.Load(),
		UpBodyBytes: c.upBodyBytes.Load(),
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

// laneTrack follows one lane request. Its methods are called by one
// goroutine at a time (the main thread) and accept a nil receiver.
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

// mainTime counts the main-thread time d serveSchemeTask spent on an up
// request with a body of n bytes.
func (t *laneTrack) mainTime(d time.Duration, n int64) {
	if t == nil || t.down {
		return
	}
	t.c.upMainNanos.Add(int64(d))
	t.c.upBodyBytes.Add(n)
	for {
		m := t.c.upMainMax.Load()
		if int64(d) <= m || t.c.upMainMax.CompareAndSwap(m, int64(d)) {
			return
		}
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

// laneTrackFor returns the counter of a request for path, nil unless the
// test hook is on.
func laneTrackFor(path string) *laneTrack {
	if !schemeProbe() {
		return nil
	}
	return laneProbe.track(path)
}
