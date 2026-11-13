package fake

// CanvasAttacher on Backend: canvases placed inside a fake WebView surface
// (internal/platform/canvas.go, step D of the canvas spec). An attached
// canvas behaves like the Canvas of a canvas surface (Frame, Input, ...
// drive its sink) and also records its layer and Detach calls.

import (
	"errors"
	"image"
	"sync"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

var (
	_ platform.CanvasAttacher = (*Backend)(nil)
	_ platform.AttachedCanvas = (*AttachedCanvas)(nil)
)

// ErrDetached is returned by Present on a detached canvas.
var ErrDetached = errors.New("fake: canvas detached")

var attachedCfgs sync.Map // *Backend -> *attachedCfg

type attachedCfg struct {
	mu   sync.Mutex
	list map[platform.SurfaceID][]*AttachedCanvas
}

func (b *Backend) attachedCfg() *attachedCfg {
	c, _ := attachedCfgs.LoadOrStore(b, &attachedCfg{list: map[platform.SurfaceID][]*AttachedCanvas{}})
	return c.(*attachedCfg)
}

// AttachCanvas implements platform.CanvasAttacher. The surface must be an
// open WebView surface (not a canvas surface). The first CanvasFrame comes
// as a queued native event, as when a real view is first displayed.
func (b *Backend) AttachCanvas(id platform.SurfaceID, layer platform.CanvasLayer, sink platform.CanvasSink) (platform.Canvas, error) {
	b.mustMain("AttachCanvas")
	s := b.Surface(id)
	if s == nil || s.Closed() {
		return nil, errors.New("fake: AttachCanvas: no open surface")
	}
	if b.Canvas(id) != nil {
		return nil, errors.New("fake: AttachCanvas: not a WebView surface")
	}
	if sink == nil {
		return nil, errors.New("fake: AttachCanvas: nil sink")
	}
	cfg := b.canvasCfg()
	cfg.mu.Lock()
	scale := cfg.scale
	cfg.mu.Unlock()
	ac := &AttachedCanvas{Canvas: &Canvas{b: b, s: s, scale: scale}, layer: layer}
	ac.Canvas.sink = attachedSink{ac, sink}
	reg := b.attachedCfg()
	reg.mu.Lock()
	reg.list[id] = append(reg.list[id], ac)
	reg.mu.Unlock()
	b.enqueue(ac.Canvas.frame)
	return ac, nil
}

// Attached returns the canvases attached to surface id, in attach order
// (detached ones included).
func (b *Backend) Attached(id platform.SurfaceID) []*AttachedCanvas {
	c := b.attachedCfg()
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*AttachedCanvas(nil), c.list[id]...)
}

// AttachedCanvas is a fake platform.AttachedCanvas. Its Canvas methods
// (Frame, Input, Focus, Resize, ...) do nothing once it is detached.
type AttachedCanvas struct {
	*Canvas

	amu      sync.Mutex
	layer    platform.CanvasLayer
	layers   int // SetLayer calls
	detached bool
	detaches int
}

// attachedSink drops callbacks after Detach.
type attachedSink struct {
	c    *AttachedCanvas
	sink platform.CanvasSink
}

func (s attachedSink) CanvasFrame() {
	if !s.c.Detached() {
		s.sink.CanvasFrame()
	}
}

func (s attachedSink) CanvasInput(ev input.Event) {
	if !s.c.Detached() {
		s.sink.CanvasInput(ev)
	}
}

func (s attachedSink) CanvasResized(w, h, scale float32) {
	if !s.c.Detached() {
		s.sink.CanvasResized(w, h, scale)
	}
}

func (s attachedSink) CanvasFocus(f bool) {
	if !s.c.Detached() {
		s.sink.CanvasFocus(f)
	}
}

// Size is the layer's bounds, or the surface's size for zero bounds.
func (c *AttachedCanvas) Size() (w, h float32) {
	c.amu.Lock()
	b := c.layer.Bounds
	c.amu.Unlock()
	if b.Width > 0 && b.Height > 0 {
		return float32(b.Width), float32(b.Height)
	}
	return c.Canvas.Size()
}

// Present records img unless the canvas is detached.
func (c *AttachedCanvas) Present(img *image.RGBA) error {
	if c.Detached() {
		c.b.mustMain("Present")
		return ErrDetached
	}
	return c.Canvas.Present(img)
}

// SetLayer implements platform.AttachedCanvas: it records layer and, when
// the size changed, reports CanvasResized.
func (c *AttachedCanvas) SetLayer(layer platform.CanvasLayer) error {
	c.b.mustMain("SetLayer")
	if c.Detached() {
		return ErrDetached
	}
	ow, oh := c.Size()
	c.amu.Lock()
	c.layer = layer
	c.layers++
	c.amu.Unlock()
	if w, h := c.Size(); w != ow || h != oh {
		c.Canvas.sink.CanvasResized(w, h, c.Scale())
	}
	return nil
}

// Detach implements platform.AttachedCanvas.
func (c *AttachedCanvas) Detach() {
	c.b.mustMain("Detach")
	c.amu.Lock()
	defer c.amu.Unlock()
	c.detaches++
	c.detached = true
}

// Layer returns the current layer.
func (c *AttachedCanvas) Layer() platform.CanvasLayer {
	c.amu.Lock()
	defer c.amu.Unlock()
	return c.layer
}

// SetLayers counts SetLayer calls.
func (c *AttachedCanvas) SetLayers() int { c.amu.Lock(); defer c.amu.Unlock(); return c.layers }

// Detached reports whether Detach was called.
func (c *AttachedCanvas) Detached() bool { c.amu.Lock(); defer c.amu.Unlock(); return c.detached }

// Detaches counts Detach calls.
func (c *AttachedCanvas) Detaches() int { c.amu.Lock(); defer c.amu.Unlock(); return c.detaches }
