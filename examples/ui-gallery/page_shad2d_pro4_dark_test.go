package main

import (
	"testing"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

// TestSettingDrawerPreviewFollowsDarkTheme: in a dark gallery the embedded
// Pro layout starts in dark mode (realDark) instead of forcing the light
// algorithm over the dark page; a light gallery keeps the light menu.
func TestSettingDrawerPreviewFollowsDarkTheme(t *testing.T) {
	page, ok := gallery.Find("SettingDrawer")
	if !ok {
		t.Fatal("SettingDrawer page missing")
	}
	for _, tc := range []struct {
		name string
		th   *theme.Theme
		want string
	}{
		{"dark", theme.AntDesign(theme.Dark()), "realDark"},
		{"light", theme.AntDesign(), "light"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shad2dReset()
			d := ui.NewDriver(page.Render, ui.DriverOptions{Width: 1200, Height: 800, Theme: tc.th})
			defer d.Runner().Close()
			d.Settle()
			if got := shad2dDemo.settings.NavTheme; got != tc.want {
				t.Fatalf("NavTheme = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSettingDrawerPreviewFollowsThemeToggle: toggling the gallery's dark
// mode while the page is open moves the preview along, but a style the user
// picked in the drawer survives the toggle.
func TestSettingDrawerPreviewFollowsThemeToggle(t *testing.T) {
	page, ok := gallery.Find("SettingDrawer")
	if !ok {
		t.Fatal("SettingDrawer page missing")
	}
	shad2dReset()
	d := ui.NewDriver(page.Render, ui.DriverOptions{Width: 1200, Height: 800, Theme: theme.AntDesign()})
	defer d.Runner().Close()
	d.Settle()
	step := func(th *theme.Theme, want string) {
		t.Helper()
		d.SetTheme(th)
		d.Settle()
		if got := shad2dDemo.settings.NavTheme; got != want {
			t.Fatalf("NavTheme = %q, want %q", got, want)
		}
	}
	step(theme.AntDesign(theme.Dark()), "realDark")
	step(theme.AntDesign(), "light")
	shad2dDemo.settings.NavTheme = "dark" // the user picks the dark menu
	step(theme.AntDesign(theme.Dark()), "dark")
	step(theme.AntDesign(), "dark")
}
