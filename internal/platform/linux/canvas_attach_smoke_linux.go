//go:build linux && !android

package linux

// Test hooks for internal/platform/linux/internal/smokeattach: inspect a
// canvas attached to a WebView surface (canvas_attach_linux.go) and drive
// it with input that GDK routes like real input. For the internal smoke
// test only.
//
// Clicks use gdk_test_simulate_button, moves gdk_device_warp and screen
// reads gdk_pixbuf_get_from_window. On X11 (Xvfb included) all three go
// through the X server, so GDK's own window picking (stacking, input
// shapes, pass-through) decides who gets the event. The Wayland backend
// implements none of them: the hooks then report ErrSmokeUnsupported and
// the smoke test skips those checks.

import (
	"errors"
	"image/color"
	"sync"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// ErrSmokeUnsupported is returned where the GDK backend cannot synthesize
// input or read the screen (Wayland).
var ErrSmokeUnsupported = errors.New("not supported by this GDK backend")

var attachSmoke struct {
	once sync.Once
	err  error

	simulateButton    func(win ptr, x, y int32, button uint32, mods uint32, typ int32) bool
	translate         func(src, dst ptr, x, y int32, dx, dy *int32) bool
	getAllocation     func(w ptr, a *gdkRect)
	getParent         func(w ptr) ptr
	childGetProperty  func(container, child ptr, name string, v unsafe.Pointer)
	valueInit         func(v unsafe.Pointer, typ uintptr) unsafe.Pointer
	valueGetInt       func(v unsafe.Pointer) int32
	valueUnset        func(v unsafe.Pointer)
	getPassThrough    func(o, w ptr) bool
	windowGetOrigin   func(win ptr, x, y *int32) int32
	windowGetScreen   func(win ptr) ptr
	displayDefault    func() ptr
	displaySeat       func(d ptr) ptr
	seatPointer       func(seat ptr) ptr
	deviceWarp        func(dev, screen ptr, x, y int32)
	pixbufFromWindow  func(win ptr, x, y, w, h int32) ptr
	pixbufPixels      func(pb ptr) ptr
	pixbufNChannels   func(pb ptr) int32
	x11DisplayType    func() uintptr
	typeCheckInstance func(inst ptr, typ uintptr) bool
}

func loadAttachSmoke() error {
	s := &attachSmoke
	s.once.Do(func() {
		if err := loadAttachFFI(); err != nil {
			s.err = err
			return
		}
		gdk, _, _ := open("libgdk-3.so.0")
		pixbuf, _, _ := open("libgdk_pixbuf-2.0.so.0")
		var bd binder
		t, o := libGTK, libGObject
		bd.req(&s.simulateButton, "gdk_test_simulate_button", gdk, t)
		bd.req(&s.translate, "gtk_widget_translate_coordinates", t)
		bd.req(&s.getAllocation, "gtk_widget_get_allocation", t)
		bd.req(&s.getParent, "gtk_widget_get_parent", t)
		bd.req(&s.childGetProperty, "gtk_container_child_get_property", t)
		bd.req(&s.valueInit, "g_value_init", o)
		bd.req(&s.valueGetInt, "g_value_get_int", o)
		bd.req(&s.valueUnset, "g_value_unset", o)
		bd.req(&s.getPassThrough, "gtk_overlay_get_overlay_pass_through", t)
		bd.req(&s.windowGetOrigin, "gdk_window_get_origin", gdk, t)
		bd.req(&s.windowGetScreen, "gdk_window_get_screen", gdk, t)
		bd.req(&s.displayDefault, "gdk_display_get_default", gdk, t)
		bd.req(&s.displaySeat, "gdk_display_get_default_seat", gdk, t)
		bd.req(&s.seatPointer, "gdk_seat_get_pointer", gdk, t)
		bd.req(&s.deviceWarp, "gdk_device_warp", gdk, t)
		bd.req(&s.pixbufFromWindow, "gdk_pixbuf_get_from_window", gdk, t)
		bd.req(&s.pixbufPixels, "gdk_pixbuf_get_pixels", pixbuf, gdk, t)
		bd.req(&s.pixbufNChannels, "gdk_pixbuf_get_n_channels", pixbuf, gdk, t)
		bd.opt(&s.x11DisplayType, "gdk_x11_display_get_type", gdk, t)
		bd.req(&s.typeCheckInstance, "g_type_check_instance_is_a", o)
		if len(bd.missing) > 0 {
			s.err = errors.New("smoke hooks: missing symbols")
		}
	})
	return s.err
}

// x11 reports whether GDK runs on X11.
func attachSmokeX11() bool {
	s := &attachSmoke
	if s.x11DisplayType == nil {
		return false
	}
	d := s.displayDefault()
	return d != 0 && s.typeCheckInstance(d, s.x11DisplayType())
}

// SmokeAttachState describes an attached canvas.
type SmokeAttachState struct {
	Frame          [4]float64 // x, y, w, h relative to the overlay (content area)
	InOverlay      bool       // the widget's parent is the surface's overlay
	AboveWeb       bool       // after the web view in the overlay's stacking order
	PassThrough    bool       // the overlay child is pass-through
	Shape          bool       // an input shape is set
	Focused        bool       // the widget has keyboard focus
	WebTransparent bool       // the web view's background was made transparent
	X11            bool       // clicks, moves and screen reads are available
}

func smokeAttached(cv platform.Canvas) (*attached, error) {
	a, ok := cv.(*attached)
	if !ok {
		return nil, errors.New("not a linux attached canvas")
	}
	if err := loadAttachSmoke(); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *surface) overlayIndex(w ptr) int {
	h := attachHosts[s]
	if h == nil {
		return -1
	}
	var v canvasGValue
	sm := &attachSmoke
	sm.valueInit(unsafe.Pointer(&v), canvasGTypeInt)
	sm.childGetProperty(h.overlay, w, "index", unsafe.Pointer(&v))
	n := sm.valueGetInt(unsafe.Pointer(&v))
	sm.valueUnset(unsafe.Pointer(&v))
	return int(n)
}

// SmokeAttachState reports where cv sits. Main thread.
func (b *backend) SmokeAttachState(cv platform.Canvas) (st SmokeAttachState, err error) {
	a, err := smokeAttached(cv)
	if err != nil {
		return st, err
	}
	st.WebTransparent = a.h.bgSet
	st.X11 = attachSmokeX11()
	if a.closed || a.s.closed {
		return st, nil
	}
	sm, f := &attachSmoke, &attachFFI
	st.InOverlay = sm.getParent(a.widget) == a.h.overlay
	if !st.InOverlay {
		return st, nil
	}
	var x, y int32
	sm.translate(a.widget, a.h.overlay, 0, 0, &x, &y)
	st.Frame = [4]float64{float64(x), float64(y), float64(f.widgetGetAllocW(a.widget)), float64(f.widgetGetAllocH(a.widget))}
	st.AboveWeb = a.s.overlayIndex(a.widget) > a.s.overlayIndex(a.s.web)
	st.PassThrough = sm.getPassThrough(a.h.overlay, a.widget)
	st.Shape = a.hasShape
	st.Focused = f.widgetHasFocus(a.widget)
	return st, nil
}

// windowPoint converts content point (x, y) of s to coordinates in the
// toplevel's GdkWindow (the overlay has no window of its own, so its
// allocation is relative to that window).
func (s *surface) windowPoint(x, y float64) (ptr, int32, int32) {
	h := attachHosts[s]
	var al gdkRect
	attachSmoke.getAllocation(h.overlay, &al)
	return attachFFI.widgetGetWindow(h.overlay), al.X + int32(x), al.Y + int32(y)
}

// SmokeAttachClick sends a left click at content point (x, y) through the
// X server (gdk_test_simulate_button). Main thread.
func (b *backend) SmokeAttachClick(cv platform.Canvas, x, y float64) error {
	a, err := smokeAttached(cv)
	if err != nil {
		return err
	}
	if !attachSmokeX11() {
		return ErrSmokeUnsupported
	}
	win, wx, wy := a.s.windowPoint(x, y)
	if win == 0 {
		return errors.New("overlay not realized")
	}
	const press, release = 4, 7 // GDK_BUTTON_PRESS, GDK_BUTTON_RELEASE
	if !attachSmoke.simulateButton(win, wx, wy, 1, 0, press) || !attachSmoke.simulateButton(win, wx, wy, 1, 0, release) {
		return errors.New("gdk_test_simulate_button failed")
	}
	return nil
}

// SmokeAttachMove warps the pointer to content point (x, y). Main thread.
func (b *backend) SmokeAttachMove(cv platform.Canvas, x, y float64) error {
	a, err := smokeAttached(cv)
	if err != nil {
		return err
	}
	if !attachSmokeX11() {
		return ErrSmokeUnsupported
	}
	sm := &attachSmoke
	win, wx, wy := a.s.windowPoint(x, y)
	var ox, oy int32
	sm.windowGetOrigin(win, &ox, &oy)
	dev := sm.seatPointer(sm.displaySeat(sm.displayDefault()))
	if dev == 0 {
		return errors.New("no pointer device")
	}
	sm.deviceWarp(dev, sm.windowGetScreen(win), ox+wx, oy+wy)
	return nil
}

// SmokeAttachPixel reads the window's pixel at content point (x, y).
func (b *backend) SmokeAttachPixel(cv platform.Canvas, x, y float64) (color.RGBA, error) {
	a, err := smokeAttached(cv)
	if err != nil {
		return color.RGBA{}, err
	}
	if !attachSmokeX11() {
		return color.RGBA{}, ErrSmokeUnsupported
	}
	sm := &attachSmoke
	win, wx, wy := a.s.windowPoint(x, y)
	pb := sm.pixbufFromWindow(win, wx, wy, 1, 1)
	if pb == 0 {
		return color.RGBA{}, errors.New("gdk_pixbuf_get_from_window failed")
	}
	defer gObjectUnref(pb)
	p := unsafe.Slice((*byte)(toPtr(sm.pixbufPixels(pb))), max(sm.pixbufNChannels(pb), 3))
	return color.RGBA{p[0], p[1], p[2], 255}, nil
}
