package biometric_test

import (
	"errors"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/biometric"
)

func TestBiometric(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	k, err := biometric.Available(a)
	if err != nil || k != biometric.FaceID || biometric.String(k) != "faceID" {
		t.Fatal(k, err)
	}
	fb.SetBiometry(biometric.None, biometric.ErrNotEnrolled)
	if k, err := biometric.Available(a); k != biometric.None || !errors.Is(err, biometric.ErrNotEnrolled) {
		t.Fatal(k, err)
	}
	if err := biometric.Authenticate(a, biometric.Options{AllowPasscode: true}); err != nil {
		t.Fatal(err)
	}
	reqs := fb.AuthRequests()
	if len(reqs) != 1 || reqs[0].Reason == "" || !reqs[0].AllowPasscode {
		t.Fatalf("%+v", reqs)
	}
	for _, want := range []error{biometric.ErrCanceled, biometric.ErrFailed, biometric.ErrLockedOut} {
		fb.SetAuthResult(want)
		if err := biometric.Authenticate(a, biometric.Options{Reason: "r"}); !errors.Is(err, want) {
			t.Fatal(err)
		}
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if err := biometric.Authenticate(m, biometric.Options{}); !errors.Is(err, biometric.ErrUnsupported) {
		t.Fatal(err)
	}
}
