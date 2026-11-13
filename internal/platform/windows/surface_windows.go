//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

type surface struct {
	b    *Backend
	id   platform.SurfaceID
	opts platform.SurfaceOptions
	hwnd uintptr

	style   uint32
	created bool // NewSurface returned: events may be reported
	closed  bool
	shown   bool

	canvas *canvas // canvas surfaces only (canvas_windows.go); no WebView then

	controller uintptr // ICoreWebView2Controller
	webview    uintptr // ICoreWebView2
	ready      bool    // webview set up; pending ran
	failed     error
	pending    []func()

	lastW, lastH int // last reported logical client size

	// Menu state: menu is owned by the window (DestroyWindow frees it).
	menu         uintptr
	accel        uintptr
	spec         *menuSpec
	explicitMenu bool // set with SetAppMenu(id, ...), not the default

	ext surfaceExt // round-2 window capabilities (surface_ext_windows.go)
	web webExt     // advanced WebView capabilities (webview_ext_windows.go)
}

func (b *Backend) NewSurface(id platform.SurfaceID, opts platform.SurfaceOptions) (platform.Surface, error) {
	return b.newSurface(id, opts, nil)
}

// newSurface creates a WebView surface; ex is nil for NewSurface. Canvas
// surfaces (canvas_surface_windows.go) share newWindow and finishSurface
// and put a canvas child window where the WebView would go.
func (b *Backend) newSurface(id platform.SurfaceID, opts platform.SurfaceOptions, ex *platform.SurfaceExtras) (platform.Surface, error) {
	if b.h == nil {
		return nil, fmt.Errorf("tango: NewSurface before Init")
	}
	if b.envErr != nil {
		// The environment failed earlier; let a later surface try again
		// (e.g. after the runtime was installed or repaired).
		err := b.envErr
		b.env, b.envErr, b.envStarted = 0, nil, false
		return nil, err
	}
	if err := b.startEnvironment(); err != nil {
		return nil, err
	}
	s, err := b.newWindow(id, opts, ex)
	if err != nil {
		return nil, err
	}

	url := opts.URL
	if url == "" {
		url = "about:blank"
	}
	s.whenReady(func() { s.navigate(url) })
	b.whenEnvironment(s.createWebView)

	s.finishSurface()
	return s, nil
}

// newWindow creates and registers the top-level window of a surface,
// without content, with the default menu, initial placement and creation
// extras applied. ex may be nil.
func (b *Backend) newWindow(id platform.SurfaceID, opts platform.SurfaceOptions, ex *platform.SurfaceExtras) (*surface, error) {
	if b.h == nil {
		return nil, fmt.Errorf("tango: NewSurface before Init")
	}
	s := &surface{b: b, id: id, opts: opts}
	s.style = wsOverlappedWindow | wsClipChildren
	if !opts.Resizable {
		s.style &^= wsThickFrame | wsMaximizeBox
	}
	exStyle, owner := s.prepareExtras(ex)
	b.creating = s
	hwnd, _, err := procCreateWindowExW.Call(exStyle,
		uintptr(unsafe.Pointer(u16(surfaceClass))), uintptr(unsafe.Pointer(u16(opts.Title))),
		uintptr(s.style), cwUseDefault, cwUseDefault, cwUseDefault, cwUseDefault,
		owner, 0, instance(), 0)
	b.creating = nil
	if hwnd == 0 {
		if s.hwnd != 0 {
			delete(b.surfaces, s.hwnd)
		}
		return nil, fmt.Errorf("tango: CreateWindowExW: %v", err)
	}
	s.hwnd = hwnd
	b.surfaces[hwnd] = s
	b.byID[id] = s

	if b.defaultMenu != nil {
		s.applyMenu(b.defaultMenu)
	}
	s.placeInitially()
	s.applyExtras(ex)
	return s, nil
}

// finishSurface marks s created (events are reported from now on) and
// shows it if asked to.
func (s *surface) finishSurface() {
	s.created = true
	s.lastW, s.lastH = s.Size()
	if s.opts.Visible {
		s.Show()
	}
}

// placeInitially sizes the window so its client area is the requested
// logical size at the window's DPI, centered on its monitor's work area.
func (s *surface) placeInitially() {
	w, h := defaultSurfaceSize(s.opts.Width, s.opts.Height)
	dpi := dpiOf(s.hwnd)
	r := rect{Right: int32(dipToPx(w, dpi)), Bottom: int32(dipToPx(h, dpi))}
	s.adjustRect(&r, dpi)
	ow, oh := int(r.Right-r.Left), int(r.Bottom-r.Top)
	x, y := 0, 0
	flags := uintptr(swpNoZOrder | swpNoActivate)
	if work, ok := nearestWorkArea(s.hwnd); ok {
		x, y = centerIn(int(work.Left), int(work.Top), int(work.Right), int(work.Bottom), ow, oh)
	} else {
		flags |= swpNoMove // leave the position Windows chose
	}
	procSetWindowPos.Call(s.hwnd, 0, uintptr(int32(x)), uintptr(int32(y)), uintptr(ow), uintptr(oh), flags)
}

// nearestWorkArea returns the work area (the monitor minus taskbars) of
// the monitor hwnd is on, or the closest one.
func nearestWorkArea(hwnd uintptr) (rect, bool) {
	monitor, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
	return info.Work, monitor != 0 && ok != 0
}

// clientRect returns hwnd's client area in pixels (left and top are 0).
func clientRect(hwnd uintptr) rect {
	var r rect
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

// whenReady runs fn now if the webview is set up, else after. A canvas
// surface has no WebView: WebView-only calls (Navigate, Eval, Reload,
// zoom, DevTools, ...) do nothing.
func (s *surface) whenReady(fn func()) {
	if s.closed || s.failed != nil || s.canvas != nil {
		return
	}
	if s.ready {
		fn()
		return
	}
	s.pending = append(s.pending, fn)
}

func (s *surface) createWebView() {
	if s.closed {
		return
	}
	if s.b.envErr != nil {
		s.fail(s.b.envErr)
		return
	}
	start := func(h uintptr) uintptr { return comCall(s.b.env, envCreateController, s.hwnd, h) }
	if hr := withHandler(s.controllerCreated, start); failed(hr) {
		s.fail(hresultError("CreateCoreWebView2Controller", hr))
	}
}

// controllerCreated completes createWebView. A window closed in the
// meantime gets its controller closed instead of set up.
func (s *surface) controllerCreated(hr, controller uintptr) {
	switch {
	case s.closed:
		if controller != 0 {
			comCall(controller, ctlClose)
		}
	case controller == 0 || failed(hr):
		s.fail(hresultError("creating the WebView2 controller", hr))
	default:
		addRef(controller) // borrowed from the callback; kept until destroyed
		s.setUp(controller)
	}
}

func (s *surface) fail(err error) {
	log.Printf("tango: surface %d: %v", s.id, err)
	s.failed = err
	s.pending = nil
	s.dropWebWaiters(err)
}

func (s *surface) setUp(controller uintptr) {
	s.controller = controller
	comCall(controller, ctlGetCoreWebView2, uintptr(unsafe.Pointer(&s.webview)))
	if s.webview == 0 {
		s.fail(fmt.Errorf("ICoreWebView2Controller::get_CoreWebView2 returned nothing"))
		return
	}
	var settings uintptr
	comCall(s.webview, wvGetSettings, uintptr(unsafe.Pointer(&settings)))
	if settings != 0 {
		comCall(settings, setPutIsScriptEnabled, 1)
		comCall(settings, setPutIsWebMessageEnabled, 1)
		comCall(settings, setPutIsStatusBarEnabled, 0)
		comCall(settings, setPutAreDevToolsEnabled, boolArg(s.opts.Debug))
		release(settings)
	}

	// The shim first, then the core's bridge: document-created scripts run
	// in the order they were added, before any page script.
	s.addDocumentScript(topFrameOnly(nativeShim))
	if s.opts.BridgeScript != "" {
		s.addDocumentScript(topFrameOnly(s.opts.BridgeScript))
	}
	s.sxInstallFileDrop()  // sysx_drop_windows.go: every dropped file's path
	s.installDragSession() // surface_ext_windows.go: drag sessions for file: drops

	// add registers an event handler; the token is written to a local
	// whose address is taken in the call (go:uintptrescapes).
	add := func(obj uintptr, index int, fn func(sender, args uintptr)) {
		var tok int64
		withHandler(fn, func(h uintptr) uintptr { return comCall(obj, index, h, uintptr(unsafe.Pointer(&tok))) })
	}
	add(s.webview, wvAddNavigationStarting, s.navigationStarting)
	add(s.webview, wvAddNavigationCompleted, s.navigationCompleted)
	add(s.webview, wvAddWebMessageReceived, s.webMessageReceived)
	add(s.webview, wvAddWebResourceRequested, s.webResourceRequested)
	comCall(s.webview, wvAddWebResourceRequestedFilter, uintptr(unsafe.Pointer(u16(assetOrigin+"/*"))), webResourceContextAll)
	add(controller, ctlAddAcceleratorKeyPressed, s.acceleratorKeyPressed)
	s.initWebExt() // WebHost events, user agent, extra headers: before the first navigation

	comCall(controller, ctlPutIsVisible, 1)
	s.resizeWebView()
	s.attachedWebReady() // canvas_attach_windows.go: stacking and background
	if active, _, _ := procGetActiveWindow.Call(); active == s.hwnd {
		s.focusWebView()
	}

	s.ready = true
	pending := s.pending
	s.pending = nil
	for _, fn := range pending {
		if s.closed {
			return
		}
		fn()
	}
}

func noopHandler(_, _ uintptr) {}

func (s *surface) addDocumentScript(js string) {
	withHandler(noopHandler, func(h uintptr) uintptr {
		return comCall(s.webview, wvAddScriptToExecuteOnDocumentCreated, uintptr(unsafe.Pointer(u16(js))), h)
	})
}

func (s *surface) navigate(url string) {
	s.ext.selfNav = url // a file: URL the app loads is not a dropped file
	comCall(s.webview, wvNavigate, uintptr(unsafe.Pointer(u16(url))))
}

func (s *surface) navigationCompleted(_, args uintptr) {
	if s.closed || s.canceledNavigation(args) {
		return
	}
	if ok, _ := comOut[int32](args, navCompletedGetIsSuccess); ok == 0 {
		s.navigationFailed(args)
		return
	}
	s.navigationSucceeded()
	s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceLoaded, URL: comString(s.webview, wvGetSource)})
}

func (s *surface) webMessageReceived(_, args uintptr) {
	if s.closed {
		return
	}
	// get_WebMessageAsJson rather than TryGetWebMessageAsString: the latter
	// is a NUL-terminated LPWSTR, which cuts a message at its first U+0000,
	// and every core internal message starts with one.
	p, hr := comOut[uintptr](args, msgGetWebMessageAsJson)
	if failed(hr) {
		return
	}
	text, ok := webMessageText(takeWstr(p))
	if !ok {
		return // not a string; the shim always posts strings
	}
	origin := originOf(comString(args, msgGetSource))
	if text == sxDropMessage || text == wxDragSessionMessage {
		// Messages the host acts on itself. The drop message carries File
		// objects whose full paths the host reads, and any page can post
		// it with files the user picked in <input type=file>: only the
		// app's own (trusted) pages may send it, checked before the paths
		// are looked at. Never passed on to the core.
		if !s.trustsOrigin(origin) {
			return
		}
		if text == wxDragSessionMessage {
			s.noteDragSession() // surface_ext_windows.go
			return
		}
		s.sxFileDropMessage(text, args) // sysx_drop_windows.go
		return
	}
	s.b.h.OnMessage(s.id, origin, text)
}

// webMessageText decodes a message's get_WebMessageAsJson form; ok is
// false when the page posted something other than a string. JSON escapes
// U+0000, so the message arrives whole.
func webMessageText(msgJSON string) (text string, ok bool) {
	if !strings.HasPrefix(msgJSON, `"`) { // null would decode to ""
		return "", false
	}
	if err := json.Unmarshal([]byte(msgJSON), &text); err != nil {
		return "", false
	}
	return text, true
}

// originTruster is implemented by the core's Host (package tango): it
// reports whether a page at origin is one of the app's own (the asset
// origin or the origin the core navigated the surface to). Duck-typed,
// outside the platform contract.
type originTruster interface {
	TrustsOrigin(id platform.SurfaceID, origin string) bool
}

// trustsOrigin asks the core whether origin is trusted; hosts that cannot
// tell (fakes) trust nothing.
func (s *surface) trustsOrigin(origin string) bool {
	t, ok := s.b.h.(originTruster)
	return ok && origin != "" && origin != "null" && t.TrustsOrigin(s.id, origin)
}

// webResourceRequested answers requests to assetOrigin synchronously with
// Host.ServeAsset, buffering the body into an IStream.
func (s *surface) webResourceRequested(_, args uintptr) {
	if s.closed || s.b.env == 0 {
		return
	}
	req, hr := comOut[uintptr](args, resReqGetRequest)
	if hr != sOK || req == 0 {
		return
	}
	defer release(req)
	uri := comString(req, reqGetURI)
	if !s.extraRequestHeaders(req, uri) {
		return // not an asset request: only the extra headers were set
	}
	method := comString(req, reqGetMethod)
	var body []byte
	if content, hr := comOut[uintptr](req, reqGetContent); hr == sOK && content != 0 {
		body = readStream(content)
		release(content)
	}
	rec := newAssetResponse()
	r, err := newAssetRequest(method, uri, requestHeaders(req), body)
	if err != nil {
		http.Error(rec, err.Error(), http.StatusBadRequest)
	} else {
		s.b.h.ServeAsset(s.id, rec, r)
	}
	status, reason, headers, data := rec.finish()
	stream := memStream(data)
	var resp uintptr
	hr = comCall(s.b.env, envCreateWebResourceResponse, stream, uintptr(status),
		uintptr(unsafe.Pointer(u16(reason))), uintptr(unsafe.Pointer(u16(headers))), uintptr(unsafe.Pointer(&resp)))
	release(stream)
	if failed(hr) || resp == 0 {
		return // WebView2 then performs the request itself
	}
	comCall(args, resReqPutResponse, resp)
	release(resp)
}

// requestHeaders copies a WebView2 request's headers, walking the
// ICoreWebView2HttpHeadersCollectionIterator (capped at 1000 entries).
func requestHeaders(req uintptr) http.Header {
	h := http.Header{}
	headers, hr := comOut[uintptr](req, reqGetHeaders)
	if hr != sOK || headers == 0 {
		return h
	}
	defer release(headers)
	it, hr := comOut[uintptr](headers, headersGetIterator)
	if hr != sOK || it == 0 {
		return h
	}
	defer release(it)
	for range 1000 {
		if current, _ := comOut[int32](it, iterGetHasCurrentHeader); current == 0 {
			break
		}
		var name, value uintptr // CoTaskMemAlloc'd strings, freed by takeWstr
		comCall(it, iterGetCurrentHeader, uintptr(unsafe.Pointer(&name)), uintptr(unsafe.Pointer(&value)))
		h.Add(takeWstr(name), takeWstr(value))
		if more, _ := comOut[int32](it, iterMoveNext); more == 0 {
			break
		}
	}
	return h
}

// acceleratorKeyPressed sees keys while the WebView has focus, which
// never reach the host's TranslateAccelerator. Matching menu accelerators
// are marked handled and run from a posted WM_COMMAND, outside the WebView2
// callback.
func (s *surface) acceleratorKeyPressed(_, args uintptr) {
	if s.spec == nil || len(s.spec.plan.Accels) == 0 {
		return
	}
	switch kind, _ := comOut[int32](args, accelGetKeyEventKind); kind {
	case keyEventKeyDown, keyEventSystemKeyDown:
	default:
		return // key up, or a character message
	}
	vk, _ := comOut[uint32](args, accelGetVirtualKey)
	cmd, ok := matchAccel(s.spec.plan.Accels, uint16(vk), currentModifiers())
	if !ok {
		return
	}
	comCall(args, accelPutHandled, 1)
	postMessage(s.hwnd, wmCommand, uintptr(cmd)|1<<16, 0)
}

func (s *surface) resizeWebView() {
	if s.controller == 0 {
		return
	}
	putBounds(s.controller, clientRect(s.hwnd))
}

func (s *surface) focusWebView() {
	if s.controller != 0 {
		comCall(s.controller, ctlMoveFocus, moveFocusProgrammatic)
	}
}

// focusContent gives keyboard focus to the content: the canvas or the
// WebView.
func (s *surface) focusContent() {
	if s.focusAttached() { // an attached canvas had focus (canvas_attach_windows.go)
		return
	}
	if s.canvas != nil {
		s.canvas.focus()
		return
	}
	s.focusWebView()
}

func (s *surface) event(ev platform.SurfaceEvent) {
	if s.created && !s.closed {
		s.b.h.OnSurfaceEvent(s.id, ev)
	}
}

// message handles the surface window's messages.
func (s *surface) message(m uint32, wp, lp uintptr) (uintptr, bool) {
	if r, ok := s.extMessage(m, wp, lp); ok {
		return r, true
	}
	switch m {
	case wmSize:
		s.resizeWebView()
		if s.canvas != nil && wp != sizeMinimized {
			s.canvas.fitParent()
		}
		if wp != sizeMinimized {
			s.layoutAttached() // canvas_attach_windows.go
		}
		if wp != sizeMinimized && s.created {
			w, h := s.Size()
			if w != s.lastW || h != s.lastH {
				s.lastW, s.lastH = w, h
				s.event(platform.SurfaceEvent{Kind: platform.SurfaceResized, Width: w, Height: h})
			}
		}
		s.sized(wp)
		return 0, true
	case wmMove, wmMoving:
		if s.controller != 0 {
			comCall(s.controller, ctlNotifyParentWindowPositionChanged)
		}
		if m == wmMove {
			s.moved()
		}
		return 0, false
	case wmDpiChanged:
		r := (*rect)(native(lp))
		procSetWindowPos.Call(s.hwnd, 0, uintptr(r.Left), uintptr(r.Top),
			uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), swpNoZOrder|swpNoActivate)
		return 0, true
	case wmGetMinMaxInfo:
		if s.opts.MinWidth > 0 || s.opts.MinHeight > 0 {
			dpi := dpiOf(s.hwnd)
			r := rect{Right: int32(dipToPx(s.opts.MinWidth, dpi)), Bottom: int32(dipToPx(s.opts.MinHeight, dpi))}
			s.adjustRect(&r, dpi)
			mmi := (*minMaxInfo)(native(lp))
			if s.opts.MinWidth > 0 {
				mmi.MinTrackSize.X = r.Right - r.Left
			}
			if s.opts.MinHeight > 0 {
				mmi.MinTrackSize.Y = r.Bottom - r.Top
			}
		}
		s.maxTrack(lp)
		return 0, true
	case wmActivate:
		kind := platform.SurfaceFocused
		if loword(wp) == waInactive {
			kind = platform.SurfaceBlurred
		}
		s.event(platform.SurfaceEvent{Kind: kind})
		return 0, false
	case wmSetFocus:
		s.focusContent()
		return 0, true
	case wmActivateApp:
		s.b.setActive(wp != 0)
		return 0, true
	case wmClose:
		if s.b.h.ShouldClose(s.id) {
			s.Close()
		}
		return 0, true
	case wmCommand:
		if lp == 0 { // from a menu (0) or an accelerator (1)
			s.runCommand(loword(wp))
			return 0, true
		}
	case wmDestroy:
		s.destroyed()
		return 0, true
	}
	return 0, false
}

// destroyed releases everything once the window is gone and reports
// SurfaceClosed.
func (s *surface) destroyed() {
	if s.closed {
		return
	}
	s.closed = true
	s.pending = nil
	s.closeWebExt()
	if s.canvas != nil {
		s.canvas.destroy() // no sink callbacks after SurfaceClosed
	}
	s.destroyAttached() // canvas_attach_windows.go: likewise for attached canvases
	if s.controller != 0 {
		comCall(s.controller, ctlClose)
		release(s.webview)
		release(s.controller)
		s.controller, s.webview = 0, 0
	}
	if s.accel != 0 {
		procDestroyAcceleratorTable.Call(s.accel)
		s.accel = 0
	}
	releaseMenuBitmaps(s.menu) // the menu itself is freed with the window
	s.menu = 0
	delete(s.b.surfaces, s.hwnd)
	delete(s.b.byID, s.id)
	if s.created {
		s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceClosed})
	}
}

// platform.Surface

func (s *surface) Show() {
	if s.closed {
		return
	}
	s.beforeShow()
	if !s.shown {
		s.shown = true
		procShowWindow.Call(s.hwnd, uintptr(firstShowCmd(s.ext.pendingState, s.noActivate())))
		procUpdateWindow.Call(s.hwnd)
		if !s.noActivate() {
			procSetForegroundWindow.Call(s.hwnd)
		}
		s.afterFirstShow()
		return
	}
	procShowWindow.Call(s.hwnd, s.showCmd())
}

func (s *surface) Hide() {
	if !s.closed {
		s.releaseOwner()
		procShowWindow.Call(s.hwnd, swHide)
	}
}

func (s *surface) Close() {
	if !s.closed {
		s.releaseOwner()
		procDestroyWindow.Call(s.hwnd) // WM_DESTROY → destroyed()
	}
}

func (s *surface) SetTitle(title string) {
	if !s.closed {
		procSetWindowTextW.Call(s.hwnd, uintptr(unsafe.Pointer(u16(title))))
	}
}

func (s *surface) SetSize(width, height int) {
	if s.closed {
		return
	}
	dpi := dpiOf(s.hwnd)
	r := rect{Right: int32(dipToPx(width, dpi)), Bottom: int32(dipToPx(height, dpi))}
	s.adjustRect(&r, dpi)
	procSetWindowPos.Call(s.hwnd, 0, 0, 0, uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
		swpNoMove|swpNoZOrder|swpNoActivate)
}

func (s *surface) Size() (int, int) {
	if s.closed {
		return s.lastW, s.lastH
	}
	r := clientRect(s.hwnd)
	dpi := dpiOf(s.hwnd)
	return pxToDIP(int(r.Right-r.Left), dpi), pxToDIP(int(r.Bottom-r.Top), dpi)
}

func (s *surface) Focus() {
	if s.closed {
		return
	}
	if r, _, _ := procIsIconic.Call(s.hwnd); r != 0 {
		procShowWindow.Call(s.hwnd, swRestore)
	}
	procSetForegroundWindow.Call(s.hwnd)
	s.focusContent()
}

func (s *surface) Navigate(url string) {
	s.whenReady(func() { s.navigate(url) })
}

func (s *surface) Eval(js string) {
	s.whenReady(func() {
		withHandler(noopHandler, func(h uintptr) uintptr {
			return comCall(s.webview, wvExecuteScript, uintptr(unsafe.Pointer(u16(js))), h)
		})
	})
}

func (s *surface) OpenDevTools() {
	if !s.opts.Debug {
		return
	}
	s.whenReady(func() { comCall(s.webview, wvOpenDevToolsWindow) })
}

func (s *surface) Native() uintptr { return s.hwnd }

// devtools calls a DevTools protocol method, fire-and-forget.
func (s *surface) devtools(method, params string) {
	s.whenReady(func() {
		withHandler(noopHandler, func(h uintptr) uintptr {
			return comCall(s.webview, wvCallDevToolsProtocolMethod,
				uintptr(unsafe.Pointer(u16(method))), uintptr(unsafe.Pointer(u16(params))), h)
		})
	})
}
