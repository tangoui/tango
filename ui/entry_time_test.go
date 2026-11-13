package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func dtDriver(page func(*Context)) *Driver {
	return NewDriver(page, DriverOptions{Width: 900, Height: 700})
}

func TestEntryTimePickerClickColumnsAndConfirm(t *testing.T) {
	var v time.Time
	changed := 0
	var field ID
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w := TimePicker(c, &v)
			field = w.f.n.id
			if w.Changed() {
				changed++
			}
		}, layout.Pad(20))
	})
	d.Frame()
	d.Click(d.r.c.rects[field].Center())
	if _, ok := d.Find("h05"); !ok {
		t.Fatal("panel did not open")
	}
	d.Click(mustFind(t, d, "h05").Center())
	d.Click(mustFind(t, d, "m05").Center())
	if !v.IsZero() {
		t.Fatal("value committed before confirm")
	}
	if err := d.ClickLabel("确定"); err != nil {
		t.Fatal(err)
	}
	if v.Hour() != 5 || v.Minute() != 5 || v.Second() != 0 || changed != 1 {
		t.Fatalf("v = %v changed = %d", v, changed)
	}
	if _, ok := d.Find("h05"); ok {
		t.Fatal("panel still open after confirm")
	}
	// Escape discards a draft.
	d.Click(d.r.c.rects[field].Center())
	d.Click(mustFind(t, d, "h07").Center())
	d.Key(input.KeyEscape, 0)
	if _, ok := d.Find("h07"); ok {
		t.Fatal("escape did not close")
	}
	if v.Hour() != 5 {
		t.Fatalf("escape committed draft: %v", v)
	}
}

func TestEntryTimePickerKeyboardAndTyping(t *testing.T) {
	v := time.Date(2026, 1, 1, 10, 20, 30, 0, time.UTC)
	var field ID
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w := TimePicker(c, &v)
			field = w.f.n.id
		}, layout.Pad(20))
	})
	d.Frame()
	d.Click(d.r.c.rects[field].Center())
	d.Key(input.KeyDown, 0)  // hour 11
	d.Key(input.KeyRight, 0) // minute column
	d.Key(input.KeyUp, 0)    // minute 19
	d.Key(input.KeyEnter, 0)
	if v.Hour() != 11 || v.Minute() != 19 || v.Second() != 30 {
		t.Fatalf("keyboard result %v", v)
	}
	// Typing a time and pressing Enter parses it.
	var empty time.Time
	var f2 ID
	d2 := dtDriver(func(c *Context) {
		Column(c, func() { f2 = TimePicker(c, &empty).Format("15:04").f.n.id }, layout.Pad(20))
	})
	d2.Frame()
	d2.Click(d2.r.c.rects[f2].Center())
	d2.Type("08:45")
	if _, ok := d2.Find("m45"); !ok {
		t.Fatal("minute column missing")
	}
	d2.Key(input.KeyEnter, 0)
	if empty.Hour() != 8 || empty.Minute() != 45 {
		t.Fatalf("typed value %v", empty)
	}
}

func TestEntryTimePickerDisabledClearOutside(t *testing.T) {
	v := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	dis := true
	var field ID
	var w *TimePickerW
	d := dtDriver(func(c *Context) {
		Column(c, func() {
			w = TimePicker(c, &v).Disabled(dis)
			field = w.f.n.id
			Button(c, "外部")
		}, layout.Pad(20), layout.Gap(300))
	})
	d.Frame()
	d.Click(d.r.c.rects[field].Center())
	if w.IsOpen() {
		t.Fatal("disabled picker opened")
	}
	dis = false
	d.Frame()
	d.Click(d.r.c.rects[field].Center())
	if !w.IsOpen() {
		t.Fatal("picker did not open")
	}
	d.Click(mustFind(t, d, "外部").Center())
	if w.IsOpen() {
		t.Fatal("outside click did not close")
	}
	// Clear button appears on hover at the suffix.
	r := d.r.c.rects[field]
	p := geom.Pt(r.Right()-11-7, r.Y+r.H/2)
	d.Move(p)
	d.Click(p)
	if !v.IsZero() {
		t.Fatalf("clear failed: %v", v)
	}
}

func TestEntryTimePanelKeys(t *testing.T) {
	v := time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC)
	var id ID
	d := dtDriver(func(c *Context) {
		w := TimePanel(c, &v).Format("15:04").Steps(1, 15, 1)
		id = w.n.id
	})
	d.Frame()
	d.r.c.SetFocus(id)
	d.Key(input.KeyDown, 0) // wraps to 00
	if v.Hour() != 0 {
		t.Fatalf("hour %d", v.Hour())
	}
	d.Key(input.KeyRight, 0)
	d.Key(input.KeyDown, 0)
	if v.Minute() != 15 {
		t.Fatalf("minute %d", v.Minute())
	}
	if _, ok := d.Find("m07"); ok {
		t.Fatal("minute step ignored")
	}
}
