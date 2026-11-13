package uihost_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/uihost"
)

type attached struct {
	r   *ui.Runner
	at  *uihost.Attached
	err error
}

// webSurface opens a WebView surface of w×h.
func webSurface(t *testing.T, a *tango.App, w, h int) *tango.Surface {
	t.Helper()
	s, err := a.NewSurface(tango.SurfaceOptions{Title: "Web", Width: w, Height: h})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// attach calls uihost.Attach on the main thread and lets the first frame
// run.
func attach(t *testing.T, a *tango.App, fb *fake.Backend, s *tango.Surface, l uihost.Layer, page func(*ui.Context)) (*ui.Runner, *uihost.Attached, *fake.AttachedCanvas) {
	t.Helper()
	o := tango.Await(a, func() attached {
		r, at, err := uihost.Attach(s, l, page, ui.RunnerOptions{})
		return attached{r, at, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	fb.Do(func() {}) // the queued first frame
	list := fb.Attached(s.ID())
	if len(list) == 0 {
		t.Fatal("no fake attached canvas")
	}
	return o.r, o.at, list[len(list)-1]
}

func TestAttachFirstFrame(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	s := webSurface(t, a, 800, 600)
	var size geom.Size
	l := uihost.Layer{Bounds: tango.Rect{X: 10, Y: 20, Width: 200, Height: 100}, Above: true, PassThrough: true}
	_, at, cv := attach(t, a, fb, s, l, func(c *ui.Context) {
		size = c.Size()
		ui.Text(c, "overlay")
	})
	if cv.Presents() < 1 {
		t.Fatalf("presents = %d", cv.Presents())
	}
	if w, h := cv.LastFrameSize(); w != 400 || h != 200 {
		t.Fatalf("frame %dx%d, want 400x200", w, h)
	}
	var got geom.Size
	fb.Do(func() { got = size })
	if got != (geom.Size{W: 200, H: 100}) {
		t.Fatalf("page saw %v", got)
	}
	want := platform.CanvasLayer{Bounds: l.Bounds, Above: true, PassThrough: true}
	if cv.Layer() != want {
		t.Fatalf("backend layer %+v, want %+v", cv.Layer(), want)
	}
	if at.Surface() != s || at.Layer() != l {
		t.Fatal("Attached accessors")
	}
	if sz := tango.Await(a, func() [3]float32 { w, h, sc := at.Size(); return [3]float32{w, h, sc} }); sz != [3]float32{200, 100, 2} {
		t.Fatalf("Size = %v", sz)
	}
}

func TestAttachFillFollowsSurface(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 300, 200)
	var size geom.Size
	_, _, cv := attach(t, a, fb, s, uihost.Layer{}, func(c *ui.Context) { size = c.Size() })
	for cv.FlushFrame() {
	}
	cv.Resize(500, 400, 1)
	if !cv.FlushFrame() {
		t.Fatal("resize did not request a frame")
	}
	var got geom.Size
	fb.Do(func() { got = size })
	if got != (geom.Size{W: 500, H: 400}) {
		t.Fatalf("page saw %v", got)
	}
}

func TestAttachInput(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 400, 300)
	clicks := 0
	_, _, cv := attach(t, a, fb, s, uihost.Layer{Above: true}, func(c *ui.Context) {
		if ui.Button(c, "OK").Block().Clicked() {
			clicks++
		}
	})
	for cv.FlushFrame() {
	}
	at := geom.Point{X: 150, Y: 10}
	cv.Input(input.Pointer{Kind: input.Move, Pos: at})
	cv.FlushFrame()
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
	cv.Focus(true)
	if !cv.FlushFrame() {
		t.Fatal("focus did not request a frame")
	}
}

func TestAttachSurfaceCloseClosesRunner(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 200, 100)
	r, at, cv := attach(t, a, fb, s, uihost.Layer{Above: true}, func(c *ui.Context) { ui.Text(c, "x") })
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
	if err := tango.Await(a, r.Frame); err != nil {
		t.Fatal(err)
	}
	if cv.Presents() != n {
		t.Fatal("closed runner presented a frame")
	}
	if !tango.Await(a, at.Detached) {
		t.Fatal("Detached false after the surface closed")
	}
	if err := tango.Await(a, func() error { return at.SetLayer(uihost.Layer{}) }); !errors.Is(err, uihost.ErrDetached) {
		t.Fatalf("SetLayer after close: %v", err)
	}
	tango.Await(a, func() bool { at.Detach(); return true }) // harmless
}

func TestAttachSetLayer(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 400, 300)
	var size geom.Size
	_, at, cv := attach(t, a, fb, s, uihost.Layer{Bounds: tango.Rect{Width: 100, Height: 50}, Above: true},
		func(c *ui.Context) { size = c.Size() })
	for cv.FlushFrame() {
	}
	nl := uihost.Layer{Bounds: tango.Rect{X: 5, Y: 5, Width: 120, Height: 60}}
	if err := tango.Await(a, func() error { return at.SetLayer(nl) }); err != nil {
		t.Fatal(err)
	}
	if cv.SetLayers() != 1 || cv.Layer().Bounds != nl.Bounds || cv.Layer().Above {
		t.Fatalf("backend layer %+v (%d calls)", cv.Layer(), cv.SetLayers())
	}
	if !cv.FlushFrame() {
		t.Fatal("SetLayer did not request a frame")
	}
	var got geom.Size
	fb.Do(func() { got = size })
	if got != (geom.Size{W: 120, H: 60}) {
		t.Fatalf("page saw %v", got)
	}
	if tango.Await(a, at.Layer) != nl {
		t.Fatal("Layer not updated")
	}
}

func TestAttachDetach(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 200, 100)
	r, at, cv := attach(t, a, fb, s, uihost.Layer{Above: true}, func(c *ui.Context) { ui.Text(c, "x") })
	for cv.FlushFrame() {
	}
	tango.Await(a, func() bool { at.Detach(); at.Detach(); return true })
	if cv.Detaches() != 1 || !cv.Detached() {
		t.Fatalf("detaches = %d", cv.Detaches())
	}
	// The fake still runs Frame (it only checks the surface), but the
	// detached canvas's sink must not render.
	n := cv.Presents()
	cv.Frame()
	cv.Input(input.Pointer{Kind: input.Move, Pos: geom.Pt(1, 1)})
	if err := tango.Await(a, r.Frame); err != nil {
		t.Fatal(err)
	}
	if cv.Presents() != n {
		t.Fatal("detached canvas presented a frame")
	}
	if fs := fb.Surface(s.ID()); fs == nil || fs.Closed() {
		t.Fatal("Detach closed the surface")
	}
	if err := tango.Await(a, func() error { return cv.Present(nil) }); !errors.Is(err, fake.ErrDetached) {
		t.Fatalf("Present after Detach: %v", err)
	}
}

func TestAttachUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	s := webSurface(t, a, 200, 100)
	o := tango.Await(a, func() attached {
		r, at, err := uihost.Attach(s, uihost.Layer{}, func(*ui.Context) {}, ui.RunnerOptions{})
		return attached{r, at, err}
	})
	if !errors.Is(o.err, tango.ErrUnsupported) || o.r != nil || o.at != nil {
		t.Fatalf("got %v %v %v", o.r, o.at, o.err)
	}
}

func TestAttachOffMainThread(t *testing.T) {
	a, _ := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 200, 100)
	if _, _, err := uihost.Attach(s, uihost.Layer{}, func(*ui.Context) {}, ui.RunnerOptions{}); err == nil {
		t.Fatal("Attach off the main thread succeeded")
	}
}

func TestAttachClosedSurface(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 200, 100)
	fb.RequestClose(s.ID())
	o := tango.Await(a, func() attached {
		r, at, err := uihost.Attach(s, uihost.Layer{}, func(*ui.Context) {}, ui.RunnerOptions{})
		return attached{r, at, err}
	})
	if o.err == nil {
		t.Fatal("Attach to a closed surface succeeded")
	}
}

func TestAttachToCanvasSurfaceFails(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	_, s, _ := open(t, a, fb, 100, 100, func(*ui.Context) {})
	o := tango.Await(a, func() attached {
		r, at, err := uihost.Attach(s, uihost.Layer{}, func(*ui.Context) {}, ui.RunnerOptions{})
		return attached{r, at, err}
	})
	if o.err == nil {
		t.Fatal("Attach to a canvas surface succeeded")
	}
}

func TestAttachLayerHitAlphaAnchor(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 400, 300)
	l := uihost.Layer{Bounds: tango.Rect{X: 300, Y: 200, Width: 80, Height: 60}, Above: true, PassThrough: true,
		HitAlpha: 32, Anchor: uihost.AnchorBottomRight}
	_, at, cv := attach(t, a, fb, s, l, func(c *ui.Context) {})
	want := platform.CanvasLayer{Bounds: l.Bounds, Above: true, PassThrough: true, HitAlpha: 32, Anchor: platform.AnchorBottomRight}
	if cv.Layer() != want {
		t.Fatalf("backend layer %+v, want %+v", cv.Layer(), want)
	}
	l.Anchor, l.HitAlpha = uihost.AnchorCenter, 0
	if err := tango.Await(a, func() error { return at.SetLayer(l) }); err != nil {
		t.Fatal(err)
	}
	if got := cv.Layer(); got.Anchor != platform.AnchorCenter || got.HitAlpha != 0 {
		t.Fatalf("after SetLayer %+v", got)
	}
}
