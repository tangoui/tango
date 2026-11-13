package uihost_test

// The Windows GPU glue (platform.CanvasHWND → d3d11.NewPresenter's shape)
// on the fake backend, on every OS: SetGPUHooksHWND selects the HWND
// target whatever GOOS the test runs on.

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/d3d11"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/uihost"
)

func hooksHWND(t *testing.T, g *fakeGPU) *logRec {
	t.Helper()
	lr := &logRec{}
	var np func(uintptr) (paint.Renderer, func(image.Point, float32) error, error)
	if g != nil {
		np = g.newPresenter
	}
	t.Cleanup(uihost.SetGPUHooksHWND(np, lr.logf))
	return lr
}

func TestHWNDGPUPath(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	lr := hooksHWND(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(1.5)
	fb.EnableCanvasHWND(0)
	fb.EnableCanvasMetal(0) // must not be used with the HWND target
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })

	if cv.GPUWindowCalls() != 1 || cv.GPUWindow() != fake.FakeHWND || cv.MetalCalls() != 0 {
		t.Fatalf("UseGPUWindow calls %d hwnd %#x, metal calls %d", cv.GPUWindowCalls(), cv.GPUWindow(), cv.MetalCalls())
	}
	layers, renders, presents := g.stats()
	if layers != 1 || g.layers[0] != fake.FakeHWND {
		t.Fatalf("NewPresenter calls %v", g.layers)
	}
	if renders < 1 || len(presents) != renders || presents[0] != image.Pt(450, 300) {
		t.Fatalf("renders %d presents %v", renders, presents)
	}
	if cv.Presents() < 1 || cv.LastFrame() != nil {
		t.Fatalf("canvas presents %d, CPU image %v", cv.Presents(), cv.LastFrame() != nil)
	}
	if got := rendererOf(a, r); got != "gpu" {
		t.Fatalf("Renderer = %q", got)
	}
	if l := lr.get(); len(l) != 0 {
		t.Fatalf("logs %q", l)
	}
}

// Without CanvasHWND (layered, attached, transparent canvases answer
// ErrUnsupported) the window stays on the plain CPU path.
func TestHWNDUnsupportedStaysCPU(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	lr := hooksHWND(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	if cv.GPUWindowCalls() != 1 || cv.MetalCalls() != 0 {
		t.Fatalf("UseGPUWindow calls %d, metal calls %d", cv.GPUWindowCalls(), cv.MetalCalls())
	}
	if n, _, _ := g.stats(); n != 0 {
		t.Fatal("NewPresenter called")
	}
	if w, h := cv.LastFrameSize(); w != 300 || h != 200 || rendererOf(a, r) != "cpu" {
		t.Fatalf("CPU frame %dx%d renderer %q", w, h, rendererOf(a, r))
	}
	if l := lr.get(); len(l) != 1 || !strings.Contains(l[0], "GPU unavailable") {
		t.Fatalf("logs %q", l)
	}
}

// NewPresenter failing (no device, d3dcompiler_47.dll missing) leaves the
// window in CPU mode: frames go to Present(img), which the switched canvas
// shows in its sibling window.
func TestHWNDPresenterFailsStaysCPU(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	lr := &logRec{}
	calls := 0
	t.Cleanup(uihost.SetGPUHooksHWND(func(uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		calls++
		return nil, nil, fmt.Errorf("d3d11: NewPresenter: d3dcompiler_47.dll: %w", d3d11.ErrNoDevice)
	}, lr.logf))
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	fb.EnableCanvasHWND(0)
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	if calls != 1 || cv.GPUWindow() != fake.FakeHWND {
		t.Fatalf("NewPresenter calls %d, hwnd %#x", calls, cv.GPUWindow())
	}
	for i := 0; i < 2; i++ {
		if i > 0 {
			r.Invalidate()
			cv.FlushFrame()
		}
		if w, h := cv.LastFrameSize(); w != 600 || h != 400 {
			t.Fatalf("frame %d: CPU frame %dx%d", i, w, h)
		}
	}
	if got := rendererOf(a, r); got != "cpu" {
		t.Fatalf("Renderer = %q", got)
	}
	if l := lr.get(); len(l) != 1 || !strings.Contains(l[0], "GPU unavailable") || !strings.Contains(l[0], "d3dcompiler_47.dll") {
		t.Fatalf("logs %q", l)
	}
}

// d3d11's present counts consecutive failures and reports ErrDeviceLost
// after three: the glue falls back to the CPU for good, once, and does not
// keep calling present.
func TestHWNDPresentDeviceLostFallsBack(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	lr := hooksHWND(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasHWND(0)
	r, s, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "x") })
	g.mu.Lock()
	g.presErr = fmt.Errorf("d3d11: present: 3 consecutive failures: %w", d3d11.ErrDeviceLost)
	g.mu.Unlock()
	r.Invalidate()
	cv.FlushFrame()
	if rendererOf(a, r) != "cpu" || !cv.Pending() {
		t.Fatal("no fallback after present lost the device")
	}
	_, _, p0 := g.stats()
	for i := 0; i < 3; i++ {
		if i > 0 {
			r.Invalidate()
		}
		cv.FlushFrame()
		if w, h := cv.LastFrameSize(); w != 300 || h != 200 {
			t.Fatalf("CPU frame %dx%d", w, h)
		}
	}
	if _, _, p := g.stats(); len(p) != len(p0) {
		t.Fatalf("present called after the fallback (%d -> %d)", len(p0), len(p))
	}
	if cv.Pending() {
		t.Fatal("CPU frames keep asking for frames (spinning)")
	}
	if l := lr.get(); len(l) != 1 || !strings.Contains(l[0], "GPU lost") {
		t.Fatalf("logs %q", l)
	}
	tango.Await(a, func() bool { s.Close(); return true })
	fb.Do(func() {})
	if n := g.closeCount(); n != 1 {
		t.Fatalf("renderer closed %d times, want 1", n)
	}
}

// A frame over the instance-count limit (or the texture limit) is drawn on
// the CPU and shown through Present(img) for that frame only; the next
// fitting frame is back on the GPU and Present(nil) hides the CPU frame.
// The log names the cause.
func TestHWNDTooLargeTemporaryCPU(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	lr := hooksHWND(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasHWND(0)
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "x") })
	g.mu.Lock()
	g.failAt, g.failErr = g.renders+1, fmt.Errorf("d3d11: execute: instance count exceeds the device buffer limit: %w", gpu.ErrTargetTooLarge)
	g.mu.Unlock()
	r.Invalidate()
	cv.FlushFrame()
	if w, h := cv.LastFrameSize(); w != 300 || h != 200 || cv.LastFrame() == nil {
		t.Fatalf("oversized frame not on the CPU: %dx%d", w, h)
	}
	if rendererOf(a, r) != "gpu" || cv.Pending() {
		t.Fatalf("renderer %q pending %v", rendererOf(a, r), cv.Pending())
	}
	g.mu.Lock()
	g.failAt = 0
	g.mu.Unlock()
	r.Invalidate()
	cv.FlushFrame()
	if cv.LastFrame() != nil {
		t.Fatal("fitting frame did not return to the GPU")
	}
	found := false
	for _, l := range lr.get() {
		if strings.Contains(l, "instance count") {
			found = true
		}
		if strings.Contains(l, "GPU lost") {
			t.Fatalf("treated as device loss: %q", l)
		}
	}
	if !found {
		t.Fatalf("log does not name the cause: %q", lr.get())
	}
}

func TestHWNDReleasedOnClose(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	hooksHWND(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasHWND(0)
	r, s, _ := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	if rendererOf(a, r) != "gpu" || g.closeCount() != 0 {
		t.Fatalf("renderer %q closes %d before close", rendererOf(a, r), g.closeCount())
	}
	for i := 0; i < 2; i++ { // closing again is harmless
		tango.Await(a, func() bool { s.Close(); return true })
		fb.Do(func() {})
		if n := g.closeCount(); n != 1 {
			t.Fatalf("close %d: renderer closed %d times, want 1", i+1, n)
		}
	}
}

func TestHWNDAttachNoGPU(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "gpu")
	g := &fakeGPU{}
	hooksHWND(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasHWND(0)
	s := webSurface(t, a, 800, 600)
	_, _, acv := attach(t, a, fb, s, uihost.Layer{Bounds: tango.Rect{Width: 200, Height: 100}}, func(c *ui.Context) { ui.Text(c, "x") })
	if n, _, _ := g.stats(); n != 0 || acv.GPUWindowCalls() != 0 {
		t.Fatalf("NewPresenter calls %d, UseGPUWindow calls %d for an attached layer", n, acv.GPUWindowCalls())
	}
}
