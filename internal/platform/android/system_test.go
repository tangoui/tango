package android

import (
	"slices"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestDecodeTheme(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want platform.Theme
	}{
		{`{"dark":true,"accent":"#1A73E8"}`, platform.Theme{Dark: true, Accent: 0x1a73e8ff}},
		{`{"dark":false,"accent":""}`, platform.Theme{}},
		{`{"dark":false,"accent":"#12"}`, platform.Theme{}},
		{`{"dark":true}`, platform.Theme{Dark: true}},
		{`!unsupported`, platform.Theme{}},
	} {
		if got := decodeTheme(tc.in); got != tc.want {
			t.Errorf("decodeTheme(%s) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestDecodeLocales(t *testing.T) {
	if got := decodeLocales(`["zh-Hans-CN","en-US","zh-Hans-CN","","und"]`); !slices.Equal(got, []string{"zh-Hans-CN", "en-US"}) {
		t.Errorf("decodeLocales = %q", got)
	}
	if got := decodeLocales(`[]`); got != nil {
		t.Errorf("decodeLocales([]) = %q, want nil", got)
	}
	if got := decodeLocales(`oops`); got != nil {
		t.Errorf("decodeLocales(oops) = %q, want nil", got)
	}
}
