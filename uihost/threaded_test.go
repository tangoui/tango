package uihost_test

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

// threadedCanvas wraps a fake canvas as a platform.CanvasThreaded. The
// frame clock never ticks by itself: a test runs the requested frame with
// flush (or a frame regardless with tick), on the clock's thread. Present
// only records, from any thread; SetCursor, SetTextInput and
// SetTextInputState record their goroutine and reach the fake canvas only
// on the main thread (off it the fake would panic).
type threadedCanvas struct {
	platform.Canvas
	sink     platform.CanvasSink
	clockErr error // returned by StartFrameClock
	eager    bool  // request queues the tick on the UI thread at once

	mu           sync.Mutex
	th           *uithread.Thread // the thread StartFrameClock ran on
	gid          uint64           // its goroutine id
	tickFn       func()
	pending      bool // request since the last frame
	requests     int
	requestPanic any
	stopped      bool
	presentHook  func(*image.RGBA)
	releaseHook  func()
	presents     int
	presentGID   uint64 // goroutine of the last Present
	metalGIDs    []uint64
	cursorGIDs   []uint64 // SetCursor callers
	textGIDs     []uint64 // SetTextInput callers
	reqGIDs      []uint64 // frame request callers
}

// UseMetalLayer implements platform.CanvasMetal through the fake canvas,
// recording the caller's goroutine. Called on a UI thread it fails, as on
// macOS, where the switch is main-thread only (the fake would panic).
func (c *threadedCanvas) UseMetalLayer() (uintptr, error) {
	c.mu.Lock()
	c.metalGIDs = append(c.metalGIDs, uithread.GoroutineID())
	c.mu.Unlock()
	if uithread.Current() != nil {
		return 0, errors.New("threadedCanvas: UseMetalLayer off the main thread")
	}
	return c.Canvas.(platform.CanvasMetal).UseMetalLayer()
}

// dragOver reports a file drag at (x, y) to the sink, on the main thread.
func (c *threadedCanvas) dragOver(fb *fake.Backend, x, y float32) {
	fb.Do(func() { c.sink.(platform.CanvasDropTarget).CanvasDragOver(x, y) })
}

func (c *threadedCanvas) StartFrameClock(tick func()) (request, stop func(), err error) {
	if c.clockErr != nil {
		return nil, nil, c.clockErr
	}
	th := uithread.Current()
	if th == nil {
		return nil, nil, errors.New("threadedCanvas: StartFrameClock off a UI thread")
	}
	c.mu.Lock()
	c.th, c.gid, c.tickFn = th, uithread.GoroutineID(), tick
	c.mu.Unlock()
	return c.request, c.stop, nil
}

func (c *threadedCanvas) request() {
	c.mu.Lock()
	c.pending = true
	c.requests++
	c.reqGIDs = append(c.reqGIDs, uithread.GoroutineID())
	eager, th, fn := c.eager, c.th, c.tickFn
	panicValue := c.requestPanic
	c.requestPanic = nil
	c.mu.Unlock()
	if panicValue != nil {
		panic(panicValue)
	}
	if eager {
		th.Post(func() {
			c.mu.Lock()
			c.pending = false
			c.mu.Unlock()
			fn()
		})
	}
}

func (c *threadedCanvas) stop() {
	c.mu.Lock()
	c.stopped = true
	c.mu.Unlock()
}

func (c *threadedCanvas) MainGeometry() (w, h, scale float32) {
	w, h = c.Canvas.Size()
	return w, h, c.Canvas.Scale()
}

func (c *threadedCanvas) Present(img *image.RGBA) error {
	c.mu.Lock()
	hook := c.presentHook
	c.mu.Unlock()
	if hook != nil {
		hook(img)
	}
	c.mu.Lock()
	c.presents++
	c.presentGID = uithread.GoroutineID()
	c.mu.Unlock()
	return nil
}

func (c *threadedCanvas) ReleaseThreadedOnMain() {
	if c.releaseHook != nil {
		c.releaseHook()
	}
}

func (c *threadedCanvas) SetCursor(cur input.Cursor) {
	c.mu.Lock()
	c.cursorGIDs = append(c.cursorGIDs, uithread.GoroutineID())
	c.mu.Unlock()
	if uithread.Current() == nil {
		c.Canvas.SetCursor(cur)
	}
}

func (c *threadedCanvas) SetTextInput(on bool, caret geom.Rect) {
	c.mu.Lock()
	c.textGIDs = append(c.textGIDs, uithread.GoroutineID())
	c.mu.Unlock()
	if uithread.Current() == nil {
		c.Canvas.SetTextInput(on, caret)
	}
}

func (c *threadedCanvas) SetShortcutCapture(on bool) {
	c.Canvas.(platform.CanvasShortcutCapture).SetShortcutCapture(on)
}

// started reports whether StartFrameClock succeeded.
func (c *threadedCanvas) started() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.th != nil
}

// tick runs one frame on the clock's thread and waits for it, after the
// tasks already queued there.
func (c *threadedCanvas) tick(t *testing.T) {
	t.Helper()
	c.mu.Lock()
	th, fn := c.th, c.tickFn
	c.pending = false
	c.mu.Unlock()
	if th == nil {
		t.Fatal("frame clock not started")
	}
	if !th.Do(fn) {
		t.Fatal("UI thread stopped")
	}
}

// flush runs a frame if one was requested and reports whether it did.
func (c *threadedCanvas) flush(t *testing.T) bool {
	t.Helper()
	c.sync(t) // forwarded tasks may still request one
	c.mu.Lock()
	p := c.pending
	c.mu.Unlock()
	if p {
		c.tick(t)
	}
	return p
}

// sync waits until the UI thread has run the tasks queued so far.
func (c *threadedCanvas) sync(t *testing.T) {
	t.Helper()
	c.mu.Lock()
	th := c.th
	c.mu.Unlock()
	if th == nil || !th.Do(func() {}) {
		t.Fatal("no running UI thread")
	}
}

// clockCfg configures the frame clock of threadedCanvases.
type clockCfg struct {
	err   error // StartFrameClock's result
	eager bool  // tick as soon as a frame is requested
}

// useThreadedCanvases makes the canvases of the following uihost.Open
// (and, with attached, uihost.Attach) calls threadedCanvases with clock
// cfg, and returns them by surface.
func useThreadedCanvases(t *testing.T, attached bool, cfg clockCfg) func(platform.SurfaceID) *threadedCanvas {
	t.Helper()
	var mu sync.Mutex
	byID := map[platform.SurfaceID]*threadedCanvas{}
	origNew, origAttach := corehook.NewCanvasSurface, corehook.AttachCanvas
	corehook.NewCanvasSurface = func(app, opts any, sink platform.CanvasSink) (any, platform.Canvas, error) {
		s, cv, e := origNew(app, opts, sink)
		if e != nil {
			return s, cv, e
		}
		tc := &threadedCanvas{Canvas: cv, sink: sink, clockErr: cfg.err, eager: cfg.eager}
		mu.Lock()
		byID[s.(*tango.Surface).ID()] = tc
		mu.Unlock()
		return s, tc, nil
	}
	if attached {
		corehook.AttachCanvas = func(s any, l platform.CanvasLayer, sink platform.CanvasSink) (platform.Canvas, error) {
			cv, e := origAttach(s, l, sink)
			if e != nil {
				return cv, e
			}
			tc := &threadedCanvas{Canvas: cv, sink: sink, clockErr: cfg.err, eager: cfg.eager}
			mu.Lock()
			byID[s.(*tango.Surface).ID()] = tc
			mu.Unlock()
			return tc, nil
		}
	}
	t.Cleanup(func() { corehook.NewCanvasSurface, corehook.AttachCanvas = origNew, origAttach })
	return func(id platform.SurfaceID) *threadedCanvas {
		mu.Lock()
		defer mu.Unlock()
		return byID[id]
	}
}

// openThreaded opens a window on a threadedCanvas without running a frame.
func openThreaded(t *testing.T, a *tango.App, fb *fake.Backend, page func(*ui.Context)) (*ui.Runner, *tango.Surface, *threadedCanvas, *fake.Canvas) {
	t.Helper()
	canvases := useThreadedCanvases(t, false, clockCfg{})
	o := tango.Await(a, func() opened {
		r, s, err := uihost.Open(a, uihost.Options{
			Surface: tango.SurfaceOptions{Title: "UI", Width: 200, Height: 100},
		}, page, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	fb.Do(func() {}) // the canvas's first CanvasFrame, queued on the main thread
	tc := canvases(o.s.ID())
	if tc == nil || !tc.started() {
		t.Fatal("no threaded canvas with a running frame clock")
	}
	closeAtEnd(t, fb, o.s, tc)
	return o.r, o.s, tc, fb.Canvas(o.s.ID())
}

// closeAtEnd closes a threaded window when the test ends and waits for its
// UI thread to exit. Windows of different tests must not draw at the same
// time: package-level layout and cache state is not safe for concurrent
// frames yet (plan Task 9), and the memory warning reaches the runners of
// every app.
func closeAtEnd(t *testing.T, fb *fake.Backend, s *tango.Surface, tc *threadedCanvas) {
	t.Helper()
	t.Cleanup(func() {
		fb.RequestClose(s.ID())
		select {
		case <-tc.th.Done():
		case <-time.After(faketest.Timeout):
			t.Error("UI thread did not exit after close")
		}
	})
}

// mainGID returns the goroutine id of a's main thread.
func mainGID(a *tango.App) uint64 { return tango.Await(a, uithread.GoroutineID) }

// postFromGoroutine calls r.Post from a new goroutine and returns the ids
// of that goroutine and of the one fn ran on.
func postFromGoroutine(t *testing.T, r *ui.Runner) (poster, ran uint64) {
	t.Helper()
	got := make(chan uint64, 1)
	posted := make(chan uint64)
	go func() {
		r.Post(func() { got <- uithread.GoroutineID() })
		posted <- uithread.GoroutineID()
	}()
	poster = <-posted
	select {
	case ran = <-got:
	case <-time.After(faketest.Timeout):
		t.Fatal("posted function did not run")
	}
	return poster, ran
}

func TestThreadedHostForwardsInputInOrder(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var mu sync.Mutex
	var got []input.Event
	var gids []uint64
	_, _, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) {
		b := ui.Div(c, layout.Size(50, 50))
		b.Focusable(input.CursorDefault)
		id := b.ID()
		c.SetFocus(id)
		c.TerminalEvents(id, func(ev input.Event) bool { // runs in Runner.HandleEvent
			mu.Lock()
			got = append(got, ev)
			gids = append(gids, uithread.GoroutineID())
			mu.Unlock()
			return true
		})
	})
	tc.tick(t) // the page focuses the box and registers the handler
	evs := []input.Event{
		input.Pointer{Kind: input.Move, Pos: geom.Pt(5, 6)},
		input.KeyEvent{Key: input.KeyA, Down: true},
		input.Scroll{Pos: geom.Pt(5, 6), DY: -3},
	}
	for _, ev := range evs {
		cv.Input(ev) // on the main thread, which only queues it
	}
	tc.sync(t)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != len(evs) {
		t.Fatalf("runner got %v, want %v", got, evs)
	}
	for i := range evs {
		if got[i] != evs[i] {
			t.Fatalf("event %d = %#v, want %#v", i, got[i], evs[i])
		}
		if gids[i] != tc.gid {
			t.Fatalf("event %d handled on goroutine %d, UI thread is %d", i, gids[i], tc.gid)
		}
	}
}

func TestEventsBeforeFirstFrameAreKept(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	type seen struct {
		size  geom.Size
		ptr   geom.Point
		theme *theme.Theme
	}
	var mu sync.Mutex
	var frames []seen
	r, _, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) {
		p, _ := c.Pointer()
		mu.Lock()
		frames = append(frames, seen{c.Size(), p, c.Theme})
		mu.Unlock()
	})
	dark := theme.AntDesign(theme.Dark())
	at := geom.Pt(30, 40)
	cv.Input(input.Pointer{Kind: input.Move, Pos: at})
	cv.Resize(400, 300, 1.5)
	r.Post(func() { r.SetTheme(dark) })
	tc.sync(t)
	mu.Lock()
	n := len(frames)
	mu.Unlock()
	if n != 0 {
		t.Fatalf("page ran %d times before the first frame", n)
	}
	if !tc.flush(t) {
		t.Fatal("no frame requested")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(frames) == 0 {
		t.Fatal("first frame did not run the page")
	}
	want := seen{geom.Size{W: 400, H: 300}, at, dark}
	if f := frames[0]; f != want {
		t.Fatalf("first frame saw %+v, want %+v", f, want)
	}
}

func TestNonThreadedCanvasKeepsMainThreadModel(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var mu sync.Mutex
	var pageGID uint64
	var onThread bool
	_, _, cv := open(t, a, fb, 100, 100, func(c *ui.Context) {
		mu.Lock()
		pageGID, onThread = uithread.GoroutineID(), uithread.Current() != nil
		mu.Unlock()
	})
	if cv.Presents() < 1 {
		t.Fatal("no frame on the main thread")
	}
	main := mainGID(a)
	mu.Lock()
	defer mu.Unlock()
	if pageGID != main || onThread {
		t.Fatalf("page ran on goroutine %d (UI thread: %v), main is %d", pageGID, onThread, main)
	}
}

func TestMainThreadWindowImplementsUIPoster(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, _, _ := open(t, a, fb, 100, 100, func(c *ui.Context) {})
	poster, ran := postFromGoroutine(t, r)
	if ran == poster {
		t.Fatal("Runner.Post ran fn inline on the posting goroutine")
	}
	if main := mainGID(a); ran != main {
		t.Fatalf("fn ran on goroutine %d, main is %d", ran, main)
	}
}

func TestAttachNeverBindsThread(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	canvases := useThreadedCanvases(t, true, clockCfg{})
	s := webSurface(t, a, 300, 200)
	r, _, _ := attach(t, a, fb, s, uihost.Layer{Above: true}, func(c *ui.Context) {})
	tc := canvases(s.ID())
	if tc == nil {
		t.Fatal("attached canvas was not wrapped")
	}
	if tc.started() {
		t.Fatal("Attach started a frame clock")
	}
	poster, ran := postFromGoroutine(t, r)
	if main := mainGID(a); ran != main || ran == poster {
		t.Fatalf("fn ran on goroutine %d (poster %d), main is %d", ran, poster, main)
	}
}

func TestFrameRunsOnUIThread(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var mu sync.Mutex
	var pageGID uint64
	r, _, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) {
		mu.Lock()
		pageGID = uithread.GoroutineID()
		mu.Unlock()
		ui.Text(c, "threaded")
	})
	if !tc.flush(t) {
		t.Fatal("Open did not request a first frame")
	}
	main := mainGID(a)
	mu.Lock()
	g := pageGID
	mu.Unlock()
	if g != tc.gid || g == main {
		t.Fatalf("page ran on goroutine %d, UI thread %d, main %d", g, tc.gid, main)
	}
	tc.mu.Lock()
	presents, presentGID := tc.presents, tc.presentGID
	tc.mu.Unlock()
	if presents < 1 || presentGID != tc.gid {
		t.Fatalf("presents %d, last on goroutine %d, UI thread %d", presents, presentGID, tc.gid)
	}
	if n := cv.Invalidates(); n != 0 {
		t.Fatalf("canvas Invalidate called %d times; threaded windows ask the frame clock", n)
	}
	if _, ran := postFromGoroutine(t, r); ran != tc.gid {
		t.Fatalf("Runner.Post ran on goroutine %d, UI thread %d", ran, tc.gid)
	}
}

func TestThreadedClockErrorKeepsMainThreadModel(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	canvases := useThreadedCanvases(t, false, clockCfg{err: errors.New("no display link")})
	var mu sync.Mutex
	var pageGID uint64
	o := tango.Await(a, func() opened {
		r, s, err := uihost.Open(a, uihost.Options{
			Surface: tango.SurfaceOptions{Title: "UI", Width: 100, Height: 100},
		}, func(c *ui.Context) {
			mu.Lock()
			pageGID = uithread.GoroutineID()
			mu.Unlock()
		}, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	fb.Do(func() {})
	if canvases(o.s.ID()) == nil {
		t.Fatal("canvas was not wrapped")
	}
	fb.Canvas(o.s.ID()).FlushFrame()
	main := mainGID(a)
	mu.Lock()
	defer mu.Unlock()
	if pageGID != main {
		t.Fatalf("page ran on goroutine %d, main is %d", pageGID, main)
	}
}

func TestThreadedCloseStopsClockAndThread(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	_, s, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) {})
	tc.flush(t)
	if !fb.RequestClose(s.ID()) {
		t.Fatal("surface did not close")
	}
	select {
	case <-tc.th.Done():
	case <-time.After(faketest.Timeout):
		t.Fatal("UI thread did not exit after close")
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if !tc.stopped {
		t.Fatal("frame clock not stopped")
	}
}

func TestThreadedGPUSetupOnMainThread(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "gpu")
	g := &fakeGPU{}
	var createdOn *uithread.Thread
	t.Cleanup(uihost.SetGPUHooks(func(layer uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		createdOn = uithread.Current()
		return g.newPresenter(layer)
	}, nil))
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, _, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) { ui.Text(c, "gpu") })
	if createdOn != tc.th {
		t.Fatal("GPU presenter not created on its frame thread")
	}
	main := mainGID(a)
	tc.mu.Lock()
	gids := slices.Clone(tc.metalGIDs)
	tc.mu.Unlock()
	if len(gids) != 1 || gids[0] != main {
		t.Fatalf("UseMetalLayer on goroutines %v, main is %d", gids, main)
	}
	if !tc.flush(t) {
		t.Fatal("no first frame")
	}
	if layers, renders, presents := g.stats(); layers != 1 || renders < 1 || len(presents) != renders {
		t.Fatalf("presenter calls %d, renders %d, presents %v", layers, renders, presents)
	}
	if got := rendererOf(a, r); got != "gpu" {
		t.Fatalf("Renderer = %q", got)
	}
}

func TestThreadedDropRunsAfterDragOver(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	dir := t.TempDir()
	p := filepath.Join(dir, "drop.png")
	if err := os.WriteFile(p, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	var files []ui.UploadFile
	r, s, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) {
		ui.Upload(c, &files).Dragger()
	})
	tango.Await(a, func() struct{} { uihost.AcceptDrops(r, s); return struct{}{} })
	for tc.flush(t) {
	}
	// The drop reports no position: it lands where the queued drag-over
	// left the drag, so it must run on the UI thread after it.
	tc.dragOver(fb, 100, 30)
	if !fb.DropFiles(s.ID(), []string{p}, 0, 0) {
		t.Fatal("drop not delivered")
	}
	for tc.flush(t) {
	}
	var n int
	var name string
	tc.th.Do(func() {
		n = len(files)
		if n > 0 {
			name = files[0].Name
		}
	})
	if n != 1 || name != "drop.png" {
		t.Fatalf("files after drop: %d %q", n, name)
	}
}

// A clock that ticks as soon as Open's hand-over asks for the first frame
// finds the host complete (run with -race).
func TestThreadedEagerClockFirstFrame(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	canvases := useThreadedCanvases(t, false, clockCfg{eager: true})
	ran := make(chan struct{}, 1)
	o := tango.Await(a, func() opened {
		r, s, err := uihost.Open(a, uihost.Options{
			Surface: tango.SurfaceOptions{Title: "UI", Width: 100, Height: 100},
		}, func(c *ui.Context) {
			ui.Text(c, "eager")
			select {
			case ran <- struct{}{}:
			default:
			}
		}, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	select {
	case <-ran:
	case <-time.After(faketest.Timeout):
		t.Fatal("first frame did not run the page")
	}
	tc := canvases(o.s.ID())
	closeAtEnd(t, fb, o.s, tc)
	tc.sync(t)
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.presents < 1 {
		t.Fatal("first frame not presented")
	}
}
