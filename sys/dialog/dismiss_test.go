package dialog_test

import (
	"errors"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/dialog"
)

func TestMessageDismissal(t *testing.T) {
	cases := []struct {
		name    string
		buttons []string
		n       int
		err     error
		wantN   int
		wantErr error
	}{
		{"single canceled", nil, -1, platform.ErrCanceled, 0, nil},
		{"single one button canceled", []string{"Close"}, -1, platform.ErrCanceled, 0, nil},
		{"single negative", nil, -1, nil, 0, nil},
		{"multi canceled", []string{"Yes", "No"}, -1, platform.ErrCanceled, -1, dialog.ErrCanceled},
		{"multi out of range", []string{"Yes", "No"}, 7, nil, -1, dialog.ErrCanceled},
		{"multi chosen", []string{"Yes", "No"}, 1, nil, 1, nil},
		{"backend error", []string{"Yes", "No"}, 0, errors.New("boom"), 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, fb := faketest.NewApp(tango.Options{})
			fb.OnMessageDialog = func(platform.SurfaceID, platform.MessageOptions) (int, error) { return c.n, c.err }
			faketest.Start(t, a, fb)
			n, err := dialog.Message(a, dialog.MessageOptions{Buttons: c.buttons})
			if c.name == "backend error" {
				if err != c.err {
					t.Fatalf("err %v", err)
				}
				return
			}
			if n != c.wantN || !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("got (%d, %v), want (%d, %v)", n, err, c.wantN, c.wantErr)
			}
		})
	}
}

func TestFileOptionsPassThrough(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	var got platform.FileOptions
	fb.OnOpenDialog = func(_ platform.SurfaceID, o platform.FileOptions) ([]string, error) {
		got = o
		return []string{"/x"}, nil
	}
	faketest.Start(t, a, fb)
	if _, err := dialog.Open(a, dialog.FileOptions{CanCreateDirs: true, ShowHidden: true, PickFolders: true}); err != nil {
		t.Fatal(err)
	}
	if !got.CanCreateDirs || !got.ShowHidden || !got.PickFolders {
		t.Fatalf("%+v", got)
	}
}
