package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func geomPt(x, y float32) geom.Point { return geom.Pt(x, y) }

func TestEntryDateTimePickerPickAndConfirm(t *testing.T) {
	var v time.Time
	changed := 0
	var w *DateTimePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = DateTimePicker(c, &v)
			if w.Changed() {
				changed++
			}
		}, layout.Pad(20))
	})
	d.Frame()
	d.Click(d.r.c.rects[w.f.n.id].Center())
	d.Click(mustFind(t, d, "2026-01-15").Center())
	d.Click(mustFind(t, d, "h06").Center())
	d.Click(mustFind(t, d, "m03").Center())
	if !v.IsZero() {
		t.Fatal("committed before confirm")
	}
	if err := d.ClickLabel("确定"); err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 15, 6, 3, 0, 0, time.UTC)
	if !v.Equal(want) || changed != 1 {
		t.Fatalf("v = %v changed %d", v, changed)
	}
	// Keyboard: arrows drive the time columns, Enter commits.
	d.Click(d.r.c.rects[w.f.n.id].Center())
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEnter, 0)
	if v.Hour() != 7 || v.Day() != 15 {
		t.Fatalf("keyboard v = %v", v)
	}
}

func TestEntryDateTimePickerTyping(t *testing.T) {
	var v time.Time
	var w *DateTimePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() { w = DateTimePicker(c, &v).Format("2006-01-02 15:04") }, layout.Pad(20))
	})
	d.Frame()
	d.Click(d.r.c.rects[w.f.n.id].Center())
	d.Type("2026-03-08 21:15")
	d.Key(input.KeyEnter, 0)
	if !v.Equal(time.Date(2026, 3, 8, 21, 15, 0, 0, time.UTC)) {
		t.Fatalf("typed v = %v", v)
	}
	if w.IsOpen() {
		t.Fatal("still open")
	}
}
