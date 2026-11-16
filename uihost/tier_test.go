package uihost_test

import (
	"errors"
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
	"image"
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

func TestAutoRendererPromotesWhenWindowGrows(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "auto")
	g := &fakeGPU{}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, _, cv := open(t, a, fb, 240, 160, func(c *ui.Context) { ui.Text(c, "small") })
	cv.Resize(800, 600, 1)
	cv.FlushFrame()
	fb.Do(func() {})
	if got := rendererOf(a, r); got != "gpu" {
		t.Fatalf("after resize: renderer=%q", got)
	}
	cv.Resize(240, 160, 1)
	cv.FlushFrame()
	fb.Do(func() {})
	if got := rendererOf(a, r); got != "gpu" {
		t.Fatalf("promotion should be sticky: renderer=%q", got)
	}
}

func TestAutoRendererPromotesDenseSmallWindow(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "auto")
	g := &fakeGPU{}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, _, _ := open(t, a, fb, 240, 160, func(c *ui.Context) {
		ui.Div(c, layout.Fill()).Paint(func(l *paint.List, r geom.Rect) {
			for i := 0; i < 300; i++ {
				l.Fill(geom.R(float32(i%20), float32(i/20), 1, 1), geom.Radii{}, theme.Hex("#112233"))
			}
		})
	})
	if got := rendererOf(a, r); got != "gpu" {
		t.Fatalf("dense: renderer=%q", got)
	}
}

func TestAutoRendererFailedPromotionStaysCPU(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "auto")
	hooks(t, nil)
	a, fb := faketest.Run(t, tango.Options{})
	// No GPU target exists. Enlarging a CPU window must still draw successfully.
	r, _, cv := open(t, a, fb, 240, 160, func(c *ui.Context) { ui.Text(c, "cpu fallback") })
	cv.Resize(800, 600, 1)
	cv.FlushFrame()
	fb.Do(func() {})
	if got := rendererOf(a, r); got != "cpu" {
		t.Fatalf("failed target: renderer=%q", got)
	}
	if w, h := cv.LastFrameSize(); w != 800 || h != 600 {
		t.Fatalf("CPU fallback frame %dx%d", w, h)
	}
}

func TestAutoRendererFailedPresenterIsNotRetried(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "auto")
	g := &fakeGPU{}
	hooks(t, g)
	attempts := 0
	t.Cleanup(uihost.SetGPUHooks(func(uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		attempts++
		return nil, nil, errors.New("no device")
	}, nil))
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, _, cv := open(t, a, fb, 240, 160, func(c *ui.Context) { ui.Text(c, "failure") })
	cv.Resize(800, 600, 1)
	cv.FlushFrame()
	fb.Do(func() {})
	for i := 0; i < 3; i++ {
		cv.Invalidate()
		cv.FlushFrame()
		fb.Do(func() {})
	}
	if attempts != 1 {
		t.Fatalf("presenter retries=%d, want one attempt", attempts)
	}
	if rendererOf(a, r) != "cpu" {
		t.Fatal("failed promotion did not keep CPU renderer")
	}
}
