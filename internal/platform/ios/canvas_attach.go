//go:build ios

package ios

// Canvases attached to WebView pages (docs/specs/mobile-canvas.md §1.4,
// CanvasAttacher → uihost.Attach). The overlay is a TGCanvasOverlay — a
// TGCanvasView — inserted in the page's view above or below its
// WKWebView (canvas_attach.m), so it shows, hides, rotates and leaves
// with the page. Frame scheduling, touches, keys, text input, pointer and
// accessibility are those of canvas pages (canvas.go and friends); this
// file adds placement and pass-through.
//
//   - Placement: Bounds are points from the page view's top-left; zero
//     Bounds fill it. The autoresizing mask keeps a frame at its anchor
//     while UIKit resizes the page; the page's layout pass then places
//     every anchored overlay exactly (CanvasLayer.AnchoredBounds from the
//     size its Bounds were given for).
//   - Pass-through (Above only, as on the Mac): the overlay keeps the
//     alpha of the last presented frame; its hitTest: answers nil over
//     pixels below CanvasLayer.HitThreshold, so UIKit gives touches,
//     scrolls and the pointer there to the web view.
//   - Below the web view the page must be see-through: the page view, the
//     web view's scroll view and its under-page area turn clear while a
//     canvas is attached below (the web view is never opaque); the page's
//     CSS must not paint a background either.
//   - GPU: none. Overlays present CPU frames only, as on the desktop
//     (UseMetalLayer returns ErrUnsupported), since they are usually
//     transparent and need the frame's alpha for hit testing.
//   - Focus: a touch on the overlay makes it first responder (CanvasFocus
//     true, hardware keys and text input go to it); when the web view
//     takes first responder again (a tap into a web form field) the
//     overlay reports CanvasFocus false. A tap on the page outside form
//     fields leaves focus where it was: WebKit does not take first
//     responder for it.

/*
#include "canvas.h"
#include "canvas_attach.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"image"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

var (
	_ platform.CanvasAttacher = (*backend)(nil)
	_ platform.AttachedCanvas = (*overlay)(nil)
)

var errDetached = errors.New("tango/ios: canvas detached")

// overlay is a canvas attached to a WebView page. Main thread only.
type overlay struct {
	*canvas
	layer platform.CanvasLayer

	// Page size the layer's Bounds were given for (Attach or SetLayer).
	refW, refH int

	mask hitMask // alpha of the last frame, while pass-through applies
}

// AttachCanvas implements platform.CanvasAttacher.
func (b *backend) AttachCanvas(sid platform.SurfaceID, layer platform.CanvasLayer, sink platform.CanvasSink) (platform.Canvas, error) {
	if sink == nil {
		return nil, errors.New("tango/ios: AttachCanvas: nil sink")
	}
	s := b.surfaces[sid]
	if s == nil || s.closed {
		return nil, fmt.Errorf("tango/ios: AttachCanvas: no open surface %d", sid)
	}
	if s.cv != nil {
		return nil, errors.New("tango/ios: AttachCanvas: not a WebView surface")
	}
	var pw, ph C.double
	if C.tgc_page_size(s.sid(), &pw, &ph) == 0 {
		return nil, errors.New("tango/ios: AttachCanvas: not a WebView surface")
	}
	canvasSeq++
	c := &canvas{h: canvasSeq, s: s, sink: sink, keys: map[uint16]input.KeyEvent{}}
	o := &overlay{canvas: c, layer: layer, refW: roundPt(pw), refH: roundPt(ph)}
	c.ov = o
	canvases[c.h] = c
	f := placeOverlay(layer, o.refW, o.refH, o.refW, o.refH)
	if C.tgc_attach(c.handle(), s.sid(), cbool(layer.Above), cbool(o.passThrough()),
		C.double(f.X), C.double(f.Y), C.double(f.W), C.double(f.H), C.int(f.Mask)) == 0 {
		delete(canvases, c.h)
		return nil, errors.New("tango/ios: AttachCanvas: cannot place the canvas")
	}
	s.overlays = append(s.overlays, o)
	if !layer.Above {
		s.refreshUnderlay()
	}
	c.readGeometry()
	if C.tgc_in_background() != 0 {
		c.apply(c.sched.Background())
	}
	// As for NewCanvasSurface, the sink hears nothing before we return:
	// the first frame and the focus report come from the wake source.
	c.ready = true
	c.wantFocus = C.tgc_focused(c.handle()) != 0
	c.Invalidate()
	startPointerSelftest(c)
	c.a11yStart()
	return o, nil
}

func roundPt(v C.double) int { return int(float64(v) + 0.5) }

func (o *overlay) passThrough() bool { return o.layer.Above && o.layer.PassThrough }

// takesPoint reports whether the overlay takes pointer input at (x, y),
// view coordinates in points.
func (o *overlay) takesPoint(x, y float64) bool {
	return !o.passThrough() || o.mask.opaqueAt(x, y, o.layer.HitThreshold())
}

// Present implements platform.Canvas (CPU frames only) and keeps the
// frame's alpha for hit testing.
func (o *overlay) Present(img *image.RGBA) error {
	if o.closed {
		return errDetached
	}
	if img == nil {
		return errors.New("tango/ios: Present: nil image (attached canvases have no GPU path)")
	}
	if err := o.canvas.Present(img); err != nil {
		return err
	}
	if o.passThrough() {
		o.mask.set(img, o.scale)
	} else if !o.mask.empty() {
		o.mask.reset()
	}
	return nil
}

// UseMetalLayer is not available on attached canvases.
func (o *overlay) UseMetalLayer() (uintptr, error) {
	return 0, fmt.Errorf("tango/ios: UseMetalLayer on an attached canvas: %w", platform.ErrUnsupported)
}

// SetLayer implements platform.AttachedCanvas.
func (o *overlay) SetLayer(layer platform.CanvasLayer) error {
	if o.closed {
		return errDetached
	}
	old := o.layer
	o.layer = layer
	var pw, ph C.double
	if C.tgc_page_size(o.s.sid(), &pw, &ph) != 0 {
		o.refW, o.refH = roundPt(pw), roundPt(ph)
	}
	o.place()
	if layer.Above != old.Above {
		o.s.refreshUnderlay()
	}
	if o.passThrough() && o.mask.empty() {
		o.Invalidate() // a mask arrives with the next frame
	}
	return nil
}

// place puts the view where the layer says for the page's current size.
func (o *overlay) place() {
	var pw, ph C.double
	if C.tgc_page_size(o.s.sid(), &pw, &ph) == 0 {
		return
	}
	f := placeOverlay(o.layer, o.refW, o.refH, roundPt(pw), roundPt(ph))
	C.tgc_overlay_place(o.handle(), cbool(o.layer.Above), cbool(o.passThrough()),
		C.double(f.X), C.double(f.Y), C.double(f.W), C.double(f.H), C.int(f.Mask))
}

// Detach implements platform.AttachedCanvas.
func (o *overlay) Detach() {
	if o.closed {
		return
	}
	s := o.s
	o.destroyOverlay()
	for i, x := range s.overlays {
		if x == o {
			s.overlays = append(s.overlays[:i], s.overlays[i+1:]...)
			break
		}
	}
	if !o.layer.Above && !s.closed {
		s.refreshUnderlay()
	}
}

func (o *overlay) destroyOverlay() {
	o.mask.reset()
	o.canvas.destroy() // removes the view (tgc_destroy)
}

// destroyOverlays runs from surface.Close, before the page goes.
func (s *surface) destroyOverlays() {
	for _, o := range s.overlays {
		o.destroyOverlay()
	}
	s.overlays = nil
}

// refreshUnderlay makes the page see-through while a canvas is attached
// below its web view.
func (s *surface) refreshUnderlay() {
	under := 0
	for _, o := range s.overlays {
		if !o.layer.Above && !o.closed {
			under = 1
			break
		}
	}
	C.tgc_page_underlay(s.sid(), C.int(under))
}

//export tangoIOSOverlayHit
func tangoIOSOverlayHit(h C.uintptr_t, x, y C.double) C.int {
	c := lookup(h)
	if c == nil || c.ov == nil || c.ov.takesPoint(float64(x), float64(y)) {
		return 1
	}
	return 0
}

// tangoIOSPageLayout runs from the page's viewDidLayoutSubviews: anchored
// overlays go exactly where AnchoredBounds puts them (autoresizing only
// approximates AnchorCenter).
//
//export tangoIOSPageLayout
func tangoIOSPageLayout(sid C.uint64_t, w, h C.double) {
	b := current.Load()
	if b == nil {
		return
	}
	s := b.surfaces[platform.SurfaceID(sid)]
	if s == nil || s.closed {
		return
	}
	for _, o := range s.overlays {
		if !o.closed && o.layer.Bounds.Width > 0 && o.layer.Bounds.Height > 0 {
			o.place()
		}
	}
}
