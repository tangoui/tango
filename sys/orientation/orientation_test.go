package orientation_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/orientation"
)

func TestOrientation(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	changed := make(chan orientation.Orientation, 1)
	orientation.OnChange(a, func(o orientation.Orientation) { changed <- o })
	faketest.Start(t, a, fb)

	if o, err := orientation.Get(a); err != nil || o != orientation.Portrait || orientation.String(o) != "portrait" {
		t.Fatal(o, err)
	}
	fb.Rotate(orientation.LandscapeLeft)
	select {
	case o := <-changed:
		if !orientation.IsLandscape(o) || orientation.String(o) != "landscapeLeft" {
			t.Fatal(o)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("no change")
	}
	if err := orientation.Lock(a, orientation.Landscape); err != nil {
		t.Fatal(err)
	}
	if err := orientation.Unlock(a); err != nil {
		t.Fatal(err)
	}
	if got := fb.OrientationLocks(); !slices.Equal(got, []platform.OrientationMask{platform.MaskLandscape, 0}) {
		t.Fatal(got)
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := orientation.Get(m); !errors.Is(err, orientation.ErrUnsupported) {
		t.Fatal(err)
	}
}
