package menu_test

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/i18n"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/menu"
)

// hasLabel reports whether items (recursively) contain label.
func hasLabel(items []platform.MenuItem, label string) bool {
	for _, it := range items {
		if it.Label == label || hasLabel(it.Submenu, label) {
			return true
		}
	}
	return false
}

// A change of the system language, reported by the backend's
// LocaleWatcher, re-applies attached menus so that role labels follow.
func TestMenusFollowSystemLanguage(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetSystemLocales("en-US")
	i18n.Init(a)
	t.Cleanup(func() { i18n.SetLocale("") })
	if got := i18n.Locale(); got != "en" {
		t.Fatalf("Locale() = %q, want en", got)
	}

	m := menu.New()
	edit := m.Submenu("Edit")
	edit.Add("", nil).Role = menu.Undo
	it := edit.Add("Probe", nil)
	if err := menu.SetAppMenu(a, m); err != nil {
		t.Fatal(err)
	}
	// Changed without Update: only a re-apply makes it visible, which is
	// what tells the test the locale change rebuilt the menu.
	// Menus are edited on the main thread, where a language change reads them.
	tango.Await(a, func() bool { it.Label = "Probe 2"; return true })
	if hasLabel(fb.Menu(0), "Probe 2") {
		t.Fatal("menu re-applied too early")
	}

	fb.SetSystemLocales("ja-JP") // the user switches the system to Japanese
	if got := i18n.Locale(); got != "ja" {
		t.Fatalf("Locale() after the change = %q, want ja", got)
	}
	waitFor(t, "menu re-applied after the language change", func() bool {
		return hasLabel(fb.Menu(0), "Probe 2")
	})
	var role platform.MenuItem
	for _, sub := range fb.Menu(0) {
		for _, c := range sub.Submenu {
			if c.Role != "" {
				role = c
			}
		}
	}
	if role.Role == "" {
		t.Fatalf("role item lost: %+v", fb.Menu(0))
	}

	// i18n.SetLocale re-applies as well.
	// Menus are edited on the main thread, where a language change reads them.
	tango.Await(a, func() bool { it.Label = "Probe 3"; return true })
	i18n.SetLocale("zh-CN")
	waitFor(t, "menu re-applied after SetLocale", func() bool {
		return hasLabel(fb.Menu(0), "Probe 3")
	})
}
