package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Table-only glyphs (24×24 grid, 1.8 stroke, original path data).
var (
	tblIconFunnel  = icon.New("table-filter", "", "M3 4H21L14.2 12.4V19.6L9.8 21.5V12.4Z")
	tblIconDensity = icon.New("table-density", "M4 4.5H20M4 19.5H20M12 7.5V16.5M9 10L12 7L15 10M9 14L12 17L15 14", "")
	// Column pinning (column settings): bar at the top / middle / bottom.
	tblIconPinTop    = icon.New("table-pin-top", "M4 3.5H20M12 21V8M7 12.5L12 7.5L17 12.5", "")
	tblIconPinMiddle = icon.New("table-pin-middle", "M4 12H20M12 2.5V8.5M9 5.5L12 8.5L15 5.5M12 21.5V15.5M9 18.5L12 15.5L15 18.5", "")
	tblIconPinBottom = icon.New("table-pin-bottom", "M4 20.5H20M12 3V16M7 11.5L12 16.5L17 11.5", "")
)

func tblDrawSorter(th *theme.Theme, tk *theme.TableToken, l *paint.List, r geom.Rect, order SortOrder) {
	idle := tk.SorterIdleColor
	up, down := idle, idle
	if order == SortAsc {
		up = th.Primary.Base
	} else if order == SortDesc {
		down = th.Primary.Base
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	hw, h := float32(4), float32(4.5)
	if th.Flavor == theme.Element {
		hw, h = 5, 5
	}
	g := float32(1.5)
	l.Path(paint.NewPath().MoveTo(cx, cy-g-h).LineTo(cx+hw, cy-g).LineTo(cx-hw, cy-g).Close(), up)
	l.Path(paint.NewPath().MoveTo(cx, cy+g+h).LineTo(cx-hw, cy+g).LineTo(cx+hw, cy+g).Close(), down)
}

// tblDrawExpand paints the expand / tree toggle: Ant's bordered plus/minus
// square or Element's rotating arrow, which points toward the inline end
// when closed (left in RTL) and down when open.
func tblDrawExpand(th *theme.Theme, l *paint.List, r geom.Rect, open float32, hov, rtl bool) {
	if th.Flavor == theme.Element {
		col := th.TextSecondary
		if hov {
			col = th.Primary.Base
		}
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
	const s = 17
	b := geom.Rect{X: float32(int(r.X + (r.W-s)/2)), Y: float32(int(r.Y + (r.H-s)/2)), W: s, H: s}
	bc := th.BorderSecondary
	fg := th.Text
	if hov {
		bc, fg = th.Primary.Hover, th.Primary.Hover
	}
	l.Fill(b, geom.Rad(th.RadiusSM), th.BgContainer)
	l.Stroke(b, geom.Rad(th.RadiusSM), 1, bc)
	cx, cy := b.X+s/2, b.Y+s/2
	l.Fill(geom.Rect{X: b.X + 4, Y: cy - 0.5, W: s - 8, H: 1}, geom.Radii{}, fg)
	if open < 1 {
		hgt := (s - 8) * (1 - open)
		l.Fill(geom.Rect{X: cx - 0.5, Y: cy - hgt/2, W: 1, H: hgt}, geom.Radii{}, fg)
	}
}
