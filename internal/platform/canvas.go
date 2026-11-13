package platform

import (
	"image"
	"math"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Canvas surfaces: windows whose content is drawn by Tango's own UI (package
// ui) instead of a WebView. Everything here is optional; the core discovers
// support with type assertions. See docs/specs/canvas-surface.md.

// CanvasSink receives canvas callbacks. It is implemented by the core
// (package uihost); backends call it on the main thread only.
type CanvasSink interface {
	// CanvasFrame asks for a frame now: after Invalidate/InvalidateAt fire,
	// after a size or scale change, and when the canvas is first shown. The
	// sink renders and calls Canvas.Present before returning.
	CanvasFrame()
	// CanvasInput delivers input in logical pixels relative to the top-left
	// of the content area (see ui/window.go for event conventions).
	CanvasInput(ev input.Event)
	// CanvasResized reports a new logical size or device scale.
	CanvasResized(w, h, scale float32)
	// CanvasFocus reports keyboard focus gained or lost by the canvas.
	CanvasFocus(focused bool)
}

// Canvas is a drawing target supplied by a backend.
type Canvas interface {
	// Size is the drawable content size in logical pixels.
	Size() (w, h float32)
	// Scale is the device-pixel ratio.
	Scale() float32
	// Invalidate schedules one CanvasFrame soon (next vsync if possible).
	// Callable from any goroutine; calls coalesce.
	Invalidate()
	// InvalidateAt schedules a CanvasFrame no later than t. Callable from
	// any goroutine; an earlier pending time wins.
	InvalidateAt(t time.Time)
	// SetCursor sets the pointer shape over the canvas. Main thread.
	SetCursor(c input.Cursor)
	// SetTextInput enables or disables the platform text-input client (IME)
	// and positions its candidate window at caret (logical pixels). While
	// enabled, text arrives as input.TextInput / input.Composition. Main
	// thread.
	SetTextInput(enabled bool, caret geom.Rect)
	// Present displays a finished frame. img is premultiplied RGBA of
	// Size()*Scale() device pixels and is only valid during the call: copy
	// or upload it before returning. Main thread.
	Present(img *image.RGBA) error
}

// CanvasDamagePresenter is optionally implemented by a Canvas that can
// show a CPU frame by updating only the parts that changed since the last
// frame it presented (ui.DamagePresenter on the platform side; package
// uihost discovers it with a type assertion).
//
// img is the complete frame, with the same ownership and layout as for
// Present. dirty lists device-pixel rectangles within img.Bounds() that
// differ from the previous frame presented on this canvas (by Present or
// PresentDamage); an empty list means nothing changed. The canvas must
// keep its copy of the last frame for this to be correct: when it cannot
// (no previous frame, a different size, a GPU or fallback mode, a mode
// where retained content is not reliable) it presents img whole, exactly
// as Present does. Main thread. On error nothing may be on screen: the
// caller presents the whole frame with Present.
type CanvasDamagePresenter interface {
	PresentDamage(img *image.RGBA, dirty []image.Rectangle) error
}

// DamageRects prepares the dirty list of a PresentDamage call for a
// canvas: rectangles are clipped to bounds and empty ones dropped. full is
// true when updating the parts would not pay off (more than maxRects
// rectangles, or their total area above half of bounds): the caller then
// presents the whole frame.
func DamageRects(dirty []image.Rectangle, bounds image.Rectangle, maxRects int) (rects []image.Rectangle, full bool) {
	area := 0
	for _, r := range dirty {
		r = r.Intersect(bounds)
		if r.Empty() {
			continue
		}
		rects = append(rects, r)
		area += r.Dx() * r.Dy()
	}
	if len(rects) > maxRects || area*2 > bounds.Dx()*bounds.Dy() {
		return nil, true
	}
	return rects, false
}

// DamageToLogical converts a device-pixel rectangle to the smallest
// logical-pixel rectangle (integer coordinates) covering it at scale, for
// platforms whose invalidation APIs take logical units.
func DamageToLogical(r image.Rectangle, scale float64) image.Rectangle {
	if scale <= 0 {
		scale = 1
	}
	return image.Rect(
		int(math.Floor(float64(r.Min.X)/scale)), int(math.Floor(float64(r.Min.Y)/scale)),
		int(math.Ceil(float64(r.Max.X)/scale)), int(math.Ceil(float64(r.Max.Y)/scale)))
}

// CanvasCreator is implemented by backends that can create a Surface whose
// content is a Canvas. The returned Surface is a normal window: WebView-only
// methods (Navigate, Eval, OpenDevTools) are no-ops and the Surface's other
// optional interfaces (Geometry, Stateful, Styled, ...) behave as for a
// WebView surface. Host.OnSurfaceEvent and Host.ShouldClose are reported for
// id exactly as for NewSurface.
type CanvasCreator interface {
	NewCanvasSurface(id SurfaceID, opts SurfaceOptions, ex SurfaceExtras, sink CanvasSink) (Surface, Canvas, error)
}

// CanvasLayer places a canvas inside an existing WebView surface.
type CanvasLayer struct {
	Bounds      Rect // relative to the content area, logical pixels; zero = fill
	Above       bool // above the WebView (true) or below it (false; the page must be transparent)
	PassThrough bool // pointer events over transparent pixels go to the layer below

	// HitAlpha is the pass-through threshold: with PassThrough, a pixel
	// of the last presented frame takes pointer input only if its alpha
	// is at least HitAlpha. 0 means 1 (any non-transparent pixel); 32
	// lets soft shadows and anti-aliased fringes pass clicks through.
	HitAlpha uint8
	// Anchor keeps non-zero Bounds at a fixed distance from a corner (or
	// the centre) of the content area when the window is resized; X/Y of
	// Bounds are still given from the top-left for the current size.
	// Ignored for zero Bounds (fill).
	Anchor CanvasAnchor
}

// CanvasAnchor chooses which part of the content area non-zero
// CanvasLayer.Bounds stay attached to on resize.
type CanvasAnchor uint8

const (
	AnchorTopLeft CanvasAnchor = iota // zero value: offsets from the top-left stay fixed
	AnchorTopRight
	AnchorBottomLeft
	AnchorBottomRight
	AnchorCenter // the centre offset from the content area's centre stays fixed
)

// HitThreshold returns the effective HitAlpha (at least 1).
func (l CanvasLayer) HitThreshold() uint8 { return max(l.HitAlpha, 1) }

// AnchoredBounds returns where Bounds (given for a content area of
// oldW×oldH) sit in a content area of newW×newH under Anchor. Zero Bounds
// stay zero (fill).
func (l CanvasLayer) AnchoredBounds(oldW, oldH, newW, newH int) Rect {
	r := l.Bounds
	if r.Width <= 0 || r.Height <= 0 {
		return r
	}
	dx, dy := newW-oldW, newH-oldH
	switch l.Anchor {
	case AnchorTopRight:
		r.X += dx
	case AnchorBottomLeft:
		r.Y += dy
	case AnchorBottomRight:
		r.X += dx
		r.Y += dy
	case AnchorCenter:
		r.X += dx / 2
		r.Y += dy / 2
	}
	return r
}

// CanvasAttacher is implemented by backends that can mix a canvas with a
// WebView in one window (step D of the canvas spec).
type CanvasAttacher interface {
	AttachCanvas(id SurfaceID, layer CanvasLayer, sink CanvasSink) (Canvas, error)
}

// AttachedCanvas is implemented by the Canvas that CanvasAttacher returns
// when the backend can move or remove it later. Both methods run on the
// main thread.
type AttachedCanvas interface {
	Canvas
	// SetLayer moves the canvas: new bounds, stacking or pass-through. A
	// size change reports CanvasResized and asks for a frame.
	SetLayer(layer CanvasLayer) error
	// Detach removes the canvas from its surface. Afterwards the sink is
	// never called again, Invalidate does nothing and Present returns an
	// error. Closing the surface detaches its canvases implicitly (the
	// core learns of it from SurfaceClosed). Detach is idempotent.
	Detach()
}

// CanvasMetal is implemented by macOS canvases that can switch to a
// CAMetalLayer for GPU presentation. Discover it with a type assertion on
// the Canvas from CanvasCreator.NewCanvasSurface.
type CanvasMetal interface {
	// UseMetalLayer replaces the view's backing layer with a CAMetalLayer
	// once and returns it (a CAMetalLayer*, owned by the view: valid until
	// the surface closes; retain it to keep it longer). Calling it again
	// returns the same layer. Main thread; it never calls the sink itself
	// but schedules one CanvasFrame, so the first GPU frame follows without
	// an Invalidate.
	//
	// Frame scheduling is unchanged: CanvasFrame still comes only after
	// Invalidate/InvalidateAt, a size or scale change, or the switch
	// itself, at most once per display refresh, and a still window costs
	// no frames. Input, IME, cursor and focus behave as before.
	//
	// The platform keeps the layer's frame on the view's bounds, its
	// contentsScale on the backing scale factor (updated before
	// CanvasResized is reported), opaque set and implicit animations off.
	// Everything else is the caller's (the GPU presenter's): device,
	// pixelFormat, framebufferOnly, colorspace and drawableSize, which it
	// must set to Size()*Scale() device pixels on CanvasResized or at the
	// start of each CanvasFrame.
	//
	// After a successful call, Present(nil) means "the GPU presented
	// already" and returns nil. Present(img) still works as a CPU fallback
	// (e.g. when the GPU presenter cannot start): the image is shown in a
	// sublayer above the drawables, not blitted through Metal, and hidden
	// again by the next Present(nil).
	//
	// Returns ErrUnsupported for attached (CanvasAttacher) canvases, for
	// transparent canvases (Appearance.Transparent or a Material) and so
	// for PassThrough layers, and an error off the main thread or after
	// the surface closed.
	UseMetalLayer() (layer uintptr, err error)
}

// CanvasHWND is implemented by Windows canvases that can hand their child
// HWND to a GPU presenter (a DXGI flip-model swap chain). Discover it with
// a type assertion on the Canvas from CanvasCreator.NewCanvasSurface; it
// parallels CanvasMetal.
type CanvasHWND interface {
	// UseGPUWindow switches the canvas to GPU presentation once and
	// returns the canvas's own child HWND, which keeps receiving input,
	// IME, focus and accessibility queries. The HWND is owned by the
	// surface: valid until it closes; the caller must release its swap
	// chain before or when the surface closes. Calling it again returns
	// the same HWND. Main thread; it never calls the sink itself but
	// schedules one CanvasFrame, so the first GPU frame follows without an
	// Invalidate.
	//
	// Frame scheduling is unchanged: CanvasFrame still comes only after
	// Invalidate/InvalidateAt, a size or scale change (WM_SIZE and
	// WM_DPICHANGED_AFTERPARENT report CanvasResized and produce a frame
	// synchronously, so the presenter resizes its buffers at the start of
	// present, on the main thread), or the switch itself, at most once
	// per display refresh, and a still window costs no frames. Input,
	// IME, cursor, focus and DPI behave as before.
	//
	// After a successful call nothing is drawn with GDI on the HWND any
	// more: WM_PAINT only validates the window (BeginPaint/EndPaint) and
	// runs a CanvasFrame when one is due, never blitting a CPU frame.
	// Present(nil) means "the GPU presented already" and returns nil.
	// Present(img) still works as a CPU fallback (the GPU presenter cannot
	// start, a frame beyond a device limit, a lost device): the image is
	// shown in a sibling child window placed above the HWND, which passes
	// pointer input through to the canvas and never takes focus, and the
	// next Present(nil) hides it again.
	//
	// Returns ErrUnsupported for attached (CanvasAttacher) canvases,
	// layered ones and so PassThrough layers, and for transparent
	// canvases (Appearance.Transparent or a Material), and an error off
	// the main thread or after the surface closed.
	UseGPUWindow() (hwnd uintptr, err error)
}

// CanvasDropTarget is optionally implemented by a CanvasSink that wants
// drop feedback (a highlight) while files are dragged over its canvas.
// Backends call it only for canvases of surfaces created with
// SurfaceExtras.AcceptFiles, on the main thread. The drop itself is
// reported to the Host as SurfaceFilesDropped (X/Y in the surface's
// content coordinates), as for WebView surfaces.
type CanvasDropTarget interface {
	// CanvasDragOver reports files dragged over the canvas at (x, y),
	// logical pixels relative to the canvas's top-left; called when the
	// drag enters and as it moves.
	CanvasDragOver(x, y float32)
	// CanvasDragLeave reports that the drag left the canvas, was
	// cancelled or was dropped (it precedes SurfaceFilesDropped).
	CanvasDragLeave()
}

// Accessibility (docs/specs/a11y-bridge.md, docs/accessibility.md). A
// canvas draws its own widgets, so assistive technologies see nothing
// unless the backend exposes the semantics tree of the ui package
// (ui/a11y) through the native API: NSAccessibility, UI Automation,
// AT-SPI2. Both interfaces are optional; the core discovers them with type
// assertions and a backend without them costs nothing.

// CanvasAccessible is implemented by a Canvas (from CanvasCreator or
// CanvasAttacher) whose backend can expose an a11y.Tree.
//
// Activation. The core builds no tree while AccessibilityActive is false.
// When the backend detects an assistive technology (macOS: the view is
// first asked for its accessibility children, hit test or focused
// element; Windows: WM_GETOBJECT with UiaRootObjectId; AT-SPI:
// org.a11y.Status IsEnabled) it makes AccessibilityActive return true and
// invalidates the canvas; the next frame calls SetAccessibilityTree with
// the full tree, later frames that change it call
// UpdateAccessibilityNodes. When the backend decides the technology left
// it returns false again; the next frame calls SetAccessibilityTree(nil).
// Until the first tree arrives the backend answers queries with the
// canvas alone (no children) and announces the layout change when it
// does.
//
// Order and threading. All methods except AccessibilityActive run on
// the main thread, from inside CanvasFrame before Present. One frame calls, in this order: SetAccessibilityTree or
// UpdateAccessibilityNodes, then AccessibilityFocusChanged if the
// effective focus moved, then AccessibilityAnnounce zero or more times.
// The methods must not call the sink; native notifications may be posted
// at once or after the frame. After the surface closes or the canvas is
// detached the core calls none of them; the backend releases its native
// objects in its own teardown.
//
// Snapshots. A tree is immutable and stays valid after the call (the
// runner builds a new one every frame): keep it as the data source of the
// native objects and answer queries from it, never by calling back into
// the core. Node.Bounds are logical pixels relative to the canvas's
// top-left; convert them to screen coordinates when a query asks (window
// moves and scale changes then need no new tree). NodeIDs are unique
// within one canvas only: key native objects by (canvas, NodeID).
type CanvasAccessible interface {
	// AccessibilityActive reports whether an assistive technology uses
	// the canvas. Called at the start of every frame from the UI thread;
	// must be cheap (an atomic load).
	AccessibilityActive() bool
	// SetAccessibilityTree replaces the whole tree: the first tree after
	// activation (every node is new), or nil to drop all native objects
	// after deactivation.
	SetAccessibilityTree(tree *a11y.Tree)
	// UpdateAccessibilityNodes delivers the next snapshot. changed lists
	// added and modified nodes in depth-first order, then removed ones
	// (tree.Node returns nil for those), as ui.AccessibilityHost does. The
	// backend compares with the snapshot it kept to choose notifications
	// (value, name, selection, structure, destroyed).
	UpdateAccessibilityNodes(tree *a11y.Tree, changed []a11y.NodeID)
	// AccessibilityFocusChanged reports the node that now has keyboard
	// focus in the last tree: tree.Focus, or its ActiveDescendant when it
	// has one (already resolved). 0 means no node of the canvas has
	// focus. Backends post their focus notification only while the canvas
	// itself has keyboard focus.
	AccessibilityFocusChanged(id a11y.NodeID)
	// AccessibilityAnnounce asks for text to be spoken once. source is 0
	// for ui.Context.Announce and the root of a live region whose content
	// changed otherwise (the core computes live-region changes; it skips
	// them in the first tree after activation). live is the politeness:
	// LiveAssertive interrupts, LivePolite waits. Backends with native
	// live regions (UIA LiveRegionChanged) may raise that event on source
	// instead of speaking text.
	AccessibilityAnnounce(text string, live a11y.Live, source a11y.NodeID)
}

// CanvasAccessibleSink is optionally implemented by a CanvasSink (package
// uihost implements it) that serves CanvasAccessible. Backends call it on
// the main thread only, never from inside CanvasFrame; a request that
// arrives on another thread (a UIA or D-Bus worker) is posted to the main
// thread first.
type CanvasAccessibleSink interface {
	// AccessibilityAction performs act on node id of the last tree for
	// an assistive technology (press, increment, setValue with arg, ...).
	// It fails when the node is not in that tree or does not declare the
	// action; nil means accepted, not done: the widget handles it in a
	// following frame, which the sink requests. For a11y.Focus the backend
	// first gives the canvas keyboard focus (first responder, SetFocus,
	// gtk_widget_grab_focus).
	AccessibilityAction(id a11y.NodeID, act a11y.Action, arg string) error
}
