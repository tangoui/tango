//go:build linux && !android

package linux

// Canvas widget (docs/specs/canvas-surface.md, step B): a GtkDrawingArea
// whose content is drawn by Tango's own UI. Frames rendered by the core
// arrive in Present as premultiplied RGBA and are copied into a cairo
// ARGB32 image surface owned by the canvas; the "draw" signal paints it.
// Input handling (pointer, scroll, keys, IME, focus) is in
// canvas_input_linux.go.
//
// NewCanvasSurface (platform.CanvasCreator) builds the window with the
// same newWindow / finishSurface as WebView surfaces (surface_linux.go) and
// packs the widget from newCanvasWidget where the web view would go.
//
// Frame scheduling mirrors the darwin backend:
//
//   - Invalidate / InvalidateAt may be called from any goroutine. They
//     record the request under canvasQueue.mu and add one GLib idle source
//     (g_idle_add_full is thread safe) that runs canvasPerform on the main
//     loop; further requests coalesce until it runs.
//   - requestFrame installs a GdkFrameClock tick callback
//     (gtk_widget_add_tick_callback). On the next tick it queues a redraw,
//     which GTK paints in the same frame clock cycle, so frames follow
//     vsync and several requests make one frame. After a few idle ticks the
//     callback removes itself: a still window costs nothing.
//   - InvalidateAt keeps one g_timeout source per canvas, re-armed to the
//     earliest pending deadline; when it fires it goes through
//     requestFrame. Any frame satisfies a pending deadline.
//   - Size and scale changes ("size-allocate", "notify::scale-factor")
//     report CanvasResized and redraw; the draw handler asks the sink for
//     a frame first, so a resize never shows a stale-size frame for long.
//   - The sink is never called before start: a draw that happens earlier
//     is remembered and start asks for a frame.

import (
	"errors"
	"image"
	"math"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

var (
	_ platform.CanvasCreator = (*backend)(nil)
	_ platform.Canvas        = (*canvas)(nil)
)

var canvasErrClosed = errors.New("tango/linux: canvas is closed")

// canvasTimerTolerance lets a timer that fires a hair early count as due.
const canvasTimerTolerance = time.Millisecond

// canvas implements platform.Canvas for one GtkDrawingArea. Fields are
// main-thread only unless noted.
type canvas struct {
	widget ptr // GtkDrawingArea; we hold a reference until "destroy"
	sink   platform.CanvasSink
	im     ptr // GtkIMMulticontext

	ready  bool // start was called; sink callbacks allowed
	closed bool // the widget was destroyed; nothing reaches the sink
	w, h   float64
	scale  float64

	// Cursor (gdk_window_set_cursor persists on the widget's GdkWindow).
	cursor     input.Cursor
	cursors    map[input.Cursor]ptr // GdkCursor cache; 0 = inherit
	applied    input.Cursor
	appliedWin ptr // GdkWindow the applied cursor was set on
	inside     bool

	// Keyboard and text input (canvas_input_linux.go).
	textOn    bool
	caret     geom.Rect
	imFocused bool
	marked    string // IME pre-edit
	markedCur int    // caret within marked, runes
	downKeys  map[input.Key]bool
	clicks    canvasClickCounter
	focused   bool // last focus reported to the sink

	// File drop (canvas_drop_linux.go).
	dropOn    bool // a drag destination is set: the overlay (glOverlay) when there is one, else the widget
	dropHover bool // CanvasDragOver reported, CanvasDragLeave pending

	// Frame scheduling.
	sched      canvasFrameSched
	tickID     uint32 // gtk_widget_add_tick_callback id, 0 when none
	drawWanted bool   // the next draw must ask the sink for a frame
	timerSrc   uint32 // g_timeout source for timerAt, 0 when none
	timerAt    time.Time
	inFrame    bool
	inDraw     bool // inside the "draw" handler
	missed     bool // a draw happened before start

	// The frame being shown: a cairo ARGB32 image surface.
	surf         ptr
	surfW, surfH int

	ax *canvasA11y // AT-SPI bridge (canvas_a11y_linux.go); nil until used

	// GPU presentation (canvas_gl_linux.go).
	owner     *surface // the canvas surface; nil for attached canvases
	tid       int      // the main thread (gettid), where newCanvasWidget ran
	glOverlay ptr      // GtkOverlay around the drawing area; 0 when not wrapped
	glArea    ptr      // GtkGLArea after UseGLArea; we hold a reference
	gl        canvasGL

	// Guarded by canvasQueue.mu.
	queued bool
	qFrame bool
	qAt    time.Time
	qFocus bool
}

// canvasQueue carries requests from any goroutine to the main thread.
var canvasQueue struct {
	mu    sync.Mutex
	dirty []*canvas
}

var (
	canvasPerformOnce sync.Once
	canvasPerformID   ptr // permanent dispatch entry running canvasPerform
)

// NewCanvasSurface implements platform.CanvasCreator. Main thread.
func (b *backend) NewCanvasSurface(id platform.SurfaceID, o platform.SurfaceOptions, ex platform.SurfaceExtras, sink platform.CanvasSink) (platform.Surface, platform.Canvas, error) {
	if sink == nil {
		return nil, nil, errors.New("tango/linux: NewCanvasSurface: nil sink")
	}
	if !b.loaded.Load() {
		return nil, nil, errors.New("tango/linux: NewCanvasSurface before Init")
	}
	if ex.Bounds.Width > 0 { // as NewSurfaceEx
		o.Width = ex.Bounds.Width
	}
	if ex.Bounds.Height > 0 {
		o.Height = ex.Bounds.Height
	}
	c, widget, err := newCanvasWidget(sink)
	if err != nil {
		return nil, nil, err
	}
	packed := c.wrapGL() // the GtkOverlay for UseGLArea (canvas_gl_linux.go), or widget
	s, err := b.newWindow(id, o)
	if err != nil {
		gtkWidgetDestroy(packed)
		return nil, nil, err
	}
	gtkBoxPackStart(s.box, packed, true, true, 0)
	s.content, s.canvas = widget, c
	c.owner = s
	b.finishSurface(s, &ex)
	if o.Visible {
		gtkWindowPresent(s.win)
	}
	// The sink is first called from the main loop: start only queues the
	// first frame and focus report.
	c.start()
	return s, c, nil
}

// newCanvasWidget creates a canvas widget. It returns the canvas, the
// GtkWidget* to pack into a container, or an error when the native
// symbols are unavailable. Main thread, after Init.
//
// The widget is a normal GTK widget: it lives until gtk_widget_destroy (on
// it or on the window containing it), which also tears the canvas down.
// A caller that fails before packing it must destroy it.
func newCanvasWidget(sink platform.CanvasSink) (*canvas, ptr, error) {
	if sink == nil {
		return nil, 0, errors.New("tango/linux: newCanvasWidget: nil sink")
	}
	if unsafe.Sizeof(ptr(0)) != 8 {
		return nil, 0, errors.New("tango/linux: canvas surfaces need a 64-bit target")
	}
	if err := loadCanvasFFI(); err != nil {
		return nil, 0, err
	}
	initCallbacks()
	canvasPerformOnce.Do(func() {
		canvasPerformID = dispatch.add(func(_, _ ptr) ptr {
			canvasPerform()
			return 0 // G_SOURCE_REMOVE; the entry is permanent (no notify)
		})
	})

	f := &canvasFFI
	w := newCanvasArea() // GtkDrawingArea subclass exposing the AT-SPI tree (canvas_a11y_linux.go)
	if w == 0 {
		return nil, 0, errors.New("tango/linux: gtk_drawing_area_new failed")
	}
	f.objectRefSink(w) // our reference, dropped in onDestroy
	c := &canvas{
		widget:     w,
		sink:       sink,
		scale:      1,
		cursors:    map[input.Cursor]ptr{},
		downKeys:   map[input.Key]bool{},
		drawWanted: true,
		tid:        syscall.Gettid(),
	}
	f.widgetSetCanFocus(w, true)
	f.widgetAddEvents(w, canvasEventMask)
	f.widgetSetHexpand(w, true)
	f.widgetSetVexpand(w, true)

	connect(w, "draw", 1, c.onDraw)
	connect(w, "size-allocate", 1, c.onGeometry)
	connect(w, "notify::scale-factor", 1, c.onGeometry)
	connect(w, "realize", 0, c.onRealize)
	connect(w, "unrealize", 0, c.onUnrealize)
	connect(w, "destroy", 0, c.onDestroy)
	c.connectInput()

	c.w, c.h, c.scale = c.geometry()
	return c, w, nil
}

// start enables sink callbacks. The caller has put the widget into its
// window (and shown it when visible). It takes keyboard focus within the
// window and arranges for the first frame and focus report to come from
// the main loop, never from inside start.
func (c *canvas) start() {
	if c.closed || c.ready {
		return
	}
	c.w, c.h, c.scale = c.geometry()
	canvasFFI.widgetGrabFocus(c.widget)
	c.ready = true
	c.enqueue(func(c *canvas) {
		c.qFocus = true
		c.qFrame = c.qFrame || c.missed || c.drawWanted
	})
}

// geometry reads the allocated logical size and the integer scale factor.
func (c *canvas) geometry() (w, h, scale float64) {
	f := &canvasFFI
	w = float64(max(f.widgetGetAllocatedWidth(c.widget), 0))
	h = float64(max(f.widgetGetAllocatedHeight(c.widget), 0))
	scale = float64(f.widgetGetScaleFactor(c.widget))
	if scale < 1 {
		scale = 1
	}
	return w, h, scale
}

// checkGeometry reports a size or scale change and marks the next draw as
// needing a frame; it returns whether there was one.
func (c *canvas) checkGeometry() bool {
	if c.closed {
		return false
	}
	w, h, scale := c.geometry()
	if w == c.w && h == c.h && scale == c.scale {
		return false
	}
	c.w, c.h, c.scale = w, h, scale
	c.drawWanted = true
	if c.ready {
		c.sink.CanvasResized(float32(w), float32(h), float32(scale))
	}
	return true
}

// ---- platform.Canvas ----

func (c *canvas) Size() (float32, float32) { return float32(c.w), float32(c.h) }
func (c *canvas) Scale() float32           { return float32(c.scale) }

// enqueue records a request for the main thread. Any goroutine.
func (c *canvas) enqueue(set func(c *canvas)) {
	canvasQueue.mu.Lock()
	set(c)
	signal := !c.queued
	if signal {
		c.queued = true
		canvasQueue.dirty = append(canvasQueue.dirty, c)
	}
	canvasQueue.mu.Unlock()
	if signal {
		gIdleAddFull(gPriorityDefault, cbData1, canvasPerformID, 0)
	}
}

// Invalidate implements platform.Canvas. Any goroutine.
func (c *canvas) Invalidate() { c.enqueue(func(c *canvas) { c.qFrame = true }) }

// InvalidateAt implements platform.Canvas. Any goroutine.
func (c *canvas) InvalidateAt(t time.Time) {
	c.enqueue(func(c *canvas) {
		if c.qAt.IsZero() || t.Before(c.qAt) {
			c.qAt = t
		}
	})
}

// canvasPerform runs queued requests on the main thread.
func canvasPerform() {
	canvasQueue.mu.Lock()
	list := canvasQueue.dirty
	canvasQueue.dirty = nil
	type req struct {
		c     *canvas
		frame bool
		at    time.Time
		focus bool
	}
	reqs := make([]req, len(list))
	for i, c := range list {
		reqs[i] = req{c, c.qFrame, c.qAt, c.qFocus}
		c.queued, c.qFrame, c.qAt, c.qFocus = false, false, time.Time{}, false
	}
	canvasQueue.mu.Unlock()
	for _, r := range reqs {
		c := r.c
		if c.closed {
			continue
		}
		if r.focus {
			c.focusMaybeChanged()
		}
		if !r.at.IsZero() {
			if c.timerAt.IsZero() || r.at.Before(c.timerAt) {
				c.timerAt = r.at
				c.armTimer()
			}
		}
		if r.frame {
			c.requestFrame()
		}
	}
}

// requestFrame asks for a CanvasFrame at the next frame clock tick.
func (c *canvas) requestFrame() {
	if c.closed {
		return
	}
	if c.sched.request() {
		c.tickID = canvasFFI.widgetAddTickCallback(c.widget, cbArg3, dispatch.add(c.onTick), cbFree)
	}
}

// onTick is the GtkTickCallback (widget, frame_clock): it runs in the
// frame clock's update phase, before layout and paint of the same frame.
func (c *canvas) onTick(_, _ ptr) ptr {
	if c.closed {
		c.sched.stopped()
		c.tickID = 0
		return 0
	}
	draw, keep := c.sched.tick()
	if draw {
		queue, now := c.gl.tick(c.checkGeometry(), c.drawWanted, c.ready)
		switch {
		case queue:
			// A size change (or the first frame) renders inside the draw
			// handler, so the frame matches the allocation it paints.
			c.drawWanted = true
			canvasFFI.widgetQueueDraw(c.widget)
		case now:
			// Render now, in the frame clock's update phase: Present queues
			// a redraw of what changed (all of it, or only the damage of
			// a partial frame, canvas_damage_linux.go; on the GL path the
			// presenter queues a render of the GLArea), painted in the
			// same frame clock cycle.
			if c.gl.on {
				c.drawWanted = false
			}
			c.frame()
		}
	}
	if !keep {
		c.tickID = 0 // GTK removes it and calls cbFree
	}
	return gbool(keep)
}

// armTimer (re)sets the single timeout source to timerAt.
func (c *canvas) armTimer() {
	if c.timerSrc != 0 {
		canvasFFI.sourceRemove(c.timerSrc) // its notify drops the dispatch entry
		c.timerSrc = 0
	}
	if c.timerAt.IsZero() || c.closed {
		return
	}
	ms := canvasTimeoutMS(time.Now(), c.timerAt)
	c.timerSrc = gTimeoutAddFull(gPriorityDefault, ms, cbData1, dispatch.add(c.onTimer), cbFree)
}

func (c *canvas) onTimer(_, _ ptr) ptr {
	c.timerSrc = 0 // returning G_SOURCE_REMOVE below
	if c.closed || c.timerAt.IsZero() {
		return 0
	}
	if !time.Now().Add(canvasTimerTolerance).Before(c.timerAt) {
		c.timerAt = time.Time{}
		c.requestFrame()
	} else {
		c.armTimer() // fired early (ms rounding): wait for the rest
	}
	return 0
}

// onDraw handles "draw" (widget, cairo_t*). When a frame is wanted the
// sink renders it first (Present copies into c.surf without queueing
// another draw); then the current frame is painted.
func (c *canvas) onDraw(_, cr ptr) ptr {
	if c.closed {
		return 0
	}
	c.inDraw = true
	defer func() { c.inDraw = false }()
	if c.checkGeometry() && !c.gl.drawFrames() {
		c.requestFrame() // GL path: frames never run in here
	}
	if c.drawWanted && c.gl.drawFrames() {
		c.drawWanted = false
		c.sched.drew()
		c.frame()
	}
	if c.surf != 0 && c.gl.paintSurface() {
		canvasFFI.setSourceSurface(cr, c.surf, 0, 0)
		canvasFFI.paint(cr) // clipped by GTK to the invalid region
	}
	return 1 // handled: skip the drawing area's default background
}

// frame asks the sink for a frame.
func (c *canvas) frame() {
	if c.closed || c.inFrame {
		return
	}
	if !c.timerAt.IsZero() {
		c.timerAt = time.Time{} // this frame satisfies the deadline
		c.armTimer()
	}
	if !c.ready {
		c.missed = true // start asks for a frame
		return
	}
	c.inFrame = true
	defer func() { c.inFrame = false }()
	c.sink.CanvasFrame()
}

// onGeometry handles "size-allocate" and "notify::scale-factor".
func (c *canvas) onGeometry(_, _ ptr) ptr {
	if c.checkGeometry() {
		if c.gl.resizeFrame() {
			c.resizeGL() // canvas_gl_linux.go
		} else {
			canvasFFI.widgetQueueDraw(c.widget)
		}
	}
	return 0
}

func (c *canvas) onRealize(_, _ ptr) ptr {
	if c.closed {
		return 0
	}
	if c.im != 0 {
		canvasFFI.imSetClientWindow(c.im, canvasFFI.widgetGetWindow(c.widget))
	}
	c.appliedWin = 0
	c.applyCursor()
	return 0
}

func (c *canvas) onUnrealize(_, _ ptr) ptr {
	if c.im != 0 {
		canvasFFI.imSetClientWindow(c.im, 0)
	}
	c.appliedWin = 0
	return 0
}

// onDestroy handles the widget's "destroy" (it or its window destroyed).
func (c *canvas) onDestroy(_, _ ptr) ptr {
	c.destroy()
	return 0
}

// destroy tears the canvas down; later calls do nothing. The surface calls
// it from its window's "destroy", before reporting SurfaceClosed and
// before GTK destroys the widget itself.
func (c *canvas) destroy() {
	if c.closed {
		return
	}
	c.closed = true
	c.a11yDestroy() // canvas_a11y_linux.go
	f := &canvasFFI
	if c.tickID != 0 {
		f.widgetRemoveTickCallback(c.widget, c.tickID)
		c.tickID = 0
	}
	c.sched.stopped()
	if c.timerSrc != 0 {
		f.sourceRemove(c.timerSrc)
		c.timerSrc = 0
	}
	if c.im != 0 {
		if c.imFocused {
			f.imFocusOut(c.im)
			c.imFocused = false
		}
		f.imSetClientWindow(c.im, 0)
		gObjectUnref(c.im) // finalizing it disconnects its handlers
		c.im = 0
	}
	for k, cur := range c.cursors {
		if cur != 0 {
			gObjectUnref(cur)
		}
		delete(c.cursors, k)
	}
	if c.surf != 0 {
		f.surfaceDestroy(c.surf)
		c.surf = 0
	}
	if c.glArea != 0 {
		gObjectUnref(c.glArea) // the reference taken in UseGLArea; the overlay holds one until GTK destroys it
		c.glArea = 0
	}
	gObjectUnref(c.widget) // the reference taken in newCanvasWidget; the container still holds one
}

// editCommand reports an Edit menu action (whose accelerator GtkWindow
// handled before the canvas saw the key) as the matching Ctrl shortcut.
func (c *canvas) editCommand(cmd string) {
	k, m, ok := canvasEditKey(cmd)
	if !ok || c.closed {
		return
	}
	c.input(input.KeyEvent{Key: k, Down: true, Mods: m})
	c.input(input.KeyEvent{Key: k, Down: false, Mods: m})
}

// SetCursor implements platform.Canvas. Main thread.
func (c *canvas) SetCursor(cur input.Cursor) {
	if c.closed {
		return
	}
	c.cursor = cur
	c.applyCursor()
}

// applyCursor sets the cursor on the widget's GdkWindow when it changed.
// The GdkWindow keeps it, so it shows whenever the pointer is over the
// canvas without re-applying on every motion.
func (c *canvas) applyCursor() {
	f := &canvasFFI
	if !f.widgetGetRealized(c.widget) {
		return
	}
	win := f.widgetGetWindow(c.widget)
	if win == 0 || (win == c.appliedWin && c.applied == c.cursor) {
		return
	}
	f.windowSetCursor(win, c.gdkCursor(c.cursor))
	c.applied, c.appliedWin = c.cursor, win
}

// gdkCursor returns the cached GdkCursor for cur, or 0 to inherit the
// parent's (default arrow) cursor.
func (c *canvas) gdkCursor(cur input.Cursor) ptr {
	if p, ok := c.cursors[cur]; ok {
		return p
	}
	var p ptr
	if cur != input.CursorDefault {
		display := canvasFFI.widgetGetDisplay(c.widget)
		for _, name := range canvasCursorNames(cur) {
			if p = canvasFFI.cursorNewFromName(display, name); p != 0 {
				break
			}
		}
	}
	c.cursors[cur] = p
	return p
}

// Present implements platform.Canvas: it copies img into the canvas's
// cairo surface and, outside a draw, queues one. Main thread. After
// UseGLArea, Present(nil) reports a GPU frame and Present(img) hides the
// GLArea while CPU frames come (canvas_gl_linux.go).
func (c *canvas) Present(img *image.RGBA) error {
	if c.closed {
		return canvasErrClosed
	}
	if img == nil {
		if c.gl.on {
			c.presentGL(false)
			return nil
		}
		return errors.New("tango/linux: Present: nil image")
	}
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	off := img.PixOffset(r.Min.X, r.Min.Y)
	n := img.Stride*(h-1) + w*4
	if off < 0 || off+n > len(img.Pix) {
		return errors.New("tango/linux: Present: image buffer too small")
	}
	if w > math.MaxInt16 || h > math.MaxInt16 { // cairo's image size limit is 32767
		return errors.New("tango/linux: Present: image too large")
	}
	f := &canvasFFI
	if c.surf == 0 || c.surfW != w || c.surfH != h {
		if c.surf != 0 {
			f.surfaceDestroy(c.surf)
			c.surf = 0
		}
		s := f.imageSurfaceCreate(canvasCairoFormatARGB32, int32(w), int32(h))
		if s == 0 || f.surfaceStatus(s) != 0 { // CAIRO_STATUS_SUCCESS
			if s != 0 {
				f.surfaceDestroy(s) // error surfaces are inert but still counted
			}
			return errors.New("tango/linux: Present: cairo_image_surface_create failed")
		}
		c.surf, c.surfW, c.surfH = s, w, h
	}
	stride := int(f.imageSurfaceGetStride(c.surf))
	data := f.imageSurfaceGetData(c.surf)
	if data == 0 || stride < int(f.formatStrideForWidth(canvasCairoFormatARGB32, int32(w))) {
		return errors.New("tango/linux: Present: unexpected cairo surface layout")
	}
	f.surfaceFlush(c.surf) // finish pending cairo access before writing directly
	dst := unsafe.Slice((*byte)(toPtr(data)), stride*h)
	canvasRGBAToARGB32(dst, stride, img.Pix[off:off+n], img.Stride, w, h)
	f.surfaceMarkDirty(c.surf)
	f.surfaceSetDeviceScale(c.surf, c.scale, c.scale)
	c.presentGL(true)
	if !c.inDraw {
		f.widgetQueueDraw(c.widget)
	}
	return nil
}
