package platform

import (
	"image"
	"math"
)

// WebViews embedded in a canvas surface's layout (docs/specs/webview-embed.md).
// Everything here is optional; the core discovers support with a type
// assertion on the Backend.

// WebViewEmbedder is implemented by backends that can place a WebView as
// a child view inside the content area of another surface (a canvas
// surface from CanvasCreator), so that a self-drawn layout can host it.
type WebViewEmbedder interface {
	// NewEmbeddedWebView creates a WebView inside the content area of
	// parent. id is a new SurfaceID allocated by the core: messages
	// (Host.OnMessage), asset requests (Host.ServeAsset), surface events
	// (SurfaceLoaded, SurfaceLoadFailed, SurfaceClosed, and
	// SurfaceFocused / SurfaceBlurred when the web view gains or loses
	// keyboard focus inside its window) and the WebHost decisions are
	// reported for id exactly as for a WebView surface.
	//
	// opts.URL, Debug and BridgeScript apply as for NewSurface; the
	// window fields (title, size, visibility, ...) are ignored. web
	// carries the creation-time user agent and headers (WebCreator).
	//
	// The view is created hidden: it shows only after a SetPlacement
	// with a non-empty Clip. It stacks above the parent's content (the
	// canvas); views created later stack above earlier ones. Closing the
	// parent closes its embedded views first, each reporting
	// SurfaceClosed. Main thread.
	NewEmbeddedWebView(id, parent SurfaceID, opts SurfaceOptions, web WebOptions) (EmbeddedWebView, error)
}

// EmbeddedWebView is a WebView living inside another surface.
//
// Its Surface methods: Navigate, Eval and OpenDevTools work as for a
// WebView surface. Show and Hide set an application-level hidden flag
// that combines with the placement (hidden wins). Close removes and
// destroys the view and reports SurfaceClosed. SetTitle and SetSize do
// nothing. Size reports the frame size of the last placement, rounded.
// Focus is FocusWeb. Native returns the platform web view object
// (WKWebView*, ICoreWebView2Controller*, WebKitWebView*).
//
// The value also implements the optional WebView interfaces
// (webview_ext.go: WebFinder, WebSnapshot, WebCookies, ...) that the
// backend's WebView surfaces implement, but none of the window
// interfaces (Geometry, Stateful, Styled, ...).
type EmbeddedWebView interface {
	Surface
	// SetPlacement moves, resizes and clips the view. Main thread; cheap
	// when nothing changed, so callers may call it every frame.
	SetPlacement(p WebPlacement)
	// FocusWeb gives keyboard focus to the web view (first responder,
	// MoveFocus, gtk_widget_grab_focus).
	FocusWeb()
	// BlurWeb gives keyboard focus back to the parent's content view
	// (the canvas) if the web view has it; otherwise it does nothing.
	BlurWeb()
}

// WebPlacement places an embedded WebView. Coordinates are logical pixels
// relative to the top-left corner of the parent's content area.
type WebPlacement struct {
	// Frame is the whole web view: the page's viewport has Frame's size.
	Frame RectF
	// Clip is the visible part of Frame (ancestors clipping, scrolling);
	// it is intersected with Frame. An empty Clip hides the view.
	Clip RectF
}

// RectF is a rectangle in logical pixels.
type RectF struct{ X, Y, W, H float64 }

// Empty reports whether r has no area.
func (r RectF) Empty() bool { return !(r.W > 0 && r.H > 0) }

// Intersect returns the common part of r and o (zero when they do not
// overlap).
func (r RectF) Intersect(o RectF) RectF {
	x0, y0 := math.Max(r.X, o.X), math.Max(r.Y, o.Y)
	x1, y1 := math.Min(r.X+r.W, o.X+o.W), math.Min(r.Y+r.H, o.Y+o.H)
	if x1 <= x0 || y1 <= y0 {
		return RectF{}
	}
	return RectF{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// EmbedGeometry is a placement converted for a "clip container"
// implementation: a native container sized to the visible part, whose
// child — the web view — is offset so that only the visible part shows.
type EmbedGeometry struct {
	// Visible is false when nothing of the view shows (empty clip or
	// hidden); the other fields are then zero.
	Visible bool
	// Clip is the container rectangle relative to the parent's content
	// area, in the units requested (device pixels for DeviceGeometry).
	Clip image.Rectangle
	// Frame is the web view rectangle relative to the container's
	// top-left corner (its origin is usually ≤ 0).
	Frame image.Rectangle
}

// Geometry converts p for a clip container at scale (device pixels per
// logical pixel; 1 for platforms that position in logical units). Edges
// are rounded to the nearest unit independently, so neighbouring views
// stay seamless. hidden forces Visible false.
func (p WebPlacement) Geometry(scale float64, hidden bool) EmbedGeometry {
	if scale <= 0 {
		scale = 1
	}
	clip := p.Clip.Intersect(p.Frame)
	if hidden || clip.Empty() || p.Frame.Empty() {
		return EmbedGeometry{}
	}
	snap := func(r RectF) image.Rectangle {
		return image.Rect(
			int(math.Round(r.X*scale)), int(math.Round(r.Y*scale)),
			int(math.Round((r.X+r.W)*scale)), int(math.Round((r.Y+r.H)*scale)))
	}
	c, f := snap(clip), snap(p.Frame)
	if c.Empty() || f.Empty() {
		return EmbedGeometry{}
	}
	return EmbedGeometry{Visible: true, Clip: c, Frame: f.Sub(c.Min)}
}

// Size returns the frame size rounded to whole logical pixels, for
// Surface.Size.
func (p WebPlacement) Size() (w, h int) {
	return int(math.Round(p.Frame.W)), int(math.Round(p.Frame.H))
}
