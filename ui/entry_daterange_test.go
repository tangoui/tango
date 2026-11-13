package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func dtJan(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }

func TestEntryRangePickerTwoClicksSorted(t *testing.T) {
	var v [2]time.Time
	changed := 0
	var w *RangePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = RangePicker(c, &v)
			if w.Changed() {
				changed++
			}
		}, layout.Pad(20))
	})
	d.Frame()
	d.Click(d.r.c.rects[w.n.id].Center())
	if !w.IsOpen() {
		t.Fatal("not open")
	}
	d.Click(mustFind(t, d, "2026-01-20").Center())
	if !v[0].IsZero() || w.st.pick != 1 {
		t.Fatal("first click should only start the range")
	}
	// Hover preview follows the pointer.
	d.Move(mustFind(t, d, "2026-02-03").Center())
	if !w.st.hover.Equal(time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("hover = %v", w.st.hover)
	}
	d.Click(mustFind(t, d, "2026-01-10").Center())
	if !v[0].Equal(dtJan(10)) || !v[1].Equal(dtJan(20)) || changed != 1 {
		t.Fatalf("range %v changed %d", v, changed)
	}
	if w.IsOpen() {
		t.Fatal("still open")
	}
}

func TestEntryRangePickerKeyboardPresetDisabled(t *testing.T) {
	var v [2]time.Time
	var w *RangePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = RangePicker(c, &v).
				Presets(DatePreset{Label: "本月", Range: [2]time.Time{dtJan(1), dtJan(31)}}).
				DisabledDate(func(t time.Time) bool { return t.Day() == 5 && t.Month() == 1 })
			Button(c, "外部")
		}, layout.Pad(20), layout.Gap(420))
	})
	d.Frame()
	d.Click(d.r.c.rects[w.n.id].Center()) // cursor starts at today (Jan 1)
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyEnter, 0) // Jan 3
	d.Key(input.KeyDown, 0)  // Jan 10
	d.Key(input.KeyEnter, 0)
	if !v[0].Equal(dtJan(3)) || !v[1].Equal(dtJan(10)) {
		t.Fatalf("keyboard range %v", v)
	}
	// Disabled day is ignored.
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Click(mustFind(t, d, "2026-01-05").Center())
	if w.st.pick != 0 {
		t.Fatal("disabled day picked")
	}
	// Presets commit immediately.
	d.Click(mustFind(t, d, "本月").Center())
	if !v[0].Equal(dtJan(1)) || !v[1].Equal(dtJan(31)) || w.IsOpen() {
		t.Fatalf("preset %v open %v", v, w.IsOpen())
	}
	// Escape and outside click close.
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Key(input.KeyEscape, 0)
	if w.IsOpen() {
		t.Fatal("escape did not close")
	}
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Click(mustFind(t, d, "外部").Center())
	if w.IsOpen() {
		t.Fatal("outside click did not close")
	}
}

func TestEntryRangePickerDisabledAndEndSegment(t *testing.T) {
	v := [2]time.Time{dtJan(3), dtJan(10)}
	dis := true
	var w *RangePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() { w = RangePicker(c, &v).Disabled(dis) }, layout.Pad(20))
	})
	d.Frame()
	d.Click(d.r.c.rects[w.n.id].Center())
	if w.IsOpen() {
		t.Fatal("disabled picker opened")
	}
	dis = false
	d.Frame()
	// Clicking the end segment keeps the start and picks a new end.
	r := d.r.c.rects[w.n.id]
	d.Click(r.Center().Add(geomPt(r.W/4, 0)))
	if w.st.active != 1 || w.st.pick != 1 {
		t.Fatalf("active %d pick %d", w.st.active, w.st.pick)
	}
	d.Click(mustFind(t, d, "2026-01-25").Center())
	if !v[0].Equal(dtJan(3)) || !v[1].Equal(dtJan(25)) {
		t.Fatalf("range %v", v)
	}
}
