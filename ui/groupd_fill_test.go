package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// gdTextColor returns the color of the first text op whose glyphs start
// inside r.
func gdTextColor(d *Driver, r geom.Rect) (theme.Color, bool) {
	for _, op := range d.r.List().Ops {
		if t, ok := op.(paint.Text); ok && r.Contains(t.Origin) {
			return t.Color, true
		}
	}
	return theme.Color{}, false
}

func TestAlertDarkEffect(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn(theme.Dark())} {
		d := NewDriver(func(c *Context) {
			Column(c, func() {
				Alert(c, "error", "Dark alert").Effect(AlertEffectDark).Description("details").Closable().Show()
			}, layout.Pad(20))
		}, DriverOptions{Width: 600, Height: 300, Theme: th})
		d.Settle()
		tk := th.Components.Alert
		if t2bCount(d, tk.ErrorDarkBg) == 0 {
			t.Fatalf("%s: dark alert does not fill with ErrorDarkBg", th.Name)
		}
		if t2bCount(d, tk.ErrorBg) != 0 && tk.ErrorBg != tk.ErrorDarkBg && tk.ErrorBg != th.BgContainer {
			t.Errorf("%s: dark alert still paints the light background", th.Name)
		}
		r, ok := d.FindText("Dark alert")
		if !ok {
			t.Fatal("title missing")
		}
		col, ok := gdTextColor(d, r.Expand(2))
		if want := th.TextOn(tk.ErrorDarkBg, tk.DarkTextColor); !ok || col != want {
			t.Errorf("%s: title color %v, want %v", th.Name, col, want)
		}
	}
}

func TestAlertCenter(t *testing.T) {
	var box geom.Rect
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Alert(c, "info", "Centered").Center().Show()
		}, layout.Pad(20))
	}, DriverOptions{Width: 600, Height: 200})
	d.Settle()
	box = geom.Rect{X: 20, Y: 20, W: 560}
	r, ok := d.FindText("Centered")
	if !ok {
		t.Fatal("title missing")
	}
	mid := box.X + box.W/2
	// The icon (14 + 8) sits before the title, so the title center is a
	// little right of the box center; it must be far from the start.
	if cx := r.X + r.W/2; cx < mid-10 || cx > mid+30 {
		t.Fatalf("centered title at %v, box center %v", cx, mid)
	}
	d2 := NewDriver(func(c *Context) {
		Column(c, func() { Alert(c, "info", "Start").Show() }, layout.Pad(20))
	}, DriverOptions{Width: 600, Height: 200})
	d2.Settle()
	if r, _ := d2.FindText("Start"); r.X > 80 {
		t.Fatalf("default alert title not at the start: %v", r)
	}
}
