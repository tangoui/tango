//go:build linux && !android

package linux

// Window extensions (contract round 2, ext.go): CreatorEx, Geometry,
// Stateful, Styled, PageControl, Dragger, and the surface events
// SurfaceNavigating / LoadFailed / Moved / StateChanged / FilesDropped.
//
// The file-drop handling (claim the drop in "drag-drop", fetch
// text/uri-list ourselves, answer in "drag-data-received") follows the
// approach of Wails v3 pkg/application/linux_cgo_gtk3.go
// (https://github.com/wailsapp/wails, MIT License, Copyright (c) 2018-Present
// Lea Anthony).

import (
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// GTK / GDK / WebKit enum values used here.
const (
	gtkWinPosCenter          = 1 // GtkWindowPosition
	gtkWinPosCenterOnParent  = 4
	gdkHintDialog            = 1 // GdkWindowTypeHint
	gdkHintUtility           = 5
	webkitLoadStarted        = 0 // WebKitLoadEvent
	webkitNetErrorCancelled  = 302
	webkitPrintResponsePrint = 0
)

var (
	_ platform.CreatorEx     = (*backend)(nil)
	_ platform.Geometry      = (*surface)(nil)
	_ platform.Stateful      = (*surface)(nil)
	_ platform.Styled        = (*surface)(nil)
	_ platform.PageControl   = (*surface)(nil)
	_ platform.Dragger       = (*surface)(nil)
	_ platform.EdgeResizer   = (*surface)(nil)
	_ platform.SurfaceStatus = (*surface)(nil)
)

// surfaceExt is the per-surface state of the extensions (surface.ext).
type surfaceExt struct {
	wstate     uint32 // last GdkWindowState from window-state-event
	state      platform.WindowState
	posKnown   bool
	lastX      int // contract coordinates
	lastY      int
	maxW, maxH int
	app        platform.Appearance // as currently applied
	appInit    bool
	rgba       bool // RGBA visual installed
	modalTo    ptr  // transient parent
	dropArmed  bool // our drag-drop handler requested text/uri-list
	dropX      int32
	dropY      int32
}

// defaultAppearance is what a fresh GTK window looks like (the zero
// Appearance with Opacity normalized as SetAppearance does).
var defaultAppearance = platform.Appearance{Opacity: 1}

func (b *backend) NewSurfaceEx(id platform.SurfaceID, o platform.SurfaceOptions, ex platform.SurfaceExtras) (platform.Surface, error) {
	if ex.Bounds.Width > 0 {
		o.Width = ex.Bounds.Width
	}
	if ex.Bounds.Height > 0 {
		o.Height = ex.Bounds.Height
	}
	return b.newSurface(id, o, &ex)
}

// initExt runs from newSurface after the widgets exist and before the
// window is first shown (so the RGBA visual is set before realize). ex is
// nil for plain NewSurface.
func (s *surface) initExt(ex *platform.SurfaceExtras) {
	s.ext.app, s.ext.appInit = defaultAppearance, true
	if s.web != 0 {
		connect(s.web, "load-changed", 1, s.onLoadStarted)
		connectExt(s.web, "load-failed", 3, s.onLoadFailed)
	}
	connect(s.win, "window-state-event", 1, s.onWindowState)
	if wx.ok {
		connect(s.win, "configure-event", 1, s.onMoved)
	}
	if ex == nil || !wx.ok {
		return
	}

	parent := s.b.parentWindow(ex.Parent)
	switch ex.Kind {
	case platform.KindPanel:
		wx.windowSetTypeHint(s.win, gdkHintUtility)
		wx.windowSetSkipTaskbar(s.win, true)
		wx.windowSetSkipPager(s.win, true)
		if parent != 0 {
			gtkWindowSetTransientFor(s.win, parent)
			s.ext.modalTo = parent
		}
	case platform.KindModal, platform.KindSheet: // no sheets on Linux
		if parent != 0 {
			wx.windowSetTypeHint(s.win, gdkHintDialog)
			gtkWindowSetTransientFor(s.win, parent)
			wx.windowSetModal(s.win, true)
			wx.windowSetDestroyWithPar(s.win, true)
			s.ext.modalTo = parent
		}
	}

	if ex.Centered || (ex.Bounds.X == 0 && ex.Bounds.Y == 0) {
		pos := int32(gtkWinPosCenter)
		if s.ext.modalTo != 0 {
			pos = gtkWinPosCenterOnParent
		}
		wx.windowSetPosition(s.win, pos)
	} else {
		x, y := screenOrigin().toGdk(ex.Bounds.X, ex.Bounds.Y)
		wx.windowMove(s.win, int32(x), int32(y))
	}

	if ex.MaxWidth > 0 || ex.MaxHeight > 0 {
		s.ext.maxW, s.ext.maxH = ex.MaxWidth, ex.MaxHeight
		s.applyLimits()
	}
	s.SetAppearance(ex.Appearance) // the zero value is the default: no-op

	if ex.State != platform.StateNormal {
		s.SetState(ex.State) // GTK remembers these until the window is mapped
	}
	if ex.AcceptFiles {
		s.enableFileDrop()
	}
}

// Events ------------------------------------------------------------------

func (s *surface) onLoadStarted(_, event ptr) ptr {
	if !s.closed && int32(event) == webkitLoadStarted {
		s.b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceNavigating, URL: goStr(webkitWebViewGetURI(s.web))})
	}
	return 0
}

// onLoadFailed: load-failed(web_view, load_event, failing_uri, GError*).
// The GError belongs to the emission. Cancelled loads (a new navigation
// replaced the old one) are not failures. Returning FALSE lets WebKit show
// its error page.
func (s *surface) onLoadFailed(a [7]uintptr) uintptr {
	gerr := a[3]
	if s.closed || gerr == 0 {
		return 0
	}
	// struct GError { GQuark domain; gint code; gchar *message; }
	domain := *(*uint32)(toPtr(gerr))
	code := *(*int32)(unsafe.Add(toPtr(gerr), 4))
	if wx.networkErrorQuark != nil && domain == wx.networkErrorQuark() && code == webkitNetErrorCancelled {
		return 0
	}
	msg := goStr(*(*ptr)(unsafe.Add(toPtr(gerr), 8)))
	s.b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceLoadFailed, URL: goStr(a[2]), Error: msg})
	return 0
}

// onMoved is a second configure-event handler (the first reports sizes).
// On Wayland the position is always 0,0, so after the first one no Moved
// events are sent.
func (s *surface) onMoved(_, _ ptr) ptr {
	if s.closed {
		return 0
	}
	x, y := s.position()
	if !s.ext.posKnown || x != s.ext.lastX || y != s.ext.lastY {
		s.ext.posKnown, s.ext.lastX, s.ext.lastY = true, x, y
		s.b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceMoved, X: x, Y: y})
	}
	return 0
}

// onWindowState reads GdkEventWindowState { GdkEventType type; GdkWindow
// *window; gint8 send_event; GdkWindowState changed_mask;
// GdkWindowState new_window_state; } — on LP64 new_window_state is at 24.
func (s *surface) onWindowState(_, ev ptr) ptr {
	if s.closed || ev == 0 {
		return 0
	}
	s.ext.wstate = *(*uint32)(unsafe.Add(toPtr(ev), 24))
	if st := stateFromGdk(s.ext.wstate); st != s.ext.state {
		s.ext.state = st
		s.b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceStateChanged, State: st})
	}
	return 0
}

// File drops ---------------------------------------------------------------

// enableFileDrop handles drops on the web view, or on the canvas widget
// of a canvas surface (canvas_drop_linux.go).
func (s *surface) enableFileDrop() {
	if s.canvas != nil {
		s.canvas.enableFileDrop(func(paths []string, x, y int) {
			if !s.closed {
				s.b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceFilesDropped, Paths: paths, X: x, Y: y})
			}
		})
		return
	}
	if !wx.dropOK || s.web == 0 {
		return
	}
	connectExt(s.web, "drag-drop", 4, s.onDragDrop)
	connectExt(s.web, "drag-data-received", 6, s.onDragData)
}

// onDragDrop: drag-drop(widget, context, x, y, time). External drags that
// offer text/uri-list are claimed (TRUE keeps WebKit from navigating to the
// file); drags started inside the app (HTML5 drag and drop) go to WebKit.
func (s *surface) onDragDrop(a [7]uintptr) uintptr {
	widget, ctx := a[0], a[1]
	if s.closed || wx.dragGetSourceWidget(ctx) != 0 {
		return 0
	}
	uriList := wx.atomIntern("text/uri-list", false)
	found := false
	// struct GList { gpointer data; GList *next; GList *prev; }
	for l := wx.dragContextTargets(ctx); l != 0; l = *(*ptr)(unsafe.Add(toPtr(l), unsafe.Sizeof(ptr(0)))) {
		if *(*ptr)(toPtr(l)) == uriList {
			found = true
			break
		}
	}
	if !found {
		return 0
	}
	s.ext.dropArmed, s.ext.dropX, s.ext.dropY = true, int32(a[2]), int32(a[3])
	wx.dragGetData(widget, ctx, uriList, uint32(a[4]))
	return 1
}

// onDragData: drag-data-received(widget, context, x, y, selection_data,
// info, time). Only the answer to our own request is consumed; WebKit's
// requests during drag-motion pass through.
func (s *surface) onDragData(a [7]uintptr) uintptr {
	widget, ctx, sd, ts := a[0], a[1], a[4], uint32(a[6])
	if s.closed || !s.ext.dropArmed || sd == 0 {
		return 0
	}
	if wx.selectionGetTarget(sd) != wx.atomIntern("text/uri-list", false) {
		return 0
	}
	s.ext.dropArmed = false
	var paths []string
	if v := wx.selectionGetURIs(sd); v != 0 {
		for i := uintptr(0); ; i++ {
			u := *(*ptr)(unsafe.Add(toPtr(v), i*unsafe.Sizeof(ptr(0))))
			if u == 0 {
				break
			}
			if p, ok := localPath(goStr(u)); ok {
				paths = append(paths, p)
			}
		}
		wx.strfreev(v)
	}
	wx.dragFinish(ctx, len(paths) > 0, false, ts)
	wx.signalStopEmission(widget, "drag-data-received")
	if len(paths) > 0 {
		s.b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceFilesDropped, Paths: paths, X: int(s.ext.dropX), Y: int(s.ext.dropY)})
	}
	return 0
}

// localPath converts a dropped URI with GLib (exact hostname rules), or
// the pure-Go fileURIToPath when g_filename_from_uri is missing.
func localPath(uri string) (string, bool) {
	if wx.filenameFromURI != nil {
		if p := wx.filenameFromURI(uri, nil, nil); p != 0 {
			return takeStr(p), true
		}
		return "", false
	}
	return fileURIToPath(uri)
}

// Geometry -------------------------------------------------------------------

// screenOrigin is the primary display's origin in GDK coordinates (see
// screenOffset). Only X11 has global window positions; on Wayland (and
// without GdkMonitor) it is zero, so window positions pass through as
// before.
func screenOrigin() screenOffset {
	if wx.monitorGetGeometry == nil || wx.x11DisplayType == nil || wx.displayGetDefault == nil {
		return screenOffset{}
	}
	d := wx.displayGetDefault()
	if d == 0 || !wx.typeIsA(d, wx.x11DisplayType()) {
		return screenOffset{}
	}
	m := wx.displayGetPrimary(d)
	if m == 0 && wx.displayGetNMonitors(d) > 0 {
		m = wx.displayGetMonitor(d, 0)
	}
	if m == 0 {
		return screenOffset{}
	}
	var geo [4]int32 // GdkRectangle
	wx.monitorGetGeometry(m, &geo)
	return screenOffset{int(geo[0]), int(geo[1])}
}

// position is the window's position in contract coordinates.
// gtk_window_get_position with the default NorthWest gravity reports the
// top-left corner of the window manager frame on X11, as the contract wants
// (gtk_window_move takes the same reference point).
func (s *surface) position() (int, int) {
	var x, y int32
	wx.windowGetPosition(s.win, &x, &y)
	return screenOrigin().toContract(int(x), int(y))
}

func (s *surface) Bounds() platform.Rect {
	if s.closed || !wx.ok {
		return platform.Rect{}
	}
	x, y := s.position()
	w, h := s.Size()
	return platform.Rect{X: x, Y: y, Width: w, Height: h}
}

func (s *surface) SetBounds(r platform.Rect) {
	if s.closed || !wx.ok {
		return
	}
	if r.Width > 0 && r.Height > 0 {
		s.applySize(r.Width, r.Height)
	}
	x, y := screenOrigin().toGdk(r.X, r.Y)
	wx.windowMove(s.win, int32(x), int32(y)) // ignored on Wayland
}

// Center centers on the work area of the window's monitor. Work area and
// gtk_window_move are both in GDK coordinates, so no translation is
// needed. Before the window is realized (or without GdkMonitor) GTK
// centers it on next map.
func (s *surface) Center() {
	if s.closed || !wx.ok {
		return
	}
	if wx.monitorGetWorkarea != nil && wx.displayGetDefault != nil {
		if gw := wx.widgetGetWindow(s.win); gw != 0 {
			if mon := wx.displayMonitorAtWin(wx.displayGetDefault(), gw); mon != 0 {
				var wa [4]int32 // GdkRectangle {x, y, width, height}
				wx.monitorGetWorkarea(mon, &wa)
				var w, h int32
				gtkWindowGetSize(s.win, &w, &h)
				wx.windowMove(s.win, wa[0]+(wa[2]-w)/2, wa[1]+(wa[3]-h)/2)
				return
			}
		}
	}
	wx.windowSetPosition(s.win, gtkWinPosCenter)
}

func (s *surface) SetSizeLimits(minW, minH, maxW, maxH int) {
	if s.closed || !wx.ok {
		return
	}
	s.minW, s.minH = max(minW, 0), max(minH, 0)
	s.ext.maxW, s.ext.maxH = max(maxW, 0), max(maxH, 0)
	if s.resizable { // keep the content-box minimum applySize uses in sync
		mw, mh := int32(-1), int32(-1)
		if s.minW > 0 {
			mw = int32(s.minW)
		}
		if s.minH > 0 {
			mh = int32(s.minH)
		}
		gtkWidgetSetSizeRequest(s.box, mw, mh)
	}
	s.applyLimits()
}

func (s *surface) applyLimits() {
	g, mask := geometryHints(s.minW, s.minH, s.ext.maxW, s.ext.maxH)
	if mask == 0 {
		wx.windowSetGeometryHints(s.win, 0, nil, 0)
		return
	}
	wx.windowSetGeometryHints(s.win, 0, &g, mask)
}

// Stateful -------------------------------------------------------------------

// State is derived from the last window-state-event, so it reflects a
// SetState only once the window manager applied it.
func (s *surface) State() platform.WindowState { return stateFromGdk(s.ext.wstate) }

func (s *surface) SetState(st platform.WindowState) {
	if s.closed || !wx.ok {
		return
	}
	for _, op := range stateOps(s.ext.wstate, st) {
		switch op {
		case opDeiconify:
			wx.windowDeiconify(s.win)
		case opUnfullscreen:
			wx.windowUnfullscreen(s.win)
		case opUnmaximize:
			wx.windowUnmaximize(s.win)
		case opMaximize:
			wx.windowMaximize(s.win)
		case opFullscreen:
			wx.windowFullscreen(s.win)
		case opIconify:
			gtkWindowIconify(s.win)
		}
	}
}

// Styled ---------------------------------------------------------------------

// SetAppearance diffs a against what is applied. Unsupported on GTK:
// TitleBarHidden (no way to keep the WM buttons without a title bar, so it
// is frameless like TitleBarNone), Material, NoShadow (the WM/compositor
// decides), NoMinimize / NoMaximize (no GTK 3 API).
func (s *surface) SetAppearance(a platform.Appearance) {
	if s.closed || !wx.ok {
		return
	}
	if a.Opacity <= 0 || a.Opacity > 1 {
		a.Opacity = 1
	}
	prev := s.ext.app
	if !s.ext.appInit {
		prev = defaultAppearance
	}
	if (a.TitleBar == platform.TitleBarDefault) != (prev.TitleBar == platform.TitleBarDefault) {
		wx.windowSetDecorated(s.win, a.TitleBar == platform.TitleBarDefault)
	}
	if a.Transparent != prev.Transparent || a.Background != prev.Background {
		s.applyBackground(a)
	}
	if a.AlwaysOnTop != prev.AlwaysOnTop {
		wx.windowSetKeepAbove(s.win, a.AlwaysOnTop)
	}
	if a.Opacity != prev.Opacity {
		wx.widgetSetOpacity(s.win, a.Opacity) // needs a compositor
	}
	if a.NoClose != prev.NoClose {
		wx.windowSetDeletable(s.win, !a.NoClose)
	}
	s.ext.app, s.ext.appInit = a, true
}

// applyBackground sets transparency or a solid page background. The RGBA
// visual only takes effect if set before the window is realized; switching
// to transparent at runtime is best effort.
func (s *surface) applyBackground(a platform.Appearance) {
	if a.Transparent {
		if !s.ext.rgba {
			if vis := wx.screenGetRGBAVisual(wx.widgetGetScreen(s.win)); vis != 0 { // 0: no compositor
				wx.widgetSetVisual(s.win, vis)
				s.ext.rgba = true
			}
		}
		wx.widgetSetAppPaintable(s.win, true)
		if s.web != 0 {
			clear := [4]float64{}
			wx.webViewSetBackground(s.web, &clear)
		}
	} else {
		if s.ext.rgba {
			wx.widgetSetVisual(s.win, wx.screenGetSystemVisual(wx.widgetGetScreen(s.win)))
			s.ext.rgba = false
		}
		wx.widgetSetAppPaintable(s.win, false)
		c := [4]float64{1, 1, 1, 1} // WebKit's default
		if a.Background != 0 {
			c = rgbaColor(a.Background)
			c[3] = 1 // opaque: translucency needs Transparent
		}
		if s.web != 0 { // a canvas paints its own background
			wx.webViewSetBackground(s.web, &c)
		}
	}
	wx.widgetQueueDraw(s.win)
}

// PageControl ------------------------------------------------------------------

func (s *surface) Reload() {
	if !s.closed && wx.ok && s.web != 0 {
		wx.webViewReload(s.web)
	}
}

func (s *surface) SetZoom(f float64) {
	if s.closed || !wx.ok || s.web == 0 {
		return
	}
	if f <= 0 {
		f = 1
	}
	wx.webViewSetZoom(s.web, f)
}

func (s *surface) Zoom() float64 {
	if s.closed || !wx.ok || s.web == 0 {
		return 1
	}
	return wx.webViewGetZoom(s.web)
}

// Print shows the GTK print dialog (a nested main loop). The operation is
// released when printing finishes, or right away if the user cancels.
func (s *surface) Print() {
	if s.closed || wx.printOpNew == nil || s.web == 0 {
		return
	}
	op := wx.printOpNew(s.web)
	if op == 0 {
		return
	}
	owned := true
	release := func() {
		if owned {
			owned = false
			gObjectUnref(op)
		}
	}
	connect(op, "finished", 0, func(_, _ ptr) ptr {
		later(0, release) // not from inside the emission
		return 0
	})
	if wx.printOpRunDialog(op, s.win) != webkitPrintResponsePrint {
		release()
	}
}

// Dragger --------------------------------------------------------------------

// pointerRoot returns the pointer position in root (logical) coordinates.
func pointerRoot() (int32, int32, bool) {
	if wx.deviceGetPosition == nil {
		return 0, 0, false
	}
	seat := wx.displayGetSeat(wx.displayGetDefault())
	if seat == 0 {
		return 0, 0, false
	}
	dev := wx.seatGetPointer(seat)
	if dev == 0 {
		return 0, 0, false
	}
	var x, y int32
	wx.deviceGetPosition(dev, nil, &x, &y)
	return x, y, true
}

// StartMove begins a WM move with the primary button (the page saw the
// mouse-down; GDK_CURRENT_TIME stands in for its timestamp).
func (s *surface) StartMove() {
	if s.closed || !wx.ok {
		return
	}
	if x, y, ok := pointerRoot(); ok {
		wx.windowBeginMoveDrag(s.win, 1, x, y, 0)
	}
}

func (s *surface) StartResize(edge string) {
	if s.closed || !wx.ok || !s.resizable {
		return
	}
	e, ok := resizeEdge(edge)
	if !ok {
		return
	}
	if x, y, ok := pointerRoot(); ok {
		wx.windowBeginResizeDrag(s.win, e, 1, x, y, 0)
	}
}

// HandlesEdgeResize implements platform.EdgeResizer: an undecorated GTK
// window has no native resize border, so the core's injected JS must call
// StartResize from the page edges.
func (s *surface) HandlesEdgeResize() bool { return false }

// SurfaceStatus ----------------------------------------------------------------

// IsVisible reports gtk_widget_get_visible on the window: false after Hide
// (and HideApp), still true while minimized.
func (s *surface) IsVisible() bool {
	return !s.closed && wx.widgetGetVisible != nil && wx.widgetGetVisible(s.win)
}

// IsFocused reports gtk_window_is_active: the window has keyboard focus
// within the toplevel that the window manager made active.
func (s *surface) IsFocused() bool {
	return !s.closed && wx.windowIsActive != nil && wx.windowIsActive(s.win)
}
