package uihost_test

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/uihost"
)

// A canvas implementing platform.CanvasInsets: the runner sees its safe
// area and keyboard inset, and a change reaches the page on the frame the
// canvas requests.
func TestInsetsForwarded(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var safe, kb geom.Insets
	_, _, cv := open(t, a, fb, 390, 844, func(c *ui.Context) {
		safe, kb = c.SafeArea(), c.KeyboardInset()
		ui.Text(c, "insets")
	})
	for cv.FlushFrame() {
	}
	seen := func() (s, k geom.Insets) {
		fb.Do(func() { s, k = safe, kb })
		return
	}
	if s, k := seen(); s != (geom.Insets{}) || k != (geom.Insets{}) {
		t.Fatalf("initial insets %v %v", s, k)
	}

	polls := cv.InsetPolls()
	cv.SetSafeAreaInsets(geom.Insets{Top: 47, Bottom: 34})
	if !cv.FlushFrame() {
		t.Fatal("safe-area change requested no frame")
	}
	if s, _ := seen(); s != (geom.Insets{Top: 47, Bottom: 34}) {
		t.Fatalf("safe area %v", s)
	}
	if cv.InsetPolls() <= polls {
		t.Fatal("window did not poll the canvas")
	}

	cv.SetKeyboardInset(geom.Insets{Bottom: 336})
	if !cv.FlushFrame() {
		t.Fatal("keyboard change requested no frame")
	}
	if s, k := seen(); k != (geom.Insets{Bottom: 336}) || s.Top != 47 {
		t.Fatalf("keyboard %v safe %v", k, s)
	}

	cv.SetKeyboardInset(geom.Insets{})
	cv.FlushFrame()
	if _, k := seen(); k != (geom.Insets{}) {
		t.Fatalf("keyboard hidden %v", k)
	}

	// An unchanged value requests nothing.
	for cv.FlushFrame() {
	}
	cv.SetSafeAreaInsets(geom.Insets{Top: 47, Bottom: 34})
	if cv.Pending() {
		t.Fatal("unchanged insets requested a frame")
	}
}

// Attached canvases forward their insets too (the fake AttachedCanvas
// inherits CanvasInsets from its Canvas).
func TestInsetsAttached(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 400, 300)
	var kb geom.Insets
	_, _, cv := attach(t, a, fb, s, uihost.Layer{Above: true}, func(c *ui.Context) {
		kb = c.KeyboardInset()
	})
	for cv.FlushFrame() {
	}
	cv.SetKeyboardInset(geom.Insets{Bottom: 120})
	if !cv.FlushFrame() {
		t.Fatal("keyboard change requested no frame")
	}
	var got geom.Insets
	fb.Do(func() { got = kb })
	if got != (geom.Insets{Bottom: 120}) {
		t.Fatalf("attached keyboard inset %v", got)
	}
}
