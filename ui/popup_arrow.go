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
