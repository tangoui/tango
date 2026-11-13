package ui

// RTL tests of group G2: date family, time pickers, Calendar, Slider,
// ColorPicker and Upload (docs/specs/rtl-and-component-tokens.md §3.5,
// §3.6).

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// g2Driver renders page in an RTL scope (Ant light theme).
func g2Driver(page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() { page(c) })
	}, DriverOptions{Width: 700, Height: 600, Theme: theme.AntDesign()})
	d.Settle()
	return d
}

func g2Near(a, b float32) bool { return a-b < 0.75 && b-a < 0.75 }

// TestG2SliderRTL: min at the right, pointer ↔ value mirrored, the filled
// part from the right, arrow keys toward the max side (Left) increase.
func TestG2SliderRTL(t *testing.T) {
	v := 20.0
	var w *SliderW
	d := g2Driver(func(c *Context) {
		Column(c, func() { w = Slider(c, &v, 0, 100).Layout(layout.W(412)) }, layout.Pad(20))
	})
	tr := p2bRect(t, d, w.track)
	h := p2bRect(t, d, w.thumbs[0])
	want := tr.Right() - sliderPad - (tr.W-2*sliderPad)*0.2
	if !g2Near(h.Center().X, want) {
		t.Fatalf("handle at x %v, want %v (min at the right)", h.Center().X, want)
	}
	// The track fill runs from the right end to the handle.
	var fill *paint.FillRect
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Color == d.r.c.Theme.Components.Slider.TrackBg && f.Rect.Y > tr.Y && f.Rect.Bottom() < tr.Bottom() {
			fill = &f
		}
	}
	if fill == nil || !g2Near(fill.Rect.Right(), tr.Right()-sliderPad) || !g2Near(fill.Rect.X, want) {
		t.Fatalf("track fill %+v, want [%v, %v]", fill, want, tr.Right()-sliderPad)
	}
	// A rail press at 70% from the right edge.
	d.Click(geom.Pt(tr.Right()-sliderPad-(tr.W-2*sliderPad)*0.7, tr.Y+11))
	if v != 70 {
		t.Fatalf("rail press: v = %v, want 70", v)
	}
	keys := []struct {
		k    input.Key
		want float64
	}{{input.KeyLeft, 71}, {input.KeyLeft, 72}, {input.KeyRight, 71}, {input.KeyUp, 72}, {input.KeyDown, 71}, {input.KeyHome, 0}, {input.KeyEnd, 100}}
	for _, k := range keys {
		d.Key(k.k, 0)
		d.Settle()
		if v != k.want {
			t.Fatalf("key %v: v = %v, want %v", k.k, v, k.want)
		}
	}
}

// TestG2SliderLTRKeys keeps LTR: Right increases.
func TestG2SliderLTRKeys(t *testing.T) {
	v := 50.0
	var w *SliderW
	d := NewDriver(func(c *Context) {
		w = Slider(c, &v, 0, 100).Layout(layout.W(300))
	}, DriverOptions{Width: 400, Height: 200})
	d.Settle()
	d.Click(p2bRect(t, d, w.thumbs[0]).Center())
	d.Key(input.KeyRight, 0)
	d.Settle()
	d.Key(input.KeyRight, 0)
	d.Settle()
	d.Key(input.KeyLeft, 0)
	d.Settle()
	if v != 51 {
		t.Fatalf("LTR keys: v = %v, want 51", v)
	}
}

// TestG2SliderRangeRTLDrag: dragging the end handle toward the left
// increases it.
func TestG2SliderRangeRTLDrag(t *testing.T) {
	lo, hi := 20.0, 60.0
	var w *SliderW
	d := g2Driver(func(c *Context) {
		Column(c, func() { w = SliderRange(c, &lo, &hi, 0, 100).Layout(layout.W(412)) }, layout.Pad(20))
	})
	tr := p2bRect(t, d, w.track)
	h1 := p2bRect(t, d, w.thumbs[1])
	if h0 := p2bRect(t, d, w.thumbs[0]); h0.X <= h1.X {
		t.Fatalf("start handle %v not right of end handle %v", h0, h1)
	}
	x80 := tr.Right() - sliderPad - (tr.W-2*sliderPad)*0.8
	d.Drag(h1.Center(), geom.Pt(x80, h1.Center().Y))
	if hi != 80 || lo != 20 {
		t.Fatalf("drag: lo %v hi %v", lo, hi)
	}
}

// TestG2DateGridRTL: the week runs right to left, the cursor moves
// visually (Left = next day), and the range strip of the start day covers
// its inline-end half.
func TestG2DateGridRTL(t *testing.T) {
	if !dtMoveCursorOK() {
		t.Fatal("dtMoveCursor")
	}
	val := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	var w *DatePickerPanelW
	d := g2Driver(func(c *Context) { w = DatePickerPanel(c, &val) })
	sun, ok1 := d.Find("2026-01-04")
	mon, ok2 := d.Find("2026-01-05")
	if !ok1 || !ok2 {
		t.Fatal("day cells not found")
	}
	if !(sun.X > mon.X) || sun.Y != mon.Y {
		t.Fatalf("week not right to left: Sun %v Mon %v", sun, mon)
	}
	// Keyboard on the focused panel: Left is the next day in RTL.
	d.r.c.SetFocus(w.n.id)
	d.Settle()
	d.Key(input.KeyLeft, 0)
	d.Settle()
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if want := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC); !val.Equal(want) {
		t.Fatalf("Left+Enter picked %v, want %v", val, want)
	}
}

func dtMoveCursorOK() bool {
	d := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	l, _ := dtMoveCursor(d, input.KeyLeft, true)
	r, _ := dtMoveCursor(d, input.KeyRight, true)
	l2, _ := dtMoveCursor(d, input.KeyLeft, false)
	u, _ := dtMoveCursor(d, input.KeyUp, true)
	return l.Day() == 16 && r.Day() == 14 && l2.Day() == 14 && u.Day() == 8
}

// TestG2DateRangeStripRTL: the selected range's start strip covers the
// left half of its cell in RTL (the range continues to the left).
func TestG2DateRangeStripRTL(t *testing.T) {
	a := time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)
	d := g2Driver(func(c *Context) { RangePicker(c, &[2]time.Time{a, b}).Open() })
	cell, ok := d.Find("2026-01-13")
	if !ok {
		t.Fatal("start day not found")
	}
	strip := d.r.c.Theme.Primary.Bg
	found := false
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Color == strip && f.Rect.Y >= cell.Y && f.Rect.Bottom() <= cell.Bottom() &&
			f.Rect.X >= cell.X-0.5 && f.Rect.Right() <= cell.Right()+0.5 {
			found = true
			if !g2Near(f.Rect.X, cell.X) || !g2Near(f.Rect.W, cell.W/2) {
				t.Fatalf("start strip %v in cell %v, want the left half", f.Rect, cell)
			}
		}
	}
	if !found {
		t.Fatal("no range strip in the start cell")
	}
}

// TestG2RangePickerRTLField: the start end is at the right (press on the
// right half opens the start), the calendar icon at the left.
func TestG2RangePickerRTLField(t *testing.T) {
	var v [2]time.Time
	var w *RangePickerW
	d := g2Driver(func(c *Context) { w = RangePicker(c, &v) })
	r := d.r.c.rects[w.n.id]
	d.Click(geom.Pt(r.Right()-30, r.Y+r.H/2))
	if !w.IsOpen() || w.st.active != 0 {
		t.Fatalf("press on the right half: open=%v active=%d, want the start", w.IsOpen(), w.st.active)
	}
	d.Click(geom.Pt(r.X+40, r.Y+r.H/2))
	if w.st.active != 1 {
		t.Fatalf("press on the left half: active=%d, want the end", w.st.active)
	}
	if cr := w.st.clearR; cr.X > r.X+r.W/2 {
		t.Fatalf("suffix icon at %v, want the inline end (left)", cr)
	}
}

// TestG2DatePickerRTLField: icon / clear rect at the left in RTL.
func TestG2DatePickerRTLField(t *testing.T) {
	v := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	var w *DatePickerW
	d := g2Driver(func(c *Context) { w = DatePicker(c, &v) })
	r := d.r.c.rects[w.n.id]
	if cr := w.st.clearR; !g2Near(cr.X, r.X+11) {
		t.Fatalf("clear rect %v, want x %v", cr, r.X+11)
	}
}

// TestG2TimeColumnsStayLTR: hh / mm / ss read left to right in RTL (Q5).
func TestG2TimeColumnsStayLTR(t *testing.T) {
	v := time.Date(2026, 1, 15, 10, 20, 30, 0, time.UTC)
	d := g2Driver(func(c *Context) { TimePanel(c, &v) })
	h, ok1 := d.Find("h10")
	m, ok2 := d.Find("m20")
	s, ok3 := d.Find("s30")
	if !ok1 || !ok2 || !ok3 {
		t.Fatal("time options not found")
	}
	if !(h.X < m.X && m.X < s.X) {
		t.Fatalf("time columns not left to right: h %v m %v s %v", h, m, s)
	}
}

// TestG2CalendarRTL: first weekday at the right; Left moves to the next
// day.
func TestG2CalendarRTL(t *testing.T) {
	val := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	d := g2Driver(func(c *Context) { Calendar(c, &val).Card() })
	sun, ok1 := d.Find("2026-01-04")
	mon, ok2 := d.Find("2026-01-05")
	if !ok1 || !ok2 || !(sun.X > mon.X) {
		t.Fatalf("calendar week not right to left: %v %v", sun, mon)
	}
	d.Click(mustFind(t, d, "2026-01-15").Center())
	d.Key(input.KeyLeft, 0)
	d.Settle()
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if val.Day() != 16 {
		t.Fatalf("Left+Enter in RTL picked %v, want the 16th", val)
	}
}

// TestG2ColorHueRTL: the hue bar starts at the right; Left increases.
func TestG2ColorHueRTL(t *testing.T) {
	col := theme.FromHSV(theme.HSV{H: 180, S: 1, V: 1})
	d := g2Driver(func(c *Context) { ColorPickerPanel(c, &col) })
	bar := mustFind(t, d, d.r.c.Locale().ColorPicker.Hue)
	// Press near the right end: hue near 0.
	d.Click(geom.Pt(bar.Right()-9, bar.Y+bar.H/2))
	d.Settle()
	h0 := theme.ToHSVA(col).H
	if h0 > 20 && h0 < 340 {
		t.Fatalf("press at the right end: hue %v, want ≈0", h0)
	}
	d.Click(geom.Pt(bar.X+bar.W/2, bar.Y+bar.H/2))
	d.Settle()
	h1 := theme.ToHSVA(col).H
	d.Key(input.KeyLeft, 0)
	d.Settle()
	if h2 := theme.ToHSVA(col).H; !(h2 > h1) {
		t.Fatalf("Left in RTL: hue %v -> %v, want an increase", h1, h2)
	}
}

// TestG2UploadListRTL: the remove button sits at the inline end (left) of
// the file row and the progress bar fills from the right.
func TestG2UploadListRTL(t *testing.T) {
	files := []UploadFile{{Name: "a.txt", Status: "done"}, {Name: "b.txt", Status: "uploading", Percent: 50}}
	d := g2Driver(func(c *Context) {
		Column(c, func() { Upload(c, &files) }, layout.W(400))
	})
	rr := mustFind(t, d, d.r.c.Locale().RemoveItem("a.txt"))
	row := mustFind(t, d, "a.txt")
	if !(rr.X < row.X+row.W/2) {
		t.Fatalf("remove %v not at the left of row %v", rr, row)
	}
	var bar, fill *paint.FillRect
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.H == 2 {
			switch f.Color {
			case d.r.c.Theme.FillSecondary:
				bar = &f
			case d.r.c.Theme.Primary.Base:
				fill = &f
			}
		}
	}
	if bar == nil || fill == nil || !g2Near(fill.Rect.Right(), bar.Rect.Right()) || !g2Near(fill.Rect.W, bar.Rect.W/2) {
		t.Fatalf("progress bar %+v fill %+v, want the fill at the right half", bar, fill)
	}
}

// TestG2MirrorCheck: the node layout of the G2 components mirrors in RTL
// (time columns are an LTR scope and skipped).
func TestG2MirrorCheck(t *testing.T) {
	val := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	rv := [2]time.Time{val, val.AddDate(0, 0, 5)}
	v, lo, hi := 30.0, 20.0, 60.0
	col := theme.Hex("#1677ff")
	files := []UploadFile{{Name: "a.txt", Status: "done"}, {Name: "b.txt", Status: "error"}}
	pages := map[string]func(c *Context){
		"panel":    func(c *Context) { DatePickerPanel(c, &val) },
		"calendar": func(c *Context) { Calendar(c, &val).Card() },
		"calfull":  func(c *Context) { Calendar(c, &val) },
		"slider": func(c *Context) {
			Column(c, func() {
				Slider(c, &v, 0, 100)
				SliderRange(c, &lo, &hi, 0, 100).Marks(map[float64]string{0: "0", 50: "50"})
			}, layout.W(400))
		},
		"color":  func(c *Context) { ColorPickerPanel(c, &col) },
		"upload": func(c *Context) { Column(c, func() { Upload(c, &files) }, layout.W(400)) },
		"range":  func(c *Context) { RangePicker(c, &rv).Open() },
		"time":   func(c *Context) { TimePanel(c, &val) },
	}
	for name, p := range pages {
		for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
			d := NewDriver(p, DriverOptions{Width: 900, Height: 700, Theme: th})
			rep := d.MirrorCheck(p)
			if !rep.OK() {
				t.Errorf("%s/%v: %s", name, th.Flavor, rep)
			}
		}
	}
}
