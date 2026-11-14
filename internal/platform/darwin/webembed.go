//go:build darwin && !ios

package darwin

// WebViews embedded in a canvas surface's layout (docs/specs/webview-embed.md).
//
// Each embedded view is a TangoWebView inside a TangoEmbedClip: a flipped
// NSView that clips its subviews, added on top of the container that
// holds the parent's content view (surface.setContent). The clip view is
// sized to the visible part of the placement and the web view is offset
// inside it, so scrolling a self-drawn list or a parent clip cuts the web
// view exactly. Positions are snapped to device pixels.
//
// The web view gets its own surface record (win is the parent's window)
// so the delegate, scheme handler and every WebView extension of
// webview_ext.go work unchanged under the embedded view's SurfaceID. The
// record is not in backend.surfaces: window-level calls never reach it.

import (
	"errors"
	"image"
	"net/http"
	"sync"

	"github.com/ebitengine/purego"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.WebViewEmbedder = (*backend)(nil)
	_ platform.EmbeddedWebView = (*embedded)(nil)

	_ platform.WebFinder    = (*embedded)(nil)
	_ platform.WebCookies   = (*embedded)(nil)
	_ platform.WebData      = (*embedded)(nil)
	_ platform.WebUserAgent = (*embedded)(nil)
	_ platform.WebPDF       = (*embedded)(nil)
	_ platform.WebSnapshot  = (*embedded)(nil)
	_ platform.WebDownloads = (*embedded)(nil)
	_ platform.PageControl  = (*embedded)(nil)
)

var errEmbedParent = errors.New("tango/darwin: embedded WebView: no such parent surface")

// embedded is one WebView inside a parent surface. Main thread only.
type embedded struct {
	s      *surface // the web view's own record (s.win is the parent's window)
	parent *surface
	clip   id // TangoEmbedClip
	place  platform.WebPlacement
	hidden bool
	geo    platform.EmbedGeometry // last applied
	scale  float64                // scale geo was computed for
	closed bool
}

var (
	embedsOf     = map[*surface][]*embedded{} // parent → its embedded views, bottom to top
	embedByWeb   = map[id]*embedded{}
	embedHostOf  = map[*surface]id{}
	embedClassOK bool
)

func registerEmbedClasses() {
	if embedClassOK {
		return
	}
	embedClassOK = true
	classDef("TangoEmbedClip", "NSView", nil, []objc.MethodDef{
		method("isFlipped", func(_ id, _ objc.SEL) bool { return true }),
	})
	// TangoEmbedHost fills the parent's container above its content and
	// holds the clip views in top-left coordinates, so they keep their
	// place while the window resizes. It takes no input itself: points
	// outside every clip view fall through to the canvas.
	purego.RegisterFunc(&msgEmbedHit, mustDlsym(libObjC, "objc_msgSendSuper2"))
	classDef("TangoEmbedHost", "NSView", nil, []objc.MethodDef{
		method("isFlipped", func(_ id, _ objc.SEL) bool { return true }),
		method("hitTest:", func(self id, _ objc.SEL, pt nsPoint) id {
			hit := msgEmbedHit(&objcSuper{receiver: self, class: class("TangoEmbedHost")}, sel("hitTest:"), pt)
			if hit == self {
				return 0
			}
			return hit
		}),
	})
}

var msgEmbedHit func(sup *objcSuper, s objc.SEL, p nsPoint) id

// embedHost returns the parent's host view, creating it on top of the
// container. Inside withPool.
func embedHost(p *surface) id {
	if h := embedHostOf[p]; h != 0 {
		return h
	}
	container := send(p.view, "superview")
	if container == 0 {
		return 0
	}
	h := msgInitRect(send(class("TangoEmbedHost"), "alloc"), sel("initWithFrame:"), msgRect(container, sel("bounds")))
	send(h, "setAutoresizingMask:", nsViewWidthHeightSizable)
	send(container, "addSubview:", uintptr(h))
	release(h) // the container keeps it
	embedHostOf[p] = h
	return h
}

// NewEmbeddedWebView implements platform.WebViewEmbedder.
func (b *backend) NewEmbeddedWebView(sid, parent platform.SurfaceID, opts platform.SurfaceOptions, web platform.WebOptions) (platform.EmbeddedWebView, error) {
	if !isMainThread() {
		return nil, errors.New("tango/darwin: NewEmbeddedWebView must run on the main thread")
	}
	p := b.surfaces[parent]
	if p == nil || p.closed || p.view == 0 {
		return nil, errEmbedParent
	}
	registerEmbedClasses()
	e := &embedded{parent: p, hidden: false}
	s := &surface{b: b, id: sid, win: p.win}
	e.s = s
	var err error
	withPool(func() {
		host := embedHost(p)
		if host == 0 {
			err = errEmbedParent
			return
		}
		rect := nsRect{Size: nsSize{Width: 1, Height: 1}}
		e.clip = msgInitRect(send(class("TangoEmbedClip"), "alloc"), sel("initWithFrame:"), rect)
		send(e.clip, "setWantsLayer:", 1)
		if respondsTo(e.clip, "setClipsToBounds:") { // macOS 14+: not clipping by default
			send(e.clip, "setClipsToBounds:", 1)
		}
		if l := send(e.clip, "layer"); l != 0 {
			send(l, "setMasksToBounds:", 1)
		}
		send(e.clip, "setHidden:", 1)

		s.delegate = alloc("TangoSurfaceDelegate")
		pendingWeb = &web
		cfg := s.webConfiguration(opts)
		s.web = msgInitRectID(send(class("TangoWebView"), "alloc"), sel("initWithFrame:configuration:"), rect, cfg)
		release(cfg)
		send(s.web, "setNavigationDelegate:", uintptr(s.delegate))
		s.initWebExt()
		pendingWeb = nil
		if respondsTo(s.web, "setInspectable:") {
			send(s.web, "setInspectable:", boolArg(opts.Debug))
		}
		s.view = s.web
		send(e.clip, "addSubview:", uintptr(s.web))
		send(host, "addSubview:", uintptr(e.clip)) // above earlier views
	})
	if err != nil {
		return nil, err
	}
	b.byDelegate[s.delegate] = s
	b.byWebView[s.web] = s
	embedByWeb[s.web] = e
	embedsOf[p] = append(embedsOf[p], e)

	start := opts.URL
	if start == "" {
		start = "about:blank"
	}
	s.Navigate(start)
	return e, nil
}

// SetPlacement implements platform.EmbeddedWebView.
func (e *embedded) SetPlacement(p platform.WebPlacement) {
	if e.closed {
		return
	}
	e.place = p
	e.apply()
}

// apply puts the clip view and the web view where the placement says.
func (e *embedded) apply() {
	scale := 1.0
	if w := e.parent.win; w != 0 {
		if sc := msgGetFloat(w, sel("backingScaleFactor")); sc > 0 {
			scale = sc
		}
	}
	g := e.place.Geometry(scale, e.hidden)
	if g == e.geo && scale == e.scale {
		return
	}
	wasVisible := e.geo.Visible
	e.geo, e.scale = g, scale
	withPool(func() {
		if !g.Visible {
			if wasVisible {
				e.resignIfFirst()
				send(e.clip, "setHidden:", 1)
			}
			return
		}
		msgSetRect(e.clip, sel("setFrame:"), devRect(g.Clip, scale))   // the host view is flipped
		msgSetRect(e.s.web, sel("setFrame:"), devRect(g.Frame, scale)) // the clip view is flipped
		if !wasVisible {
			send(e.clip, "setHidden:", 0)
		}
	})
}

// devRect converts a device-pixel rectangle to points (top-left origin).
func devRect(r image.Rectangle, scale float64) nsRect {
	return nsRect{
		Origin: nsPoint{X: float64(r.Min.X) / scale, Y: float64(r.Min.Y) / scale},
		Size:   nsSize{Width: float64(r.Dx()) / scale, Height: float64(r.Dy()) / scale},
	}
}

// resignIfFirst hands keyboard focus back to the parent's content view
// when the web view, about to disappear, has it.
func (e *embedded) resignIfFirst() {
	if e.webIsFirst() {
		send(e.parent.win, "makeFirstResponder:", uintptr(e.parent.view))
	}
}

func (e *embedded) webIsFirst() bool {
	fr := send(e.parent.win, "firstResponder")
	if fr == 0 {
		return false
	}
	return fr == e.s.web || (sendBool(fr, "isKindOfClass:", uintptr(class("NSView"))) && sendBool(fr, "isDescendantOf:", uintptr(e.s.web)))
}

// FocusWeb implements platform.EmbeddedWebView.
func (e *embedded) FocusWeb() {
	if e.closed {
		return
	}
	send(e.parent.win, "makeFirstResponder:", uintptr(e.s.web))
}

// BlurWeb implements platform.EmbeddedWebView.
func (e *embedded) BlurWeb() {
	if e.closed {
		return
	}
	e.resignIfFirst()
}

var (
	webSuperOnce sync.Once
	msgWebSuper  func(sup *objcSuper, s objc.SEL) uintptr
)

// webSuperBool sends a BOOL message to TangoWebView's superclass
// implementation. The class is named explicitly: WebKit observes its web
// views with KVO, whose dynamic subclass would make the object's own
// class (objc.ID.SendSuper) dispatch back into TangoWebView.
func webSuperBool(self id, name string) bool {
	webSuperOnce.Do(func() {
		purego.RegisterFunc(&msgWebSuper, mustDlsym(libObjC, "objc_msgSendSuper2"))
	})
	return byte(msgWebSuper(&objcSuper{receiver: self, class: class("TangoWebView")}, sel(name))) != 0
}

// webFirstResponder is called by TangoWebView's becomeFirstResponder /
// resignFirstResponder (surface_ext.go).
func webFirstResponder(web id, first bool) {
	e := embedByWeb[web]
	if e == nil || e.closed {
		return
	}
	kind := platform.SurfaceBlurred
	if first {
		kind = platform.SurfaceFocused
	}
	e.s.b.h.OnSurfaceEvent(e.s.id, platform.SurfaceEvent{Kind: kind})
}

func (e *embedded) Show() {
	if e.closed || !e.hidden {
		return
	}
	e.hidden = false
	e.apply()
}

func (e *embedded) Hide() {
	if e.closed || e.hidden {
		return
	}
	e.hidden = true
	e.apply()
}

func (e *embedded) Close() {
	if e.closed {
		return
	}
	e.destroy()
	list := embedsOf[e.parent]
	for i, x := range list {
		if x == e {
			embedsOf[e.parent] = append(list[:i:i], list[i+1:]...)
			break
		}
	}
	if len(embedsOf[e.parent]) == 0 {
		delete(embedsOf, e.parent)
	}
}

// destroy removes the views, detaches the delegate and reports
// SurfaceClosed. Like surface.destroy, the Objective-C objects are only
// autoreleased.
func (e *embedded) destroy() {
	e.closed = true
	s := e.s
	withPool(func() {
		if !e.parent.closed {
			e.resignIfFirst()
		}
		s.detachWebView()
		s.closed = true
		send(e.clip, "removeFromSuperview")
	})
	delete(s.b.byDelegate, s.delegate)
	delete(embedByWeb, s.web)
	for _, obj := range []id{s.web, s.ucc, s.delegate, e.clip} {
		autorelease(obj)
	}
	s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceClosed})
}

// destroyEmbeds closes the embedded views of a closing parent (called
// from surface.destroy before the parent reports SurfaceClosed).
func (s *surface) destroyEmbeds() {
	list := embedsOf[s]
	delete(embedsOf, s)
	delete(embedHostOf, s)
	for i := len(list) - 1; i >= 0; i-- {
		if !list[i].closed {
			list[i].destroy()
		}
	}
}

func (e *embedded) SetTitle(string)  {}
func (e *embedded) SetSize(int, int) {}
func (e *embedded) Size() (int, int) { return e.place.Size() }
func (e *embedded) Focus()           { e.FocusWeb() }
func (e *embedded) Native() uintptr  { return uintptr(e.s.web) }
func (e *embedded) Navigate(u string) {
	if !e.closed {
		e.s.Navigate(u)
	}
}

func (e *embedded) Eval(js string) {
	if !e.closed {
		e.s.Eval(js)
	}
}

func (e *embedded) OpenDevTools() {
	if !e.closed {
		e.s.OpenDevTools()
	}
}

// WebView extensions: the web view's own surface record serves them.

func (e *embedded) Find(text string, o platform.FindOptions, done func(platform.FindResult, error)) {
	e.s.Find(text, o, done)
}
func (e *embedded) FindNext(back bool, done func(platform.FindResult, error)) {
	e.s.FindNext(back, done)
}
func (e *embedded) StopFind() { e.s.StopFind() }
func (e *embedded) Cookies(u string, done func([]*http.Cookie, error)) {
	e.s.Cookies(u, done)
}
func (e *embedded) SetCookie(c *http.Cookie, done func(error))    { e.s.SetCookie(c, done) }
func (e *embedded) DeleteCookie(c *http.Cookie, done func(error)) { e.s.DeleteCookie(c, done) }
func (e *embedded) ClearCookies(domain string, done func(error))  { e.s.ClearCookies(domain, done) }
func (e *embedded) ClearBrowsingData(k platform.BrowsingData, done func(error)) {
	e.s.ClearBrowsingData(k, done)
}
func (e *embedded) SetUserAgent(ua string) { e.s.SetUserAgent(ua) }
func (e *embedded) PrintToPDF(o platform.PDFOptions, done func([]byte, error)) {
	e.s.PrintToPDF(o, done)
}
func (e *embedded) Screenshot(done func([]byte, error)) { e.s.Screenshot(done) }
func (e *embedded) CancelDownload(dl uint64)            { e.s.CancelDownload(dl) }
func (e *embedded) Reload()                             { e.s.Reload() }
func (e *embedded) SetZoom(f float64)                   { e.s.SetZoom(f) }
func (e *embedded) Zoom() float64                       { return e.s.Zoom() }
func (e *embedded) Print()                              { e.s.Print() }
