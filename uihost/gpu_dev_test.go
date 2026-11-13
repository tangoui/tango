//go:build tango_dev

package uihost_test

import (
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/uihost"
)

// A tan dev worker's windows live in the dev host and stay on the CPU.
func TestDevOpenNoGPU(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "gpu")
	g := &fakeGPU{}
	hooks(t, g)
	addr, hfb := startHost(t)
	hfb.EnableCanvasMetal(0)
	wa, wfb := faketest.Run(t, tango.Options{})
	wfb.EnableCanvasMetal(0)
	if err := uihost.ConnectDev(addr, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(uihost.ResetDev)
	o := tango.Await(wa, func() opened {
		r, s, err := uihost.Open(wa, uihost.Options{Surface: tango.SurfaceOptions{Title: "Remote", Width: 300, Height: 200}},
			func(c *ui.Context) { ui.Text(c, "x") }, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	eventually(t, "host window", func() bool { return len(hfb.Surfaces()) == 1 })
	cv := hfb.Canvas(hfb.Surfaces()[0].ID())
	eventually(t, "first frame", func() bool { w, h := cv.LastFrameSize(); return w == 600 && h == 400 })
	if cv.MetalCalls() != 0 {
		t.Fatalf("host canvas UseMetalLayer calls %d", cv.MetalCalls())
	}
	if n, _, _ := g.stats(); n != 0 {
		t.Fatal("NewPresenter called")
	}
	if got := rendererOf(wa, o.r); got != "cpu" {
		t.Fatalf("Renderer = %q", got)
	}
}
