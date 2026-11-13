package dialog_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/dialog"
)

func TestMessage(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	var gotParent platform.SurfaceID
	var gotOpts platform.MessageOptions
	fb.OnMessageDialog = func(p platform.SurfaceID, o platform.MessageOptions) (int, error) {
		gotParent, gotOpts = p, o
		return 1, nil
	}
	faketest.Start(t, a, fb)
	s, err := a.NewSurface(tango.SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	n, err := dialog.Message(a, dialog.MessageOptions{Parent: s, Kind: dialog.Question, Title: "T", Message: "M", Buttons: []string{"No", "Yes"}})
	if n != 1 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
	if gotParent != s.ID() || gotOpts.Kind != platform.MessageQuestion || gotOpts.Title != "T" || !slices.Equal(gotOpts.Buttons, []string{"No", "Yes"}) {
		t.Fatalf("%v %+v", gotParent, gotOpts)
	}
}

func TestOpenSaveFromMainThread(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	var opened platform.FileOptions
	fb.OnOpenDialog = func(p platform.SurfaceID, o platform.FileOptions) ([]string, error) {
		opened = o
		return []string{"/a", "/b"}, nil
	}
	fb.OnSaveDialog = func(p platform.SurfaceID, o platform.FileOptions) (string, error) {
		return o.Directory + "/" + o.Filename, nil
	}
	faketest.Start(t, a, fb)
	type res struct {
		paths []string
		path  string
		err1  error
		err2  error
	}
	// A dialog from a main-thread callback must not deadlock.
	r := tango.Await(a, func() res {
		p, e1 := dialog.Open(a, dialog.FileOptions{Multiple: true, Filters: []dialog.FileFilter{{Name: "Img", Patterns: []string{"*.png"}}}})
		s, e2 := dialog.Save(a, dialog.FileOptions{Directory: "/tmp", Filename: "x.txt"})
		return res{p, s, e1, e2}
	})
	if r.err1 != nil || !slices.Equal(r.paths, []string{"/a", "/b"}) || !opened.Multiple || opened.Filters[0].Name != "Img" {
		t.Fatalf("open %v %v %+v", r.paths, r.err1, opened)
	}
	if r.err2 != nil || r.path != "/tmp/x.txt" {
		t.Fatalf("save %q %v", r.path, r.err2)
	}
}

func TestCanceled(t *testing.T) {
	a, _ := faketest.Run(t, tango.Options{})
	if _, err := dialog.Open(a, dialog.FileOptions{}); !errors.Is(err, dialog.ErrCanceled) {
		t.Fatal(err)
	}
	if _, err := dialog.Save(a, dialog.FileOptions{}); !errors.Is(err, platform.ErrCanceled) {
		t.Fatal(err)
	}
}

func TestUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := dialog.Message(a, dialog.MessageOptions{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := dialog.Open(a, dialog.FileOptions{}); !errors.Is(err, dialog.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := dialog.Save(a, dialog.FileOptions{}); !errors.Is(err, dialog.ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestAfterQuit(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	a.Quit()
	if err := a.Run(); err != nil {
		t.Fatal(err)
	}
	<-fb.Done()
	if _, err := dialog.Message(a, dialog.MessageOptions{}); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
}
