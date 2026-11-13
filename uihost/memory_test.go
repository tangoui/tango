package uihost_test

import (
	"image"
	"sync"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/uihost"
)

// trimGPU is fakeGPU with gpu.Renderer's Trim.
type trimGPU struct {
	*fakeGPU
	mu    sync.Mutex
	trims int
}

func (g *trimGPU) Trim() {
	g.mu.Lock()
	g.trims++
	g.mu.Unlock()
}

func (g *trimGPU) trimCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.trims
}

// A memory warning trims the window's GPU renderer and asks every window
// (GPU or CPU) for a frame, which draws as before.
func TestMemoryWarningTrims(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &trimGPU{fakeGPU: &fakeGPU{}}
	lr := &logRec{}
	t.Cleanup(setTrimHooks(g, lr))
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, s1, cv := open(t, a, fb, 200, 100, func(c *ui.Context) { ui.Text(c, "memory") })
	if rendererOf(a, r) != "gpu" {
		t.Fatalf("renderer %q, logs %v", rendererOf(a, r), lr.get())
	}
	// A second window on the CPU path.
	_, _, cv2 := openWith(t, a, fb, uihost.Options{}, ui.RunnerOptions{Renderer: paint.NewCPU()},
		func(c *ui.Context) { ui.Text(c, "cpu") })
	for cv.FlushFrame() {
	}
	for cv2.FlushFrame() {
	}
	_, renders, _ := g.stats()

	if tango.EventMemoryWarning != platform.AppMemoryWarning {
		t.Fatal("EventMemoryWarning")
	}
	fb.AppEvent(platform.AppEvent{Kind: platform.AppMemoryWarning})
	fb.Do(func() {})
	if n := g.trimCount(); n != 1 {
		t.Fatalf("Trim called %d times, want 1", n)
	}
	if !cv.FlushFrame() {
		t.Fatal("no frame after the memory warning")
	}
	if _, r2, _ := g.stats(); r2 <= renders {
		t.Fatal("GPU window did not render after the trim")
	}
	if !cv2.FlushFrame() {
		t.Fatal("CPU window got no frame after the memory warning")
	}
	if rendererOf(a, r) != "gpu" {
		t.Fatal("trim switched the window off the GPU")
	}

	// Closed windows are not trimmed.
	tango.Await(a, func() struct{} { s1.Close(); return struct{}{} })
	fb.Do(func() {})
	fb.AppEvent(platform.AppEvent{Kind: platform.AppMemoryWarning})
	fb.Do(func() {})
	if n := g.trimCount(); n != 1 {
		t.Fatalf("closed window trimmed: %d", n)
	}
}

// setTrimHooks makes the Metal presenter hook return g.
func setTrimHooks(g *trimGPU, lr *logRec) func() {
	return uihost.SetGPUHooks(func(layer uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		_, present, err := g.fakeGPU.newPresenter(layer)
		return g, present, err
	}, lr.logf)
}
