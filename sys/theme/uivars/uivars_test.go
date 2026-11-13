package uivars_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	systheme "github.com/tangoui/tango/sys/theme"
	"github.com/tangoui/tango/sys/theme/uivars"
	uitheme "github.com/tangoui/tango/ui/theme"
)

const call = "window.__tangoUIVars&&window.__tangoUIVars("

// pushed returns the variables of the last push to s, nil when none.
func pushed(fb *fake.Backend, s *tango.Surface) map[string]string {
	evals := fb.Surface(s.ID()).Evals()
	for i := len(evals) - 1; i >= 0; i-- {
		if js, ok := strings.CutPrefix(evals[i], call); ok {
			var v map[string]string
			if err := json.Unmarshal([]byte(strings.TrimSuffix(js, ");")), &v); err != nil {
				return nil
			}
			return v
		}
	}
	return nil
}

func waitVars(t *testing.T, fb *fake.Backend, s *tango.Surface, want map[string]string) {
	t.Helper()
	deadline := time.Now().Add(faketest.Timeout)
	for {
		got := pushed(fb, s)
		if got != nil && reflect.DeepEqual(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("variables not pushed (last push has %d entries, want %d)", len(got), len(want))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSyncPage(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	fb.SetTheme(platform.Theme{Accent: 0x112233ff})
	var seen []systheme.Theme
	gen := func(sys systheme.Theme) *uitheme.Theme {
		seen = append(seen, sys)
		opts := []uitheme.Option{}
		if sys.Dark {
			opts = append(opts, uitheme.Dark())
		}
		if sys.Accent != 0 {
			opts = append(opts, uitheme.WithPrimary(uitheme.Color{R: uint8(sys.Accent >> 24), G: uint8(sys.Accent >> 16), B: uint8(sys.Accent >> 8), A: 0xff}))
		}
		return uitheme.ElementPlus(opts...)
	}
	if err := uivars.SyncPage(a, gen); err != nil {
		t.Fatal(err)
	}
	faketest.Start(t, a, fb)
	s, err := a.NewSurface(tango.SurfaceOptions{URL: "/"})
	if err != nil {
		t.Fatal(err)
	}

	// The bridge script carries both variants for document start.
	bs := fb.Surface(s.ID()).Options().BridgeScript
	for _, th := range []*uitheme.Theme{uitheme.ElementPlus(), uitheme.ElementPlus(uitheme.Dark())} {
		js, _ := json.Marshal(th.CSSVars())
		if !strings.Contains(bs, string(js)) {
			t.Fatal("bridge script lacks a document-start variant")
		}
	}
	if !strings.Contains(bs, `"tango-ui-vars"`) || !strings.Contains(bs, `"tangouivars"`) {
		t.Fatal("bridge script lacks the style element or the event")
	}

	// Loaded page: exactly CSSVars of the theme for the current system.
	fb.Load(s.ID())
	waitVars(t, fb, s, gen(systheme.Theme{Accent: 0x112233ff}).CSSVars())
	fb.SetTheme(platform.Theme{Dark: true, Accent: 0x445566ff})
	want := gen(systheme.Theme{Dark: true, Accent: 0x445566ff}).CSSVars()
	waitVars(t, fb, s, want)

	// A later surface gets the current variables.
	s2, err := a.NewSurface(tango.SurfaceOptions{URL: "/"})
	if err != nil {
		t.Fatal(err)
	}
	fb.Load(s2.ID())
	waitVars(t, fb, s2, want)

	// Refresh re-runs the generator, which a second SyncPage replaced.
	custom := uitheme.AntDesign(uitheme.Dark())
	custom.Name = "mine"
	if err := uivars.SyncPage(a, func(systheme.Theme) *uitheme.Theme { return custom }); err != nil {
		t.Fatal(err)
	}
	uivars.Refresh(a)
	waitVars(t, fb, s, custom.CSSVars())
	waitVars(t, fb, s2, custom.CSSVars())

	// nil removes the variables.
	_ = uivars.SyncPage(a, func(systheme.Theme) *uitheme.Theme { return nil })
	uivars.Refresh(a)
	waitVars(t, fb, s, map[string]string{})
}

func TestDefaultAndVars(t *testing.T) {
	if !reflect.DeepEqual(uivars.Vars(uivars.Default(systheme.Theme{Dark: true})), uitheme.AntDesign(uitheme.Dark()).CSSVars()) {
		t.Fatal("Default dark is not AntDesign dark")
	}
	if !reflect.DeepEqual(uivars.Vars(uivars.Default(systheme.Theme{})), uitheme.AntDesign().CSSVars()) {
		t.Fatal("Default light is not AntDesign")
	}
	if len(uivars.Vars(nil)) != 0 {
		t.Fatal("Vars(nil) not empty")
	}
}

func TestUnsupported(t *testing.T) {
	a, _ := faketest.NewMinimalApp(tango.Options{})
	if err := uivars.SyncPage(a, nil); !errors.Is(err, systheme.ErrUnsupported) {
		t.Fatal(err)
	}
	uivars.Refresh(a) // no-op
}
