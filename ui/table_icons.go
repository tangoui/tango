package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// tblDrawSorter paints the up / down sort carets centred in r: the active
// direction in the primary color, the other one idle (or the hover color
// while the header is hovered). Sizes come from the Table token.
func tblDrawSorter(th *theme.Theme, tk *theme.TableToken, l *paint.List, r geom.Rect, order SortOrder, hov bool) {
	idle := tk.SorterIdleColor
	if hov && tk.SorterHoverColor.A > 0 {
		idle = tk.SorterHoverColor
	}
	up, down := idle, idle
	if order == SortAsc {
		up = th.Primary.Base
	} else if order == SortDesc {
		down = th.Primary.Base
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	hw, h := tk.SorterArrowWidth/2, tk.SorterArrowHeight
	g := tk.SorterGap / 2
	l.Path(paint.NewPath().MoveTo(cx, cy-g-h).LineTo(cx+hw, cy-g).LineTo(cx-hw, cy-g).Close(), up)
	l.Path(paint.NewPath().MoveTo(cx, cy+g+h).LineTo(cx-hw, cy+g).LineTo(cx+hw, cy+g).Close(), down)
}

// tblExpandSize is the drawn size of the expand / tree toggle.
func tblExpandSize(tk *theme.TableToken) float32 {
	s := tk.ExpandIconSize
	if tk.ExpandIconScale > 0 {
		s *= tk.ExpandIconScale
	}
	return s
}

// tblDrawExpand paints the expand / tree toggle: Ant's bordered plus/minus
// box (TableToken.ExpandIcon*) or, with ExpandIconArrow, Element's
// rotating arrow, which points toward the inline end when closed (left in
// RTL) and down when open.
func tblDrawExpand(tk *theme.TableToken, l *paint.List, r geom.Rect, open float32, hov, rtl bool) {
	col := tk.ExpandIconColor
	if hov {
		col = tk.ExpandIconHoverColor
	}
	if tk.ExpandIconArrow {
		s := float32(12)
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		k, ox, oy := s/24, cx-s/2, cy-s/2
		p := paint.NewPath().MoveTo(ox+9.5*k, oy+6*k).LineTo(ox+15.5*k, oy+12*k).LineTo(ox+9.5*k, oy+18*k)
		ang := float64(open) * 1.5707963
		if rtl {
			// Mirrored: the closed arrow points left and turns the other
			// way to point down.
			p = paint.NewPath().MoveTo(ox+14.5*k, oy+6*k).LineTo(ox+8.5*k, oy+12*k).LineTo(ox+14.5*k, oy+18*k)
			ang = -ang
		}
		if open > 0 {
			p = p.Rotated(ang, cx, cy)
		}
		l.StrokePath(p, paint.StrokeStyle{Width: 1.8 * s / 24 * 1.3, Cap: paint.CapRound}, col)
		return
	}
	// antd expand.js: a CSS box of ExpandIconSize with a 1px border, the
	// lines inset paddingXXS − lineWidth (3) inside the border, the whole
	// scaled by ExpandIconScale (16/17).
	sc := tk.ExpandIconScale
	if sc <= 0 {
		sc = 1
	}
	s := tk.ExpandIconSize * sc
	b := geom.Rect{X: float32(int(r.X + (r.W-s)/2)), Y: float32(int(r.Y + (r.H-s)/2)), W: s, H: s}
	rad := geom.Rad(tk.ExpandIconRadius * sc)
	bc := tk.ExpandIconBorder
	if hov {
		bc = col // border currentcolor on hover
	}
	l.Fill(b, rad, tk.ExpandIconBg)
	l.Stroke(b, rad, 1, bc)
	// Pixel-aligned 1px lines on row / column m with an odd length
	// centred on them (the scaled box is 16px: no centre pixel).
	m := float32(int((s - 1) / 2))
	ln := float32(int(s-8*sc)/2*2 + 1)
	st := m + 0.5 - ln/2
	l.Fill(geom.Rect{X: b.X + st, Y: b.Y + m, W: ln, H: 1}, geom.Radii{}, col)
	if open < 1 {
		hgt := ln * (1 - open)
		l.Fill(geom.Rect{X: b.X + m, Y: b.Y + m + 0.5 - hgt/2, W: 1, H: hgt}, geom.Radii{}, col)
	}
}
