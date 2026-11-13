package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func TestMoreTimeSelectItems(t *testing.T) {
	items := moreTimeItems("08:30", "18:30", "00:15", "09:00", "18:00")
	if len(items) != 41 || items[0].Value != "08:30" || items[40].Value != "18:30" {
		t.Fatalf("items %d %v..%v", len(items), items[0], items[len(items)-1])
	}
	for _, it := range items {
		want := it.Value <= "09:00" || it.Value >= "18:00"
		if it.Disabled != want {
			t.Fatalf("%s disabled=%v", it.Value, it.Disabled)
		}
	}
	if got := moreTimeItems("bad", "18:00", "00:30", "", ""); got != nil {
		t.Fatal("bad start accepted")
	}
	if got := moreTimeItems("00:00", "23:59", "00:00", "", ""); got != nil {
		t.Fatal("zero step accepted")
	}
}

func TestMoreTimeSelectPickKeysClear(t *testing.T) {
	val := ""
	changes := 0
	disabled := false
	var w *TimeSelectW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = TimeSelect(c, &val).Start("08:30").End("18:30").Step("00:15").MinTime("09:00").Disabled(disabled)
			if w.Changed() {
				changes++
			}
			Button(c, "其他")
		}, layout.Pad(20), layout.Gap(8))
	})
	d.Frame()
	box := d.r.c.rects[w.n.id]
	if box.W != 160 || box.H != 32 {
		t.Fatalf("ant box %v", box)
	}
	d.Click(box.Center())
	if !w.IsOpen() {
		t.Fatal("not open")
	}
	// Disabled times ignore clicks.
	d.ClickLabel("08:45")
	if val != "" || !w.IsOpen() {
		t.Fatalf("disabled option picked: %q", val)
	}
	d.ClickLabel("10:00")
	if val != "10:00" || changes != 1 || w.IsOpen() {
		t.Fatalf("pick: %q %d open=%v", val, changes, w.IsOpen())
	}
	// Keyboard: focused after picking; Down opens at the value, Down moves,
	// Enter picks.
	d.Key(input.KeyDown, 0)
	if !w.IsOpen() || w.st.active != w.index("10:00") {
		t.Fatalf("down open: active=%d", w.st.active)
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEnter, 0)
	if val != "10:15" || w.IsOpen() {
		t.Fatalf("keys: %q", val)
	}
	// Space opens, Escape closes.
	d.Key(input.KeySpace, 0)
	d.Key(input.KeyEscape, 0)
	if w.IsOpen() {
		t.Fatal("escape")
	}
	// Tab away closes the list.
	d.Key(input.KeySpace, 0)
	d.Key(input.KeyTab, 0)
	if w.IsOpen() {
		t.Fatal("tab away")
	}
	// Opening on a late value scrolls it into the panel.
	val = "18:15"
	d.Frame()
	d.Click(box.Center())
	pr := d.r.c.rects[w.st.panel]
	opt := mustFind(t, d, "18:15")
	if opt.Y < pr.Y || opt.Bottom() > pr.Bottom() {
		t.Fatalf("selected %v outside panel %v", opt, pr)
	}
	if pr.H > 270 {
		t.Fatalf("panel too tall %v", pr)
	}
	// Keyboard moves keep the highlight inside the panel.
	for i := 0; i < 12; i++ {
		d.Key(input.KeyUp, 0)
	}
	hl := mustFind(t, d, w.items[w.st.active].Label)
	if pr = d.r.c.rects[w.st.panel]; hl.Y < pr.Y || hl.Bottom() > pr.Bottom() {
		t.Fatalf("highlight %v outside panel %v", hl, pr)
	}
	// Outside click closes.
	d.Click(geom.Pt(590, 390))
	if w.IsOpen() {
		t.Fatal("outside click")
	}
	// Clear icon on hover.
	d.Move(box.Center())
	d.Click(geom.Pt(box.Right()-18, box.Center().Y))
	if val != "" || w.IsOpen() {
		t.Fatalf("clear: %q open=%v", val, w.IsOpen())
	}
	// Disabled control does not open.
	disabled = true
	d.Frame()
	d.Click(box.Center())
	if w.IsOpen() {
		t.Fatal("disabled opened")
	}
}

func TestMoreTimeSelectElement(t *testing.T) {
	val := "09:30"
	var w *TimeSelectW
	d := NewDriver(func(c *Context) {
		Column(c, func() { w = TimeSelect(c, &val).Step("00:30") }, layout.Pad(20))
	}, DriverOptions{Width: 600, Height: 500, Theme: theme.ElementPlus()})
	d.Frame()
	box := d.r.c.rects[w.n.id]
	if box.W != 240 {
		t.Fatalf("element width %v", box)
	}
	d.Click(box.Center())
	opt := mustFind(t, d, "10:00")
	if opt.H != 34 {
		t.Fatalf("element option %v", opt)
	}
	d.ClickLabel("09:00")
	if val != "09:00" {
		t.Fatalf("pick %q", val)
	}
}
