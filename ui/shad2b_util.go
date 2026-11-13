package ui

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// shad2bIconArrowLeft mirrors icon.ArrowRight (original path data).
var shad2bIconArrowLeft = icon.New("shad2b-arrow-left", "M20 12H4.5M11 5.5L4.5 12L11 18.5", "")

// Small shared helpers of the shadcn data-display styles (shad2b_*).

// shad2bCalDay is the day button size of the card Calendar: 32 (size-8)
// for shadcn, Ant's 24 otherwise.
func shad2bCalDay(th *theme.Theme) float32 {
	if th.Flavor == theme.StyleShadcn {
		return 32
	}
	return 24
}

// shad2bCarouselArrow paints a shadcn CarouselPrevious/Next: a round
// outline icon button (bg-background, border, shadow-xs; hover bg-accent)
// with a size-4 arrow in the foreground color.
func shad2bCarouselArrow(l *paint.List, th *theme.Theme, r geom.Rect, ic *icon.Icon, hov bool, op float32) {
	if op < 1 {
		l.Opacity(op)
		defer l.Pop()
	}
	rad := geom.Rad(r.W / 2)
	if len(th.ButtonShadow) > 0 {
		l.Shadows(r, rad, th.ButtonShadow)
	}
	bg := th.BgContainer
	if hov {
		bg = th.BgTextHover
	}
	l.Fill(r, rad, bg)
	l.Stroke(r, rad, 1, th.Border)
	s := float32(16)
	switch ic {
	case icon.Left:
		ic = shad2bIconArrowLeft
	case icon.Right:
		ic = icon.ArrowRight
	case icon.Up:
		ic = icon.ArrowUp
	case icon.Down:
		ic = icon.ArrowDown
	}
	icon.Draw(l, ic, geom.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}, th.Text, 0)
}

// shad2bTreeChevron paints the size-4 chevron-right of a shadcn tree row
// in col (TreeToken.SwitcherColor: muted foreground), rotated 90° as t
// goes from collapsed (0) to open (1). The row metrics live in TreeToken.
func shad2bTreeChevron(l *paint.List, col theme.Color, r geom.Rect, t float32) {
	shad2bTreeChevronDir(l, col, r, t, false)
}

// shad2bTreeChevronDir is shad2bTreeChevron for an RTL scope (rtl): the
// collapsed chevron points left and turns the other way to point down.
func shad2bTreeChevronDir(l *paint.List, col theme.Color, r geom.Rect, t float32, rtl bool) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	p := paint.NewPath().MoveTo(cx-1.5, cy-4).LineTo(cx+2.5, cy).LineTo(cx-1.5, cy+4)
	ang := float64(t) * math.Pi / 2
	if rtl {
		p = paint.NewPath().MoveTo(cx+1.5, cy-4).LineTo(cx-2.5, cy).LineTo(cx+1.5, cy+4)
		ang = -ang
	}
	l.StrokePath(p.Rotated(ang, cx, cy), paint.StrokeStyle{Width: 1.5, Cap: paint.CapRound}, col)
}
