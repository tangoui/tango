package uihost_test

// The Linux GPU glue (platform.CanvasGL → gl.NewPresenter's shape) on the
// fake backend, on every OS: SetGPUHooksGL selects the GtkGLArea target
// whatever GOOS the test runs on.

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/gl"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/uihost"
)

func hooksGL(t *testing.T, np func(uintptr) (paint.Renderer, func(image.Point, float32) error, error)) *logRec {
	t.Helper()
	lr := &logRec{}
	t.Cleanup(uihost.SetGPUHooksGL(np, lr.logf))
	return lr
}

// TestLinuxGPUTarget: EnableCanvasGL selects the GL presenter on the
// GtkGLArea; without CanvasGL, or when the presenter cannot start, the
// window draws on the CPU; a lost device falls back for good.
func TestLinuxGPUTarget(t *testing.T) {
	t.Run("gpu", func(t *testing.T) {
		t.Setenv("TANGO_RENDERER", "")
		g := &fakeGPU{}
		lr := hooksGL(t, g.newPresenter)
		a, fb := faketest.Run(t, tango.Options{})
		fb.SetCanvasScale(2)
		fb.EnableCanvasGL(0)
		fb.EnableCanvasHWND(0)  // must not be used with the GL target
		fb.EnableCanvasMetal(0) // nor this
		r, s, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
		if cv.GLAreaCalls() != 1 || cv.GLArea() != fake.FakeGLArea || cv.GPUWindowCalls() != 0 || cv.MetalCalls() != 0 {
			t.Fatalf("UseGLArea calls %d area %#x, hwnd calls %d, metal calls %d", cv.GLAreaCalls(), cv.GLArea(), cv.GPUWindowCalls(), cv.MetalCalls())
		}
		layers, renders, presents := g.stats()
		if layers != 1 || g.layers[0] != fake.FakeGLArea {
			t.Fatalf("NewPresenter calls %v", g.layers)
		}
		if renders < 1 || len(presents) != renders || presents[0] != image.Pt(600, 400) {
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
		tango.Await(a, func() bool { s.Close(); return true })
		fb.Do(func() {})
		if n := g.closeCount(); n != 1 {
			t.Fatalf("renderer closed %d times, want 1", n)
		}
	})

	// Without CanvasGL (attached, layered, transparent canvases answer
	// ErrUnsupported; GL unavailable is an error) the window stays on the
	// plain CPU path.
	t.Run("unsupported", func(t *testing.T) {
		t.Setenv("TANGO_RENDERER", "")
		g := &fakeGPU{}
		lr := hooksGL(t, g.newPresenter)
		a, fb := faketest.Run(t, tango.Options{})
		fb.EnableCanvasHWND(0)
		r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
		if cv.GLAreaCalls() != 1 || cv.GPUWindowCalls() != 0 {
			t.Fatalf("UseGLArea calls %d, hwnd calls %d", cv.GLAreaCalls(), cv.GPUWindowCalls())
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
	})

	// NewPresenter failing (no GL 3.3, shader rejected) gives the GLArea
	// back (ReleaseGLArea): the window is on the plain CPU path.
	t.Run("presenter fails", func(t *testing.T) {
		t.Setenv("TANGO_RENDERER", "")
		calls := 0
		lr := hooksGL(t, func(uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
			calls++
			return nil, nil, fmt.Errorf("gl: OpenGL 2.1 (llvmpipe), need 3.3: %w", gl.ErrNoDevice)
		})
		a, fb := faketest.Run(t, tango.Options{})
		fb.SetCanvasScale(2)
		fb.EnableCanvasGL(0)
		r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
		if n, released := cv.GLAreaReleases(); calls != 1 || n != 1 || !released || cv.GLArea() != 0 {
			t.Fatalf("NewPresenter calls %d, ReleaseGLArea calls %d (released %v), area %#x", calls, n, released, cv.GLArea())
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
		if cv.LastFrame() == nil {
			t.Fatal("no CPU image presented")
		}
		if l := lr.get(); len(l) != 1 || !strings.Contains(l[0], "GPU unavailable") || !strings.Contains(l[0], "need 3.3") {
			t.Fatalf("logs %q", l)
		}
	})

	// A lost device (make_current failing, GL_OUT_OF_MEMORY, a context
	// reset) switches to the CPU once and for good.
	t.Run("lost", func(t *testing.T) {
		t.Setenv("TANGO_RENDERER", "")
		g := &fakeGPU{}
		lr := hooksGL(t, g.newPresenter)
		a, fb := faketest.Run(t, tango.Options{})
		fb.EnableCanvasGL(0)
		r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "x") })
		g.mu.Lock()
		g.presErr = fmt.Errorf("gl: GLArea: no GL context: %w", gpu.ErrDeviceLost)
		g.mu.Unlock()
		r.Invalidate()
		cv.FlushFrame()
		if rendererOf(a, r) != "cpu" {
			t.Fatal("no fallback after present lost the device")
		}
		cv.FlushFrame()
		if w, h := cv.LastFrameSize(); w != 300 || h != 200 {
			t.Fatalf("CPU frame %dx%d", w, h)
		}
		if n := g.closeCount(); n != 1 {
			t.Fatalf("GPU renderer closed %d times after the fallback, want 1", n)
		}
		if l := lr.get(); len(l) != 1 || !strings.Contains(l[0], "GPU lost") {
			t.Fatalf("logs %q", l)
		}
	})
}

// namedGL is a fake GL presenter's renderer reporting its GL_RENDERER
// (gl.NewPresenter's GLRenderer).
type namedGL struct {
	*fakeGPU
	name string
}

func (g namedGL) GLRenderer() string { return g.name }

// TestLinuxSoftwareGL: a presenter on Mesa's software rasterizers
// (llvmpipe, softpipe, swrast) counts as no GPU: it is closed, the GLArea
// given back, the window stays on the plain CPU path and the log says
// why — unless TANGO_RENDERER=gpu. Any other renderer is used.
func TestLinuxSoftwareGL(t *testing.T) {
	cases := []struct {
		name, mode, renderer string
		gpu                  bool
	}{
		{"llvmpipe", "", "llvmpipe (LLVM 15.0.6, 128 bits)", false},
		{"softpipe", "", "Gallium 0.4 on SoftPipe", false},
		{"swrast", "", "Software Rasterizer (SWRAST)", false},
		{"llvmpipe forced", "gpu", "llvmpipe (LLVM 15.0.6, 128 bits)", true},
		{"hardware", "", "Mesa Intel(R) UHD Graphics 620 (KBL GT2)", true},
		{"no name", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("TANGO_RENDERER", c.mode)
			g := &fakeGPU{}
			lr := hooksGL(t, func(area uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
				_, pr, err := g.newPresenter(area)
				return namedGL{g, c.renderer}, pr, err
			})
			a, fb := faketest.Run(t, tango.Options{})
			fb.SetCanvasScale(2)
			fb.EnableCanvasGL(0)
			r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
			n, released := cv.GLAreaReleases()
			_, renders, presents := g.stats()
			if !c.gpu {
				if got := rendererOf(a, r); got != "cpu" {
					t.Fatalf("Renderer = %q, want cpu", got)
				}
				if g.closeCount() != 1 || n != 1 || !released || cv.GLArea() != 0 {
					t.Fatalf("presenter closed %d times, ReleaseGLArea calls %d (released %v), area %#x", g.closeCount(), n, released, cv.GLArea())
				}
				if renders != 0 || len(presents) != 0 {
					t.Fatalf("GPU renders %d presents %v", renders, presents)
				}
				if w, h := cv.LastFrameSize(); w != 600 || h != 400 {
					t.Fatalf("CPU frame %dx%d", w, h)
				}
				if l := lr.get(); len(l) != 1 || !strings.Contains(l[0], "GPU unavailable") || !strings.Contains(l[0], "software GL renderer "+c.renderer) {
					t.Fatalf("logs %q", l)
				}
				return
			}
			if got := rendererOf(a, r); got != "gpu" {
				t.Fatalf("Renderer = %q, want gpu", got)
			}
			if g.closeCount() != 0 || n != 0 || cv.GLArea() != fake.FakeGLArea {
				t.Fatalf("presenter closed %d times, ReleaseGLArea calls %d, area %#x", g.closeCount(), n, cv.GLArea())
			}
			if renders < 1 || len(presents) < 1 || cv.LastFrame() != nil {
				t.Fatalf("GPU renders %d presents %v, CPU image %v", renders, presents, cv.LastFrame() != nil)
			}
			if l := lr.get(); len(l) != 0 {
				t.Fatalf("logs %q", l)
			}
		})
	}
}
