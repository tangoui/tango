package device_test

import (
	"errors"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/device"
)

func TestDevice(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	kb := make(chan float64, 4)
	device.OnKeyboard(a, func(h float64) { kb <- h })
	faketest.Start(t, a, fb)

	if err := device.RequestReview(a); err != nil {
		t.Fatal(err)
	}
	if err := device.OpenSettings(a); err != nil {
		t.Fatal(err)
	}
	if r, s := fb.DeviceCalls(); r != 1 || s != 1 {
		t.Fatal(r, s)
	}
	if err := device.KeepScreenOn(a, true); err != nil || !fb.KeepScreenOn() {
		t.Fatal(err)
	}
	if err := device.SetBrightness(a, 1.7); err != nil {
		t.Fatal(err)
	}
	if v, err := device.Brightness(a); err != nil || v != 1 {
		t.Fatal(v, err)
	}
	if err := device.ResetBrightness(a); err != nil {
		t.Fatal(err)
	}
	sb := device.StatusBar{Style: device.StatusBarLight, Color: "#102030"}
	if err := device.SetStatusBar(a, sb); err != nil {
		t.Fatal(err)
	}
	if got := fb.StatusBars(); len(got) != 1 || got[0] != sb {
		t.Fatal(got)
	}

	fb.ShowKeyboard(291)
	if h := recv(t, kb); h != 291 {
		t.Fatal(h)
	}
	if h, err := device.KeyboardHeight(a); err != nil || h != 291 {
		t.Fatal(h, err)
	}
	if err := device.HideKeyboard(a); err != nil {
		t.Fatal(err)
	}
	if h := recv(t, kb); h != 0 {
		t.Fatal(h)
	}
}

func TestUnsupported(t *testing.T) {
	m, _ := faketest.RunMinimal(t, tango.Options{})
	for _, err := range []error{device.RequestReview(m), device.OpenSettings(m), device.KeepScreenOn(m, true),
		device.SetStatusBar(m, device.StatusBar{}), device.HideKeyboard(m)} {
		if !errors.Is(err, device.ErrUnsupported) {
			t.Fatal(err)
		}
	}
	if _, err := device.Brightness(m); !errors.Is(err, device.ErrUnsupported) {
		t.Fatal(err)
	}
}

func recv[T any](t *testing.T, c chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(faketest.Timeout):
		t.Fatal("event missing")
	}
	panic("unreachable")
}
