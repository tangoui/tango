package appctl_test

import (
	"errors"
	"math"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/appctl"
)

func TestControls(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(appctl.Hide(a))
	if !fb.AppControl().Hidden {
		t.Fatal("not hidden")
	}
	must(appctl.Show(a))
	must(appctl.SetBadge(a, "3"))
	must(appctl.SetProgress(a, 1.5))
	must(appctl.RequestAttention(a, true))
	must(appctl.SetDockVisible(a, false))
	st := fb.AppControl()
	if st.Hidden || st.Badge != "3" || st.Progress != 1 || st.Attention != 1 || !st.Critical || !st.DockHidden {
		t.Fatalf("%+v", st)
	}
	must(appctl.SetProgress(a, math.NaN()))
	must(appctl.SetBadge(a, "new")) // the fake supports text badges
	if st := fb.AppControl(); st.Progress != -1 || st.Badge != "new" {
		t.Fatalf("%+v", st)
	}
}

func TestPartialSupport(t *testing.T) {
	a, fb := faketest.NewApp(tango.Options{})
	fb.SetAppControlUnsupported(appctl.CtlDock | appctl.CtlBadgeText)
	faketest.Start(t, a, fb)
	if appctl.Supports(a, appctl.CtlDock) || !appctl.Supports(a, appctl.CtlBadge|appctl.CtlProgress) {
		t.Fatal("Supports")
	}
	if err := appctl.SetDockVisible(a, false); !errors.Is(err, appctl.ErrUnsupported) {
		t.Fatal(err)
	}
	if err := appctl.SetBadge(a, "12+"); err != nil || fb.AppControl().Badge != "12+" {
		t.Fatal(err, fb.AppControl().Badge)
	}
	if err := appctl.SetBadge(a, "hi"); !errors.Is(err, appctl.ErrUnsupported) || fb.AppControl().Badge != "" {
		t.Fatal(err, fb.AppControl().Badge)
	}
}

func TestUnsupportedAndQuit(t *testing.T) {
	a, _ := faketest.RunMinimal(t, tango.Options{})
	if err := appctl.SetBadge(a, "1"); !errors.Is(err, appctl.ErrUnsupported) {
		t.Fatal(err)
	}
	b, fb := faketest.NewApp(tango.Options{})
	b.Quit()
	_ = b.Run()
	<-fb.Done()
	if err := appctl.Hide(b); !errors.Is(err, tango.ErrNotRunning) {
		t.Fatal(err)
	}
}

// A window created after SetBadge / SetProgress gets them re-applied.
func TestReappliedOnNewSurface(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	if err := appctl.SetBadge(a, "7"); err != nil {
		t.Fatal(err)
	}
	if err := appctl.SetProgress(a, 0.5); err != nil {
		t.Fatal(err)
	}
	// Simulate the platform losing them (a new taskbar button starts bare).
	tango.Await(a, func() bool { fb.SetBadge(""); fb.SetProgress(-1); return true })
	if _, err := a.NewSurface(tango.SurfaceOptions{}); err != nil {
		t.Fatal(err)
	}
	if st := fb.AppControl(); st.Badge != "7" || st.Progress != 0.5 {
		t.Fatalf("%+v", st)
	}
}
