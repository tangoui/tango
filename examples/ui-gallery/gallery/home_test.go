package gallery

import (
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// TestHomeThemeSwitch drives the design-system switch of the gallery home.
func TestHomeThemeSwitch(t *testing.T) {
	sel := "home"
	var cur *theme.Theme
	page := func(c *ui.Context) {
		Home(c, &sel)
		cur = c.Theme
	}
	d := ui.NewDriver(page, ui.DriverOptions{Width: 1200, Height: 800})
	d.Settle()
	if cur.Flavor != theme.Ant {
		t.Fatalf("start flavor %v", cur.Flavor)
	}
	for _, step := range []struct {
		label string
		want  theme.Flavor
	}{{"shadcn/ui", theme.StyleShadcn}, {"Element Plus", theme.Element}, {"shadcn/ui", theme.StyleShadcn}} {
		if err := d.ClickLabel(step.label); err != nil {
			t.Fatal(err)
		}
		d.Settle()
		if cur.Flavor != step.want {
			t.Errorf("after %s: flavor %v", step.label, cur.Flavor)
		}
	}
	homeStyle, homeDark = "", nil
}
