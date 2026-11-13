//go:build linux && !android

package linux

// Canvases attached to WebView surfaces (docs/specs/canvas-surface.md,
// step D). The canvas is the GtkDrawingArea of canvas_linux.go (frame
// scheduling, cairo surface, input, IME) added to a GtkOverlay that holds
// the web view; this file adds placement, stacking, pass-through and
// focus.
//
// Widget tree. A WebView surface packs a GtkOverlay into its content box
// (packWeb) and adds the WKWebView as the overlay's first *overlay* child,
// not as its main child: the main child is always drawn below every
// overlay, so a canvas below the page would otherwise need the web view
// re-parented (unrealize/realize of WebKit's widget) whenever the first
// one is attached or SetLayer flips it. With every layer an overlay child,
// stacking is just the overlay list order: canvases below the page are
// moved to the front of the list, canvases above it to the end, with
// gtk_overlay_reorder_overlay (GTK 3.18+), which restacks the overlay
// children's GdkWindows; their order decides both drawing and event
// delivery. The overlay has no main child, so it measures as 0×0 and its
// allocation is what the box gives it (the content area).
//
// Placement. The overlay's "get-child-position" signal places each canvas:
// zero Bounds fill the overlay, non-zero Bounds follow Anchor relative to
// the content size when the layer was set. The web view keeps the default
// placement (fill). A new allocation reaches the canvas as size-allocate,
// which reports CanvasResized and draws a new frame.
//
// Transparency. A canvas paints its frame with CAIRO_OPERATOR_OVER on a
// widget that is app-paintable (no theme background), so the page shows
// through transparent pixels of a canvas above it. The canvas shares the
// toplevel's visual: blending happens inside GDK's client-side window
// tree, so it needs no RGBA visual (that is only needed to see the desktop
// through the window, Appearance.Transparent). While a canvas is below the
// page the web view's background is transparent
// (webkit_web_view_set_background_color alpha 0); the page must not paint
// one either.
//
// Pass-through (Above and PassThrough). Present keeps a mask of the
// frame's pixels with alpha >= HitThreshold and turns it into the canvas
// widget's input shape (gtk_widget_input_shape_combine_region; rectangles
// merged per band, rebuilt when the mask changes and at most 30 times a
// second). The overlay child window around the canvas is made pass-through
// (gtk_overlay_set_overlay_pass_through), so a point outside the input
// shape hits nothing in the canvas's branch and GDK delivers it to the
// next window below: the web view. Before the first frame the shape is
// empty: the whole canvas passes through.
//
// Focus. A click on the canvas grabs focus (canvas_input_linux.go); a
// click on the page lets WebKit grab it back; GtkWindow restores whichever
// had it when the window is activated again. Detaching a focused canvas
// focuses the web view. Unlike a canvas surface, attaching does not take
// focus.

import (
	"errors"
	"fmt"
	"image"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/purego"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.CanvasAttacher = (*backend)(nil)
	_ platform.AttachedCanvas = (*attached)(nil)
)

var errDetached = errors.New("tango/linux: canvas detached")

const attachRegionInterval = time.Second / 30

var attachFFI struct {
	once  sync.Once
	err   error // core symbols missing: no overlay, no attaching
	cairo ptr

	overlayNew            func() ptr
	overlayAddOverlay     func(o, w ptr)
	overlayReorder        func(o, w ptr, pos int32) // 3.18
	overlaySetPassThrough func(o, w ptr, on bool)   // 3.18
	widgetInputShape      func(w, region ptr)
	widgetQueueResize     func(w ptr)
	widgetSetAppPaintable func(w ptr, on bool)
	widgetShow            func(w ptr)
	widgetGetParentWindow func(w ptr) ptr
	widgetGetWindow       func(w ptr) ptr
	widgetGetAllocW       func(w ptr) int32
	widgetGetAllocH       func(w ptr) int32
	widgetGrabFocus       func(w ptr)
	widgetHasFocus        func(w ptr) bool
	windowRaise           func(win ptr)
	regionCreate          func() ptr
	regionCreateRects     func(rects *canvasRect, n int32) ptr
	regionDestroy         func(r ptr)

	posCB ptr // get-child-position trampoline
}

// loadAttachFFI binds the symbols once; main thread, after Init.
func loadAttachFFI() error {
	f := &attachFFI
	f.once.Do(func() {
		if libGTK == 0 {
			f.err = errors.New("tango/linux: attach used before Init")
			return
		}
		gdk, _, _ := open("libgdk-3.so.0")
		var err error
		if f.cairo, _, err = open("libcairo.so.2"); err != nil {
			f.err = err
			return
		}
		var bd binder
		t, c := libGTK, f.cairo
		bd.req(&f.overlayNew, "gtk_overlay_new", t)
		bd.req(&f.overlayAddOverlay, "gtk_overlay_add_overlay", t)
		bd.req(&f.overlayReorder, "gtk_overlay_reorder_overlay", t)
		bd.req(&f.overlaySetPassThrough, "gtk_overlay_set_overlay_pass_through", t)
		bd.req(&f.widgetInputShape, "gtk_widget_input_shape_combine_region", t)
		bd.req(&f.widgetQueueResize, "gtk_widget_queue_resize", t)
		bd.req(&f.widgetSetAppPaintable, "gtk_widget_set_app_paintable", t)
		bd.req(&f.widgetShow, "gtk_widget_show", t)
		bd.req(&f.widgetGetParentWindow, "gtk_widget_get_parent_window", t)
		bd.req(&f.widgetGetWindow, "gtk_widget_get_window", t)
		bd.req(&f.widgetGetAllocW, "gtk_widget_get_allocated_width", t)
		bd.req(&f.widgetGetAllocH, "gtk_widget_get_allocated_height", t)
		bd.req(&f.widgetGrabFocus, "gtk_widget_grab_focus", t)
		bd.req(&f.widgetHasFocus, "gtk_widget_has_focus", t)
		bd.req(&f.windowRaise, "gdk_window_raise", gdk, t)
		bd.req(&f.regionCreate, "cairo_region_create", c)
		bd.req(&f.regionCreateRects, "cairo_region_create_rectangles", c)
		bd.req(&f.regionDestroy, "cairo_region_destroy", c)
		if len(bd.missing) > 0 {
			f.err = fmt.Errorf("tango/linux: attach: missing native symbols: %v", bd.missing)
			return
		}
		// gboolean get-child-position (GtkOverlay*, GtkWidget*, GdkRectangle*, gpointer)
		f.posCB = purego.NewCallback(func(overlay, widget, alloc, _ ptr) ptr {
			return attachChildPosition(overlay, widget, alloc)
		})
	})
	return f.err
}

// attachHost is the overlay of one WebView surface and its canvases.
type attachHost struct {
	s       *surface
	overlay ptr
	list    []*attached // in attach order
	bgSet   bool        // the web view's background was made transparent
}

var (
	attachHosts     = map[*surface]*attachHost{}
	attachByOverlay = map[ptr]*attachHost{}
	attachByWidget  = map[ptr]*attached{}
)

// packWeb packs the web view into the content box: inside a GtkOverlay
// when the symbols exist (canvases can then be attached), else directly.
// Called by newSurface.
func (s *surface) packWeb() {
	if loadAttachFFI() != nil {
		gtkBoxPackStart(s.box, s.web, true, true, 0)
		return
	}
	f := &attachFFI
	ov := f.overlayNew()
	if ov == 0 {
		gtkBoxPackStart(s.box, s.web, true, true, 0)
		return
	}
	gtkBoxPackStart(s.box, ov, true, true, 0)
	f.overlayAddOverlay(ov, s.web) // an overlay child, not the main child: see the file comment
	gSignalConnectData(ov, "get-child-position", f.posCB, 0, 0, 0)
	h := &attachHost{s: s, overlay: ov}
	attachHosts[s] = h
	attachByOverlay[ov] = h
}

// attached is a canvas attached to a WebView surface. Main thread only.
type attached struct {
	*canvas
	s     *surface
	h     *attachHost
	layer platform.CanvasLayer
	// Content (overlay) size when layer was set; Anchor offsets are
	// relative to it. 0: take the size at the first placement.
	baseW, baseH int

	mask         []byte
	maskW, maskH int
	maskThr      uint8
	maskScale    float64
	shapeDirty   bool
	shapeArmed   bool
	shapeAt      time.Time
	hasShape     bool
}

// AttachCanvas implements platform.CanvasAttacher. Main thread. The sink
// is first called from the main loop.
func (b *backend) AttachCanvas(sid platform.SurfaceID, layer platform.CanvasLayer, sink platform.CanvasSink) (platform.Canvas, error) {
	if sink == nil {
		return nil, errors.New("tango/linux: AttachCanvas: nil sink")
	}
	if !b.loaded.Load() {
		return nil, errors.New("tango/linux: AttachCanvas before Init")
	}
	s := b.surfaces[sid]
	if s == nil || s.closed {
		return nil, fmt.Errorf("tango/linux: AttachCanvas: no open surface %d", sid)
	}
	if s.web == 0 {
		return nil, errors.New("tango/linux: AttachCanvas: not a WebView surface")
	}
	h := attachHosts[s]
	if h == nil {
		if err := loadAttachFFI(); err != nil {
			return nil, err
		}
		return nil, errors.New("tango/linux: AttachCanvas: the web view is not in an overlay")
	}
	c, widget, err := newCanvasWidget(sink)
	if err != nil {
		return nil, err
	}
	f := &attachFFI
	a := &attached{canvas: c, s: s, h: h, layer: layer}
	a.baseW, a.baseH = h.size()
	f.widgetSetAppPaintable(widget, true)
	attachByWidget[widget] = a
	connect(widget, "destroy", 0, func(_, _ ptr) ptr {
		delete(attachByWidget, widget)
		return 0
	})
	h.list = append(h.list, a)
	f.overlayAddOverlay(h.overlay, widget)
	f.widgetShow(widget)
	h.restack()
	a.applyPassThrough()
	if !layer.Above {
		s.refreshWebBackground()
	}
	// As canvas.start, without taking focus from the page.
	c.w, c.h, c.scale = c.geometry()
	c.ready = true
	c.enqueue(func(c *canvas) {
		c.qFocus = true
		c.qFrame = true
	})
	return a, nil
}

// size is the overlay's (content area's) allocated size.
func (h *attachHost) size() (int, int) {
	f := &attachFFI
	w, ht := int(f.widgetGetAllocW(h.overlay)), int(f.widgetGetAllocH(h.overlay))
	if w <= 1 && ht <= 1 {
		return 0, 0 // not allocated yet
	}
	return w, ht
}

// restack orders the overlay children: canvases below the page first
// (earlier ones lower), then the web view, then canvases above it.
func (h *attachHost) restack() {
	f := &attachFFI
	var order []ptr
	for _, a := range h.list {
		if !a.layer.Above && !a.closed {
			order = append(order, a.widget)
		}
	}
	nBelow := len(order)
	for _, a := range h.list {
		if a.layer.Above && !a.closed {
			order = append(order, a.widget)
		}
	}
	for i, w := range order {
		pos := int32(i)
		if i >= nBelow {
			pos = int32(i + 1) // after the web view
		}
		f.overlayReorder(h.overlay, w, pos)
	}
	// gtk_overlay_reorder_overlay restacks the child windows itself; raise
	// them in order as well in case a GTK version only reorders the list.
	s := h.s
	raise := func(w ptr) {
		if win := f.widgetGetParentWindow(w); win != 0 && win != f.widgetGetWindow(h.overlay) {
			f.windowRaise(win)
		}
	}
	for i, w := range order {
		if i == nBelow {
			raise(s.web)
		}
		raise(w)
	}
	if nBelow == len(order) {
		raise(s.web)
	}
}

// attachChildPosition handles "get-child-position": canvases get their
// layer's rectangle, everything else the default placement.
func attachChildPosition(overlay, widget, alloc ptr) ptr {
	h := attachByOverlay[overlay]
	a := attachByWidget[widget]
	if h == nil || a == nil || a.h != h || a.closed || alloc == 0 {
		return 0
	}
	w, ht := h.size()
	if a.baseW == 0 && a.baseH == 0 {
		a.baseW, a.baseH = w, ht
	}
	r := attachBounds(a.layer, a.baseW, a.baseH, w, ht)
	out := (*gdkRect)(toPtr(alloc))
	*out = gdkRect{X: int32(r.X), Y: int32(r.Y), Width: int32(max(r.Width, 1)), Height: int32(max(r.Height, 1))}
	return 1
}

func (a *attached) passThrough() bool { return a.layer.Above && a.layer.PassThrough }

// applyPassThrough sets the overlay child's pass-through flag and the
// input shape for the current layer.
func (a *attached) applyPassThrough() {
	attachFFI.overlaySetPassThrough(a.h.overlay, a.widget, a.passThrough())
	a.applyShape()
}

// Present implements platform.Canvas and keeps the frame's alpha for the
// input shape.
func (a *attached) Present(img *image.RGBA) error {
	if a.closed {
		return errDetached
	}
	if err := a.canvas.Present(img); err != nil {
		return err
	}
	if !a.passThrough() {
		a.mask, a.maskW, a.maskH = nil, 0, 0
		return nil
	}
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	thr := a.layer.HitThreshold()
	fresh := w != a.maskW || h != a.maskH || thr != a.maskThr || a.scale != a.maskScale
	if fresh {
		a.mask = make([]byte, w*h)
		a.maskW, a.maskH, a.maskThr, a.maskScale = w, h, thr, a.scale
	}
	if attachHitMask(a.mask, img.Pix[img.PixOffset(r.Min.X, r.Min.Y):], img.Stride, w, h, thr) || fresh {
		a.scheduleShape()
	}
	return nil
}

// scheduleShape rebuilds the input shape from the main loop (Present runs
// inside "draw"): at the next idle moment, or 1/30 s after the last
// rebuild if that is later.
func (a *attached) scheduleShape() {
	a.shapeDirty = true
	if a.shapeArmed {
		return
	}
	a.shapeArmed = true
	var ms uint32
	if wait := attachRegionInterval - time.Since(a.shapeAt); wait > 0 {
		ms = uint32(wait/time.Millisecond) + 1
	}
	later(ms, func() {
		a.shapeArmed = false
		if a.shapeDirty {
			a.applyShape()
		}
	})
}

// applyShape sets the canvas's input shape from the mask (pass-through),
// or removes it.
func (a *attached) applyShape() {
	a.shapeDirty = false
	a.shapeAt = time.Now()
	if a.closed {
		return
	}
	f := &attachFFI
	if !a.passThrough() {
		if a.hasShape {
			f.widgetInputShape(a.widget, 0)
			a.hasShape = false
		}
		return
	}
	rects := attachMaskRects(a.mask, a.maskW, a.maskH)
	var region ptr
	if len(rects) == 0 {
		region = f.regionCreate()
	} else {
		// Device pixels to the widget's logical pixels, rounding outwards.
		sc := max(a.maskScale, 1)
		lr := make([]canvasRect, len(rects))
		for i, r := range rects {
			x0 := int32(math.Floor(float64(r[0]) / sc))
			y0 := int32(math.Floor(float64(r[1]) / sc))
			x1 := int32(math.Ceil(float64(r[0]+r[2]) / sc))
			y1 := int32(math.Ceil(float64(r[1]+r[3]) / sc))
			lr[i] = canvasRect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
		}
		region = f.regionCreateRects(&lr[0], int32(len(lr)))
	}
	if region == 0 {
		return
	}
	f.widgetInputShape(a.widget, region) // GTK copies it
	f.regionDestroy(region)
	a.hasShape = true
}

// SetLayer implements platform.AttachedCanvas. Main thread.
func (a *attached) SetLayer(l platform.CanvasLayer) error {
	if a.closed {
		return errDetached
	}
	old := a.layer
	wasPass := a.passThrough()
	a.layer = l
	a.baseW, a.baseH = a.h.size()
	if l.Above != old.Above {
		a.h.restack()
		a.s.refreshWebBackground()
	}
	if a.passThrough() != wasPass || l.HitThreshold() != old.HitThreshold() {
		a.mask, a.maskW, a.maskH = nil, 0, 0
		a.applyPassThrough()
		if a.passThrough() {
			a.Invalidate() // the next frame brings a mask
		}
	}
	attachFFI.widgetQueueResize(a.widget) // get-child-position runs again
	return nil
}

// Detach implements platform.AttachedCanvas. Main thread; idempotent.
func (a *attached) Detach() {
	if a.closed {
		return
	}
	s, h := a.s, a.h
	focused := attachFFI.widgetHasFocus(a.widget)
	widget := a.widget
	a.mask = nil
	a.canvas.destroy() // closed: no sink calls; drops our widget reference
	for i, x := range h.list {
		if x == a {
			h.list = append(h.list[:i:i], h.list[i+1:]...)
			break
		}
	}
	gtkWidgetDestroy(widget) // removes it from the overlay
	if s.closed {
		return
	}
	if focused {
		attachFFI.widgetGrabFocus(s.web)
	}
	if !a.layer.Above {
		s.refreshWebBackground()
	}
}

// ---- surface side ----

// destroyAttached runs from the window's "destroy", before SurfaceClosed;
// GTK destroys the widgets with the window.
func (s *surface) destroyAttached() {
	h := attachHosts[s]
	if h == nil {
		return
	}
	for _, a := range h.list {
		a.mask = nil
		a.canvas.destroy()
	}
	h.list = nil
	delete(attachByOverlay, h.overlay)
	delete(attachHosts, s)
}

// refreshWebBackground makes the web view's background transparent while
// a canvas is below it and restores the appearance's background after.
func (s *surface) refreshWebBackground() {
	h := attachHosts[s]
	if h == nil || s.closed || s.web == 0 || !wx.ok {
		return
	}
	under := false
	for _, a := range h.list {
		under = under || (!a.layer.Above && !a.closed)
	}
	switch {
	case under:
		clear := [4]float64{}
		wx.webViewSetBackground(s.web, &clear)
		h.bgSet = true
	case h.bgSet:
		app := s.ext.app
		if !s.ext.appInit {
			app = defaultAppearance
		}
		s.applyBackground(app)
		h.bgSet = false
	}
}

// PresentDamage shadows the canvas's partial path: an attached canvas
// keeps its input shape from whole frames only.
func (a *attached) PresentDamage(img *image.RGBA, _ []image.Rectangle) error {
	return a.Present(img)
}
