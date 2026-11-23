package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestMiddlePressedUsesVisibleHit(t *testing.T) {
	var middle, cover bool
	d := NewDriver(func(c *Context) {
		b := Div(c, layout.Size(60, 60))
		b.Interact(input.CursorDefault)
		middle = c.MiddlePressed(b.ID())
		if cover {
			Overlay(c, OverlayOptions{Place: ScreenFill}, func() {})
		}
	}, DriverOptions{Width: 120, Height: 120})
	d.Frame()
	for _, over := range []bool{false, true} {
		cover = over
		d.Frame()
		d.Inject(input.Pointer{Kind: input.Press, Button: input.ButtonMiddle, Pos: geom.Pt(30, 30)})
		d.Frame()
		if middle == over {
			t.Fatalf("covered=%v, middle press registered=%v", over, middle)
		}
	}
}

func TestInputNumberSubmittedWithoutChange(t *testing.T) {
	n := float64(4)
	var submitted, changed bool
	d := NewDriver(func(c *Context) {
		w := InputNumber(c, &n).Controls(false)
		submitted, changed = w.Submitted(), w.Changed()
	}, DriverOptions{Width: 240, Height: 90})
	d.Frame()
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	if !submitted || changed || n != 4 {
		t.Fatalf("submit=%v changed=%v value=%v", submitted, changed, n)
	}
}
