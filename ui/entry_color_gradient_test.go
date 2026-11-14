package ui

// ColorPicker gradient mode (ColorPickerGradient /
// ColorPickerPanelGradient), Trigger(TriggerHover), and the unchanged
// single-color ColorPickerPanel.

import (
	"math"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

func cpTestGradient() theme.Gradient {
	return theme.Gradient{Angle: 90, Stops: []theme.GradientStop{
		{Offset: 0, Color: theme.Hex("#1677ff")}, {Offset: 1, Color: theme.Hex("#722ed1")},
	}}
}

// gradBarRect returns the stop bar of the open panel.
func gradBarRect(t *testing.T, d *Driver) geom.Rect {
	t.Helper()
	r, ok := d.Find(ZhCN.ColorPicker.Gradient)
	if !ok {
		t.Fatal("gradient stop bar missing")
	}
	return r
}

// TestColorPickerGradientStops: pressing the rail adds a stop, the color
// editors change the selected stop, dragging moves it, keys select /
// remove, and the application's slice is never modified in place.
func TestColorPickerGradientStops(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus()} {
		g := cpTestGradient()
		orig := g.Stops
		var w *ColorPickerW
		changed := 0
		d := NewDriver(func(c *Context) {
			Column(c, func() {
				w = ColorPickerGradient(c, &g).ShowText().Presets(theme.Hex("#00ff00")).Open()
				if w.Changed() {
					changed++
				}
			}, layout.Pad(10))
		}, DriverOptions{Width: 700, Height: 800, Theme: th})
		d.Frame()
		d.Frame()
		if w.text() != "linear-gradient(90deg, #1677ff 0%, #722ed1 100%)" {
			t.Fatalf("text %q", w.text())
		}
		bar := gradBarRect(t, d)
		inset := w.tk.HandleSize / 2
		at := func(f float32) geom.Point {
			return geom.Pt(bar.X+inset+f*(bar.W-2*inset), bar.Y+bar.H/2)
		}
		// Add a stop in the middle: it takes the gradient's color there.
		d.Click(at(0.5))
		if len(g.Stops) != 3 || w.st.gsel != 2 {
			t.Fatalf("add: %+v sel=%d", g.Stops, w.st.gsel)
		}
		if mid := g.Stops[2]; mid.Offset < 0.49 || mid.Offset > 0.51 || mid.Color != cpTestGradient().At(mid.Offset) {
			t.Fatalf("added stop %+v", mid)
		}
		if len(orig) != 2 || orig[0].Color != theme.Hex("#1677ff") {
			t.Fatal("the application's slice was modified")
		}
		// The editors change the selected stop (here a preset swatch).
		d.ClickLabel("#00FF00")
		if g.Stops[2].Color != theme.Hex("#00ff00") || g.Stops[0].Color != theme.Hex("#1677ff") {
			t.Fatalf("edit selected stop: %+v", g.Stops)
		}
		// Drag the first stop to 25%.
		d.Drag(at(0), at(0.25))
		if w.st.gsel != 0 || g.Stops[0].Offset < 0.24 || g.Stops[0].Offset > 0.26 {
			t.Fatalf("drag: sel=%d %+v", w.st.gsel, g.Stops)
		}
		// Keyboard: Right moves, Down selects the next stop by offset,
		// Delete removes, never below two stops.
		before := g.Stops[0].Offset
		d.Key(input.KeyRight, 0)
		if g.Stops[0].Offset <= before {
			t.Fatalf("right: %v -> %v", before, g.Stops[0].Offset)
		}
		d.Key(input.KeyDown, 0)
		if w.st.gsel != 2 { // 0.26 -> 0.5
			t.Fatalf("down selects by offset: %d", w.st.gsel)
		}
		d.Key(input.KeyDelete, 0)
		if len(g.Stops) != 2 {
			t.Fatalf("delete: %+v", g.Stops)
		}
		d.Key(input.KeyDelete, 0)
		if len(g.Stops) != 2 {
			t.Fatal("two stops must stay")
		}
		// Enter adds one after the selected stop.
		d.Key(input.KeyEnter, 0)
		if len(g.Stops) != 3 {
			t.Fatalf("enter adds: %+v", g.Stops)
		}
		if changed == 0 {
			t.Fatal("Changed never reported")
		}
	}
}

// TestColorPickerGradientAngleClearEmpty: the angle input edits Angle,
// clearing drops the stops (the angle stays), and editing an empty
// gradient creates two stops.
func TestColorPickerGradientAngleClearEmpty(t *testing.T) {
	g := cpTestGradient()
	var w *ColorPickerW
	d := NewDriver(func(c *Context) {
		w = ColorPickerGradient(c, &g).AllowClear().Presets(theme.Hex("#ff0000")).Open()
	}, DriverOptions{Width: 700, Height: 800})
	d.Frame()
	d.Frame()
	r, ok := d.FindText(ZhCN.ColorPicker.Angle)
	if !ok {
		t.Fatal("angle label missing")
	}
	d.Click(geom.Pt(r.Right()+30, r.Y+r.H/2))
	d.Key(input.KeyA, input.ModCtrl|input.ModMeta)
	d.Type("45")
	d.Key(input.KeyEnter, 0)
	if g.Angle != 45 {
		t.Fatalf("angle %v", g.Angle)
	}
	d.ClickLabel(ZhCN.ColorPicker.ClearColor)
	if len(g.Stops) != 0 || g.Angle != 45 || w.text() != "" {
		t.Fatalf("clear: %+v text %q", g, w.text())
	}
	d.ClickLabel("#FF0000")
	if len(g.Stops) != 2 || g.Stops[0].Color != theme.Hex("#ff0000") || g.Stops[1].Offset != 1 {
		t.Fatalf("empty edit: %+v", g.Stops)
	}
}

// TestColorPickerGradientTriggerPaint: the trigger swatch is the gradient
// along its angle.
func TestColorPickerGradientTriggerPaint(t *testing.T) {
	g := cpTestGradient()
	var w *ColorPickerW
	d := driver(func(c *Context) { w = ColorPickerGradient(c, &g) })
	d.Frame()
	var l paint.List
	w.paint(&l, d.r.c.rects[w.n.id])
	found := false
	for _, op := range l.Ops {
		if gr, ok := op.(paint.Gradient); ok && len(gr.Stops) == 2 && gr.Stops[0].Color == theme.Hex("#1677ff") {
			found = gr.End.X > gr.Start.X && gr.End.Y == gr.Start.Y // 90deg: left to right
		}
	}
	if !found {
		t.Fatal("trigger gradient not painted")
	}
	p0, p1 := cpGradientLine(geom.Rect{W: 10, H: 10}, 180)
	near := func(a, b geom.Point) bool {
		return math.Abs(float64(a.X-b.X)) < 1e-4 && math.Abs(float64(a.Y-b.Y)) < 1e-4
	}
	if !near(p0, geom.Pt(5, 0)) || !near(p1, geom.Pt(5, 10)) {
		t.Fatalf("180deg line %v %v", p0, p1)
	}
}

// TestColorPickerPanelGradient: the inline panel edits a gradient too.
func TestColorPickerPanelGradient(t *testing.T) {
	g := cpTestGradient()
	changed := false
	d := NewDriver(func(c *Context) { changed = ColorPickerPanelGradient(c, &g).Changed() || changed }, DriverOptions{Width: 600, Height: 700})
	d.Frame()
	bar := gradBarRect(t, d)
	d.Click(geom.Pt(bar.X+bar.W/2, bar.Y+bar.H/2))
	if len(g.Stops) != 3 || !changed {
		t.Fatalf("panel add: %+v", g.Stops)
	}
}

// TestColorPickerPanelSingleUnchanged: the single-color panel (used by the
// code editor) keeps its signature and behavior: no gradient UI, changes
// apply at once to *val in every design system.
func TestColorPickerPanelSingleUnchanged(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		col := theme.Hex("#ff0000")
		var w *ColorPickerPanelW
		d := NewDriver(func(c *Context) {
			w = ColorPickerPanel(c, &col).Presets(theme.Hex("#00ff00"))
		}, DriverOptions{Width: 600, Height: 700, Theme: th})
		d.Frame()
		if _, ok := d.Find(ZhCN.ColorPicker.Gradient); ok {
			t.Fatal("gradient bar in the single-color panel")
		}
		if w.cp.grad != nil || w.cp.val != &col {
			t.Fatal("single panel must bind the color itself")
		}
		d.ClickLabel("#00FF00")
		if col != theme.Hex("#00ff00") {
			t.Fatalf("%v: preset pick -> %v", th.Flavor, col)
		}
	}
}

// TestColorPickerHoverTrigger: hovering opens the panel and leaving
// closes it; a click pins it; the default stays click-only.
func TestColorPickerHoverTrigger(t *testing.T) {
	col := theme.Hex("#1677ff")
	var w *ColorPickerW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			w = ColorPicker(c, &col).Trigger(TriggerHover)
		}, layout.Pad(20))
	}, DriverOptions{Width: 700, Height: 700})
	d.Frame()
	r := d.r.c.rects[w.n.id]
	d.Move(r.Center())
	d.Advance(200 * time.Millisecond)
	d.Frame()
	if !w.IsOpen() || !w.st.hoverOpen {
		t.Fatal("hover did not open")
	}
	// Moving into the panel keeps it open.
	pr := d.r.c.rects[w.st.pop.panel]
	d.Move(pr.Center())
	d.Advance(300 * time.Millisecond)
	d.Frame()
	if !w.IsOpen() {
		t.Fatal("panel closed while hovered")
	}
	d.Move(geom.Pt(690, 690))
	d.Advance(300 * time.Millisecond)
	d.Frame()
	d.Frame()
	if w.IsOpen() {
		t.Fatal("leaving did not close")
	}
	// Hover open, then click: pinned (stays after leaving).
	d.Move(r.Center())
	d.Advance(200 * time.Millisecond)
	d.Frame()
	d.Click(r.Center())
	d.Move(geom.Pt(690, 10))
	d.Advance(300 * time.Millisecond)
	d.Frame()
	if !w.IsOpen() || w.st.hoverOpen {
		t.Fatal("click must pin the panel")
	}

	// Default trigger: hovering does nothing.
	col2 := col
	var w2 *ColorPickerW
	d2 := driver(func(c *Context) { w2 = ColorPicker(c, &col2) })
	d2.Frame()
	d2.Move(d2.r.c.rects[w2.n.id].Center())
	d2.Advance(300 * time.Millisecond)
	d2.Frame()
	if w2.IsOpen() {
		t.Fatal("click trigger opened on hover")
	}
}
