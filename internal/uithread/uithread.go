// Package uithread runs a task loop on a goroutine locked to its own OS
// thread: the UI thread of one window.
package uithread

import (
	"bytes"
	"runtime"
	"strconv"
	"sync"
)

// Thread is a goroutine locked to an OS thread and the FIFO of tasks it runs.
type Thread struct {
	name    string // for debugging
	gid     uint64 // goroutine id; set before Start returns, then read-only
	onPanic func(any)

	mu             sync.Mutex
	queue          []func()
	stopped        bool
	pump, wakePump func()
	// pumping: the thread is inside pump (guarded by mu). Only then does a
	// Post need wakePump; otherwise the loop sees the queue before pumping.
	pumping bool

	wake chan struct{} // capacity 1: "the queue may be non-empty"
	done chan struct{}

	awaiting bool          // guarded by mu
	entered  chan struct{} // guarded by mu; closed on the next entry
}

// registry maps the goroutine id of each running Thread to the Thread.
var registry sync.Map // uint64 -> *Thread

// Start starts a thread. When it returns the thread runs and its goroutine
// id is recorded. A panicking task crashes the process.
func Start(name string) *Thread { return StartWith(name, nil) }

// StartWith is Start with a panic handler: a task that panics is recovered
// and onPanic is called with the value on the thread, which keeps running.
// A nil onPanic lets the panic propagate, as Start does.
func StartWith(name string, onPanic func(any)) *Thread {
	t := &Thread{
		name:    name,
		onPanic: onPanic,
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	started := make(chan struct{})
	go t.run(started)
	<-started
	return t
}

func (t *Thread) run(started chan<- struct{}) {
	// Never unlocked: the OS thread exits with the goroutine.
	runtime.LockOSThread()
	// The thread exits with the goroutine, so its QoS goes with it
	// (darwin: user-interactive, like the main thread; qos_darwin.go).
	raiseQoS()
	t.gid = GoroutineID()
	registry.Store(t.gid, t)
	defer func() {
		registry.Delete(t.gid)
		close(t.done)
	}()
	close(started)

	for {
		t.mu.Lock()
		batch, stopped, pump := t.queue, t.stopped, t.pump
		t.queue = nil
		// Set with the empty-queue check, under the same lock: a Post
		// either lands before it (and runs next) or sees pumping and wakes.
		t.pumping = len(batch) == 0 && !stopped && pump != nil
		t.mu.Unlock()
		if len(batch) == 0 {
			if stopped {
				return
			}
			if pump != nil {
				pump()
				t.mu.Lock()
				t.pumping = false
				t.mu.Unlock()
			} else {
				<-t.wake
			}
			continue
		}
		for i, fn := range batch {
			batch[i] = nil
			t.call(fn)
		}
	}
}

func (t *Thread) call(fn func()) {
	if t.onPanic != nil {
		defer func() {
			if v := recover(); v != nil {
				t.onPanic(v)
			}
		}()
	}
	fn()
}

// Post queues fn to run on the thread after the tasks queued before it. It
// may be called from any goroutine, the thread included (fn then runs after
// the current task returns). It returns false, dropping fn, if the thread is
// stopped.
func (t *Thread) Post(fn func()) bool {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return false
	}
	t.queue = append(t.queue, fn)
	if t.wakePump != nil && !t.pumping {
		// Running a task or about to look at the queue: no native wake
		// (each one costs the pump an extra run-loop turn).
		t.mu.Unlock()
		return true
	}
	t.mu.Unlock()
	t.signal()
	return true
}

// SetIdlePump installs (or clears) the UI thread's native event pump.
// Only the owned thread may call it. wake must latch a signal even if pump
// has not entered the native loop yet; it must not execute callbacks.
// On return from clearing it, no concurrent Post/Stop can call the old wake.
func (t *Thread) SetIdlePump(pump, wake func()) {
	t.mu.Lock()
	t.pump, t.wakePump = pump, wake
	t.mu.Unlock()
}

func (t *Thread) signal() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.wakePump != nil {
		t.wakePump()
		return
	}
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// Do runs fn on the thread and waits for it to return. Called on the thread
// itself it runs fn directly (even while draining after Stop). Otherwise it
// returns false, without running fn, if the thread is stopped.
func (t *Thread) Do(fn func()) bool {
	if t.On() {
		fn()
		return true
	}
	ran := make(chan struct{})
	if !t.Post(func() {
		defer close(ran)
		fn()
	}) {
		return false
	}
	<-ran
	return true
}

// On reports whether the caller is the thread.
func (t *Thread) On() bool { return GoroutineID() == t.gid }

// Stop makes the thread exit once it has run the tasks already queued;
// later Posts return false. It does not wait (see Done). Repeated calls have
// no effect.
func (t *Thread) Stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
	t.signal()
}

// Done is closed when the thread has exited.
func (t *Thread) Done() <-chan struct{} { return t.done }

// SetAwaitingMain records whether the thread is blocked waiting for the main
// thread, so the main thread does not in turn wait for it.
func (t *Thread) SetAwaitingMain(on bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.awaiting == on {
		return
	}
	t.awaiting = on
	if on && t.entered != nil {
		close(t.entered)
		t.entered = nil
	}
}

// AwaitingMain reports what SetAwaitingMain last recorded.
func (t *Thread) AwaitingMain() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.awaiting
}

// WatchAwaitingMain atomically checks whether the thread is awaiting main
// and, if not, returns a channel closed the next time it starts to wait.
func (t *Thread) WatchAwaitingMain() (awaiting bool, entered <-chan struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.awaiting {
		return true, nil
	}
	if t.entered == nil {
		t.entered = make(chan struct{})
	}
	return false, t.entered
}

// Current returns the Thread the caller runs on, or nil if it is not on one.
func Current() *Thread {
	if v, ok := registry.Load(GoroutineID()); ok {
		return v.(*Thread)
	}
	return nil
}

// GoroutineID returns the calling goroutine's id, parsed from its stack
// header ("goroutine N [...]").
func GoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := bytes.TrimPrefix(buf[:n], []byte("goroutine "))
	if i := bytes.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	id, _ := strconv.ParseUint(string(s), 10, 64)
	return id
}
