package haptics_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/haptics"
)

func TestPlay(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	all := []haptics.Kind{haptics.Light, haptics.Medium, haptics.Heavy, haptics.Selection, haptics.Success, haptics.Warning, haptics.Error}
	for _, k := range all {
		if err := haptics.Play(a, k); err != nil {
			t.Fatal(err)
		}
	}
	if got := fb.Haptics(); !slices.Equal(got, all) {
		t.Fatal(got)
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if err := haptics.Play(m, haptics.Light); !errors.Is(err, haptics.ErrUnsupported) {
		t.Fatal(err)
	}
}
