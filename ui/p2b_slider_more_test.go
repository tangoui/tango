package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// TestSliderRangeNoCrossAndPushable: AllowCross(false) stops a dragged
// handle at the other one; Pushable keeps a gap, pushing the other handle
// with the pointer and the keyboard until it reaches the end.
func TestSliderRangeNoCrossAndPushable(t *testing.T) {
	lo, hi := 20.0, 60.0
	cross, push := false, 0.0
	var w *SliderW
	d := fillDriver(func() *theme.Theme { return theme.AntDesign() }, func(c *Context) {
		Column(c, func() {
			w = SliderRange(c, &lo, &hi, 0, 100).AllowCross(cross).Pushable(push).Layout(layout.W(412))
		}, layout.Pad(20))
	})
	tr := p2bRect(t, d, w.track)
	at := func(v float64) float32 { return tr.X + sliderPad + (tr.W-2*sliderPad)*float32(v/100) }
	// No crossing: the start handle stops at the end handle.
	h0 := p2bRect(t, d, w.thumbs[0])
	d.Drag(h0.Center(), geom.Pt(at(90), h0.Center().Y))
	if lo != 60 || hi != 60 {
		t.Fatalf("no-cross drag: lo %v hi %v", lo, hi)
	}
	// Pushable(10): dragging the start handle to 70 pushes the end to 80.
	lo, hi, push = 20, 60, 10
	d.Settle()
	h0 = p2bRect(t, d, w.thumbs[0])
	d.Drag(h0.Center(), geom.Pt(at(70), h0.Center().Y))
	if lo != 70 || hi != 80 {
		t.Fatalf("push drag: lo %v hi %v", lo, hi)
	}
	// The end of the slider stops the push: the start handle reaches 90.
	h0 = p2bRect(t, d, w.thumbs[0])
	d.Drag(h0.Center(), geom.Pt(tr.Right(), h0.Center().Y))
	if lo != 90 || hi != 100 {
		t.Fatalf("push to the end: lo %v hi %v", lo, hi)
	}
	// Keyboard on the end handle pushes the start one; Home stops at
	// min+gap.
	lo, hi = 20, 30
	d.Settle()
	d.r.c.SetFocus(w.thumbs[1].id)
	d.Settle()
	d.Key(input.KeyLeft, 0)
	if hi != 29 || lo != 19 {
		t.Fatalf("key push: lo %v hi %v", lo, hi)
	}
	d.Key(input.KeyHome, 0)
	if hi != 10 || lo != 0 {
		t.Fatalf("Home push: lo %v hi %v", lo, hi)
	}
}

// TestSliderWithInputRangeAndVertical: WithInput builds two linked boxes
// for a range (start before, end after the track) and boxes for vertical
// sliders (below; a range has the end box above).
func TestSliderWithInputRangeAndVertical(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		lo, hi := 20.0, 30.0
		var w *SliderW
		d := fillDriver(th, func(c *Context) {
			Column(c, func() {
				w = SliderRange(c, &lo, &hi, 0, 100).WithInput().Pushable(5).Layout(layout.W(600))
			}, layout.Pad(20))
		})
		d.EnableAccessibility()
		d.Settle()
		mn, ok := d.A11yFind(a11y.RoleSpinButton, ZhCN.Slider.Min)
		mx, ok2 := d.A11yFind(a11y.RoleSpinButton, ZhCN.Slider.Max)
		if !ok || !ok2 || mn.ValueNow != 20 || mx.ValueNow != 30 {
			t.Fatalf("range inputs %+v / %+v", mn, mx)
		}
		tr := p2bRect(t, d, w.track)
		if mn.Bounds.X >= tr.X || mx.Bounds.X <= tr.Right() {
			t.Fatalf("boxes should flank the track: %v %v %v", mn.Bounds, tr, mx.Bounds)
		}
		// Raising the start box past the gap pushes the end value.
		for k := 0; k < 8; k++ {
			if err := d.PerformAction(mn.ID, a11y.Increment, ""); err != nil {
				t.Fatal(err)
			}
			d.Settle()
		}
		if lo != 28 || hi != 33 {
			t.Fatalf("input push: lo %v hi %v", lo, hi)
		}
		v, a, b := 40.0, 10.0, 70.0
		var sv, sr *SliderW
		d2 := fillDriver(th, func(c *Context) {
			Row(c, func() {
				sv = Slider(c, &v, 0, 100).Vertical(200).WithInput().A11yLabel("Level")
				sr = SliderRange(c, &a, &b, 0, 100).Vertical(200).WithInput()
			}, layout.Pad(20), layout.Gap(40))
		})
		d2.EnableAccessibility()
		d2.Settle()
		sp, ok := d2.A11yFind(a11y.RoleSpinButton, "Level")
		if !ok || sp.ValueNow != 40 || sp.Bounds.Y < p2bRect(t, d2, sv.track).Bottom() {
			t.Fatalf("vertical input %+v", sp)
		}
		rt := p2bRect(t, d2, sr.track)
		mx2, _ := d2.A11yFind(a11y.RoleSpinButton, ZhCN.Slider.Max)
		mn2, _ := d2.A11yFind(a11y.RoleSpinButton, ZhCN.Slider.Min)
		if mx2 == nil || mn2 == nil || mx2.Bounds.Y+mx2.Bounds.H > rt.Y || mn2.Bounds.Y < rt.Bottom() {
			t.Fatalf("vertical range inputs %+v %+v track %v", mx2, mn2, rt)
		}
		if err := d2.PerformAction(sp.ID, a11y.Increment, ""); err != nil {
			t.Fatal(err)
		}
		d2.Settle()
		if v != 41 {
			t.Fatalf("vertical input increment: %v", v)
		}
	})
}
