package uihost_test

import (
	"errors"
	"image"
	"testing"
	"testing/synctest"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/apphook"

	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/dialog"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/uihost"
)

func TestResizeWaitsForNewSizeFrame(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "cpu")
	a, fb := faketest.Run(t, tango.Options{})
	_, _, tc, cv := openThreaded(t, a, fb, func(*ui.Context) {})
	tc.flush(t)
	presented := make(chan image.Point, 4)
	tc.mu.Lock()
	tc.eager = true
	tc.presentHook = func(img *image.RGBA) {
		// Returning from resize when the frame merely starts is too soon.
		time.Sleep(4 * time.Millisecond)
		presented <- img.Bounds().Size()
	}
	tc.mu.Unlock()
	if !cv.Resize(120, 80, 2) {
		t.Fatal("resize did not run")
	}
	select {
	case got := <-presented:
		if got != image.Pt(240, 160) {
			t.Fatalf("presented %v, want 240x160", got)
		}
	default:
		t.Fatal("resize returned before the new-size Present finished")
	}
}

func TestResizeGivesUpAfterOneRefresh(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	_, _, tc, cv := openThreaded(t, a, fb, func(*ui.Context) {})
	release := blockUI(t, tc)
	defer release()
	var elapsed time.Duration
	if !fb.Do(func() {
		start := time.Now()
		cv.Resize(120, 80, 1)
		elapsed = time.Since(start)
	}) {
		t.Fatal("resize did not run")
	}
	if elapsed < 12*time.Millisecond || elapsed >= 50*time.Millisecond {
		t.Fatalf("blocked resize waited %s, want about 16.7 ms", elapsed)
	}
}

func TestResizeWhileUIThreadAwaitsMain(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, _, tc, cv := openThreaded(t, a, fb, func(*ui.Context) {})
	result := make(chan struct {
		wait bool
		time.Duration
	}, 1)
	r.Post(func() {
		tango.Await(a, func() bool {
			start := time.Now()
			cv.Resize(120, 80, 1)
			result <- struct {
				wait bool
				time.Duration
			}{tc.th.AwaitingMain(), time.Since(start)}
			return true
		})
	})
	select {
	case got := <-result:
		if !got.wait || got.Duration >= 12*time.Millisecond {
			t.Fatalf("resize with Await in flight: marked=%t, duration=%s", got.wait, got.Duration)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resize deadlocked with Await")
	}
}

func lifecycleDone(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal(what)
	}
}

func TestCloseDoesNotWait(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	hooks(t, g)
	keep := false
	a, fb := faketest.Run(t, tango.Options{QuitOnLastClose: &keep})
	fb.EnableCanvasMetal(0)
	r, s, tc, _ := openThreaded(t, a, fb, func(*ui.Context) {})
	release := blockUI(t, tc)
	defer release()
	native := make(chan bool, 1)
	fb.Do(func() {
		tc.releaseHook = func() {
			select {
			case <-tc.th.Done():
				native <- fb.IsMainThread() && g.closeCount() == 1
			default:
				native <- false
			}
		}
	})
	queued := make(chan struct{}, 1)
	r.Post(func() { queued <- struct{}{} })
	closed := make(chan struct{})
	go func() { fb.RequestClose(s.ID()); close(closed) }()
	lifecycleDone(t, closed, "close waited for UI")
	if g.closeCount() != 0 {
		t.Fatal("GPU released while the UI thread was still blocked")
	}
	r.Post(func() { queued <- struct{}{} })
	release()
	lifecycleDone(t, tc.th.Done(), "UI thread did not exit after close")
	if len(queued) != 0 {
		t.Fatal("queued work ran after close")
	}
	select {
	case ordered := <-native:
		if !ordered {
			t.Fatal("native release did not run on main after renderer release and Done")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("native release was not posted after UI exit")
	}
}

func TestCloseWhileUIThreadAwaitsMain(t *testing.T) {
	keep := false
	a, fb := faketest.NewApp(tango.Options{QuitOnLastClose: &keep})
	observed := make(chan bool, 1)
	var tc *threadedCanvas
	fb.OnMessageDialog = func(id platform.SurfaceID, _ platform.MessageOptions) (int, error) {
		waiting := tc.th.AwaitingMain()
		observed <- fb.RequestClose(id) && waiting
		return 0, nil
	}
	faketest.Start(t, a, fb)
	r, s, canvas, _ := openThreaded(t, a, fb, func(*ui.Context) {})
	tc = canvas
	answer := make(chan error, 1)
	r.Post(func() {
		_, err := dialog.Message(a, dialog.MessageOptions{Parent: s, Buttons: []string{"OK"}})
		answer <- err
	})
	lifecycleDone(t, tc.th.Done(), "close during a dialog deadlocked")
	if !<-observed {
		t.Error("dialog was not marked as waiting on the main thread when closed")
	}
	if err := <-answer; err != nil {
		t.Fatal(err)
	}
}

func TestQuitWaitsAtMostOneSecond(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	canvases := useThreadedCanvases(t, false, clockCfg{})
	terminated := make(chan struct{})
	a.OnLifecycle(func(ev tango.Lifecycle) {
		if ev == tango.Terminating {
			close(terminated)
		}
	})
	ready := make(chan struct{})
	a.OnReady(func() { close(ready) })
	run := make(chan error, 1)
	go func() { run <- a.Run() }()
	t.Cleanup(a.Quit)
	lifecycleDone(t, ready, "app did not start")
	open := func() *threadedCanvas {
		t.Helper()
		id := tango.Await(a, func() platform.SurfaceID {
			_, s, err := uihost.Open(a, uihost.Options{}, func(*ui.Context) {}, ui.RunnerOptions{})
			if err != nil {
				return 0
			}
			return s.ID()
		})
		tc := canvases(id)
		if tc == nil {
			t.Fatal("no threaded canvas")
		}
		t.Cleanup(tc.th.Stop)
		return tc
	}
	stuck1, stuck2, waiting := open(), open(), open()
	for _, tc := range []*threadedCanvas{stuck1, stuck2} {
		release := blockUI(t, tc) // would stay blocked well past the deadline
		defer release()
	}
	waitResult := make(chan bool, 1)
	native := make(chan bool, 1)
	fb.Do(func() {
		waiting.releaseHook = func() {
			select {
			case <-terminated:
				native <- fb.IsMainThread()
			default:
				native <- false
			}
		}
	})
	awaitTaskStarted := make(chan struct{})
	waiting.th.Post(func() {
		close(awaitTaskStarted)
		<-terminated // the fake has stopped draining main tasks at this point
		waitResult <- tango.Await(a, func() bool { return true })
	})
	lifecycleDone(t, awaitTaskStarted, "waiting task did not start")
	start := time.Now()
	a.Quit()
	select {
	case err := <-run:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(1800 * time.Millisecond):
		t.Fatal("quit waited beyond the shared 1s budget")
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond || elapsed >= 1500*time.Millisecond {
		t.Errorf("quit took %s, want about 1s total for both blocked threads", elapsed)
	}
	select {
	case ran := <-waitResult:
		if ran {
			t.Error("stranded main task ran after the main loop exited")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("UI thread waiting on main was not released")
	}
	select {
	case ok := <-native:
		if !ok {
			t.Error("quit did not release completed native canvas on main after Terminating")
		}
	default:
		t.Error("blocked windows prevented release of a completed window on quit")
	}
	for _, tc := range []*threadedCanvas{stuck1, stuck2} {
		if tc.th.Post(func() {}) {
			t.Error("quit did not stop a blocked UI thread")
		}
	}
}

func TestResizeReentrantKeepsLatestGeometry(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "cpu")
	a, fb := faketest.Run(t, tango.Options{})
	_, s, tc, cv := openThreaded(t, a, fb, func(*ui.Context) {})
	presented := make(chan image.Point, 1)
	tc.mu.Lock()
	tc.presentHook = func(img *image.RGBA) { presented <- img.Bounds().Size() }
	tc.mu.Unlock()
	s.OnResized(func(w, h int) {
		if w == 120 && h == 80 {
			cv.Resize(150, 90, 2) // delivered to the sink before the outer resize
		}
	})
	if !cv.Resize(120, 80, 1) || !tc.flush(t) {
		t.Fatal("resize did not produce a frame")
	}
	if got := <-presented; got != image.Pt(300, 180) {
		t.Errorf("last Present = %v, want latest geometry 300x180", got)
	}
}

func TestQuitStopsQueuedRunnerWorkBeforeUnblockingAwait(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var tc *threadedCanvas
	var waitersReleasedFirst bool
	// Installed before the window's quit notification. If shutdown already
	// woke the UI, let it consume its queue before the window can stop it.
	fb.Do(func() {
		corehook.OnQuit(a, func() {
			select {
			case <-corehook.Done(a):
				waitersReleasedFirst = true
				tc.th.Do(func() {})
			default:
			}
		})
	})
	r, _, canvas, _ := openThreaded(t, a, fb, func(*ui.Context) {})
	tc = canvas
	started := make(chan struct{})
	result := make(chan error, 1)
	r.Post(func() {
		_, err := tango.TryAwaitAsync(a, func(func(bool)) { close(started) })
		result <- err
	})
	lifecycleDone(t, started, "async wait did not start")
	queued := make(chan struct{}, 1)
	r.Post(func() { queued <- struct{}{} })
	a.Quit()
	lifecycleDone(t, tc.th.Done(), "quit did not unblock the UI thread")
	if err := <-result; !errors.Is(err, tango.ErrNotRunning) {
		t.Errorf("await returned %v, want ErrNotRunning", err)
	}
	if waitersReleasedFirst || len(queued) != 0 {
		t.Errorf("waiters released before stop=%t; queued business tasks run=%d", waitersReleasedFirst, len(queued))
	}
}

func TestResizeStopsWaitingWhenUIThreadStartsAwait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, fb := faketest.Run(t, tango.Options{})
		r, _, tc, cv := openThreaded(t, a, fb, func(*ui.Context) {})
		tc.flush(t)
		enterAwait := make(chan struct{})
		awaited := make(chan struct{})
		r.Post(func() {
			<-enterAwait
			tango.Await(a, func() bool { return true })
			close(awaited)
		})
		resized := make(chan bool, 1)
		go func() { resized <- cv.Resize(120, 80, 1) }()
		synctest.Wait() // main and UI are both blocked; virtual time has not advanced
		select {
		case <-resized:
			close(enterAwait)
			t.Fatal("main skipped waiting before the UI started Await")
		default:
		}
		start := time.Now()
		close(enterAwait)
		if !<-resized {
			t.Fatal("resize did not run")
		}
		if elapsed := time.Since(start); elapsed != 0 {
			t.Errorf("resize waited %v after UI entered Await, want no timeout", elapsed)
		}
		lifecycleDone(t, awaited, "UI did not finish its main-thread call")
	})
}

func TestQuitDoesNotReleaseNewRejectedAwaitBeforeStoppingRunner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a, fb := faketest.Run(t, tango.Options{})
		enterAwait := make(chan struct{})
		result := make(chan error, 1)
		queued := make(chan struct{}, 1)
		var ranEarly bool
		fb.Do(func() {
			corehook.OnQuit(a, func() {
				// The main queue is sealed but the release barrier is
				// not open yet. A rejected Await must still wait.
				close(enterAwait)
				synctest.Wait()
				ranEarly = len(result) != 0 || len(queued) != 0
			})
		})
		r, _, tc, _ := openThreaded(t, a, fb, func(*ui.Context) {})
		r.Post(func() {
			<-enterAwait
			_, err := tango.TryAwaitAsync(a, func(func(bool)) {
				t.Error("late Await started work on the main thread")
			})
			result <- err
		})
		r.Post(func() { queued <- struct{}{} })
		synctest.Wait() // the first task must have passed the runner's closed check
		a.Quit()
		lifecycleDone(t, tc.th.Done(), "quit did not unblock the late Await")
		if err := <-result; !errors.Is(err, tango.ErrNotRunning) {
			t.Errorf("late Await returned %v, want ErrNotRunning", err)
		}
		if ranEarly || len(queued) != 0 {
			t.Errorf("rejected Await returned before stop=%t; queued business tasks run=%d", ranEarly, len(queued))
		}
	})
}

// quitDialogBackend models both a late start in the current main queue
// drain and a previously accepted dialog canceled before Run returns.
type quitDialogBackend struct {
	*fake.Backend
	start func(func(int, error))
}

func (b quitDialogBackend) MessageDialog(_ platform.SurfaceID, _ platform.MessageOptions, done func(int, error)) {
	b.start(done)
}

func TestQuitStopsRunnerBeforeDialogCancellation(t *testing.T) {
	for _, mode := range []string{"late-start", "native-quit-active", "normal-cancel"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fb := fake.New()
				started := make(chan struct{})
				cancelled := make(chan struct{})
				var cancel func()
				b := quitDialogBackend{fb, func(done func(int, error)) {
					cancel = func() {
						done(-1, platform.ErrCanceled)
						synctest.Wait() // let the UI resume before backend shutdown continues
						close(cancelled)
					}
					close(started)
					if mode == "late-start" {
						cancel()
					}
				}}
				a := apphook.NewApp(b, tango.Options{}).(*tango.App)
				if mode == "native-quit-active" {
					a.OnLifecycle(func(ev tango.Lifecycle) {
						if ev == tango.Terminating {
							cancel() // backend is quitting; App.shutdown has not started
						}
					})
				}
				faketest.Start(t, a, fb)
				r, _, _, _ := openThreaded(t, a, fb, func(*ui.Context) {})
				answer := make(chan error, 1)
				queued := make(chan struct{}, 1)
				mainGate := make(chan struct{})
				if mode == "late-start" {
					a.Post(func() { <-mainGate })
					a.Quit() // ahead of the dialog start in the same App.Post drain
				}
				r.Post(func() {
					_, err := dialog.Message(a, dialog.MessageOptions{Buttons: []string{"Continue", "Stop"}})
					answer <- err
				})
				r.Post(func() { queued <- struct{}{} })
				synctest.Wait()
				close(mainGate)
				lifecycleDone(t, started, "dialog start did not run")
				switch mode {
				case "native-quit-active":
					if !fb.RequestQuit() {
						t.Fatal("native quit was not accepted")
					}
				case "normal-cancel":
					fb.Do(cancel)
				}
				lifecycleDone(t, cancelled, "dialog was not canceled")
				if err := <-answer; !errors.Is(err, dialog.ErrCanceled) {
					t.Errorf("dialog returned %v, want ErrCanceled", err)
				}
				if ran := len(queued) != 0; ran != (mode == "normal-cancel") {
					t.Errorf("queued Runner.Post ran=%t after cancellation (%s)", ran, mode)
				}
			})
		})
	}
}
