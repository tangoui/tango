package location_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/location"
)

func TestCurrent(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetLocation(location.Location{Latitude: 31.2, Longitude: 121.5, Accuracy: 10}, nil)
	l, err := location.Current(a, location.Options{Accuracy: location.High})
	if err != nil || l.Latitude != 31.2 || l.Longitude != 121.5 {
		t.Fatal(l, err)
	}
	fb.SetLocation(location.Location{}, location.ErrPermissionDenied)
	if _, err := location.Current(a, location.Options{}); !errors.Is(err, location.ErrPermissionDenied) {
		t.Fatal(err)
	}
}

func wait(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(faketest.Timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestWatch(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	got1 := make(chan location.Location, 4)
	got2 := make(chan error, 4)
	stop1, err := location.Watch(a, location.Options{Accuracy: location.Low, DistanceFilter: 50}, func(l location.Location, err error) { got1 <- l })
	if err != nil {
		t.Fatal(err)
	}
	if o, on := fb.LocationUpdates(); !on || o.Accuracy != location.Low || o.DistanceFilter != 50 {
		t.Fatal(o, on)
	}
	stop2, _ := location.Watch(a, location.Options{Accuracy: location.High, DistanceFilter: 5}, func(l location.Location, err error) { got2 <- err })
	if o, _ := fb.LocationUpdates(); o.Accuracy != location.High || o.DistanceFilter != 5 {
		t.Fatal("merged options", o)
	}
	fb.MoveTo(location.Location{Latitude: 1}, nil)
	if l := <-got1; l.Latitude != 1 {
		t.Fatal(l)
	}
	if err := <-got2; err != nil {
		t.Fatal(err)
	}
	stop2()
	stop2() // idempotent
	wait(t, func() bool { o, _ := fb.LocationUpdates(); return o.Accuracy == location.Low })
	stop1()
	wait(t, func() bool { _, on := fb.LocationUpdates(); return !on })

	// An error ends every watch.
	errc := make(chan error, 1)
	location.Watch(a, location.Options{}, func(_ location.Location, err error) { errc <- err })
	fb.MoveTo(location.Location{}, location.ErrNotAvailable)
	if err := <-errc; !errors.Is(err, location.ErrNotAvailable) {
		t.Fatal(err)
	}
	if fb.MoveTo(location.Location{}, nil) {
		t.Fatal("session still running")
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := location.Watch(m, location.Options{}, func(location.Location, error) {}); !errors.Is(err, location.ErrUnsupported) {
		t.Fatal(err)
	}
}
