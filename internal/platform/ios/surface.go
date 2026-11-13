//go:build ios

package ios

/*
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

import (
	"github.com/tangoui/tango/internal/platform"
)

// nativeShim is the backend's half of the JS bridge, injected at document
// start before opts.BridgeScript.
const nativeShim = `window.__tangoNative={post:s=>webkit.messageHandlers.tango.postMessage(String(s))};`

// surface is one page: a view controller with a full-screen WKWebView.
// Showing the first page makes it the window's root; later ones are
// presented full screen over it; Hide and Close take a page off that stack.
// Title and size belong to the window manager on iOS: SetTitle sets the
// view controller's title (not displayed), SetSize is ignored and Size
// reports the page's current size in points.
type surface struct {
	b      *backend
	id     platform.SurfaceID
	closed bool
	cv     *canvas // canvas pages (canvas.go): no web view; nil for WebView pages
}

func (b *backend) newSurface(id platform.SurfaceID, opts platform.SurfaceOptions) (*surface, error) {
	return b.newSurfaceUA(id, opts, "")
}

// newSurfaceUA is newSurface with a custom User-Agent set before the first
// request ("" keeps WebKit's); see NewSurfaceWeb in webview_ext.go.
func (b *backend) newSurfaceUA(id platform.SurfaceID, opts platform.SurfaceOptions, ua string) (*surface, error) {
	s := &surface{b: b, id: id}
	shim, f1 := cstring(nativeShim + bodyShim)
	defer f1()
	bridge, f2 := cstring(opts.BridgeScript)
	defer f2()
	title, f3 := cstring(opts.Title)
	defer f3()
	debug := C.int(0)
	if opts.Debug {
		debug = 1
	}
	C.tg_surface_new(C.uint64_t(id), shim, bridge, debug, title)
	b.surfaces[id] = s
	if ua != "" {
		s.SetUserAgent(ua)
	}

	url := opts.URL
	if url == "" {
		url = "about:blank"
	}
	s.Navigate(url)
	if opts.Visible {
		s.Show()
	}
	return s, nil
}

func (s *surface) sid() C.uint64_t { return C.uint64_t(s.id) }

func (s *surface) Show() {
	if !s.closed {
		C.tg_surface_show(s.sid())
	}
}

func (s *surface) Hide() {
	if !s.closed {
		C.tg_surface_hide(s.sid())
	}
}

// Close takes the page off screen, releases its web view and reports
// SurfaceClosed. iOS has no user gesture that closes a page, so ShouldClose
// is never asked.
func (s *surface) Close() {
	if s.closed {
		return
	}
	s.closed = true
	if s.cv != nil {
		s.cv.destroy()
	}
	C.tg_surface_close(s.sid())
	delete(s.b.surfaces, s.id)
	s.b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceClosed})
}

func (s *surface) SetTitle(title string) {
	if s.closed {
		return
	}
	t, free := cstring(title)
	defer free()
	C.tg_surface_set_title(s.sid(), t)
}

// SetSize is ignored: pages always fill the screen.
func (s *surface) SetSize(width, height int) {}

func (s *surface) Size() (int, int) {
	if s.closed {
		return 0, 0
	}
	var w, h C.int
	C.tg_surface_size(s.sid(), &w, &h)
	return int(w), int(h)
}

// Focus is a no-op: the topmost shown page has focus.
func (s *surface) Focus() {}

func (s *surface) Navigate(url string) {
	if s.closed {
		return
	}
	u, free := cstring(url)
	defer free()
	C.tg_surface_navigate(s.sid(), u)
}

func (s *surface) Eval(js string) {
	if s.closed {
		return
	}
	j, free := cstring(js)
	defer free()
	C.tg_surface_eval(s.sid(), j)
}

// OpenDevTools is a no-op: with Debug the page is inspectable from Safari's
// Develop menu on the Mac.
func (s *surface) OpenDevTools() {}

// Native returns the page's UIViewController*.
func (s *surface) Native() uintptr {
	if s.closed {
		return 0
	}
	return uintptr(C.tg_surface_native(s.sid()))
}

// live returns the open surface with id, or nil.
func live(id C.uint64_t) (*backend, *surface) {
	b := current.Load()
	if b == nil || !b.ready.Load() {
		return nil, nil
	}
	s := b.surfaces[platform.SurfaceID(id)]
	if s == nil || s.closed {
		return b, nil
	}
	return b, s
}

//export tangoIOSLoaded
func tangoIOSLoaded(id C.uint64_t, url *C.char) {
	if b, s := live(id); s != nil {
		b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceLoaded, URL: C.GoString(url)})
	}
}

//export tangoIOSResized
func tangoIOSResized(id C.uint64_t, w, h C.int) {
	if b, s := live(id); s != nil {
		b.h.OnSurfaceEvent(s.id, platform.SurfaceEvent{Kind: platform.SurfaceResized, Width: int(w), Height: int(h)})
	}
}

//export tangoIOSMessage
func tangoIOSMessage(id C.uint64_t, origin, msg *C.char, n C.int) {
	if b, s := live(id); s != nil {
		b.h.OnMessage(s.id, C.GoString(origin), C.GoStringN(msg, n))
	}
}
