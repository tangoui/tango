package ui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/uithread"
)

// threadWin is a test Window whose Invalidate, like a real host's, may be
// called from any goroutine; it counts the calls.
type threadWin struct {
	damageWin
	invalidates atomic.Int32
}

func (w *threadWin) Invalidate() { w.invalidates.Add(1) }

func newThreadWin() *threadWin { return &threadWin{damageWin: *newWin()} }

// startUIThread starts a UI thread that the test stops when it ends.
func startUIThread(t *testing.T) *uithread.Thread {
	t.Helper()
	th := uithread.Start(t.Name())
	t.Cleanup(func() {
		th.Stop()
		<-th.Done()
	})
	return th
}

func TestPostAfterCloseDropped(t *testing.T) {
	for _, bound := range []bool{false, true} {
		r := NewRunner(func(*Context) {}, RunnerOptions{})
		var th *uithread.Thread
		if bound {
			th = startUIThread(t)
			r.BindThread(th)
		}
		w := newThreadWin()
		r.Attach(w)
		r.Close()
		ran := false
		r.Post(func() { ran = true })
		if th != nil {
			th.Do(func() {}) // FIFO: a task Post queued has run by now
		}
		if ran {
			t.Fatalf("bound=%v: Post ran fn after Close", bound)
		}
		if n := w.invalidates.Load(); n != 0 {
			t.Fatalf("bound=%v: Post after Close requested %d frames", bound, n)
		}
	}

	// Posted before Close, run after it: dropped too.
	th := startUIThread(t)
	r := NewRunner(func(*Context) {}, RunnerOptions{})
	r.BindThread(th)
	w := newThreadWin()
	r.Attach(w)
	gate := make(chan struct{})
	th.Post(func() { <-gate }) // hold the thread
	ran := false
	r.Post(func() { ran = true })
	r.Close()
	close(gate)
	th.Do(func() {})
	if ran {
		t.Fatal("a task posted before Close ran after it")
	}
	if n := w.invalidates.Load(); n != 0 {
		t.Fatalf("dropped task requested %d frames", n)
	}
}

func TestPostFromUIThreadRunsAfterFrame(t *testing.T) {
	th := startUIThread(t)
	var r *Runner
	posted, ran := false, false
	r = NewRunner(func(c *Context) {
		if !posted {
			posted = true
			r.Post(func() { ran = true })
		}
	}, RunnerOptions{})
	r.BindThread(th)
	w := newThreadWin()
	r.Attach(w)

	var err error
	ranInFrame := false
	var before int32
	th.Do(func() {
		err = r.Frame()
		ranInFrame = ran
		before = w.invalidates.Load() // the posted task is still queued
	})
	if err != nil {
		t.Fatal(err)
	}
	if !posted {
		t.Fatal("the page did not run")
	}
	if ranInFrame {
		t.Fatal("fn posted from the page ran before Frame returned")
	}
	th.Do(func() {}) // FIFO: the posted task runs first
	if !ran {
		t.Fatal("fn posted from the page did not run after the frame")
	}
	if w.invalidates.Load() <= before {
		t.Fatal("the posted task did not request a frame")
	}
}

func TestPostUnboundRunsInline(t *testing.T) {
	r := NewRunner(func(*Context) {}, RunnerOptions{})
	w := newThreadWin()
	r.Attach(w)
	ran := false
	r.Post(func() {
		ran = true
		if n := w.invalidates.Load(); n != 0 {
			t.Errorf("frame requested before fn ran (%d)", n)
		}
	})
	if !ran {
		t.Fatal("unbound Post did not run fn before returning")
	}
	if n := w.invalidates.Load(); n != 1 {
		t.Fatalf("unbound Post requested %d frames, want 1", n)
	}
}

func TestInvalidateSafeFromAnyGoroutine(t *testing.T) {
	th := startUIThread(t)
	r := NewRunner(func(c *Context) { Div(c) }, RunnerOptions{})
	r.BindThread(th)
	w := newThreadWin()
	r.Attach(w)

	const goroutines, calls = 8, 100
	var posted atomic.Int32
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range calls {
				r.Invalidate()
				r.Post(func() { posted.Add(1) })
			}
		}()
	}
	wg.Add(1)
	go func() { // frames run on the UI thread meanwhile
		defer wg.Done()
		for range 20 {
			th.Do(func() {
				if err := r.Frame(); err != nil {
					t.Error(err)
				}
			})
		}
	}()
	wg.Wait()
	th.Do(func() {}) // run the tasks still queued
	if n := posted.Load(); n != goroutines*calls {
		t.Fatalf("%d posted tasks ran, want %d", n, goroutines*calls)
	}
	if n := w.invalidates.Load(); n < 2*goroutines*calls {
		t.Fatalf("%d Invalidate calls reached the window, want >= %d", n, 2*goroutines*calls)
	}
}

// posterWin is a threadWin whose host posts UI work (UIPoster): PostUI
// records fn without running it, or forwards it to fwd when set.
type posterWin struct {
	threadWin
	mu     sync.Mutex
	queued []func()
	fwd    chan func()
}

func (w *posterWin) PostUI(fn func()) {
	if w.fwd != nil {
		w.fwd <- fn
		return
	}
	w.mu.Lock()
	w.queued = append(w.queued, fn)
	w.mu.Unlock()
}

// runQueued runs the recorded fns, as the host's UI thread would.
func (w *posterWin) runQueued() {
	w.mu.Lock()
	q := w.queued
	w.queued = nil
	w.mu.Unlock()
	for _, fn := range q {
		fn()
	}
}

func TestPostUnboundWithPosterQueues(t *testing.T) {
	r := NewRunner(func(*Context) {}, RunnerOptions{})
	w := &posterWin{threadWin: threadWin{damageWin: *newWin()}}
	r.Attach(w)
	ran := false
	r.Post(func() { ran = true })
	if ran {
		t.Fatal("Post ran fn inline although the window posts UI work")
	}
	if n := w.invalidates.Load(); n != 0 {
		t.Fatalf("Post requested %d frames before fn ran", n)
	}
	w.runQueued()
	if !ran {
		t.Fatal("the posted fn did not run on the host's UI thread")
	}
	if n := w.invalidates.Load(); n != 1 {
		t.Fatalf("the posted fn requested %d frames, want 1", n)
	}

	// Queued, then Close, then run: dropped.
	ran = false
	r.Post(func() { ran = true })
	r.Close()
	w.runQueued()
	if ran {
		t.Fatal("a fn posted before Close ran after it")
	}
	if n := w.invalidates.Load(); n != 1 {
		t.Fatalf("the dropped fn requested a frame (%d)", n)
	}
}

func TestPostUnboundWithPosterFromGoroutine(t *testing.T) {
	r := NewRunner(func(c *Context) { Div(c) }, RunnerOptions{})
	w := &posterWin{threadWin: threadWin{damageWin: *newWin()}, fwd: make(chan func())}
	r.Attach(w)

	const goroutines, calls = 8, 100
	done := make(chan struct{})
	count := 0  // touched only by the "UI" goroutine below
	go func() { // the host's UI thread
		defer close(done)
		for fn := range w.fwd {
			fn()
			if err := r.Frame(); err != nil {
				t.Error(err)
			}
		}
	}()
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range calls {
				r.Post(func() { count++ })
			}
		}()
	}
	wg.Wait()
	close(w.fwd)
	<-done
	if count != goroutines*calls {
		t.Fatalf("%d posted fns ran, want %d", count, goroutines*calls)
	}
}

func TestBindThreadAfterAttachPanics(t *testing.T) {
	th := startUIThread(t)
	r := NewRunner(func(*Context) {}, RunnerOptions{})
	r.Attach(newWin())
	defer func() {
		v := recover()
		if v == nil {
			t.Fatal("BindThread after Attach did not panic")
		}
		if msg := fmt.Sprint(v); !strings.Contains(msg, "Attach") {
			t.Fatalf("panic %q does not mention Attach", msg)
		}
	}()
	r.BindThread(th)
}

// A Post that finds the bound thread already stopped while the runner is
// still open (the host is handing the window back to its main thread) goes
// through the window's UIPoster instead of being dropped.
func TestPostToStoppedThreadFallsBackToPoster(t *testing.T) {
	r := NewRunner(func(*Context) {}, RunnerOptions{})
	th := uithread.Start(t.Name())
	r.BindThread(th)
	w := &posterWin{threadWin: threadWin{damageWin: *newWin()}}
	r.Attach(w)
	th.Stop()
	<-th.Done()
	ran := false
	r.Post(func() { ran = true })
	w.runQueued()
	if !ran {
		t.Fatal("Post to a stopped thread was dropped while the runner is open")
	}
	r.Close()
	ran = false
	r.Post(func() { ran = true })
	w.runQueued()
	if ran {
		t.Fatal("Post after Close ran")
	}
}

// A Post the stopped thread refused is dropped when the window has no
// UIPoster, even if the host cleared the binding meanwhile: it never runs
// on the posting goroutine.
func TestFailedPostWithoutPosterDropped(t *testing.T) {
	r := NewRunner(func(*Context) {}, RunnerOptions{})
	th := uithread.Start(t.Name())
	r.BindThread(th)
	r.Attach(newThreadWin()) // no UIPoster
	th.Stop()
	<-th.Done()
	afterFailedPost = func(r *Runner) { corehook.ReturnRunnerToMain(r) } // the handover races the Post
	t.Cleanup(func() { afterFailedPost = nil })
	ran := false
	r.Post(func() { ran = true })
	if ran {
		t.Fatal("a refused Post ran inline on the posting goroutine")
	}
	afterFailedPost = nil
	// Unbound from the start and no poster: the headless inline path.
	h := NewRunner(func(*Context) {}, RunnerOptions{})
	h.Attach(newThreadWin())
	h.Post(func() { ran = true })
	if !ran {
		t.Fatal("headless Post did not run inline")
	}
}
