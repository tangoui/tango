package uihost_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/uihost"
)

// Embedded WebViews of threaded windows (design §5.6): the UI thread
// decides the placements of each frame, the main thread applies them.

// webPage builds a WebView below a spacer of height *top, set on the UI
// thread.
func webPage(top *float32, w **uihost.WebViewW) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c, func() {
			ui.Div(c, layout.H(*top))
			*w = uihost.WebView(c, "web", uihost.WebViewOptions{URL: "/page.html"}, layout.W(200), layout.H(100))
		}, layout.Pad(10))
	}
}

// frameRegion returns the frame's only native region as a placement.
func frameRegion(t *testing.T, tc *threadedCanvas, r *ui.Runner) platform.WebPlacement {
	t.Helper()
	var regs []ui.NativeRegion
	tc.th.Do(func() { regs = r.NativeRegions() })
	if len(regs) != 1 {
		t.Fatalf("native regions %v, want one", regs)
	}
	rf := func(r geom.Rect) platform.RectF {
		return platform.RectF{X: float64(r.X), Y: float64(r.Y), W: float64(r.W), H: float64(r.H)}
	}
	return platform.WebPlacement{Frame: rf(regs[0].Rect), Clip: rf(regs[0].Visible)}
}

// threadedEmbed returns the fake WebView behind w, read on the UI thread.
func threadedEmbed(t *testing.T, fb *fake.Backend, tc *threadedCanvas, w **uihost.WebViewW) *fake.Embedded {
	t.Helper()
	var s *tango.Surface
	tc.th.Do(func() { s = (*w).Surface() })
	if s == nil {
		t.Fatal("no WebView surface on the UI side")
	}
	e := fb.Embedded(s.ID())
	if e == nil {
		t.Fatal("no fake embedded view")
	}
	return e
}

func TestWebViewPlacementPostedEachFrame(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	top := float32(50)
	var w *uihost.WebViewW
	r, s, tc, _ := openThreaded(t, a, fb, webPage(&top, &w))
	tasks := func() int { return tango.Await(a, func() int { return uihost.PlacementTasks(r) }) }
	tc.flush(t)
	syncAppPosts(a) // creates the WebView and places it
	tc.sync(t)      // the UI side learns its surface
	e := threadedEmbed(t, fb, tc, &w)
	if p, want := e.Placement(), frameRegion(t, tc, r); p != want || p.Frame != (platform.RectF{X: 10, Y: 60, W: 200, H: 100}) {
		t.Fatalf("first placement %+v, want %+v", p, want)
	}
	if !uihost.ThreadedStarted(r) {
		t.Fatal("a window with a WebView left its UI thread")
	}
	n := tasks()
	for i := range 3 {
		r.Post(func() { top += 10 })
		if !tc.flush(t) {
			t.Fatal("no frame")
		}
		syncAppPosts(a)
		if got := tasks(); got != n+i+1 {
			t.Fatalf("frame %d: %d placement tasks, want %d", i, got-n, i+1)
		}
		if p, want := e.Placement(), frameRegion(t, tc, r); p != want {
			t.Fatalf("frame %d: placement %+v, want this frame's region %+v", i, p, want)
		}
	}
	// Frames queued while main is busy are applied once, the latest wins.
	n = tasks()
	release := blockMain(t, a)
	for range 3 {
		r.Post(func() { top += 5 })
		tc.flush(t)
	}
	release()
	syncAppPosts(a)
	if got := tasks(); got != n+1 {
		t.Fatalf("three queued frames applied as %d tasks, want 1", got-n)
	}
	if p, want := e.Placement(), frameRegion(t, tc, r); p != want {
		t.Fatalf("after coalescing: placement %+v, want the last frame's %+v", p, want)
	}
	// A placement still queued when the window closes is dropped.
	last := e.Placement()
	gate := make(chan struct{})
	a.Post(func() { <-gate; s.Close() })
	r.Post(func() { top += 5 })
	tc.flush(t)
	close(gate)
	syncAppPosts(a)
	if p := e.Placement(); p != last {
		t.Fatalf("placement after close %+v, want %+v kept", p, last)
	}
}

// With threadedNativeViews off, the first frame with a native view hands
// the window back to the main-thread model, once, through the startup
// fallback: work posted meanwhile is kept, and the page, its WebView and
// Runner.Post run on main afterwards.
func TestNativeViewWindowCanOptOut(t *testing.T) {
	t.Cleanup(uihost.SetThreadedNativeViews(false))
	a, fb := faketest.Run(t, tango.Options{})
	var show atomic.Bool
	var pageGID atomic.Uint64
	var w *uihost.WebViewW
	top := float32(0)
	web := webPage(&top, &w)
	r, s, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) {
		pageGID.Store(uithread.GoroutineID())
		if show.Load() {
			web(c)
		}
	})
	var cancels atomic.Int32
	tc.cancelHook = func() { cancels.Add(1) }
	tc.flush(t)
	if g := pageGID.Load(); g != tc.gid {
		t.Fatalf("page ran on %d before the native view, UI thread %d", g, tc.gid)
	}
	show.Store(true)
	r.Post(func() {})
	tc.flush(t) // the first frame with a native view
	var mu sync.Mutex
	var postGID uint64
	r.Post(func() { mu.Lock(); postGID = uithread.GoroutineID(); mu.Unlock() })
	select {
	case <-tc.th.Done():
	case <-time.After(faketest.Timeout):
		t.Fatal("UI thread did not exit after the switch")
	}
	main := mainGID(a)
	cv := fb.Canvas(s.ID())
	for deadline := time.Now().Add(faketest.Timeout); time.Now().Before(deadline); {
		fb.Do(func() {})
		settle(cv)
		if pageGID.Load() == main && tango.Await(a, func() bool { return w.Surface() != nil }) {
			break
		}
	}
	if !uihost.MainFallback(r) || uihost.ThreadedStarted(r) {
		t.Fatal("window not back in the main-thread model")
	}
	if g := pageGID.Load(); g != main {
		t.Fatalf("page runs on %d after the switch, main is %d", g, main)
	}
	mu.Lock()
	pg := postGID
	mu.Unlock()
	if pg != main {
		t.Fatalf("Runner.Post during the switch ran on %d, main is %d", pg, main)
	}
	e := embedOf(t, a, fb, &w)
	if p := e.Placement(); p.Frame != (platform.RectF{X: 10, Y: 10, W: 200, H: 100}) {
		t.Fatalf("WebView on main placed at %+v", p)
	}
	if _, ran := postFromGoroutine(t, r); ran != main {
		t.Fatalf("Runner.Post after the switch ran on %d, main is %d", ran, main)
	}
	if n := cancels.Load(); n != 1 {
		t.Fatalf("native rollback ran %d times, want 1", n)
	}
}

// A shortcut pressed while a window is being handed back to main (R2)
// fires once: the buffered event does not match again on main.
func TestShortcutFiresOnceAcrossSwitchBack(t *testing.T) {
	t.Cleanup(uihost.SetThreadedNativeViews(false))
	a, fb := faketest.Run(t, tango.Options{})
	var show atomic.Bool
	var w *uihost.WebViewW
	top := float32(0)
	web := webPage(&top, &w)
	r, s, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) {
		if show.Load() {
			web(c)
		}
	})
	tc.flush(t)
	fired := 0
	shortcut(t, s, "CmdOrCtrl+K", uihost.ShortcutOptions{}, func() { fired++ })
	gate := make(chan struct{})
	a.Post(func() { // runs after the switch began, before the handover
		<-gate
		tc.sink.CanvasInput(input.KeyEvent{Key: input.KeyK, Mods: cmd(), Down: true})
		tc.sink.CanvasInput(input.KeyEvent{Key: input.KeyK, Mods: cmd()})
	})
	show.Store(true)
	r.Post(func() {})
	tc.flush(t) // the switch frame posts the handover behind the gate
	close(gate)
	<-tc.th.Done()
	cv := fb.Canvas(s.ID())
	for deadline := time.Now().Add(faketest.Timeout); !uihost.MainFallback(r) && time.Now().Before(deadline); {
		fb.Do(func() {})
	}
	settle(cv)
	fb.Do(func() {
		if fired != 1 {
			t.Errorf("shortcut fired %d times across the switch, want 1", fired)
		}
	})
}

// A threaded window without WebViews queues no placement work on main.
func TestWindowWithoutWebViewQueuesNoPlacementTasks(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	n := 0
	r, _, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) { ui.Text(c, "no web") })
	for range 4 {
		r.Post(func() { n++ })
		tc.flush(t)
	}
	syncAppPosts(a)
	if got := tango.Await(a, func() int { return uihost.PlacementTasks(r) }); got != 0 {
		t.Fatalf("%d placement tasks for a window without WebViews", got)
	}
}

// Frames coalesced while main is busy keep the focus request one of them
// made: the page gets focus once.
func TestWebViewCoalescedFrameKeepsFocus(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var w *uihost.WebViewW
	r, s, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) {
		ui.Column(c, func() {
			ui.Button(c, "before")
			w = uihost.WebView(c, "web", uihost.WebViewOptions{}, layout.W(200), layout.H(100))
		})
	})
	fb.EmitFocus(s.ID(), true)
	for tc.flush(t) {
	}
	syncAppPosts(a)
	tc.sync(t)
	e := threadedEmbed(t, fb, tc, &w)
	tab := func() {
		tc.th.Do(func() {
			r.HandleEvent(input.KeyEvent{Key: input.KeyTab, Down: true})
			r.HandleEvent(input.KeyEvent{Key: input.KeyTab})
		})
	}
	tab() // the button
	for tc.flush(t) {
	}
	syncAppPosts(a)
	release := blockMain(t, a)
	tab() // the WebView box: a focus request, then more frames
	for tc.flush(t) {
	}
	r.Post(func() {})
	tc.flush(t)
	release()
	syncAppPosts(a)
	if f, _ := e.FocusCalls(); f != 1 {
		t.Fatalf("FocusWeb calls = %d after coalesced frames, want 1", f)
	}
}
