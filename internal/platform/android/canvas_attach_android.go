//go:build android

package android

/*
#include <stdlib.h>
#include "jni_android.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"math"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Canvases attached to WebView pages (docs/specs/mobile-canvas.md §1.4,
// uihost.Attach). The Java side is a CanvasOverlay: a CanvasView holding a
// translucent TextureView, added to the page's frame (Page.Frame) above or
// below the WebView. Its Surface gives an ANativeWindow exactly as the
// SurfaceView of a canvas page does, so frames, input, text input and
// accessibility are the canvas code's (canvas_android.go and friends); the
// buffers are premultiplied RGBA_8888 instead of RGBX, and presentation is
// on the CPU only.
//
// Placement. Fixed Bounds are converted to pixels and kept at their anchor
// by Go: Page.Frame asks nativePlace on every layout, which applies
// CanvasLayer.AnchoredBounds from the frame size the bounds were given for
// (the size at the first layout after Attach or SetLayer) to the current
// one (canvas_attach_plan.go). Zero Bounds fill the frame.
//
// Pass-through. The overlay keeps the alpha of its last frame that reached
// the screen (hitMask). CanvasView.dispatchTouchEvent asks nativeHit on
// ACTION_DOWN; below the layer's HitThreshold it refuses the event, so the
// frame offers the whole gesture to the WebView underneath. Pointer
// streams (mouse, trackpad: hover and scroll) are decided the same way,
// once at the stream's start, so a scroll does not hop between the overlay
// and the page as it crosses transparent pixels. The mask is dropped
// (everything passes through) when the Surface goes away or changes size
// and when a presentation fails, until the next frame is on screen.
//
// Below the WebView (Above false) the WebView's background is transparent
// while such a canvas is attached; the page must not paint one either.

const canvasOverlayClassName = "org/tango/host/CanvasOverlay"

var canvasOverlayClass C.jclass

var (
	mCanvasNewOverlay = &jmethod{cls: &canvasClass, static: true, name: "newOverlay",
		sig: "(Lorg/tango/host/Page;JZZIIII)Lorg/tango/host/CanvasOverlay;"}
	mOvSetLayer = &jmethod{cls: &canvasOverlayClass, name: "setLayer", sig: "(ZZIIII)V"}
	mOvDetach   = &jmethod{cls: &canvasOverlayClass, name: "detach", sig: "()V"}
)

// overlayJava: the Java host can attach canvases to WebView pages.
var overlayJava bool

// lookupOverlay resolves the overlay class and methods. Main thread, from
// lookupCanvas. A host without them keeps working; AttachCanvas then
// reports ErrUnsupported.
func lookupOverlay(e env) {
	if canvasClass == 0 {
		return
	}
	n := C.CString(canvasOverlayClassName)
	canvasOverlayClass = C.tg_find_class(e.p, n)
	C.free(unsafe.Pointer(n))
	if canvasOverlayClass == 0 {
		logf(logWarn, "%v: attached canvases (uihost.Attach) disabled", errJavaClass(canvasOverlayClassName))
		return
	}
	for _, m := range []*jmethod{mCanvasNewOverlay, mOvSetLayer, mOvDetach} {
		name, sig := C.CString(m.name), C.CString(m.sig)
		st := C.int(0)
		if m.static {
			st = 1
		}
		m.id = C.tg_method(e.p, *m.cls, name, sig, st)
		C.free(unsafe.Pointer(name))
		C.free(unsafe.Pointer(sig))
		if m.id == nil {
			logf(logWarn, "%v: attached canvases (uihost.Attach) disabled", errJavaClass(m.name+m.sig))
			return
		}
	}
	overlayJava = true
}

var (
	_ platform.CanvasAttacher = (*Backend)(nil)
	_ platform.AttachedCanvas = (*overlay)(nil)

	errDetached = errors.New("tango/android: canvas detached")
)

// overlayIDBase starts the ids of attached canvases, which share the
// canvases map and the Java TangoCanvas registry with canvas pages (keyed
// by SurfaceID): far above any SurfaceID.
const overlayIDBase = 1 << 40

var overlaySeq int64 // main thread

// overlay is a canvas attached to a WebView page. Main thread only.
type overlay struct {
	*canvas
	s     *surface
	layer platform.CanvasLayer

	// Frame size (px) the layer's Bounds were given for; 0 until the first
	// layout after Attach / SetLayer. lastW, lastH: the size at the last
	// layout.
	refW, refH   int
	lastW, lastH int

	mask hitMask
}

// AttachCanvas implements platform.CanvasAttacher.
func (b *Backend) AttachCanvas(sid platform.SurfaceID, layer platform.CanvasLayer, sink platform.CanvasSink) (platform.Canvas, error) {
	if sink == nil {
		return nil, errors.New("tango/android: AttachCanvas: nil sink")
	}
	if canvasClass == 0 || !overlayJava {
		return nil, fmt.Errorf("tango/android: the Java host cannot attach canvases (copy mobile/android again): %w", platform.ErrUnsupported)
	}
	if !onMainThread() {
		return nil, errors.New("tango/android: AttachCanvas must run on the main thread")
	}
	s := b.surface(sid)
	if s == nil || s.closed {
		if canvases[int64(sid)] != nil {
			return nil, fmt.Errorf("tango/android: AttachCanvas: surface %d is a canvas page, not a WebView page: %w", sid, platform.ErrUnsupported)
		}
		return nil, fmt.Errorf("tango/android: AttachCanvas: no open surface %d", sid)
	}
	overlaySeq++
	id := platform.SurfaceID(overlayIDBase + overlaySeq)
	c := &canvas{b: b, id: id, sink: sink, density: 1}
	o := &overlay{canvas: c, s: s, layer: layer}
	c.ov = o
	c.sched.Background() // no frames until the TextureView's Surface exists
	canvases[int64(id)] = c
	var err error
	ok := withEnv(func(e env) {
		var obj C.jobject
		r := layer.Bounds
		obj, err = e.callStaticObject(mCanvasNewOverlay, jvObj(s.page), jvLong(int64(id)),
			jvBool(layer.Above), jvBool(layer.PassThrough), jvInt(r.X), jvInt(r.Y), jvInt(r.Width), jvInt(r.Height))
		if err == nil && obj == 0 {
			err = errors.New("tango/android: AttachCanvas: the page is gone")
		}
		if err == nil {
			c.page = e.global(obj)
		}
	})
	if !ok && err == nil {
		err = errJava
	}
	if err != nil {
		delete(canvases, int64(id))
		c.sched.Close()
		return nil, err
	}
	s.overlays = append(s.overlays, o)
	if canvasDebug > 0 {
		logf(logInfo, "canvas %d: attached to surface %d: %+v", c.id, sid, layer)
	}
	return o, nil
}

func (o *overlay) passThrough() bool { return o.layer.Above && o.layer.PassThrough }

// Present implements platform.Canvas and keeps the frame's alpha for
// pass-through hit testing.
func (o *overlay) Present(img *image.RGBA) error {
	if o.closed {
		return o.detachedErr()
	}
	err := o.canvas.Present(img)
	o.keepMask(img, nil, err)
	return err
}

// PresentDamage implements platform.CanvasDamagePresenter; the mask is
// updated over the dirty region only.
func (o *overlay) PresentDamage(img *image.RGBA, dirty []image.Rectangle) error {
	if o.closed {
		return o.detachedErr()
	}
	err := o.canvas.PresentDamage(img, dirty)
	if img == nil || err != nil {
		o.keepMask(img, nil, err)
		return err
	}
	rects, full := platform.DamageRects(dirty, img.Bounds(), damageMaxRects)
	switch {
	case full:
		o.keepMask(img, nil, nil)
	case len(rects) > 0:
		u := rects[0]
		for _, r := range rects[1:] {
			u = u.Union(r)
		}
		o.keepMask(img, &u, nil)
	}
	return err
}

// detachedErr is Present's error after Detach or the page's closing; a
// frame during which the page closed is dropped quietly.
func (o *overlay) detachedErr() error {
	if o.closedInFrame {
		return nil
	}
	return errDetached
}

// keepMask records a presentation for pass-through hit testing (see
// hitMask.presented).
func (o *overlay) keepMask(img *image.RGBA, r *image.Rectangle, err error) {
	if !o.passThrough() {
		o.mask.reset()
		return
	}
	o.mask.presented(img, r, err)
}

// surfaceGone forgets the mask when the overlay's Surface is destroyed or
// changes size: until the next frame is on screen nothing of the overlay
// is known to be there. Main thread.
func (o *overlay) surfaceGone() { o.mask.reset() }

// UseNativeWindow: attached canvases present on the CPU only.
func (o *overlay) UseNativeWindow() (uintptr, error) {
	return 0, fmt.Errorf("tango/android: attached canvases present on the CPU: %w", platform.ErrUnsupported)
}

// SetLayer implements platform.AttachedCanvas.
func (o *overlay) SetLayer(layer platform.CanvasLayer) error {
	if !onMainThread() {
		return errors.New("tango/android: SetLayer must run on the main thread")
	}
	if o.closed {
		return errDetached
	}
	o.layer = layer
	o.refW, o.refH = o.lastW, o.lastH // the bounds are for the current size
	r := layer.Bounds
	withEnv(func(e env) {
		e.callVoid(o.page, mOvSetLayer, jvBool(layer.Above), jvBool(layer.PassThrough),
			jvInt(r.X), jvInt(r.Y), jvInt(r.Width), jvInt(r.Height))
	})
	if !o.passThrough() {
		o.mask.reset()
	} else if o.mask.w == 0 {
		o.Invalidate() // a mask arrives with the next frame
	}
	return nil
}

// Detach implements platform.AttachedCanvas.
func (o *overlay) Detach() {
	if o.closed {
		return
	}
	withEnv(func(e env) {
		// Removing the view destroys the TextureView's Surface:
		// nativeSurfaceDestroyed releases the window on the way.
		e.callVoid(o.page, mOvDetach)
		o.drop(e)
	})
}

// drop forgets the overlay (Detach, or its page closed). Main thread.
func (o *overlay) drop(e env) {
	o.teardown(e)
	o.mask = hitMask{}
	s := o.s
	for i, x := range s.overlays {
		if x == o {
			s.overlays = append(s.overlays[:i], s.overlays[i+1:]...)
			break
		}
	}
}

// dropOverlays forgets the canvases of a closed page; the Java side
// stopped them with the page. Main thread.
func (s *surface) dropOverlays(e env) {
	for len(s.overlays) > 0 {
		s.overlays[len(s.overlays)-1].drop(e)
	}
}

const noPlace = math.MinInt64

// nativePlace: Page.Frame lays out an overlay with fixed bounds in a frame
// of w×h pixels.
//
//export Java_org_tango_host_TangoCanvas_nativePlace
func Java_org_tango_host_TangoCanvas_nativePlace(penv *C.JNIEnv, cls C.jclass, id C.jlong, w, h C.jint, density C.jfloat) C.jlong {
	c := canvasFor(id)
	if c == nil || c.ov == nil {
		return noPlace
	}
	o := c.ov
	o.lastW, o.lastH = int(w), int(h)
	if o.refW <= 0 || o.refH <= 0 {
		o.refW, o.refH = o.lastW, o.lastH
	}
	r, ok := overlayRect(o.layer, float32(density), o.refW, o.refH, o.lastW, o.lastH)
	if !ok {
		return noPlace
	}
	if canvasDebug > 0 {
		logf(logDebug, "canvas %d: placed at %v in %dx%d px", c.id, r, o.lastW, o.lastH)
	}
	return C.jlong(packPoint(r.Min))
}

// nativeHit: whether a gesture starting at (x, y), view pixels, belongs
// to the overlay (pass-through refuses it over transparent pixels).
//
//export Java_org_tango_host_TangoCanvas_nativeHit
func Java_org_tango_host_TangoCanvas_nativeHit(penv *C.JNIEnv, cls C.jclass, id C.jlong, x, y C.jfloat) C.jboolean {
	c := canvasFor(id)
	if c == nil || c.ov == nil {
		return C.JNI_FALSE
	}
	o := c.ov
	hit := !o.passThrough() || o.mask.hit(float32(x), float32(y), o.layer.HitThreshold())
	if canvasDebug > 0 {
		logf(logInfo, "canvas %d: hit test (%.0f, %.0f): %v", c.id, float32(x), float32(y), hit)
	}
	if hit {
		return C.JNI_TRUE
	}
	return C.JNI_FALSE
}
