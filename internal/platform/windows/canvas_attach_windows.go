//go:build windows && (amd64 || arm64)

package windows

// Canvases attached to WebView surfaces (docs/specs/canvas-surface.md,
// step D). The canvas is a child window of the "TangoCanvas" class
// (canvas_windows.go: frame scheduling, DIB, input, IME) created in the
// surface's top-level window, which is also the parent the WebView2
// controller puts its own child window in. This file adds placement,
// stacking, transparency, pointer pass-through and focus.
//
// Stacking. Siblings are ordered with SetWindowPos: HWND_TOP for a canvas
// above the WebView, HWND_BOTTOM below it (later canvases above earlier
// ones of the same kind). The WebView2 child window appears when the
// controller is created, asynchronously and on top of its siblings, so
// the order is applied again then (attachedWebReady) and on every resize.
//
// Transparency. A canvas above the WebView is a layered child window
// (WS_EX_LAYERED, Windows 8+, needs the Windows 8+ compatibility entry in
// the application manifest): frames go to the screen with
// UpdateLayeredWindow from the premultiplied BGRA DIB (AC_SRC_ALPHA), so
// the page shows through transparent pixels. Layered windows get no
// WM_PAINT-driven drawing, so paintNow renders and pushes the frame
// directly (paintLayered). When the system refuses the layered style or
// UpdateLayeredWindow fails, the canvas falls back to the opaque BitBlt
// path of canvas_windows.go for good. A canvas below the WebView is an
// ordinary (opaque) child without WS_CLIPSIBLINGS, so it paints under the
// WebView's window; while one is attached the WebView's default
// background is transparent (ICoreWebView2Controller2
// put_DefaultBackgroundColor alpha 0) and the page must not paint one.
//
// Pass-through (Above and PassThrough). Present keeps a mask of the
// frame's pixels with alpha >= HitThreshold. Two mechanisms use it:
//
//   - WM_NCHITTEST returns HTTRANSPARENT below the threshold. The system
//     then tries the windows underneath, but only those of the same
//     thread: enough for the surface window itself, not for WebView2's
//     window, which belongs to the browser process.
//   - So the canvas also gets a window region (SetWindowRgn) covering the
//     mask, rebuilt when the mask changes, at most 30 times a second.
//     Outside the region the canvas does not exist for hit testing, so
//     clicks, wheel and moves go to whatever window is below, whatever
//     process owns it. The region also clips drawing: pixels with alpha
//     below the threshold are not shown (none are lost with the default
//     threshold 1).
//
// Focus. A click on the canvas focuses it (canvas_input_windows.go); a
// click on the page focuses WebView2's window, which the canvas sees as
// WM_KILLFOCUS. The surface remembers whether a canvas or the page had
// focus last and restores it when the window is activated again
// (focusContent). Detaching a focused canvas moves focus to the WebView.

import (
	"errors"
	"fmt"
	"image"
	"log"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

var (
	_ platform.CanvasAttacher = (*Backend)(nil)
	_ platform.AttachedCanvas = (*attached)(nil)
)

const (
	attachWmNCHitTest = 0x0084
	attachWmRegion    = 0x0400 + 0x7A3 // canvas class private: rebuild the window region

	attachHTTransparent = ^uintptr(0) // HTTRANSPARENT (-1)

	attachHwndTop    = 0
	attachHwndBottom = 1

	attachWsExLayered = 0x00080000
	attachUlwAlpha    = 0x2
	attachAcSrcAlpha  = 0x1
	attachRdhRects    = 1 // RDH_RECTANGLES

	attachRegionInterval = time.Second / 30
)

var errDetached = errors.New("tango/windows: canvas detached")

var (
	attachGDI32 = newSysLib(inSystemDir("gdi32.dll"))

	attachUpdateLayeredWindow = user32.export("UpdateLayeredWindow")
	attachSetWindowRgn        = user32.export("SetWindowRgn")
	attachIsChild             = user32.export("IsChild")
	attachExtCreateRegion     = attachGDI32.export("ExtCreateRegion")
	attachCreateRectRgn       = attachGDI32.export("CreateRectRgn")
)

// attachSize is SIZE.
type attachSize struct{ CX, CY int32 }

// attachRgnHeader is RGNDATAHEADER (32 bytes, the size of two RECTs).
type attachRgnHeader struct {
	Size, Type, Count, RgnSize uint32
	Bound                      rect
}

var _ = [1]struct{}{}[unsafe.Sizeof(attachRgnHeader{})-2*unsafe.Sizeof(rect{})]

// attached is a canvas attached to a WebView surface. UI thread only.
type attached struct {
	*canvas
	s     *surface
	layer platform.CanvasLayer
	// Content size (logical) when layer was set: Anchor offsets are
	// relative to it.
	baseW, baseH int

	layered   bool // WS_EX_LAYERED + UpdateLayeredWindow in use
	noLayered bool // layering failed once: stay opaque
	presents  int  // Present calls, to tell whether a frame was pushed

	// Pass-through: frame pixels at or above the threshold.
	mask         []byte
	maskW, maskH int
	maskThr      uint8
	rgnDirty     bool
	rgnArmed     bool
	rgnAt        time.Time
	rgnTimer     *time.Timer
	hasRgn       bool
}

var (
	attachedBySurface = map[*surface][]*attached{}
	attachedByCanvas  = map[*canvas]*attached{}
	attachFocus       = map[*surface]*attached{} // canvas that had focus last; absent: the WebView
	attachBGSet       = map[*surface]bool{}      // the WebView background was made transparent
)

// AttachCanvas implements platform.CanvasAttacher. UI thread. The sink is
// not called before it returns: the first frame and the focus report come
// from posted messages.
func (b *Backend) AttachCanvas(sid platform.SurfaceID, layer platform.CanvasLayer, sink platform.CanvasSink) (platform.Canvas, error) {
	if sink == nil {
		return nil, errors.New("tango/windows: AttachCanvas: nil sink")
	}
	s := b.byID[sid]
	if s == nil || s.closed {
		return nil, fmt.Errorf("tango/windows: AttachCanvas: no open surface %d", sid)
	}
	if s.canvas != nil {
		return nil, errors.New("tango/windows: AttachCanvas: not a WebView surface")
	}
	if err := registerCanvasClass(); err != nil {
		return nil, err
	}
	c := &canvas{parent: s.hwnd, sink: sink, downKeys: map[input.Key]bool{}, scale: 1, dpi: baseDPI}
	a := &attached{canvas: c, s: s, layer: layer}
	a.baseW, a.baseH = s.Size()
	x, y, w, h := a.deviceRect()
	style := uintptr(canvasWsChild | canvasWsVisible)
	if layer.Above {
		style |= canvasWsClipSiblings
	}
	c.SetCursor(input.CursorDefault)
	attachedByCanvas[c] = a
	canvasCreating = c
	hwnd, _, err := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16(canvasClass))), 0, style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		s.hwnd, canvasChildID, instance(), 0)
	canvasCreating = nil
	if hwnd == 0 {
		delete(attachedByCanvas, c)
		if c.hwnd != 0 {
			delete(canvasByHwnd, c.hwnd)
		}
		return nil, fmt.Errorf("tango/windows: AttachCanvas: CreateWindowExW: %v", err)
	}
	c.hwnd = hwnd
	canvasByHwnd[hwnd] = c
	c.disableIME()
	c.readGeometry()
	attachedBySurface[s] = append(attachedBySurface[s], a)

	a.applyStyle()
	s.restackAttached()
	if !layer.Above {
		s.refreshWebBackground()
	}
	if a.passThrough() {
		a.applyRegion() // empty until the first frame
	}
	c.start()
	if a.layered {
		c.Invalidate() // layered windows are drawn by paintNow, not WM_PAINT
	}
	return a, nil
}

func (a *attached) passThrough() bool { return a.layer.Above && a.layer.PassThrough }

// deviceRect is the canvas rectangle in the parent's client area, in
// device pixels, for the current content size.
func (a *attached) deviceRect() (x, y, w, h int32) {
	cw, ch := a.s.Size()
	r := attachBounds(a.layer, a.baseW, a.baseH, cw, ch)
	return attachDevice(r, float64(dpiOf(a.s.hwnd))/baseDPI)
}

// place moves and sizes the window for the current content size; a size
// change reaches the canvas as WM_SIZE (CanvasResized and a frame).
func (a *attached) place() {
	if a.closed {
		return
	}
	x, y, w, h := a.deviceRect()
	procSetWindowPos.Call(a.hwnd, 0, uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoZOrder|swpNoActivate)
}

// applyStyle makes the window layered (above the WebView) or not, and
// clips it by its siblings only above the WebView.
func (a *attached) applyStyle() {
	st := windowLong(a.hwnd, wxGwlStyle)
	want := st &^ canvasWsClipSiblings
	if a.layer.Above {
		want |= canvasWsClipSiblings
	}
	if want != st {
		setWindowLong(a.hwnd, wxGwlStyle, want)
	}

	layered := a.layer.Above && !a.noLayered
	if layered == a.layered {
		return
	}
	ex := windowLong(a.hwnd, wxGwlExStyle)
	if !layered {
		setWindowLong(a.hwnd, wxGwlExStyle, ex&^attachWsExLayered)
		a.layered = false
		canvasInvalidateRect.Call(a.hwnd, 0, 0) // back to WM_PAINT + BitBlt
		return
	}
	setWindowLong(a.hwnd, wxGwlExStyle, ex|attachWsExLayered)
	if windowLong(a.hwnd, wxGwlExStyle)&attachWsExLayered == 0 {
		a.layerFailed("the system refused WS_EX_LAYERED on a child window (no Windows 8+ manifest entry?)")
		return
	}
	a.layered = true
	a.pushLayered()
}

// layerFailed switches to the opaque path for the rest of the canvas's
// life.
func (a *attached) layerFailed(why string) {
	log.Printf("tango/windows: attached canvas: %s; drawing it opaque", why)
	a.noLayered = true
	ex := windowLong(a.hwnd, wxGwlExStyle)
	if ex&attachWsExLayered != 0 {
		setWindowLong(a.hwnd, wxGwlExStyle, ex&^attachWsExLayered)
	}
	a.layered = false
	canvasInvalidateRect.Call(a.hwnd, 0, 0)
}

// pushLayered shows the DIB with UpdateLayeredWindow and reports whether
// it did. A DIB of another size than the window (a frame for the new size
// is on its way) is not shown.
func (a *attached) pushLayered() bool {
	c := a.canvas
	if !a.layered || c.closed || c.dib == 0 || int32(c.dibW) != c.pxW || int32(c.dibH) != c.pxH {
		return false
	}
	canvasGdiFlush.Call()
	size := attachSize{c.pxW, c.pxH}
	var src point
	blend := [4]byte{0, 0, 255, attachAcSrcAlpha} // AC_SRC_OVER, flags, constant alpha, AC_SRC_ALPHA
	r, _, err := attachUpdateLayeredWindow.Call(c.hwnd, 0, 0, uintptr(unsafe.Pointer(&size)),
		c.memDC, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), attachUlwAlpha)
	if r == 0 {
		a.layerFailed(fmt.Sprintf("UpdateLayeredWindow: %v", err))
		return false
	}
	return true
}

// paintLayered stands in for WM_PAINT while the window is layered (see
// paintNow): render a frame if one is due, then push it.
func (a *attached) paintLayered() {
	c := a.canvas
	if c.inPaint || c.sinkDepth > 0 {
		c.Invalidate() // no re-entrant frames: come back from the queue
		return
	}
	n := a.presents
	c.inPaint = true
	if c.ready && !c.closed {
		changed := c.checkGeometry()
		if !c.closed && (changed || c.frameWanted || c.dib == 0) {
			c.frame()
		}
	}
	c.inPaint = false
	if !c.closed && a.presents == n {
		a.pushLayered()
	}
}

// Present implements platform.Canvas: the frame goes to the DIB (and the
// screen), and its alpha to the pass-through mask.
func (a *attached) Present(img *image.RGBA) error {
	if a.closed {
		return errDetached
	}
	if err := a.canvas.Present(img); err != nil {
		return err
	}
	a.presents++
	if a.layered {
		a.pushLayered()
	}
	a.updateMask(img)
	return nil
}

// updateMask records which pixels of img take pointer input.
func (a *attached) updateMask(img *image.RGBA) {
	if !a.passThrough() {
		a.mask, a.maskW, a.maskH = nil, 0, 0
		return
	}
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	thr := a.layer.HitThreshold()
	fresh := w != a.maskW || h != a.maskH || thr != a.maskThr
	if fresh {
		a.mask = make([]byte, w*h)
		a.maskW, a.maskH, a.maskThr = w, h, thr
	}
	if attachHitMask(a.mask, img.Pix[img.PixOffset(r.Min.X, r.Min.Y):], img.Stride, w, h, thr) || fresh {
		a.scheduleRegion()
	}
}

// opaqueAt reports whether client pixel (x, y) takes pointer input.
func (a *attached) opaqueAt(x, y int32) bool {
	if a.maskW == 0 || x < 0 || y < 0 || int(x) >= a.maskW || int(y) >= a.maskH {
		return false
	}
	return a.mask[int(y)*a.maskW+int(x)] != 0
}

// scheduleRegion rebuilds the window region from a posted message
// (Present runs inside painting): right away, or 1/30 s after the last
// rebuild if that is later.
func (a *attached) scheduleRegion() {
	a.rgnDirty = true
	if a.rgnArmed {
		return
	}
	a.rgnArmed = true
	hwnd := a.hwnd
	wait := attachRegionInterval - time.Since(a.rgnAt)
	if wait <= 0 {
		if !postMessage(hwnd, attachWmRegion, 0, 0) {
			a.rgnArmed = false
		}
		return
	}
	if a.rgnTimer == nil {
		a.rgnTimer = time.AfterFunc(wait, func() { postMessage(hwnd, attachWmRegion, 0, 0) })
	} else {
		a.rgnTimer.Reset(wait)
	}
}

// applyRegion sets the window region from the mask (pass-through) or
// removes it.
func (a *attached) applyRegion() {
	a.rgnDirty = false
	a.rgnAt = time.Now()
	if a.closed {
		return
	}
	if !a.passThrough() {
		if a.hasRgn {
			attachSetWindowRgn.Call(a.hwnd, 0, 1)
			a.hasRgn = false
		}
		return
	}
	var hrgn uintptr
	rects := attachMaskRects(a.mask, a.maskW, a.maskH)
	if len(rects) == 0 {
		hrgn, _, _ = attachCreateRectRgn.Call(0, 0, 0, 0)
	} else {
		buf := make([]rect, 2+len(rects)) // RGNDATAHEADER, then the rectangles
		hd := (*attachRgnHeader)(unsafe.Pointer(&buf[0]))
		hd.Size, hd.Type, hd.Count = 32, attachRdhRects, uint32(len(rects))
		hd.RgnSize = uint32(len(rects)) * uint32(unsafe.Sizeof(rect{}))
		hd.Bound = rect{Right: int32(a.maskW), Bottom: int32(a.maskH)}
		for i, r := range rects {
			buf[2+i] = rect{Left: r[0], Top: r[1], Right: r[0] + r[2], Bottom: r[1] + r[3]}
		}
		hrgn, _, _ = attachExtCreateRegion.Call(0, uintptr(len(buf))*unsafe.Sizeof(rect{}), uintptr(unsafe.Pointer(&buf[0])))
	}
	if hrgn == 0 {
		return
	}
	if r, _, _ := attachSetWindowRgn.Call(a.hwnd, hrgn, 1); r == 0 {
		canvasDeleteObject.Call(hrgn) // the system owns it only on success
		return
	}
	a.hasRgn = true
}

// message handles the attached-canvas messages ahead of the canvas class
// (canvas.message); ok false lets the canvas handle m.
func (a *attached) message(m uint32, wp, lp uintptr) (uintptr, bool) {
	if a.closed {
		return 0, false
	}
	switch m {
	case attachWmNCHitTest:
		if !a.passThrough() {
			return 0, false
		}
		p := point{int32(int16(loword(lp))), int32(int16(hiword(lp)))}
		canvasScreenToClient.Call(a.hwnd, uintptr(unsafe.Pointer(&p)))
		if !a.opaqueAt(p.X, p.Y) {
			return attachHTTransparent, true
		}
	case attachWmRegion:
		a.rgnArmed = false
		if a.rgnDirty {
			a.applyRegion()
		}
		return 0, true
	case canvasWmSetFocus:
		attachFocus[a.s] = a
	case canvasWmKillFocus:
		// wp is the window getting focus: another child of the surface
		// (WebView2's window, another canvas) takes over; 0 or a window
		// elsewhere means the surface was deactivated, so keep it.
		if wp != 0 && attachFocus[a.s] == a {
			if r, _, _ := attachIsChild.Call(a.s.hwnd, wp); r != 0 {
				delete(attachFocus, a.s)
			}
		}
	}
	return 0, false
}

// SetLayer implements platform.AttachedCanvas. UI thread.
func (a *attached) SetLayer(l platform.CanvasLayer) error {
	if a.closed {
		return errDetached
	}
	old := a.layer
	wasPass := a.passThrough()
	a.layer = l
	a.baseW, a.baseH = a.s.Size()
	a.applyStyle()
	if l.Above != old.Above {
		a.s.restackAttached()
		a.s.refreshWebBackground()
	}
	a.place()
	switch {
	case a.passThrough() && (!wasPass || l.HitThreshold() != old.HitThreshold()):
		a.mask, a.maskW, a.maskH = nil, 0, 0
		a.applyRegion() // empty until the next frame brings a mask
		a.Invalidate()
	case !a.passThrough() && wasPass:
		a.mask, a.maskW, a.maskH = nil, 0, 0
		a.applyRegion() // removes it
	}
	return nil
}

// Detach implements platform.AttachedCanvas. UI thread; idempotent.
func (a *attached) Detach() {
	if a.closed {
		return
	}
	s := a.s
	focused := false
	if f, _, _ := canvasGetFocus.Call(); f == a.hwnd {
		focused = true
	}
	a.teardown()
	list := attachedBySurface[s]
	for i, x := range list {
		if x == a {
			attachedBySurface[s] = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	if len(attachedBySurface[s]) == 0 {
		delete(attachedBySurface, s)
	}
	procDestroyWindow.Call(a.hwnd) // WM_NCDESTROY drops it from canvasByHwnd
	delete(attachedByCanvas, a.canvas)
	if s.closed {
		return
	}
	if focused {
		s.focusWebView()
	}
	if !a.layer.Above {
		s.refreshWebBackground()
	}
}

// teardown stops the canvas: no sink calls, no timers.
func (a *attached) teardown() {
	if a.rgnTimer != nil {
		a.rgnTimer.Stop()
	}
	a.mask = nil
	if attachFocus[a.s] == a {
		delete(attachFocus, a.s)
	}
	a.canvas.destroy()
}

// ---- surface side ----

// destroyAttached runs from surface.destroyed, before SurfaceClosed: the
// child windows themselves go with the surface window.
func (s *surface) destroyAttached() {
	for _, a := range attachedBySurface[s] {
		a.teardown()
		delete(attachedByCanvas, a.canvas)
	}
	delete(attachedBySurface, s)
	delete(attachBGSet, s)
	delete(attachFocus, s)
}

// layoutAttached follows a resize of the surface (WM_SIZE, not minimized).
func (s *surface) layoutAttached() {
	list := attachedBySurface[s]
	if len(list) == 0 {
		return
	}
	for _, a := range list {
		a.place()
	}
	s.restackAttached()
}

// restackAttached puts canvases above or below the WebView's window:
// canvases below at the bottom (earlier ones lower), canvases above at
// the top (later ones higher).
func (s *surface) restackAttached() {
	list := attachedBySurface[s]
	const flags = swpNoMove | swpNoSize | swpNoActivate
	for i := len(list) - 1; i >= 0; i-- {
		if a := list[i]; !a.layer.Above && !a.closed {
			procSetWindowPos.Call(a.hwnd, attachHwndBottom, 0, 0, 0, 0, flags)
		}
	}
	for _, a := range list {
		if a.layer.Above && !a.closed {
			procSetWindowPos.Call(a.hwnd, attachHwndTop, 0, 0, 0, 0, flags)
		}
	}
}

// attachedWebReady runs once the WebView2 controller exists (setUp): its
// window was created on top of the canvases, and the background could not
// be set before.
func (s *surface) attachedWebReady() {
	if len(attachedBySurface[s]) == 0 {
		return
	}
	s.refreshWebBackground()
	s.restackAttached()
}

// hasUnderlay reports whether a canvas is attached below the WebView.
func (s *surface) hasUnderlay() bool {
	for _, a := range attachedBySurface[s] {
		if !a.layer.Above && !a.closed {
			return true
		}
	}
	return false
}

// refreshWebBackground makes the WebView's default background transparent
// while a canvas is below it and restores the appearance's colour after.
// Before the controller exists attachedWebReady applies it.
func (s *surface) refreshWebBackground() {
	if s.closed || s.controller == 0 {
		return
	}
	switch {
	case s.hasUnderlay():
		s.putBackground(0) // COREWEBVIEW2_COLOR {0, 0, 0, 0}
		attachBGSet[s] = true
	case attachBGSet[s]:
		s.putBackground(webviewColor(s.ext.app))
		delete(attachBGSet, s)
	}
}

// focusAttached gives focus back to the canvas that had it when the
// surface lost activation; false when the WebView had it.
func (s *surface) focusAttached() bool {
	a := attachFocus[s]
	if a == nil || a.closed {
		return false
	}
	canvasSetFocus.Call(a.hwnd)
	return true
}

// PresentDamage shadows the canvas's partial path: an attached canvas
// keeps its pass-through mask and layered bitmap from whole frames only.
func (a *attached) PresentDamage(img *image.RGBA, _ []image.Rectangle) error {
	return a.Present(img)
}
