//go:build darwin && !ios

package darwin

import "time"

// Frame-scheduling decisions of canvas (canvas.go), kept free of AppKit so
// they can be tested. An animating window asks for its next frame with
// InvalidateAt(now+16ms) every frame; arming a CFRunLoopTimer for that and
// taking its callback costs two CFRunLoopTimerSetNextFireDate calls and a
// run-loop timer wake-up per frame, although the display link's next
// refresh produces the same frame.

// timerTarget is the earliest pending deadline: the InvalidateAt deadline
// or the fallback pacing time (zero: none, the timer idles far in the
// future).
func timerTarget(timerAt, throttleAt time.Time) time.Time {
	if !throttleAt.IsZero() && (timerAt.IsZero() || throttleAt.Before(timerAt)) {
		return throttleAt
	}
	return timerAt
}

// timerArm remembers what the CFRunLoopTimer is armed to, so armTimer
// skips CFRunLoopTimerSetNextFireDate when the deadline did not change.
type timerArm struct {
	at    time.Time // armed deadline; zero: far future
	known bool      // at reflects the timer (false: never armed)
}

// need reports whether the timer must be set to t (zero: far future), and
// records t as armed when it does.
func (a *timerArm) need(t time.Time) bool {
	if a.known && a.at.Equal(t) {
		return false
	}
	a.at, a.known = t, true
	return true
}

// fired records that the timer fired: its interval is farFuture, so
// CoreFoundation moved it to the far future.
func (a *timerArm) fired() { a.at, a.known = time.Time{}, true }

// reset forgets the armed date (the next need always sets the timer).
func (a *timerArm) reset() { *a = timerArm{} }

// linkCovers reports whether the display link's next refresh satisfies a
// frame deadline t, so the canvas can ask the link for a frame instead of
// arming the timer. The timer would only call requestFrame at t, and the
// frame would then wait for the first refresh after t; when t is no later
// than the next refresh, asking now produces the frame at the same
// refresh. hasLink: the canvas has a display link; running: it is
// unpaused and lastTick was its latest tick; period: the refresh interval
// estimate (0: unknown). A deadline that is already due is covered by any
// link (the timer would fire at once). Like canvasTimerFired, a deadline
// within timerTolerance of the refresh counts as reached.
func linkCovers(t, now time.Time, hasLink, running bool, lastTick time.Time, period time.Duration) bool {
	if t.IsZero() || !hasLink {
		return false
	}
	if !t.After(now.Add(timerTolerance)) {
		return true
	}
	if !running || period <= 0 || lastTick.IsZero() || now.Sub(lastTick) >= period {
		return false // the next refresh's time is unknown
	}
	return !t.After(lastTick.Add(period + timerTolerance))
}

// tickWindow is the number of recent tick intervals tickPeriod keeps.
const tickWindow = 8

// maxTickInterval bounds what counts as one refresh interval (slower
// displays than 20 Hz are not expected; longer gaps are stalls).
const maxTickInterval = 50 * time.Millisecond

// tickPeriod estimates the display refresh interval from display-link
// tick times: the shortest of the last tickWindow intervals, so a late or
// missed tick never lengthens it (a short estimate only makes linkCovers
// fall back to the timer).
type tickPeriod struct {
	last time.Time
	ivs  [tickWindow]time.Duration
	n    int // intervals recorded (≤ tickWindow are valid)
}

// tick records a display-link tick at now.
func (p *tickPeriod) tick(now time.Time) {
	if !p.last.IsZero() {
		if d := now.Sub(p.last); d > 0 && d <= maxTickInterval {
			p.ivs[p.n%tickWindow] = d
			p.n++
		}
	}
	p.last = now
}

// pause forgets the run of ticks (the link was paused; the next tick is
// not one interval after the last).
func (p *tickPeriod) pause() { *p = tickPeriod{} }

// lastTick returns the latest tick time (zero: none since the last pause).
func (p *tickPeriod) lastTick() time.Time { return p.last }

// get returns the estimate (0: unknown).
func (p *tickPeriod) get() time.Duration {
	var m time.Duration
	for i := 0; i < min(p.n, tickWindow); i++ {
		if m == 0 || p.ivs[i] < m {
			m = p.ivs[i]
		}
	}
	return m
}
