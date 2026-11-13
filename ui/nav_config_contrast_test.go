package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/theme"
)

// TestProviderHighContrastNested: a nested HighContrast(true) themes only
// its scope, and reuses the derived theme across frames.
func TestProviderHighContrastNested(t *testing.T) {
	var before, inner probe
	var seen []*theme.Theme
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			addProbe(c, &before)
			ConfigProvider(c).HighContrast(true).Body(func() {
				addProbe(c, &inner)
				seen = append(seen, c.Theme)
			})
		})
	}, DriverOptions{Width: 200, Height: 200})
	d.Settle()
	d.Frame()
	if inner.th == nil || !inner.th.HighContrast || inner.th.Dark {
		t.Fatalf("inner theme not the light high-contrast variant: %+v", inner.th)
	}
	if before.th.HighContrast || d.Runner().Context().Theme.HighContrast {
		t.Error("nested provider raised the contrast outside its scope")
	}
	if len(seen) < 2 || seen[0] != seen[len(seen)-1] {
		t.Error("high-contrast theme rebuilt across frames")
	}
}

// TestProviderHighContrastRoot: the root provider switches the window
// theme once and keeps it.
func TestProviderHighContrastRoot(t *testing.T) {
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).HighContrast(true).Body(func() { Text(c, "x") })
	}, DriverOptions{Width: 100, Height: 100})
	d.Settle()
	win := d.Runner().Context().Theme
	if !win.HighContrast {
		t.Fatal("root provider did not raise the window contrast")
	}
	d.Frame()
	if d.Runner().Context().Theme != win {
		t.Error("window theme replaced on a later frame")
	}
}

// TestProviderInheritsHighContrast: a theme rebuilt by Dark or
// PrimaryColor inside a high-contrast window stays high-contrast;
// HighContrast(false) opts the scope out.
func TestProviderInheritsHighContrast(t *testing.T) {
	var dark, primary, off probe
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ConfigProvider(c).Dark(true).Body(func() { addProbe(c, &dark) })
			ConfigProvider(c).PrimaryColor(theme.Hex("#722ed1")).Body(func() { addProbe(c, &primary) })
			ConfigProvider(c).HighContrast(false).Body(func() { addProbe(c, &off) })
		})
	}, DriverOptions{Width: 200, Height: 200, Theme: theme.HighContrast(theme.AntDesign())})
	d.Settle()
	if !dark.th.HighContrast || !dark.th.Dark {
		t.Errorf("Dark scope: HighContrast=%v Dark=%v", dark.th.HighContrast, dark.th.Dark)
	}
	if !primary.th.HighContrast {
		t.Error("PrimaryColor scope lost high contrast")
	}
	if off.th.HighContrast || off.th.Dark {
		t.Errorf("HighContrast(false) scope: HighContrast=%v Dark=%v", off.th.HighContrast, off.th.Dark)
	}
	if !d.Runner().Context().Theme.HighContrast {
		t.Error("window theme changed")
	}
}

// TestProviderHighContrastExplicitTheme: Theme(th).HighContrast(true)
// draws the high-contrast variant of th.
func TestProviderHighContrastExplicitTheme(t *testing.T) {
	var inner probe
	el := theme.ElementPlus()
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ConfigProvider(c).Theme(el).HighContrast(true).Body(func() { addProbe(c, &inner) })
		})
	}, DriverOptions{Width: 100, Height: 100})
	d.Settle()
	if !inner.th.HighContrast || inner.th.Flavor != theme.Element {
		t.Fatalf("got HighContrast=%v Flavor=%v", inner.th.HighContrast, inner.th.Flavor)
	}
}
