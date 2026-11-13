//go:build ios

package ios

// Canvas surfaces on iOS (docs/specs/mobile-canvas.md §1.2, milestone M1):
// a page of the page stack whose content is drawn by package ui instead of
// a WKWebView. The UIKit half is canvas.m (TGCanvasPage, TGCanvasView);
// this file implements platform.Canvas on it.
//
// Frame scheduling follows mobilecanvas.Sched, shared with Android:
//
//   - Invalidate / InvalidateAt may be called from any goroutine. They
//     only record the request; the first one after a frame signals a
//     CFRunLoopSource in the main run loop's common modes (tgc_signal), so
//     it also fires while UIKit tracks a scroll and inside nested run loops
//     that a main-queue block would wait behind.
//   - The source's callback (tangoIOSCanvasWake) lets the scheduler
//     resume the view's CADisplayLink and arm its deadline timer (one
//     CFRunLoopTimer per canvas, for InvalidateAt).
//   - Each display link tick with a pending request runs CanvasFrame; a
//     few idle ticks pause the link, so a still page costs nothing.
//   - layoutSubviews reports size and scale changes: CanvasResized, then a
//     synchronous frame, so a rotation never shows a stretched old frame.
//   - In the background no frame ever runs (iOS terminates apps that
//     submit GPU work there); a request made meanwhile is drawn when the
//     app returns to the foreground.

/*
#cgo LDFLAGS: -framework QuartzCore
#include <stdlib.h>
#include "canvas.h"
*/
import "C"

import (
	"errors"
	"image"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/mobilecanvas"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

var (
	_ platform.CanvasCreator = (*backend)(nil)
	_ platform.Canvas        = (*canvas)(nil)
	_ platform.CanvasMetal   = (*canvas)(nil)
	_ platform.CanvasInsets  = (*canvas)(nil)

	_ platform.CanvasTextInputState = (*canvas)(nil)
)

var errCanvasClosed = errors.New("tango/ios: canvas surface is closed")

// canvas implements platform.Canvas for one TGCanvasView. Fields are
// main-thread only; sched's request methods are safe from any goroutine.
type canvas struct {
	s     *surface
	h     uintptr // handle naming the view in canvas.m
	sink  platform.CanvasSink
	sched mobilecanvas.Sched

	ready   bool // NewCanvasSurface returned: sink callbacks allowed
	closed  bool
	inFrame bool

	w, hgt, scale float32
	safe          geom.Insets

	metal bool // tgc_use_metal succeeded

	textOn bool
	caret  geom.Rect
	doc    textDoc    // the UITextInput document (canvas_text.go)
	traits textTraits // as last applied to the view
	kbd    geom.Insets

	focused   bool // as reported to the sink
	wantFocus bool // as last reported by the view

	touches mobilecanvas.Touches[uintptr]
	clock   mobilecanvas.Clock
	evs     []input.TouchEvent
	keys    map[uint16]input.KeyEvent // HID usage -> its reported press

	ax canvasA11y // accessibility bridge (canvas_a11y.go)

	hover  hoverState // iPad pointer over the view (canvas_pointer.go)
	cursor int        // pointer style last set (pointerStyle)
	ov     *overlay   // set for canvases attached to a WebView page (canvas_attach.go)
}

// Main-thread registry of open canvases by handle.
var (
	canvases  = map[uintptr]*canvas{}
	canvasSeq uintptr
)

// NewCanvasSurface implements platform.CanvasCreator. The page fills the
// screen like a WebView page: Width, Height, Min* and Resizable are
// ignored; it joins the page stack when shown.
func (b *backend) NewCanvasSurface(id platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras, sink platform.CanvasSink) (platform.Surface, platform.Canvas, error) {
	if sink == nil {
		return nil, nil, errors.New("tango/ios: NewCanvasSurface: nil sink")
	}
	canvasSeq++
	c := &canvas{h: canvasSeq, sink: sink, keys: map[uint16]input.KeyEvent{}}
	s := &surface{b: b, id: id, cv: c}
	c.s = s
	title, free := cstring(opts.Title)
	defer free()
	canvases[c.h] = c
	C.tgc_new(C.uintptr_t(c.h), C.uint64_t(id), title)
	c.readGeometry()
	if C.tgc_in_background() != 0 {
		c.apply(c.sched.Background())
	}
	b.surfaces[id] = s
	if opts.Visible {
		s.Show()
	}
	// The sink is never called before NewCanvasSurface returns: the first
	// frame and the focus report come from the wake source.
	c.ready = true
	c.wantFocus = C.tgc_focused(C.uintptr_t(c.h)) != 0
	c.Invalidate()
	startSelftest(c)
	startPointerSelftest(c)
	c.a11yStart()
	return s, c, nil
}

func (c *canvas) handle() C.uintptr_t { return C.uintptr_t(c.h) }

// readGeometry refreshes the cached size, scale and safe area; it reports
// whether size or scale changed.
func (c *canvas) readGeometry() bool {
	var w, h, scale, top, left, bottom, right C.double
	C.tgc_geometry(c.handle(), &w, &h, &scale, &top, &left, &bottom, &right)
	c.safe = mobilecanvas.Clamp(geom.Insets{Top: float32(top), Left: float32(left), Bottom: float32(bottom), Right: float32(right)})
	nw, nh, ns := float32(w), float32(h), float32(scale)
	if ns <= 0 {
		ns = 1
	}
	if nw == c.w && nh == c.hgt && ns == c.scale {
		return false
	}
	c.w, c.hgt, c.scale = nw, nh, ns
	return true
}

// ---- platform.Canvas ----

func (c *canvas) Size() (float32, float32) { return c.w, c.hgt }
func (c *canvas) Scale() float32           { return c.scale }

// Invalidate implements platform.Canvas. Any goroutine.
func (c *canvas) Invalidate() {
	if c.sched.Invalidate() {
		C.tgc_signal()
	}
}

// InvalidateAt implements platform.Canvas. Any goroutine.
func (c *canvas) InvalidateAt(t time.Time) {
	if c.sched.InvalidateAt(t) {
		C.tgc_signal()
	}
}

// SetCursor sets the iPad pointer's shape over the canvas
// (canvas_pointer.go). Phones have no pointer: it does nothing there.
func (c *canvas) SetCursor(cur input.Cursor) {
	if c.closed {
		return
	}
	c.setCursor(cur)
}

// SetTextInput turns text input on or off: the view's UITextInput then
// shows or hides the soft keyboard (canvas_text.go). caret places the
// input method's candidate window.
func (c *canvas) SetTextInput(enabled bool, caret geom.Rect) {
	if c.closed {
		return
	}
	c.setTextInput(enabled, caret)
}

// Present shows a CPU frame (or, after UseMetalLayer, notes that the GPU
// presented when img is nil). The canvas does not implement
// platform.CanvasDamagePresenter: the GPU path keeps its own retained
// target, and the CPU path is only a fallback, so it always shows whole
// frames.
func (c *canvas) Present(img *image.RGBA) error {
	if c.closed {
		return errCanvasClosed
	}
	if img == nil {
		if c.metal {
			C.tgc_hide_cpu(c.handle())
			return nil
		}
		return errors.New("tango/ios: Present: nil image")
	}
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	off := img.PixOffset(r.Min.X, r.Min.Y)
	if n := img.Stride*(h-1) + w*4; off < 0 || off+n > len(img.Pix) {
		return errors.New("tango/ios: Present: image buffer too small")
	}
	// tgc_present copies the pixels before returning.
	if C.tgc_present(c.handle(), unsafe.Pointer(&img.Pix[off]), C.int(w), C.int(h), C.int(img.Stride)) != 0 {
		return errors.New("tango/ios: Present: cannot create the frame image")
	}
	return nil
}

// UseMetalLayer implements platform.CanvasMetal with a CAMetalLayer
// sublayer that fills the view (a UIView's own layer class cannot change
// after creation). The view keeps its frame and contentsScale in step
// before CanvasResized is reported; Present(img) still works and shows
// the image in a sublayer above it until the next Present(nil).
func (c *canvas) UseMetalLayer() (uintptr, error) {
	if c.closed {
		return 0, errCanvasClosed
	}
	l := uintptr(C.tgc_use_metal(c.handle()))
	if l == 0 {
		return 0, errors.New("tango/ios: UseMetalLayer: cannot create a CAMetalLayer")
	}
	if !c.metal {
		c.metal = true
		c.Invalidate() // the first GPU frame follows without an Invalidate
	}
	return l, nil
}

// SafeAreaInsets implements platform.CanvasInsets (view.safeAreaInsets,
// cached when UIKit reports a change).
func (c *canvas) SafeAreaInsets() geom.Insets { return c.safe }

// KeyboardInset implements platform.CanvasInsets: the part of the view
// the soft keyboard covers at the bottom (canvas_text.m
// updateKeyboardInset), cached when UIKit reports a keyboard change.
func (c *canvas) KeyboardInset() geom.Insets { return c.kbd }

// ---- scheduling ----

// apply carries out a scheduler action. Main thread.
func (c *canvas) apply(a mobilecanvas.Action) {
	if c.closed {
		return
	}
	switch a.Link {
	case mobilecanvas.LinkResume:
		C.tgc_link(c.handle(), 1)
	case mobilecanvas.LinkPause:
		C.tgc_link(c.handle(), 0)
	}
	if a.Arm {
		in := C.double(-1)
		if !a.At.IsZero() {
			in = C.double(max(time.Until(a.At).Seconds(), 0))
		}
		C.tgc_timer(c.handle(), in)
	}
	if a.Frame {
		c.frame()
	}
}

// frame runs CanvasFrame. A frame requested from inside one (the sink
// asked for another) waits for the next tick.
func (c *canvas) frame() {
	if c.closed || !c.ready {
		return
	}
	if c.inFrame {
		c.Invalidate()
		return
	}
	c.inFrame = true
	defer func() { c.inFrame = false }()
	c.sink.CanvasFrame()
}

// updateFocus reports CanvasFocus when the view's focus changed. Losing
// it cancels the fingers still down first (touch-input.md §2).
func (c *canvas) updateFocus() {
	if !c.ready || c.closed || c.wantFocus == c.focused {
		return
	}
	if !c.wantFocus {
		c.cancelTouches()
		c.releaseKeys()
	}
	c.focused = c.wantFocus
	c.sink.CanvasFocus(c.focused)
}

// lookup returns the open canvas with handle h, or nil.
func lookup(h C.uintptr_t) *canvas {
	c := canvases[uintptr(h)]
	if c == nil || c.closed {
		return nil
	}
	return c
}

//export tangoIOSCanvasWake
func tangoIOSCanvasWake() {
	now := time.Now()
	for _, c := range canvases {
		if c.closed {
			continue
		}
		c.updateFocus()
		c.apply(c.sched.Wake(now))
	}
}

//export tangoIOSCanvasTick
func tangoIOSCanvasTick(h C.uintptr_t) {
	if c := lookup(h); c != nil {
		c.apply(c.sched.Tick(time.Now()))
	}
}

//export tangoIOSCanvasTimer
func tangoIOSCanvasTimer(h C.uintptr_t) {
	if c := lookup(h); c != nil {
		c.apply(c.sched.TimerFired(time.Now()))
	}
}

//export tangoIOSCanvasLayout
func tangoIOSCanvasLayout(h C.uintptr_t, w, hgt, scale C.double) {
	c := lookup(h)
	if c == nil || !c.readGeometry() || !c.ready {
		return
	}
	c.sink.CanvasResized(c.w, c.hgt, c.scale)
	if c.inFrame {
		c.Invalidate()
		return
	}
	c.apply(c.sched.SyncFrame()) // refused in the background
}

//export tangoIOSCanvasInsets
func tangoIOSCanvasInsets(h C.uintptr_t, top, left, bottom, right C.double) {
	c := lookup(h)
	if c == nil {
		return
	}
	in := mobilecanvas.Clamp(geom.Insets{Top: float32(top), Left: float32(left), Bottom: float32(bottom), Right: float32(right)})
	if in == c.safe {
		return
	}
	c.safe = in
	if c.ready {
		c.Invalidate() // the runner polls SafeAreaInsets every frame
	}
}

//export tangoIOSCanvasFocus
func tangoIOSCanvasFocus(h C.uintptr_t, focused C.int) {
	c := lookup(h)
	if c == nil {
		return
	}
	c.wantFocus = focused != 0
	c.updateFocus() // before NewCanvasSurface returns, the wake source reports it
}

//export tangoIOSCanvasBackground
func tangoIOSCanvasBackground(bg C.int) {
	now := time.Now()
	for _, c := range canvases {
		if c.closed {
			continue
		}
		if bg != 0 {
			c.apply(c.sched.Background())
		} else {
			c.apply(c.sched.Foreground(now))
		}
	}
}

// ---- touches ----

func (c *canvas) sample(s *C.tgc_sample) mobilecanvas.Sample {
	return mobilecanvas.Sample{
		Pos:      geom.Point{X: float32(s.x), Y: float32(s.y)},
		Pressure: mobilecanvas.Pressure(float32(s.force), float32(s.maxForce)),
		Time:     c.clock.Time(mobilecanvas.Seconds(float64(s.time))),
	}
}

//export tangoIOSCanvasTouch
func tangoIOSCanvasTouch(h C.uintptr_t, phase C.int, key C.uintptr_t, ps *C.tgc_sample, n C.int) {
	c := lookup(h)
	if c == nil || n <= 0 {
		return
	}
	raw := unsafe.Slice(ps, int(n))
	cur := c.sample(&raw[n-1])
	k := uintptr(key)
	evs := c.evs[:0]
	switch phase {
	case C.TGC_TOUCH_BEGAN:
		evs = c.touches.Begin(evs, k, cur)
	case C.TGC_TOUCH_MOVED:
		hist := make([]mobilecanvas.Sample, 0, n-1)
		for i := range raw[:n-1] {
			hist = append(hist, c.sample(&raw[i]))
		}
		evs = c.touches.Move(evs, k, hist, cur)
	case C.TGC_TOUCH_ENDED:
		evs = c.touches.End(evs, k, cur)
	case C.TGC_TOUCH_CANCELLED:
		evs = c.touches.Cancel(evs, k, cur)
	}
	c.evs = evs[:0]
	c.deliver(evs)
}

func (c *canvas) deliver(evs []input.TouchEvent) {
	if !c.ready {
		return
	}
	for _, ev := range evs {
		if c.closed {
			return
		}
		c.sink.CanvasInput(ev)
	}
}

// cancelTouches cancels every finger still down.
func (c *canvas) cancelTouches() {
	evs := c.touches.CancelAll(c.evs[:0], time.Now())
	c.evs = evs[:0]
	c.deliver(evs)
}

// ---- hardware keys ----

//export tangoIOSCanvasPress
func tangoIOSCanvasPress(h C.uintptr_t, down C.int, usage C.int, flags C.int64_t, chars, charsIgnoringMods *C.char) C.int {
	c := lookup(h)
	if c == nil || !c.ready {
		return 0
	}
	u := uint16(usage)
	if down == 0 {
		ev, ok := c.keys[u]
		if !ok {
			return 0 // its press went to UIKit: so does the release
		}
		delete(c.keys, u)
		c.sink.CanvasInput(releaseEvent(ev, int64(flags)))
		return 1
	}
	p := decodePress(u, int64(flags), C.GoString(chars), C.GoString(charsIgnoringMods), c.textOn, c.doc.ime.Marked())
	if p.route == routeSystem || p.route == routeText {
		return 0 // UIKit: routeText types through UITextInput
	}
	if prev, ok := c.keys[u]; ok { // a release we never saw
		c.sink.CanvasInput(releaseEvent(prev, int64(flags)))
	}
	c.keys[u] = p.ev
	c.sink.CanvasInput(p.ev)
	return 1
}

// releaseKeys reports every key still down as released (focus lost).
func (c *canvas) releaseKeys() {
	for u, ev := range c.keys {
		delete(c.keys, u)
		c.sink.CanvasInput(input.KeyEvent{Key: ev.Key, Code: ev.Code})
	}
}

// ---- teardown ----

// destroy runs from surface.Close: the page leaves the stack and the
// sink is never called again.
func (c *canvas) destroy() {
	if c.closed {
		return
	}
	c.sched.Close()
	c.closed = true
	C.tgc_destroy(c.handle())
	delete(canvases, c.h)
}
