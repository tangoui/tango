package safearea_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/safearea"
)

func TestGetAndChange(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	if err := safearea.SyncPage(a); err != nil {
		t.Fatal(err)
	}
	changed := make(chan safearea.Insets, 1)
	safearea.OnChange(a, func(in safearea.Insets) { changed <- in })
	faketest.Start(t, a, fb)

	s, err := a.NewSurface(tango.SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fb.Surface(s.ID()).Options().BridgeScript, "--tango-safe-") {
		t.Fatal("page script not installed")
	}
	in, err := safearea.Get(a)
	if err != nil || in != (safearea.Insets{}) {
		t.Fatal(in, err)
	}
	want := safearea.Insets{Top: 59, Bottom: 34}
	fb.SetSafeArea(want)
	select {
	case got := <-changed:
		if got != want {
			t.Fatal(got)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("no change")
	}
	evals := fb.Surface(s.ID()).Evals()
	if len(evals) == 0 || !strings.Contains(evals[len(evals)-1], "__tangoSafeArea(59,34,0,0)") {
		t.Fatal(evals)
	}
	// A page asking for the values (document start) gets them.
	fb.Message(s.ID(), "\x00tango-int:safearea:get")
	tango.Await(a, func() bool { return true })
	evals = fb.Surface(s.ID()).Evals()
	if !strings.Contains(evals[len(evals)-1], "__tangoSafeArea(59,34,0,0)") {
		t.Fatal(evals)
	}
}

func TestUnsupported(t *testing.T) {
	m, _ := faketest.NewMinimalApp(tango.Options{})
	if err := safearea.SyncPage(m); !errors.Is(err, safearea.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := safearea.Get(m); !errors.Is(err, safearea.ErrUnsupported) {
		t.Fatal(err)
	}
}
