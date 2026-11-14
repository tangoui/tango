//go:build darwin && !ios

package darwin

import (
	"strconv"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

// nativeShim is the backend's half of the JS bridge, injected at document
// start before opts.BridgeScript.
const nativeShim = `window.__tangoNative={post:s=>webkit.messageHandlers.tango.postMessage(String(s))};`

// Values from NSWindow.h, NSView.h, NSGraphics.h and WKUserScript.h.
const (
	nsWindowStyleMaskTitled         = 1 << 0
	nsWindowStyleMaskClosable       = 1 << 1
	nsWindowStyleMaskMiniaturizable = 1 << 2
	nsWindowStyleMaskResizable      = 1 << 3

	nsBackingStoreBuffered = 2 // NSBackingStoreBuffered, the only non-deprecated kind

	nsViewWidthSizable       = 1 << 1
	nsViewHeightSizable      = 1 << 4
	nsViewWidthHeightSizable = nsViewWidthSizable | nsViewHeightSizable

	wkUserScriptInjectionTimeAtDocumentStart = 0
)

const (
	// messageHandlerName is the window.webkit.messageHandlers entry the shim
	// posts to.
	messageHandlerName = "tango"

	defaultWidth, defaultHeight = 1024, 768
)

type surface struct {
	b        *backend
	id       platform.SurfaceID
	win      id
	web      id // WKWebView; 0 for a canvas surface
	ucc      id
	view     id         // content view: web, or the canvas view
	canvas   *canvas    // canvas surfaces only (canvas.go)
	overlays []*overlay // canvases attached to a WebView surface (canvas_attach.go)
	delegate id
	closed   bool

	ext surfaceExt // round-2 window state; surface_ext.go

	laneCaps []string // fast lanes of a WebView surface, fixed at creation (lanes.go)
}

func (b *backend) NewSurface(sid platform.SurfaceID, opts platform.SurfaceOptions) (platform.Surface, error) {
	return b.newSurface(sid, opts, nil)
}

// newSurface creates a WebView surface; ex is nil for NewSurface (see
// NewSurfaceEx). Canvas surfaces (canvas.go) share newWindow and
// finishSurface and put a canvas view where the WebView would go.
func (b *backend) newSurface(sid platform.SurfaceID, opts platform.SurfaceOptions, ex *platform.SurfaceExtras) (platform.Surface, error) {
	s, rect := b.newWindow(sid, opts, ex)
	s.laneCaps = envLaneCaps() // read once, before the core can ask
	withPool(func() {
		cfg := s.webConfiguration(opts)
		s.web = msgInitRectID(send(class("TangoWebView"), "alloc"), sel("initWithFrame:configuration:"), rect, cfg)
		release(cfg) // the web view keeps its own copy
		send(s.web, "setNavigationDelegate:", uintptr(s.delegate))
		s.initWebExt() // round 4: webview_ext.go

		// WKWebView.inspectable (macOS 13.3+) gates Safari's Web Inspector.
		if respondsTo(s.web, "setInspectable:") {
			send(s.web, "setInspectable:", boolArg(opts.Debug))
		}
		s.setContent(s.web, rect)
	})
	b.byWebView[s.web] = s
	b.finishSurface(s, opts, ex)

	start := opts.URL
	if start == "" {
		start = "about:blank"
	}
	s.Navigate(start)
	if opts.Visible {
		s.Show()
	}
	return s, nil
}

// webConfiguration prepares an owned WKWebViewConfiguration: the asset
// scheme handler, developer extras when debugging, and a user content
// controller (kept in s.ucc) carrying the message handler and the bridge
// scripts.
func (s *surface) webConfiguration(opts platform.SurfaceOptions) id {
	cfg := alloc("WKWebViewConfiguration")
	send(cfg, "setURLSchemeHandler:forURLScheme:", uintptr(schemeHandler()), uintptr(nsString(assetScheme)))
	// developerExtrasEnabled is not public API on WKPreferences, but it is
	// key-value coding compliant and is what enables "Inspect Element".
	send(send(cfg, "preferences"), "setValue:forKey:",
		uintptr(nsNumberBool(opts.Debug)), uintptr(nsString("developerExtrasEnabled")))

	s.ucc = alloc("WKUserContentController")
	send(s.ucc, "addScriptMessageHandler:name:", uintptr(s.delegate), uintptr(nsString(messageHandlerName)))
	for _, src := range []string{nativeShim, opts.BridgeScript} {
		if src != "" {
			addUserScript(s.ucc, src)
		}
	}
	send(cfg, "setUserContentController:", uintptr(s.ucc))
	return cfg
}

// newWindow creates the NSWindow and its delegate for a surface, without
// content. rect is the initial content rectangle.
func (b *backend) newWindow(sid platform.SurfaceID, opts platform.SurfaceOptions, ex *platform.SurfaceExtras) (*surface, nsRect) {
	s := &surface{b: b, id: sid}
	rect := nsRect{Size: nsSize{
		Width:  float64(positiveOr(opts.Width, defaultWidth)),
		Height: float64(positiveOr(opts.Height, defaultHeight)),
	}}
	withPool(func() {
		raw := send(class(windowClass(ex)), "alloc")
		s.win = msgInitWindow(raw, sel("initWithContentRect:styleMask:backing:defer:"),
			rect, windowStyle(opts.Resizable, ex), nsBackingStoreBuffered, false)
		// Ownership stays with the surface; destroy releases it.
		send(s.win, "setReleasedWhenClosed:", boolArg(false))
		send(s.win, "setTitle:", uintptr(nsString(opts.Title)))
		if opts.MinWidth > 0 || opts.MinHeight > 0 {
			floor := nsSize{Width: float64(opts.MinWidth), Height: float64(opts.MinHeight)}
			msgSetSize(s.win, sel("setContentMinSize:"), floor)
		}
		send(s.win, "center")

		s.delegate = alloc("TangoSurfaceDelegate")
		send(s.win, "setDelegate:", uintptr(s.delegate))
	})
	return s, rect
}

// windowStyle is the NSWindowStyleMask for a new surface window: a normal
// titled window, optionally resizable, plus whatever the window kind adds.
func windowStyle(resizable bool, ex *platform.SurfaceExtras) uint {
	mask := uint(nsWindowStyleMaskTitled | nsWindowStyleMaskClosable | nsWindowStyleMaskMiniaturizable)
	if resizable {
		mask |= nsWindowStyleMaskResizable
	}
	return mask | kindStyle(ex)
}

func positiveOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}

// setContent puts view (a WebView or canvas view) in a plain container
// that becomes the window's content view, and makes it first responder.
// WebKit docks the inspector next to the web view in its superview, which
// must not be the window's frame view; the container also hosts the
// NSVisualEffectView of a Material (surface_ext.go). Inside withPool.
func (s *surface) setContent(view id, rect nsRect) {
	s.view = view
	container := msgInitRect(send(class("NSView"), "alloc"), sel("initWithFrame:"), rect)
	send(view, "setAutoresizingMask:", nsViewWidthHeightSizable)
	send(container, "addSubview:", uintptr(view))
	send(s.win, "setContentView:", uintptr(container))
	release(container)
	send(s.win, "makeFirstResponder:", uintptr(view))
}

// finishSurface registers s and applies the creation extras.
func (b *backend) finishSurface(s *surface, opts platform.SurfaceOptions, ex *platform.SurfaceExtras) {
	b.surfaces[s.id] = s
	b.byDelegate[s.delegate] = s
	s.initExt(opts, ex)
}

// addUserScript installs src to run at document start in the main frame
// only; the content controller retains the script.
func addUserScript(ucc id, src string) {
	us := send(class("WKUserScript"), "alloc")
	us = send(us, "initWithSource:injectionTime:forMainFrameOnly:",
		uintptr(nsString(src)), wkUserScriptInjectionTimeAtDocumentStart, boolArg(true))
	send(ucc, "addUserScript:", uintptr(us))
	release(us)
}

func nsNumberBool(v bool) id { return send(class("NSNumber"), "numberWithBool:", boolArg(v)) }

func (s *surface) Show() {
	if s.closed || s.showSheet() {
		return
	}
	withPool(func() {
		send(s.win, "makeKeyAndOrderFront:", 0)
	})
	s.afterShow()
}

func (s *surface) Hide() {
	if s.closed {
		return
	}
	send(s.win, "orderOut:", 0)
}

func (s *surface) Close() {
	if s.closed {
		return
	}
	withPool(func() {
		s.endSheet()
		// windowWillClose: does the cleanup and emits SurfaceClosed.
		send(s.win, "close")
	})
}

func (s *surface) SetTitle(title string) {
	if s.closed {
		return
	}
	withPool(func() { send(s.win, "setTitle:", uintptr(nsString(title))) })
}

func (s *surface) SetSize(width, height int) {
	if s.closed {
		return
	}
	msgSetSize(s.win, sel("setContentSize:"), nsSize{float64(width), float64(height)})
}

func (s *surface) Size() (int, int) {
	if s.closed {
		return 0, 0
	}
	r := s.contentSize()
	return int(r.Width + 0.5), int(r.Height + 0.5)
}

func (s *surface) contentSize() nsSize {
	frame := msgRect(s.win, sel("frame"))
	return msgContentRectForRect(s.win, sel("contentRectForFrameRect:"), frame).Size
}

var _ platform.SurfaceStatus = (*surface)(nil)

// IsVisible: on screen or in the Dock (minimized), and the app not hidden.
func (s *surface) IsVisible() bool {
	if s.closed || sendBool(s.b.app, "isHidden") {
		return false
	}
	return sendBool(s.win, "isVisible") || sendBool(s.win, "isMiniaturized")
}

// IsFocused: the key window of the active app.
func (s *surface) IsFocused() bool {
	return !s.closed && sendBool(s.b.app, "isActive") && sendBool(s.win, "isKeyWindow")
}

func (s *surface) Focus() {
	if s.closed {
		return
	}
	s.b.bringToFront()
	send(s.win, "makeKeyAndOrderFront:", 0)
}

func (s *surface) Navigate(url string) {
	if s.closed || s.web == 0 {
		return
	}
	withPool(func() {
		u := nsURL(url)
		if u == 0 {
			u = nsURL("about:blank")
		}
		send(s.web, "loadRequest:", uintptr(send(class("NSURLRequest"), "requestWithURL:", uintptr(u))))
	})
}

func (s *surface) Eval(js string) {
	if s.closed || s.web == 0 {
		return
	}
	withPool(func() {
		send(s.web, "evaluateJavaScript:completionHandler:", uintptr(nsString(js)), 0)
	})
}

// OpenDevTools uses WebKit's private _inspector (no public API exists); it
// is a no-op unless the surface was created with Debug.
func (s *surface) OpenDevTools() {
	if s.closed || !respondsTo(s.web, "_inspector") {
		return
	}
	if insp := send(s.web, "_inspector"); insp != 0 && respondsTo(insp, "show") {
		send(insp, "show")
	}
}

func (s *surface) Native() uintptr { return uintptr(s.win) }

// destroy tears the surface down from windowWillClose: (main thread). The
// Objective-C objects are only autoreleased: AppKit is still executing
// -close on the window, so they must outlive the current event.
func (s *surface) destroy() {
	if s.closed {
		return
	}
	s.closed = true
	if sheet := send(s.win, "attachedSheet"); sheet != 0 {
		send(s.win, "endSheet:returnCode:", uintptr(sheet), nsModalResponseCancel)
	}
	if s.web != 0 {
		s.detachWebView()
	}
	if s.canvas != nil {
		s.canvas.destroy()
	}
	s.destroyOverlays()
	send(s.win, "setDelegate:", 0)

	b := s.b
	delete(b.surfaces, s.id)
	delete(b.byDelegate, s.delegate)
	for _, obj := range []id{s.web, s.ucc, s.delegate, s.win} {
		autorelease(obj)
	}
	b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceClosed})
}

// detachWebView breaks the WebView's references back to the delegate (the
// script message handler retains it) and stops any pending load.
func (s *surface) detachWebView() {
	send(s.ucc, "removeScriptMessageHandlerForName:", uintptr(nsString(messageHandlerName)))
	send(s.ucc, "removeAllUserScripts")
	send(s.web, "stopLoading")
	send(s.web, "setNavigationDelegate:", 0)
	s.destroyWebExt() // webview_ext.go
	delete(s.b.byWebView, s.web)
}

func (s *surface) url() string {
	if s.web == 0 {
		return ""
	}
	var u string
	withPool(func() { u = goString(send(send(s.web, "URL"), "absoluteString")) })
	return u
}

func registerWindowClasses() {
	// Borderless and utility windows refuse key / main status by default;
	// surfaces always accept it so they can take keyboard input.
	yes := func(_ id, _ objc.SEL) bool { return true }
	classDef("TangoWindow", "NSWindow", nil, []objc.MethodDef{
		method("canBecomeKeyWindow", yes),
		method("canBecomeMainWindow", yes),
	})
	registerExtClasses()

	// of maps a delegate instance back to its live surface.
	of := func(self id) *surface {
		b := theBackend
		if b == nil {
			return nil
		}
		if s := b.byDelegate[self]; s != nil && !s.closed {
			return s
		}
		return nil
	}
	// onSurface adapts a one-argument delegate callback (a notification, a
	// navigation, a message) to a func of the live surface.
	onSurface := func(selector string, fn func(s *surface, arg id)) objc.MethodDef {
		return method(selector, func(self id, _ objc.SEL, _, arg id) {
			if s := of(self); s != nil {
				fn(s, arg)
			}
		})
	}
	// onNotify is onSurface for NSWindowDelegate notifications, which take
	// only the NSNotification.
	onNotify := func(selector string, fn func(s *surface)) objc.MethodDef {
		return method(selector, func(self id, _ objc.SEL, _ id) {
			if s := of(self); s != nil {
				fn(s)
			}
		})
	}
	focusChanged := func(kind platform.SurfaceEventKind) func(*surface) {
		return func(s *surface) {
			s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: kind})
			s.canvas.focusMaybeChanged()
			s.overlaysFocusMaybeChanged()
		}
	}

	own := []objc.MethodDef{
		// NSWindowDelegate
		method("windowShouldClose:", func(self id, _ objc.SEL, _ id) bool {
			s := of(self)
			return s == nil || s.b.h.ShouldClose(s.id)
		}),
		onNotify("windowWillClose:", (*surface).destroy),
		onNotify("windowDidResize:", func(s *surface) {
			s.layoutOverlays() // canvas_attach.go
			w, h := s.Size()
			s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceResized, Width: w, Height: h})
			s.afterResize() // surface_ext.go
		}),
		onNotify("windowDidBecomeKey:", focusChanged(platform.SurfaceFocused)),
		onNotify("windowDidResignKey:", focusChanged(platform.SurfaceBlurred)),
		// WKNavigationDelegate
		onSurface("webView:didFinishNavigation:", func(s *surface, _ id) {
			s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceLoaded, URL: s.url()})
		}),
		// WKScriptMessageHandler
		onSurface("userContentController:didReceiveScriptMessage:", (*surface).receiveScriptMessage),
	}
	protocols := []string{"NSWindowDelegate", "WKNavigationDelegate", "WKScriptMessageHandler", "WKUIDelegate", "WKDownloadDelegate"}
	all := append(own, extDelegateMethods(of)...)
	all = append(all, webDelegateMethods(of)...)
	classDef("TangoSurfaceDelegate", "NSObject", protocols, all)
}

// receiveScriptMessage forwards a WKScriptMessage to the host. The message
// handler is visible to every frame, but only the main frame (the one the
// shim was injected into) may talk to Go, and only string bodies are valid.
func (s *surface) receiveScriptMessage(msg id) {
	frame := send(msg, "frameInfo")
	if frame == 0 || !sendBool(frame, "isMainFrame") {
		return
	}
	payload := send(msg, "body")
	if payload == 0 || !sendBool(payload, "isKindOfClass:", uintptr(class("NSString"))) {
		return
	}
	var origin string
	if so := send(frame, "securityOrigin"); so != 0 {
		origin = formatOrigin(goString(send(so, "protocol")), goString(send(so, "host")), sendInt(so, "port"))
	}
	s.b.h.OnMessage(s.id, origin, goString(payload))
}

// formatOrigin renders a WKSecurityOrigin as scheme://host[:port].
func formatOrigin(scheme, host string, port int) string {
	o := scheme + "://" + host
	if port != 0 {
		o += ":" + strconv.Itoa(port)
	}
	return o
}
