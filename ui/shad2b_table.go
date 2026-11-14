package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// shadcn/ui data-table look of Table (Ant and Element keep their own).
//
//   - rows: cell p-2 with text-sm leading-5 (37px with the border), header
//     h-10 (41px with the border);
//   - no tint on the sorted column; the sortable header is a ghost button
//     (accent pill on hover) with one arrow after the title: up/down when
//     sorted, a muted up-down glyph otherwise;
//   - selected rows bg-muted, hover bg-muted/50 (theme tokens).

const (
	shad2bSorterSize = float32(14) // size-3.5 arrow
	shad2bSorterGap  = float32(6)
	shad2bPillPadH   = float32(6)
)

// shad2bMetrics sets the shadcn row and header heights.
// The paddings come from the Table token (metrics): shadcn has one
// density (p-2) and Small trims the vertical padding.
func (f *tblFrame) shad2bMetrics() {
	t := f.t
	f.lh = 20
	f.rh = f.lh + 2*f.pv + 1
	f.hh = max(f.rh, 41) // TableHead h-10 + the row border
	if t.rowH > 0 {
		f.rh = t.rowH
	}
}

// shad2bHeadText is the header title color: muted, foreground while the
// sort button is hovered or the column is sorted.
func shad2bHeadText(th *theme.Theme, tk *theme.TableToken, order SortOrder, hov bool) theme.Color {
	if hov || order != SortNone {
		return th.Text
	}
	return tk.HeaderColor
}

// shad2bHeadSort paints the ghost-button pill behind a sortable header title
// (hover / keyboard focus) and the sort arrow at the end of r (the title
// plus the arrow slot): the right end, or the left one in RTL.
func shad2bHeadSort(l *paint.List, th *theme.Theme, tk *theme.TableToken, r geom.Rect, order SortOrder, hov, focus, rtl bool) {
	ph := float32(32)
	pill := geom.Rect{X: r.X - shad2bPillPadH, Y: r.Y + (r.H-ph)/2, W: r.W + 2*shad2bPillPadH, H: ph}
	rad := geom.Rad(th.Radius)
	if hov {
		l.Fill(pill, rad, th.BgTextHover)
	}
	if focus {
		drawFocusRing(l, th, pill, rad)
	}
	ic, col := icon.ChevronsUpDown, tk.SorterIdleColor
	switch order {
	case SortAsc:
		ic, col = icon.ArrowUp, th.Text
	case SortDesc:
		ic, col = icon.ArrowDown, th.Text
	}
	if hov {
		col = th.Text
	}
	s := shad2bSorterSize
	x := r.Right() - s
	if rtl {
		x = r.X
	}
	icon.Draw(l, ic, geom.Rect{X: x, Y: r.Y + (r.H-s)/2, W: s, H: s}, col, 0)
}
