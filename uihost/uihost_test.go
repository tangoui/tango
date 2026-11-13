package uihost_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/uihost"
)

type opened struct {
	r   *ui.Runner
	s   *tango.Surface
	err error
}

// open calls uihost.Open on the main thread and lets the first frame run.
func open(t *testing.T, a *tango.App, fb *fake.Backend, w, h int, page func(*ui.Context)) (*ui.Runner, *tango.Surface, *fake.Canvas) {
	t.Helper()
	o := tango.Await(a, func() opened {
		r, s, err := uihost.Open(a, uihost.Options{
			Surface: tango.SurfaceOptions{Title: "UI", Width: w, Height: h},
		}, page, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	fb.Do(func() {}) // the queued first frame
	cv := fb.Canvas(o.s.ID())
	if cv == nil {
		t.Fatal("no fake canvas")
	}
	return o.r, o.s, cv
}

func TestFirstFramePresented(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	var size geom.Size
	_, s, cv := open(t, a, fb, 300, 200, func(c *ui.Context) {
		size = c.Size()
		ui.Text(c, "hello")
	})
	if cv.Presents() < 1 {
		t.Fatalf("presents = %d", cv.Presents())
	}
	if w, h := cv.LastFrameSize(); w != 600 || h != 400 {
		t.Fatalf("frame %dx%d, want 600x400", w, h)
	}
	if size != (geom.Size{W: 300, H: 200}) {
		t.Fatalf("page saw size %v", size)
	}
	if fs := cv.Surface(); fs.Title() != "UI" || len(fs.Navigations()) != 0 {
		t.Fatalf("surface title %q navs %v", fs.Title(), fs.Navigations())
	}
	if w, h := s.Size(); w != 300 || h != 200 {
		t.Fatalf("surface size %dx%d", w, h)
	}
}

func TestClick(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	clicks := 0
	var ptr geom.Point
	_, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) {
		if ui.Button(c, "OK").Block().Clicked() {
			clicks++
		}
		ptr, _ = c.Pointer()
	})
	for cv.FlushFrame() { // settle
	}
	at := geom.Point{X: 150, Y: 10}
	before := cv.Invalidates()
	cv.Input(input.Pointer{Kind: input.Move, Pos: at})
	if cv.Invalidates() <= before || !cv.Pending() {
		t.Fatal("input did not request a frame")
	}
	cv.FlushFrame()
	if ptr != at {
		t.Fatalf("page saw pointer %v, want %v", ptr, at)
	}
	if cv.Cursor() != input.CursorPointer {
		t.Fatalf("cursor = %v", cv.Cursor())
	}
	cv.Input(input.Pointer{Kind: input.Press, Pos: at, Button: input.ButtonLeft})
	cv.Input(input.Pointer{Kind: input.Release, Pos: at, Button: input.ButtonLeft})
	if !cv.FlushFrame() {
		t.Fatal("no frame after click")
	}
	var n int
	fb.Do(func() { n = clicks })
	if n != 1 {
		t.Fatalf("clicks = %d", n)
	}
}

func TestAnimationRequestsFrames(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	_, _, cv := open(t, a, fb, 100, 100, func(c *ui.Context) {
		c.AnimFrom(ui.ID(1), "x", 0, 1, time.Hour)
	})
	for range 3 {
		before, ats := cv.Invalidates(), len(cv.InvalidateAts())
		if !cv.Frame() {
			t.Fatal("frame did not run")
		}
		if cv.Invalidates() <= before || !cv.Pending() {
			t.Fatal("animating frame did not request another")
		}
		if len(cv.InvalidateAts()) <= ats {
			t.Fatal("no InvalidateAt while animating")
		}
	}
}

func TestStaticPageIdles(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	_, _, cv := open(t, a, fb, 100, 100, func(c *ui.Context) { ui.Text(c, "still") })
	for i := 0; cv.FlushFrame(); i++ {
		if i > 5 {
			t.Fatal("static page keeps requesting frames")
		}
	}
}

func TestCloseClosesRunner(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, s, cv := open(t, a, fb, 100, 100, func(c *ui.Context) { ui.Text(c, "bye") })
	closed := make(chan struct{})
	s.OnClosed(func() { close(closed) })
	if !fb.RequestClose(s.ID()) {
		t.Fatal("surface did not close")
	}
	select {
	case <-closed:
	case <-time.After(faketest.Timeout):
		t.Fatal("OnClosed did not run")
	}
	n := cv.Presents()
	// A closed runner neither renders nor presents.
	if err := tango.Await(a, r.Frame); err != nil {
		t.Fatal(err)
	}
	if cv.Presents() != n {
		t.Fatal("closed runner presented a frame")
	}
	if cv.Frame() {
		t.Fatal("frame ran on a closed canvas")
	}
}

func TestFocus(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	_, _, cv := open(t, a, fb, 100, 100, func(c *ui.Context) { ui.Text(c, "f") })
	for cv.FlushFrame() {
	}
	cv.Focus(false)
	if !cv.FlushFrame() {
		t.Fatal("focus change did not request a frame")
	}
}

func TestResize(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var size geom.Size
	_, s, cv := open(t, a, fb, 100, 100, func(c *ui.Context) { size = c.Size() })
	resized := make(chan [2]int, 1)
	s.OnResized(func(w, h int) { resized <- [2]int{w, h} })
	for cv.FlushFrame() {
	}
	cv.Resize(400, 300, 1.5)
	if !cv.FlushFrame() {
		t.Fatal("resize did not request a frame")
	}
	if w, h := cv.LastFrameSize(); w != 600 || h != 450 {
		t.Fatalf("frame %dx%d, want 600x450", w, h)
	}
	var got geom.Size
	fb.Do(func() { got = size })
	if got != (geom.Size{W: 400, H: 300}) {
		t.Fatalf("page saw %v", got)
	}
	if wh := <-resized; wh != [2]int{400, 300} {
		t.Fatalf("OnResized %v", wh)
	}
}

func TestUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	o := tango.Await(a, func() opened {
		r, s, err := uihost.Open(a, uihost.Options{}, func(*ui.Context) {}, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if !errors.Is(o.err, tango.ErrUnsupported) || o.r != nil || o.s != nil {
		t.Fatalf("got %v %v %v", o.r, o.s, o.err)
	}
}

func TestOffMainThread(t *testing.T) {
	a, _ := faketest.Run(t, tango.Options{})
	if _, _, err := uihost.Open(a, uihost.Options{}, func(*ui.Context) {}, ui.RunnerOptions{}); err == nil {
		t.Fatal("Open off the main thread succeeded")
	}
}

func TestClipboard(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, _, _ := open(t, a, fb, 100, 100, func(c *ui.Context) {})
	got := tango.Await(a, func() string {
		w := r.Context().Window()
		w.WriteClipboard("héllo")
		return w.ReadClipboard()
	})
	if got != "héllo" || fb.ClipboardText() != "héllo" {
		t.Fatalf("read %q, backend %q", got, fb.ClipboardText())
	}
}

func TestOnCloseVeto(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	_, s, cv := open(t, a, fb, 100, 100, func(c *ui.Context) { ui.Text(c, "stay") })
	s.OnClose(func() bool { return false })
	if fb.RequestClose(s.ID()) {
		t.Fatal("closed despite OnClose")
	}
	if !cv.Frame() {
		t.Fatal("canvas stopped after a vetoed close")
	}
}
