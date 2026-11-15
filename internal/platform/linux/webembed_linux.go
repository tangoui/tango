//go:build linux && !android

package linux

// Web views embedded in a canvas surface's layout (platform.WebViewEmbedder,
// internal/platform/webembed.go).
//
// Widget tree. A canvas surface already packs its drawing area as the
// main child of a GtkOverlay (wrapGL, canvas_gl_linux.go). Each embedded
// view adds one overlay child to it: a GtkLayout, the clip container,
// holding the WebKitWebView.
//
//	box -> GtkOverlay -> drawing area (main child)
//	                  -> GtkGLArea (overlay child, while UseGLArea is on; kept lowest)
//	                  -> GtkLayout -> WebKitWebView   (embedded view 1)
//	                  -> GtkLayout -> WebKitWebView   (embedded view 2, above 1)
//
// Clipping. The overlay's "get-child-position" signal gives each layout
// the placement's Clip rectangle; the overlay puts every overlay child in
// a child GdkWindow of that size, and the GtkLayout has windows of its
// own as well, so the web view is clipped to Clip. Inside the layout the
// web view sits at Frame's offset (gtk_layout_move; usually negative) with
// Frame's size (gtk_widget_set_size_request). GtkLayout measures 0×0 and
// positions children anywhere, so neither widget is under-allocated. The
// web view is created inside its layout and never re-parented (WebKit's
// widget does not survive unrealize/realize well); the layout is shown
// and hidden instead.
//
// Stacking. The overlay keeps its children's windows in list order (GTK
// re-shows, and so raises, them on every allocation): views created later
// are above earlier ones. UseGLArea after an embed moves the GLArea to the
// front of the list (embedLowerGL), so it stays below the web views.
//
// Focus. A click on the page lets WebKit grab focus; FocusWeb grabs it;
// BlurWeb gives it back to the canvas's drawing area. The web view's own
// focus-in/out events report SurfaceFocused / SurfaceBlurred for the
// embedded id (they also follow the window gaining or losing focus while
// the web view is its focus widget). Hiding or closing a focused view
// hands focus back to the canvas.
//
// The web part (user content manager, bridge, message gate, scheme
// requests, load tracking, WebView extensions) is a *surface with only
// its web fields set (createWeb / wireWeb / releaseWeb in
// surface_linux.go), registered in byWeb under the embedded id but never
// in backend.surfaces; its win is the parent's window, used as the parent
// of permission dialogs. It is a field, not an embedded struct, so none
// of the window methods are promoted.

import (
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.WebViewEmbedder = (*backend)(nil)

	_ platform.EmbeddedWebView = (*embeddedWeb)(nil)
	_ platform.WebFinder       = (*embeddedWeb)(nil)
	_ platform.WebCookies      = (*embeddedWeb)(nil)
	_ platform.WebData         = (*embeddedWeb)(nil)
	_ platform.WebUserAgent    = (*embeddedWeb)(nil)
	_ platform.WebPDF          = (*embeddedWeb)(nil)
	_ platform.WebSnapshot     = (*embeddedWeb)(nil)
	_ platform.WebDownloads    = (*embeddedWeb)(nil)
)

var embedFFI struct {
	once sync.Once
	err  error

	layoutNew         func(hadj, vadj ptr) ptr
	layoutPut         func(l, w ptr, x, y int32)
	layoutMove        func(l, w ptr, x, y int32)
	overlayAddOverlay func(o, w ptr)
	overlayReorder    func(o, w ptr, pos int32) // 3.18
	widgetQueueResize func(w ptr)
	widgetGrabFocus   func(w ptr)
	widgetIsFocus     func(w ptr) bool

	posCB ptr // get-child-position trampoline
}

// loadEmbedFFI binds the symbols once; main thread, after Init.
func loadEmbedFFI() error {
	f := &embedFFI
	f.once.Do(func() {
		if libGTK == 0 {
			f.err = errors.New("tango/linux: embedded web view used before Init")
			return
		}
		var bd binder
		t := libGTK
		bd.req(&f.layoutNew, "gtk_layout_new", t)
		bd.req(&f.layoutPut, "gtk_layout_put", t)
		bd.req(&f.layoutMove, "gtk_layout_move", t)
		bd.req(&f.overlayAddOverlay, "gtk_overlay_add_overlay", t)
		bd.req(&f.overlayReorder, "gtk_overlay_reorder_overlay", t)
		bd.req(&f.widgetQueueResize, "gtk_widget_queue_resize", t)
		bd.req(&f.widgetGrabFocus, "gtk_widget_grab_focus", t)
		bd.req(&f.widgetIsFocus, "gtk_widget_is_focus", t)
		if len(bd.missing) > 0 {
			f.err = fmt.Errorf("tango/linux: embedded web view: missing native symbols: %v", bd.missing)
			return
		}
		// gboolean get-child-position (GtkOverlay*, GtkWidget*, GdkRectangle*, gpointer)
		f.posCB = purego.NewCallback(func(overlay, widget, alloc, _ ptr) ptr {
			return embedChildPosition(overlay, widget, alloc)
		})
	})
	return f.err
}

// embedHost is the overlay of one canvas surface and its embedded views.
type embedHost struct {
	s       *surface
	overlay ptr
	list    []*embeddedWeb // in creation order (bottom to top)
}

var (
	embedHosts     = map[*surface]*embedHost{}
	embedByOverlay = map[ptr]*embedHost{}
	embedByLayout  = map[ptr]*embeddedWeb{}
)

// embeddedWeb is a WebView embedded in a canvas surface. Main thread only.
type embeddedWeb struct {
	w      *surface // the web part; w.closed is the embedded view's closed flag
	h      *embedHost
	layout ptr // GtkLayout clip container (overlay child)

	p      platform.WebPlacement
	hidden bool // Show / Hide
	track  embedTracker
}

// NewEmbeddedWebView implements platform.WebViewEmbedder. Main thread.
func (b *backend) NewEmbeddedWebView(id, parent platform.SurfaceID, o platform.SurfaceOptions, web platform.WebOptions) (platform.EmbeddedWebView, error) {
	if !b.loaded.Load() {
		return nil, errors.New("tango/linux: NewEmbeddedWebView before Init")
	}
	ps := b.surfaces[parent]
	if ps == nil || ps.closed {
		return nil, fmt.Errorf("tango/linux: NewEmbeddedWebView: no open surface %d", parent)
	}
	if ps.canvas == nil {
		return nil, errors.New("tango/linux: NewEmbeddedWebView: the parent is not a canvas surface")
	}
	if ps.canvas.glOverlay == 0 {
		// GTK older than 3.18: the canvas has no overlay to stack on.
		return nil, fmt.Errorf("%w: the canvas has no GtkOverlay", platform.ErrUnsupported)
	}
	if err := loadEmbedFFI(); err != nil {
		return nil, fmt.Errorf("%w: %v", platform.ErrUnsupported, err)
	}
	f := &embedFFI

	w := &surface{b: b, id: id, debug: o.Debug, win: ps.win}
	pendingWeb = &web // read by initWebExt (wireWeb)
	defer func() { pendingWeb = nil }()
	if err := w.createWeb(o); err != nil {
		return nil, err
	}
	layout := f.layoutNew(0, 0)
	if layout == 0 {
		gtkWidgetDestroy(w.web)
		w.releaseWeb()
		return nil, errors.New("tango/linux: gtk_layout_new failed")
	}
	f.layoutPut(layout, w.web, 0, 0)
	gtkWidgetShow(w.web) // the layout stays hidden until a visible placement
	w.wireWeb()
	// As initExt does for WebView surfaces, after initWebExt's load-failed
	// handler (which hides policy interruptions from this one).
	connect(w.web, "load-changed", 1, w.onLoadStarted)
	connectExt(w.web, "load-failed", 3, w.onLoadFailed)
	connect(w.web, "focus-in-event", 1, func(_, _ ptr) ptr {
		if !w.closed {
			b.host.OnSurfaceEvent(id, platform.SurfaceEvent{Kind: platform.SurfaceFocused})
		}
		return 0 // propagate: WebKit handles focus too
	})
	connect(w.web, "focus-out-event", 1, func(_, _ ptr) ptr {
		if !w.closed {
			b.host.OnSurfaceEvent(id, platform.SurfaceEvent{Kind: platform.SurfaceBlurred})
		}
		return 0
	})

	h := embedHosts[ps]
	if h == nil {
		h = &embedHost{s: ps, overlay: ps.canvas.glOverlay}
		gSignalConnectData(h.overlay, "get-child-position", f.posCB, 0, 0, 0)
		embedHosts[ps] = h
		embedByOverlay[h.overlay] = h
	}
	e := &embeddedWeb{w: w, h: h, layout: layout}
	h.list = append(h.list, e)
	embedByLayout[layout] = e
	f.overlayAddOverlay(h.overlay, layout) // last: above the canvas, the GLArea and earlier views
	b.byWeb[w.web] = w

	url := o.URL
	if url == "" {
		url = "about:blank"
	}
	webkitWebViewLoadURI(w.web, url)
	return e, nil
}

// embedChildPosition handles "get-child-position" on a canvas overlay:
// embedded views get their clip rectangle, everything else (the GLArea)
// the default placement.
func embedChildPosition(overlay, widget, alloc ptr) ptr {
	e := embedByLayout[widget]
	if e == nil || e.w.closed || e.h.overlay != overlay || alloc == 0 {
		return 0
	}
	c := e.track.clip
	out := (*gdkRect)(toPtr(alloc))
	*out = gdkRect{X: int32(c.Min.X), Y: int32(c.Min.Y), Width: int32(max(c.Dx(), 1)), Height: int32(max(c.Dy(), 1))}
	return 1
}

// embedLowerGL keeps a GLArea that UseGLArea just added to a canvas
// overlay below the embedded web views (canvas_gl_linux.go).
func embedLowerGL(overlay, area ptr) {
	if h := embedByOverlay[overlay]; h != nil && len(h.list) > 0 {
		embedFFI.overlayReorder(overlay, area, 0)
	}
}

// apply brings the widgets to the current placement and hidden flag.
func (e *embeddedWeb) apply() {
	f := &embedFFI
	st := e.track.update(e.p.Geometry(1, e.hidden)) // GTK positions in logical pixels
	if st.Hide {
		e.BlurWeb() // a hidden focus widget would leave the window without focus
		gtkWidgetHide(e.layout)
	}
	if st.Frame {
		fr := e.track.frame
		gtkWidgetSetSizeRequest(e.w.web, int32(fr.Dx()), int32(fr.Dy()))
		f.layoutMove(e.layout, e.w.web, int32(fr.Min.X), int32(fr.Min.Y)) // queues a resize
	}
	if st.Clip {
		f.widgetQueueResize(e.layout) // get-child-position runs again
	}
	if st.Show {
		gtkWidgetShow(e.layout)
	}
}

// platform.EmbeddedWebView ------------------------------------------------

func (e *embeddedWeb) SetPlacement(p platform.WebPlacement) {
	if e.w.closed || p == e.p {
		return
	}
	e.p = p
	e.apply()
}

func (e *embeddedWeb) FocusWeb() {
	if !e.w.closed {
		embedFFI.widgetGrabFocus(e.w.web)
	}
}

func (e *embeddedWeb) BlurWeb() {
	if e.w.closed || e.h.s.closed || !embedFFI.widgetIsFocus(e.w.web) {
		return
	}
	embedFFI.widgetGrabFocus(e.h.s.content) // the canvas's drawing area
}

// platform.Surface ----------------------------------------------------------

func (e *embeddedWeb) Show() {
	if !e.w.closed && e.hidden {
		e.hidden = false
		e.apply()
	}
}

func (e *embeddedWeb) Hide() {
	if !e.w.closed && !e.hidden {
		e.hidden = true
		e.apply()
	}
}

// Close destroys the view and reports SurfaceClosed. Idempotent.
func (e *embeddedWeb) Close() {
	w := e.w
	if w.closed {
		return
	}
	e.BlurWeb()
	w.closed = true
	h := e.h
	for i, x := range h.list {
		if x == e {
			h.list = append(h.list[:i:i], h.list[i+1:]...)
			break
		}
	}
	delete(embedByLayout, e.layout)
	gtkWidgetDestroy(e.layout) // removes it from the overlay, destroys the web view
	w.releaseWeb()
	w.b.host.OnSurfaceEvent(w.id, platform.SurfaceEvent{Kind: platform.SurfaceClosed})
}

func (e *embeddedWeb) SetTitle(string)  {}
func (e *embeddedWeb) SetSize(int, int) {}
func (e *embeddedWeb) Size() (int, int) { return e.p.Size() }
func (e *embeddedWeb) Focus()           { e.FocusWeb() }
func (e *embeddedWeb) Navigate(u string) {
	e.w.Navigate(u)
}
func (e *embeddedWeb) Eval(js string)  { e.w.Eval(js) }
func (e *embeddedWeb) OpenDevTools()   { e.w.OpenDevTools() }
func (e *embeddedWeb) Native() uintptr { return e.w.web }

// closeEmbedded runs from the parent window's "destroy", before anything
// else: every embedded view is closed and reports SurfaceClosed.
func (s *surface) closeEmbedded() {
	h := embedHosts[s]
	if h == nil {
		return
	}
	for _, e := range append([]*embeddedWeb(nil), h.list...) {
		e.Close()
	}
	delete(embedByOverlay, h.overlay)
	delete(embedHosts, s)
}

// WebView extensions (webview_ext_linux.go), forwarded to the web part. --

func (e *embeddedWeb) Find(text string, opts platform.FindOptions, done func(platform.FindResult, error)) {
	e.w.Find(text, opts, done)
}

func (e *embeddedWeb) FindNext(backward bool, done func(platform.FindResult, error)) {
	e.w.FindNext(backward, done)
}

func (e *embeddedWeb) StopFind() { e.w.StopFind() }

func (e *embeddedWeb) Cookies(uri string, done func([]*http.Cookie, error)) {
	e.w.Cookies(uri, done)
}

func (e *embeddedWeb) SetCookie(c *http.Cookie, done func(error)) { e.w.SetCookie(c, done) }

func (e *embeddedWeb) DeleteCookie(c *http.Cookie, done func(error)) { e.w.DeleteCookie(c, done) }

func (e *embeddedWeb) ClearCookies(domain string, done func(error)) {
	e.w.ClearCookies(domain, done)
}

func (e *embeddedWeb) ClearBrowsingData(kinds platform.BrowsingData, done func(error)) {
	e.w.ClearBrowsingData(kinds, done)
}

func (e *embeddedWeb) SetUserAgent(ua string) { e.w.SetUserAgent(ua) }

func (e *embeddedWeb) PrintToPDF(opts platform.PDFOptions, done func([]byte, error)) {
	e.w.PrintToPDF(opts, done)
}

func (e *embeddedWeb) Screenshot(done func([]byte, error)) { e.w.Screenshot(done) }

func (e *embeddedWeb) CancelDownload(id uint64) { e.w.CancelDownload(id) }
