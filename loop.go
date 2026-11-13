// Main-thread queue, Await and AwaitAsync.
//
// The queue (post/drain/close) is adapted from mygo's loop.go
// (github.com/egoist/mygo v0.9.0), Copyright (c) 2026 MyGo contributors,
// MIT License.

package tango

import (
	"bytes"
	"errors"
	"runtime"
	"strconv"
	"sync"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/native"
)

// mainLoop is the FIFO of functions waiting to run on the main thread. The
// backend runs it (Host.Drain) after Backend.Wake.
type mainLoop struct {
	app *App

	mu      sync.Mutex
	queue   []func()
	running bool // backend initialized: Wake may be called
	closed  bool
}

// post queues fn. It reports false once the loop has shut down and fn will
// never run.
func (l *mainLoop) post(fn func()) bool {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return false
	}
	l.queue = append(l.queue, fn)
	wake := l.running && len(l.queue) == 1
	l.mu.Unlock()
	if wake {
		l.app.b.Wake()
	}
	return true
}

// start marks the backend ready for Wake and wakes it for anything posted
// before Run.
func (l *mainLoop) start() {
	l.mu.Lock()
	l.running = true
	pending := len(l.queue) > 0
	l.mu.Unlock()
	if pending {
		l.app.b.Wake()
	}
}

// drain runs queued functions one at a time until the queue is empty. Main
// thread only. A function that pumps a nested loop (AwaitAsync on the main
// thread, a modal dialog) runs the ones queued behind it, which it may be
// waiting for: so the backend is woken again while some wait.
func (l *mainLoop) drain() {
	for {
		l.mu.Lock()
		if len(l.queue) == 0 {
			l.mu.Unlock()
			return
		}
		fn := l.queue[0]
		l.queue[0] = nil
		l.queue = l.queue[1:]
		more := len(l.queue) > 0 && l.running
		l.mu.Unlock()
		if more {
			l.app.b.Wake()
		}
		// A panicking function is reported and the queue goes on.
		l.app.guard(whereMain, fn)
	}
}

// close rejects later posts and drops what is still queued. Waiters are
// released through App.closedCh.
func (l *mainLoop) close() {
	l.mu.Lock()
	l.closed = true
	l.running = false
	l.queue = nil
	l.mu.Unlock()
}

// ---- main-thread detection ----
//
// The contract has no IsMainThread, so the core tracks it itself. A flag
// set "while a main-thread callback runs" is not enough on its own: another
// goroutine checking it at that moment would wrongly see true. So the core
// records which goroutine is acting as the main thread:
//
//   - Run stores its goroutine id for its whole duration (desktop: the
//     native loop runs inside Run, on that goroutine, and Go runs callbacks
//     from C on the goroutine that made the C call).
//   - Every Host method also stores its goroutine id if none is stored, and
//     restores 0 on return. This covers hosts that drive the core only via
//     callbacks (mobile, where the native host owns the main thread and each
//     callback may arrive on a fresh goroutine).
//
// Host methods only ever run on the main thread, and nested ones share the
// goroutine, so at most one goroutine id is stored at a time.

// mainGoroutine is the id of the goroutine that ran package initialization:
// the process' main goroutine. Used only to catch Await on it before Run.
var mainGoroutine = goroutineID()

func goroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := bytes.TrimPrefix(buf[:n], []byte("goroutine "))
	if i := bytes.IndexByte(s, ' '); i >= 0 {
		s = s[:i]
	}
	id, _ := strconv.ParseUint(string(s), 10, 64)
	return id
}

// isMainGoroutine reports whether the caller is the goroutine running
// main.main. On desktop that is the one that ran package initialization;
// on mobile the native host starts main.main on a goroutine of its own,
// whose id the backend reports (native.MainGoroutine).
func isMainGoroutine() bool {
	if native.RunOwnsMainThread {
		return goroutineID() == mainGoroutine
	}
	id := native.MainGoroutine()
	return id != 0 && goroutineID() == id
}

func noop() {}

// enterMain records the calling goroutine as the main thread if none is
// recorded, returning the function that undoes it.
func (a *App) enterMain() func() {
	if a.mainG.Load() != 0 {
		return noop
	}
	a.mainG.Store(goroutineID())
	return func() { a.mainG.Store(0) }
}

// isMain reports whether the caller runs on the main thread.
func (a *App) isMain() bool {
	m := a.mainG.Load()
	return m != 0 && goroutineID() == m
}

// mainBeforeRun reports whether the caller is the main goroutine and Run
// has not started: waiting for the main thread here would deadlock.
func (a *App) mainBeforeRun() bool {
	return !a.ran.Load() && isMainGoroutine()
}

func (a *App) checkWait(what string) {
	if a.mainBeforeRun() {
		panic("tango: " + what + " called on the main goroutine before App.Run would deadlock; call it from OnReady")
	}
}

// ---- public scheduling API ----

// Post schedules fn to run on the main thread, after everything posted
// before it. Safe from any goroutine; before Run, fn waits for Run to start.
// After the app has quit, fn is dropped.
func (a *App) Post(fn func()) { a.loop.post(fn) }

// run runs fn on the main thread now if the caller is on it, else posts it.
func (a *App) run(fn func()) {
	if a.isMain() {
		fn()
		return
	}
	a.loop.post(fn)
}

// Await runs fn on the main thread and returns its result. On the main
// thread it calls fn directly. After the app has quit it returns the zero
// value without calling fn. It panics on the main goroutine before Run.
// If fn panics off the main thread, the panic goes to Options.OnPanic and
// Await returns the zero value; on the main thread it propagates to the
// caller like any call.
func Await[T any](a *App, fn func() T) T {
	v, _ := await(a, fn)
	return v
}

// await is Await reporting whether fn ran (and returned).
func await[T any](a *App, fn func() T) (T, bool) {
	if a.isMain() {
		return fn(), true
	}
	a.checkWait("Await")
	var zero T
	var v T
	var ok bool
	done := make(chan struct{})
	// close is deferred so a panicking fn (reported by drain) releases
	// the caller too.
	if !a.loop.post(func() { defer close(done); v = fn(); ok = true }) {
		return zero, false
	}
	select {
	case <-done:
		return v, ok
	case <-a.closedCh:
		select {
		case <-done:
			return v, ok
		default:
			return zero, false
		}
	}
}

// awaitVoid runs fn on the main thread and waits, reporting whether it ran.
func (a *App) awaitVoid(fn func()) bool {
	_, ok := await(a, func() struct{} { fn(); return struct{}{} })
	return ok
}

// AwaitAsync calls start on the main thread and blocks until start's done
// is called (from any goroutine; later calls are ignored), returning its
// value. On the main thread it calls start directly and processes native
// events with Backend.Step(true) until done, so dialogs and other nested
// work keep running. If the app quits first it returns the zero value. It
// panics on the main goroutine before Run.
//
// Where the platform cannot process native events while the main thread
// waits (Android), AwaitAsync on the main thread returns the zero value at
// once without calling start; TryAwaitAsync reports that as
// ErrNestedWait.
func AwaitAsync[T any](a *App, start func(done func(T))) T {
	v, _ := TryAwaitAsync(a, start)
	return v
}

// ErrNestedWait is returned by TryAwaitAsync called on the main thread of
// a platform that cannot keep processing native events while the main
// thread waits (Android): waiting there would never end. Call from another
// goroutine instead.
var ErrNestedWait = errors.New("tango: cannot wait for asynchronous work on the main thread on this platform; call from another goroutine")

// ErrPanicked is returned by TryAwaitAsync when start panicked before
// calling done. The panic went to Options.OnPanic.
var ErrPanicked = errors.New("tango: the function run on the main thread panicked")

// TryAwaitAsync is AwaitAsync reporting why no value was produced:
// ErrNotRunning when the app quit (or quits) before done was called,
// ErrNestedWait on the main thread where the platform cannot wait there
// (start is then not called), ErrPanicked when start panicked off the main
// thread before calling done (on the main thread the panic propagates to
// the caller).
func TryAwaitAsync[T any](a *App, start func(done func(T))) (T, error) {
	var zero T
	ch := make(chan T, 1)
	var once sync.Once
	if a.isMain() {
		if nl, ok := a.b.(platform.NestedLoop); ok && !nl.NestedLoopSupported() {
			return zero, ErrNestedWait
		}
		done := func(v T) {
			once.Do(func() {
				ch <- v
				// The main thread may be blocked in Step(true).
				a.wakeIfRunning()
			})
		}
		start(done)
		for {
			select {
			case v := <-ch:
				return v, nil
			default:
			}
			if !a.b.Step(true) {
				select {
				case v := <-ch:
					return v, nil
				default:
					return zero, ErrNotRunning
				}
			}
		}
	}
	a.checkWait("AwaitAsync")
	done := func(v T) { once.Do(func() { ch <- v }) }
	panicked := make(chan struct{})
	if !a.loop.post(func() {
		if !a.guard(whereMain, func() { start(done) }) {
			once.Do(func() { close(panicked) })
		}
	}) {
		return zero, ErrNotRunning
	}
	select {
	case v := <-ch:
		return v, nil
	case <-panicked:
		return zero, ErrPanicked
	case <-a.closedCh:
		select {
		case v := <-ch:
			return v, nil
		default:
			return zero, ErrNotRunning
		}
	}
}

func (a *App) wakeIfRunning() {
	a.loop.mu.Lock()
	running := a.loop.running
	a.loop.mu.Unlock()
	if running {
		a.b.Wake()
	}
}
