package mobilecanvas

import (
	"math"
	"slices"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Touch bookkeeping (docs/specs/mobile-canvas.md §2.1, docs/specs/touch-input.md
// §2). The native side reports each finger by a platform key (a UITouch
// pointer on iOS, a pointer id on Android); Touches turns those reports
// into input.TouchEvent values with stable IDs and remembers which fingers
// are down so a system interruption can cancel them all.

// MaxCoalesced is the most intermediate samples delivered per finger and
// move report (iOS coalesced touches, Android historical points).
const MaxCoalesced = 4

// Sample is one platform report of a finger.
type Sample struct {
	Pos      geom.Point // logical pixels, relative to the content area
	Pressure float32    // 0..1; 0: not reported
	Time     time.Time  // zero: unknown (the runner uses the frame time)
}

// finger is a finger that is down.
type finger struct {
	id   int64
	last Sample
	seq  uint64 // begin order, for CancelAll
}

// Touches maps platform finger keys to input.TouchEvent IDs. IDs start at 1
// and increase; one is never reused while its finger is down (nor ever,
// before the int64 wraps). The zero value is ready. Main thread.
type Touches[K comparable] struct {
	next int64
	seq  uint64
	live map[K]*finger
}

// Len returns the number of fingers down.
func (t *Touches[K]) Len() int { return len(t.live) }

// ID returns the ID of the finger k, if it is down.
func (t *Touches[K]) ID(k K) (int64, bool) {
	if f, ok := t.live[k]; ok {
		return f.id, true
	}
	return 0, false
}

// Begin appends the TouchBegan event of a new finger k to dst. If k is
// still down (its end was lost) the stale finger is cancelled first.
func (t *Touches[K]) Begin(dst []input.TouchEvent, k K, s Sample) []input.TouchEvent {
	if f, ok := t.live[k]; ok {
		dst = append(dst, touchEvent(f.id, input.TouchCancelled, f.last.Pos, 0, s.Time))
		delete(t.live, k)
	}
	if t.live == nil {
		t.live = map[K]*finger{}
	}
	t.next++
	t.seq++
	f := &finger{id: t.next, last: s, seq: t.seq}
	t.live[k] = f
	return append(dst, touchEvent(f.id, input.TouchBegan, s.Pos, s.Pressure, s.Time))
}

// Move appends the TouchMoved events of finger k to dst: up to
// MaxCoalesced of the intermediate samples in history (oldest first, the
// current sample excluded; see Coalesce), then cur. A finger that is not
// down is ignored.
func (t *Touches[K]) Move(dst []input.TouchEvent, k K, history []Sample, cur Sample) []input.TouchEvent {
	f, ok := t.live[k]
	if !ok {
		return dst
	}
	for _, s := range Coalesce(history) {
		dst = append(dst, touchEvent(f.id, input.TouchMoved, s.Pos, s.Pressure, s.Time))
	}
	f.last = cur
	return append(dst, touchEvent(f.id, input.TouchMoved, cur.Pos, cur.Pressure, cur.Time))
}

// End appends the TouchEnded event of finger k to dst and frees k. A
// finger that is not down is ignored.
func (t *Touches[K]) End(dst []input.TouchEvent, k K, s Sample) []input.TouchEvent {
	return t.finish(dst, k, input.TouchEnded, s)
}

// Cancel appends the TouchCancelled event of finger k to dst and frees k.
// A finger that is not down is ignored.
func (t *Touches[K]) Cancel(dst []input.TouchEvent, k K, s Sample) []input.TouchEvent {
	return t.finish(dst, k, input.TouchCancelled, s)
}

func (t *Touches[K]) finish(dst []input.TouchEvent, k K, ph input.TouchPhase, s Sample) []input.TouchEvent {
	f, ok := t.live[k]
	if !ok {
		return dst
	}
	delete(t.live, k)
	p := s.Pressure
	if ph == input.TouchCancelled {
		p = 0
	}
	return append(dst, touchEvent(f.id, ph, s.Pos, p, s.Time))
}

// CancelAll appends a TouchCancelled event, at its last position, for
// every finger still down (in the order they went down) and frees them
// all: the system took the touches away (touchesCancelled, ACTION_CANCEL,
// the window lost focus). at is the events' time (zero: unknown).
func (t *Touches[K]) CancelAll(dst []input.TouchEvent, at time.Time) []input.TouchEvent {
	if len(t.live) == 0 {
		return dst
	}
	fs := make([]*finger, 0, len(t.live))
	for _, f := range t.live {
		fs = append(fs, f)
	}
	slices.SortFunc(fs, func(a, b *finger) int {
		switch {
		case a.seq < b.seq:
			return -1
		case a.seq > b.seq:
			return 1
		}
		return 0
	})
	for _, f := range fs {
		dst = append(dst, touchEvent(f.id, input.TouchCancelled, f.last.Pos, 0, at))
	}
	clear(t.live)
	return dst
}

func touchEvent(id int64, ph input.TouchPhase, p geom.Point, pressure float32, at time.Time) input.TouchEvent {
	return input.TouchEvent{ID: id, Phase: ph, Pos: p, Pressure: pressure, Time: at}
}

// Coalesce picks at most MaxCoalesced of the intermediate samples of one
// move report, evenly spread and always keeping the newest, oldest first.
// history holds the samples before the current one (iOS:
// coalescedTouchesForTouch: without its last element; Android: the
// historical points). The result may share history's backing array.
func Coalesce(history []Sample) []Sample {
	n := len(history)
	if n <= MaxCoalesced {
		return history
	}
	out := make([]Sample, MaxCoalesced)
	for i := range out {
		// Indices ceil((i+1)*n/Max)-1: spread over history, the last
		// one is n-1.
		out[i] = history[((i+1)*n+MaxCoalesced-1)/MaxCoalesced-1]
	}
	return out
}

// Pressure normalizes a platform pressure reading to 0..1: force over
// maxForce (iOS force / maximumPossibleForce; Android passes its pressure
// with maxForce 1). It returns 0, "not reported", when maxForce is not
// positive or the reading is not a number.
func Pressure(force, maxForce float32) float32 {
	if !(maxForce > 0) || force != force {
		return 0
	}
	p := force / maxForce
	switch {
	case p < 0:
		return 0
	case p > 1:
		return 1
	}
	return p
}

// Clock converts platform event timestamps on the system uptime clock
// (iOS UITouch.timestamp, Android getEventTime) to wall-clock time. The
// first conversion records the offset between the two clocks; later ones
// reuse it, so event spacing stays exact. Main thread.
type Clock struct {
	// Now returns the current time; nil means time.Now.
	Now func() time.Time

	base time.Time // wall-clock time of uptime 0
	set  bool
}

// clockResync is how far into the future a converted timestamp may land
// before the offset is recorded again (the uptime clock jumped or the
// device slept in a way the offset missed).
const clockResync = 50 * time.Millisecond

// Time converts an uptime timestamp to wall-clock time.
func (c *Clock) Time(uptime time.Duration) time.Time {
	now := c.now()
	if c.set {
		t := c.base.Add(uptime)
		if !t.After(now.Add(clockResync)) {
			return t
		}
	}
	c.base, c.set = now.Add(-uptime), true
	return now
}

// Reset forgets the offset; the next Time records it again.
func (c *Clock) Reset() { c.set = false }

func (c *Clock) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Seconds converts an iOS timestamp (seconds as a double) to a Duration.
func Seconds(s float64) time.Duration {
	if s != s || s <= 0 {
		return 0
	}
	if s >= math.MaxInt64/float64(time.Second) {
		return math.MaxInt64
	}
	return time.Duration(math.Round(s * float64(time.Second)))
}

// Millis converts an Android timestamp (uptimeMillis) to a Duration.
func Millis(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

// Nanos converts an Android frame or event time in nanoseconds to a
// Duration.
func Nanos(ns int64) time.Duration { return time.Duration(ns) }
