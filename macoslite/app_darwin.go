//go:build darwin

package macoslite

import (
	"image"
	"math"
	"runtime"
	"sync"
)

// AppKit must run on the process' main thread, which is the thread package
// initialization runs on; keep the main goroutine there.
func init() { runtime.LockOSThread() }

// nativeBackend is the platform side of an App: the AppKit implementation in
// window_darwin.go, or a recording fake in tests. Every method except
// dispatchMain runs on the main thread.
type nativeBackend interface {
	open(o Options) error // create the window (Run)
	loop() error          // run the event loop until close
	setNeedsDisplay()     // ask AppKit for a frame
	dispatchMain(func())  // queue work for the main thread; any goroutine
	present(img *image.RGBA)
	close() // close the window and end loop; window may be gone already
}

// App owns one native window. Create it with New and call Run on the main
// goroutine.
type App struct {
	opts Options
	view View
	be   nativeBackend

	mu      sync.Mutex
	pending bool // a setNeedsDisplay is queued and no frame has run since
	stopped bool

	// Main thread only.
	drawing bool
	img     *image.RGBA
	fonts   fontCache
}

// New validates the options and returns an App. It does not touch native
// APIs; the window is created by Run.
func New(o Options, v View) (*App, error) {
	be := &appkitBackend{}
	a, err := newWithBackend(o, v, be)
	if err != nil {
		return nil, err
	}
	be.app = a
	return a, nil
}

func newWithBackend(o Options, v View, be nativeBackend) (*App, error) {
	if err := validate(o, v); err != nil {
		return nil, err
	}
	return &App{opts: o, view: v, be: be}, nil
}

// Run creates the window and runs the AppKit event loop until Stop is called
// or the window is closed. It must be called on the main goroutine.
func (a *App) Run() error {
	a.mu.Lock()
	stopped := a.stopped
	a.mu.Unlock()
	if stopped {
		return nil
	}
	if err := a.be.open(a.opts); err != nil {
		return err
	}
	return a.be.loop()
}

// Stop closes the window and makes Run return. Safe from any goroutine and
// safe to call more than once; View.Close runs once, on the main thread.
func (a *App) Stop() { a.be.dispatchMain(func() { a.finish(true) }) }

// handleClose runs when the user closed the window.
func (a *App) handleClose() { a.finish(false) }

func (a *App) finish(closeNative bool) {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return
	}
	a.stopped = true
	a.mu.Unlock()
	if closeNative {
		a.be.close()
	}
	a.img = nil
	a.fonts.clear()
	a.view.Close()
}

// Invalidate schedules a redraw. Safe from any goroutine; calls made before
// the next frame coalesce into one.
func (a *App) Invalidate() {
	a.mu.Lock()
	if a.stopped || a.pending {
		a.mu.Unlock()
		return
	}
	a.pending = true
	a.mu.Unlock()
	a.be.dispatchMain(func() {
		a.mu.Lock()
		stopped := a.stopped
		a.mu.Unlock()
		if !stopped {
			a.be.setNeedsDisplay()
		}
	})
}

// TrimCaches releases cached fonts; the current frame stays on screen.
// Safe from any goroutine: the fonts are released on the main thread.
func (a *App) TrimCaches() { a.be.dispatchMain(a.fonts.clear) }

// drawFrame renders one frame of w×h points at scale and presents it. It is
// the only caller of View.Draw and ignores re-entrant calls.
func (a *App) drawFrame(w, h, scale float64) {
	if a.drawing {
		return
	}
	a.mu.Lock()
	a.pending = false
	stopped := a.stopped
	a.mu.Unlock()
	if stopped || !(scale > 0) {
		return
	}
	pw, ph := int(math.Ceil(w*scale)), int(math.Ceil(h*scale))
	if pw <= 0 || ph <= 0 {
		return
	}
	if a.img == nil || a.img.Rect.Dx() != pw || a.img.Rect.Dy() != ph {
		a.img = image.NewRGBA(image.Rect(0, 0, pw, ph))
	} else {
		clear(a.img.Pix)
	}
	a.drawing = true
	defer func() { a.drawing = false }()
	c := newCanvas(a.img, scale)
	c.fonts = &a.fonts
	a.view.Draw(&Frame{Width: w, Height: h, Scale: scale}, c)
	a.be.present(a.img)
}

// backingChanged runs when the window's backing scale changes (moved
// between a 1x and a Retina screen): the layer holds our own CGImage, so
// AppKit will not redraw it at the new scale by itself.
func (a *App) backingChanged() {
	a.mu.Lock()
	stopped := a.stopped
	a.mu.Unlock()
	if !stopped {
		a.be.setNeedsDisplay()
	}
}

// terminating runs from applicationShouldTerminate: (Dock Quit, logout).
// It closes the window and runs View.Close before AppKit can exit; closing
// the window also stops the event loop, so Run returns normally.
func (a *App) terminating() { a.finish(true) }
