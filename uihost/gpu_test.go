package uihost_test

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/render/gpu/metal"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/uihost"
)

// fakeGPU stands in for metal.NewPresenter: a Renderer that counts
// frames (and fails from frame failAt on with failErr) and a present that
// records the sizes it is given.
type fakeGPU struct {
	mu       sync.Mutex
	layers   []uintptr
	renders  int
	failAt   int // 0: never
	failErr  error
	presents []image.Point
	scales   []float32
	presErr  error // returned by present when set
	closes   int
	maxPx    float32 // >0: Render fails with ErrTargetTooLarge above this many device pixels wide
}

// Close makes the fake renderer an io.Closer, as metal's is.
func (g *fakeGPU) Close() error {
	g.mu.Lock()
	g.closes++
	g.mu.Unlock()
	return nil
}

func (g *fakeGPU) closeCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closes
}

func (g *fakeGPU) newPresenter(layer uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
	g.mu.Lock()
	g.layers = append(g.layers, layer)
	g.mu.Unlock()
	return g, g.present, nil
}

func (g *fakeGPU) Render(l *paint.List, size geom.Size, scale float32) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.renders++
	if g.maxPx > 0 && size.W*scale > g.maxPx {
		return fmt.Errorf("metal: execute: %w", metal.ErrTargetTooLarge)
	}
	if g.failAt > 0 && g.renders >= g.failAt {
		return g.failErr
	}
	return nil
}

func (g *fakeGPU) present(size image.Point, scale float32) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.presents = append(g.presents, size)
	g.scales = append(g.scales, scale)
	return g.presErr
}

func (g *fakeGPU) stats() (layers, renders int, presents []image.Point) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.layers), g.renders, append([]image.Point(nil), g.presents...)
}

// logRec collects GPU log lines.
type logRec struct {
	mu    sync.Mutex
	lines []string
}

func (l *logRec) logf(f string, a ...any) {
	l.mu.Lock()
	l.lines = append(l.lines, fmt.Sprintf(f, a...))
	l.mu.Unlock()
}

func (l *logRec) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

func hooks(t *testing.T, g *fakeGPU) *logRec {
	t.Helper()
	lr := &logRec{}
	var np func(uintptr) (paint.Renderer, func(image.Point, float32) error, error)
	if g != nil {
		np = g.newPresenter
	}
	t.Cleanup(uihost.SetGPUHooks(np, lr.logf))
	return lr
}

func rendererOf(a *tango.App, r *ui.Runner) string {
	return tango.Await(a, func() string { return uihost.Renderer(r) })
}

func TestGPUPath(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	lr := hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	fb.EnableCanvasMetal(0)
	r, _, cv := open(t, a, fb, 301, 200, func(c *ui.Context) { ui.Text(c, "hello") })

	if cv.MetalCalls() != 1 || cv.MetalLayer() != fake.FakeMetalLayer {
		t.Fatalf("metal calls %d layer %#x", cv.MetalCalls(), cv.MetalLayer())
	}
	layers, renders, presents := g.stats()
	if layers != 1 || g.layers[0] != fake.FakeMetalLayer {
		t.Fatalf("NewPresenter calls %v", g.layers)
	}
	if renders < 1 || len(presents) != renders {
		t.Fatalf("renders %d presents %v", renders, presents)
	}
	if presents[0] != image.Pt(602, 400) {
		t.Fatalf("present size %v, want 602x400", presents[0])
	}
	// The runner presented nil (the GPU did it), not a CPU image.
	if cv.Presents() < 1 || cv.LastFrame() != nil {
		t.Fatalf("canvas presents %d last %v", cv.Presents(), cv.LastFrame() != nil)
	}
	if got := rendererOf(a, r); got != "gpu" {
		t.Fatalf("Renderer = %q", got)
	}
	if l := lr.get(); len(l) != 0 {
		t.Fatalf("logs %q", l)
	}
}

func TestGPURendererOptionWins(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	o := tango.Await(a, func() opened {
		r, s, err := uihost.Open(a, uihost.Options{Surface: tango.SurfaceOptions{Width: 100, Height: 100}},
			func(c *ui.Context) {}, ui.RunnerOptions{Renderer: paint.NewCPU()})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	fb.Do(func() {})
	cv := fb.Canvas(o.s.ID())
	if cv.MetalCalls() != 0 {
		t.Fatalf("UseMetalLayer called %d times", cv.MetalCalls())
	}
	if w, _ := cv.LastFrameSize(); w != 100 {
		t.Fatalf("CPU frame width %d", w)
	}
}

func TestGPUEnvCPU(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "cpu")
	g := &fakeGPU{}
	lr := hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	fb.EnableCanvasMetal(0)
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	if cv.MetalCalls() != 0 {
		t.Fatalf("UseMetalLayer called %d times", cv.MetalCalls())
	}
	if n, _, _ := g.stats(); n != 0 {
		t.Fatal("NewPresenter called")
	}
	if w, h := cv.LastFrameSize(); w != 600 || h != 400 {
		t.Fatalf("CPU frame %dx%d", w, h)
	}
	if got := rendererOf(a, r); got != "cpu" {
		t.Fatalf("Renderer = %q", got)
	}
	if l := lr.get(); len(l) != 0 {
		t.Fatalf("logs %q", l)
	}
}

func TestGPUUnsupportedLogsOnce(t *testing.T) {
	for _, mode := range []string{"", "gpu"} {
		t.Run("mode="+mode, func(t *testing.T) {
			t.Setenv("TANGO_RENDERER", mode)
			g := &fakeGPU{}
			lr := hooks(t, g)
			a, fb := faketest.Run(t, tango.Options{}) // CanvasMetal off
			fb.SetCanvasScale(2)
			for i := 0; i < 3; i++ {
				r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
				if cv.MetalCalls() != 1 {
					t.Fatalf("UseMetalLayer calls %d", cv.MetalCalls())
				}
				if w, h := cv.LastFrameSize(); w != 600 || h != 400 {
					t.Fatalf("CPU frame %dx%d", w, h)
				}
				if got := rendererOf(a, r); got != "cpu" {
					t.Fatalf("Renderer = %q", got)
				}
			}
			if n, _, _ := g.stats(); n != 0 {
				t.Fatal("NewPresenter called")
			}
			l := lr.get()
			if len(l) != 1 || !strings.Contains(l[0], "GPU unavailable") || !strings.Contains(l[0], "using CPU") {
				t.Fatalf("logs %q", l)
			}
			if loud := strings.Contains(l[0], "WARNING"); loud != (mode == "gpu") {
				t.Fatalf("log %q for mode %q", l[0], mode)
			}
		})
	}
}

func TestGPUPresenterFails(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	lr := &logRec{}
	t.Cleanup(uihost.SetGPUHooks(func(uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		return nil, nil, fmt.Errorf("NewPresenter: %w", metal.ErrUnsupported)
	}, lr.logf))
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	fb.EnableCanvasMetal(0)
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	if w, h := cv.LastFrameSize(); w != 600 || h != 400 {
		t.Fatalf("CPU frame %dx%d", w, h)
	}
	if got := rendererOf(a, r); got != "cpu" {
		t.Fatalf("Renderer = %q", got)
	}
	if l := lr.get(); len(l) != 1 || !strings.Contains(l[0], "GPU unavailable") {
		t.Fatalf("logs %q", l)
	}
}

func TestFallbackOnDeviceError(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{failErr: fmt.Errorf("metal: execute: %w", metal.ErrDeviceLost)}
	lr := hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	fb.EnableCanvasMetal(0)
	n := 0
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) {
		n++
		ui.Text(c, fmt.Sprint("frame ", n))
	})
	_, first, _ := g.stats()
	g.mu.Lock()
	g.failAt = first + 3 // the third frame from now loses the device
	g.mu.Unlock()
	for i := 0; i < 2; i++ { // two more frames on the GPU
		r.Invalidate()
		if !cv.FlushFrame() {
			t.Fatal("no frame")
		}
	}
	if cv.LastFrame() != nil || rendererOf(a, r) != "gpu" {
		t.Fatal("left the GPU early")
	}
	r.Invalidate()
	cv.FlushFrame() // device lost
	if got := rendererOf(a, r); got != "cpu" {
		t.Fatalf("Renderer = %q after device loss", got)
	}
	if !cv.Pending() {
		t.Fatal("no frame requested after the fallback")
	}
	_, renders, _ := g.stats()
	for i := 0; i < 3; i++ { // the window keeps producing frames, on the CPU
		before := cv.Presents()
		if i > 0 {
			r.Invalidate()
		}
		if !cv.FlushFrame() {
			t.Fatal("no frame after the fallback")
		}
		if cv.Presents() != before+1 {
			t.Fatalf("presents %d -> %d", before, cv.Presents())
		}
		if w, h := cv.LastFrameSize(); w != 600 || h != 400 {
			t.Fatalf("CPU frame %dx%d", w, h)
		}
	}
	if _, r2, _ := g.stats(); r2 != renders {
		t.Fatalf("GPU renderer used after the fallback (%d -> %d)", renders, r2)
	}
	var lost []string
	for _, l := range lr.get() {
		if strings.Contains(l, "GPU lost") {
			lost = append(lost, l)
		}
	}
	if len(lost) != 1 {
		t.Fatalf("fallback logs %q", lr.get())
	}
}

func TestGPUTransientErrorRedraws(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{failErr: errors.New("metal: command buffer failed")}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "x") })
	g.mu.Lock()
	g.failAt = g.renders + 1
	g.mu.Unlock()
	r.Invalidate()
	cv.FlushFrame() // fails
	if !cv.Pending() {
		t.Fatal("no redraw requested after a transient GPU error")
	}
	g.mu.Lock()
	g.failAt = 0
	g.mu.Unlock()
	_, _, p0 := g.stats()
	before := len(p0)
	cv.FlushFrame()
	if _, _, p := g.stats(); len(p) != before+1 {
		t.Fatal("redraw did not present")
	}
	if rendererOf(a, r) != "gpu" {
		t.Fatal("transient error fell back to the CPU")
	}
}

func TestMetalScaleChange(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	fb.EnableCanvasMetal(0)
	_, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "x") })
	steps := []struct {
		w, h  int
		scale float32
		want  image.Point
	}{
		{300, 200, 1, image.Pt(300, 200)},
		{300, 200, 2, image.Pt(600, 400)},
		{333, 201, 1.5, image.Pt(500, 302)}, // round(499.5)=500, round(301.5)=302
	}
	for _, s := range steps {
		cv.Resize(s.w, s.h, s.scale)
		if !cv.FlushFrame() {
			t.Fatal("no frame after resize")
		}
		_, _, p := g.stats()
		g.mu.Lock()
		sc := g.scales[len(g.scales)-1]
		g.mu.Unlock()
		if got := p[len(p)-1]; got != s.want || sc != s.scale {
			t.Fatalf("%dx%d@%v: present %v@%v, want %v", s.w, s.h, s.scale, got, sc, s.want)
		}
	}
}

func TestAttachNoGPU(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "gpu")
	g := &fakeGPU{}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	s := webSurface(t, a, 800, 600)
	_, _, acv := attach(t, a, fb, s, uihost.Layer{Bounds: tango.Rect{Width: 200, Height: 100}}, func(c *ui.Context) { ui.Text(c, "x") })
	if n, _, _ := g.stats(); n != 0 {
		t.Fatal("NewPresenter called for an attached layer")
	}
	if acv.Presents() < 1 {
		t.Fatal("attached layer not presented")
	}
}

func TestFallbackOnPresentDeviceLost(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	lr := hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "x") })
	g.mu.Lock()
	g.presErr = fmt.Errorf("present: %w", metal.ErrDeviceLost)
	g.mu.Unlock()
	r.Invalidate()
	cv.FlushFrame()
	if rendererOf(a, r) != "cpu" || !cv.Pending() {
		t.Fatal("no fallback after present lost the device")
	}
	cv.FlushFrame()
	if w, h := cv.LastFrameSize(); w != 300 || h != 200 {
		t.Fatalf("CPU frame %dx%d", w, h)
	}
	if l := lr.get(); len(l) != 1 || !strings.Contains(l[0], "GPU lost") {
		t.Fatalf("logs %q", l)
	}
}

// Closing a GPU window must free the Metal resources (renderer Close), once.
func TestGPUReleasedOnClose(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	fb.EnableCanvasMetal(0)
	r, s, _ := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	if rendererOf(a, r) != "gpu" || g.closeCount() != 0 {
		t.Fatalf("renderer %q closes %d before close", rendererOf(a, r), g.closeCount())
	}
	tango.Await(a, func() bool { s.Close(); return true })
	fb.Do(func() {})
	if n := g.closeCount(); n != 1 {
		t.Fatalf("renderer closed %d times after window close, want 1", n)
	}
	tango.Await(a, func() bool { s.Close(); return true }) // closing again is harmless
	fb.Do(func() {})
	if n := g.closeCount(); n != 1 {
		t.Fatalf("renderer closed %d times after a second close, want 1", n)
	}
}

// After a fallback the GPU is already released; closing the window does not
// close it again.
func TestGPUReleasedOnceAfterFallback(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{failAt: 1, failErr: fmt.Errorf("metal: %w", metal.ErrDeviceLost)}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	fb.EnableCanvasMetal(0)
	r, s, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	for i := 0; i < 3 && rendererOf(a, r) != "cpu"; i++ {
		r.Invalidate()
		cv.FlushFrame()
	}
	if rendererOf(a, r) != "cpu" {
		t.Fatal("did not fall back")
	}
	tango.Await(a, func() bool { s.Close(); return true })
	fb.Do(func() {})
	if n := g.closeCount(); n != 1 {
		t.Fatalf("renderer closed %d times, want 1", n)
	}
}

// A frame beyond the GPU texture limit is drawn and presented on the CPU,
// the window stays on the GPU, and frames return to the GPU once it fits.
func TestGPUOversizeUsesCPUTemporarily(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	g := &fakeGPU{maxPx: 1000}
	lr := hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.SetCanvasScale(2)
	fb.EnableCanvasMetal(0)
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	if cv.LastFrame() != nil {
		t.Fatal("first frame should be on the GPU")
	}

	cv.Resize(600, 200, 2) // 1200 device px > 1000
	cv.FlushFrame()
	img := cv.LastFrame()
	if img == nil || img.Rect.Dx() != 1200 || img.Rect.Dy() != 400 {
		t.Fatalf("oversized frame not presented on the CPU: %v", img)
	}
	if got := rendererOf(a, r); got != "gpu" {
		t.Fatalf("Renderer = %q, an oversized frame must not fall back for good", got)
	}
	if cv.Pending() {
		t.Fatal("oversized frame asked for another frame (retry loop)")
	}
	r.Invalidate()
	cv.FlushFrame() // still too large: CPU again, still no loop
	if cv.LastFrame() == nil || cv.Pending() {
		t.Fatal("second oversized frame")
	}

	_, _, before := g.stats()
	cv.Resize(300, 200, 2) // back within the limit
	cv.FlushFrame()
	_, _, after := g.stats()
	if cv.LastFrame() != nil || len(after) <= len(before) {
		t.Fatalf("did not return to the GPU: last=%v presents %d->%d", cv.LastFrame() != nil, len(before), len(after))
	}
	n := 0
	for _, l := range lr.get() {
		if strings.Contains(l, "texture limit") {
			n++
		}
		if strings.Contains(l, "GPU lost") {
			t.Fatalf("treated as device loss: %q", l)
		}
	}
	if n != 1 {
		t.Fatalf("texture-limit log lines = %d, want 1", n)
	}
}
