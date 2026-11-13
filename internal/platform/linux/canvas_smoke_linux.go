//go:build linux && !android

package linux

// Test hooks for internal/platform/linux/internal/smokecanvas: synthetic
// GdkEvents sent straight to a canvas widget with gtk_widget_event, and an
// offscreen render of the widget with gtk_widget_draw. They exercise the
// event translation, the IM routing and the pixel path on a real GTK
// without a real mouse, keyboard or screenshot tool.
//
// Every synthetic event is filled through the offsets in canvas_keys.go
// and read back with GDK's own accessors (gdk_event_get_coords, ...), so
// the smoke test also checks those offsets against the installed GTK.

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
)

var canvasSmoke struct {
	once sync.Once
	err  error

	eventNew       func(typ int32) ptr
	eventFree      func(ev ptr)
	widgetEvent    func(w, ev ptr) bool
	objectRef      func(o ptr) ptr
	getCoords      func(ev ptr, x, y *float64) bool
	getState       func(ev ptr, state *uint32) bool
	getButton      func(ev ptr, button *uint32) bool
	getKeyval      func(ev ptr, keyval *uint32) bool
	getKeycode     func(ev ptr, code *uint16) bool
	getScrollDelta func(ev ptr, dx, dy *float64) bool
	getScrollDir   func(ev ptr, dir *uint32) bool
	widgetDraw     func(w, cr ptr)
	cairoCreate    func(s ptr) ptr
	cairoDestroy   func(cr ptr)
}

func loadCanvasSmoke() error {
	canvasSmoke.once.Do(func() {
		if err := loadCanvasFFI(); err != nil {
			canvasSmoke.err = err
			return
		}
		var bd binder
		t, d, o, c := libGTK, canvasFFI.gdk, libGObject, canvasFFI.cairo
		k := &canvasSmoke
		bd.req(&k.eventNew, "gdk_event_new", d, t)
		bd.req(&k.eventFree, "gdk_event_free", d, t)
		bd.req(&k.widgetEvent, "gtk_widget_event", t)
		bd.req(&k.objectRef, "g_object_ref", o)
		bd.req(&k.getCoords, "gdk_event_get_coords", d, t)
		bd.req(&k.getState, "gdk_event_get_state", d, t)
		bd.req(&k.getButton, "gdk_event_get_button", d, t)
		bd.req(&k.getKeyval, "gdk_event_get_keyval", d, t)
		bd.req(&k.getKeycode, "gdk_event_get_keycode", d, t)
		bd.req(&k.getScrollDelta, "gdk_event_get_scroll_deltas", d, t)
		bd.req(&k.getScrollDir, "gdk_event_get_scroll_direction", d, t)
		bd.req(&k.widgetDraw, "gtk_widget_draw", t)
		bd.req(&k.cairoCreate, "cairo_create", c)
		bd.req(&k.cairoDestroy, "cairo_destroy", c)
		if len(bd.missing) > 0 {
			canvasSmoke.err = errors.New("smoke: missing symbols: " + strings.Join(bd.missing, ", "))
		}
	})
	return canvasSmoke.err
}

func canvasSmokeOf(cv platform.Canvas) (*canvas, error) {
	c, ok := cv.(*canvas)
	if !ok || c.closed {
		return nil, errors.New("smoke: not an open linux canvas")
	}
	return c, nil
}

// canvasSmokeEvents builds, checks and sends synthetic events to one canvas.
type canvasSmokeEvents struct {
	c    *canvas
	win  ptr
	errs []string
	time uint32
}

func (e *canvasSmokeEvents) errorf(format string, args ...any) {
	e.errs = append(e.errs, fmt.Sprintf(format, args...))
}

func canvasSetU32(ev ptr, off uintptr, v uint32) { *(*uint32)(unsafe.Add(toPtr(ev), off)) = v }
func canvasSetU16(ev ptr, off uintptr, v uint16) { *(*uint16)(unsafe.Add(toPtr(ev), off)) = v }
func canvasSetF64(ev ptr, off uintptr, v float64) {
	*(*float64)(unsafe.Add(toPtr(ev), off)) = v
}

// send creates an event of typ, lets fill set its fields, verifies them
// with verify, delivers it to the widget and frees it.
func (e *canvasSmokeEvents) send(typ int32, fill func(ev ptr), verify func(ev ptr)) {
	k := &canvasSmoke
	ev := k.eventNew(typ)
	if ev == 0 {
		e.errorf("gdk_event_new(%d) failed", typ)
		return
	}
	// GdkEventAny: window at 8 (gdk_event_free unrefs it), send_event at 16.
	*(*ptr)(unsafe.Add(toPtr(ev), 8)) = k.objectRef(e.win)
	*(*int8)(unsafe.Add(toPtr(ev), 16)) = 1
	fill(ev)
	if verify != nil {
		verify(ev)
	}
	k.widgetEvent(e.c.widget, ev)
	k.eventFree(ev)
}

func (e *canvasSmokeEvents) checkCoords(name string, ev ptr, x, y float64) {
	var gx, gy float64
	if !canvasSmoke.getCoords(ev, &gx, &gy) || gx != x || gy != y {
		e.errorf("%s: gdk_event_get_coords = %v,%v, wrote %v,%v", name, gx, gy, x, y)
	}
}

func (e *canvasSmokeEvents) checkState(name string, ev ptr, state uint32) {
	var g uint32
	if !canvasSmoke.getState(ev, &g) || g != state {
		e.errorf("%s: gdk_event_get_state = %#x, wrote %#x", name, g, state)
	}
}

func (e *canvasSmokeEvents) motion(x, y float64, state uint32) {
	e.send(canvasGdkMotionNotify, func(ev ptr) {
		canvasSetU32(ev, 20, e.tick())
		canvasSetF64(ev, canvasEvMotionX, x)
		canvasSetF64(ev, canvasEvMotionY, y)
		canvasSetU32(ev, canvasEvMotionState, state)
	}, func(ev ptr) {
		e.checkCoords("motion", ev, x, y)
		e.checkState("motion", ev, state)
	})
}

func (e *canvasSmokeEvents) button(typ int32, n uint32, x, y float64, state uint32) {
	e.send(typ, func(ev ptr) {
		canvasSetU32(ev, canvasEvButtonTime, e.tick())
		canvasSetF64(ev, canvasEvButtonX, x)
		canvasSetF64(ev, canvasEvButtonY, y)
		canvasSetU32(ev, canvasEvButtonState, state)
		canvasSetU32(ev, canvasEvButtonButton, n)
	}, func(ev ptr) {
		e.checkCoords("button", ev, x, y)
		e.checkState("button", ev, state)
		var b uint32
		if !canvasSmoke.getButton(ev, &b) || b != n {
			e.errorf("button: gdk_event_get_button = %d, wrote %d", b, n)
		}
	})
}

func (e *canvasSmokeEvents) crossing(typ int32, x, y float64) {
	e.send(typ, func(ev ptr) {
		canvasSetU32(ev, 32, e.tick()) // time
		canvasSetF64(ev, canvasEvCrossingX, x)
		canvasSetF64(ev, canvasEvCrossingY, y)
		canvasSetU32(ev, canvasEvCrossingState, 0)
	}, func(ev ptr) {
		e.checkCoords("crossing", ev, x, y)
		e.checkState("crossing", ev, 0)
	})
}

func (e *canvasSmokeEvents) scroll(dir uint32, x, y, dx, dy float64) {
	e.send(canvasGdkScroll, func(ev ptr) {
		canvasSetU32(ev, 20, e.tick())
		canvasSetF64(ev, canvasEvScrollX, x)
		canvasSetF64(ev, canvasEvScrollY, y)
		canvasSetU32(ev, canvasEvScrollState, 0)
		canvasSetU32(ev, canvasEvScrollDirection, dir)
		canvasSetF64(ev, canvasEvScrollDeltaX, dx)
		canvasSetF64(ev, canvasEvScrollDeltaY, dy)
	}, func(ev ptr) {
		e.checkCoords("scroll", ev, x, y)
		var d uint32
		if dir == canvasGdkScrollSmooth {
			var gx, gy float64
			if !canvasSmoke.getScrollDelta(ev, &gx, &gy) || gx != dx || gy != dy {
				e.errorf("scroll: gdk_event_get_scroll_deltas = %v,%v, wrote %v,%v", gx, gy, dx, dy)
			}
		} else if !canvasSmoke.getScrollDir(ev, &d) || d != dir {
			e.errorf("scroll: gdk_event_get_scroll_direction = %d, wrote %d", d, dir)
		}
	})
}

func (e *canvasSmokeEvents) key(press bool, keyval, state uint32) {
	e.keyHW(press, keyval, state, 38) // any plausible X keycode
}

func (e *canvasSmokeEvents) keyHW(press bool, keyval, state uint32, hw uint16) {
	typ := int32(canvasGdkKeyPress)
	if !press {
		typ = canvasGdkKeyRelease
	}
	e.send(typ, func(ev ptr) {
		canvasSetU32(ev, 20, e.tick())
		canvasSetU32(ev, canvasEvKeyState, state)
		canvasSetU32(ev, canvasEvKeyKeyval, keyval)
		canvasSetU16(ev, canvasEvKeyHWCode, hw)
	}, func(ev ptr) {
		e.checkState("key", ev, state)
		var kv uint32
		if !canvasSmoke.getKeyval(ev, &kv) || kv != keyval {
			e.errorf("key: gdk_event_get_keyval = %#x, wrote %#x", kv, keyval)
		}
		var code uint16
		if !canvasSmoke.getKeycode(ev, &code) || code != hw {
			e.errorf("key: gdk_event_get_keycode = %d, wrote %d", code, hw)
		}
	})
}

func (e *canvasSmokeEvents) tap(keyval, state uint32) {
	e.key(true, keyval, state)
	e.key(false, keyval, state)
}

func (e *canvasSmokeEvents) tick() uint32 { e.time += 50; return e.time }

// SmokeCanvasInput sends a fixed script of synthetic events to cv (see
// internal/smokecanvas for the expected input events). It returns an error
// when GDK's accessors disagree with the offsets the canvas reads.
func (b *backend) SmokeCanvasInput(cv platform.Canvas) error {
	c, err := canvasSmokeOf(cv)
	if err != nil {
		return err
	}
	if err := loadCanvasSmoke(); err != nil {
		return err
	}
	win := canvasFFI.widgetGetWindow(c.widget)
	if win == 0 {
		return errors.New("smoke: canvas widget not realized")
	}
	e := &canvasSmokeEvents{c: c, win: win, time: 100000}

	// Pointer.
	e.crossing(canvasGdkEnterNotify, 5, 5)
	e.motion(30, 20, 0)
	e.button(canvasGdkButtonPress, 1, 100, 50, canvasGdkShiftMask)
	e.button(canvasGdkButtonRelease, 1, 100, 50, canvasGdkShiftMask|canvasGdkButton1Mask)
	e.button(canvasGdkButtonPress, 1, 100, 50, canvasGdkShiftMask) // second click
	e.button(canvasGdk2ButtonPress, 1, 100, 50, canvasGdkShiftMask)
	e.button(canvasGdkButtonRelease, 1, 100, 50, canvasGdkShiftMask|canvasGdkButton1Mask)
	e.time += 5000 // not a third click
	e.button(canvasGdkButtonPress, 3, 10, 10, 0)
	e.button(canvasGdkButtonRelease, 3, 10, 10, canvasGdkButton3Mask)
	e.scroll(canvasGdkScrollSmooth, 40, 40, 0, 0.75)
	e.scroll(canvasGdkScrollDown, 40, 40, 0, 0)
	e.crossing(canvasGdkLeaveNotify, -1, 5)

	// Keys with text input off.
	e.tap(0xff51, 0)                 // Left
	e.tap('c', canvasGdkControlMask) // Ctrl+C
	e.tap(0xffe1, 0)                 // Shift_L: ShiftLeft down (Mods Shift) and up (no Shift)
	e.tap('a', 0)                    // a: KeyA (no TextInput while off)
	// New keys: F5, keypad 1 with NumLock on and off, '/' by keycode.
	e.keyHW(true, 0xffc2, 0, 71) // F5
	e.keyHW(false, 0xffc2, 0, 71)
	e.keyHW(true, 0xffb1, 1<<4, 87) // KP_1 (NumLock on: GDK_MOD2_MASK)
	e.keyHW(false, 0xffb1, 1<<4, 87)
	e.keyHW(true, 0xff9c, 0, 87) // KP_End (NumLock off)
	e.keyHW(false, 0xff9c, 0, 87)
	e.keyHW(true, 0xff57, 0, 115) // the navigation block End
	e.keyHW(false, 0xff57, 0, 115)
	// Keypad 8 pressed with NumLock off, released after NumLock went on:
	// the release still reports Up with Code Numpad8.
	e.keyHW(true, 0xff97, 0, 80)     // KP_Up
	e.keyHW(false, 0xffb8, 1<<4, 80) // KP_8
	e.keyHW(true, '/', 0, 61)        // slash
	e.keyHW(false, '/', 0, 61)

	// Keys with text input on: printable keys go through the IM context.
	c.SetTextInput(true, geom.R(30, 20, 2, 18))
	e.tap('a', 0)    // the simple context commits "a"
	e.tap(0xff0d, 0) // Return: not used by the IM, reported as a key
	// Ctrl+Shift+U 4 f 6 0 space: GtkIMContextSimple's hex entry shows a
	// pre-edit and commits U+4F60 (optional check, depends on the module).
	e.tap('U', canvasGdkControlMask|canvasGdkShiftMask)
	for _, k := range []uint32{'4', 'f', '6', '0', ' '} {
		e.tap(k, 0)
	}
	c.SetTextInput(false, geom.Rect{})

	if len(e.errs) > 0 {
		return errors.New(strings.Join(e.errs, "; "))
	}
	return nil
}

// SmokeCanvasRender draws the canvas widget into an offscreen cairo image
// surface at its device scale (gtk_widget_draw runs the "draw" handler) and
// returns the pixels as RGBA, alpha unpremultiplied only for opaque pixels.
func (b *backend) SmokeCanvasRender(cv platform.Canvas) (*image.RGBA, error) {
	c, err := canvasSmokeOf(cv)
	if err != nil {
		return nil, err
	}
	if err := loadCanvasSmoke(); err != nil {
		return nil, err
	}
	f := &canvasFFI
	pw, ph := int(c.w*c.scale), int(c.h*c.scale)
	if pw <= 0 || ph <= 0 {
		return nil, errors.New("smoke: empty canvas")
	}
	surf := f.imageSurfaceCreate(canvasCairoFormatARGB32, int32(pw), int32(ph))
	if surf == 0 || f.surfaceStatus(surf) != 0 {
		return nil, errors.New("smoke: cairo_image_surface_create failed")
	}
	defer f.surfaceDestroy(surf)
	f.surfaceSetDeviceScale(surf, c.scale, c.scale)
	cr := canvasSmoke.cairoCreate(surf)
	canvasSmoke.widgetDraw(c.widget, cr)
	canvasSmoke.cairoDestroy(cr)
	f.surfaceFlush(surf)
	stride := int(f.imageSurfaceGetStride(surf))
	data := unsafe.Slice((*byte)(toPtr(f.imageSurfaceGetData(surf))), stride*ph)
	img := image.NewRGBA(image.Rect(0, 0, pw, ph))
	for y := 0; y < ph; y++ {
		for x := 0; x < pw; x++ {
			v := *(*uint32)(unsafe.Pointer(&data[y*stride+x*4])) // native-endian 0xAARRGGBB
			o := img.PixOffset(x, y)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = uint8(v>>16), uint8(v>>8), uint8(v), uint8(v>>24)
		}
	}
	return img, nil
}
