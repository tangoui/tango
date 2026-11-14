//go:build linux && !android

package linux

// GPU presentation (platform.CanvasGL). A canvas surface packs its
// drawing area as the main child of a GtkOverlay (wrapGL, at creation:
// re-parenting the drawing area later would unrealize it and drop focus
// and the IM client window). UseGLArea adds a GtkGLArea as the overlay's
// only overlay child, above the drawing area, and hands it to the caller
// (internal/render/gpu/gl.NewPresenter), which connects its "render" signal and blits
// its frames into it. The drawing area keeps everything else: input, IME,
// cursor, focus, AT-SPI, frame scheduling. Only drawing changes:
//
//	cairo: tick → CanvasFrame → Present(img) → cairo surface; "draw" paints it
//	GL:    tick → CanvasFrame → (presenter: gtk_gl_area_queue_render) → Present(nil)
//
// Input. The overlay puts the GLArea into a child GdkWindow of its own,
// which gtk_overlay_set_overlay_pass_through makes pass-through
// (gdk_window_set_pass_through: not a target itself, its children may
// be). That is not enough: GtkGLArea creates an input-only event window
// inside it on realize, which would take every pointer event (verified
// under Xvfb: the drawing area got a leave event and no press). So the
// GLArea's own windows are made pass-through too, on every realize
// (passThroughGL), and GDK's picking skips both: pointer events, crossing
// events and the pointer cursor belong to the drawing area's window below.
// The GLArea never takes focus.
//
// Frames. On the GL path frames never run from the drawing area's draw
// handler (canvasGL.tick, plan file). The presenter answers a frame with
// gtk_gl_area_queue_render, a queue_draw of the GLArea; issued from inside
// GTK's paint phase it would invalidate the window while it is being
// painted, and redrawing the covered drawing area just to run a frame is
// wasted (GTK 3.24 does still paint it, verified under Xvfb). So a tick
// renders at once, in the update phase, and a size or scale change renders
// synchronously from "size-allocate" (resizeGL), in the layout phase;
// either way the render is queued before GTK paints that frame clock
// cycle, at the GLArea's new size.
//
// Release. ReleaseGLArea (uihost: the presenter could not start, or the
// GL renderer is a software rasterizer) destroys the GLArea and returns
// the canvas to the cairo path; the overlay stays, holding only the
// drawing area.
//
// CPU fallback. Present(img) still copies img into the cairo surface; on
// the GL path it also hides the GLArea, so the drawing area (which paints
// the surface again, canvasGL.paintSurface) shows it, and the next
// Present(nil) shows the GLArea again (canvasGL.present). Partial frames
// are presented whole (canvasGL.partial).
//
// Only opaque canvas surfaces switch: attached canvases (layered,
// pass-through, below a WebView) and transparent ones
// (Appearance.Transparent when UseGLArea is called) report
// ErrUnsupported. When GTK cannot give the GLArea an OpenGL 3.3 core
// context (gtk_gl_area_get_error after realize, or an older context) the
// area is destroyed and the canvas stays on cairo.

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.CanvasGL = (*canvas)(nil)
	_ platform.CanvasGL = (*attached)(nil)
)

// Required OpenGL version (core profile).
const canvasGLMajor, canvasGLMinor = 3, 3

// wrapGL makes the canvas's drawing area the main child of a new
// GtkOverlay and returns the widget to pack into the surface: the
// overlay, or the drawing area itself when the GL symbols are missing
// (UseGLArea then reports ErrUnsupported). Main thread, before packing.
func (c *canvas) wrapGL() ptr {
	if loadCanvasGLFFI() != nil {
		return c.widget
	}
	f := &canvasGLFFI
	ov := f.overlayNew()
	if ov == 0 {
		return c.widget
	}
	f.containerAdd(ov, c.widget)
	c.glOverlay = ov
	return ov
}

// transparent reports whether the canvas's surface is see-through.
func (c *canvas) transparent() bool {
	s := c.owner
	return s != nil && s.ext.appInit && s.ext.app.Transparent
}

// UseGLArea implements platform.CanvasGL.
func (c *canvas) UseGLArea() (uintptr, error) {
	if c.closed {
		return 0, canvasErrClosed
	}
	if syscall.Gettid() != c.tid {
		return 0, errors.New("tango/linux: UseGLArea must run on the main thread")
	}
	if c.gl.on {
		return c.glArea, nil
	}
	if c.gl.released {
		return 0, platform.ErrUnsupported
	}
	if !canvasGLSupport(attachByWidget[c.widget] != nil, c.transparent()) {
		return 0, platform.ErrUnsupported
	}
	if c.glOverlay == 0 {
		if err := loadCanvasGLFFI(); err != nil {
			return 0, fmt.Errorf("%w: %v", platform.ErrUnsupported, err)
		}
		return 0, platform.ErrUnsupported
	}
	f := &canvasGLFFI
	a := f.glAreaNew()
	if a == 0 {
		return 0, errors.New("tango/linux: gtk_gl_area_new failed")
	}
	canvasFFI.objectRefSink(a) // our reference, dropped in destroy (or below)
	f.glAreaSetRequired(a, canvasGLMajor, canvasGLMinor)
	if f.glAreaSetUseES != nil {
		f.glAreaSetUseES(a, false)
	}
	f.glAreaSetAutoRender(a, false)
	f.glAreaSetHasAlpha(a, false)
	f.glAreaSetHasDepth(a, false)
	f.glAreaSetHasStencil(a, false)
	f.widgetSetCanFocus(a, false)
	connect(a, "realize", 0, func(w, _ ptr) ptr { // after GtkGLArea's own realize (RUN_FIRST)
		passThroughGL(w)
		return 0
	})
	f.overlayAddOverlay(c.glOverlay, a)
	f.overlaySetPassThrough(c.glOverlay, a, true)
	f.widgetShow(a)
	// Realizing it realizes the overlay and the toplevel first if needed
	// (the window may be hidden still) and creates the GL context.
	f.widgetRealize(a)
	if err := canvasGLAreaError(a); err != nil {
		gtkWidgetDestroy(a) // removes it from the overlay
		gObjectUnref(a)
		return 0, err
	}
	c.glArea = a
	c.gl.on = true
	// From now on frames run outside the drawing area's draw handler; ask
	// for the first GPU frame from the main loop (never re-entrantly).
	c.drawWanted = false
	c.enqueue(func(c *canvas) { c.qFrame = true })
	return a, nil
}

// ReleaseGLArea implements platform.CanvasGL: it destroys the GLArea and
// goes back to the cairo path (frames from the drawing area's draw
// handler, partial presents), asking for a whole frame.
func (c *canvas) ReleaseGLArea() {
	if c.closed || syscall.Gettid() != c.tid || !c.gl.release() {
		return
	}
	a := c.glArea
	c.glArea = 0
	gtkWidgetDestroy(a) // removes it from the overlay, unrealizes it (its context goes)
	gObjectUnref(a)     // the reference taken in UseGLArea
	// The cairo surface may be stale or empty: the next draw renders.
	c.drawWanted = true
	c.enqueue(func(c *canvas) { c.qFrame = true })
}

// passThroughGL makes the realized GLArea's own GdkWindows (its
// input-only event window) pass-through, so pointer input reaches the
// drawing area below.
func passThroughGL(area ptr) {
	f := &canvasGLFFI
	parent := f.widgetGetParentWindow(area)
	if parent == 0 {
		return
	}
	list := f.windowGetChildren(parent)
	for l := list; l != 0; l = (*gListNode)(toPtr(l)).next {
		win := (*gListNode)(toPtr(l)).data
		var owner ptr
		f.windowGetUserData(win, &owner)
		if owner == area {
			f.windowSetPassThrough(win, true)
		}
	}
	f.listFree(list)
}

// gListNode mirrors GList { gpointer data; GList *next, *prev; }.
type gListNode struct{ data, next, prev ptr }

// canvasGLAreaError checks a GLArea after realize: GTK's error, a realize
// that did not happen, or a context older than 3.3.
func canvasGLAreaError(a ptr) error {
	f := &canvasGLFFI
	if e := f.glAreaGetError(a); e != 0 {
		// const GError* owned by the area: read, do not free.
		return fmt.Errorf("tango/linux: UseGLArea: %s", goStr((*gErrorLayout)(toPtr(e)).message))
	}
	if !canvasFFI.widgetGetRealized(a) {
		return errors.New("tango/linux: UseGLArea: the GtkGLArea could not be realized")
	}
	ctx := f.glAreaGetContext(a)
	if ctx == 0 {
		return errors.New("tango/linux: UseGLArea: the GtkGLArea has no GL context")
	}
	// GDK reads the version when the context is first made current.
	f.glAreaMakeCurrent(a)
	if e := f.glAreaGetError(a); e != 0 {
		return fmt.Errorf("tango/linux: UseGLArea: %s", goStr((*gErrorLayout)(toPtr(e)).message))
	}
	var major, minor int32
	f.glContextGetVersion(ctx, &major, &minor)
	if major < canvasGLMajor || (major == canvasGLMajor && minor < canvasGLMinor) {
		return fmt.Errorf("tango/linux: UseGLArea: OpenGL %d.%d, need %d.%d core", major, minor, canvasGLMajor, canvasGLMinor)
	}
	return nil
}

// UseGLArea implements platform.CanvasGL: an attached canvas shares its
// overlay with a WebView (layered, maybe pass-through), so it stays on
// cairo.
func (a *attached) UseGLArea() (uintptr, error) { return 0, platform.ErrUnsupported }

// ReleaseGLArea implements platform.CanvasGL: nothing to release.
func (a *attached) ReleaseGLArea() {}

// presentGL applies Present(img) (cpu true, the frame is in the cairo
// surface already) or Present(nil) to the GLArea.
func (c *canvas) presentGL(cpu bool) {
	switch c.gl.present(cpu) {
	case glAreaHide:
		canvasGLFFI.widgetHide(c.glArea)
	case glAreaShow:
		canvasGLFFI.widgetShow(c.glArea)
	}
}

// resizeGL runs the frame that follows a size or scale change on the GL
// path, synchronously (see the file comment). Inside a frame or a draw it
// is requested from the next tick instead.
func (c *canvas) resizeGL() {
	if c.inFrame || c.inDraw {
		c.requestFrame()
		return
	}
	c.drawWanted = false
	c.sched.drew()
	c.frame()
}
