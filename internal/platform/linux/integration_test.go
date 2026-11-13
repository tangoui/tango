package linux

import "testing"

func TestShowItemsArgs(t *testing.T) {
	if got := showItemsArgs("/home/a b/it's.txt"); got != `(['file:///home/a%20b/it%27s.txt'], '')` && got != `(['file:///home/a%20b/it\'s.txt'], '')` {
		t.Fatal(got)
	}
	if got := portalReadArgs("org.freedesktop.appearance", "color-scheme"); got != "('org.freedesktop.appearance', 'color-scheme')" {
		t.Fatal(got)
	}
}

func TestThemeIsDark(t *testing.T) {
	for _, c := range []struct {
		scheme     uint32
		have, pref bool
		name       string
		want       bool
	}{
		{1, true, false, "Adwaita", true},
		{2, true, true, "Adwaita-dark", false},
		{0, true, false, "Adwaita-dark", true},
		{0, false, true, "Adwaita", true},
		{0, false, false, "Yaru:dark", true},
		{0, false, false, "Breeze", false},
	} {
		if got := themeIsDark(c.scheme, c.have, c.pref, c.name); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestPortalAccent(t *testing.T) {
	if got := portalAccent(0.2078, 0.5176, 0.8941); got != 0x3584e4ff {
		t.Fatalf("%#x", got)
	}
	if portalAccent(-1, 0, 0) != 0 || portalAccent(2, 0, 0) != 0 {
		t.Fatal("unset accent")
	}
}

func TestLauncher(t *testing.T) {
	if n, ok := launcherCount("12+"); !ok || n != 12 {
		t.Fatal(n, ok)
	}
	if _, ok := launcherCount("new"); ok {
		t.Fatal("text accepted")
	}
	got := launcherArgs("application://com.example.app.desktop", launcherState{count: 3, countOn: true, progress: 0.5, progOn: true})
	want := "('application://com.example.app.desktop', {'count': <int64 3>, 'count-visible': <true>, 'progress': <double 0.5>, 'progress-visible': <true>, 'urgent': <false>})"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if p := launcherPath("application://x.desktop"); p[:35] != "/com/canonical/unity/launcherentry/" {
		t.Fatal(p)
	}
}

func TestContrastIsHigh(t *testing.T) {
	for _, c := range []struct {
		contrast uint32
		have     bool
		name     string
		want     bool
	}{
		{0, false, "Adwaita", false},
		{1, true, "Adwaita", true},
		{0, true, "Adwaita", false},
		{0, true, "HighContrast", true}, // portal without a preference, legacy theme
		{0, false, "HighContrastInverse", true},
		{0, false, "Adwaita-high-contrast", true},
		{2, true, "Yaru", false}, // unknown future value
	} {
		if got := contrastIsHigh(c.contrast, c.have, c.name); got != c.want {
			t.Errorf("contrastIsHigh(%d, %v, %q) = %v, want %v", c.contrast, c.have, c.name, got, c.want)
		}
	}
}
