//go:build linux && !android

package linux

import (
	"errors"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// WebKit / GTK enum values.
const (
	gtkWindowToplevel         = 0
	gtkOrientationVertical    = 1
	webkitLoadCommitted       = 2 // WebKitLoadEvent (STARTED: surface_ext_linux.go)
	webkitLoadFinished        = 3
	webkitInjectTopFrame      = 1 // WebKitUserContentInjectedFrames
	webkitInjectDocumentStart = 0 // WebKitUserScriptInjectionTime

	defaultWidth, defaultHeight = 1024, 768
)

type surface struct {
	b  *backend
	id platform.SurfaceID

	win, box, web, ucm ptr     // web, ucm: 0 for a canvas surface
	content            ptr     // the content widget: web, or the canvas widget
	canvas             *canvas // canvas surfaces only (canvas_linux.go)
	menubar, accel     ptr
	msgHandler         uint // script-message-received connection on ucm

	// Script message authentication (msggate.go): the token gate, the
	// installed document-start script (referenced; 0 for a canvas surface)
	// and the BridgeScript it carries.
	gate   msgGate
	script ptr
	bridge string

	resizable  bool
	minW, minH int
	debug      bool
	closed     bool
	lastW      int32
	lastH      int32
	ext        surfaceExt // window extensions (surface_ext_linux.go)
	wext       *webExt    // WebView extensions (webview_ext_linux.go); nil for canvas surfaces
}

func (b *backend) NewSurface(id platform.SurfaceID, o platform.SurfaceOptions) (platform.Surface, error) {
	return b.newSurface(id, o, nil)
}

// newSurface backs NewSurface and NewSurfaceEx (ex nil for the former).
// Canvas surfaces (canvas_linux.go) share newWindow and finishSurface and
// put a canvas widget where the web view would go.
func (b *backend) newSurface(id platform.SurfaceID, o platform.SurfaceOptions, ex *platform.SurfaceExtras) (platform.Surface, error) {
	if !b.loaded.Load() {
		return nil, errors.New("tango/linux: NewSurface before Init")
	}
	s, err := b.newWindow(id, o)
	if err != nil {
		return nil, err
	}

	if err := s.createWeb(o); err != nil {
		gtkWidgetDestroy(s.win)
		return nil, err
	}
	s.packWeb() // canvas_attach_linux.go: box -> GtkOverlay -> web view
	s.wireWeb()
	s.content = s.web
	b.byWeb[s.web] = s
	b.finishSurface(s, ex)

	url := o.URL
	if url == "" {
		url = "about:blank"
	}
	webkitWebViewLoadURI(s.web, url)
	if o.Visible {
		gtkWindowPresent(s.win)
	}
	return s, nil
}

// createWeb creates the web view of s (not packed anywhere yet) and its
// user content manager, which carries the shim + bridge and the "tango"
// script message handler. Shared by WebView surfaces and embedded web
// views (webembed_linux.go). On failure nothing is left behind.
func (s *surface) createWeb(o platform.SurfaceOptions) error {
	s.ucm = webkitUserContentManagerNew()
	s.bridge = o.BridgeScript
	s.installScript(s.gate.init())
	webkitUserContentManagerRegisterHandler(s.ucm, messageHandlerName)
	s.msgHandler = connect(s.ucm, "script-message-received::"+messageHandlerName, 1, s.onScriptMessage)

	s.web = webkitWebViewNewWithUserContentManager(s.ucm)
	if s.web == 0 {
		gSignalHandlerDisconnect(s.ucm, s.msgHandler)
		s.msgHandler = 0
		gObjectUnref(s.ucm)
		s.ucm = 0
		webkitUserScriptUnref(s.script)
		s.script = 0
		return errors.New("tango/linux: webkit_web_view_new failed")
	}
	if o.Debug {
		webkitSettingsSetEnableDeveloperExtras(webkitWebViewGetSettings(s.web), true)
	}
	if webkitSettingsSetEnablePageCache != nil {
		// A document restored from the back/forward cache runs no
		// document-start script, so it would keep a token the gate has
		// already revoked (msggate.go): back and forward reload instead.
		webkitSettingsSetEnablePageCache(webkitWebViewGetSettings(s.web), false)
	}
	return nil
}

// wireWeb connects the web view's load tracking and the WebView
// extensions, once it is packed and before its first load.
func (s *surface) wireWeb() {
	connect(s.web, "load-changed", 1, s.onLoadChanged)
	s.initWebExt() // webview_ext_linux.go; before initExt and the first load
}

// releaseWeb drops what createWeb set up besides the widget itself
// (destroyed with its container), when the surface goes away.
func (s *surface) releaseWeb() {
	if s.msgHandler != 0 {
		gSignalHandlerDisconnect(s.ucm, s.msgHandler)
		s.msgHandler = 0
	}
	if s.ucm != 0 {
		webkitUserContentManagerUnregisterHandler(s.ucm, messageHandlerName)
		gObjectUnref(s.ucm) // the web view holds its own reference
		s.ucm = 0
	}
	if s.script != 0 {
		webkitUserScriptUnref(s.script)
		s.script = 0
	}
	if s.web != 0 {
		delete(s.b.byWeb, s.web)
	}
}

// newWindow creates the GtkWindow of a surface and its vertical content
// box (menu bar on top, then the content widget), without content.
func (b *backend) newWindow(id platform.SurfaceID, o platform.SurfaceOptions) (*surface, error) {
	s := &surface{b: b, id: id, resizable: o.Resizable, minW: o.MinWidth, minH: o.MinHeight, debug: o.Debug}
	w, h := o.Width, o.Height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	s.win = gtkWindowNew(gtkWindowToplevel)
	if s.win == 0 {
		return nil, errors.New("tango/linux: gtk_window_new failed")
	}
	gtkWindowSetTitle(s.win, o.Title)
	s.box = gtkBoxNew(gtkOrientationVertical, 0)
	gtkContainerAdd(s.win, s.box)
	gtkWindowSetResizable(s.win, o.Resizable)
	s.applySize(w, h)
	return s, nil
}

// finishSurface wires the window once its content widget (s.content) is
// packed: accelerators, window signals, registration, the app menu, and
// the creation extras (ex nil for plain NewSurface). The window is not
// shown yet.
func (b *backend) finishSurface(s *surface, ex *platform.SurfaceExtras) {
	s.accel = gtkAccelGroupNew()
	gtkWindowAddAccelGroup(s.win, s.accel)

	connect(s.win, "delete-event", 1, s.onDeleteEvent)
	connect(s.win, "destroy", 0, s.onDestroy)
	connect(s.win, "focus-in-event", 1, s.onFocusIn)
	connect(s.win, "focus-out-event", 1, s.onFocusOut)
	connect(s.win, "configure-event", 1, s.onConfigure)

	b.surfaces[s.id] = s
	if b.appMenu != nil {
		s.installMenu(b.appMenu)
	}
	gtkWidgetShowAll(s.box)
	s.initExt(ex)
}

// applySize sets the window's content size. GTK 3 ignores the default size
// of a non-resizable window and sizes it to its request, so that case uses
// a size request on the content box instead.
func (s *surface) applySize(w, h int) {
	if s.resizable {
		minW, minH := int32(-1), int32(-1)
		if s.minW > 0 {
			minW = int32(s.minW)
		}
		if s.minH > 0 {
			minH = int32(s.minH)
		}
		gtkWidgetSetSizeRequest(s.box, minW, minH)
	} else {
		gtkWidgetSetSizeRequest(s.box, int32(w), int32(h))
	}
	gtkWindowSetDefaultSize(s.win, int32(w), int32(h))
	gtkWindowResize(s.win, int32(w), int32(h))
}

// Signal handlers (main thread). ------------------------------------------

func (s *surface) onDeleteEvent(_, _ ptr) ptr {
	if s.closed {
		return 0
	}
	// TRUE stops GTK's default handler, which destroys the window.
	return gbool(!s.b.host.ShouldClose(s.id))
}

func (s *surface) onDestroy(_, _ ptr) ptr {
	if s.closed {
		return 0
	}
	s.closed = true
	b := s.b
	s.closeEmbedded() // webembed_linux.go: embedded web views first, each reporting SurfaceClosed
	if s.canvas != nil {
		s.canvas.destroy() // before SurfaceClosed: the sink sees nothing more
	}
	s.destroyAttached() // canvas_attach_linux.go: likewise for attached canvases
	s.releaseWeb()
	gObjectUnref(s.accel)
	delete(b.surfaces, s.id)
	b.focusOut(s.id)
	b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceClosed})
	return 0
}

func (s *surface) onFocusIn(_, _ ptr) ptr {
	if !s.closed {
		s.b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceFocused})
		s.b.focusIn(s.id)
	}
	return 0 // propagate
}

func (s *surface) onFocusOut(_, _ ptr) ptr {
	if !s.closed {
		s.b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceBlurred})
		s.b.focusOut(s.id)
	}
	return 0
}

// onConfigure reports size changes. configure-event also fires on moves;
// gtk_window_get_size (not the event's GdkWindow size, which includes
// client-side decoration shadows) is compared with the last size reported.
func (s *surface) onConfigure(_, _ ptr) ptr {
	if s.closed {
		return 0
	}
	var w, h int32
	gtkWindowGetSize(s.win, &w, &h)
	if w != s.lastW || h != s.lastH {
		s.lastW, s.lastH = w, h
		s.b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceResized, Width: int(w), Height: int(h)})
	}
	return 0
}

func (s *surface) onLoadChanged(_, event ptr) ptr {
	if s.closed || s.web == 0 {
		return 0
	}
	switch int32(event) {
	case webkitLoadStarted:
		// Revoke the current document's token before anything else: from
		// here to the commit the old document is still running while the
		// view's URI may already be the new one.
		s.installScript(s.gate.started())
	case webkitLoadCommitted:
		s.gate.committed(goStr(webkitWebViewGetURI(s.web)))
	case webkitLoadFinished:
		s.gate.finished()
		s.b.host.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceLoaded, URL: goStr(webkitWebViewGetURI(s.web))})
	}
	return 0
}

// installScript replaces the document-start user script with one carrying
// token. The next document committed in the top frame gets it; documents
// already loaded keep the script they ran.
func (s *surface) installScript(token string) {
	if s.ucm == 0 {
		return
	}
	if s.script != 0 {
		if webkitUserContentManagerRemoveScript != nil {
			webkitUserContentManagerRemoveScript(s.ucm, s.script)
		} else {
			// Before 2.32. This user content manager holds no other
			// script, so removing all of them removes just this one.
			webkitUserContentManagerRemoveAllScripts(s.ucm)
		}
		webkitUserScriptUnref(s.script)
	}
	src := documentStartScript(token, s.bridge)
	s.script = webkitUserScriptNew(src, webkitInjectTopFrame, webkitInjectDocumentStart, 0, 0)
	webkitUserContentManagerAddScript(s.ucm, s.script)
}

// onScriptMessage handles script-message-received::tango. In the 4.0/4.1
// API the argument is a WebKitJavascriptResult (6.0 passes a JSCValue
// directly, but this backend never loads 6.0); both the result and its
// JSCValue are owned by the signal emission.
func (s *surface) onScriptMessage(_, result ptr) ptr {
	if s.closed || s.web == 0 || result == 0 {
		return 0
	}
	v := webkitJavascriptResultGetJSValue(result)
	if v == 0 {
		return 0
	}
	// WebKitGTK 4.0/4.1 does not tell which frame or document posted, and
	// every frame can reach the handler: only messages carrying the
	// committed top-frame document's token are accepted, with the origin
	// recorded at its commit (msggate.go).
	origin, msg, ok := s.gate.check(jscString(v))
	if !ok {
		return 0
	}
	s.b.host.OnMessage(s.id, origin, msg)
	return 0
}

// jscString converts a JSCValue to a Go string. to_string_as_bytes keeps
// embedded NULs; to_string is the fallback.
func jscString(v ptr) string {
	if jscValueToStringAsBytes != nil {
		if by := jscValueToStringAsBytes(v); by != 0 {
			var n uintptr
			data := gBytesGetData(by, &n)
			var out string
			if data != 0 && n > 0 {
				out = string(unsafe.Slice((*byte)(toPtr(data)), n))
			}
			gBytesUnref(by)
			return out
		}
	}
	return takeStr(jscValueToString(v))
}

// platform.Surface ---------------------------------------------------------

func (s *surface) Show() {
	if !s.closed {
		gtkWindowPresent(s.win)
	}
}

func (s *surface) Hide() {
	if !s.closed {
		gtkWidgetHide(s.win)
	}
}

func (s *surface) Close() {
	if !s.closed {
		gtkWidgetDestroy(s.win) // "destroy" emits SurfaceClosed
	}
}

func (s *surface) SetTitle(title string) {
	if !s.closed {
		gtkWindowSetTitle(s.win, title)
	}
}

func (s *surface) SetSize(w, h int) {
	if !s.closed && w > 0 && h > 0 {
		s.applySize(w, h)
	}
}

func (s *surface) Size() (int, int) {
	if s.closed {
		return 0, 0
	}
	var w, h int32
	gtkWindowGetSize(s.win, &w, &h)
	return int(w), int(h)
}

func (s *surface) Focus() {
	if !s.closed {
		gtkWindowPresent(s.win)
	}
}

// Navigate, Eval and OpenDevTools do nothing on a canvas surface.
func (s *surface) Navigate(url string) {
	if !s.closed && s.web != 0 {
		webkitWebViewLoadURI(s.web, url)
	}
}

func (s *surface) Eval(js string) {
	if s.closed || s.web == 0 {
		return
	}
	if webkitWebViewEvaluateJavascript != nil {
		webkitWebViewEvaluateJavascript(s.web, js, len(js), nil, nil, 0, 0, 0)
	} else {
		webkitWebViewRunJavascript(s.web, js, 0, 0, 0)
	}
}

func (s *surface) OpenDevTools() {
	if !s.closed && s.debug && s.web != 0 {
		webkitWebInspectorShow(webkitWebViewGetInspector(s.web))
	}
}

func (s *surface) Native() uintptr { return s.win }
