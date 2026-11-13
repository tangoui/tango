package ios

// Pure parts of attached canvases and iPad pointer input
// (docs/specs/mobile-canvas.md §1.4, §2.1): overlay placement in the page
// view, the pass-through alpha mask, trackpad and wheel scroll deltas and
// the pointer shape for a cursor. No cgo, so they are tested on any host.

import (
	"image"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

// UIViewAutoresizing bits.
const (
	uiResizeFlexLeft   = 1 << 0
	uiResizeFlexWidth  = 1 << 1
	uiResizeFlexRight  = 1 << 2
	uiResizeFlexTop    = 1 << 3
	uiResizeFlexHeight = 1 << 4
	uiResizeFlexBottom = 1 << 5
)

// overlayFrame is where an overlay view goes in its page view (points,
// top-left origin, as UIKit's coordinates are) and the autoresizing mask
// that keeps it there while UIKit resizes the page.
type overlayFrame struct {
	X, Y, W, H float64
	Mask       int
}

// placeOverlay places layer in a page view of w×h points, its Bounds
// having been given for a page of refW×refH (CanvasLayer.AnchoredBounds).
// Zero Bounds fill the page. The mask keeps anchored frames at their
// corner during a rotation; for AnchorCenter UIKit would split the change
// in proportion to the margins, so the page's layout pass places every
// anchored overlay exactly again (tangoIOSPageLayout).
func placeOverlay(l platform.CanvasLayer, refW, refH, w, h int) overlayFrame {
	r := l.Bounds
	if r.Width <= 0 || r.Height <= 0 {
		return overlayFrame{W: float64(w), H: float64(h), Mask: uiResizeFlexWidth | uiResizeFlexHeight}
	}
	r = l.AnchoredBounds(refW, refH, w, h)
	return overlayFrame{X: float64(r.X), Y: float64(r.Y), W: float64(r.Width), H: float64(r.Height), Mask: anchorMask(l.Anchor)}
}

// anchorMask is the autoresizing mask that keeps a fixed-size frame at
// anchor a: the margins away from it are flexible.
func anchorMask(a platform.CanvasAnchor) int {
	switch a {
	case platform.AnchorTopRight:
		return uiResizeFlexLeft | uiResizeFlexBottom
	case platform.AnchorBottomLeft:
		return uiResizeFlexRight | uiResizeFlexTop
	case platform.AnchorBottomRight:
		return uiResizeFlexLeft | uiResizeFlexTop
	case platform.AnchorCenter:
		return uiResizeFlexLeft | uiResizeFlexRight | uiResizeFlexTop | uiResizeFlexBottom
	}
	return uiResizeFlexRight | uiResizeFlexBottom // AnchorTopLeft
}

// hitMask keeps the alpha channel of the last frame an overlay presented,
// for pass-through hit testing. The zero value is empty: nothing is
// opaque.
type hitMask struct {
	a     []byte
	w, h  int
	scale float64 // device pixels per point
}

// set copies the alpha of img (scale device pixels per point).
func (m *hitMask) set(img *image.RGBA, scale float32) {
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		m.reset()
		return
	}
	if cap(m.a) < w*h {
		m.a = make([]byte, w*h)
	}
	m.a = m.a[:w*h]
	for y := range h {
		row := img.Pix[img.PixOffset(r.Min.X, r.Min.Y+y):]
		dst := m.a[y*w : (y+1)*w]
		for x := range dst {
			dst[x] = row[x*4+3]
		}
	}
	m.w, m.h, m.scale = w, h, float64(max(scale, 1e-3))
}

// reset empties the mask and frees its buffer.
func (m *hitMask) reset() { *m = hitMask{} }

func (m *hitMask) empty() bool { return m.w == 0 }

// opaqueAt reports whether the pixel under (x, y) (points, view
// coordinates) has alpha of at least threshold. Points outside the frame
// and an empty mask are transparent.
func (m *hitMask) opaqueAt(x, y float64, threshold uint8) bool {
	if m.w == 0 || x < 0 || y < 0 {
		return false
	}
	px, py := int(x*m.scale), int(y*m.scale)
	if px >= m.w || py >= m.h {
		return false
	}
	return m.a[py*m.w+px] >= threshold
}

// scrollLine is the logical pixels one notch of a mouse wheel scrolls, as
// on the Mac (darwin scrollLine).
const scrollLine = 40

// scrollDelta converts one update of a scroll pan gesture recognizer —
// its translation since the previous update, in points — to input.Scroll
// deltas. UIKit's translation follows the content (two fingers moving
// down drag the content down, revealing what is above), input.Scroll's
// positive DY reveals what is below, hence the sign. Continuous
// (trackpad) translations are precise points; a discrete one (a mouse
// wheel) is one notch per update and per axis, whatever distance UIKit
// gave it, so it scrolls scrollLine.
func scrollDelta(tx, ty float64, discrete bool) (dx, dy float32) {
	if discrete {
		return float32(0-notch(tx)) * scrollLine, float32(0-notch(ty)) * scrollLine
	}
	return float32(0 - tx), float32(0 - ty) // 0-x: no negative zero
}

func notch(v float64) float64 {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// Pointer styles canvas_pointer.m knows (TGP_STYLE_* in canvas_pointer.h).
const (
	pointerSystem   = 0 // the system pointer (an arrow-less dot on iPad)
	pointerBeam     = 1 // vertical I-beam: text
	pointerResizeEW = 2 // horizontal double arrow
	pointerResizeNS = 3 // vertical double arrow
)

// pointerStyle chooses the iPad pointer shape for c. iPadOS has no cursor
// images: the pointer is a shape, and the hover effects of
// UIPointerEffect need a view per control, which a self-drawn UI does not
// have. So text gets the beam, the resize cursors a drawn double arrow,
// and everything else (including CursorPointer over links and buttons)
// the system pointer.
func pointerStyle(c input.Cursor) int {
	switch c {
	case input.CursorText:
		return pointerBeam
	case input.CursorResizeEW:
		return pointerResizeEW
	case input.CursorResizeNS:
		return pointerResizeNS
	}
	return pointerSystem
}

// Hover phases passed to tangoIOSCanvasHover (TGP_HOVER_* in
// canvas_pointer.h).
const (
	hoverEnter = 0
	hoverMove  = 1
	hoverLeave = 2
)

// hoverState tracks whether the pointer is over (the opaque part of) a
// canvas, so that a pass-through overlay reports Leave when the pointer
// moves onto a transparent pixel and Move again when it comes back.
type hoverState struct{ inside bool }

// event returns the pointer event for one hover update at pos, or false
// when there is none to report. opaque says whether pos takes pointer
// input (always true without pass-through).
func (s *hoverState) event(phase int, pos input.Pointer, opaque bool) (input.Pointer, bool) {
	pos.Source = input.SourceMouse
	if phase == hoverLeave || !opaque {
		if !s.inside {
			return input.Pointer{}, false
		}
		s.inside = false
		pos.Kind = input.Leave
		return pos, true
	}
	s.inside = true
	pos.Kind = input.Move
	return pos, true
}
