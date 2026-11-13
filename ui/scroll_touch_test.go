package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func TestScrollFollowsFingerDrag(t *testing.T) {
	var sb *ScrollBox
	val := ""
	d := NewDriver(func(c *Context) {
		sb = Scroll(c, func() {
			Input(c, &val).Placeholder("x")
			for i := 0; i < 60; i++ {
				Text(c, "row")
			}
		}, layout.H(200), layout.W(300))
	}, DriverOptions{Width: 300, Height: 200})
	d.Frame()
	// Start the drag on the Input: the scroll area takes it over.
	d.TouchDrag(geom.Point{X: 100, Y: 150}, geom.Point{X: 100, Y: 30})
	d.Frame()
	if _, y := sb.Offset(); y < 80 {
		t.Fatalf("finger drag scrolled to y=%.1f, want about 120", y)
	}
}

func TestScrollIgnoresMouseDrag(t *testing.T) {
	var sb *ScrollBox
	d := NewDriver(func(c *Context) {
		sb = Scroll(c, func() {
			for i := 0; i < 60; i++ {
				Text(c, "row")
			}
		}, layout.H(200), layout.W(300))
	}, DriverOptions{Width: 300, Height: 200})
	d.Frame()
	d.Drag(geom.Point{X: 100, Y: 150}, geom.Point{X: 100, Y: 30})
	d.Frame()
	if _, y := sb.Offset(); y != 0 {
		t.Fatalf("mouse drag scrolled to y=%.1f, want 0 (mouse drags select / drag widgets)", y)
	}
}
