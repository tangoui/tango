//go:build darwin && !ios

package darwin

// Canvases attached to WebView surfaces (docs/specs/canvas-surface.md,
// step D). The canvas is a TangoCanvasOverlay — a TangoCanvasView
// subclass — added to the container view that holds the WKWebView
// (surface.setContent), above or below the web view. Frame scheduling,
// input and text input are the canvas surface's (canvas.go,
// canvas_view.go); this file adds placement and pointer pass-through.
//
// Pass-through. The overlay keeps the alpha channel of the last presented
// frame. Its hitTest: returns nil over pixels with alpha 0, so AppKit
// delivers clicks, wheel and drags there to the WebView underneath. Mouse
// moves are not hit-tested by AppKit (every tracking area gets them), so
// two more pieces keep the layers apart:
//
//   - the overlay's own mouseMoved:/mouseEntered:/cursorUpdate: ignore
//     transparent pixels (reporting input.Leave when the pointer leaves
//     the opaque part) and never set the cursor there, so WebKit's cursor
//     shows over the page;
//   - a local event monitor swallows mouse moves over the opaque part of
//     an overlay above the WebView and hands them to the overlay alone,
//     so the page does not see hover through the overlay and WebKit's
//     (asynchronous) cursor updates do not override the overlay's cursor.
//
// Focus. A click on the overlay makes it first responder (CanvasFocus
// true); a click on the page makes the WKWebView first responder again
// (NSWindow does that for any view accepting first responder), which the
// overlay reports as CanvasFocus false.
//
// A canvas below the WebView (Above false) needs a see-through WebView:
// while one is attached, drawsBackground is off and the under-page colour
// is clear (applyWebBackground); the page must not paint a background.

import (
	"errors"
	"fmt"
	"image"
	"math"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

var (
	_ platform.CanvasAttacher = (*backend)(nil)
	_ platform.AttachedCanvas = (*overlay)(nil)
)

const (
	nsViewMinXMargin = 1
	nsViewMaxXMargin = 4
	nsViewMinYMargin = 8
	nsViewMaxYMargin = 32

	nsWindowAboveArg = 1 // NSWindowAbove

	nsEventTypeMouseMoved  = 5
	nsEventTypeMouseExited = 9
	nsEventMaskMouseMoved  = 1 << nsEventTypeMouseMoved
)

var errDetached = errors.New("tango/darwin: canvas detached")

// overlay is a canvas attached to a WebView surface. Main thread only.
type overlay struct {
	*canvas
	layer platform.CanvasLayer

	// Content-area size the layer's Bounds were given for (Attach or
	// SetLayer); anchored placement is computed from it (layoutOverlays).
	refW, refH int

	// Alpha of the last presented frame, kept while pass-through applies.
	mask      []byte
	maskW     int
	maskH     int
	maskScale float64
}

var (
	overlayByView = map[id]*overlay{}
	underPageSet  = map[*surface]bool{} // applyWebBackground changed the under-page colour
	webHoverOff   = map[*surface]bool{} // the page was told the pointer left (routeOverlayMove)

	overlayOnce       sync.Once
	msgSuperID        func(sup *objcSuper, s objc.SEL, a id)
	msgSuperHit       func(sup *objcSuper, s objc.SEL, p nsPoint) id
	msgHitTest        func(obj id, s objc.SEL, p nsPoint) id
	msgEnterExitEvent func(cls id, s objc.SEL, typ uint, loc nsPoint, flags uint, ts float64, wn int, ctx id, evNum, tracking int, userData uintptr) id
	overlayMonitor    objc.Block
)

func loadOverlay() {
	overlayOnce.Do(func() {
		superFn := mustDlsym(libObjC, "objc_msgSendSuper2")
		purego.RegisterFunc(&msgSuperID, superFn)
		purego.RegisterFunc(&msgSuperHit, superFn)
		purego.RegisterFunc(&msgHitTest, msgSendAddr)
		purego.RegisterFunc(&msgEnterExitEvent, msgSendAddr)
		registerOverlayView()
		overlayMonitor = objc.NewBlock(func(_ objc.Block, ev objc.ID) objc.ID {
			if routeOverlayMove(ev) {
				return 0
			}
			return ev
		})
		send(class("NSEvent"), "addLocalMonitorForEventsMatchingMask:handler:", nsEventMaskMouseMoved, uintptr(overlayMonitor))
	})
}

func overlayOf(self id) *overlay {
	if o := overlayByView[self]; o != nil && !o.closed {
		return o
	}
	return nil
}

func overlaySuper(self id) *objcSuper {
	return &objcSuper{receiver: self, class: class("TangoCanvasOverlay")}
}

func registerOverlayView() {
	classDef("TangoCanvasOverlay", "TangoCanvasView", nil, []objc.MethodDef{
		method("hitTest:", func(self id, _ objc.SEL, p nsPoint) id {
			hit := msgSuperHit(overlaySuper(self), sel("hitTest:"), p)
			o := overlayOf(self)
			if hit == 0 || o == nil || !o.passThrough() {
				return hit
			}
			local := msgConvertPoint(self, sel("convertPoint:fromView:"), p, send(self, "superview"))
			if o.opaqueAt(local) {
				return hit
			}
			return 0
		}),
		method("mouseMoved:", func(self id, _ objc.SEL, ev id) {
			o := overlayOf(self)
			if o != nil && o.passThrough() && !o.opaqueAt(o.viewPoint(ev)) {
				o.leave(ev)
				return
			}
			if o != nil && !o.inside {
				o.inside = true
				o.applyCursor()
			}
			msgSuperID(overlaySuper(self), sel("mouseMoved:"), ev)
		}),
		method("mouseEntered:", func(self id, _ objc.SEL, ev id) {
			if o := overlayOf(self); o != nil && o.passThrough() && !o.opaqueAt(o.viewPoint(ev)) {
				return
			}
			msgSuperID(overlaySuper(self), sel("mouseEntered:"), ev)
		}),
		method("mouseExited:", func(self id, _ objc.SEL, ev id) {
			if o := overlayOf(self); o != nil {
				o.leave(ev)
			}
		}),
		method("cursorUpdate:", func(self id, _ objc.SEL, ev id) {
			o := overlayOf(self)
			if o == nil {
				return
			}
			if o.passThrough() {
				p := msgGetPoint(o.s.win, sel("mouseLocationOutsideOfEventStream"))
				if !o.opaqueAt(msgConvertPoint(self, sel("convertPoint:fromView:"), p, 0)) {
					return // the WebView owns the cursor here
				}
			}
			o.applyCursor()
		}),
	})
}

// leave reports the pointer leaving the overlay (or its opaque part).
func (o *overlay) leave(ev id) {
	if !o.inside {
		return
	}
	o.inside = false
	o.input(input.Pointer{Kind: input.Leave, Pos: o.loc(ev), Mods: eventMods(ev)})
}

// viewPoint is the event location in view coordinates.
func (o *overlay) viewPoint(ev id) nsPoint {
	return msgConvertPoint(o.view, sel("convertPoint:fromView:"), msgGetPoint(ev, sel("locationInWindow")), 0)
}

func (o *overlay) passThrough() bool { return o.layer.Above && o.layer.PassThrough }

// opaqueAt reports whether the last presented frame has a pixel at p
// (view coordinates: logical pixels, top-left origin) whose alpha reaches
// the layer's hit threshold (CanvasLayer.HitAlpha, at least 1).
func (o *overlay) opaqueAt(p nsPoint) bool {
	if o.maskW == 0 || p.X < 0 || p.Y < 0 {
		return false
	}
	x, y := int(p.X*o.maskScale), int(p.Y*o.maskScale)
	if x >= o.maskW || y >= o.maskH {
		return false
	}
	return o.mask[y*o.maskW+x] >= o.layer.HitThreshold()
}

// routeOverlayMove runs in the mouse-moved event monitor: when the pointer
// is over the opaque part of an overlay above a WebView it delivers the
// event to that overlay only and reports true (swallow).
func routeOverlayMove(ev id) bool {
	if len(overlayByView) == 0 || ev == 0 {
		return false
	}
	win := send(ev, "window")
	b := theBackend
	if win == 0 || b == nil {
		return false
	}
	for _, s := range b.surfaces {
		if s.win != win || s.closed || len(s.overlays) == 0 {
			continue
		}
		o := s.overlayAt(msgGetPoint(ev, sel("locationInWindow")))
		if o == nil || !o.layer.Above {
			// Over the page again: WebKit's own mouse moves restore its
			// hover state.
			delete(webHoverOff, s)
			return false
		}
		if !webHoverOff[s] {
			webHoverOff[s] = true
			s.webPointerLeft(ev)
		}
		send(o.view, "mouseMoved:", uintptr(ev))
		return true
	}
	return false
}

// webPointerLeft tells the page that the pointer left it, because an
// opaque overlay now covers the pointer and swallows the moves: WebKit
// gets a synthetic mouse move to a point just outside the WebView's
// bounds, so :hover styles and mouseleave/mouseout handlers see the
// pointer go (its tracking area sends no mouseExited: here, as the
// pointer is still inside its frame).
func (s *surface) webPointerLeft(ev id) {
	if s.web == 0 || s.closed {
		return
	}
	outside := msgConvertPoint(s.web, sel("convertPoint:toView:"), nsPoint{X: -10, Y: -10}, 0)
	if sendBool(s.web, "isFlipped") {
		outside = msgConvertPoint(s.web, sel("convertPoint:toView:"),
			nsPoint{X: -10, Y: msgRect(s.web, sel("bounds")).Size.Height + 10}, 0)
	}
	exited := msgEnterExitEvent(class("NSEvent"), sel("enterExitEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:trackingNumber:userData:"),
		nsEventTypeMouseExited, outside, uint(send(ev, "modifierFlags")), msgGetFloat(ev, sel("timestamp")),
		sendInt(s.win, "windowNumber"), 0, 0, 0, 0)
	if exited != 0 {
		send(s.webMoveTarget(), "mouseExited:", uintptr(exited))
	}
}

// webMoveTarget is the object WebKit handles mouse moves in: the owner
// of the WebView's tracking area that asks for moves (WebKit's private
// WKMouseTrackingObserver; WKWebView's own mouseMoved: ignores them), or
// the WebView itself when there is none.
func (s *surface) webMoveTarget() id {
	const trackMoved = 0x02 // NSTrackingMouseMoved
	for _, ta := range arrayItems(send(s.web, "trackingAreas")) {
		if send(ta, "options")&trackMoved != 0 {
			if owner := send(ta, "owner"); owner != 0 && respondsTo(owner, "mouseMoved:") {
				return owner
			}
		}
	}
	return s.web
}

// overlayAt returns the overlay AppKit's hit testing finds at p (window
// coordinates), or nil.
func (s *surface) overlayAt(p nsPoint) *overlay {
	container := send(s.web, "superview")
	if container == 0 {
		return nil
	}
	sup := send(container, "superview")
	if sup == 0 {
		return nil
	}
	hit := msgHitTest(container, sel("hitTest:"), msgConvertPoint(sup, sel("convertPoint:fromView:"), p, 0))
	return overlayOf(hit)
}

// AttachCanvas implements platform.CanvasAttacher.
func (b *backend) AttachCanvas(sid platform.SurfaceID, layer platform.CanvasLayer, sink platform.CanvasSink) (platform.Canvas, error) {
	if sink == nil {
		return nil, errors.New("tango/darwin: AttachCanvas: nil sink")
	}
	if !isMainThread() {
		return nil, errors.New("tango/darwin: AttachCanvas must run on the main thread")
	}
	s := b.surfaces[sid]
	if s == nil || s.closed {
		return nil, fmt.Errorf("tango/darwin: AttachCanvas: no open surface %d", sid)
	}
	if s.web == 0 {
		return nil, errors.New("tango/darwin: AttachCanvas: not a WebView surface")
	}
	container := send(s.web, "superview")
	if container == 0 {
		return nil, errors.New("tango/darwin: AttachCanvas: WebView has no container")
	}
	loadCanvas()
	loadOverlay()
	canvasSeq++
	o := &overlay{
		canvas: &canvas{s: s, sink: sink, key: canvasSeq, downKeys: map[input.Key]bool{}},
		layer:  layer,
	}
	withPool(func() {
		rect, mask := overlayFrame(container, layer)
		o.view = msgInitRect(send(class("TangoCanvasOverlay"), "alloc"), sel("initWithFrame:"), rect)
		canvasByView[o.view] = o.canvas
		canvasByKey[o.key] = o.canvas
		overlayByView[o.view] = o
		send(o.view, "setWantsLayer:", 1)
		send(o.view, "setLayerContentsRedrawPolicy:", nsViewLayerContentsRedrawOnSetNeedsDisplay)
		send(o.view, "setAutoresizingMask:", mask)
		o.place(container)
		o.setGravity()
		o.installScheduling()
		o.refW, o.refH = containerSize(container)
		if s.ext.acceptFiles {
			o.registerDrop() // canvas_drop.go
		}
	})
	s.overlays = append(s.overlays, o)
	if !layer.Above {
		s.refreshWebBackground()
	}
	o.w, o.h, o.scale = o.geometry()
	o.ready = true
	// As for NewCanvasSurface, the sink hears nothing before we return:
	// the first frame and the focus state come from the run loop source.
	o.enqueue(func(c *canvas) {
		c.qFocus = true
		c.qFrame = true
	})
	return o, nil
}

// overlayFrame converts layer bounds (top-left origin) to a frame in the
// container and the autoresizing mask that keeps it at its anchor.
func overlayFrame(container id, l platform.CanvasLayer) (nsRect, uintptr) {
	cb := msgRect(container, sel("bounds"))
	r := l.Bounds
	if r.Width <= 0 || r.Height <= 0 {
		return cb, nsViewWidthHeightSizable
	}
	return rectInContainer(container, cb, r), anchorMask(l.Anchor, sendBool(container, "isFlipped"))
}

// rectInContainer converts r (top-left origin) to a frame in container.
func rectInContainer(container id, cb nsRect, r platform.Rect) nsRect {
	y := cb.Size.Height - float64(r.Y) - float64(r.Height)
	if sendBool(container, "isFlipped") {
		y = float64(r.Y)
	}
	return nsRect{Origin: nsPoint{float64(r.X), y}, Size: nsSize{float64(r.Width), float64(r.Height)}}
}

// anchorMask is the autoresizing mask that keeps a fixed-size frame at
// anchor a. For AnchorCenter AppKit would split a size change in
// proportion to the margins, so layoutOverlays recomputes the frame
// exactly after each resize; the flexible margins only keep it close
// until then.
func anchorMask(a platform.CanvasAnchor, flipped bool) uintptr {
	top, bottom := uintptr(nsViewMinYMargin), uintptr(nsViewMaxYMargin) // flexible margin that keeps the top / bottom edge
	if flipped {
		top, bottom = bottom, top
	}
	switch a {
	case platform.AnchorTopRight:
		return nsViewMinXMargin | top
	case platform.AnchorBottomLeft:
		return nsViewMaxXMargin | bottom
	case platform.AnchorBottomRight:
		return nsViewMinXMargin | bottom
	case platform.AnchorCenter:
		return nsViewMinXMargin | nsViewMaxXMargin | nsViewMinYMargin | nsViewMaxYMargin
	}
	return nsViewMaxXMargin | top // AnchorTopLeft
}

// containerSize is the container's size in whole logical pixels.
func containerSize(container id) (int, int) {
	cb := msgRect(container, sel("bounds"))
	return int(math.Round(cb.Size.Width)), int(math.Round(cb.Size.Height))
}

// layoutOverlays puts centre-anchored overlays exactly in place after the
// window resized (windowDidResize:); corner anchors follow from their
// autoresizing masks.
func (s *surface) layoutOverlays() {
	for _, o := range s.overlays {
		l := o.layer
		if o.closed || l.Anchor != platform.AnchorCenter || l.Bounds.Width <= 0 || l.Bounds.Height <= 0 {
			continue
		}
		withPool(func() {
			container := send(o.view, "superview")
			if container == 0 {
				return
			}
			w, h := containerSize(container)
			r := l.AnchoredBounds(o.refW, o.refH, w, h)
			msgSetRect(o.view, sel("setFrame:"), rectInContainer(container, msgRect(container, sel("bounds")), r))
		})
	}
}

// place (re)inserts the view above or below the WebView.
func (o *overlay) place(container id) {
	pos := uintptr(nsWindowBelowArg)
	if o.layer.Above {
		pos = nsWindowAboveArg
	}
	send(container, "addSubview:positioned:relativeTo:", uintptr(o.view), pos, uintptr(o.s.web))
}

// Present implements platform.Canvas and keeps the frame's alpha for
// hit testing.
func (o *overlay) Present(img *image.RGBA) error {
	if o.closed {
		return errDetached
	}
	if err := o.canvas.Present(img); err != nil {
		return err
	}
	if !o.passThrough() {
		o.mask, o.maskW, o.maskH = nil, 0, 0
		return nil
	}
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	if cap(o.mask) < w*h {
		o.mask = make([]byte, w*h)
	}
	o.mask = o.mask[:w*h]
	for y := range h {
		row := img.Pix[img.PixOffset(r.Min.X, r.Min.Y+y):]
		dst := o.mask[y*w : (y+1)*w]
		for x := range dst {
			dst[x] = row[x*4+3]
		}
	}
	o.maskW, o.maskH, o.maskScale = w, h, o.scale
	return nil
}

// SetLayer implements platform.AttachedCanvas.
func (o *overlay) SetLayer(layer platform.CanvasLayer) error {
	if !isMainThread() {
		return errors.New("tango/darwin: SetLayer must run on the main thread")
	}
	if o.closed {
		return errDetached
	}
	old := o.layer
	o.layer = layer
	withPool(func() {
		container := send(o.view, "superview")
		if container == 0 {
			return
		}
		rect, mask := overlayFrame(container, layer)
		o.refW, o.refH = containerSize(container)
		send(o.view, "setAutoresizingMask:", mask)
		if layer.Above != old.Above {
			// Re-inserting would resign first responder; keep it.
			first := send(o.s.win, "firstResponder") == o.view
			retain(o.view)
			send(o.view, "removeFromSuperview")
			o.place(container)
			release(o.view)
			if first {
				send(o.s.win, "makeFirstResponder:", uintptr(o.view))
			}
		}
		msgSetRect(o.view, sel("setFrame:"), rect) // setFrameSize: reports a size change
	})
	if layer.Above != old.Above {
		o.s.refreshWebBackground()
	}
	if o.passThrough() && o.maskW == 0 {
		o.Invalidate() // a mask arrives with the next frame
	}
	return nil
}

// Detach implements platform.AttachedCanvas.
func (o *overlay) Detach() {
	if o.closed {
		return
	}
	s := o.s
	withPool(func() {
		first := send(s.win, "firstResponder") == o.view
		send(o.view, "removeFromSuperview")
		if first && !s.closed {
			send(s.win, "makeFirstResponder:", uintptr(s.web))
		}
		o.destroyOverlay()
	})
	for i, x := range s.overlays {
		if x == o {
			s.overlays = append(s.overlays[:i], s.overlays[i+1:]...)
			break
		}
	}
	if !o.layer.Above && !s.closed {
		s.refreshWebBackground()
	}
}

func (o *overlay) destroyOverlay() {
	delete(overlayByView, o.view)
	o.mask = nil
	o.canvas.destroy()
}

// destroyOverlays runs from surface.destroy.
func (s *surface) destroyOverlays() {
	for _, o := range s.overlays {
		o.destroyOverlay()
	}
	s.overlays = nil
	delete(underPageSet, s)
	delete(webHoverOff, s)
}

func (s *surface) overlaysFocusMaybeChanged() {
	for _, o := range s.overlays {
		o.focusMaybeChanged()
	}
}

// hasUnderlay reports whether a canvas is attached below the WebView.
func (s *surface) hasUnderlay() bool {
	for _, o := range s.overlays {
		if !o.layer.Above && !o.closed {
			return true
		}
	}
	return false
}

// applyWebBackground sets the WebView's own background for the
// appearance (see is a transparent or material window, bg its colour) and
// the canvases below the WebView. Called by applyAppearance.
//
// drawsBackground is not public API on macOS but is the documented way
// (WebKit headers, Safari) to get a see-through WKWebView.
func (s *surface) applyWebBackground(see bool, bg id) {
	if s.web == 0 {
		return
	}
	a := s.ext.appearance
	under := s.hasUnderlay()
	send(s.web, "setValue:forKey:", uintptr(nsNumberBool(!see && a.Background == 0 && !under)), uintptr(nsString("drawsBackground")))
	if !respondsTo(s.web, "setUnderPageBackgroundColor:") { // macOS 12+
		return
	}
	switch {
	case under:
		send(s.web, "setUnderPageBackgroundColor:", uintptr(send(class("NSColor"), "clearColor")))
	case see || a.Background != 0:
		send(s.web, "setUnderPageBackgroundColor:", uintptr(bg))
	case underPageSet[s]:
		send(s.web, "setUnderPageBackgroundColor:", 0) // back to the default
	}
	if under || see || a.Background != 0 {
		underPageSet[s] = true
	} else {
		delete(underPageSet, s)
	}
}

// refreshWebBackground re-applies applyWebBackground for the current
// appearance after canvases below the WebView came or went.
func (s *surface) refreshWebBackground() {
	if s.closed || s.web == 0 {
		return
	}
	withPool(func() {
		a := s.ext.appearance
		see := a.Transparent || a.Material != platform.MaterialNone
		var bg id
		switch {
		case see:
			bg = send(class("NSColor"), "clearColor")
		case a.Background != 0:
			r, g, b, al := rgbaParts(a.Background)
			bg = msgColorRGBA(class("NSColor"), sel("colorWithSRGBRed:green:blue:alpha:"), r, g, b, al)
		}
		s.applyWebBackground(see, bg)
	})
}

// PresentDamage shadows the canvas's partial path: an overlay keeps its
// hit-test mask from whole frames only (and may be transparent).
func (o *overlay) PresentDamage(img *image.RGBA, _ []image.Rectangle) error {
	return o.Present(img)
}
