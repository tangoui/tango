package uihost_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/uihost"
)

// embedOf returns the fake embedded WebView behind w.
func embedOf(t *testing.T, a *tango.App, fb *fake.Backend, w **uihost.WebViewW) *fake.Embedded {
	t.Helper()
	var s *tango.Surface
	tango.Await(a, func() struct{} { s = (*w).Surface(); return struct{}{} })
	if s == nil {
		t.Fatal("no WebView surface")
	}
	e := fb.Embedded(s.ID())
	if e == nil {
		t.Fatal("no fake embedded view")
	}
	return e
}

func wait(t *testing.T, cv *fake.Canvas, what string, cond func() bool) {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) {
		settle(cv)
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting: " + what)
}

func TestWebViewFollowsBox(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var w *uihost.WebViewW
	var created *tango.Surface
	_, win, cv := open(t, a, fb, 400, 300, func(c *ui.Context) {
		ui.Column(c, func() {
			ui.Div(c, layout.H(50))
			w = uihost.WebView(c, "web", uihost.WebViewOptions{
				URL:     "/page.html",
				Created: func(s *tango.Surface) { created = s },
			}, layout.W(200), layout.H(100))
		}, layout.Pad(10))
	})
	settle(cv)
	e := embedOf(t, a, fb, &w)
	if created == nil || uihost.EmbeddedIn(created) != win || e.Parent() != win.ID() {
		t.Fatalf("created %v, parent %v", created, e.Parent())
	}
	if !strings.HasSuffix(e.Options().URL, "/page.html") || !strings.Contains(e.Options().BridgeScript, "__tangoEmbedFocus") {
		t.Fatalf("options %+v", e.Options())
	}
	want := platform.RectF{X: 10, Y: 60, W: 200, H: 100}
	if p := e.Placement(); p.Frame != want || p.Clip != want {
		t.Fatalf("placement %+v, want %v", p, want)
	}
	if w, h := created.Size(); w != 200 || h != 100 {
		t.Fatalf("size %dx%d", w, h)
	}
}

func TestWebViewClippedAndRemoved(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var w *uihost.WebViewW
	show, scroll := true, -1.0
	_, _, cv := open(t, a, fb, 300, 300, func(c *ui.Context) {
		sb := ui.Scroll(c, func() {
			ui.Div(c, layout.H(100))
			if show {
				w = uihost.WebView(c, "web", uihost.WebViewOptions{}, layout.H(150))
			}
			ui.Div(c, layout.H(500))
		}, layout.H(200))
		if scroll >= 0 {
			sb.ScrollTo(0, scroll)
			scroll = -1
		}
	})
	settle(cv)
	e := embedOf(t, a, fb, &w)
	p := e.Placement()
	if p.Frame.Y != 100 || p.Frame.H != 150 || p.Clip.Y != 100 || p.Clip.H != 100 {
		t.Fatalf("placement %+v: want the bottom half clipped by the scroll box", p)
	}
	tango.Await(a, func() struct{} { scroll = 400; cv.Invalidate(); return struct{}{} })
	settle(cv)
	if p := e.Placement(); !p.Clip.Empty() || p.Frame.Y != -300 {
		t.Fatalf("scrolled out: placement %+v", p)
	}
	tango.Await(a, func() struct{} { show = false; cv.Invalidate(); return struct{}{} })
	settle(cv)
	if !e.Closed() {
		t.Fatal("WebView not closed after its box went away")
	}
}

func TestWebViewKeepAlive(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var w *uihost.WebViewW
	show := true
	_, _, cv := open(t, a, fb, 300, 300, func(c *ui.Context) {
		if show {
			w = uihost.WebView(c, "web", uihost.WebViewOptions{KeepAlive: time.Hour}, layout.H(100))
		}
	})
	settle(cv)
	e := embedOf(t, a, fb, &w)
	tango.Await(a, func() struct{} { show = false; cv.Invalidate(); return struct{}{} })
	settle(cv)
	if e.Closed() || !e.Placement().Clip.Empty() {
		t.Fatalf("kept WebView: closed %v placement %+v", e.Closed(), e.Placement())
	}
	tango.Await(a, func() struct{} { show = true; cv.Invalidate(); return struct{}{} })
	settle(cv)
	if e2 := embedOf(t, a, fb, &w); e2 != e || e.Placement().Clip.Empty() {
		t.Fatal("WebView not reused when its box came back")
	}
}

func TestWebViewFreezesUnderOverlay(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var w *uihost.WebViewW
	open1 := false
	_, _, cv := open(t, a, fb, 400, 300, func(c *ui.Context) {
		ui.Column(c, func() {
			b := ui.Div(c, layout.W(100), layout.H(30))
			w = uihost.WebView(c, "web", uihost.WebViewOptions{}, layout.W(300), layout.H(200))
			if open1 {
				ui.Overlay(c, ui.OverlayOptions{Anchor: b.ID(), Place: ui.BottomStart}, func() {
					ui.Div(c, layout.W(80), layout.H(80))
				})
			}
		})
	})
	settle(cv)
	e := embedOf(t, a, fb, &w)
	tango.Await(a, func() struct{} { open1 = true; cv.Invalidate(); return struct{}{} })
	frozen := func() bool { return tango.Await(a, func() bool { return w.Frozen() }) }
	wait(t, cv, "frozen", frozen)
	if !e.Placement().Clip.Empty() {
		t.Fatalf("frozen but native view visible: %+v", e.Placement())
	}
	tango.Await(a, func() struct{} { open1 = false; cv.Invalidate(); return struct{}{} })
	settle(cv)
	if frozen() || e.Placement().Clip.Empty() {
		t.Fatal("native view not back after the overlay closed")
	}
}

func TestWebViewCoverHideAndIgnore(t *testing.T) {
	for _, mode := range []uihost.CoverMode{uihost.CoverHide, uihost.CoverIgnore} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			a, fb := faketest.Run(t, tango.Options{})
			var w *uihost.WebViewW
			_, _, cv := open(t, a, fb, 400, 300, func(c *ui.Context) {
				w = uihost.WebView(c, "web", uihost.WebViewOptions{Covered: mode}, layout.W(300), layout.H(200))
				w.Freeze(true)
			})
			settle(cv)
			e := embedOf(t, a, fb, &w)
			hidden := e.Placement().Clip.Empty()
			if hidden != (mode == uihost.CoverHide) {
				t.Fatalf("mode %d: hidden = %v", mode, hidden)
			}
		})
	}
}

func TestWebViewFocus(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var w *uihost.WebViewW
	var before, after ui.ID
	var focused ui.ID
	_, win, cv := open(t, a, fb, 400, 400, func(c *ui.Context) {
		ui.Column(c, func() {
			before = ui.Button(c, "before").ID()
			w = uihost.WebView(c, "web", uihost.WebViewOptions{}, layout.W(200), layout.H(100))
			after = ui.Button(c, "after").ID()
		})
		focused = c.Focused()
	})
	fb.EmitFocus(win.ID(), true) // the window is the key window
	settle(cv)
	e := embedOf(t, a, fb, &w)
	cv.Input(input.KeyEvent{Key: input.KeyTab, Down: true})
	cv.Input(input.KeyEvent{Key: input.KeyTab})
	settle(cv)
	if focused != before {
		t.Fatalf("first Tab: focus %v, want the button before", focused)
	}
	cv.Input(input.KeyEvent{Key: input.KeyTab, Down: true})
	cv.Input(input.KeyEvent{Key: input.KeyTab})
	settle(cv)
	if f, _ := e.FocusCalls(); f != 1 {
		t.Fatalf("FocusWeb calls = %d, want 1", f)
	}
	if ev := e.Evals(); len(ev) == 0 || !strings.Contains(ev[len(ev)-1], "__tangoEmbedFocus(false)") {
		t.Fatalf("evals %q: want the first element focused", ev)
	}
	// The page reports focus, then asks to leave forwards.
	fb.EmitFocus(e.ID(), true)
	settle(cv)
	fb.EmitMessage(e.ID(), "\x00tango-int:webembed:next")
	fb.EmitFocus(e.ID(), false)
	settle(cv)
	if _, b := e.FocusCalls(); b != 1 {
		t.Fatalf("BlurWeb calls = %d", b)
	}
	if focused != after {
		t.Fatalf("after leaving the page: focus %v, want the button after (%v)", focused, after)
	}
	// A click in the page moves the UI's focus to the box.
	fb.EmitFocus(e.ID(), true)
	settle(cv)
	if focused != w.ID() {
		t.Fatalf("page focused: UI focus %v, want the box", focused)
	}
	if f, _ := e.FocusCalls(); f != 1 {
		t.Fatalf("page focus echoed back: FocusWeb calls = %d", f)
	}
}

func TestWebViewUnsupportedAndParentClose(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetEmbedErr(fmt.Errorf("no: %w", platform.ErrUnsupported))
	var w *uihost.WebViewW
	_, win, cv := open(t, a, fb, 300, 300, func(c *ui.Context) {
		w = uihost.WebView(c, "web", uihost.WebViewOptions{}, layout.H(100))
	})
	settle(cv)
	err := tango.Await(a, func() error { return w.Err() })
	if !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("Err = %v", err)
	}

	fb.SetEmbedErr(nil)
	a2, fb2 := faketest.Run(t, tango.Options{})
	_, win2, cv2 := open(t, a2, fb2, 300, 300, func(c *ui.Context) {
		w = uihost.WebView(c, "web", uihost.WebViewOptions{}, layout.H(100))
	})
	settle(cv2)
	e := embedOf(t, a2, fb2, &w)
	tango.Await(a2, func() struct{} { win2.Close(); return struct{}{} })
	if !e.Closed() {
		t.Fatal("embedded WebView survived its window")
	}
	_ = win
}
