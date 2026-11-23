package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// drawPopupArrow draws a triangular arrow on the side of panel that faces
// anchor, pointing at the anchor's centre (clamped inside the panel).
// A non-zero border colour outlines the two outer edges (Element).
func drawPopupArrow(l *paint.List, panel, anchor geom.Rect, bg, border theme.Color) {
	const half, h = 7, 7
	p := paint.NewPath()
	edge := paint.NewPath()
	switch {
	case panel.Bottom() <= anchor.Y+1: // panel above: arrow at bottom
		cx := max(panel.X+12+half, min(anchor.X+anchor.W/2, panel.Right()-12-half))
		y := panel.Bottom() - 1
		p.MoveTo(cx-half-1, y).LineTo(cx, y+h).LineTo(cx+half+1, y).Close()
		edge.MoveTo(cx-half, y+0.5).LineTo(cx, y+h).LineTo(cx+half, y+0.5)
	case panel.Y >= anchor.Bottom()-1: // below: arrow at top
		cx := max(panel.X+12+half, min(anchor.X+anchor.W/2, panel.Right()-12-half))
		y := panel.Y + 1
		p.MoveTo(cx-half-1, y).LineTo(cx, y-h).LineTo(cx+half+1, y).Close()
		edge.MoveTo(cx-half, y-0.5).LineTo(cx, y-h).LineTo(cx+half, y-0.5)
	case panel.Right() <= anchor.X+1: // left of anchor: arrow on the right
		cy := max(panel.Y+12+half, min(anchor.Y+anchor.H/2, panel.Bottom()-12-half))
		x := panel.Right() - 1
		p.MoveTo(x, cy-half-1).LineTo(x+h, cy).LineTo(x, cy+half+1).Close()
		edge.MoveTo(x+0.5, cy-half).LineTo(x+h, cy).LineTo(x+0.5, cy+half)
	default: // right of anchor: arrow on the left
		cy := max(panel.Y+12+half, min(anchor.Y+anchor.H/2, panel.Bottom()-12-half))
		x := panel.X + 1
		p.MoveTo(x, cy-half-1).LineTo(x-h, cy).LineTo(x, cy+half+1).Close()
		edge.MoveTo(x-0.5, cy-half).LineTo(x-h, cy).LineTo(x-0.5, cy+half)
	}
	l.Path(p, bg)
	if border.A > 0 {
		l.StrokePath(edge, paint.StrokeStyle{Width: 1}, border)
	}
}

func findOverlay(c *Context, id ID) *overlay {
	for i := len(c.overlays) - 1; i >= 0; i-- {
		if o := c.overlays[i]; o.root.id == id {
			return o
		}
	}
	return nil
}

// elementPopupArrow follows the placement after edge flipping and shifts.
// Element Plus popper.scss paints a 10px square rotated by 45 degrees,
// half outside the panel, and Popper's arrow modifier uses 5px padding.
func elementPopupArrow(c *Context, id ID, bg, border theme.Color) {
	if o := findOverlay(c, id); o != nil {
		prev := o.root.over
		o.root.over = func(l *paint.List, panel geom.Rect) {
			if prev != nil {
				prev(l, panel)
			}
			drawElementPopupArrow(l, panel, o.anchor, o.placed, bg, border)
		}
	}
}

// elementPopupZoom follows theme-chalk's zoom-in-top, including the
// bottom origin when Popper has flipped the overlay above its anchor.
func elementPopupZoom(c *Context, id ID, progress float32) {
	if progress >= 1 {
		return
	}
	if o := findOverlay(c, id); o != nil {
		o.root.xf = func(r geom.Rect) geom.Affine {
			y := r.Y
			if o.placed == Top || o.placed == TopStart || o.placed == TopEnd {
				y = r.Bottom()
			}
			return geom.ScaleAround(1, progress, geom.Pt(r.Center().X, y))
		}
	}
}

func drawElementPopupArrow(l *paint.List, panel, anchor geom.Rect, place Placement, bg, border theme.Color) {
	const half = float32(7.071068) // half the diagonal of the 10px square
	const inset = 5 + 5            // Popper arrow padding + half the square
	clamp := func(v, start, length float32) float32 {
		pad := min(inset, length/2)
		return max(start+pad, min(v, start+length-pad))
	}
	x := clamp(anchor.X+anchor.W/2, panel.X, panel.W)
	y := clamp(anchor.Y+anchor.H/2, panel.Y, panel.H)
	var a, b, d geom.Point
	switch place {
	case TopStart, TopEnd, Top:
		y = panel.Bottom() - 0.5
		a, b, d = geom.Pt(x-half, y), geom.Pt(x, y+half), geom.Pt(x+half, y)
	case BottomStart, BottomEnd, Bottom:
		y = panel.Y + 0.5
		a, b, d = geom.Pt(x-half, y), geom.Pt(x, y-half), geom.Pt(x+half, y)
	case LeftStart, LeftEnd, Left:
		x = panel.Right() - 0.5
		a, b, d = geom.Pt(x, y-half), geom.Pt(x+half, y), geom.Pt(x, y+half)
	case RightStart, RightEnd, Right:
		x = panel.X + 0.5
		a, b, d = geom.Pt(x, y-half), geom.Pt(x-half, y), geom.Pt(x, y+half)
	default:
		return
	}
	// The corner radius is 2px in the arrow's unrotated square.
	const corner = 1.414214
	near := func(p geom.Point) geom.Point {
		return geom.Pt(b.X+(p.X-b.X)*corner/half, b.Y+(p.Y-b.Y)*corner/half)
	}
	before, after := near(a), near(d)
	edge := paint.NewPath().MoveTo(a.X, a.Y).LineTo(before.X, before.Y).
		QuadTo(b.X, b.Y, after.X, after.Y).LineTo(d.X, d.Y)
	l.Path(edge, bg) // path fill closes the edge against the panel
	l.StrokePath(edge, paint.StrokeStyle{Width: 1}, border)
}
