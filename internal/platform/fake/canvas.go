package fake

// Canvas surfaces (internal/platform/canvas.go): CanvasCreator on Backend
// and a recording Canvas with helpers that simulate the native side.

import (
	"image"
	"slices"
	"sync"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

var _ platform.CanvasCreator = (*Backend)(nil)

// canvasCfg is a backend's canvas configuration, kept beside Backend so the
// round-1 type stays as it is.
type canvasCfg struct {
	mu       sync.Mutex
	scale    float32
	canvases map[platform.SurfaceID]*Canvas
}

var canvasCfgs sync.Map // *Backend -> *canvasCfg

func (b *Backend) canvasCfg() *canvasCfg {
	if c, ok := canvasCfgs.Load(b); ok {
		return c.(*canvasCfg)
	}
	c, _ := canvasCfgs.LoadOrStore(b, &canvasCfg{scale: 1, canvases: map[platform.SurfaceID]*Canvas{}})
	return c.(*canvasCfg)
}

// SetCanvasScale sets the device scale of canvases created from now on
// (1 by default).
func (b *Backend) SetCanvasScale(scale float32) {
	c := b.canvasCfg()
	c.mu.Lock()
	c.scale = scale
	c.mu.Unlock()
}

// Canvas returns the canvas of surface id, or nil.
func (b *Backend) Canvas(id platform.SurfaceID) *Canvas {
	c := b.canvasCfg()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.canvases[id]
}

// NewCanvasSurface implements platform.CanvasCreator. The surface is a
// normal fake Surface (created as by NewSurfaceEx). A visible canvas gets
// its first CanvasFrame as a queued native event, as when a real window is
// first shown.
func (b *Backend) NewCanvasSurface(id platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras, sink platform.CanvasSink) (platform.Surface, platform.Canvas, error) {
	b.mustMain("NewCanvasSurface")
	opts.URL = "" // no page
	ns, err := b.NewSurfaceEx(id, opts, ex)
	if err != nil {
		return nil, nil, err
	}
	cfg := b.canvasCfg()
	cfg.mu.Lock()
	cv := &Canvas{b: b, s: ns.(*Surface), sink: sink, scale: cfg.scale}
	cfg.canvases[id] = cv
	cfg.mu.Unlock()
	if opts.Visible {
		b.enqueue(cv.frame)
	}
	return ns, cv, nil
}

// Canvas is a fake platform.Canvas. Invalidate and InvalidateAt only
// record the request; frames run when a test calls Frame or FlushFrame
// (and once after creation). Its accessors are safe from any goroutine.
type Canvas struct {
	b    *Backend
	s    *Surface
	sink platform.CanvasSink

	mu            sync.Mutex
	scale         float32
	pending       bool // Invalidate since the last frame
	invalidates   int
	invalidateAts []time.Time
	frames        int
	presents      int
	last          *image.RGBA
	cursor        input.Cursor
	textOn        bool
	caret         geom.Rect
}

// ---- platform.Canvas ----

func (c *Canvas) Size() (w, h float32) {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	return float32(c.s.w), float32(c.s.h)
}

func (c *Canvas) Scale() float32 { c.mu.Lock(); defer c.mu.Unlock(); return c.scale }

func (c *Canvas) Invalidate() {
	c.mu.Lock()
	c.invalidates++
	c.pending = true
	c.mu.Unlock()
}

func (c *Canvas) InvalidateAt(t time.Time) {
	c.mu.Lock()
	c.invalidateAts = append(c.invalidateAts, t)
	c.mu.Unlock()
}

func (c *Canvas) SetCursor(cur input.Cursor) {
	c.b.mustMain("SetCursor")
	c.mu.Lock()
	c.cursor = cur
	c.mu.Unlock()
}

func (c *Canvas) SetTextInput(enabled bool, caret geom.Rect) {
	c.b.mustMain("SetTextInput")
	c.mu.Lock()
	c.textOn, c.caret = enabled, caret
	c.mu.Unlock()
}

// Present records a copy of img.
func (c *Canvas) Present(img *image.RGBA) error {
	c.b.mustMain("Present")
	var cp *image.RGBA
	if img != nil {
		cp = &image.RGBA{Pix: slices.Clone(img.Pix), Stride: img.Stride, Rect: img.Rect}
	}
	c.mu.Lock()
	c.presents++
	c.last = cp
	c.mu.Unlock()
	return nil
}

// ---- simulation (any goroutine) ----

// frame runs CanvasFrame unless the surface closed. Main thread.
func (c *Canvas) frame() {
	if c.s.Closed() {
		return
	}
	c.mu.Lock()
	c.pending = false
	c.frames++
	c.mu.Unlock()
	c.sink.CanvasFrame()
}

// Frame runs one CanvasFrame on the main thread, as after a vsync. It
// reports false if the surface closed or the loop stopped.
func (c *Canvas) Frame() bool {
	ran := false
	ok := c.b.Do(func() {
		if !c.s.Closed() {
			c.frame()
			ran = true
		}
	})
	return ok && ran
}

// FlushFrame runs a CanvasFrame if Invalidate was called since the last
// frame and reports whether it did.
func (c *Canvas) FlushFrame() bool {
	ran := false
	c.b.Do(func() {
		c.mu.Lock()
		p := c.pending
		c.mu.Unlock()
		if p && !c.s.Closed() {
			c.frame()
			ran = true
		}
	})
	return ran
}

// Input delivers ev to the sink on the main thread.
func (c *Canvas) Input(ev input.Event) bool {
	return c.b.Do(func() {
		if !c.s.Closed() {
			c.sink.CanvasInput(ev)
		}
	})
}

// Focus reports a focus change to the sink.
func (c *Canvas) Focus(focused bool) bool {
	return c.b.Do(func() {
		if !c.s.Closed() {
			c.sink.CanvasFocus(focused)
		}
	})
}

// Resize simulates the user resizing the window to w×h logical pixels at
// scale: the surface reports SurfaceResized to the host and the sink gets
// CanvasResized.
func (c *Canvas) Resize(w, h int, scale float32) bool {
	return c.b.Do(func() {
		if c.s.Closed() {
			return
		}
		c.s.mu.Lock()
		c.s.w, c.s.h = w, h
		c.s.mu.Unlock()
		c.mu.Lock()
		c.scale = scale
		c.mu.Unlock()
		c.b.h.OnSurfaceEvent(c.s.id, platform.SurfaceEvent{Kind: platform.SurfaceResized, Width: w, Height: h})
		c.sink.CanvasResized(float32(w), float32(h), scale)
	})
}

// Surface returns the canvas' surface. Close it, or use
// Backend.RequestClose, to simulate the window closing.
func (c *Canvas) Surface() *Surface { return c.s }

// Frames counts CanvasFrame calls; Presents counts Present calls;
// Invalidates counts Invalidate calls.
func (c *Canvas) Frames() int      { c.mu.Lock(); defer c.mu.Unlock(); return c.frames }
func (c *Canvas) Presents() int    { c.mu.Lock(); defer c.mu.Unlock(); return c.presents }
func (c *Canvas) Invalidates() int { c.mu.Lock(); defer c.mu.Unlock(); return c.invalidates }

// Pending reports whether Invalidate was called since the last frame.
func (c *Canvas) Pending() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.pending }

// InvalidateAts returns every InvalidateAt time, in call order.
func (c *Canvas) InvalidateAts() []time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.invalidateAts)
}

// LastFrame returns a copy of the last presented image (nil if none).
func (c *Canvas) LastFrame() *image.RGBA { c.mu.Lock(); defer c.mu.Unlock(); return c.last }

// LastFrameSize returns the device-pixel size of the last presented image.
func (c *Canvas) LastFrameSize() (w, h int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.last == nil {
		return 0, 0
	}
	return c.last.Rect.Dx(), c.last.Rect.Dy()
}

// Cursor returns the cursor last set.
func (c *Canvas) Cursor() input.Cursor { c.mu.Lock(); defer c.mu.Unlock(); return c.cursor }

// TextInput returns the text-input state last set.
func (c *Canvas) TextInput() (enabled bool, caret geom.Rect) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.textOn, c.caret
}
