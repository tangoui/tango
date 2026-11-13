package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func p2bRect(t *testing.T, d *Driver, n *node) geom.Rect {
	t.Helper()
	r, ok := d.r.c.rects[n.id]
	if !ok {
		t.Fatal("node has no rect")
	}
	return r
}

func TestP2BSliderRangeDragKeys(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		lo, hi := 20.0, 60.0
		var w *SliderW
		d := fillDriver(th, func(c *Context) {
			Column(c, func() {
				w = SliderRange(c, &lo, &hi, 0, 100).A11yLabel("Price").Layout(layout.W(412))
			}, layout.Pad(20))
		})
		d.EnableAccessibility()
		tr := p2bRect(t, d, w.track)
		// Handles sit on their values.
		h0, h1 := p2bRect(t, d, w.thumbs[0]), p2bRect(t, d, w.thumbs[1])
		want0 := tr.X + sliderPad + (tr.W-2*sliderPad)*0.2
		if d0 := h0.Center().X - want0; d0 > 0.5 || d0 < -0.5 {
			t.Fatalf("handle 0 at %v, want x %v", h0, want0)
		}
		// Dragging the end handle.
		x80 := tr.X + sliderPad + (tr.W-2*sliderPad)*0.8
		d.Drag(h1.Center(), geom.Pt(x80, h1.Center().Y))
		if hi != 80 || lo != 20 {
			t.Fatalf("drag: lo %v hi %v", lo, hi)
		}
		// A rail press moves the nearest handle.
		d.Click(geom.Pt(tr.X+sliderPad+(tr.W-2*sliderPad)*0.1, tr.Y+11))
		if lo != 10 || hi != 80 {
			t.Fatalf("rail press: lo %v hi %v", lo, hi)
		}
		// Dragging the start handle past the end swaps them (Ant): the
		// dragged handle continues as the end and takes the focus.
		h0 = p2bRect(t, d, w.thumbs[0])
		d.Drag(h0.Center(), geom.Pt(tr.Right(), h0.Center().Y))
		if lo != 80 || hi != 100 {
			t.Fatalf("crossing: lo %v hi %v", lo, hi)
		}
		if d.r.c.focus != w.thumbs[1].id {
			t.Fatal("focus did not follow the crossing handle")
		}
		// Keyboard on the focused end handle (limited by the start).
		d.Key(input.KeyHome, 0)
		d.Key(input.KeyPageUp, 0)
		d.Key(input.KeyRight, 0)
		if hi != 91 {
			t.Fatalf("keys: hi %v", hi)
		}
		d.Key(input.KeyHome, 0)
		if hi != 80 {
			t.Fatalf("Home: hi %v", hi)
		}
		// Semantics: a named group of two sliders with linked limits.
		if _, ok := d.A11yFind(a11y.RoleGroup, "Price"); !ok {
			t.Fatal("range group missing")
		}
		mn, ok := d.A11yFind(a11y.RoleSlider, ZhCN.Slider.Min)
		mx, ok2 := d.A11yFind(a11y.RoleSlider, ZhCN.Slider.Max)
		if !ok || !ok2 || mn.ValueNow != 80 || mn.ValueMax != 80 || mx.ValueMin != 80 || mx.ValueMax != 100 {
			t.Fatalf("sliders %+v / %+v", mn, mx)
		}
		if err := d.PerformAction(mx.ID, a11y.SetValue, "90"); err != nil || hi != 90 {
			t.Fatalf("set value: %v hi %v", err, hi)
		}
	})
}

func TestP2BSliderMarksDotsTooltip(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		v := 30.0
		var w *SliderW
		d := fillDriver(th, func(c *Context) {
			Column(c, func() {
				w = Slider(c, &v, 0, 100).Step(10).Dots().Marks(map[float64]string{0: "0°C", 100: "100°C"}).
					Tooltip(func(x float64) string { return itoaEnt(int(x)) + "%" }).Layout(layout.W(300))
			}, layout.Pad(40))
		})
		tr := p2bRect(t, d, w.track)
		if tr.H < sliderThick+20 {
			t.Fatalf("marks need room: track %v", tr)
		}
		// Pressing a mark label moves the handle there.
		d.Click(geom.Pt(tr.Right()-sliderPad, tr.Bottom()-6))
		if v != 100 {
			t.Fatalf("mark press: %v", v)
		}
		v = 30
		d.Settle()
		if _, ok := d.FindText("30%"); ok {
			t.Fatal("tooltip shown without hover")
		}
		h := p2bRect(t, d, w.thumbs[0])
		d.Move(h.Center())
		d.Settle()
		if _, ok := d.FindText("30%"); !ok {
			t.Fatal("hover tooltip missing")
		}
		// Snapping to the step while dragging; the tooltip follows.
		d.Drag(h.Center(), geom.Pt(tr.X+sliderPad+(tr.W-2*sliderPad)*0.57, h.Center().Y))
		if v != 60 {
			t.Fatalf("snap: %v", v)
		}
		d.Move(geom.Pt(5, 5))
		d.Settle()
		if _, ok := d.FindText("60%"); ok {
			t.Fatal("tooltip stays after leaving")
		}
	})
}

func TestP2BSliderVerticalAndInput(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		v := 0.0
		var w *SliderW
		d := fillDriver(th, func(c *Context) {
			Column(c, func() { w = Slider(c, &v, 0, 100).Vertical(220) }, layout.Pad(40))
		})
		tr := p2bRect(t, d, w.track)
		if tr.H != 220 || tr.W != sliderThick {
			t.Fatalf("vertical track %v", tr)
		}
		h := p2bRect(t, d, w.thumbs[0])
		if h.Center().Y < tr.Bottom()-sliderPad-1 {
			t.Fatalf("min should be at the bottom: %v", h)
		}
		d.Click(geom.Pt(tr.X+11, tr.Y+sliderPad+(tr.H-2*sliderPad)*0.25))
		if v != 75 {
			t.Fatalf("vertical press: %v", v)
		}
		d.Key(input.KeyUp, 0)
		d.Key(input.KeyPageDown, 0)
		if v != 66 {
			t.Fatalf("vertical keys: %v", v)
		}
		// WithInput: the number box and the slider share the value.
		x := 3.0
		d2 := fillDriver(th, func(c *Context) {
			Column(c, func() { Slider(c, &x, 0, 10).WithInput().A11yLabel("Level").Layout(layout.W(400)) }, layout.Pad(20))
		})
		d2.EnableAccessibility()
		sp, ok := d2.A11yFind(a11y.RoleSpinButton, "Level")
		if !ok || sp.ValueNow != 3 {
			t.Fatalf("input %+v", sp)
		}
		if err := d2.PerformAction(sp.ID, a11y.Increment, ""); err != nil {
			t.Fatal(err)
		}
		d2.Settle()
		if x != 4 {
			t.Fatalf("input increment: %v", x)
		}
		if sl, _ := d2.A11yFind(a11y.RoleSlider, "Level"); sl == nil || sl.ValueNow != 4 {
			t.Fatalf("slider %+v", sl)
		}
	})
}

func TestP2BSliderConfigDefaults(t *testing.T) {
	v := 1.0
	var w *SliderW
	d := fillDriver(func() *theme.Theme { return theme.AntDesign() }, func(c *Context) {
		ConfigProvider(c).Defaults("Slider", Props{"showInput": true, "dots": true, "step": 0.5}).Body(func() {
			w = Slider(c, &v, 0, 5)
		})
	})
	_ = d
	if !w.input || !w.dots || w.step != 0.5 {
		t.Fatalf("defaults: input %v dots %v step %v", w.input, w.dots, w.step)
	}
}
