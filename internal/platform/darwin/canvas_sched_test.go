//go:build darwin && !ios

package darwin

import (
	"testing"
	"time"
)

func TestTimerTarget(t *testing.T) {
	base := time.Unix(1000, 0)
	at, thr := base.Add(30*time.Millisecond), base.Add(10*time.Millisecond)
	cases := []struct {
		name         string
		timerAt, thr time.Time
		want         time.Time
	}{
		{"idle", time.Time{}, time.Time{}, time.Time{}},
		{"deadline only", at, time.Time{}, at},
		{"throttle only", time.Time{}, thr, thr},
		{"throttle earlier", at, thr, thr},
		{"deadline earlier", thr, at, thr},
	}
	for _, c := range cases {
		if got := timerTarget(c.timerAt, c.thr); !got.Equal(c.want) {
			t.Errorf("%s: timerTarget = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTimerArmSkipsUnchangedFireDate(t *testing.T) {
	base := time.Unix(1000, 0)
	var a timerArm
	if !a.need(time.Time{}) {
		t.Fatal("first arm (idle) must set the timer: its state is unknown")
	}
	if a.need(time.Time{}) {
		t.Fatal("idle again: the timer is already far in the future")
	}
	d := base.Add(16 * time.Millisecond)
	if !a.need(d) {
		t.Fatal("a new deadline must arm the timer")
	}
	if a.need(d) {
		t.Fatal("the same deadline must not re-arm")
	}
	if !a.need(base.Add(8 * time.Millisecond)) {
		t.Fatal("an earlier deadline must re-arm")
	}
	if !a.need(base.Add(40 * time.Millisecond)) {
		t.Fatal("a later deadline must re-arm")
	}
	if !a.need(time.Time{}) {
		t.Fatal("going idle must move the timer to the far future")
	}
	// The timer fires: CoreFoundation moves it a full interval (farFuture)
	// ahead, so it is idle without a call; the same deadline again needs one.
	a.need(d)
	a.fired()
	if a.need(time.Time{}) {
		t.Fatal("after firing the timer is already idle")
	}
	if !a.need(d) {
		t.Fatal("after firing, the old deadline must be armed again")
	}
	// Same instant, different monotonic reading: still the same fire date.
	a.reset()
	w := time.Now()
	a.need(w)
	if a.need(w.Round(0)) {
		t.Fatal("equal instants must compare equal")
	}
	a.reset()
	if !a.need(w) {
		t.Fatal("reset forgets the armed date")
	}
}

func TestLinkCovers(t *testing.T) {
	now := time.Unix(1000, 0)
	const period = 16667 * time.Microsecond
	tick := now.Add(-time.Millisecond) // the tick that just ran
	cases := []struct {
		name    string
		t       time.Time
		ok      bool
		running bool
		period  time.Duration
		last    time.Time
		want    bool
	}{
		{"animation deadline before next refresh", now.Add(15 * time.Millisecond), true, true, period, tick, true},
		{"deadline exactly at next refresh", tick.Add(period), true, true, period, tick, true},
		{"deadline within tolerance", tick.Add(period + timerTolerance), true, true, period, tick, true},
		{"deadline after next refresh", tick.Add(period + timerTolerance + time.Microsecond), true, true, period, tick, false},
		{"120 Hz: 16 ms is two refreshes away", now.Add(16 * time.Millisecond), true, true, 8333 * time.Microsecond, tick, false},
		{"no display link", now.Add(time.Millisecond), false, true, period, tick, false},
		{"link paused: phase unknown", now.Add(5 * time.Millisecond), true, false, period, tick, false},
		{"link paused, deadline already due", now, true, false, period, tick, true},
		{"no period yet", now.Add(5 * time.Millisecond), true, true, 0, tick, false},
		{"stale tick", now.Add(5 * time.Millisecond), true, true, period, now.Add(-2 * period), false},
		{"past deadline", now.Add(-time.Second), true, true, period, tick, true},
		{"zero deadline", time.Time{}, true, true, period, tick, false},
	}
	for _, c := range cases {
		if got := linkCovers(c.t, now, c.ok, c.running, c.last, c.period); got != c.want {
			t.Errorf("%s: linkCovers = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTickPeriod(t *testing.T) {
	var p tickPeriod
	base := time.Unix(1000, 0)
	if p.get() != 0 {
		t.Fatal("no estimate before two ticks")
	}
	p.tick(base)
	if p.get() != 0 {
		t.Fatal("no estimate after one tick")
	}
	ts := base
	for _, d := range []time.Duration{16700, 16600, 17100, 16650} {
		ts = ts.Add(d * time.Microsecond)
		p.tick(ts)
	}
	if got := p.get(); got != 16600*time.Microsecond {
		t.Fatalf("period %v, want the shortest recent interval 16.6ms", got)
	}
	// A late tick (missed refresh) does not lengthen the estimate.
	ts = ts.Add(33 * time.Millisecond)
	p.tick(ts)
	if got := p.get(); got != 16600*time.Microsecond {
		t.Fatalf("period %v after a missed refresh", got)
	}
	// A pause resets: the next tick starts a new run.
	p.pause()
	if p.get() != 0 {
		t.Fatal("pause must drop the estimate")
	}
	p.tick(ts.Add(time.Second))
	p.tick(ts.Add(time.Second + 8333*time.Microsecond))
	if got := p.get(); got != 8333*time.Microsecond {
		t.Fatalf("period %v after a 120 Hz run", got)
	}
	// Old intervals age out: after a full window of 120 Hz ticks a short
	// glitch from the 60 Hz run is forgotten.
	var q tickPeriod
	ts = base
	q.tick(ts)
	ts = ts.Add(5 * time.Millisecond) // glitch
	q.tick(ts)
	for i := 0; i < tickWindow; i++ {
		ts = ts.Add(16667 * time.Microsecond)
		q.tick(ts)
	}
	if got := q.get(); got != 16667*time.Microsecond {
		t.Fatalf("period %v, want the glitch aged out", got)
	}
	// Intervals too long to be refreshes are ignored.
	var r tickPeriod
	r.tick(base)
	r.tick(base.Add(time.Second))
	if r.get() != 0 {
		t.Fatalf("period %v from a 1 s gap", r.get())
	}
}
