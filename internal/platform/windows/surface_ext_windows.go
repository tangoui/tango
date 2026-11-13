//go:build windows && (amd64 || arm64)

package windows

// Round-2 window capabilities of a surface: creation extras (CreatorEx),
// Geometry, Stateful, Styled, PageControl, Dragger, and the navigation,
// load-failure, move, state and file-drop events. Pure logic is in
// windowext.go.

import (
	"strconv"
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.CreatorEx     = (*Backend)(nil)
	_ platform.Geometry      = (*surface)(nil)
	_ platform.Stateful      = (*surface)(nil)
	_ platform.Styled        = (*surface)(nil)
	_ platform.PageControl   = (*surface)(nil)
	_ platform.Dragger       = (*surface)(nil)
	_ platform.SurfaceStatus = (*surface)(nil)
)

var (
	dwmapi = systemDLL("dwmapi.dll")

	wxGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	wxSetWindowLongPtrW        = user32.NewProc("SetWindowLongPtrW")
	wxGetWindowPlacement       = user32.NewProc("GetWindowPlacement")
	wxSetWindowPlacement       = user32.NewProc("SetWindowPlacement")
	wxIsZoomed                 = user32.NewProc("IsZoomed")
	wxEnableWindow             = user32.NewProc("EnableWindow")
	wxGetSystemMenu            = user32.NewProc("GetSystemMenu")
	wxEnableMenuItem           = user32.NewProc("EnableMenuItem")
	wxSetLayeredWindowAttrs    = user32.NewProc("SetLayeredWindowAttributes")
	wxGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	wxGetSystemMetricsForDpi   = user32.NewProc("GetSystemMetricsForDpi")
	wxGetCursorPos             = user32.NewProc("GetCursorPos")
	wxScreenToClient           = user32.NewProc("ScreenToClient")
	wxReleaseCapture           = user32.NewProc("ReleaseCapture")
	wxDwmExtendFrameIntoClient = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
	wxDwmSetWindowAttribute    = dwmapi.NewProc("DwmSetWindowAttribute")
)

const (
	wxWmNcCalcSize     = 0x0083
	wxWmNcActivate     = 0x0086
	wxWmNcLButtonDown  = 0x00A1
	wxGwlStyle         = -16
	wxGwlExStyle       = -20
	wxWsExToolWindow   = 0x00000080
	wxWsExLayered      = 0x00080000
	wxLwaAlpha         = 0x2
	wxScClose          = 0xF060
	wxMfByCommand      = 0x0000
	wxMfEnabled        = 0x0000
	wxMfGrayed         = 0x0001
	wxSwpFrameChanged  = 0x0020
	wxSwpNoOwnerZOrder = 0x0200
	wxSmCxSizeFrame    = 32
	wxSmCySizeFrame    = 33
	wxSmCxPaddedBorder = 92

	wxDwmwaSystemBackdropType = 38

	// HWND_TOPMOST (-1), HWND_NOTOPMOST (-2)
	wxHwndTopmost   = ^uintptr(0)
	wxHwndNoTopmost = ^uintptr(1)
)

// WebView2 vtable indices (WebView2.h; cross-checked with mygo, MIT, see
// doc.go).
const (
	ctlGetZoomFactor              = 7  // ICoreWebView2Controller
	ctlPutZoomFactor              = 8  // ICoreWebView2Controller
	ctl2PutDefaultBackgroundColor = 27 // ICoreWebView2Controller2
	wvAddNavigationStarting       = 7  // ICoreWebView2
	wvReload                      = 31 // ICoreWebView2
	navStartingGetURI             = 3  // ICoreWebView2NavigationStartingEventArgs
	navStartingPutCancel          = 8
	navCompletedGetWebErrorStatus = 4 // ICoreWebView2NavigationCompletedEventArgs

	webErrorStatusOperationCanceled = 14 // COREWEBVIEW2_WEB_ERROR_STATUS_OPERATION_CANCELED
)

var iidICoreWebView2Controller2 = guid("c979903e-d4ca-4228-92eb-47ee3fa96eab")

type windowPlacement struct {
	Length         uint32
	Flags          uint32
	ShowCmd        uint32
	MinPosition    point
	MaxPosition    point
	NormalPosition rect
}

type ncCalcSizeParams struct {
	Rgrc [3]rect
	Pos  uintptr
}

// surfaceExt is the round-2 state of a surface.
type surfaceExt struct {
	kind  platform.SurfaceKind
	owner uintptr // parent HWND of a modal / panel
	// ownerDisabled: a modal disabled its owner while shown.
	ownerDisabled bool

	maxW, maxH int // logical client size limits, 0 = none

	app       platform.Appearance
	hasApp    bool // an Appearance was applied (CreatorEx or SetAppearance)
	frameless bool

	acceptFiles bool
	// dragSeen is when a trusted page last reported a file drag over it
	// (wxDragSessionScript); a file: navigation counts as a drop only
	// shortly after. selfNav is the last URL the core navigated to.
	dragSeen time.Time
	selfNav  string

	pendingState platform.WindowState // applied on first Show
	fullscreen   bool
	savedStyle   uintptr
	savedPlace   windowPlacement

	lastState    platform.WindowState
	lastX, lastY int
	navURL       string  // main-frame navigation in progress
	zoom         float64 // arm64: CSS zoom factor (0 = 1)
}

// platform.CreatorEx

func (b *Backend) NewSurfaceEx(id platform.SurfaceID, opts platform.SurfaceOptions, ex platform.SurfaceExtras) (platform.Surface, error) {
	return b.newSurface(id, opts, &ex)
}

// prepareExtras is called before CreateWindowExW: it returns the extended
// style and the owner window, and sets what WM_NCCALCSIZE needs during
// creation.
func (s *surface) prepareExtras(ex *platform.SurfaceExtras) (exStyle, owner uintptr) {
	if ex == nil {
		return 0, 0
	}
	if ex.Bounds.Width > 0 {
		s.opts.Width = ex.Bounds.Width
	}
	if ex.Bounds.Height > 0 {
		s.opts.Height = ex.Bounds.Height
	}
	s.ext.maxW, s.ext.maxH = ex.MaxWidth, ex.MaxHeight
	s.ext.kind = ex.Kind
	if s.ext.kind == platform.KindSheet {
		s.ext.kind = platform.KindModal
	}
	if p := s.b.byID[ex.Parent]; p != nil && !p.closed && s.ext.kind != platform.KindWindow {
		s.ext.owner = p.hwnd // owned: stays above the parent, goes with it
	} else if s.ext.kind == platform.KindModal {
		s.ext.kind = platform.KindWindow // no parent: a normal window
	}
	if s.ext.kind == platform.KindPanel {
		exStyle |= wxWsExToolWindow
	}
	s.ext.acceptFiles = ex.AcceptFiles
	s.ext.pendingState = ex.State
	s.ext.frameless = frameless(ex.Appearance)
	return exStyle, s.ext.owner
}

// applyExtras runs after the window exists and has its default placement.
func (s *surface) applyExtras(ex *platform.SurfaceExtras) {
	if ex != nil {
		s.SetAppearance(ex.Appearance)
		if !placeCentered(ex) {
			s.SetBounds(ex.Bounds)
		} else {
			s.placeInitially() // the client size may have changed with the frame
		}
	}
	s.ext.lastX, s.ext.lastY = s.position()
}

// adjustRect grows a client rectangle to the window rectangle; a frameless
// window has no non-client area.
func (s *surface) adjustRect(r *rect, dpi int) {
	if s.ext.frameless {
		return
	}
	adjustWindowRect(r, s.style, s.menu != 0, dpi)
}

// extMessage handles messages before the base handler; ok means handled.
func (s *surface) extMessage(m uint32, wp, lp uintptr) (uintptr, bool) {
	if m == ixState.buttonMsg && m != 0 {
		s.b.taskbarButtonCreated(s.hwnd)
		return 0, true
	}
	switch m {
	case wxWmNcCalcSize:
		if wp == 0 || !s.ext.frameless {
			return 0, false
		}
		if s.ext.fullscreen {
			return 0, true // the whole window is client area
		}
		p := (*ncCalcSizeParams)(native(lp))
		r := &p.Rgrc[0]
		fx, fy := s.frameThickness()
		r.Left, r.Top, r.Right, r.Bottom = maximizedClient(r.Left, r.Top, r.Right, r.Bottom, s.isZoomed(), fx, fy)
		return 0, true
	case wxWmNcActivate:
		if s.ext.frameless {
			// lParam -1: do not repaint the (removed) non-client area,
			// which would otherwise flash a caption on Windows 10.
			r, _, _ := procDefWindowProcW.Call(s.hwnd, uintptr(m), wp, ^uintptr(0))
			return r, true
		}
	case wmDestroy:
		s.releaseOwner()
	}
	return 0, false
}

// frameThickness is the sizing frame (plus padded border) at the window's
// DPI, in pixels.
func (s *surface) frameThickness() (int32, int32) {
	dpi := dpiOf(s.hwnd)
	metric := func(i int) int32 {
		if has(wxGetSystemMetricsForDpi) {
			r, _, _ := wxGetSystemMetricsForDpi.Call(uintptr(i), uintptr(dpi))
			return int32(r)
		}
		r, _, _ := wxGetSystemMetrics.Call(uintptr(i))
		return int32(r)
	}
	pad := metric(wxSmCxPaddedBorder)
	return metric(wxSmCxSizeFrame) + pad, metric(wxSmCySizeFrame) + pad
}

// maxTrack applies the maximum size to WM_GETMINMAXINFO.
func (s *surface) maxTrack(lp uintptr) {
	if s.ext.fullscreen || (s.ext.maxW <= 0 && s.ext.maxH <= 0) {
		return
	}
	dpi := dpiOf(s.hwnd)
	r := rect{Right: int32(dipToPx(s.ext.maxW, dpi)), Bottom: int32(dipToPx(s.ext.maxH, dpi))}
	s.adjustRect(&r, dpi)
	mmi := (*minMaxInfo)(native(lp))
	if s.ext.maxW > 0 {
		mmi.MaxTrackSize.X = r.Right - r.Left
	}
	if s.ext.maxH > 0 {
		mmi.MaxTrackSize.Y = r.Bottom - r.Top
	}
}

// sized reports state changes from WM_SIZE.
func (s *surface) sized(wp uintptr) {
	s.reportState(sizeState(wp, s.ext.fullscreen))
}

func (s *surface) reportState(st platform.WindowState) {
	if !s.created || s.closed || st == s.ext.lastState {
		return
	}
	s.ext.lastState = st
	s.event(platform.SurfaceEvent{Kind: platform.SurfaceStateChanged, State: st})
}

// moved reports position changes from WM_MOVE.
func (s *surface) moved() {
	if !s.created || s.closed || s.isIconic() {
		return
	}
	x, y := s.position()
	if x == s.ext.lastX && y == s.ext.lastY {
		return
	}
	s.ext.lastX, s.ext.lastY = x, y
	s.event(platform.SurfaceEvent{Kind: platform.SurfaceMoved, X: x, Y: y})
}

// position is the window's top-left corner in logical pixels.
func (s *surface) position() (int, int) {
	var r rect
	procGetWindowRect.Call(s.hwnd, uintptr(unsafe.Pointer(&r)))
	dpi := dpiOf(s.hwnd)
	return pxToDIP(int(r.Left), dpi), pxToDIP(int(r.Top), dpi)
}

func (s *surface) isIconic() bool {
	r, _, _ := procIsIconic.Call(s.hwnd)
	return r != 0
}

func (s *surface) isZoomed() bool {
	r, _, _ := wxIsZoomed.Call(s.hwnd)
	return r != 0
}

func windowLong(hwnd uintptr, index int) uintptr {
	r, _, _ := wxGetWindowLongPtrW.Call(hwnd, uintptr(index))
	return r
}

func setWindowLong(hwnd uintptr, index int, v uintptr) {
	wxSetWindowLongPtrW.Call(hwnd, uintptr(index), v)
}

// Show/Hide/Close hooks.

func (s *surface) noActivate() bool { return s.ext.kind == platform.KindPanel }

func (s *surface) showCmd() uintptr {
	if s.noActivate() {
		return showNA
	}
	return swShow
}

// beforeShow disables a modal's owner.
func (s *surface) beforeShow() {
	if s.ext.kind == platform.KindModal && s.ext.owner != 0 && !s.ext.ownerDisabled {
		s.ext.ownerDisabled = true
		wxEnableWindow.Call(s.ext.owner, 0)
	}
}

// afterFirstShow enters a fullscreen initial state.
func (s *surface) afterFirstShow() {
	if s.ext.pendingState == platform.StateFullscreen {
		s.enterFullscreen()
	}
}

// releaseOwner re-enables a modal's owner; called before the modal hides
// or is destroyed, so activation returns to the owner.
func (s *surface) releaseOwner() {
	if s.ext.ownerDisabled {
		s.ext.ownerDisabled = false
		wxEnableWindow.Call(s.ext.owner, 1)
	}
}

// platform.SurfaceStatus

// IsVisible is IsWindowVisible: false after Hide / HideApp, true while
// minimized.
func (s *surface) IsVisible() bool {
	if s.closed || s.hwnd == 0 {
		return false
	}
	r, _, _ := ixIsWindowVisible.Call(s.hwnd)
	return r != 0
}

// IsFocused reports whether the window is the foreground window (active
// in the foreground app), as SurfaceFocused / SurfaceBlurred track it.
func (s *surface) IsFocused() bool {
	if s.closed || s.hwnd == 0 {
		return false
	}
	fg, _, _ := procGetForegroundWindow.Call()
	return fg == s.hwnd
}

// platform.Geometry

func (s *surface) Bounds() platform.Rect {
	if s.closed {
		return platform.Rect{X: s.ext.lastX, Y: s.ext.lastY, Width: s.lastW, Height: s.lastH}
	}
	x, y := s.position()
	w, h := s.Size()
	return platform.Rect{X: x, Y: y, Width: w, Height: h}
}

// SetBounds places the window's top-left corner at (X, Y) and sizes its
// client area to Width×Height (logical); a zero size keeps the size.
func (s *surface) SetBounds(b platform.Rect) {
	if s.closed {
		return
	}
	dpi := dpiOf(s.hwnd)
	flags := uintptr(swpNoZOrder | swpNoActivate)
	var w, h int32
	if b.Width > 0 && b.Height > 0 {
		r := rect{Right: int32(dipToPx(b.Width, dpi)), Bottom: int32(dipToPx(b.Height, dpi))}
		s.adjustRect(&r, dpi)
		w, h = r.Right-r.Left, r.Bottom-r.Top
	} else {
		flags |= swpNoSize
	}
	procSetWindowPos.Call(s.hwnd, 0, uintptr(int32(dipToPx(b.X, dpi))), uintptr(int32(dipToPx(b.Y, dpi))),
		uintptr(w), uintptr(h), flags)
}

func (s *surface) Center() {
	if s.closed {
		return
	}
	var r rect
	procGetWindowRect.Call(s.hwnd, uintptr(unsafe.Pointer(&r)))
	mon, _, _ := procMonitorFromWindow.Call(s.hwnd, monitorDefaultToNearest)
	mi := monitorInfo{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if ok, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); mon == 0 || ok == 0 {
		return
	}
	x, y := centerIn(int(mi.Work.Left), int(mi.Work.Top), int(mi.Work.Right), int(mi.Work.Bottom),
		int(r.Right-r.Left), int(r.Bottom-r.Top))
	procSetWindowPos.Call(s.hwnd, 0, uintptr(int32(x)), uintptr(int32(y)), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
}

func (s *surface) SetSizeLimits(minW, minH, maxW, maxH int) {
	s.opts.MinWidth, s.opts.MinHeight = minW, minH
	s.ext.maxW, s.ext.maxH = maxW, maxH
	if s.closed || s.ext.fullscreen || s.isZoomed() || s.isIconic() {
		return
	}
	w, h := s.Size()
	if nw, nh := clampSize(w, minW, maxW), clampSize(h, minH, maxH); nw != w || nh != h {
		s.SetSize(nw, nh)
	}
}

// platform.Stateful

func (s *surface) State() platform.WindowState {
	if s.closed {
		return s.ext.lastState
	}
	if !s.shown {
		return s.ext.pendingState
	}
	return stateOf(s.isIconic(), s.isZoomed(), s.ext.fullscreen)
}

func (s *surface) SetState(st platform.WindowState) {
	if s.closed {
		return
	}
	if !s.shown {
		s.ext.pendingState = st // applied by the first Show
		return
	}
	if st != platform.StateFullscreen && s.ext.fullscreen {
		s.exitFullscreen()
	}
	switch st {
	case platform.StateNormal:
		if s.isIconic() || s.isZoomed() {
			procShowWindow.Call(s.hwnd, showCmdRestore)
		}
	case platform.StateMinimized:
		procShowWindow.Call(s.hwnd, showCmdMinimize)
	case platform.StateMaximized:
		procShowWindow.Call(s.hwnd, showCmdMaximize)
	case platform.StateFullscreen:
		s.enterFullscreen()
	}
}

// enterFullscreen turns the window into a popup covering its monitor,
// keeping its style and placement to come back to.
func (s *surface) enterFullscreen() {
	if s.ext.fullscreen {
		return
	}
	if s.isIconic() {
		procShowWindow.Call(s.hwnd, showCmdRestore)
	}
	mon, _, _ := procMonitorFromWindow.Call(s.hwnd, monitorDefaultToNearest)
	mi := monitorInfo{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if ok, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); mon == 0 || ok == 0 {
		return
	}
	s.ext.savedPlace = windowPlacement{}
	s.ext.savedPlace.Length = uint32(unsafe.Sizeof(s.ext.savedPlace))
	wxGetWindowPlacement.Call(s.hwnd, uintptr(unsafe.Pointer(&s.ext.savedPlace)))
	s.ext.savedStyle = windowLong(s.hwnd, wxGwlStyle)
	s.ext.fullscreen = true // before the frame changes: WM_NCCALCSIZE / WM_SIZE read it
	setWindowLong(s.hwnd, wxGwlStyle, uintptr(fullscreenStyle(uint32(s.ext.savedStyle))))
	m := mi.Monitor
	procSetWindowPos.Call(s.hwnd, 0, uintptr(m.Left), uintptr(m.Top), uintptr(m.Right-m.Left), uintptr(m.Bottom-m.Top),
		wxSwpFrameChanged|wxSwpNoOwnerZOrder)
	s.reportState(platform.StateFullscreen)
}

func (s *surface) exitFullscreen() {
	if !s.ext.fullscreen {
		return
	}
	s.ext.fullscreen = false
	setWindowLong(s.hwnd, wxGwlStyle, s.ext.savedStyle)
	wxSetWindowPlacement.Call(s.hwnd, uintptr(unsafe.Pointer(&s.ext.savedPlace)))
	procSetWindowPos.Call(s.hwnd, 0, 0, 0, 0, 0,
		swpNoMove|swpNoSize|swpNoZOrder|wxSwpNoOwnerZOrder|wxSwpFrameChanged)
	s.reportState(stateOf(s.isIconic(), s.isZoomed(), false))
}

// platform.Styled

func (s *surface) SetAppearance(a platform.Appearance) {
	if s.closed {
		return
	}
	first := !s.ext.hasApp
	old := s.ext.app
	s.ext.app, s.ext.hasApp = a, true

	// Frame and caption buttons.
	style := uintptr(boxStyle(s.style, a, s.opts.Resizable))
	nowFrameless := frameless(a)
	frameChanged := uintptr(s.style) != style || nowFrameless != s.ext.frameless || first
	s.style = uint32(style)
	s.ext.frameless = nowFrameless
	if s.ext.fullscreen {
		s.ext.savedStyle = s.ext.savedStyle&^(wsMinimizeBox|wsMaximizeBox) | style&(wsMinimizeBox|wsMaximizeBox)
	} else if frameChanged {
		cur := windowLong(s.hwnd, wxGwlStyle)
		setWindowLong(s.hwnd, wxGwlStyle, cur&^(wsMinimizeBox|wsMaximizeBox)|style&(wsMinimizeBox|wsMaximizeBox))
		procSetWindowPos.Call(s.hwnd, 0, 0, 0, 0, 0,
			swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|wxSwpNoOwnerZOrder|wxSwpFrameChanged)
	}
	if first || old.NoClose != a.NoClose {
		if menu, _, _ := wxGetSystemMenu.Call(s.hwnd, 0); menu != 0 {
			state := uintptr(wxMfByCommand | wxMfEnabled)
			if a.NoClose {
				state = wxMfByCommand | wxMfGrayed
			}
			wxEnableMenuItem.Call(menu, wxScClose, state)
		}
	}

	// DWM: frame extension (shadow, transparency) and backdrop.
	if m := frameMargins(a); first || m != frameMargins(old) {
		if has(wxDwmExtendFrameIntoClient) {
			wxDwmExtendFrameIntoClient.Call(s.hwnd, uintptr(unsafe.Pointer(&m)))
		}
	}
	if (first && a.Material != platform.MaterialNone) || (!first && a.Material != old.Material) {
		if has(wxDwmSetWindowAttribute) { // Windows 11 22H2+; fails harmlessly before
			v := backdropType(a.Material)
			wxDwmSetWindowAttribute.Call(s.hwnd, wxDwmwaSystemBackdropType, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
		}
	}

	if first || old.AlwaysOnTop != a.AlwaysOnTop {
		after := wxHwndNoTopmost
		if a.AlwaysOnTop {
			after = wxHwndTopmost
		}
		if a.AlwaysOnTop || !first {
			procSetWindowPos.Call(s.hwnd, after, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		}
	}

	if first || old.Opacity != a.Opacity {
		alpha, layered := layeredAlpha(a.Opacity)
		ex := windowLong(s.hwnd, wxGwlExStyle)
		if layered {
			if ex&wxWsExLayered == 0 {
				setWindowLong(s.hwnd, wxGwlExStyle, ex|wxWsExLayered)
			}
			wxSetLayeredWindowAttrs.Call(s.hwnd, 0, uintptr(alpha), wxLwaAlpha)
		} else if ex&wxWsExLayered != 0 {
			setWindowLong(s.hwnd, wxGwlExStyle, ex&^wxWsExLayered)
		}
	}

	if s.canvas != nil {
		s.canvas.setTransparent(canvasSeeThrough(a))
	}
	if first || webviewColor(old) != webviewColor(a) {
		color := webviewColor(a)
		s.whenReady(func() { s.putBackground(color) })
	}
}

// putBackground sets the WebView's default background
// (ICoreWebView2Controller2; the color is passed by value in a register).
func (s *surface) putBackground(color uintptr) {
	c2 := queryInterface(s.controller, &iidICoreWebView2Controller2)
	if c2 == 0 {
		return
	}
	comCall(c2, ctl2PutDefaultBackgroundColor, color)
	release(c2)
}

// platform.PageControl

func (s *surface) Reload() {
	s.whenReady(func() { comCall(s.webview, wvReload) })
}

func (s *surface) SetZoom(factor float64) {
	if factor <= 0 {
		factor = 1
	}
	s.ext.zoom = factor
	s.whenReady(func() { putZoom(s, factor) })
}

func (s *surface) Zoom() float64 {
	if zoomByScript || s.controller == 0 {
		if s.ext.zoom == 0 {
			return 1
		}
		return s.ext.zoom
	}
	var f float64
	comCall(s.controller, ctlGetZoomFactor, uintptr(unsafe.Pointer(&f)))
	if f <= 0 {
		return 1
	}
	return f
}

func (s *surface) Print() { s.Eval("window.print()") }

// cssZoom zooms the page with CSS (arm64, see zoom_windows_arm64.go).
func (s *surface) cssZoom(f float64) {
	withHandler(noopHandler, func(h uintptr) uintptr {
		js := "document.documentElement.style.zoom=" + strconv.FormatFloat(f, 'f', -1, 64)
		return comCall(s.webview, wvExecuteScript, uintptr(unsafe.Pointer(u16(js))), h)
	})
}

// platform.Dragger

// StartMove hands the current mouse-down to the window manager as a drag
// of the caption. Posted, so the caller is not blocked in the move loop.
func (s *surface) StartMove() {
	s.startNonClientDrag(htCaption)
}

func (s *surface) StartResize(edge string) {
	ht, ok := edgeHitTest(edge)
	if !ok || !s.opts.Resizable || s.ext.fullscreen || s.isZoomed() {
		return
	}
	s.startNonClientDrag(ht)
}

func (s *surface) startNonClientDrag(ht uintptr) {
	if s.closed || s.ext.fullscreen {
		return
	}
	var p point
	wxGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	wxReleaseCapture.Call()
	lp := uintptr(uint16(int16(p.X))) | uintptr(uint16(int16(p.Y)))<<16
	postMessage(s.hwnd, wxWmNcLButtonDown, ht, lp)
}

// HandlesEdgeResize tells the core (duck-typed, outside the contract) that
// the edges of a frameless window are not resizable natively: the WebView
// covers the whole window, so the core's injected edge detection must call
// StartResize.
func (s *surface) HandlesEdgeResize() bool { return false }

// Navigation events.

// Drag sessions. In windowed hosting WebView2 is the OLE drop target (in
// the browser process) and tells the host nothing about drags: no
// DragEnter/Drop callbacks (those exist only for composition hosting,
// ICoreWebView2CompositionController). On runtimes without
// postMessageWithAdditionalObjects a dropped file shows up only as a
// main-frame navigation to its file: URL, which a file: page (or the app
// itself) can also start. So the page reports drags instead: a document
// script posts wxDragSessionMessage, throttled, for every trusted
// (isTrusted: real user input, not synthesized) dragenter/dragover carrying
// files. webMessageReceived accepts it only from trusted origins, and a
// file: navigation becomes SurfaceFilesDropped only within
// wxDragSessionWindow of the last report.

// wxDragSessionMessage is posted by wxDragSessionScript.
const wxDragSessionMessage = "__tango_drag_session__"

// wxDragSessionWindow is how long after the last report a file: navigation
// is still taken as the drop (the script reports at most every 200 ms
// while the drag hovers; Chromium fires dragover continuously).
const wxDragSessionWindow = time.Second

const wxDragSessionScript = `(function () {
  var wv = window.chrome && window.chrome.webview;
  if (!wv || typeof wv.postMessage !== "function") return;
  var post = wv.postMessage.bind(wv), last = 0;
  function report(e) {
    if (!e.isTrusted || !e.dataTransfer) return;
    var t = e.dataTransfer.types, files = false;
    for (var i = 0; t && i < t.length; i++) if (t[i] === "Files") files = true;
    if (!files) return;
    var now = Date.now();
    if (now - last < 200) return;
    last = now;
    post("` + wxDragSessionMessage + `");
  }
  window.addEventListener("dragenter", report, true);
  window.addEventListener("dragover", report, true);
})();`

// installDragSession adds the drag-session script to surfaces that accept
// files.
func (s *surface) installDragSession() {
	if s.ext.acceptFiles {
		s.addDocumentScript(topFrameOnly(wxDragSessionScript))
	}
}

// noteDragSession records a drag report of a trusted page.
func (s *surface) noteDragSession() {
	if s.ext.acceptFiles {
		s.ext.dragSeen = time.Now()
	}
}

// isDropNavigation reports whether a main-frame navigation to uri is
// WebView2 opening a file dropped on the page: the surface accepts files,
// uri is a file: URL the core did not load itself, and a trusted page
// reported a file drag within wxDragSessionWindow before now.
func isDropNavigation(acceptFiles bool, uri, selfNav string, dragSeen, now time.Time) bool {
	if !acceptFiles || !isFileURL(uri) || dragSeen.IsZero() {
		return false
	}
	if selfNav != "" && uri == selfNav {
		return false
	}
	d := now.Sub(dragSeen)
	return d >= 0 && d <= wxDragSessionWindow
}

// navigationStarting reports main-frame navigations, and turns WebView2's
// navigation to a file dropped during a drag session (isDropNavigation)
// into SurfaceFilesDropped. Other file: navigations go through the
// navigation policy like any other.
func (s *surface) navigationStarting(_, args uintptr) {
	if s.closed {
		return
	}
	uri := comString(args, navStartingGetURI)
	self := s.ext.selfNav
	if self != "" && uri == self {
		s.ext.selfNav = "" // one-shot, like the core's own allowance
	}
	if isDropNavigation(s.ext.acceptFiles, uri, self, s.ext.dragSeen, time.Now()) {
		s.ext.dragSeen = time.Time{} // one drop per session
		comCall(args, navStartingPutCancel, 1)
		path, err := fileURLToPath(uri)
		if err != nil {
			return
		}
		x, y := s.cursorInClient()
		s.event(platform.SurfaceEvent{Kind: platform.SurfaceFilesDropped, Paths: []string{path}, X: x, Y: y})
		return
	}
	if !s.allowNavigation(args, uri) { // WebHost.ShouldNavigate (webview_ext_windows.go)
		return
	}
	s.ext.navURL = uri
	s.event(platform.SurfaceEvent{Kind: platform.SurfaceNavigating, URL: uri})
}

// navigationFailed reports a failed main-frame navigation; one canceled
// (by another navigation, or a dropped file) is not a failure.
func (s *surface) navigationFailed(args uintptr) {
	var status int32
	comCall(args, navCompletedGetWebErrorStatus, uintptr(unsafe.Pointer(&status)))
	if status == webErrorStatusOperationCanceled {
		return
	}
	u := s.ext.navURL
	if u == "" {
		u = comString(s.webview, wvGetSource)
	}
	s.event(platform.SurfaceEvent{Kind: platform.SurfaceLoadFailed, URL: u, Error: "web error status " + strconv.Itoa(int(status))})
}

// navigationSucceeded runs before SurfaceLoaded is reported.
func (s *surface) navigationSucceeded() {
	s.ext.navURL = ""
	if zoomByScript && s.ext.zoom != 0 && s.ext.zoom != 1 {
		putZoom(s, s.ext.zoom) // a new document starts unzoomed
	}
}

func (s *surface) cursorInClient() (int, int) {
	var p point
	wxGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	wxScreenToClient.Call(s.hwnd, uintptr(unsafe.Pointer(&p)))
	dpi := dpiOf(s.hwnd)
	return pxToDIP(int(p.X), dpi), pxToDIP(int(p.Y), dpi)
}
