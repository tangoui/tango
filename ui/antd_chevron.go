package ui

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// antd draws a few of its chevrons with CSS instead of an icon font: the
// glyph is a box with two borders (an L) rotated to point somewhere, or two
// bars. Those are bolder than an Outlined icon scaled to the same size, so
// the Ant flavor draws them the same way here rather than shrinking
// icon.Down / icon.Left.

// chevronDir is the way a chevron points (physical, not logical).
type chevronDir uint8

const (
	chevronLeft chevronDir = iota
	chevronRight
	chevronUp
	chevronDown
)

// chevronAngle is the rotation (radians, screen y down) that turns an L
// with its corner at the box's top-left — the un-rotated antd border box
// (border-block-start + border-inline-start) — so that the corner points
// d. The corner of that L points up-left; rotate(-45deg) makes it point
// left (antd prev), 135deg right (next), 45deg up and -135deg down
// (carousel vertical prev/next).
func chevronAngle(d chevronDir) float64 {
	switch d {
	case chevronRight:
		return 3 * math.Pi / 4
	case chevronUp:
		return math.Pi / 4
	case chevronDown:
		return -3 * math.Pi / 4
	default:
		return -math.Pi / 4
	}
}

// antdCSSChevron paints antd's CSS border chevron centered on c: an L of
// two strokes `size` long and `width` thick (square ends, as CSS borders)
// rotated about the box center to point d. The DatePicker header arrows
// are size 7, width 1.5 (date-picker/style/token.js pickerControlIcon*);
// the Carousel arrows are size arrowSize/√2, width 2 (carousel/style).
func antdCSSChevron(l *paint.List, c geom.Point, size, width float32, d chevronDir, col theme.Color) {
	if col.A == 0 {
		return
	}
	ang := chevronAngle(d)
	cs, sn := float32(math.Cos(ang)), float32(math.Sin(ang))
	// The box spans [-s/2, s/2]; the L is its top edge and left edge, each
	// drawn inside the box as a border is (border-box sizing).
	h := size / 2
	rot := func(x, y float32) (float32, float32) {
		return c.X + x*cs - y*sn, c.Y + x*sn + y*cs
	}
	// One outline for the whole L: outer corner, along the top, back along
	// the inner edge, down the left. A single filled polygon keeps a
	// translucent color from doubling where the strokes meet.
	pts := [6][2]float32{{-h, -h}, {h, -h}, {h, -h + width}, {-h + width, -h + width}, {-h + width, h}, {-h, h}}
	p := paint.NewPath()
	for i, q := range pts {
		x, y := rot(q[0], q[1])
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	p.Close()
	l.Path(p, col)
}

// antdPickerArrow paints a DatePicker header arrow centered in r: a single
// chevron (prev / next month) or, for super prev / next (year, decade),
// a second one offset pickerControlIconMargin along both box axes before
// the rotation, as the ::after border box. ltr is false in RTL, where the
// arrows point the other way.
func antdPickerArrow(l *paint.List, r geom.Rect, next, double, rtl bool, col theme.Color) {
	const size, width, margin = 7, 1.5, 4
	d := chevronLeft
	if next != rtl {
		d = chevronRight
	}
	c := geom.Pt(r.X+r.W/2, r.Y+r.H/2)
	if !double {
		antdCSSChevron(l, c, size, width, d, col)
		return
	}
	// The ::before box sits at the icon's origin and ::after at (4, 4)
	// inside the same rotated frame; centre the pair on r. In the rotated
	// frame the offset (m, m) runs along the pointing axis.
	ang := chevronAngle(d)
	dx := float32(margin) / 2 * float32(math.Cos(ang)-math.Sin(ang))
	dy := float32(margin) / 2 * float32(math.Sin(ang)+math.Cos(ang))
	antdCSSChevron(l, geom.Pt(c.X-dx, c.Y-dy), size, width, d, col)
	antdCSSChevron(l, geom.Pt(c.X+dx, c.Y+dy), size, width, d, col)
}

// antdSubmenuArrow draws antd's inline submenu arrow (menu/style index.js
// genSubMenuArrowStyle): two rounded bars 0.6·size long and 0.15·size
// thick forming a chevron, size = fontSize·5/7, in a size-wide box inset
// token.margin from the end edge. Closed (t=0) it points down, open (t=1)
// the bars swap angles to point up and the arrow lifts 0.2·size; dir
// chevronRight (end, in the item's direction) is the popup / vertical
// submenu arrow, which does not animate.
func antdSubmenuArrow(l *paint.List, s inlineSpan, r geom.Rect, th *theme.Theme, col theme.Color, t float32, dir chevronDir) {
	size := th.FontSize * 5 / 7
	bar, thick := size*0.6, size*0.15
	// insetInlineEnd: token.margin, measured from the end edge.
	cx := s.X(r.W-th.Margin-size, size) + size/2
	cy := r.Y + r.H/2
	p := paint.NewPath()
	if dir == chevronRight || dir == chevronLeft {
		// Two bars meeting at a tip on the end side (">" in LTR, "<" in
		// RTL), the same bars as the down arrow turned a quarter: each is
		// bar long at 45°, their inner ends off·√2 apart vertically.
		sign := float32(1) // +1: the end edge is to the right
		if s.rtl {
			sign = -1
		}
		h := bar / float32(math.Sqrt2) // each bar's horizontal / vertical extent
		tipX := cx + sign*h/2
		p.MoveTo(tipX-sign*h, cy-h).LineTo(tipX, cy).LineTo(tipX-sign*h, cy+h)
		l.StrokePath(p, paint.StrokeStyle{Width: thick, Cap: paint.CapRound}, col)
		return
	}
	cy -= size * 0.2 * t
	// Down (t=0): the tip at the bottom, the bars rising to either side;
	// open (t=1) the tip moves to the top, the bars turning through flat
	// as antd's swap their rotate(±45deg). Each bar ends at the shared tip,
	// so the joint is one round corner.
	h := bar / float32(math.Sqrt2)
	v := h * (1 - 2*t) // + : tip below the ends (down), - : above (up)
	tipY := cy + v/2
	p.MoveTo(cx-h, tipY-v).LineTo(cx, tipY).LineTo(cx+h, tipY-v)
	l.StrokePath(p, paint.StrokeStyle{Width: thick, Cap: paint.CapRound}, col)
}
