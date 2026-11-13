package share_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/share"
)

func TestShare(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	f := filepath.Join(t.TempDir(), "x.txt")
	os.WriteFile(f, []byte("x"), 0o600)
	if err := share.Share(a, share.Options{Text: "hi", URL: "https://example.com", Files: []string{f}, Title: "T"}); err != nil {
		t.Fatal(err)
	}
	got := fb.Shares()
	if len(got) != 1 || got[0].Text != "hi" || got[0].URL != "https://example.com" || got[0].Paths[0] != f || got[0].Title != "T" {
		t.Fatalf("%+v", got)
	}
	fb.SetShareErr(share.ErrCanceled)
	if err := share.Share(a, share.Options{Text: "x"}); !errors.Is(err, share.ErrCanceled) {
		t.Fatal(err)
	}
	// From the main thread too.
	if err := tango.Await(a, func() error { return share.Share(a, share.Options{Text: "x"}) }); !errors.Is(err, share.ErrCanceled) {
		t.Fatal(err)
	}
	if err := share.Share(a, share.Options{}); err == nil {
		t.Fatal("empty share accepted")
	}
	if err := share.Share(a, share.Options{Files: []string{filepath.Join(t.TempDir(), "missing")}}); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestUnsupportedAndQuit(t *testing.T) {
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if err := share.Share(m, share.Options{Text: "x"}); !errors.Is(err, share.ErrUnsupported) {
		t.Fatal(err)
	}
	a, fb := faketest.NewApp(tango.Options{})
	a.Quit()
	_ = a.Run()
	<-fb.Done()
	if err := share.Share(a, share.Options{Text: "x"}); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
}
