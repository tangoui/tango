// Package mobilecanvas holds the platform-independent logic of the iOS and
// Android canvas backends (docs/specs/mobile-canvas.md): the frame
// scheduler, touch bookkeeping, the input-method shadow document, UTF-16
// offsets and inset arithmetic. It has no build tags and no cgo, so it is
// tested on any host; the backends wrap it with their native glue.
package mobilecanvas

import (
	"sync"
	"sync/atomic"
	"time"
)

// Frame scheduling (docs/specs/mobile-canvas.md §1.2.2, §1.3.4).
//
// A canvas draws on the display's refresh clock (CADisplayLink, Android
// Choreographer), at most one frame per refresh and none while nothing
// asked for one:
//
//   - Invalidate and InvalidateAt may be called from any goroutine. They
//     only record the request; when they report true the platform signals
//     its main thread (CFRunLoopSource, Handler.post), which then calls
//     Wake.
//   - Wake resumes the refresh link when a frame is pending and arms the
//     deadline timer. Each Tick of the link produces a frame when one is
//     pending; IdleTicks ticks without a request pause the link, so a still
//     page costs nothing.
//   - InvalidateAt keeps the earliest deadline. When the timer fires,
//     TimerFired turns it into a pending frame. Any frame satisfies a
//     pending deadline (the core asks again after each frame).
//   - While the app is in the background no frame is ever produced (iOS
//     terminates apps that submit GPU work in the background). Requests
//     made meanwhile stay pending and Foreground resumes the link for a
//     catch-up frame.
//
// Wake, Tick, TimerFired, SyncFrame, Background, Foreground and Close run
// on the main thread; they return the Action the platform performs.

// IdleTicks is the number of refresh ticks without a frame request after
// which the refresh link pauses itself.
const IdleTicks = 4

// timerSlack is how early a deadline timer may fire and still count as due.
const timerSlack = 2 * time.Millisecond

// LinkOp tells the platform what to do with its refresh link.
type LinkOp uint8

const (
	LinkKeep   LinkOp = iota // leave the link as it is
	LinkResume               // unpause the link (frames follow on its ticks)
	LinkPause                // pause the link
)

func (o LinkOp) String() string {
	switch o {
	case LinkKeep:
		return "keep"
	case LinkResume:
		return "resume"
	case LinkPause:
		return "pause"
	}
	return "LinkOp(?)"
}

// Action is what the platform does after a scheduler call.
type Action struct {
	Link LinkOp
	// Frame: call CanvasFrame now (Tick and SyncFrame only).
	Frame bool
	// Arm: (re)arm the deadline timer to fire at At, or cancel it when At
	// is zero. Without Arm the timer stays as it is.
	Arm bool
	At  time.Time
}

// Sched is the frame scheduler of one canvas. The zero value is ready: the
// link paused, nothing pending, in the foreground.
type Sched struct {
	pending atomic.Bool // a frame was requested since the last one
	closed  atomic.Bool

	mu       sync.Mutex
	deadline time.Time // earliest InvalidateAt not yet satisfied; zero: none

	// Main thread only.
	link  bool      // the link is running
	idle  int       // ticks without a frame since the last frame
	bg    bool      // the app is in the background
	armed time.Time // what the timer is armed to; zero: not armed
}

// Invalidate requests a frame. Any goroutine. It reports whether the
// platform must wake its main thread (and then call Wake): only the first
// request after a frame does, later ones coalesce.
func (s *Sched) Invalidate() bool {
	if s.closed.Load() {
		return false
	}
	return s.pending.CompareAndSwap(false, true)
}

// InvalidateAt requests a frame no later than t. Any goroutine. An earlier
// pending deadline wins. It reports whether the platform must wake its main
// thread (and then call Wake) to re-arm the timer: only when t is earlier
// than the deadline already requested.
func (s *Sched) InvalidateAt(t time.Time) bool {
	if s.closed.Load() || t.IsZero() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.deadline.IsZero() && !t.Before(s.deadline) {
		return false
	}
	s.deadline = t
	return true
}

// Pending reports whether a frame is requested. Any goroutine.
func (s *Sched) Pending() bool { return s.pending.Load() }

// Deadline returns the earliest pending InvalidateAt time (zero: none).
// Any goroutine.
func (s *Sched) Deadline() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deadline
}

// Running reports whether the scheduler wants the link running. Main
// thread.
func (s *Sched) Running() bool { return s.link }

// InBackground reports whether the app is in the background. Main
// thread.
func (s *Sched) InBackground() bool { return s.bg }

// Wake handles the platform's main-thread signal after Invalidate or
// InvalidateAt reported true. Main thread.
func (s *Sched) Wake(now time.Time) Action {
	if s.closed.Load() {
		return Action{}
	}
	var a Action
	s.timer(now, &a)
	if s.pending.Load() {
		s.resume(&a)
	}
	return a
}

// Tick handles one refresh-link tick. It asks for a frame when one is
// pending and pauses the link after IdleTicks idle ticks. Main thread.
func (s *Sched) Tick(now time.Time) Action {
	if s.closed.Load() {
		return Action{}
	}
	if s.bg {
		// The link should be paused already; never draw in the background.
		return s.pause()
	}
	var a Action
	s.timer(now, &a)
	if s.pending.Load() {
		s.beginFrame(&a)
		return a
	}
	s.idle++
	if s.idle >= IdleTicks {
		p := s.pause()
		a.Link = p.Link
	}
	return a
}

// TimerFired handles the deadline timer firing. Main thread.
func (s *Sched) TimerFired(now time.Time) Action {
	if s.closed.Load() {
		return Action{}
	}
	s.armed = time.Time{}
	var a Action
	s.timer(now, &a)
	if s.pending.Load() {
		s.resume(&a)
	}
	return a
}

// SyncFrame asks whether the platform may draw a frame synchronously, now,
// outside the link (after a size, scale or orientation change, so no
// stretched old frame shows). It is refused in the background. An allowed
// frame consumes the pending request and deadline like a ticked one. Main
// thread.
func (s *Sched) SyncFrame() Action {
	if s.closed.Load() || s.bg {
		return Action{}
	}
	var a Action
	s.beginFrame(&a)
	return a
}

// Background pauses the link and the timer: no frame until
// Foreground. Requests made meanwhile stay pending. Main thread.
func (s *Sched) Background() Action {
	if s.closed.Load() || s.bg {
		return Action{}
	}
	s.bg = true
	a := s.pause()
	if !s.armed.IsZero() {
		s.armed = time.Time{}
		a.Arm = true
	}
	return a
}

// Foreground leaves the background. A frame requested meanwhile (or a
// deadline that passed) resumes the link for a catch-up frame; a later
// deadline re-arms the timer. Main thread.
func (s *Sched) Foreground(now time.Time) Action {
	if s.closed.Load() || !s.bg {
		return Action{}
	}
	s.bg = false
	var a Action
	s.timer(now, &a)
	if s.pending.Load() {
		s.resume(&a)
	}
	return a
}

// Close stops the scheduler for good (the canvas is gone): it pauses the
// link, cancels the timer, and later requests report false. Main thread.
func (s *Sched) Close() Action {
	if s.closed.Swap(true) {
		return Action{}
	}
	a := Action{}
	if s.link {
		s.link = false
		a.Link = LinkPause
	}
	if !s.armed.IsZero() {
		s.armed = time.Time{}
		a.Arm = true
	}
	s.pending.Store(false)
	s.mu.Lock()
	s.deadline = time.Time{}
	s.mu.Unlock()
	return a
}

// timer turns a due deadline into a pending frame and keeps the timer armed
// to the remaining deadline (not in the background).
func (s *Sched) timer(now time.Time, a *Action) {
	s.mu.Lock()
	d := s.deadline
	if !d.IsZero() && !d.After(now.Add(timerSlack)) {
		s.deadline = time.Time{}
		d = time.Time{}
		s.pending.Store(true)
	}
	s.mu.Unlock()
	if s.bg {
		d = time.Time{}
	}
	if !d.Equal(s.armed) {
		s.armed = d
		a.Arm, a.At = true, d
	}
}

// beginFrame consumes the request (before the frame runs, so a request made
// during the frame schedules the next one) and satisfies the deadline.
func (s *Sched) beginFrame(a *Action) {
	s.pending.Store(false)
	s.idle = 0
	s.mu.Lock()
	s.deadline = time.Time{}
	s.mu.Unlock()
	if !s.armed.IsZero() {
		s.armed = time.Time{}
		a.Arm, a.At = true, time.Time{}
	}
	a.Frame = true
}

func (s *Sched) resume(a *Action) {
	if s.bg || s.link {
		return
	}
	s.link = true
	s.idle = 0
	a.Link = LinkResume
}

func (s *Sched) pause() Action {
	s.idle = 0
	if !s.link {
		return Action{}
	}
	s.link = false
	return Action{Link: LinkPause}
}
