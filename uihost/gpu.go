package uihost

// GPU presentation for Open's windows. Open asks the canvas for a native
// presentation target (gpuTarget: a CAMetalLayer through
// platform.CanvasMetal on macOS, an HWND through platform.CanvasHWND on
// Windows; gpu_darwin.go, gpu_windows.go, gpu_other.go) and attaches the
// platform's presenter to it (newPresenter: metal.NewPresenter,
// d3d11.NewPresenter); the runner then renders through a gpuRenderer and
// calls Window.Present(nil), which shows the frame with the presenter.
// When the GPU cannot start the window stays on the plain CPU path; when
// it is lost later (gpu.ErrDeviceLost) the gpuRenderer switches to a
// paint.CPU for the rest of the window's life and Present(nil) shows its
// image instead.
//
// ui.Runner has no way to swap its renderer, so the switch happens inside
// gpuRenderer: the runner always sees the same (non-*paint.CPU) renderer
// and always calls Present(nil). The tan dev worker (devOpen) and attached
// layers (Attach) stay on the CPU path.
//
// TANGO_RENDERER=cpu skips the GPU; =gpu only makes the "GPU unavailable"
// log louder (it still falls back: a window on the CPU beats no window).

import (
	"errors"
	"image"
	"io"
	"log"
	"math"
	"os"
	"sync"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/paint"
)

// presenterFunc is the shape of metal.NewPresenter and d3d11.NewPresenter:
// it attaches a GPU renderer to a native target and returns it with the
// function that shows its last frame.
type presenterFunc func(target uintptr) (paint.Renderer, func(size image.Point, scale float32) error, error)

// Per-platform choice (gpu_<GOOS>.go); test hooks.
var (
	gpuTarget    func(platform.Canvas) (uintptr, error) = platformGPUTarget    // canvas -> native target
	newPresenter presenterFunc                          = platformNewPresenter // native target -> presenter
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

// Renderer reports how r's window is drawn: "gpu" (Metal, Direct3D 11) or "cpu" (no
// GPU, TANGO_RENDERER=cpu, a fallback after the GPU was lost, or a runner
// uihost did not open). Main thread.
func Renderer(r *ui.Runner) string {
	if g, ok := renderers.Load(r); ok && !g.(*gpuRenderer).lost() {
		return "gpu"
	}
	return "cpu"
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
	forced := mode == "gpu"
	target, err := gpuTarget(cv)
	if err != nil {
		logGPUUnavailable(err, forced)
		return nil
	}
	r, present, err := newPresenter(target)
	if err != nil {
		// The canvas stays switched (Metal layer, GPU window); Present(img)
		// still shows CPU frames on it (a sublayer, a sibling window), so
		// the window uses the gpuRenderer in CPU mode.
		logGPUUnavailable(err, forced)
		return &gpuRenderer{cpu: paint.NewCPU()}
	}
	return &gpuRenderer{gpu: r, present: present}
}

// gpuRenderer renders with the GPU renderer until the device is lost,
// then with a paint.CPU. Main thread.
type gpuRenderer struct {
	gpu     paint.Renderer
	present func(size image.Point, scale float32) error
	cpu     *paint.CPU // non-nil: CPU mode
	closed  bool

	// Oversized frames: a frame beyond a device limit (texture size,
	// instance count; gpu.ErrTargetTooLarge) is drawn by
	// big for that frame only; every frame tries the GPU first again.
	big      *paint.CPU
	bigFrame bool // the current frame was drawn by big
	bigNoted bool // logged for the current oversized stretch
}

func (g *gpuRenderer) lost() bool { return g.cpu != nil }

func (g *gpuRenderer) Render(l *paint.List, size geom.Size, scale float32) error {
	g.bigFrame = false
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
}

// Close releases everything when the window goes away. Main thread;
// idempotent. The runner is closed first, so no frame follows.
func (g *gpuRenderer) Close() error {
	if g.closed {
		return nil
	}
	g.closed = true
	g.releaseGPU()
	g.cpu, g.big = nil, nil
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
		if errors.Is(err, gpu.ErrDeviceLost) {
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
	return cv.Present(nil) // hides a CPU frame shown before, if any
}

// frameError handles an error from Runner.Frame on a GPU window: a lost
// device switches to the CPU, anything else (the presenter re-uploads its
// state after a transient failure) asks for another frame.
func (g *gpuRenderer) frameError(cv platform.Canvas, id platform.SurfaceID, err error) {
	if g.cpu != nil {
		return
	}
	if errors.Is(err, gpu.ErrDeviceLost) {
		g.fallback(cv, id, err)
		return
	}
	if errors.Is(err, gpu.ErrTargetTooLarge) {
		return // handled in Render; never a reason to retry in a loop
	}
	cv.Invalidate()
}
