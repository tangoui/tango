package media_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/media"
)

func TestPick(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetPickResult([]string{"/c/1.jpg", "/c/2.mov"}, nil)
	got, err := media.Pick(a, media.PickOptions{Types: media.Images | media.Videos, Limit: 3})
	if err != nil || !slices.Equal(got, []string{"/c/1.jpg", "/c/2.mov"}) {
		t.Fatal(got, err)
	}
	if _, err := media.Pick(a, media.PickOptions{}); err != nil {
		t.Fatal(err)
	}
	reqs := fb.PickRequests()
	want := []platform.PickMediaOptions{{Types: media.Images | media.Videos, Limit: 3}, {Types: media.Images, Limit: 1}}
	if !slices.Equal(reqs, want) {
		t.Fatalf("%+v", reqs)
	}
	fb.SetPickResult(nil, media.ErrCanceled)
	if _, err := media.Pick(a, media.PickOptions{}); !errors.Is(err, media.ErrCanceled) {
		t.Fatal(err)
	}
}

func TestPhoto(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetPhotoResult("/c/p.jpg", nil)
	if p, err := media.TakePhoto(a); p != "/c/p.jpg" || err != nil {
		t.Fatal(p, err)
	}
	fb.SetPhotoResult("", media.ErrNotAvailable)
	if _, err := media.TakePhoto(a); !errors.Is(err, media.ErrNotAvailable) {
		t.Fatal(err)
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := media.TakePhoto(m); !errors.Is(err, media.ErrUnsupported) {
		t.Fatal(err)
	}
}
