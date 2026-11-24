package uihost_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/uihost"
)

// nativeApp records the Create and Release calls of a NativeHost.
type nativeApp struct {
	mu       sync.Mutex
	next     uintptr
	created  []uintptr
	released []uintptr
	err      error // Create fails with err when set
}

func (n *nativeApp) opts() uihost.NativeHostOptions {
	return uihost.NativeHostOptions{
		Create: func() (uintptr, error) {
			n.mu.Lock()
			defer n.mu.Unlock()
			if n.err != nil {
				return 0, n.err
			}
			n.next += 0x1000
			n.created = append(n.created, n.next)
			return n.next, nil
		},
		Release: func(v uintptr) {
			n.mu.Lock()
			n.released = append(n.released, v)
			n.mu.Unlock()
		},
	}
}

func (n *nativeApp) counts() (created, released int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.created), len(n.released)
}

// onlyNative returns the single native view embedded in win.
func onlyNative(t *testing.T, fb *fake.Backend, win *tango.Surface) *fake.NativeView {
	t.Helper()
	list := fb.NativeViews(win.ID())
	if len(list) != 1 {
		t.Fatalf("%d native views embedded, want 1", len(list))
	}
	return list[0]
}

func TestNativeHostFollowsBox(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var app nativeApp
	var w *uihost.NativeHostW
	top := float32(50)
	opts := app.opts()
	opts.PassInput = true
	_, win, cv := open(t, a, fb, 400, 300, func(c *ui.Context) {
		ui.Column(c, func() {
			ui.Div(c, layout.H(top))
			w = uihost.NativeHost(c, "video", opts, layout.W(200), layout.H(100))
		}, layout.Pad(10))
	})
	settle(cv)
	n := onlyNative(t, fb, win)
	if c, _ := app.counts(); c != 1 || n.View() != app.created[0] {
		t.Fatalf("created %v, embedded view %#x", app.created, n.View())
	}
	if !n.Options().PassInput {
		t.Fatal("PassInput not passed to the backend")
	}
	want := platform.RectF{X: 10, Y: 60, W: 200, H: 100}
	if p := n.Placement(); p.Frame != want || p.Clip != want {
		t.Fatalf("placement %+v, want %v", p, want)
	}
	tango.Await(a, func() struct{} { top = 80; cv.Invalidate(); return struct{}{} })
	settle(cv)
	want.Y = 90
	if p := n.Placement(); p.Frame != want || p.Clip != want {
		t.Fatalf("moved: placement %+v, want %v", p, want)
	}
	if err := tango.Await(a, func() error { return w.Err() }); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	if c, r := app.counts(); c != 1 || r != 0 {
		t.Fatalf("created %d, released %d; want 1, 0", c, r)
	}
}

func TestNativeHostClippedAndRemoved(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var app nativeApp
	show, scroll := true, -1.0
	_, win, cv := open(t, a, fb, 300, 300, func(c *ui.Context) {
		sb := ui.Scroll(c, func() {
			ui.Div(c, layout.H(100))
			if show {
				uihost.NativeHost(c, "video", app.opts(), layout.H(150))
			}
			ui.Div(c, layout.H(500))
		}, layout.H(200))
		if scroll >= 0 {
			sb.ScrollTo(0, scroll)
			scroll = -1
		}
	})
	settle(cv)
	n := onlyNative(t, fb, win)
	if p := n.Placement(); p.Frame.H != 150 || p.Clip.H != 100 {
		t.Fatalf("placement %+v: want the bottom half clipped", p)
	}
	tango.Await(a, func() struct{} { scroll = 400; cv.Invalidate(); return struct{}{} })
	settle(cv)
	if p := n.Placement(); !p.Clip.Empty() {
		t.Fatalf("scrolled out: placement %+v", p)
	}
	tango.Await(a, func() struct{} { show = false; cv.Invalidate(); return struct{}{} })
	settle(cv)
	if !n.Removed() {
		t.Fatal("native view not removed after its box went away")
	}
	if c, r := app.counts(); c != 1 || r != 1 || app.released[0] != app.created[0] {
		t.Fatalf("created %v, released %v", app.created, app.released)
	}
}

func TestNativeHostKeepAlive(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var app nativeApp
	show := true
	opts := app.opts()
	opts.KeepAlive = time.Hour
	_, win, cv := open(t, a, fb, 300, 300, func(c *ui.Context) {
		if show {
			uihost.NativeHost(c, "video", opts, layout.W(100), layout.H(100))
		}
	})
	settle(cv)
	n := onlyNative(t, fb, win)
	tango.Await(a, func() struct{} { show = false; cv.Invalidate(); return struct{}{} })
	settle(cv)
	if n.Removed() || !n.Placement().Clip.Empty() {
		t.Fatalf("kept alive: removed %v, placement %+v; want hidden, not removed", n.Removed(), n.Placement())
	}
	tango.Await(a, func() struct{} { show = true; cv.Invalidate(); return struct{}{} })
	settle(cv)
	if n.Placement().Clip.Empty() || len(fb.NativeViews(win.ID())) != 1 {
		t.Fatal("the kept view did not come back")
	}
	if c, r := app.counts(); c != 1 || r != 0 {
		t.Fatalf("created %d, released %d; want 1, 0", c, r)
	}
}

func TestNativeHostHiddenUnderOverlay(t *testing.T) {
	for _, mode := range []uihost.CoverMode{uihost.CoverSnapshot, uihost.CoverHide, uihost.CoverIgnore} {
		a, fb := faketest.Run(t, tango.Options{})
		var app nativeApp
		opts := app.opts()
		opts.Covered = mode
		overlay := false
		_, win, cv := open(t, a, fb, 400, 300, func(c *ui.Context) {
			ui.Column(c, func() {
				b := ui.Div(c, layout.W(100), layout.H(30))
				uihost.NativeHost(c, "video", opts, layout.W(300), layout.H(200))
				if overlay {
					ui.Overlay(c, ui.OverlayOptions{Anchor: b.ID(), Place: ui.BottomStart}, func() {
						ui.Div(c, layout.W(80), layout.H(80))
					})
				}
			})
		})
		settle(cv)
		n := onlyNative(t, fb, win)
		tango.Await(a, func() struct{} { overlay = true; cv.Invalidate(); return struct{}{} })
		settle(cv)
		settle(cv)
		hidden := n.Placement().Clip.Empty()
		if want := mode != uihost.CoverIgnore; hidden != want {
			t.Fatalf("mode %d: hidden = %v, want %v", mode, hidden, want)
		}
		tango.Await(a, func() struct{} { overlay = false; cv.Invalidate(); return struct{}{} })
		settle(cv)
		if n.Placement().Clip.Empty() {
			t.Fatalf("mode %d: not shown again after the overlay closed", mode)
		}
	}
}

func TestNativeHostWindowCloseReleasesOnce(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var app nativeApp
	_, win, cv := open(t, a, fb, 300, 300, func(c *ui.Context) {
		uihost.NativeHost(c, "video", app.opts(), layout.W(100), layout.H(100))
	})
	settle(cv)
	n := onlyNative(t, fb, win)
	tango.Await(a, func() struct{} { win.Close(); return struct{}{} })
	fb.Do(func() {})
	if !n.Removed() {
		t.Fatal("native view not removed with its window")
	}
	if c, r := app.counts(); c != 1 || r != 1 {
		t.Fatalf("created %d, released %d; want 1, 1", c, r)
	}
}

func TestNativeHostErrors(t *testing.T) {
	t.Run("embed fails", func(t *testing.T) {
		a, fb := faketest.Run(t, tango.Options{})
		embedErr := errors.New("no embedding here")
		fb.SetNativeEmbedErr(embedErr)
		var app nativeApp
		var w *uihost.NativeHostW
		_, _, cv := open(t, a, fb, 300, 300, func(c *ui.Context) {
			w = uihost.NativeHost(c, "video", app.opts(), layout.W(100), layout.H(100))
		})
		settle(cv)
		settle(cv)
		if err := tango.Await(a, func() error { return w.Err() }); !errors.Is(err, embedErr) {
			t.Fatalf("Err() = %v, want %v", err, embedErr)
		}
		tango.Await(a, func() struct{} { cv.Invalidate(); return struct{}{} })
		settle(cv)
		// The app's view was created once, then given back; no retries.
		if c, r := app.counts(); c != 1 || r != 1 {
			t.Fatalf("created %d, released %d; want 1, 1", c, r)
		}
	})
	t.Run("create fails", func(t *testing.T) {
		a, fb := faketest.Run(t, tango.Options{})
		app := nativeApp{err: errors.New("no video")}
		var w *uihost.NativeHostW
		_, win, cv := open(t, a, fb, 300, 300, func(c *ui.Context) {
			w = uihost.NativeHost(c, "video", app.opts(), layout.W(100), layout.H(100))
		})
		settle(cv)
		settle(cv)
		if err := tango.Await(a, func() error { return w.Err() }); !errors.Is(err, app.err) {
			t.Fatalf("Err() = %v, want %v", err, app.err)
		}
		if len(fb.NativeViews(win.ID())) != 0 {
			t.Fatal("embedded a view although Create failed")
		}
	})
}

func TestNativeHostThreaded(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var app nativeApp
	top := float32(20)
	r, win, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) {
		ui.Column(c, func() {
			ui.Div(c, layout.H(top))
			uihost.NativeHost(c, "video", app.opts(), layout.W(100), layout.H(50))
		})
	})
	tc.flush(t)
	syncAppPosts(a) // creates the view and places it on main
	n := onlyNative(t, fb, win)
	if p, want := n.Placement(), frameRegion(t, tc, r); p != want || p.Frame.Y != 20 {
		t.Fatalf("first placement %+v, want %+v", p, want)
	}
	for range 3 { // frames queued while main is busy: the latest wins
		r.Post(func() { top += 10 })
		tc.flush(t)
	}
	syncAppPosts(a)
	if p, want := n.Placement(), frameRegion(t, tc, r); p != want || p.Frame.Y != 50 {
		t.Fatalf("placement %+v after the box moved, want %+v", p, want)
	}
	if c, rel := app.counts(); c != 1 || rel != 0 {
		t.Fatalf("created %d, released %d; want 1, 0", c, rel)
	}
}
