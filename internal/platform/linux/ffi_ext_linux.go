//go:build linux && !android

package linux

import (
	"sync"

	"github.com/ebitengine/purego"
)

// Native symbols for the window extensions (surface_ext_linux.go). They live
// in their own struct so they never collide with other binding files, and
// are bound by bindWindowExt with a private binder: a missing symbol
// disables the feature instead of failing Init.
var wx struct {
	ok  bool // every core symbol below "core" bound; false: extensions are no-ops
	gdk ptr  // libgdk-3

	// core (GTK >= 3.22, WebKitGTK >= 2.8 — always present with WebKitGTK 4.x)
	windowMove              func(w ptr, x, y int32)
	windowGetPosition       func(w ptr, x, y *int32)
	windowSetPosition       func(w ptr, pos int32)
	windowSetGeometryHints  func(w, widget ptr, g *gdkGeometry, mask uint32)
	windowMaximize          func(w ptr)
	windowUnmaximize        func(w ptr)
	windowDeiconify         func(w ptr)
	windowFullscreen        func(w ptr)
	windowUnfullscreen      func(w ptr)
	windowSetTypeHint       func(w ptr, hint int32)
	windowSetSkipTaskbar    func(w ptr, skip bool)
	windowSetSkipPager      func(w ptr, skip bool)
	windowSetModal          func(w ptr, modal bool)
	windowSetDestroyWithPar func(w ptr, on bool)
	windowSetDecorated      func(w ptr, on bool)
	windowSetKeepAbove      func(w ptr, on bool)
	windowSetDeletable      func(w ptr, on bool)
	windowBeginMoveDrag     func(w ptr, button, rootX, rootY int32, ts uint32)
	windowBeginResizeDrag   func(w ptr, edge, button, rootX, rootY int32, ts uint32)
	widgetSetOpacity        func(w ptr, opacity float64)
	widgetGetScreen         func(w ptr) ptr
	widgetSetVisual         func(w, visual ptr)
	widgetSetAppPaintable   func(w ptr, on bool)
	widgetQueueDraw         func(w ptr)
	screenGetRGBAVisual     func(screen ptr) ptr
	screenGetSystemVisual   func(screen ptr) ptr
	webViewSetBackground    func(v ptr, rgba *[4]float64)
	webViewReload           func(v ptr)
	webViewSetZoom          func(v ptr, z float64)
	webViewGetZoom          func(v ptr) float64
	signalStopEmission      func(instance ptr, signal string)

	// optional
	printOpNew          func(v ptr) ptr
	printOpRunDialog    func(op, parent ptr) int32
	displayGetDefault   func() ptr
	displayGetSeat      func(d ptr) ptr
	seatGetPointer      func(seat ptr) ptr
	deviceGetPosition   func(dev ptr, screen *ptr, x, y *int32)
	widgetGetWindow     func(w ptr) ptr
	displayMonitorAtWin func(d, gdkWin ptr) ptr
	monitorGetWorkarea  func(m ptr, r *[4]int32)
	selectionGetURIs    func(sd ptr) ptr
	selectionGetTarget  func(sd ptr) ptr
	dragFinish          func(ctx ptr, success, del bool, ts uint32)
	dragGetData         func(widget, ctx ptr, target ptr, ts uint32)
	dragGetSourceWidget func(ctx ptr) ptr
	dragContextTargets  func(ctx ptr) ptr // GList* of GdkAtom, owned by ctx
	atomIntern          func(name string, onlyIfExists bool) ptr
	filenameFromURI     func(uri string, host *ptr, gerr *ptr) ptr
	strfreev            func(v ptr)
	networkErrorQuark   func() uint32
	dropOK              bool // every drop symbol bound

	// optional: primary-display origin (screenOrigin)
	displayGetPrimary   func(d ptr) ptr
	displayGetNMonitors func(d ptr) int32
	displayGetMonitor   func(d ptr, i int32) ptr
	monitorGetGeometry  func(m ptr, r *[4]int32)
	x11DisplayType      func() ptr // absent in Wayland-only GDK builds
	typeIsA             func(instance, typ ptr) bool

	// optional: SurfaceStatus
	widgetGetVisible func(w ptr) bool
	windowIsActive   func(w ptr) bool
}

func bindWindowExt() {
	wx.gdk, _, _ = open("libgdk-3.so.0")
	var bd binder
	t, d, w, g, o := libGTK, wx.gdk, libWebKit, libGLib, libGObject

	bd.req(&wx.windowMove, "gtk_window_move", t)
	bd.req(&wx.windowGetPosition, "gtk_window_get_position", t)
	bd.req(&wx.windowSetPosition, "gtk_window_set_position", t)
	bd.req(&wx.windowSetGeometryHints, "gtk_window_set_geometry_hints", t)
	bd.req(&wx.windowMaximize, "gtk_window_maximize", t)
	bd.req(&wx.windowUnmaximize, "gtk_window_unmaximize", t)
	bd.req(&wx.windowDeiconify, "gtk_window_deiconify", t)
	bd.req(&wx.windowFullscreen, "gtk_window_fullscreen", t)
	bd.req(&wx.windowUnfullscreen, "gtk_window_unfullscreen", t)
	bd.req(&wx.windowSetTypeHint, "gtk_window_set_type_hint", t)
	bd.req(&wx.windowSetSkipTaskbar, "gtk_window_set_skip_taskbar_hint", t)
	bd.req(&wx.windowSetSkipPager, "gtk_window_set_skip_pager_hint", t)
	bd.req(&wx.windowSetModal, "gtk_window_set_modal", t)
	bd.req(&wx.windowSetDestroyWithPar, "gtk_window_set_destroy_with_parent", t)
	bd.req(&wx.windowSetDecorated, "gtk_window_set_decorated", t)
	bd.req(&wx.windowSetKeepAbove, "gtk_window_set_keep_above", t)
	bd.req(&wx.windowSetDeletable, "gtk_window_set_deletable", t)
	bd.req(&wx.windowBeginMoveDrag, "gtk_window_begin_move_drag", t)
	bd.req(&wx.windowBeginResizeDrag, "gtk_window_begin_resize_drag", t)
	bd.req(&wx.widgetSetOpacity, "gtk_widget_set_opacity", t)
	bd.req(&wx.widgetGetScreen, "gtk_widget_get_screen", t)
	bd.req(&wx.widgetSetVisual, "gtk_widget_set_visual", t)
	bd.req(&wx.widgetSetAppPaintable, "gtk_widget_set_app_paintable", t)
	bd.req(&wx.widgetQueueDraw, "gtk_widget_queue_draw", t)
	bd.req(&wx.screenGetRGBAVisual, "gdk_screen_get_rgba_visual", d, t)
	bd.req(&wx.screenGetSystemVisual, "gdk_screen_get_system_visual", d, t)
	bd.req(&wx.webViewSetBackground, "webkit_web_view_set_background_color", w)
	bd.req(&wx.webViewReload, "webkit_web_view_reload", w)
	bd.req(&wx.webViewSetZoom, "webkit_web_view_set_zoom_level", w)
	bd.req(&wx.webViewGetZoom, "webkit_web_view_get_zoom_level", w)
	bd.req(&wx.signalStopEmission, "g_signal_stop_emission_by_name", o)
	wx.ok = len(bd.missing) == 0

	if bd.opt(&wx.printOpNew, "webkit_print_operation_new", w) &&
		!bd.opt(&wx.printOpRunDialog, "webkit_print_operation_run_dialog", w) {
		wx.printOpNew = nil
	}
	if !(bd.opt(&wx.displayGetDefault, "gdk_display_get_default", d, t) &&
		bd.opt(&wx.displayGetSeat, "gdk_display_get_default_seat", d, t) &&
		bd.opt(&wx.seatGetPointer, "gdk_seat_get_pointer", d, t) &&
		bd.opt(&wx.deviceGetPosition, "gdk_device_get_position", d, t)) {
		wx.deviceGetPosition = nil
	}
	if !(bd.opt(&wx.widgetGetWindow, "gtk_widget_get_window", t) &&
		bd.opt(&wx.displayMonitorAtWin, "gdk_display_get_monitor_at_window", d, t) &&
		bd.opt(&wx.monitorGetWorkarea, "gdk_monitor_get_workarea", d, t)) {
		wx.monitorGetWorkarea = nil
	}
	wx.dropOK = bd.opt(&wx.selectionGetURIs, "gtk_selection_data_get_uris", t) &&
		bd.opt(&wx.selectionGetTarget, "gtk_selection_data_get_target", t) &&
		bd.opt(&wx.dragFinish, "gtk_drag_finish", t) &&
		bd.opt(&wx.dragGetData, "gtk_drag_get_data", t) &&
		bd.opt(&wx.dragGetSourceWidget, "gtk_drag_get_source_widget", t) &&
		bd.opt(&wx.dragContextTargets, "gdk_drag_context_list_targets", d, t) &&
		bd.opt(&wx.atomIntern, "gdk_atom_intern", d, t) &&
		bd.opt(&wx.strfreev, "g_strfreev", g)
	bd.opt(&wx.filenameFromURI, "g_filename_from_uri", g)
	bd.opt(&wx.networkErrorQuark, "webkit_network_error_quark", w)
	if !(bd.opt(&wx.displayGetPrimary, "gdk_display_get_primary_monitor", d, t) &&
		bd.opt(&wx.displayGetNMonitors, "gdk_display_get_n_monitors", d, t) &&
		bd.opt(&wx.displayGetMonitor, "gdk_display_get_monitor", d, t) &&
		bd.opt(&wx.monitorGetGeometry, "gdk_monitor_get_geometry", d, t)) {
		wx.monitorGetGeometry = nil
	}
	if !(bd.opt(&wx.x11DisplayType, "gdk_x11_display_get_type", d, t) &&
		bd.opt(&wx.typeIsA, "g_type_check_instance_is_a", o)) {
		wx.x11DisplayType = nil
	}
	bd.opt(&wx.widgetGetVisible, "gtk_widget_get_visible", t)
	bd.opt(&wx.windowIsActive, "gtk_window_is_active", t)
}

// Trampolines for signals with more than two arguments. Like the ones in
// callbacks_linux.go they are created once per process and look the Go
// handler up by user_data in extDispatch. One per arity, because user_data
// is the last C argument.
var (
	extDispatch = newExtTable()
	extOnce     sync.Once
	cbExt4      ptr // (instance, a, b, c, data): "load-failed"
	cbExt5      ptr // (instance, a, b, c, d, data): "drag-drop"
	cbExt7      ptr // (instance, a..f, data): "drag-data-received"
	cbExtFree   ptr // GClosureNotify
)

func initExtCallbacks() {
	extOnce.Do(func() {
		cbExt4 = purego.NewCallback(func(i, a, b, c, data ptr) ptr {
			return extDispatch.call(data, [7]uintptr{i, a, b, c})
		})
		cbExt5 = purego.NewCallback(func(i, a, b, c, d, data ptr) ptr {
			return extDispatch.call(data, [7]uintptr{i, a, b, c, d})
		})
		cbExt7 = purego.NewCallback(func(i, a, b, c, d, e, f, data ptr) ptr {
			return extDispatch.call(data, [7]uintptr{i, a, b, c, d, e, f})
		})
		cbExtFree = purego.NewCallback(func(data ptr) { extDispatch.remove(data) })
	})
}

// connectExt attaches h to a signal with 3, 4 or 6 arguments after the
// instance. args[0] is the instance.
func connectExt(instance ptr, signal string, args int, h extHandler) uint {
	initExtCallbacks()
	var tr ptr
	switch args {
	case 3:
		tr = cbExt4
	case 4:
		tr = cbExt5
	case 6:
		tr = cbExt7
	default:
		panic("tango/linux: connectExt: unsupported arity")
	}
	id := extDispatch.add(h)
	return gSignalConnectData(instance, signal, tr, id, cbExtFree, 0)
}
