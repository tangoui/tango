//go:build tango_dev

package uihost_test

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/uihost"
)

// Under tan dev the theme is generated and followed in the worker, which
// is an ordinary app with the platform backend; the host only shows
// frames.
func TestRemoteWindowFollowsTheme(t *testing.T) {
	resetLocale(t)
	addr, hfb := startHost(t)
	wa, wfb := faketest.Run(t, tango.Options{})
	if err := uihost.ConnectDev(addr, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(uihost.ResetDev)
	var s seen
	o := tango.Await(wa, func() opened {
		r, sf, err := uihost.Open(wa, uihost.Options{Surface: tango.SurfaceOptions{Title: "Remote", Width: 200, Height: 100}}, s.page, ui.RunnerOptions{})
		return opened{r, sf, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	eventually(t, "host window", func() bool { return len(hfb.Surfaces()) == 1 })
	cv := hfb.Canvas(hfb.Surfaces()[0].ID())
	builtDark := func() bool {
		cv.FlushFrame()
		d := false
		wfb.Do(func() { d = s.th != nil && s.th.Dark })
		return d
	}
	eventually(t, "first frame", func() bool { w, _ := cv.LastFrameSize(); return w > 0 })
	if builtDark() {
		t.Fatal("dark before the switch")
	}
	wfb.SetTheme(platform.Theme{Dark: true})
	eventually(t, "dark frame in the worker", builtDark)
	if n, _ := uihost.FollowStats(wa); n != 1 {
		t.Fatalf("stats %d", n)
	}
}
