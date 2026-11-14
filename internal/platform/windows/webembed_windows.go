//go:build windows && (amd64 || arm64)

package windows

// WebViews embedded in a canvas surface's layout (platform.WebViewEmbedder,
// docs/specs/webview-embed.md).
//
// Each view is a "clip container" child window of the parent surface's
// top-level window (class TangoWebEmbed, WS_CLIPSIBLINGS|WS_CLIPCHILDREN)
// with a WebView2 controller inside it. The container is sized to the
// visible part of the view (placement Clip); the controller's bounds are
// the whole view (Frame) relative to the container, usually with a
// negative origin, so the container clips the page to what the layout
// shows. Containers stack above the canvas child window, later ones above
// earlier ones. Pure placement logic is in webembed_plan.go.
//
// The WebView2 part is a *surface value that is never registered as a
// window (Backend.surfaces / byID): its hwnd is the container, its id the
// embedded id, and its embed field points back here. It reuses the
// surface's creation path (createWebView, setUp: document scripts and the
// native shim, messages, asset serving, navigation events, the
// webview_ext_windows.go handlers) and its WebView methods; setUp hands
// visibility and bounds to webReady instead of filling a client area.
// File drops are off (the inner surface has no AcceptFiles extras); menu
// accelerators pressed in the page run the parent window's menu.
//
// Focus. GotFocus / LostFocus report SurfaceFocused / SurfaceBlurred for
// the embedded id. A view that had focus when the window was deactivated
// gets it back on activation (the parent's focusContent); focus moving to
// another window of the same top-level window drops that claim. Hiding a
// focused view gives focus back to the canvas first.

import (
	"errors"
	"fmt"
	"image"
	"net/http"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.WebViewEmbedder = (*Backend)(nil)
	_ platform.EmbeddedWebView = (*embedded)(nil)

	_ platform.WebFinder    = (*embedded)(nil)
	_ platform.WebCookies   = (*embedded)(nil)
	_ platform.WebData      = (*embedded)(nil)
	_ platform.WebUserAgent = (*embedded)(nil)
	_ platform.WebHeaders   = (*embedded)(nil)
	_ platform.WebPDF       = (*embedded)(nil)
	_ platform.WebSnapshot  = (*embedded)(nil)
	_ platform.WebDownloads = (*embedded)(nil)
	_ platform.PageControl  = (*embedded)(nil)
)

const (
	embedClass = "TangoWebEmbed"

	embedWsChild        = 0x40000000 // WS_CHILD
	embedWsClipSiblings = 0x04000000 // WS_CLIPSIBLINGS
	embedWmEraseBkgnd   = 0x0014
	embedSwShowNA       = 8 // SW_SHOWNA: show without activating
)

// embedded is a WebView embedded in another surface. UI thread only.
type embedded struct {
	w      *surface // the WebView part; w.hwnd is the container window
	parent *surface
	plan   embedPlan

	focused     bool // GotFocus seen, LostFocus not yet
	focusOnShow bool // FocusWeb while hidden: focus when shown
	closed      bool
}

var (
	embedByHwnd    = map[uintptr]*embedded{}    // by container window
	embedsByParent = map[*surface][]*embedded{} // creation order (bottom to top)
	embedFocus     = map[*surface]*embedded{}   // view to refocus on activation

	embedClassOnce sync.Once
	embedClassErr  error
)

func registerEmbedClass() error {
	embedClassOnce.Do(func() {
		wc := wndClassEx{
			WndProc:   syscall.NewCallback(embedWndProc),
			Instance:  instance(),
			ClassName: u16(embedClass),
			// No cursor (WebView2 sets it) and no background brush: the
			// controller's window always covers the container.
		}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			embedClassErr = fmt.Errorf("tango: RegisterClassExW(%s): %v", embedClass, err)
		}
	})
	return embedClassErr
}

// embedWndProc is the container's window procedure.
func embedWndProc(hwnd, m, wp, lp uintptr) uintptr {
	switch uint32(m) {
	case embedWmEraseBkgnd:
		return 1
	case wmSetFocus:
		if e := embedByHwnd[hwnd]; e != nil {
			e.FocusWeb() // the container itself is never the focus
			return 0
		}
	case wmDestroy:
		if e := embedByHwnd[hwnd]; e != nil {
			e.destroyed()
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// NewEmbeddedWebView implements platform.WebViewEmbedder. Main thread.
func (b *Backend) NewEmbeddedWebView(id, parentID platform.SurfaceID, opts platform.SurfaceOptions, web platform.WebOptions) (platform.EmbeddedWebView, error) {
	if b.h == nil {
		return nil, errors.New("tango/windows: NewEmbeddedWebView before Init")
	}
	parent := b.byID[parentID]
	if parent == nil || parent.closed {
		return nil, fmt.Errorf("tango/windows: NewEmbeddedWebView: no open surface %d", parentID)
	}
	if b.envErr != nil { // as newSurface: let a later view try again
		err := b.envErr
		b.env, b.envErr, b.envStarted = 0, nil, false
		return nil, err
	}
	if err := b.startEnvironment(); err != nil {
		return nil, err
	}
	if err := registerEmbedClass(); err != nil {
		return nil, err
	}
	e := &embedded{parent: parent}
	s := &surface{b: b, id: id, opts: opts, embed: e, created: true}
	e.w = s
	hwnd, _, err := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16(embedClass))), 0,
		embedWsChild|embedWsClipSiblings|wsClipChildren, // hidden until placed
		0, 0, 0, 0, parent.hwnd, 0, instance(), 0)
	if hwnd == 0 {
		return nil, fmt.Errorf("tango/windows: NewEmbeddedWebView: CreateWindowExW: %v", err)
	}
	s.hwnd = hwnd
	embedByHwnd[hwnd] = e
	embedsByParent[parent] = append(embedsByParent[parent], e)
	procSetWindowPos.Call(hwnd, attachHwndTop, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	e.apply() // records the hidden state

	// Before the controller exists, so setUp (initWebExt) applies them
	// before the first navigation.
	if web.UserAgent != "" {
		s.SetUserAgent(web.UserAgent)
	}
	if len(web.Headers) > 0 {
		s.SetExtraHeaders(web.Headers)
	}
	url := opts.URL
	if url == "" {
		url = "about:blank"
	}
	s.whenReady(func() { s.navigate(url) })
	b.whenEnvironment(s.createWebView)
	return e, nil
}

// webReady runs from setUp once the controller exists, in place of
// filling a client area: focus events, then bounds and visibility from
// the placement.
func (e *embedded) webReady() {
	ctl := e.w.controller
	addEvent(ctl, ctlAddGotFocus, e.gotFocus)
	addEvent(ctl, ctlAddLostFocus, e.lostFocus)
	e.plan.controllerReset()
	e.apply()
}

// apply brings the container and the controller to the current placement
// and hidden flag at the parent's DPI. Nothing is called when nothing
// changed.
func (e *embedded) apply() {
	if e.closed {
		return
	}
	s := e.w
	c, k := e.plan.sync(embedScale(dpiOf(e.parent.hwnd)), s.controller != 0)
	if c.None() && k.None() {
		return
	}
	if embedBlurOnHide(c, e.hasFocus()) {
		e.BlurWeb()
	}
	if k.Move {
		putBounds(s.controller, embedRect(k.Rect))
	}
	if k.Show {
		comCall(s.controller, ctlPutIsVisible, 1)
	}
	if c.Move {
		r := c.Rect
		procSetWindowPos.Call(s.hwnd, 0, uintptr(int32(r.Min.X)), uintptr(int32(r.Min.Y)),
			uintptr(r.Dx()), uintptr(r.Dy()), swpNoZOrder|swpNoActivate)
		if s.controller != 0 {
			comCall(s.controller, ctlNotifyParentWindowPositionChanged)
		}
	}
	if c.Show {
		procShowWindow.Call(s.hwnd, embedSwShowNA)
	}
	if k.Hide {
		comCall(s.controller, ctlPutIsVisible, 0)
	}
	if c.Hide {
		procShowWindow.Call(s.hwnd, swHide)
	}
	if e.focusOnShow && e.plan.visible() && s.controller != 0 {
		e.focusOnShow = false
		s.focusWebView()
	}
}

func embedRect(r image.Rectangle) rect {
	return rect{Left: int32(r.Min.X), Top: int32(r.Min.Y), Right: int32(r.Max.X), Bottom: int32(r.Max.Y)}
}

// hasFocus reports whether keyboard focus is in the view.
func (e *embedded) hasFocus() bool {
	if e.focused {
		return true
	}
	f, _, _ := canvasGetFocus.Call()
	return e.contains(f)
}

// contains reports whether hwnd is the container or one of its
// descendants (WebView2's windows).
func (e *embedded) contains(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	if hwnd == e.w.hwnd {
		return true
	}
	r, _, _ := attachIsChild.Call(e.w.hwnd, hwnd)
	return r != 0
}

func (e *embedded) gotFocus(_, _ uintptr) {
	if e.closed {
		return
	}
	e.focused = true
	embedFocus[e.parent] = e
	e.w.event(platform.SurfaceEvent{Kind: platform.SurfaceFocused})
}

func (e *embedded) lostFocus(_, _ uintptr) {
	if e.closed {
		return
	}
	e.focused = false
	// Focus that moved to another window of the same top-level window
	// (the canvas, another view) ends the claim; none, or a window
	// elsewhere, means the window was deactivated: keep it.
	if f, _, _ := canvasGetFocus.Call(); f != 0 && embedFocus[e.parent] == e && !e.contains(f) {
		if r, _, _ := attachIsChild.Call(e.parent.hwnd, f); r != 0 || f == e.parent.hwnd {
			delete(embedFocus, e.parent)
		}
	}
	e.w.event(platform.SurfaceEvent{Kind: platform.SurfaceBlurred})
}

// destroyed releases the view once its container is gone (or is going
// with the parent) and reports SurfaceClosed.
func (e *embedded) destroyed() {
	if e.closed {
		return
	}
	p := e.parent
	if !p.closed && e.hasFocus() {
		e.BlurWeb() // focus would otherwise be left nowhere
	}
	e.closed = true
	s := e.w
	s.closed = true
	s.pending = nil
	s.closeWebExt()
	if s.controller != 0 {
		comCall(s.controller, ctlClose)
		release(s.webview)
		release(s.controller)
		s.controller, s.webview = 0, 0
	}
	delete(embedByHwnd, s.hwnd)
	list := embedsByParent[p]
	for i, x := range list {
		if x == e {
			list = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	if len(list) == 0 {
		delete(embedsByParent, p)
	} else {
		embedsByParent[p] = list
	}
	if embedFocus[p] == e {
		delete(embedFocus, p)
	}
	s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceClosed})
}

// ---- hooks called from the surface code ----

// menuOwner is the surface whose menu (accelerators) applies to s: the
// parent window for an embedded view.
func (s *surface) menuOwner() *surface {
	if s.embed != nil {
		return s.embed.parent
	}
	return s
}

// destroyEmbedded closes the surface's embedded views, each reporting
// SurfaceClosed, before the surface reports its own.
func (s *surface) destroyEmbedded() {
	for _, e := range append([]*embedded(nil), embedsByParent[s]...) {
		e.Close()
	}
	delete(embedsByParent, s)
	delete(embedFocus, s)
}

// restackEmbedded puts the embedded views back on top of their siblings,
// in creation order: WebView2's own window (a WebView parent) and the
// canvas's CPU-fallback sibling appear on top when shown.
func (s *surface) restackEmbedded() {
	for _, e := range embedsByParent[s] {
		if !e.closed {
			procSetWindowPos.Call(e.w.hwnd, attachHwndTop, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		}
	}
}

// restackEmbeddedIn is restackEmbedded for the surface whose top-level
// window is hwnd, if any.
func restackEmbeddedIn(hwnd uintptr) {
	if b := theBackend; b != nil {
		if s := b.surfaces[hwnd]; s != nil {
			s.restackEmbedded()
		}
	}
}

// movedEmbedded tells the embedded views' controllers that the window
// moved (popups, IME and the like are positioned in screen coordinates).
func (s *surface) movedEmbedded() {
	for _, e := range embedsByParent[s] {
		if e.w.controller != 0 {
			comCall(e.w.controller, ctlNotifyParentWindowPositionChanged)
		}
	}
}

// focusEmbedded gives focus back to the view that had it when the window
// was deactivated; false when the canvas (or page) should get it.
func (s *surface) focusEmbedded() bool {
	e := embedFocus[s]
	if e == nil || e.closed || !e.plan.visible() || e.w.controller == 0 {
		return false
	}
	e.w.focusWebView()
	return true
}

// ---- platform.EmbeddedWebView ----

func (e *embedded) SetPlacement(p platform.WebPlacement) {
	if e.closed {
		return
	}
	e.plan.place(p)
	e.apply()
}

func (e *embedded) FocusWeb() {
	if e.closed {
		return
	}
	if !e.plan.visible() || e.w.controller == 0 {
		e.focusOnShow = true // apply focuses once shown and ready
		return
	}
	e.w.focusWebView()
}

func (e *embedded) BlurWeb() {
	e.focusOnShow = false
	if e.closed || !e.hasFocus() {
		return
	}
	if embedFocus[e.parent] == e {
		delete(embedFocus, e.parent)
	}
	p := e.parent
	switch {
	case p.closed:
	case p.canvas != nil:
		p.canvas.focus()
	case p.controller != 0:
		p.focusWebView()
	default:
		canvasSetFocus.Call(p.hwnd)
	}
}

// ---- platform.Surface ----

func (e *embedded) Show() {
	if !e.closed && e.plan.hidden {
		e.plan.hidden = false
		e.apply()
	}
}

func (e *embedded) Hide() {
	if !e.closed && !e.plan.hidden {
		e.plan.hidden = true
		e.apply()
	}
}

func (e *embedded) Close() {
	if e.closed {
		return
	}
	procDestroyWindow.Call(e.w.hwnd) // WM_DESTROY → destroyed()
	e.destroyed()                    // in case the window was already gone
}

func (e *embedded) SetTitle(string)               {}
func (e *embedded) SetSize(int, int)              {}
func (e *embedded) Size() (int, int)              { return e.plan.size() }
func (e *embedded) Focus()                        { e.FocusWeb() }
func (e *embedded) Navigate(url string)           { e.w.Navigate(url) }
func (e *embedded) Eval(js string)                { e.w.Eval(js) }
func (e *embedded) OpenDevTools()                 { e.w.OpenDevTools() }
func (e *embedded) Native() uintptr               { return e.w.controller }
func (e *embedded) Reload()                       { e.w.Reload() }
func (e *embedded) SetZoom(f float64)             { e.w.SetZoom(f) }
func (e *embedded) Zoom() float64                 { return e.w.Zoom() }
func (e *embedded) Print()                        { e.w.Print() }
func (e *embedded) SetUserAgent(s string)         { e.w.SetUserAgent(s) }
func (e *embedded) SetExtraHeaders(h http.Header) { e.w.SetExtraHeaders(h) }
func (e *embedded) CancelDownload(id uint64)      { e.w.CancelDownload(id) }
func (e *embedded) StopFind()                     { e.w.StopFind() }

// ---- WebView interfaces (webview_ext.go), as for WebView surfaces ----

func (e *embedded) Find(text string, opts platform.FindOptions, done func(platform.FindResult, error)) {
	e.w.Find(text, opts, done)
}

func (e *embedded) FindNext(backward bool, done func(platform.FindResult, error)) {
	e.w.FindNext(backward, done)
}

func (e *embedded) Cookies(url string, done func([]*http.Cookie, error)) { e.w.Cookies(url, done) }
func (e *embedded) SetCookie(c *http.Cookie, done func(error))           { e.w.SetCookie(c, done) }
func (e *embedded) DeleteCookie(c *http.Cookie, done func(error))        { e.w.DeleteCookie(c, done) }
func (e *embedded) ClearCookies(domain string, done func(error))         { e.w.ClearCookies(domain, done) }

func (e *embedded) ClearBrowsingData(kinds platform.BrowsingData, done func(error)) {
	e.w.ClearBrowsingData(kinds, done)
}

func (e *embedded) PrintToPDF(opts platform.PDFOptions, done func([]byte, error)) {
	e.w.PrintToPDF(opts, done)
}

func (e *embedded) Screenshot(done func([]byte, error)) { e.w.Screenshot(done) }
