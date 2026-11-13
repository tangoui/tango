package clipboard_test

import (
	"errors"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/clipboard"
)

func TestReadWrite(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	if err := clipboard.WriteText(a, "héllo"); err != nil {
		t.Fatal(err)
	}
	if fb.ClipboardText() != "héllo" {
		t.Fatalf("backend has %q", fb.ClipboardText())
	}
	s, err := clipboard.ReadText(a)
	if err != nil || s != "héllo" {
		t.Fatalf("%q %v", s, err)
	}
	// From the main thread too (Await runs directly).
	got := tango.Await(a, func() string { s, _ := clipboard.ReadText(a); return s })
	if got != "héllo" {
		t.Fatalf("on main: %q", got)
	}
}

func TestBackendError(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	fb.ClipboardErr = errors.New("locked")
	faketest.Start(t, a, fb)
	if _, err := clipboard.ReadText(a); err != fb.ClipboardErr {
		t.Fatal(err)
	}
	if err := clipboard.WriteText(a, "x"); err != fb.ClipboardErr {
		t.Fatal(err)
	}
}

func TestUnsupported(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := clipboard.ReadText(a); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := clipboard.WriteText(a, "x"); !errors.Is(err, clipboard.ErrUnsupported) {
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
	if _, err := clipboard.ReadText(a); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
	if err := clipboard.WriteText(a, "x"); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
}
