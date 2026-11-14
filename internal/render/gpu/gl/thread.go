//go:build linux || darwin

package gl

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// glThread runs functions on one OS thread: a goroutine locked to its
// thread for its whole life, so a GL context made current there stays
// current for every call. A headless context owns one.
type glThread struct {
	calls     chan func()
	closeOnce sync.Once
	gone      chan struct{} // closed when the goroutine has exited through runtime.Goexit
	dead      atomic.Bool   // gone is closed
}

// errThreadGone is the panic value of do on a thread whose goroutine a
// function ended with runtime.Goexit.
const errThreadGone = "gl: GL thread ended by runtime.Goexit inside do (t.FailNow in a GL call?); its context is unusable"

// newGLThread starts the thread.
func newGLThread() *glThread {
	t := &glThread{calls: make(chan func()), gone: make(chan struct{})}
	go t.loop()
	return t
}

func (t *glThread) loop() {
	// Never unlocked: when the goroutine exits after close, Go ends the
	// OS thread with it instead of reusing a thread a context was current
	// on.
	runtime.LockOSThread()
	for fn := range t.calls {
		fn()
	}
}

// do runs fn on the thread and returns when it has run; a panic in fn is
// re-raised in the caller (the thread keeps running). Calls from several
// goroutines run one at a time. fn must not call do itself (it would wait
// for itself), and do must not be called after close.
//
// When fn calls runtime.Goexit (t.FailNow, t.Fatal, t.SkipNow in a test)
// the thread's goroutine ends with it, and with it the OS thread and the
// context current there. do then calls runtime.Goexit in the caller too,
// so the caller's goroutine ends as fn asked for (the test stops as
// failed). The thread is dead: every later do panics with errThreadGone
// instead of waiting forever (alive reports it), and close still returns.
func (t *glThread) do(fn func()) {
	if t.dead.Load() {
		panic(errThreadGone)
	}
	var (
		p        any
		panicked bool
		returned bool
	)
	done := make(chan struct{})
	call := func() {
		defer func() {
			if r := recover(); r != nil {
				p, panicked = r, true
			} else if !returned {
				// Neither returned nor panicked: runtime.Goexit, which
				// keeps unwinding this goroutine after the defer.
				t.dead.Store(true)
				close(t.gone)
			}
			close(done)
		}()
		fn()
		returned = true
	}
	select {
	case t.calls <- call:
	case <-t.gone:
		panic(errThreadGone)
	}
	<-done
	switch {
	case panicked:
		panic(p)
	case !returned:
		runtime.Goexit()
	}
}

// alive reports whether the thread still runs calls (no function ended it
// with runtime.Goexit).
func (t *glThread) alive() bool { return !t.dead.Load() }

// close stops the thread once the calls already running have returned.
func (t *glThread) close() {
	t.closeOnce.Do(func() { close(t.calls) })
}
