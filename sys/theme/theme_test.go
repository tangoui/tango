package theme_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/theme"
)

func TestSystemAndOnChange(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	if th, err := theme.System(a); err != nil || th.Dark {
		t.Fatalf("%+v %v", th, err)
	}
	got := make(chan theme.Theme, 2)
	theme.OnChange(a, func(th theme.Theme) { got <- th })
	fb.SetDisplays(nil) // not a theme event
	fb.SetTheme(platform.Theme{Dark: true, Accent: 0x0a84ffff})
	select {
	case th := <-got:
		if !th.Dark || th.Accent != 0x0a84ffff {
			t.Fatalf("%+v", th)
		}
	case <-time.After(faketest.Timeout):
		t.Fatal("no theme event")
	}
	if th, _ := theme.System(a); !th.Dark {
		t.Fatal("System not updated")
	}
}

func TestSyncPage(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	fb.SetTheme(platform.Theme{Accent: 0x112233ff})
	if err := theme.SyncPage(a); err != nil {
		t.Fatal(err)
	}
	faketest.Start(t, a, fb)
	s, err := a.NewSurface(tango.SurfaceOptions{URL: "/"})
	if err != nil {
		t.Fatal(err)
	}
	bs := fb.Surface(s.ID()).Options().BridgeScript
	if !strings.Contains(bs, "dataset.tangoTheme") {
		t.Fatalf("bridge script lacks theme sync:\n%s", bs)
	}
	waitEval := func(s *tango.Surface, want string) {
		t.Helper()
		deadline := time.Now().Add(faketest.Timeout)
		for {
			evals := fb.Surface(s.ID()).Evals()
			if strings.Contains(strings.Join(evals, "\n"), want) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s not pushed: %q", want, evals)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	// A loaded page gets the current theme, accent included.
	fb.Load(s.ID())
	waitEval(s, `__tangoTheme(false,"#112233ff")`)
	fb.SetTheme(platform.Theme{Dark: true, Accent: 0x445566ff})
	waitEval(s, `__tangoTheme(true,"#445566ff")`)
	// A surface created after the change gets the new accent, not the
	// one known at startup.
	s2, err := a.NewSurface(tango.SurfaceOptions{URL: "/"})
	if err != nil {
		t.Fatal(err)
	}
	fb.Load(s2.ID())
	waitEval(s2, `__tangoTheme(true,"#445566ff")`)
}

func TestAccentCSS(t *testing.T) {
	if theme.AccentCSS(0) != "" || theme.AccentCSS(0x0a84ffff) != "#0a84ffff" {
		t.Fatal(theme.AccentCSS(0x0a84ffff))
	}
}

func TestUnsupported(t *testing.T) {
	a, _ := faketest.NewMinimalApp(tango.Options{})
	if err := theme.SyncPage(a); !errors.Is(err, theme.ErrUnsupported) {
		t.Fatal(err)
	}
	b, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := theme.System(b); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatal(err)
	}
}
