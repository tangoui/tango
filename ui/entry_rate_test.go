package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestEntryRateClickHalfClearKeys(t *testing.T) {
	v := 0.0
	var id ID
	half, disabled := true, false
	changed := 0
	d := driver(func(c *Context) {
		Column(c, func() {
			w := Rate(c, &v)
			if half {
				w.AllowHalf()
			}
			w.Disabled(disabled)
			id = w.n.id
			if w.Changed() {
				changed++
			}
		}, layout.Pad(20))
	})
	d.Frame()
	r := d.r.c.rects[id]
	star := func(i int, frac float32) geom.Point {
		return geom.Pt(r.X+float32(i)*28+20*frac, r.Y+r.H/2)
	}
	// Right half of the 3rd star = 3.
	d.Click(star(2, 0.8))
	if v != 3 || changed != 1 {
		t.Fatalf("v=%v changed=%d", v, changed)
	}
	// Left half of the 2nd star = 1.5.
	d.Click(star(1, 0.2))
	if v != 1.5 {
		t.Fatalf("half: v=%v", v)
	}
	// Clicking the same value clears it.
	d.Click(star(1, 0.2))
	if v != 0 {
		t.Fatalf("clear: v=%v", v)
	}
	// Hover previews without changing.
	var hv float64
	d2 := driver(func(c *Context) { w := Rate(c, &v); id = w.n.id; hv = w.Hovered() })
	d2.Frame()
	r2 := d2.r.c.rects[id]
	d2.Move(geom.Pt(r2.X+4*28+10, r2.Y+r2.H/2))
	d2.Frame()
	if hv != 5 || v != 0 {
		t.Fatalf("hover: hv=%v v=%v", hv, v)
	}
	// Keyboard: focus then arrows.
	d.Click(star(0, 0.9)) // v = 1, focused
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyRight, 0)
	if v != 2 {
		t.Fatalf("keys: v=%v", v)
	}
	d.Key(input.KeyLeft, 0)
	if v != 1.5 {
		t.Fatalf("key left: v=%v", v)
	}
	d.Key(input.KeyEnd, 0)
	if v != 5 {
		t.Fatalf("end: v=%v", v)
	}
	// Disabled ignores input.
	disabled = true
	d.Frame()
	d.Click(star(0, 0.9))
	if v != 5 {
		t.Fatalf("disabled changed value: %v", v)
	}
}

func TestEntryRateTextsAndMeasure(t *testing.T) {
	v := 4.0
	var w *RateW
	d := driver(func(c *Context) { w = Rate(c, &v).Texts("极差", "失望", "一般", "满意", "惊喜") })
	d.Frame()
	if got := w.label(v); got != "满意" {
		t.Fatalf("label = %q", got)
	}
	if got := w.label(0); got != "" {
		t.Fatalf("label(0) = %q", got)
	}
	sz := w.measure(0, 0)
	if sz.W <= 5*20+4*8 {
		t.Fatalf("measure ignores text: %v", sz)
	}
	if formatRateScore(2.5) != "2.5" || formatRateScore(3) != "3" {
		t.Fatal("score format")
	}
}
