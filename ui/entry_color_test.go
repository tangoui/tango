package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// TestColorPickerHSBFormat: Format("hsb") shows hsb() text, Ant's panel
// edits hue / saturation / brightness, and Element accepts typed hsb text.
func TestColorPickerHSBFormat(t *testing.T) {
	col := theme.Hex("#3366cc") // hsb(220, 75%, 80%)
	var w *ColorPickerW
	d := NewDriver(func(c *Context) {
		w = ColorPicker(c, &col).Format("hsb").ShowText().Open()
	}, DriverOptions{Width: 600, Height: 700})
	d.Frame()
	d.Frame()
	if s := w.text(); s != "hsb(220, 75%, 80%)" {
		t.Fatalf("text = %q", s)
	}
	if w.st.bufs[1] != "220" || w.st.bufs[2] != "75" || w.st.bufs[3] != "80" {
		t.Fatalf("hsb inputs %q", w.st.bufs[1:4])
	}
	// Typing a brightness of 40%.
	d.r.c.SetFocus(w.st.ids[3])
	d.Frame()
	d.Key(input.KeyA, input.ModCtrl|input.ModMeta)
	d.Type("40")
	if h := theme.ToHSV(col); h.V < .39 || h.V > .41 || h.H < 219 || h.H > 221 {
		t.Fatalf("brightness typed -> %v (%+v)", col, h)
	}
	if w.Format("hsv"); w.text() != "hsv(220, 75%, 40%)" {
		t.Fatalf("hsv text = %q", w.text())
	}

	// Element: the value field shows hsb text and parses it.
	col2 := theme.Hex("#ff0000")
	var w2 *ColorPickerW
	d2 := NewDriver(func(c *Context) { w2 = ColorPicker(c, &col2).Format("hsb").Open() }, DriverOptions{Width: 600, Height: 700, Theme: theme.ElementPlus()})
	d2.Frame()
	d2.Frame()
	if w2.st.bufs[0] != "hsb(0, 100%, 100%)" {
		t.Fatalf("element value text %q", w2.st.bufs[0])
	}
	d2.r.c.SetFocus(w2.st.ids[0])
	d2.Frame()
	d2.Key(input.KeyA, input.ModCtrl|input.ModMeta)
	d2.Type("hsb(120, 100%, 50%)")
	d2.Key(input.KeyEnter, 0)
	d2.ClickLabel(ZhCN.OK)
	if col2 != theme.Hex("#008000") {
		t.Fatalf("element hsb typed -> %v", col2)
	}
}

// TestColorPickerPresetGroups: titled groups, each a radio group; Ant's
// titles collapse / expand their swatches (Closed starts collapsed).
func TestColorPickerPresetGroups(t *testing.T) {
	col := theme.Hex("#000000")
	groups := []ColorPresetGroup{
		{Label: "主色", Colors: []theme.Color{theme.Hex("#1677ff"), theme.Hex("#52c41a")}},
		{Label: "最近使用", Colors: []theme.Color{theme.Hex("#fa8c16")}, Closed: true},
	}
	d := NewDriver(func(c *Context) {
		ColorPicker(c, &col).PresetGroups(groups...).Open()
	}, DriverOptions{Width: 600, Height: 800})
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleRadioGroup, "主色"); !ok {
		t.Fatal("first group missing")
	}
	if _, ok := d.Find("#FA8C16"); ok {
		t.Fatal("closed group shows its swatches")
	}
	hd, ok := d.A11yFind(a11y.RoleButton, "最近使用")
	if !ok || hd.States.Has(a11y.Expanded) || !hd.States.Has(a11y.Expandable) {
		t.Fatalf("closed group header %+v", hd)
	}
	d.ClickLabel("#52C41A")
	if col != theme.Hex("#52c41a") {
		t.Fatalf("preset click -> %v", col)
	}
	// Open the second group with the keyboard.
	d.PerformAction(hd.ID, a11y.Focus, "")
	d.Settle()
	d.Key(input.KeyEnter, 0)
	if _, ok := d.Find("#FA8C16"); !ok {
		t.Fatal("group did not expand")
	}
	if g, ok := d.A11yFind(a11y.RoleRadioGroup, "最近使用"); !ok || len(g.Children) == 0 {
		t.Fatal("second group not a radio group")
	}
	d.ClickLabel("#FA8C16")
	if col != theme.Hex("#fa8c16") {
		t.Fatalf("second group preset -> %v", col)
	}
	// Collapsing the first group hides its swatches.
	if r, ok := d.FindText("主色"); ok {
		d.Click(r.Center())
	}
	if _, ok := d.Find("#1677FF"); ok {
		t.Fatal("first group did not collapse")
	}

	// Element shows every group with its title.
	col3 := theme.Hex("#000000")
	d3 := NewDriver(func(c *Context) {
		ColorPicker(c, &col3).PresetGroups(groups...).Open()
	}, DriverOptions{Width: 600, Height: 800, Theme: theme.ElementPlus()})
	d3.Settle()
	if _, ok := d3.FindText("最近使用"); !ok {
		t.Fatal("element group title missing")
	}
	if _, ok := d3.Find("#FA8C16"); !ok {
		t.Fatal("element shows all groups")
	}
}
