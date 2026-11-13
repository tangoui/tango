package uihost_test

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// A platform touch (Windows WM_POINTER, GTK touch-event) reaches the
// runner through CanvasInput: a tap clicks a button through the runner's
// pointer emulation; a cancelled touch does not.
func TestTouchReachesRunner(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	clicks := 0
	_, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) {
		if ui.Button(c, "OK").Block().Clicked() {
			clicks++
		}
	})
	for cv.FlushFrame() {
	}
	at := geom.Point{X: 150, Y: 10}
	cv.Input(input.TouchEvent{ID: 7, Phase: input.TouchBegan, Pos: at, Pressure: 0.5})
	if !cv.Pending() {
		t.Fatal("touch did not request a frame")
	}
	cv.FlushFrame()
	cv.Input(input.TouchEvent{ID: 7, Phase: input.TouchEnded, Pos: at})
	for cv.FlushFrame() {
	}
	var n int
	fb.Do(func() { n = clicks })
	if n != 1 {
		t.Fatalf("clicks after tap = %d", n)
	}
	// A cancelled touch does not click.
	cv.Input(input.TouchEvent{ID: 8, Phase: input.TouchBegan, Pos: at})
	cv.FlushFrame()
	cv.Input(input.TouchEvent{ID: 8, Phase: input.TouchCancelled, Pos: at})
	for cv.FlushFrame() {
	}
	fb.Do(func() { n = clicks })
	if n != 1 {
		t.Fatalf("clicks after cancelled touch = %d", n)
	}
}
