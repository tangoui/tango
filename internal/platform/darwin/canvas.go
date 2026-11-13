//go:build darwin && !ios

package darwin

// Canvas surfaces (docs/specs/canvas-surface.md, step A): a normal Tango
// window whose content view is a TangoCanvasView instead of a WKWebView.
// Frames rendered by the core arrive in Present as premultiplied RGBA and
// become the contents of the view's backing CALayer as a CGImage.
//
// Frame scheduling. Everything that wants a frame ends in requestFrame on
// the main thread, which allows at most one frame per display refresh:
//
//   - Invalidate / InvalidateAt may be called from any goroutine. They only
//     record the request under canvasQueue.mu and signal a run loop source
//     (thread-safe CF calls); the source's perform callback runs on the
//     main thread in the common modes, so it also fires during live resize
//     and menu tracking.
//   - requestFrame unpauses the view's CADisplayLink (NSView.displayLink,
//     macOS 14+). Each tick marks the view as needing display when a frame
//     is wanted, then AppKit calls updateLayer in the same run loop pass,
//     which calls CanvasFrame. A link idle for a few ticks pauses itself,
//     so a still window costs nothing. Without a display link a 16 ms
//     timer throttles instead.
//   - InvalidateAt arms one CFRunLoopTimer per canvas, re-set to the
//     earliest pending deadline; when it fires it goes through
//     requestFrame too. Any frame satisfies a pending deadline.
//   - Size and scale changes (setFrameSize:, viewDidChangeBackingProperties)
//     report CanvasResized and redraw synchronously, so live resize does
//     not show stale or stretched content.
//   - After UseMetalLayer the view hosts a CAMetalLayer and AppKit no
//     longer calls updateLayer: setNeedsDisplay then runs the frame
//     directly (canvas_metal.go), so the paths above stay the same.

import (
	"errors"
	"image"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

var (
	_ platform.CanvasCreator = (*backend)(nil)
	_ platform.Canvas        = (*canvas)(nil)
)

const (
	kCGImageAlphaPremultipliedLast = 1
	kCGBitmapByteOrder32Big        = 4 << 12

	nsViewLayerContentsRedrawOnSetNeedsDisplay = 1

	frameInterval  = 16 * time.Millisecond // fallback pacing without a display link
	linkIdleTicks  = 4                     // pause the display link after this many idle ticks
	farFuture      = 1e12                  // CFAbsoluteTime "never"
	timerTolerance = time.Millisecond
)

const isAMD64 = runtime.GOARCH == "amd64"

var errCanvasClosed = errors.New("tango/darwin: canvas surface is closed")

// canvas implements platform.Canvas for one TangoCanvasView. Fields are
// main-thread only unless noted.
type canvas struct {
	s    *surface
	view id
	sink platform.CanvasSink
	key  uintptr // identifies the canvas in C callbacks (timer info)

	ready    bool    // NewCanvasSurface returned; sink callbacks allowed
	closed   bool    // the window closed; nothing reaches the sink
	w, h     float64 // logical size last reported
	scale    float64
	cursor   input.Cursor
	inside   bool // pointer is over the view (tracking area)
	tracking id

	// Text input (canvas_view.go).
	textOn    bool
	caret     geom.Rect
	marked    string // IME pre-edit
	markedSel int    // caret within marked, UTF-16 units
	inKeyDown bool   // interpretKeyEvents: is running for keyDown
	curKey    input.KeyEvent
	downKeys  map[input.Key]bool // keys whose press was reported
	flags     input.Modifiers    // modifiers at the last flagsChanged:

	// Focus: first responder of a key window.
	firstResp bool
	focused   bool

	dropping bool // a file drag is over the view and we take it (canvas_drop.go)

	// Frame scheduling.
	frameWanted bool
	link        id // CADisplayLink, 0 when unavailable
	linkRunning bool
	idleTicks   int
	timer       uintptr   // CFRunLoopTimer
	timerAt     time.Time // pending InvalidateAt deadline
	throttleAt  time.Time // fallback pacing (no display link)
	lastFrame   time.Time
	inFrame     bool
	missed      bool         // AppKit displayed the view before NewCanvasSurface returned
	metalLayer  id           // CAMetalLayer hosted by the view after UseMetalLayer (canvas_metal.go)
	cpuLayer    id           // Metal mode: sublayer showing Present(img) frames (canvas_metal.go)
	tiles       *canvasTiles // CPU mode: tile sublayers of partial frames (canvas_damage.go)

	ax *canvasA11y // accessibility bridge, nil until first used (canvas_a11y.go)

	// Guarded by canvasQueue.mu.
	queued    bool
	qFrame    bool
	qAt       time.Time
	qFocus    bool
	qAttached bool
}

// canvasQueue carries requests from any goroutine to the main thread.
var canvasQueue struct {
	mu    sync.Mutex
	dirty []*canvas
}

// Main-thread registries.
var (
	canvasByView = map[id]*canvas{}
	canvasByKey  = map[uintptr]*canvas{}
	canvasSeq    uintptr
)

var (
	canvasOnce sync.Once

	canvasSource uintptr // CFRunLoopSource driving canvasPerform

	cfDataCreate                   func(alloc uintptr, bytes unsafe.Pointer, n int) uintptr
	cfRelease                      func(obj uintptr)
	cfAbsoluteTimeGetCurrent       func() float64
	cfRunLoopTimerCreate           func(alloc uintptr, fire, interval float64, flags uint, order int, cb uintptr, ctx unsafe.Pointer) uintptr
	cfRunLoopAddTimer              func(rl, timer, mode uintptr)
	cfRunLoopTimerSetNextFireDate  func(timer uintptr, fire float64)
	cfRunLoopTimerInvalidate       func(timer uintptr)
	cgColorSpaceCreateWithName     func(name uintptr) uintptr
	cgDataProviderCreateWithCFData func(data uintptr) uintptr
	cgDataProviderRelease          func(p uintptr)
	cgImageCreate                  func(w, h, bpc, bpp, stride int, space uintptr, info uint32, provider uintptr, decode unsafe.Pointer, interpolate bool, intent int32) uintptr
	cgImageRelease                 func(img uintptr)

	srgbSpace            uintptr
	kCAGravityTopLeft    uintptr
	kCAGravityBottomLeft uintptr
	nsRunLoopCommon      uintptr

	msgSuperSize   func(sup *objcSuper, s objc.SEL, sz nsSize)
	msgSuper       func(sup *objcSuper, s objc.SEL) id
	msgInitTrack   func(obj id, s objc.SEL, r nsRect, opts uint, owner, info id) id
	msgConvertRect func(obj id, s objc.SEL, r nsRect, view id) nsRect

	canvasPerformCB = purego.NewCallback(func(info uintptr) { canvasPerform() })
	canvasTimerCB   = purego.NewCallback(func(timer, info uintptr) { canvasTimerFired(info) })
)

// objcSuper is struct objc_super for objc_msgSendSuper2: lookup starts at
// the superclass of class.
type objcSuper struct {
	receiver id
	class    id
}

func superOf(self id) *objcSuper { return &objcSuper{receiver: self, class: class("TangoCanvasView")} }

// loadCanvas binds the CoreGraphics / QuartzCore functions, registers the
// view class and installs the run loop source. Main thread, after Init.
func loadCanvas() {
	canvasOnce.Do(func() {
		loadExt()
		libCG := mustDlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
		libQC := mustDlopen("/System/Library/Frameworks/QuartzCore.framework/QuartzCore")
		purego.RegisterLibFunc(&cfDataCreate, libCF, "CFDataCreate")
		purego.RegisterLibFunc(&cfRelease, libCF, "CFRelease")
		purego.RegisterLibFunc(&cfAbsoluteTimeGetCurrent, libCF, "CFAbsoluteTimeGetCurrent")
		purego.RegisterLibFunc(&cfRunLoopTimerCreate, libCF, "CFRunLoopTimerCreate")
		purego.RegisterLibFunc(&cfRunLoopAddTimer, libCF, "CFRunLoopAddTimer")
		purego.RegisterLibFunc(&cfRunLoopTimerSetNextFireDate, libCF, "CFRunLoopTimerSetNextFireDate")
		purego.RegisterLibFunc(&cfRunLoopTimerInvalidate, libCF, "CFRunLoopTimerInvalidate")
		purego.RegisterLibFunc(&cgColorSpaceCreateWithName, libCG, "CGColorSpaceCreateWithName")
		purego.RegisterLibFunc(&cgDataProviderCreateWithCFData, libCG, "CGDataProviderCreateWithCFData")
		purego.RegisterLibFunc(&cgDataProviderRelease, libCG, "CGDataProviderRelease")
		purego.RegisterLibFunc(&cgImageCreate, libCG, "CGImageCreate")
		purego.RegisterLibFunc(&cgImageRelease, libCG, "CGImageRelease")

		superFn := mustDlsym(libObjC, "objc_msgSendSuper2")
		purego.RegisterFunc(&msgSuperSize, superFn)
		purego.RegisterFunc(&msgSuper, superFn)
		purego.RegisterFunc(&msgInitTrack, msgSendAddr)
		stret := msgSendAddr
		if isAMD64 {
			stret = mustDlsym(libObjC, "objc_msgSend_stret")
		}
		purego.RegisterFunc(&msgConvertRect, stret)

		srgbSpace = cgColorSpaceCreateWithName(constPtr(libCG, "kCGColorSpaceSRGB"))
		kCAGravityTopLeft = constPtr(libQC, "kCAGravityTopLeft")
		kCAGravityBottomLeft = constPtr(libQC, "kCAGravityBottomLeft")
		nsRunLoopCommon = constPtr(libFoundation, "NSRunLoopCommonModes")

		registerCanvasView()

		var ctx [10]uintptr // CFRunLoopSourceContext; perform is the last field
		ctx[9] = canvasPerformCB
		canvasSource = cfRunLoopSourceCreate(0, 0, unsafe.Pointer(&ctx[0]))
		cfRunLoopAddSource(cfRunLoopGetMain(), canvasSource, kCFRunLoopCommonModes)
	})
}

// NewCanvasSurface implements platform.CanvasCreator.
func (b *backend) NewCanvasSurface(sid platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras, sink platform.CanvasSink) (platform.Surface, platform.Canvas, error) {
	if sink == nil {
		return nil, nil, errors.New("tango/darwin: NewCanvasSurface: nil sink")
	}
	if !isMainThread() {
		return nil, nil, errors.New("tango/darwin: NewCanvasSurface must run on the main thread")
	}
	loadCanvas()
	s, rect := b.newWindow(sid, opts, &ex)
	canvasSeq++
	c := &canvas{s: s, sink: sink, key: canvasSeq, downKeys: map[input.Key]bool{}}
	s.canvas = c
	withPool(func() {
		c.view = msgInitRect(send(class("TangoCanvasView"), "alloc"), sel("initWithFrame:"), rect)
		canvasByView[c.view] = c
		canvasByKey[c.key] = c
		send(c.view, "setWantsLayer:", 1)
		send(c.view, "setLayerContentsRedrawPolicy:", nsViewLayerContentsRedrawOnSetNeedsDisplay)
		s.setContent(c.view, rect)
		c.setGravity()
		c.installScheduling()
	})
	c.w, c.h, c.scale = c.geometry()
	b.finishSurface(s, opts, &ex)
	c.w, c.h, c.scale = c.geometry() // extras may have resized the window
	if s.ext.acceptFiles {
		c.registerDrop() // canvas_drop.go
	}
	if opts.Visible {
		s.Show()
	}
	c.ready = true
	// The sink is never called before NewCanvasSurface returns: the first
	// frame comes from AppKit displaying the view once it is on screen (or
	// is asked for here if that already happened), and focus is reported
	// from the run loop source.
	c.enqueue(func(c *canvas) {
		c.qFocus = true
		c.qFrame = c.qFrame || c.missed
	})
	return s, c, nil
}

// installScheduling adds the view's tracking area, display link and
// InvalidateAt timer (released in destroy). Inside withPool, after the
// view is in its window.
func (c *canvas) installScheduling() {
	const (
		trackEnteredExited = 0x01
		trackMoved         = 0x02
		trackCursorUpdate  = 0x04
		trackActiveInApp   = 0x40
		trackInVisibleRect = 0x200
	)
	c.tracking = msgInitTrack(send(class("NSTrackingArea"), "alloc"), sel("initWithRect:options:owner:userInfo:"),
		nsRect{}, trackEnteredExited|trackMoved|trackCursorUpdate|trackActiveInApp|trackInVisibleRect, c.view, 0)
	send(c.view, "addTrackingArea:", uintptr(c.tracking))

	if respondsTo(c.view, "displayLinkWithTarget:selector:") && nsRunLoopCommon != 0 { // macOS 14+
		c.link = retain(send(c.view, "displayLinkWithTarget:selector:", uintptr(c.view), uintptr(sel("tangoDisplayTick:"))))
		if c.link != 0 {
			send(c.link, "setPaused:", 1)
			send(c.link, "addToRunLoop:forMode:", uintptr(send(class("NSRunLoop"), "mainRunLoop")), nsRunLoopCommon)
		}
	}
	var tctx [5]uintptr // CFRunLoopTimerContext: version, info, retain, release, copyDescription
	tctx[1] = c.key
	c.timer = cfRunLoopTimerCreate(0, farFuture, farFuture, 0, 0, canvasTimerCB, unsafe.Pointer(&tctx[0]))
	cfRunLoopAddTimer(cfRunLoopGetMain(), c.timer, kCFRunLoopCommonModes)
}

// setGravity pins the layer contents to the view's top-left corner, so
// that while a new size waits for its frame the old frame shows unscaled
// instead of stretched. Gravity is in the layer's own coordinates: AppKit
// flips the backing layer of a flipped view, which swaps top and bottom.
func (c *canvas) setGravity() {
	layer := send(c.view, "layer")
	if layer == 0 || kCAGravityTopLeft == 0 || kCAGravityBottomLeft == 0 {
		return
	}
	g := kCAGravityTopLeft
	if sendBool(layer, "contentsAreFlipped") {
		g = kCAGravityBottomLeft
	}
	send(layer, "setContentsGravity:", g)
}

// geometry reads the view's logical size and the window's backing scale.
func (c *canvas) geometry() (w, h, scale float64) {
	b := msgRect(c.view, sel("bounds"))
	scale = 1
	if win := send(c.view, "window"); win != 0 {
		scale = msgGetFloat(win, sel("backingScaleFactor"))
	} else if scr := send(class("NSScreen"), "mainScreen"); scr != 0 {
		scale = msgGetFloat(scr, sel("backingScaleFactor"))
	}
	if scale <= 0 {
		scale = 1
	}
	return b.Size.Width, b.Size.Height, scale
}

// checkGeometry reports a size or scale change; it returns whether there
// was one.
func (c *canvas) checkGeometry() bool {
	if c.closed {
		return false
	}
	w, h, scale := c.geometry()
	if w == c.w && h == c.h && scale == c.scale {
		return false
	}
	c.w, c.h, c.scale = w, h, scale
	c.syncMetalLayer()
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
		purego.SyscallN(cfRunLoopSourceSignalFn, canvasSource)
		purego.SyscallN(cfRunLoopWakeUpFn, cfRunLoopGetMain())
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
	withPool(func() {
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
				}
				c.armTimer()
			}
			if r.frame {
				c.requestFrame()
			}
		}
	})
}

// requestFrame asks for a CanvasFrame at the next display refresh.
func (c *canvas) requestFrame() {
	if c.closed {
		return
	}
	c.frameWanted = true
	if c.link != 0 {
		c.idleTicks = 0
		if !c.linkRunning {
			c.linkRunning = true
			send(c.link, "setPaused:", 0)
		}
		return
	}
	next := c.lastFrame.Add(frameInterval)
	if now := time.Now(); !now.Before(next) {
		c.setNeedsDisplay()
		return
	}
	c.throttleAt = next
	c.armTimer()
}

func (c *canvas) setNeedsDisplay() {
	if c.metalLayer != 0 {
		c.metalDisplay() // layer-hosting: no updateLayer (canvas_metal.go)
		return
	}
	send(c.view, "setNeedsDisplay:", 1)
}

// displayTick runs on every display refresh while the link is unpaused.
func (c *canvas) displayTick() {
	if c.closed {
		return
	}
	if c.frameWanted {
		c.frameWanted = false
		c.idleTicks = 0
		c.setNeedsDisplay()
		return
	}
	c.idleTicks++
	if c.idleTicks >= linkIdleTicks && c.linkRunning {
		c.linkRunning = false
		send(c.link, "setPaused:", 1)
	}
}

// armTimer sets the timer to the earliest of the pending deadlines.
func (c *canvas) armTimer() {
	if c.timer == 0 {
		return
	}
	t := c.timerAt
	if !c.throttleAt.IsZero() && (t.IsZero() || c.throttleAt.Before(t)) {
		t = c.throttleAt
	}
	fire := float64(farFuture)
	if !t.IsZero() {
		fire = cfAbsoluteTimeGetCurrent() + time.Until(t).Seconds()
	}
	cfRunLoopTimerSetNextFireDate(c.timer, fire)
}

func canvasTimerFired(key uintptr) {
	c := canvasByKey[key]
	if c == nil || c.closed {
		return
	}
	withPool(func() {
		now := time.Now().Add(timerTolerance)
		if !c.throttleAt.IsZero() && !now.Before(c.throttleAt) {
			c.throttleAt = time.Time{}
			if c.frameWanted {
				c.setNeedsDisplay()
			}
		}
		if !c.timerAt.IsZero() && !now.Before(c.timerAt) {
			c.timerAt = time.Time{}
			c.requestFrame()
		}
		c.armTimer()
	})
}

// frame runs from updateLayer: AppKit is displaying the view.
func (c *canvas) frame() {
	if c.closed || c.inFrame {
		return
	}
	c.frameWanted = false
	c.throttleAt = time.Time{}
	if !c.timerAt.IsZero() {
		c.timerAt = time.Time{} // this frame satisfies the deadline
		c.armTimer()
	}
	c.checkGeometry()
	c.lastFrame = time.Now()
	if !c.ready {
		c.missed = true // NewCanvasSurface asks for a frame before it returns
		return
	}
	c.inFrame = true
	defer func() { c.inFrame = false }()
	c.sink.CanvasFrame()
}

func (c *canvas) SetCursor(cur input.Cursor) {
	if c.closed {
		return
	}
	c.cursor = cur
	if c.inside {
		c.applyCursor()
	}
}

func (c *canvas) applyCursor() {
	withPool(func() {
		if cur := send(class("NSCursor"), cursorSelector(c.cursor)); cur != 0 {
			send(cur, "set")
		}
	})
}

func (c *canvas) SetTextInput(enabled bool, caret geom.Rect) {
	if c.closed {
		return
	}
	was, old := c.textOn, c.caret
	c.caret = caret
	if was && !enabled {
		withPool(func() {
			if ctx := send(c.view, "inputContext"); ctx != 0 {
				send(ctx, "discardMarkedText")
			}
		})
		c.marked, c.markedSel = "", 0
	}
	c.textOn = enabled
	if enabled && (!was || old != caret) {
		withPool(func() {
			if ctx := send(c.view, "inputContext"); ctx != 0 {
				send(ctx, "invalidateCharacterCoordinates")
			}
		})
	}
}

// Present copies img into a CGImage and makes it the layer's contents.
func (c *canvas) Present(img *image.RGBA) error {
	if c.closed {
		return errCanvasClosed
	}
	if img == nil {
		if c.metalLayer != 0 {
			c.hideCPULayer() // the GPU presented the drawable already
			return nil
		}
		return errors.New("tango/darwin: Present: nil image")
	}
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	off := img.PixOffset(r.Min.X, r.Min.Y)
	n := img.Stride*(h-1) + w*4
	if off < 0 || off+n > len(img.Pix) {
		return errors.New("tango/darwin: Present: image buffer too small")
	}
	cg, err := newCGImage(img.Pix[off:off+n], img.Stride, w, h)
	if err != nil {
		return err
	}
	if c.metalLayer != 0 {
		c.tiles = nil // the view's old layer took the tile sublayers with it
	}
	withPool(func() {
		layer := c.presentLayer()
		ca := class("CATransaction")
		send(ca, "begin")
		send(ca, "setDisableActions:", 1)
		send(layer, "setContents:", cg)
		msgSetFloat(layer, sel("setContentsScale:"), c.scale)
		c.tilesReset(image.Pt(w, h)) // canvas_damage.go: the whole frame covers every tile
		send(ca, "commit")
	})
	cgImageRelease(cg)
	return nil
}

// newCGImage copies w×h premultiplied RGBA pixels (rows stride bytes
// apart, pix starting at the first pixel) into a new CGImage, which the
// caller releases.
func newCGImage(pix []byte, stride, w, h int) (uintptr, error) {
	data := cfDataCreate(0, unsafe.Pointer(&pix[0]), stride*(h-1)+w*4) // copies
	if data == 0 {
		return 0, errors.New("tango/darwin: Present: CFDataCreate failed")
	}
	provider := cgDataProviderCreateWithCFData(data)
	cfRelease(data)
	if provider == 0 {
		return 0, errors.New("tango/darwin: Present: CGDataProviderCreateWithCFData failed")
	}
	cg := cgImageCreate(w, h, 8, 32, stride, srgbSpace,
		kCGImageAlphaPremultipliedLast|kCGBitmapByteOrder32Big, provider, nil, false, 0)
	cgDataProviderRelease(provider)
	if cg == 0 {
		return 0, errors.New("tango/darwin: Present: CGImageCreate failed")
	}
	return cg, nil
}

// destroy runs from surface.destroy when the window closes.
func (c *canvas) destroy() {
	if c.closed {
		return
	}
	c.closed = true
	if c.link != 0 {
		send(c.link, "invalidate") // drops its reference to the view
		release(c.link)
		c.link = 0
	}
	if c.timer != 0 {
		cfRunLoopTimerInvalidate(c.timer)
		cfRelease(c.timer)
		c.timer = 0
	}
	if c.tracking != 0 {
		send(c.view, "removeTrackingArea:", uintptr(c.tracking))
		release(c.tracking)
		c.tracking = 0
	}
	if c.marked != "" {
		if ctx := send(c.view, "inputContext"); ctx != 0 {
			send(ctx, "discardMarkedText")
		}
	}
	c.a11yDestroy() // canvas_a11y.go
	delete(canvasByView, c.view)
	delete(canvasByKey, c.key)
	autorelease(c.view)
}

// focusMaybeChanged reports CanvasFocus when "first responder of the key
// window" changed. Safe on a nil canvas (WebView surfaces).
func (c *canvas) focusMaybeChanged() {
	if c == nil || c.closed {
		return
	}
	f := c.firstResp && sendBool(c.s.win, "isKeyWindow")
	if f == c.focused {
		return
	}
	if !c.ready {
		c.enqueue(func(c *canvas) { c.qFocus = true })
		return
	}
	c.focused = f
	c.sink.CanvasFocus(f)
}
