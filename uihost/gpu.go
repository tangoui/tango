package uihost

// GPU presentation for Open's windows. Open asks the canvas for a native
// presentation target (gpuTarget: a CAMetalLayer through
// platform.CanvasMetal on macOS, an HWND through platform.CanvasHWND on
// Windows, a GtkGLArea through platform.CanvasGL on Linux; gpu_darwin.go,
// gpu_windows.go, gpu_linux.go, gpu_other.go) and attaches the platform's
// presenter to it (newPresenter: metal.NewPresenter, d3d11.NewPresenter,
// gl.NewPresenter); the runner then renders through a gpuRenderer and
// calls Window.Present(nil), which shows the frame with the presenter.
// When the GPU cannot start the window stays on the plain CPU path (on
// Linux the canvas gives the GtkGLArea back, gpuRelease); when
// it is lost later (gpu.ErrDeviceLost) the gpuRenderer switches to a
// paint.CPU for the rest of the window's life and Present(nil) shows its
// image instead.
//
// ui.Runner has no way to swap its renderer, so the switch happens inside
// gpuRenderer: the runner always sees the same (non-*paint.CPU) renderer
// and always calls Present(nil). The tan dev worker (devOpen) and attached
// layers (Attach) stay on the CPU path.
//
// TANGO_RENDERER=cpu skips the GPU; =gpu makes the "GPU unavailable" log
// louder (it still falls back: a window on the CPU beats no window) and
// accepts a software OpenGL renderer.
//
// Software OpenGL. A presenter whose renderer reports its GL_RENDERER
// (interface{ GLRenderer() string }: gl.NewPresenter's) on Mesa's software
// rasterizers — the name contains "llvmpipe", "softpipe" or "swrast",
// case-insensitively, as on a VM without 3D acceleration or under
// LIBGL_ALWAYS_SOFTWARE — counts as no GPU: rasterizing on the CPU through
// GL costs more CPU than paint.CPU and adds a copy. setupGPU closes it,
// logs "GPU unavailable ... software GL renderer <name>" once and keeps
// the window on the plain CPU path, unless TANGO_RENDERER=gpu.

import (
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// presenterFunc is the shape of metal.NewPresenter, d3d11.NewPresenter and
// gl.NewPresenter:
// it attaches a GPU renderer to a native target and returns it with the
// function that shows its last frame.
type presenterFunc func(target uintptr) (paint.Renderer, func(size image.Point, scale float32) error, error)

// Per-platform choice (gpu_<GOOS>.go); test hooks.
var (
	gpuTarget    func(platform.Canvas) (uintptr, error) = platformGPUTarget    // canvas -> native target
	newPresenter presenterFunc                          = platformNewPresenter // native target -> presenter
	gpuRelease   func(platform.Canvas)                  = platformGPURelease   // undoes gpuTarget; nil: the target stays (see setupGPU)
	gpuLogf                                             = log.Printf
)

// metalTarget switches cv to a CAMetalLayer (macOS).
func metalTarget(cv platform.Canvas) (uintptr, error) {
	cm, ok := cv.(platform.CanvasMetal)
	if !ok {
		return 0, errors.New("canvas has no Metal layer")
	}
	return cm.UseMetalLayer()
}

// hwndTarget switches cv to GPU presentation on its HWND (Windows).
func hwndTarget(cv platform.Canvas) (uintptr, error) {
	ch, ok := cv.(platform.CanvasHWND)
	if !ok {
		return 0, errors.New("canvas has no GPU window")
	}
	return ch.UseGPUWindow()
}

// glAreaTarget switches cv to GPU presentation in a GtkGLArea (Linux).
func glAreaTarget(cv platform.Canvas) (uintptr, error) {
	cg, ok := cv.(platform.CanvasGL)
	if !ok {
		return 0, errors.New("canvas has no GtkGLArea")
	}
	return cg.UseGLArea()
}

// glAreaRelease gives cv's GtkGLArea back (Linux): the canvas returns to
// the plain cairo path.
func glAreaRelease(cv platform.Canvas) {
	if cg, ok := cv.(platform.CanvasGL); ok {
		cg.ReleaseGLArea()
	}
}

// softwareGLNames are the GL_RENDERER substrings (lower case) of Mesa's
// software rasterizers.
var softwareGLNames = [...]string{"llvmpipe", "softpipe", "swrast"}

// softwareGL returns r's GL_RENDERER string and true when r is an OpenGL
// renderer (interface{ GLRenderer() string }) on a software rasterizer.
func softwareGL(r paint.Renderer) (string, bool) {
	g, ok := r.(interface{ GLRenderer() string })
	if !ok {
		return "", false
	}
	name := g.GLRenderer()
	l := strings.ToLower(name)
	for _, s := range softwareGLNames {
		if strings.Contains(l, s) {
			return name, true
		}
	}
	return "", false
}

// The "GPU unavailable" log is printed once per process, not per window.
var gpuLogged struct {
	mu   sync.Mutex
	done bool
}

func logGPUUnavailable(err error, forced bool) {
	gpuLogged.mu.Lock()
	done := gpuLogged.done
	gpuLogged.done = true
	gpuLogged.mu.Unlock()
	if done {
		return
	}
	if forced {
		gpuLogf("tango: WARNING: TANGO_RENDERER=gpu but GPU unavailable, using CPU: %v", err)
		return
	}
	gpuLogf("tango: GPU unavailable, using CPU: %v", err)
}

// renderers maps a runner to its gpuRenderer (Renderer).
var renderers sync.Map // *ui.Runner -> *gpuRenderer

// Renderer reports how r's window is drawn: "gpu" (Metal, Direct3D 11,
// OpenGL), "native" (retained macOS content layers), or "cpu" (no GPU,
// TANGO_RENDERER=cpu, a fallback after the GPU
// was lost, or a runner uihost did not open). Main thread.
func Renderer(r *ui.Runner) string {
	if g, ok := renderers.Load(r); ok && !g.(*gpuRenderer).lost() {
		if g.(*gpuRenderer).nativeMode {
			return "native"
		}
		return "gpu"
	}
	return "cpu"
}

// autoCPUWindow is TANGO_RENDERER=auto's CPU-first threshold: small utility
// windows avoid Metal's drawable, heap and atlas residency cost.
func autoCPUWindow(cv platform.Canvas) bool {
	w, h := cv.Size()
	return w > 0 && h > 0 && w*h <= 100000
}

// setupGPU switches cv to GPU presentation and returns the renderer for
// the runner, or nil to stay on the CPU path. Main thread.
func setupGPU(cv platform.Canvas, ropts ui.RunnerOptions) *gpuRenderer {
	if ropts.Renderer != nil {
		return nil // the caller chose
	}
	mode := os.Getenv("TANGO_RENDERER")
	if mode == "cpu" {
		return nil
	}
	// Native layers are the default at every window size and for every
	// ui.ContentKind: they avoid Metal's fixed residency cost. A window
	// promotes itself to Metal once, after its frames stay expensive
	// (nativeRendererState) or, unless it is a ui.ContentData window, on the
	// frame after it receives keyboard input (keyboardInput: Metal's input
	// latency is about two frames lower). Forced native never promotes.
	if mode == "native" || mode == "" {
		if g := startNative(cv, mode == "native", ropts.Content); g != nil {
			return g
		}
	}
	forced := mode == "gpu"
	if mode == "auto" && autoCPUWindow(cv) {
		return &gpuRenderer{cpu: paint.NewCPU(), auto: &autoRendererState{canvas: cv}}
	}
	return startGPU(cv, forced)
}

// startGPU attempts presentation setup without applying the automatic policy.
func startGPU(cv platform.Canvas, forced bool) *gpuRenderer {
	target, err := gpuTarget(cv)
	if err != nil {
		logGPUUnavailable(err, forced)
		return nil
	}
	r, present, err := newPresenter(target)
	if err == nil && !forced {
		if name, ok := softwareGL(r); ok {
			if c, ok := r.(io.Closer); ok {
				_ = c.Close() // before the GLArea goes (gpuRelease below)
			}
			r, present = nil, nil
			err = fmt.Errorf("software GL renderer %s (TANGO_RENDERER=gpu uses it)", name)
		}
	}
	if err != nil {
		logGPUUnavailable(err, forced)
		if gpuRelease != nil {
			// Linux: the GtkGLArea goes, so it never shows an unrendered
			// buffer, and the canvas is back on the plain cairo path
			// (partial presents included).
			gpuRelease(cv)
			return nil
		}
		// The canvas stays switched (Metal layer, GPU window); Present(img)
		// still shows CPU frames on it (a sublayer, a sibling window), so
		// the window uses the gpuRenderer in CPU mode.
		return &gpuRenderer{cpu: paint.NewCPU()}
	}
	return &gpuRenderer{gpu: r, present: present}
}

// gpuRenderer renders with the GPU renderer until the device is lost,
// then with a paint.CPU. Main thread.
type gpuRenderer struct {
	gpu        paint.Renderer
	present    func(size image.Point, scale float32) error
	cpu        *paint.CPU // non-nil: CPU mode
	closed     bool
	nativeMode bool
	native     *nativeRendererState
	auto       *autoRendererState // CPU-first policy; cleared after promotion or failed attempt

	// Oversized frames: a frame beyond a device limit (texture size,
	// instance count; gpu.ErrTargetTooLarge) is drawn by
	// big for that frame only; every frame tries the GPU first again.
	big      *paint.CPU
	bigFrame bool // the current frame was drawn by big
	bigNoted bool // logged for the current oversized stretch

	trace *frameTrace // TANGO_FRAME_TRACE (frametrace.go); nil: off
}

// setTrace times Render and presentFrame into t and, when the presenter
// has a timeline (metal), feeds it drawable waits and GPU times.
func (g *gpuRenderer) setTrace(t *frameTrace) {
	g.trace = t
	g.attachTimeline()
}

// attachTimeline connects the current GPU renderer's timeline (if any) to
// the trace; called again after the renderer changes (auto promotion).
func (g *gpuRenderer) attachTimeline() {
	if g.trace == nil || g.gpu == nil {
		return
	}
	attachTimeline(g.gpu, g.trace) // frametrace.go
}

func (g *gpuRenderer) lost() bool { return g.cpu != nil }

func (g *gpuRenderer) Render(l *paint.List, size geom.Size, scale float32) error {
	if g.closed {
		return nil
	}
	g.bigFrame = false
	if g.native != nil {
		if g.native.ready {
			g.promoteNative()
		}
		if g.native != nil {
			g.native.start = time.Now()
		}
	}
	if g.auto != nil && g.auto.shouldPromote(l, size, time.Now()) {
		cv := g.auto.canvas
		g.auto = nil // try once; failures never cause a retry storm
		if promoted := startGPU(cv, false); promoted != nil && promoted.gpu != nil {
			g.gpu, g.present, g.cpu = promoted.gpu, promoted.present, nil
			g.attachTimeline()
		}
	}
	return g.trace.timed(g.traceSlot(func(t *frameTrace) *[]time.Duration { return &t.render }), func() error {
		return g.render(l, size, scale)
	})
}

// traceSlot returns the trace slice pick selects, or nil without a trace.
func (g *gpuRenderer) traceSlot(pick func(*frameTrace) *[]time.Duration) *[]time.Duration {
	if g.trace == nil {
		return nil
	}
	return pick(g.trace)
}

func (g *gpuRenderer) render(l *paint.List, size geom.Size, scale float32) error {
	if g.cpu != nil {
		return g.cpu.Render(l, size, scale)
	}
	if g.gpu == nil { // closed
		return nil
	}
	err := g.gpu.Render(l, size, scale)
	if err != nil && errors.Is(err, gpu.ErrTargetTooLarge) {
		// Not a GPU failure: draw this frame on the CPU and keep trying the
		// GPU on later frames, so a window shrinking back returns to it.
		if g.big == nil {
			g.big = paint.NewCPU()
		}
		if !g.bigNoted {
			g.bigNoted = true
			gpuLogf("tango: uihost: frame %vx%v@%v exceeds a GPU limit, drawing on the CPU until it fits: %v", size.W, size.H, scale, err)
		}
		g.bigFrame = true
		return g.big.Render(l, size, scale)
	}
	if err == nil && g.big != nil {
		g.big, g.bigNoted = nil, false // back within the limit: free the CPU buffer
	}
	return err
}

// releaseGPU frees the GPU side (device, queues or swap chain, render
// targets, atlas) if the renderer supports io.Closer, and forgets it.
// Idempotent.
func (g *gpuRenderer) releaseGPU() {
	if c, ok := g.gpu.(io.Closer); ok {
		_ = c.Close()
	}
	g.gpu, g.present = nil, nil
	g.nativeMode, g.native = false, nil
}

// Close releases everything when the window goes away. Main thread;
// idempotent. The runner is closed first, so no frame follows.
func (g *gpuRenderer) Close() error {
	if g.closed {
		return nil
	}
	g.closed = true
	g.releaseGPU()
	g.cpu, g.big, g.auto = nil, nil, nil
	return nil
}

// fallback switches to the CPU for good, once, and asks for a frame.
func (g *gpuRenderer) fallback(cv platform.Canvas, id platform.SurfaceID, err error) {
	if g.cpu != nil || g.closed {
		return
	}
	g.cpu = paint.NewCPU()
	g.releaseGPU() // errors are irrelevant once we are on the CPU
	gpuLogf("tango: uihost: surface %d: GPU lost, using CPU: %v", id, err)
	cv.Invalidate()
}

// presentFrame is window.Present(nil) on a GPU window.
func (g *gpuRenderer) presentFrame(cv platform.Canvas, id platform.SurfaceID) error {
	return g.trace.timed(g.traceSlot(func(t *frameTrace) *[]time.Duration { return &t.present }), func() error {
		return g.presentFrame1(cv, id)
	})
}

func (g *gpuRenderer) presentFrame1(cv platform.Canvas, id platform.SurfaceID) error {
	if g.cpu != nil {
		return cv.Present(g.cpu.Image())
	}
	if g.bigFrame { // oversized frame drawn on the CPU
		return cv.Present(g.big.Image())
	}
	if g.present == nil { // closed
		return nil
	}
	w, h := cv.Size()
	s := cv.Scale()
	px := image.Pt(int(math.Round(float64(w*s))), int(math.Round(float64(h*s))))
	if err := g.present(px, s); err != nil {
		if g.nativeMode || errors.Is(err, gpu.ErrDeviceLost) {
			g.fallback(cv, id, err)
			return nil
		}
		if errors.Is(err, gpu.ErrTargetTooLarge) {
			// The window grew past the limit after this frame rendered: the
			// next frame's Render takes the CPU path.
			cv.Invalidate()
			return nil
		}
		return err
	}
	g.nativePresented()
	return cv.Present(nil) // hides a CPU frame shown before, if any
}

// frameError handles an error from Runner.Frame on a GPU window: a lost
// device switches to the CPU, anything else (the presenter re-uploads its
// state after a transient failure) asks for another frame.
func (g *gpuRenderer) frameError(cv platform.Canvas, id platform.SurfaceID, err error) {
	if g.cpu != nil {
		return
	}
	if g.nativeMode || errors.Is(err, gpu.ErrDeviceLost) {
		g.fallback(cv, id, err)
		return
	}
	if errors.Is(err, gpu.ErrTargetTooLarge) {
		return // handled in Render; never a reason to retry in a loop
	}
	cv.Invalidate()
}
