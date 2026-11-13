package uihost

// Self-drawn UI inside a WebView window (docs/specs/canvas-surface.md,
// step D): Attach places a canvas drawn by a ui.Runner above or below the
// WebView of an ordinary surface.

import (
	"errors"
	"fmt"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
)

// Layer places an attached canvas in its surface.
type Layer struct {
	// Bounds is the canvas rectangle relative to the top-left corner of
	// the content area, in logical pixels (X/Y are offsets here, not
	// screen coordinates). The zero Rect fills the content area and
	// follows its size; a non-zero Rect keeps its size and its distance
	// from the corner Anchor names when the window is resized (use
	// SetLayer from Surface.OnResized for other layouts).
	Bounds tango.Rect
	// Above stacks the canvas over the WebView. Below it (false) the
	// canvas shows only through transparent parts of the page: the
	// backend makes the WebView's own background transparent, and the
	// page must not paint one either (html, body { background:
	// transparent }). A canvas below the WebView gets no pointer input
	// where the WebView covers it.
	Above bool
	// PassThrough sends pointer events over fully transparent canvas
	// pixels (alpha 0 in the last presented frame) to the WebView below.
	// Only meaningful with Above.
	PassThrough bool
	// HitAlpha is the PassThrough threshold: a pixel takes pointer input
	// only if its alpha is at least HitAlpha. 0 means 1 (any
	// non-transparent pixel); 32 lets soft shadows pass clicks through.
	HitAlpha uint8
	// Anchor keeps non-zero Bounds at a fixed distance from a corner (or
	// the centre) of the content area as the window is resized. Bounds
	// are still given from the top-left for the current size; the zero
	// value is AnchorTopLeft.
	Anchor Anchor
}

// Anchor chooses what non-zero Layer.Bounds stay attached to when the
// window is resized.
type Anchor uint8

const (
	AnchorTopLeft     Anchor = Anchor(platform.AnchorTopLeft)
	AnchorTopRight    Anchor = Anchor(platform.AnchorTopRight)
	AnchorBottomLeft  Anchor = Anchor(platform.AnchorBottomLeft)
	AnchorBottomRight Anchor = Anchor(platform.AnchorBottomRight)
	AnchorCenter      Anchor = Anchor(platform.AnchorCenter)
)

func (l Layer) platform() platform.CanvasLayer {
	return platform.CanvasLayer{Bounds: l.Bounds, Above: l.Above, PassThrough: l.PassThrough,
		HitAlpha: l.HitAlpha, Anchor: platform.CanvasAnchor(l.Anchor)}
}

// ErrDetached is returned by Attached.SetLayer after Detach or after the
// surface closed.
var ErrDetached = errors.New("uihost: canvas detached")

// Attached is a canvas attached to a WebView surface. Its methods run on
// the main thread.
type Attached struct {
	s     *tango.Surface
	h     *host
	cv    platform.Canvas
	layer Layer
	stop  func() // stops following the system (follow.go)
}

// Attach draws page with a new ui.Runner on a canvas placed inside the
// WebView surface s as layer says, and returns the runner and a handle to
// move or remove the canvas. Input over the canvas goes to the runner
// (pointer, wheel; keyboard while the canvas has focus: clicking it
// focuses it, clicking the page gives focus back to the WebView). The
// runner is closed when s closes or on Detach.
//
// Attach must be called on the main thread (App.OnReady, App.Post, a
// surface callback, ...). It returns an error wrapping tango.ErrUnsupported
// when the platform cannot mix a canvas with a WebView, and under the hot
// reload host of tan dev (attached canvases are not forwarded to the dev
// host; use uihost.Open windows there).
func Attach(s *tango.Surface, layer Layer, page func(*ui.Context), ropts ui.RunnerOptions) (*ui.Runner, *Attached, error) {
	return AttachWith(s, layer, Options{}, page, ropts)
}

// AttachWith is Attach with the theme and locale options of Open
// (Options.Theme, Flavor, AccentFromSystem, Customize, Locale);
// opts.Surface is ignored. Attach follows the system like Open with zero
// Options: the canvas's theme switches between light and dark with the
// system, as the page under it does through prefers-color-scheme, unless
// ropts.Theme is set.
func AttachWith(s *tango.Surface, layer Layer, opts Options, page func(*ui.Context), ropts ui.RunnerOptions) (*ui.Runner, *Attached, error) {
	if s == nil {
		return nil, nil, errors.New("uihost: nil surface")
	}
	if devActive() {
		return nil, nil, fmt.Errorf("uihost: Attach under tan dev: %w", tango.ErrUnsupported)
	}
	ropts, fl := followSetup(s.App(), opts, ropts) // follow.go
	r := ui.NewRunner(page, ropts)
	h := &host{app: s.App(), r: r, id: s.ID()}
	cv, err := corehook.AttachCanvas(s, layer.platform(), h)
	if err != nil {
		return nil, nil, err
	}
	h.cv = cv
	h.ax = newA11y(s.App(), cv, r, ropts) // a11y.go
	h.win = &window{app: s.App(), cv: cv, ax: h.ax}
	r.Attach(h.win)
	at := &Attached{s: s, h: h, cv: cv, layer: layer}
	s.OnClosed(h.close)
	watchMemory(s.App(), r) // memory.go
	at.stop = fl.start(r, s)
	cv.Invalidate()
	return r, at, nil
}

// Surface returns the surface the canvas is attached to.
func (a *Attached) Surface() *tango.Surface { return a.s }

// Layer returns the canvas's current placement.
func (a *Attached) Layer() Layer { return a.layer }

// Size returns the canvas size in logical pixels and its device scale
// (0, 0, 0 once detached).
func (a *Attached) Size() (w, h, scale float32) {
	if a.h.closed {
		return 0, 0, 0
	}
	w, h = a.cv.Size()
	return w, h, a.cv.Scale()
}

// Detached reports whether the canvas is gone (Detach, or the surface
// closed).
func (a *Attached) Detached() bool { return a.h.closed }

// SetLayer moves the canvas. It returns ErrDetached after Detach and an
// error wrapping tango.ErrUnsupported when the backend cannot move an
// attached canvas.
func (a *Attached) SetLayer(layer Layer) error {
	if a.h.closed {
		return ErrDetached
	}
	ac, ok := a.cv.(platform.AttachedCanvas)
	if !ok {
		return fmt.Errorf("uihost: SetLayer: %w", tango.ErrUnsupported)
	}
	if err := ac.SetLayer(layer.platform()); err != nil {
		return err
	}
	a.layer = layer
	a.cv.Invalidate()
	return nil
}

// Detach removes the canvas from the surface and closes the runner. The
// surface stays open. Calling it again does nothing.
func (a *Attached) Detach() {
	if a.h.closed {
		return
	}
	if ac, ok := a.cv.(platform.AttachedCanvas); ok {
		ac.Detach()
	}
	a.h.close()
	a.stop()
}
