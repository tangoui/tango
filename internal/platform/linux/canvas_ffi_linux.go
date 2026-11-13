//go:build linux && !android

package linux

// Native symbols of the canvas widget (canvas_linux.go,
// canvas_input_linux.go): GtkDrawingArea and widget plumbing, GdkFrameClock
// tick callbacks, GtkIMContext, GdkCursor, GdkKeymap and cairo image
// surfaces. They live in their own struct, bound on first use by
// loadCanvasFFI, so they never collide with the other binding files and a
// missing symbol only disables canvases.

import (
	"errors"
	"strings"
	"sync"
	"unsafe"
)

// canvasGValue mirrors GValue (GType g_type; union data[2]): 24 bytes on
// LP64, zero-initialised before g_value_init.
type canvasGValue struct {
	typ  uintptr
	data [2]uint64
}

const canvasGTypeInt = 6 << 2 // G_TYPE_INT

var canvasFFI struct {
	once sync.Once
	err  error

	gdk, cairo ptr

	// GTK widget
	drawingAreaNew           func() ptr
	widgetAddEvents          func(w ptr, mask int32)
	widgetSetCanFocus        func(w ptr, on bool)
	widgetGrabFocus          func(w ptr)
	widgetHasFocus           func(w ptr) bool
	widgetQueueDraw          func(w ptr)
	widgetQueueDrawArea      func(w ptr, x, y, width, height int32)
	widgetGetAllocatedWidth  func(w ptr) int32
	widgetGetAllocatedHeight func(w ptr) int32
	widgetGetScaleFactor     func(w ptr) int32
	widgetAddTickCallback    func(w, cb, data, notify ptr) uint32
	widgetRemoveTickCallback func(w ptr, id uint32)
	widgetGetWindow          func(w ptr) ptr
	widgetGetDisplay         func(w ptr) ptr
	widgetGetRealized        func(w ptr) bool
	widgetSetHexpand         func(w ptr, on bool)
	widgetSetVexpand         func(w ptr, on bool)

	// GObject / GLib
	objectRefSink func(o ptr) ptr
	sourceRemove  func(id uint32) bool

	// GtkIMContext
	imMulticontextNew   func() ptr
	imSetClientWindow   func(ctx, win ptr)
	imFocusIn           func(ctx ptr)
	imFocusOut          func(ctx ptr)
	imReset             func(ctx ptr)
	imSetCursorLocation func(ctx ptr, r *canvasRect)
	imFilterKeypress    func(ctx, ev ptr) bool
	imGetPreeditString  func(ctx ptr, str *ptr, attrs ptr, cursor *int32)

	// GDK
	cursorNewFromName func(display ptr, name string) ptr
	windowSetCursor   func(win, cursor ptr)

	// GDK, optional
	keymapForDisplay        func(display ptr) ptr
	keymapTranslate         func(km ptr, hw, state uint32, group int32, keyval *uint32, effGroup, level *int32, consumed *uint32) bool
	eventGetSourceDevice    func(ev ptr) ptr
	deviceGetSource         func(dev ptr) int32
	eventGetPointerEmulated func(ev ptr) bool // GTK 3.22+
	eventGetAxis            func(ev ptr, axis int32, v *float64) bool

	// cairo
	imageSurfaceCreate    func(format, w, h int32) ptr
	formatStrideForWidth  func(format, w int32) int32
	imageSurfaceGetData   func(s ptr) ptr
	imageSurfaceGetStride func(s ptr) int32
	surfaceFlush          func(s ptr)
	surfaceMarkDirty      func(s ptr)
	surfaceDestroy        func(s ptr)
	surfaceStatus         func(s ptr) int32
	surfaceSetDeviceScale func(s ptr, sx, sy float64)
	setSourceSurface      func(cr, s ptr, x, y float64)
	paint                 func(cr ptr)

	// Double-click rule from GtkSettings (defaults are GTK's).
	dblMS   uint32
	dblDist float64
}

// loadCanvasFFI binds the canvas symbols once. Main thread, after Init
// (load) has opened GLib / GObject / GTK.
func loadCanvasFFI() error {
	canvasFFI.once.Do(func() {
		if libGTK == 0 || libGObject == 0 || libGLib == 0 {
			canvasFFI.err = errors.New("tango/linux: canvas used before Init")
			return
		}
		canvasFFI.gdk, _, _ = open("libgdk-3.so.0")
		var err error
		if canvasFFI.cairo, _, err = open("libcairo.so.2"); err != nil {
			canvasFFI.err = err
			return
		}
		var bd binder
		t, d, o, g, c := libGTK, canvasFFI.gdk, libGObject, libGLib, canvasFFI.cairo

		bd.req(&canvasFFI.drawingAreaNew, "gtk_drawing_area_new", t)
		bd.req(&canvasFFI.widgetAddEvents, "gtk_widget_add_events", t)
		bd.req(&canvasFFI.widgetSetCanFocus, "gtk_widget_set_can_focus", t)
		bd.req(&canvasFFI.widgetGrabFocus, "gtk_widget_grab_focus", t)
		bd.req(&canvasFFI.widgetHasFocus, "gtk_widget_has_focus", t)
		bd.req(&canvasFFI.widgetQueueDraw, "gtk_widget_queue_draw", t)
		bd.req(&canvasFFI.widgetQueueDrawArea, "gtk_widget_queue_draw_area", t)
		bd.req(&canvasFFI.widgetGetAllocatedWidth, "gtk_widget_get_allocated_width", t)
		bd.req(&canvasFFI.widgetGetAllocatedHeight, "gtk_widget_get_allocated_height", t)
		bd.req(&canvasFFI.widgetGetScaleFactor, "gtk_widget_get_scale_factor", t)
		bd.req(&canvasFFI.widgetAddTickCallback, "gtk_widget_add_tick_callback", t)
		bd.req(&canvasFFI.widgetRemoveTickCallback, "gtk_widget_remove_tick_callback", t)
		bd.req(&canvasFFI.widgetGetWindow, "gtk_widget_get_window", t)
		bd.req(&canvasFFI.widgetGetDisplay, "gtk_widget_get_display", t)
		bd.req(&canvasFFI.widgetGetRealized, "gtk_widget_get_realized", t)
		bd.req(&canvasFFI.widgetSetHexpand, "gtk_widget_set_hexpand", t)
		bd.req(&canvasFFI.widgetSetVexpand, "gtk_widget_set_vexpand", t)

		bd.req(&canvasFFI.objectRefSink, "g_object_ref_sink", o)
		bd.req(&canvasFFI.sourceRemove, "g_source_remove", g)

		bd.req(&canvasFFI.imMulticontextNew, "gtk_im_multicontext_new", t)
		bd.req(&canvasFFI.imSetClientWindow, "gtk_im_context_set_client_window", t)
		bd.req(&canvasFFI.imFocusIn, "gtk_im_context_focus_in", t)
		bd.req(&canvasFFI.imFocusOut, "gtk_im_context_focus_out", t)
		bd.req(&canvasFFI.imReset, "gtk_im_context_reset", t)
		bd.req(&canvasFFI.imSetCursorLocation, "gtk_im_context_set_cursor_location", t)
		bd.req(&canvasFFI.imFilterKeypress, "gtk_im_context_filter_keypress", t)
		bd.req(&canvasFFI.imGetPreeditString, "gtk_im_context_get_preedit_string", t)

		bd.req(&canvasFFI.cursorNewFromName, "gdk_cursor_new_from_name", d, t)
		bd.req(&canvasFFI.windowSetCursor, "gdk_window_set_cursor", d, t)

		if !(bd.opt(&canvasFFI.keymapForDisplay, "gdk_keymap_get_for_display", d, t) &&
			bd.opt(&canvasFFI.keymapTranslate, "gdk_keymap_translate_keyboard_state", d, t)) {
			canvasFFI.keymapTranslate = nil
		}
		if !(bd.opt(&canvasFFI.eventGetSourceDevice, "gdk_event_get_source_device", d, t) &&
			bd.opt(&canvasFFI.deviceGetSource, "gdk_device_get_source", d, t)) {
			canvasFFI.deviceGetSource = nil
		}
		bd.opt(&canvasFFI.eventGetPointerEmulated, "gdk_event_get_pointer_emulated", d, t)
		bd.opt(&canvasFFI.eventGetAxis, "gdk_event_get_axis", d, t)

		bd.req(&canvasFFI.imageSurfaceCreate, "cairo_image_surface_create", c)
		bd.req(&canvasFFI.formatStrideForWidth, "cairo_format_stride_for_width", c)
		bd.req(&canvasFFI.imageSurfaceGetData, "cairo_image_surface_get_data", c)
		bd.req(&canvasFFI.imageSurfaceGetStride, "cairo_image_surface_get_stride", c)
		bd.req(&canvasFFI.surfaceFlush, "cairo_surface_flush", c)
		bd.req(&canvasFFI.surfaceMarkDirty, "cairo_surface_mark_dirty", c)
		bd.req(&canvasFFI.surfaceDestroy, "cairo_surface_destroy", c)
		bd.req(&canvasFFI.surfaceStatus, "cairo_surface_status", c)
		bd.req(&canvasFFI.surfaceSetDeviceScale, "cairo_surface_set_device_scale", c) // cairo >= 1.14
		bd.req(&canvasFFI.setSourceSurface, "cairo_set_source_surface", c)
		bd.req(&canvasFFI.paint, "cairo_paint", c)

		if len(bd.missing) > 0 {
			canvasFFI.err = errors.New("tango/linux: canvas: missing native symbols: " + strings.Join(bd.missing, ", "))
			return
		}
		canvasFFI.dblMS, canvasFFI.dblDist = 400, 5
		loadCanvasClickSettings(&bd, t, o)
	})
	return canvasFFI.err
}

// loadCanvasClickSettings reads gtk-double-click-time / -distance through
// g_object_get_property (g_object_get is variadic).
func loadCanvasClickSettings(bd *binder, t, o ptr) {
	var (
		settingsDefault func() ptr
		getProperty     func(obj ptr, name string, v unsafe.Pointer)
		valueInit       func(v unsafe.Pointer, typ uintptr) unsafe.Pointer
		valueGetInt     func(v unsafe.Pointer) int32
		valueUnset      func(v unsafe.Pointer)
	)
	if !(bd.opt(&settingsDefault, "gtk_settings_get_default", t) &&
		bd.opt(&getProperty, "g_object_get_property", o) &&
		bd.opt(&valueInit, "g_value_init", o) &&
		bd.opt(&valueGetInt, "g_value_get_int", o) &&
		bd.opt(&valueUnset, "g_value_unset", o)) {
		return
	}
	s := settingsDefault()
	if s == 0 {
		return
	}
	read := func(name string) int32 {
		var v canvasGValue
		valueInit(unsafe.Pointer(&v), canvasGTypeInt)
		getProperty(s, name, unsafe.Pointer(&v))
		n := valueGetInt(unsafe.Pointer(&v))
		valueUnset(unsafe.Pointer(&v))
		return n
	}
	if ms := read("gtk-double-click-time"); ms > 0 {
		canvasFFI.dblMS = uint32(ms)
	}
	if d := read("gtk-double-click-distance"); d > 0 {
		canvasFFI.dblDist = float64(d)
	}
}
