//go:build android

package android

/*
#cgo LDFLAGS: -landroid
#include <stdlib.h>
#include "jni_android.h"
#include "canvas_android.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/mobilecanvas"
	"github.com/tangoui/tango/internal/render/gpu/gl/nativewindow"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Canvas surfaces on Android (docs/specs/mobile-canvas.md §1.3, M2): a
// CanvasPage in the Activity's page stack whose CanvasView holds a
// full-page SurfaceView. Frames are drawn on the CPU by the runner and
// copied into the Surface's ANativeWindow buffers, or, with GPU
// presentation (canvas_gl_android.go, M5), drawn with OpenGL ES into it.
//
// Threads. Everything but Invalidate/InvalidateAt runs on the Android main
// thread, inside the JNI callbacks of org.tango.host.TangoCanvas. The
// frame scheduler is mobilecanvas.Sched: Invalidate records the request and,
// only when the canvas goes from idle to pending, asks Java to post a wake
// to the main Looper (TangoCanvas.requestFrame). The wake runs Sched.Wake,
// which resumes the Choreographer frame callback; each vsync runs
// Sched.Tick and, when a frame is due, CanvasFrame. The scheduler's answers
// (link on/off, deadline timer) go back to Java as the result of each
// native call, so Go never calls into Java from inside them.

const (
	canvasClassName     = "org/tango/host/TangoCanvas"
	canvasPageClassName = "org/tango/host/CanvasPage"
	canvasHostClassName = "org/tango/host/CanvasHost"
)

// canvasHostClass is the interface both canvas kinds implement (CanvasPage
// for pages, CanvasOverlay for canvases attached to WebView pages): the
// methods every canvas calls on its Java object are looked up on it.
var canvasClass, canvasPageClass, canvasHostClass C.jclass

var (
	// TangoCanvas (static).
	mCanvasNewPage      = &jmethod{cls: &canvasClass, static: true, name: "newPage", sig: "(JZLjava/lang/String;)Lorg/tango/host/CanvasPage;"}
	mCanvasRequestFrame = &jmethod{cls: &canvasClass, static: true, name: "requestFrame", sig: "(J)V"}

	// CanvasPage (instance).
	mCvShow     = &jmethod{cls: &canvasPageClass, name: "show", sig: "()V"}
	mCvHide     = &jmethod{cls: &canvasPageClass, name: "hide", sig: "()V"}
	mCvClose    = &jmethod{cls: &canvasPageClass, name: "close", sig: "()V"}
	mCvFocus    = &jmethod{cls: &canvasPageClass, name: "focus", sig: "()V"}
	mCvSetTitle = &jmethod{cls: &canvasPageClass, name: "setTitle", sig: "(Ljava/lang/String;)V"}
	mCvView     = &jmethod{cls: &canvasPageClass, name: "view", sig: "()Landroid/view/View;"}

	// CanvasHost (instance, both kinds).
	mCvSetCursor = &jmethod{cls: &canvasHostClass, name: "setCursor", sig: "(I)V"}
	mCvSetBack   = &jmethod{cls: &canvasHostClass, name: "setBackHandling", sig: "(Z)V"}
)

var canvasMethods = []*jmethod{mCanvasNewPage, mCanvasRequestFrame, mCvShow, mCvHide, mCvClose, mCvFocus, mCvSetTitle, mCvView,
	mCvSetCursor, mCvSetBack, mCvTextInput, mCvMirror}

// lookupCanvas resolves the canvas classes and methods. Main thread, from
// nativeInit. A host without them (an older copy of mobile/android) keeps
// working; NewCanvasSurface then reports ErrUnsupported.
func lookupCanvas(e env) {
	for _, c := range []struct {
		dst  *C.jclass
		name string
	}{{&canvasClass, canvasClassName}, {&canvasPageClass, canvasPageClassName}, {&canvasHostClass, canvasHostClassName}} {
		n := C.CString(c.name)
		*c.dst = C.tg_find_class(e.p, n)
		C.free(unsafe.Pointer(n))
		if *c.dst == 0 {
			canvasClass = 0
			logf(logWarn, "%v: canvas surfaces (uihost) disabled", errJavaClass(c.name))
			return
		}
	}
	for _, m := range canvasMethods {
		name, sig := C.CString(m.name), C.CString(m.sig)
		st := C.int(0)
		if m.static {
			st = 1
		}
		m.id = C.tg_method(e.p, *m.cls, name, sig, st)
		C.free(unsafe.Pointer(name))
		C.free(unsafe.Pointer(sig))
		if m.id == nil {
			canvasClass = 0
			logf(logWarn, "%v: canvas surfaces (uihost) disabled", errJavaClass(m.name+m.sig))
			return
		}
	}
	canvasDebug = int(C.tg_cv_debug())
	rendererFromProperty() // canvas_gl_android.go
	lookupOverlay(e)       // canvas_attach_android.go
}

// canvasDebug is the canvas debug level (adb shell setprop
// debug.tango.canvas 1, read when the app starts): 1 logs input and
// frame-time summaries, 2 also every frame.
var canvasDebug int

// canvases are the live canvases by surface id. Main thread.
var canvases = map[int64]*canvas{}

var (
	_ platform.CanvasCreator = (*Backend)(nil)
	_ platform.Canvas        = (*canvas)(nil)
	_ platform.CanvasInsets  = (*canvas)(nil)

	_ platform.CanvasDamagePresenter = (*canvas)(nil)
	_ platform.Surface               = canvasSurface{}
)

// NewCanvasSurface creates a full-screen canvas page on top of the page
// stack (the first page shown is the root, as for WebView pages). Main
// thread.
func (b *Backend) NewCanvasSurface(id platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras, sink platform.CanvasSink) (platform.Surface, platform.Canvas, error) {
	if canvasClass == 0 {
		return nil, nil, fmt.Errorf("tango/android: the Java host has no canvas support (copy mobile/android again): %w", platform.ErrUnsupported)
	}
	if !onMainThread() {
		return nil, nil, errors.New("tango/android: canvas surfaces must be created on the main thread")
	}
	c := &canvas{b: b, id: id, sink: sink, density: 1}
	c.sched.Background() // no frames until the Surface exists
	canvases[int64(id)] = c
	var err error
	ok := withEnv(func(e env) {
		var page C.jobject
		page, err = e.callStaticObject(mCanvasNewPage, jvLong(int64(id)), jvBool(opts.Visible), jvObj(e.str(opts.Title)))
		if err == nil && page == 0 {
			err = errors.New("tango/android: no Activity to show the canvas in")
		}
		if err == nil {
			c.page = e.global(page)
		}
	})
	if !ok && err == nil {
		err = errJava
	}
	if err != nil {
		delete(canvases, int64(id))
		c.sched.Close()
		return nil, nil, err
	}
	return canvasSurface{c}, c, nil
}

// canvas is one canvas page. Fields are main-thread only unless noted.
type canvas struct {
	b     *Backend
	id    platform.SurfaceID
	sink  platform.CanvasSink
	sched mobilecanvas.Sched // any goroutine (Invalidate) and main thread

	page    C.jobject      // global ref to the CanvasPage
	view    C.jobject      // global ref to its CanvasView, made by Native
	win     unsafe.Pointer // ANativeWindow*, nil while there is no Surface
	wpx     int            // buffer size in device pixels
	hpx     int
	density float32
	safe    geom.Insets
	kb      geom.Insets
	textOn  bool
	caret   geom.Rect // caret of the focused text field, logical pixels
	cursor  input.Cursor
	closed  bool
	inFrame bool // inside CanvasFrame
	// closedInFrame: the page closed during a frame (a button that closes
	// it); that frame's Present is dropped quietly.
	closedInFrame bool

	// ov is set for canvases attached to a WebView page
	// (canvas_attach_android.go); nil for canvas pages.
	ov *overlay

	// gl is the native window holder of GPU presentation
	// (canvas_gl_android.go); nil while frames are copied by the CPU.
	gl *nativewindow.Holder

	needFull     bool // the next present copies the whole frame
	lastW, lastH int  // size of the last frame presented
	slowLogged   bool

	keys   canvasKeys
	motion motionDecoder
	// ime is the input-method session while text input is on
	// (canvas_ime.go); imeKeys are hardware keys whose press it took, so
	// their release is not delivered either.
	ime     imeSession
	imeKeys map[int]bool
	evs     []input.Event // scratch
	floats  []float32     // scratch

	stats frameStats

	ax canvasA11y // accessibility (canvas_a11y.go)
}

// ---- platform.Canvas ----

func (c *canvas) Size() (w, h float32) {
	return mobilecanvas.Logical(float32(c.wpx), c.density), mobilecanvas.Logical(float32(c.hpx), c.density)
}

func (c *canvas) Scale() float32 { return c.density }

func (c *canvas) Invalidate() {
	if c.sched.Invalidate() {
		c.signal()
	}
}

func (c *canvas) InvalidateAt(t time.Time) {
	if c.sched.InvalidateAt(t) {
		c.signal()
	}
}

// signal asks the main thread to run Sched.Wake (TangoCanvas.requestFrame
// posts it to the main Looper). Any goroutine.
func (c *canvas) signal() {
	if onMainThread() {
		withEnv(func(e env) { e.callStaticVoid(mCanvasRequestFrame, jvLong(int64(c.id))) })
		return
	}
	wakeCanvas(int64(c.id))
}

func (c *canvas) SetCursor(cur input.Cursor) {
	if c.closed || cur == c.cursor {
		return
	}
	c.cursor = cur
	withEnv(func(e env) { e.callVoid(c.page, mCvSetCursor, jvInt(canvasPointerIcon(cur))) })
}

// Present copies a frame into the Surface. Without one (the page is in the
// background or covered) the frame is dropped and the next one is copied
// whole.
func (c *canvas) Present(img *image.RGBA) error { return c.present(img, nil) }

// PresentDamage implements platform.CanvasDamagePresenter: only the
// bounding box of dirty is copied, and the system keeps the rest of the
// previous buffer (ANativeWindow_lock copies it back and may grow the
// region; what it reports is copied from img). Without a previous frame of
// the same size the whole frame is copied; with nothing dirty nothing is
// posted.
func (c *canvas) PresentDamage(img *image.RGBA, dirty []image.Rectangle) error {
	if c.closed || img == nil || c.win == nil {
		return c.present(img, nil)
	}
	b := img.Bounds()
	if c.needFull || b.Dx() != c.lastW || b.Dy() != c.lastH {
		return c.present(img, nil)
	}
	rects, full := platform.DamageRects(dirty, b, damageMaxRects)
	if full {
		return c.present(img, nil)
	}
	if len(rects) == 0 {
		return nil
	}
	u := rects[0]
	for _, r := range rects[1:] {
		u = u.Union(r)
	}
	return c.present(img, &u)
}

// damageMaxRects is the most dirty rectangles PresentDamage merges into
// one lock region before it copies the whole frame instead.
const damageMaxRects = 16

func (c *canvas) present(img *image.RGBA, dirty *image.Rectangle) error {
	if c.closed {
		if c.closedInFrame {
			return nil
		}
		return errors.New("tango/android: canvas closed")
	}
	if img == nil {
		return nil
	}
	if c.win == nil {
		c.needFull = true
		return nil
	}
	c.gpuYieldToCPU() // an EGL-connected window cannot be locked
	b := img.Bounds()
	if b.Empty() {
		return nil
	}
	t0 := time.Now()
	var rect [4]C.int32_t
	var rp *C.int32_t
	if dirty != nil {
		d := dirty.Sub(b.Min)
		rect = [4]C.int32_t{C.int32_t(d.Min.X), C.int32_t(d.Min.Y), C.int32_t(d.Max.X), C.int32_t(d.Max.Y)}
		rp = &rect[0]
	}
	pix := img.Pix[img.PixOffset(b.Min.X, b.Min.Y):]
	var timing [3]C.int64_t
	var tp *C.int64_t
	if canvasDebug >= 2 {
		tp = &timing[0]
	}
	ret := C.tg_cv_present(c.win, (*C.uint8_t)(unsafe.Pointer(&pix[0])), C.int(img.Stride), C.int(b.Dx()), C.int(b.Dy()), rp, tp)
	runtime.KeepAlive(img)
	c.stats.present += time.Since(t0)
	if tp != nil {
		area := b
		if dirty != nil {
			area = image.Rect(int(rect[0]), int(rect[1]), int(rect[2]), int(rect[3]))
		}
		logf(logDebug, "canvas %d: present %v: lock %.2f ms, copy %.2f ms, post %.2f ms", c.id, area,
			ms(time.Duration(timing[0])), ms(time.Duration(timing[1])), ms(time.Duration(timing[2])))
	}
	switch {
	case ret == 1 && !c.slowLogged:
		c.slowLogged = true
		logf(logWarn, "canvas %d: the Surface's buffers are RGB_565, converting every frame (slow)", c.id)
	case ret < 0:
		c.needFull = true
		return fmt.Errorf("tango/android: canvas %d: presenting into the Surface failed (%d)", c.id, int(ret))
	}
	c.needFull = false
	c.lastW, c.lastH = b.Dx(), b.Dy()
	return nil
}

// ---- platform.CanvasBackHandler ----

var _ platform.CanvasBackHandler = (*canvas)(nil)

// SetBackHandling implements platform.CanvasBackHandler: while on, the
// Java host takes Back (the button and the back gesture) from the page
// stack whenever this canvas is the top-most visible one asking for it, and
// delivers it as KEYCODE_BACK down and up through nativeKey, which reports
// it as KeyEscape with Code KeyBack (canvasKeys.event). Main thread.
func (c *canvas) SetBackHandling(on bool) {
	if c.closed || on == c.keys.back {
		return
	}
	c.keys.back = on
	if canvasDebug > 0 {
		logf(logInfo, "canvas %d: back handling %v", c.id, on)
	}
	withEnv(func(e env) { e.callVoid(c.page, mCvSetBack, jvBool(on)) })
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- platform.CanvasInsets ----

func (c *canvas) SafeAreaInsets() geom.Insets { return c.safe }
func (c *canvas) KeyboardInset() geom.Insets  { return c.kb }

// ---- frames ----

// frame runs one CanvasFrame. Main thread.
func (c *canvas) frame() {
	if c.closed {
		return
	}
	if c.inFrame {
		return // a nested request (an accessibility query during the frame)
	}
	t0 := time.Now()
	c.stats.present = 0
	c.inFrame = true
	c.sink.CanvasFrame()
	c.inFrame = false
	c.stats.add(c.id, c.wpx, c.hpx, time.Since(t0))
}

// frameStats logs frame times with canvasDebug: every 60 frames the mean
// and the slowest frame, and the time spent copying into the Surface.
type frameStats struct {
	present    time.Duration // copy time of the current frame
	n          int
	sum, worst time.Duration
	copySum    time.Duration
}

func (s *frameStats) add(id platform.SurfaceID, w, h int, d time.Duration) {
	if canvasDebug == 0 {
		return
	}
	s.n++
	s.sum += d
	s.copySum += s.present
	s.worst = max(s.worst, d)
	if canvasDebug >= 2 || s.n == 1 || s.n%60 == 0 {
		logf(logInfo, "canvas %d: %dx%d px, frame %.1f ms (mean of %d %.1f ms, worst %.1f ms, copy mean %.1f ms)",
			id, w, h, ms(d), s.n, ms(s.sum)/float64(s.n), ms(s.worst), ms(s.copySum)/float64(s.n))
		if s.n%60 == 0 {
			s.n, s.sum, s.worst, s.copySum = 0, 0, 0, 0
		}
	}
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func encodeAction(a mobilecanvas.Action, uptimeMs int64, now time.Time) C.jlong {
	return C.jlong(schedAction(a, uptimeMs, now))
}

func canvasFor(id C.jlong) *canvas {
	c := canvases[int64(id)]
	if c == nil || c.closed {
		return nil
	}
	return c
}

//export Java_org_tango_host_TangoCanvas_nativeWake
func Java_org_tango_host_TangoCanvas_nativeWake(penv *C.JNIEnv, cls C.jclass, id, uptime C.jlong) C.jlong {
	c := canvasFor(id)
	if c == nil {
		return 0
	}
	now := time.Now()
	a := c.sched.Wake(now)
	if canvasDebug >= 2 {
		logf(logDebug, "canvas %d: wake: %+v", c.id, a)
	}
	return encodeAction(a, int64(uptime), now)
}

//export Java_org_tango_host_TangoCanvas_nativeFrame
func Java_org_tango_host_TangoCanvas_nativeFrame(penv *C.JNIEnv, cls C.jclass, id, uptime C.jlong) C.jlong {
	c := canvasFor(id)
	if c == nil {
		return 0
	}
	a := c.sched.Tick(time.Now())
	if a.Frame {
		c.frame()
	}
	now := time.Now()
	return encodeAction(a, int64(uptime)+time.Since(now).Milliseconds(), now)
}

//export Java_org_tango_host_TangoCanvas_nativeTimer
func Java_org_tango_host_TangoCanvas_nativeTimer(penv *C.JNIEnv, cls C.jclass, id, uptime C.jlong) C.jlong {
	c := canvasFor(id)
	if c == nil {
		return 0
	}
	now := time.Now()
	a := c.sched.TimerFired(now)
	if canvasDebug >= 2 {
		logf(logDebug, "canvas %d: timer: %+v", c.id, a)
	}
	return encodeAction(a, int64(uptime), now)
}

// nativeSurfaceChanged: the SurfaceView's Surface was created or resized
// (surfaceChanged). The canvas takes its native window, reports the new
// size and draws a frame synchronously, so no stretched old frame shows.
//
//export Java_org_tango_host_TangoCanvas_nativeSurfaceChanged
func Java_org_tango_host_TangoCanvas_nativeSurfaceChanged(penv *C.JNIEnv, cls C.jclass, id C.jlong, surface C.jobject, w, h C.jint, density C.jfloat, uptime C.jlong) C.jlong {
	c := canvasFor(id)
	if c == nil {
		return 0
	}
	if canvasDebug > 0 {
		logf(logInfo, "canvas %d: Surface %dx%d px at density %.3g", c.id, int(w), int(h), float32(density))
	}
	oldW, oldH := c.Size()
	oldD := c.density
	fresh := false
	if c.win == nil {
		c.win = C.tg_cv_window(penv, surface)
		if c.win == nil {
			logf(logWarn, "canvas %d: no native window for the Surface", c.id)
			return 0
		}
		fresh = true
	}
	if fresh || int(w) != c.wpx || int(h) != c.hpx {
		if c.ov != nil {
			c.ov.surfaceGone() // the old frame's alpha no longer matches
		}
		if C.tg_cv_geometry(c.win, w, h, C.int(btoi(c.ov != nil))) != 0 {
			logf(logWarn, "canvas %d: setBuffersGeometry(%d, %d) failed", c.id, int(w), int(h))
		}
		c.needFull = true
	}
	c.wpx, c.hpx = int(w), int(h)
	c.gpuSetWindow() // the GPU presenter draws into the new window
	if density > 0 {
		c.density = float32(density)
	}
	now := time.Now()
	a := c.sched.Foreground(now)
	lw, lh := c.Size()
	if lw != oldW || lh != oldH || c.density != oldD {
		c.sink.CanvasResized(lw, lh, c.density)
		if hst := c.b.host(); hst != nil && !c.closed && c.ov == nil {
			hst.OnSurfaceEvent(c.id, platform.SurfaceEvent{Kind: platform.SurfaceResized, Width: int(lw + 0.5), Height: int(lh + 0.5)})
		}
	}
	if c.closed {
		return 0
	}
	s := c.sched.SyncFrame()
	if s.Frame {
		c.frame()
	}
	return encodeAction(mergeActions(a, s), int64(uptime)+time.Since(now).Milliseconds(), time.Now())
}

// nativeSurfaceDestroyed: the Surface goes away (the page was covered,
// the app went to the background, the page closed). The native window is
// released before returning, as Android requires; no frame is drawn until
// a new Surface arrives.
//
//export Java_org_tango_host_TangoCanvas_nativeSurfaceDestroyed
func Java_org_tango_host_TangoCanvas_nativeSurfaceDestroyed(penv *C.JNIEnv, cls C.jclass, id, uptime C.jlong) C.jlong {
	c := canvases[int64(id)]
	if c == nil {
		return 0
	}
	if canvasDebug > 0 {
		logf(logInfo, "canvas %d: Surface destroyed", c.id)
	}
	c.releaseWindow()
	if c.ov != nil {
		c.ov.surfaceGone()
	}
	if c.closed {
		return 0
	}
	now := time.Now()
	return encodeAction(c.sched.Background(), int64(uptime), now)
}

func (c *canvas) releaseWindow() {
	c.gpuClearWindow() // the EGL surface goes before the window
	if c.win != nil {
		C.tg_cv_release(c.win)
		c.win = nil
	}
	c.needFull = true
}

// nativeInsets reports the safe area (system bars and display cut-out)
// and the soft keyboard, device pixels.
//
//export Java_org_tango_host_TangoCanvas_nativeInsets
func Java_org_tango_host_TangoCanvas_nativeInsets(penv *C.JNIEnv, cls C.jclass, id C.jlong, top, right, bottom, left, ime C.jint, density C.jfloat) {
	c := canvasFor(id)
	if c == nil {
		return
	}
	d := float32(density)
	if !(d > 0) {
		d = c.density
	}
	safe := mobilecanvas.InsetsPx(int(top), int(right), int(bottom), int(left), d)
	kb := mobilecanvas.InsetsPx(0, 0, int(ime), 0, d)
	if safe == c.safe && kb == c.kb {
		return
	}
	c.safe, c.kb = safe, kb
	if canvasDebug > 0 {
		logf(logInfo, "canvas %d: safe area %+v, keyboard %+v", c.id, safe, kb)
	}
	c.Invalidate()
}

// nativeTouch delivers a touch-screen MotionEvent packed as canvas_input.go
// describes.
//
//export Java_org_tango_host_TangoCanvas_nativeTouch
func Java_org_tango_host_TangoCanvas_nativeTouch(penv *C.JNIEnv, cls C.jclass, id, eventMs C.jlong, data C.jfloatArray) {
	c := canvasFor(id)
	if c == nil {
		return
	}
	n := int(C.tg_cv_floats(penv, data, nil, 0))
	if n <= 0 {
		return
	}
	if cap(c.floats) < n {
		c.floats = make([]float32, n)
	}
	f := c.floats[:n]
	C.tg_cv_floats(penv, data, (*C.float)(unsafe.Pointer(&f[0])), C.int(n))
	c.evs = c.motion.decode(c.evs[:0], int64(eventMs), f, c.density)
	c.deliver(c.evs)
}

// nativePointer delivers a mouse, hover or wheel event.
//
//export Java_org_tango_host_TangoCanvas_nativePointer
func Java_org_tango_host_TangoCanvas_nativePointer(penv *C.JNIEnv, cls C.jclass, id C.jlong, kind C.jint, x, y C.jfloat, button, meta C.jint, dx, dy C.jfloat) {
	c := canvasFor(id)
	if c == nil {
		return
	}
	if ev, ok := pointerEvent(int(kind), float32(x), float32(y), int(button), int(meta), float32(dx), float32(dy), c.density); ok {
		c.evs = append(c.evs[:0], ev)
		c.deliver(c.evs)
	}
}

// nativeKey delivers a hardware key. It returns false for keys the canvas
// leaves to the system (Back, volume, ...).
//
//export Java_org_tango_host_TangoCanvas_nativeKey
func Java_org_tango_host_TangoCanvas_nativeKey(penv *C.JNIEnv, cls C.jclass, id C.jlong, down C.jboolean, code, base, unicode, meta, repeat C.jint) C.jboolean {
	c := canvasFor(id)
	if c == nil {
		return C.JNI_FALSE
	}
	ev, ok := c.keys.event(down != 0, int(code), int(base), int(meta), int(repeat))
	if !ok {
		return C.JNI_FALSE
	}
	c.key(ev, int(code), int(unicode), int(meta))
	return C.JNI_TRUE
}

// nativeFocus reports the canvas view gaining or losing keyboard focus,
// or its window losing focus (fingers still down are cancelled).
//
//export Java_org_tango_host_TangoCanvas_nativeFocus
func Java_org_tango_host_TangoCanvas_nativeFocus(penv *C.JNIEnv, cls C.jclass, id C.jlong, focused C.jboolean) {
	c := canvasFor(id)
	if c == nil {
		return
	}
	if focused == 0 {
		c.evs = c.motion.cancelAll(c.evs[:0])
		c.deliver(c.evs)
		c.keys.reset()
		clear(c.imeKeys)
	}
	if !c.closed {
		c.sink.CanvasFocus(focused != 0)
	}
}

func (c *canvas) deliver(evs []input.Event) {
	for _, ev := range evs {
		if c.closed {
			return
		}
		if canvasDebug > 0 {
			logf(logDebug, "canvas %d: input %T %+v", c.id, ev, ev)
		}
		c.sink.CanvasInput(ev)
	}
}

// nativeSurfaceEvent reports page events (platform.SurfaceEventKind):
// focus changes of the page stack and the page's closing.
//
//export Java_org_tango_host_TangoCanvas_nativeSurfaceEvent
func Java_org_tango_host_TangoCanvas_nativeSurfaceEvent(penv *C.JNIEnv, cls C.jclass, id C.jlong, kind C.jint) {
	c := canvases[int64(id)]
	if c == nil {
		return
	}
	k := platform.SurfaceEventKind(kind)
	if k == platform.SurfaceClosed {
		c.teardown(env{penv})
	} else if c.closed {
		return
	}
	if h := c.b.host(); h != nil {
		h.OnSurfaceEvent(c.id, platform.SurfaceEvent{Kind: k})
	}
}

// teardown forgets the canvas once its page is gone. Main thread.
func (c *canvas) teardown(e env) {
	if c.closed {
		return
	}
	c.closed = true
	c.closedInFrame = c.inFrame
	c.sched.Close()
	c.releaseWindow()
	c.gpuClose()
	delete(canvases, int64(c.id))
	page, view := c.page, c.view
	c.page, c.view = 0, 0
	e.deleteGlobal(view)
	e.deleteGlobal(page)
}

// ---- waking the main thread from other goroutines ----

// wakeQ feeds the waker: one goroutine on a thread attached to the VM once,
// which calls TangoCanvas.requestFrame. Invalidate only gets here when a
// canvas goes from idle to pending, so the queue stays short.
var (
	wakeOnce sync.Once
	wakeQ    = make(chan int64, 64)
)

func wakeCanvas(id int64) {
	wakeOnce.Do(func() { go waker() })
	select {
	case wakeQ <- id:
	default:
		// Full: never expected; fall back to a temporary attach.
		withEnv(func(e env) { e.callStaticVoid(mCanvasRequestFrame, jvLong(id)) })
	}
}

func waker() {
	runtime.LockOSThread() // this thread stays attached to the VM for good
	var attached C.int
	p := C.tg_env(&attached)
	if p == nil {
		logf(logError, "canvas waker: cannot attach to the VM")
		for id := range wakeQ {
			withEnv(func(e env) { e.callStaticVoid(mCanvasRequestFrame, jvLong(id)) })
		}
		return
	}
	e := env{p}
	for id := range wakeQ {
		e.callStaticVoid(mCanvasRequestFrame, jvLong(id))
	}
}

// ---- platform.Surface ----

// canvasSurface is the platform.Surface of a canvas page.
type canvasSurface struct{ c *canvas }

func (s canvasSurface) call(m *jmethod, args ...C.jvalue) {
	if s.c.closed {
		return
	}
	if !onMainThread() {
		logf(logWarn, "Surface.%s called off the main thread", m.name)
	}
	withEnv(func(e env) { e.callVoid(s.c.page, m, args...) })
}

func (s canvasSurface) Show()  { s.call(mCvShow) }
func (s canvasSurface) Hide()  { s.call(mCvHide) }
func (s canvasSurface) Close() { s.call(mCvClose) }
func (s canvasSurface) Focus() { s.call(mCvFocus) }

func (s canvasSurface) SetTitle(title string) {
	if s.c.closed {
		return
	}
	withEnv(func(e env) { e.callVoid(s.c.page, mCvSetTitle, jvObj(e.str(title))) })
}

// SetSize does nothing: pages fill the Activity.
func (s canvasSurface) SetSize(width, height int) {}

// Size is the page's size in dp (0, 0 before its Surface exists).
func (s canvasSurface) Size() (int, int) {
	w, h := s.c.Size()
	return int(w + 0.5), int(h + 0.5)
}

func (canvasSurface) Navigate(string) {}
func (canvasSurface) Eval(string)     {}
func (canvasSurface) OpenDevTools()   {}

// Native returns a JNI global reference to the page's view (an
// android.view.View: the CanvasView), valid until the surface closes.
func (s canvasSurface) Native() uintptr {
	c := s.c
	if c.closed {
		return 0
	}
	if c.view == 0 {
		withEnv(func(e env) {
			if v, err := e.callObject(c.page, mCvView); err == nil && v != 0 {
				c.view = e.global(v)
			}
		})
	}
	return uintptr(c.view)
}
