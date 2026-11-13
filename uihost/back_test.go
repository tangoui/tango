package uihost_test

import (
	"slices"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/uihost"
)

// The window passes ui.Context.WantBack to a canvas implementing
// platform.CanvasBackHandler; Back delivered while it is on closes the
// overlay, after which handling goes off and Back leaves the page again.
func TestBackHandlingForwarded(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	shown := false
	_, _, cv := open(t, a, fb, 400, 300, func(c *ui.Context) {
		ui.Modal(c, &shown, "dialog").Body(func() { ui.Text(c, "body") })
	})
	for cv.FlushFrame() {
	}
	if on, calls := cv.BackHandling(); on || len(calls) != 0 {
		t.Fatalf("nothing open: on=%v calls=%v", on, calls)
	}
	if cv.Back() || cv.BackPassed() != 1 {
		t.Fatal("Back with nothing open should leave the page")
	}

	fb.Do(func() { shown = true })
	cv.Invalidate()
	for cv.FlushFrame() {
	}
	if on, _ := cv.BackHandling(); !on {
		t.Fatal("open modal: handling off")
	}
	if !cv.Back() {
		t.Fatal("Back not taken while handling is on")
	}
	for cv.FlushFrame() {
	}
	var still bool
	fb.Do(func() { still = shown })
	if still {
		t.Fatal("Back did not close the modal")
	}
	on, calls := cv.BackHandling()
	if on || !slices.Equal(calls, []bool{true, false}) {
		t.Fatalf("after close: on=%v calls=%v", on, calls)
	}
	if cv.Back() || cv.BackPassed() != 2 {
		t.Fatal("Back after closing should leave the page")
	}
}

// Attached canvases forward it too.
func TestBackHandlingAttached(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s := webSurface(t, a, 400, 300)
	open := true
	_, _, cv := attach(t, a, fb, s, uihost.Layer{Above: true}, func(c *ui.Context) {
		ui.Drawer(c, &open, "drawer").Body(func() { ui.Text(c, "body") })
	})
	for cv.FlushFrame() {
	}
	if on, _ := cv.BackHandling(); !on {
		t.Fatal("open drawer: handling off")
	}
	cv.Back()
	for cv.FlushFrame() {
	}
	if on, _ := cv.BackHandling(); on {
		t.Fatal("handling still on after Back")
	}
}
