package uihost_test

import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"testing"
)

func TestAutoRendererUsesCPUForSmallWindows(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "auto")
	g := &fakeGPU{}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, _, _ := open(t, a, fb, 240, 160, func(c *ui.Context) { ui.Text(c, "small") })
	if got := rendererOf(a, r); got != "cpu" {
		t.Fatalf("renderer=%q, want cpu", got)
	}
	if layers, _, _ := g.stats(); layers != 0 {
		t.Fatalf("GPU layers=%d, want 0", layers)
	}
}

func TestAutoRendererUsesGPUForLargeWindows(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "auto")
	g := &fakeGPU{}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, _, _ := open(t, a, fb, 800, 600, func(c *ui.Context) { ui.Text(c, "large") })
	if got := rendererOf(a, r); got != "gpu" {
		t.Fatalf("renderer=%q, want gpu", got)
	}
	if layers, _, _ := g.stats(); layers != 1 {
		t.Fatalf("GPU layers=%d, want 1", layers)
	}
}
