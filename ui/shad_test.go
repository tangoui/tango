package ui

import (
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

// TestShadcnStyleSwitch renders the same page under the three design
// systems and checks the per-style geometry and colors come from tokens.
func TestShadcnStyleSwitch(t *testing.T) {
	name := "x"
	on := true
	page := func(c *Context) {
		Column(c, func() {
			Button(c, "OK").Primary()
			Input(c, &name)
			Checkbox(c, "Agree", &on)
		})
	}
	d := NewDriver(page, DriverOptions{Width: 400, Height: 300})
	for _, tc := range []struct {
		th     *theme.Theme
		height float32
	}{
		{theme.AntDesign(), 32},
		{theme.ElementPlus(), 32},
		{theme.Shadcn(), 36},
		{theme.Shadcn(theme.Dark()), 36},
		{theme.Shadcn(theme.ShadcnOptions{Base: "slate", Compact: true}), 32},
	} {
		d.SetTheme(tc.th)
		img := d.Settle()
		r, ok := d.Find("OK")
		if !ok {
			t.Fatalf("%s: button missing", tc.th.Name)
		}
		if r.H != tc.height {
			t.Errorf("%s dark=%v: button height %v, want %v", tc.th.Name, tc.th.Dark, r.H, tc.height)
		}
		// The primary fill is the theme's primary color.
		px := img.At(int((r.X+4)*1), int((r.Y+r.H/2)*1))
		want := tc.th.Primary.Base
		if got := color.NRGBAModel.Convert(px).(color.NRGBA); !nearColor(got, want, 24) {
			t.Errorf("%s dark=%v: button fill %v, want %v", tc.th.Name, tc.th.Dark, got, want)
		}
	}
}

func nearColor(a, b color.NRGBA, tol int) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol
}

// TestConfigProviderShadcnFlavor switches flavor and dark mode through
// ConfigProvider and keeps the shadcn base color.
func TestConfigProviderShadcnFlavor(t *testing.T) {
	var got *theme.Theme
	dark := false
	flavor := theme.StyleShadcn
	page := func(c *Context) {
		ConfigProvider(c).Flavor(flavor).Dark(dark).Body(func() { got = c.Theme })
	}
	d := NewDriver(page, DriverOptions{Width: 200, Height: 100, Theme: theme.Shadcn(theme.ShadcnOptions{Base: "stone", Radius: 10})})
	d.Settle()
	if got.Flavor != theme.StyleShadcn || got.Shadcn.Options.Base != "stone" || got.RadiusLG != 10 {
		t.Fatalf("flavor=%v base=%v radius=%v", got.Flavor, got.Shadcn.Options.Base, got.RadiusLG)
	}
	dark = true
	d.Settle()
	if !got.Dark || got.Shadcn.Options.Base != "stone" {
		t.Errorf("dark toggle lost: dark=%v base=%v", got.Dark, got.Shadcn.Options.Base)
	}
	flavor = theme.Element
	d.Settle()
	if got.Flavor != theme.Element || !got.Dark {
		t.Errorf("switch to element: %v dark=%v", got.Flavor, got.Dark)
	}
}

func TestCommandFilterAndPick(t *testing.T) {
	items := []CommandItem{
		{Value: "cal", Label: "Calendar", Group: "Suggestions"},
		{Value: "calc", Label: "Calculator", Group: "Suggestions", Disabled: true},
		{Value: "prof", Label: "Profile", Group: "Settings", Keywords: []string{"account"}},
		{Value: "set", Label: "Settings", Group: "Settings"},
	}
	var picked string
	var w *CommandW
	page := func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() {
			w = Command(c, items)
			if it, ok := w.Selected(); ok {
				picked = it.Value
			}
		})
	}
	d := NewDriver(page, DriverOptions{Width: 400, Height: 500, Theme: theme.Shadcn()})
	d.Settle()
	if w.place != "Type a command or search..." || w.empty != "No results found." {
		t.Errorf("not localized: %q %q", w.place, w.empty)
	}
	r := d.r.c.rects[w.in.box.id]
	d.Click(r.Center())
	d.Type("acc")
	d.Settle()
	if _, ok := d.Find("Calendar"); ok {
		t.Error("filter kept Calendar")
	}
	if _, ok := d.Find("Profile"); !ok {
		t.Fatal("keyword match missing")
	}
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if picked != "prof" {
		t.Fatalf("picked %q", picked)
	}
	// Clear the query, arrow down skips the disabled Calculator.
	for range 3 {
		d.Key(input.KeyBackspace, 0)
	}
	d.Settle()
	d.Key(input.KeyDown, 0)
	d.Settle()
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if picked != "prof" {
		t.Errorf("down from Calendar should skip disabled Calculator, picked %q", picked)
	}
	r = d.r.c.rects[w.in.box.id]
	d.Click(r.Center())
	d.Type("zzz")
	d.Settle()
	if _, ok := d.FindText("No results found."); !ok {
		t.Error("empty text missing")
	}
	_ = w
}

func TestToggleGroup(t *testing.T) {
	bold := false
	multi := []string{}
	single := []string{"l"}
	page := func(c *Context) {
		Toggle(c, "B", &bold)
		ToggleGroup(c, &multi, Opts("x", "y"))
		ToggleGroup(c, &single, []Option{{Label: "L", Value: "l"}, {Label: "R", Value: "r"}}).Single().Outline()
		Kbd(c, "⌘K")
	}
	d := NewDriver(page, DriverOptions{Width: 400, Height: 300, Theme: theme.Shadcn()})
	d.Settle()
	d.ClickLabel("B")
	d.ClickLabel("x")
	d.ClickLabel("y")
	d.ClickLabel("R")
	d.Settle()
	if !bold || len(multi) != 2 || len(single) != 1 || single[0] != "r" {
		t.Fatalf("bold=%v multi=%v single=%v", bold, multi, single)
	}
	d.ClickLabel("x")
	d.ClickLabel("R")
	d.Settle()
	if len(multi) != 1 || multi[0] != "y" || len(single) != 0 {
		t.Errorf("untoggle: multi=%v single=%v", multi, single)
	}
	if _, ok := d.FindText("⌘K"); !ok {
		t.Error("kbd label")
	}
}
