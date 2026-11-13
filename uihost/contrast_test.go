package uihost_test

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

func TestContrastFollowsSystem(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetHighContrast(true) // already on when the window opens
	var s seen
	r, _, cv := openWith(t, a, fb, uihost.Options{}, ui.RunnerOptions{}, s.page)
	v := look(fb, cv, &s)
	want := theme.HighContrast(theme.AntDesign())
	if !v.th.HighContrast || v.th.Dark || v.th.TextTertiary != want.TextTertiary {
		t.Fatalf("opened with high contrast on: HighContrast=%v Dark=%v", v.th.HighContrast, v.th.Dark)
	}

	// Light/dark combines with it.
	fb.SetTheme(platform.Theme{Dark: true})
	v = look(fb, cv, &s)
	wantDark := theme.HighContrast(theme.AntDesign(theme.Dark()))
	if !v.th.HighContrast || !v.th.Dark || v.th.TextTertiary != wantDark.TextTertiary {
		t.Fatal("dark switch lost the high-contrast variant")
	}

	// Off: the standard dark theme.
	if !fb.SetHighContrast(false) {
		t.Fatal("no AppContrastChanged")
	}
	v = look(fb, cv, &s)
	if v.th.HighContrast || !v.th.Dark || v.th.TextTertiary != theme.AntDesign(theme.Dark()).TextTertiary {
		t.Fatal("high contrast off did not restore the standard theme")
	}

	// On again while dark.
	fb.SetHighContrast(true)
	if v = look(fb, cv, &s); !v.th.HighContrast || !v.th.Dark {
		t.Fatal("high contrast on did not apply")
	}
	if !following(a, r) {
		t.Fatal("not following the system theme any more")
	}
}

func TestContrastFixedModes(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})

	var std seen
	_, _, cvStd := openWith(t, a, fb, uihost.Options{Contrast: uihost.ContrastStandard}, ui.RunnerOptions{}, std.page)
	var hi seen
	_, _, cvHi := openWith(t, a, fb, uihost.Options{Contrast: uihost.ContrastHigh}, ui.RunnerOptions{}, hi.page)
	if v := look(fb, cvHi, &hi); !v.th.HighContrast {
		t.Fatal("ContrastHigh drew the standard theme")
	}
	fb.SetHighContrast(true)
	if v := look(fb, cvStd, &std); v.th.HighContrast {
		t.Fatal("ContrastStandard followed the system")
	}
	fb.SetHighContrast(false)
	if v := look(fb, cvHi, &hi); !v.th.HighContrast {
		t.Fatal("ContrastHigh followed the system")
	}
	// ContrastHigh still follows light/dark.
	fb.SetTheme(platform.Theme{Dark: true})
	if v := look(fb, cvHi, &hi); !v.th.HighContrast || !v.th.Dark {
		t.Fatal("ContrastHigh lost light/dark following")
	}
}

func TestContrastLeavesExplicitThemes(t *testing.T) {
	resetLocale(t)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetHighContrast(true)

	// An explicit RunnerOptions.Theme / ThemeFixed is never replaced.
	for _, opts := range []uihost.Options{{Theme: uihost.ThemeFixed}, {}} {
		fixed := theme.ElementPlus()
		var s seen
		_, _, cv := openWith(t, a, fb, opts, ui.RunnerOptions{Theme: fixed}, s.page)
		if v := look(fb, cv, &s); v.th != fixed {
			t.Fatalf("%+v: explicit theme replaced at open", opts)
		}
		fb.SetHighContrast(false)
		fb.SetHighContrast(true)
		if v := look(fb, cv, &s); v.th != fixed {
			t.Fatalf("%+v: explicit theme replaced on a contrast change", opts)
		}
	}

	// A theme the app set later stops the following, contrast included.
	var s seen
	r, _, cv := openWith(t, a, fb, uihost.Options{}, ui.RunnerOptions{}, s.page)
	mine := theme.ElementPlus()
	fb.Do(func() { r.SetTheme(mine) })
	fb.SetHighContrast(false)
	if v := look(fb, cv, &s); v.th != mine {
		t.Fatal("manual theme overwritten by a contrast change")
	}
	if following(a, r) {
		t.Fatal("still following after SetTheme")
	}
}

func TestThemeForContrast(t *testing.T) {
	o := uihost.Options{}
	if o.ThemeFor(platform.Theme{}).HighContrast {
		t.Fatal("ThemeFor applied high contrast under ContrastSystem")
	}
	if !o.ThemeForContrast(platform.Theme{Dark: true}, true).HighContrast {
		t.Fatal("ThemeForContrast(true) not high contrast")
	}
	if (uihost.Options{Contrast: uihost.ContrastStandard}).ThemeForContrast(platform.Theme{}, true).HighContrast {
		t.Fatal("ContrastStandard raised contrast")
	}
	if !(uihost.Options{Contrast: uihost.ContrastHigh}).ThemeFor(platform.Theme{}).HighContrast {
		t.Fatal("ContrastHigh ThemeFor not high contrast")
	}
	if (uihost.Options{Theme: uihost.ThemeFixed}).ThemeForContrast(platform.Theme{}, true) != nil {
		t.Fatal("ThemeFixed returned a theme")
	}
}
