package theme_test

import (
	"slices"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/theme"
)

func TestSetAppearance(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	for _, ap := range []theme.Appearance{theme.AppearanceDark, theme.AppearanceLight, theme.AppearanceSystem} {
		if err := theme.SetAppearance(a, ap); err != nil {
			t.Fatalf("SetAppearance(%q): %v", ap, err)
		}
	}
	if err := theme.SetAppearance(a, "sepia"); err == nil {
		t.Error("unknown appearance accepted")
	}
	if got, want := fb.AppAppearanceModes(), []string{"dark", "light", ""}; !slices.Equal(got, want) {
		t.Errorf("backend got %q, want %q", got, want)
	}
}
