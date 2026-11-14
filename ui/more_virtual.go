package ui

import (
	"math"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
)

// ---------------------------------------------------------------------------
// VirtualList

// VirtualListW is a vertically scrolling list of n fixed-height rows that
// only builds the rows inside the viewport (plus a small overscan), so it
// stays cheap for hundreds of thousands of items.
type VirtualListW struct {
	_ [0]func()

	c          *Context
	sb         *ScrollBox
	count      int
	itemH      float32
	overscan   int
	fn         func(i int)
	first, end int
	z          *lazy
}

// VirtualList adds a virtual list of n rows of itemHeight px; fn(i) builds
// row i (it runs inside a fixed-height row and its own identity scope, so
// row widgets keep their state while scrolling).
//
//	ui.VirtualList(c, len(rows), 32, func(i int) { ui.Text(c, rows[i]) }).Height(400)
func VirtualList(c *Context, n int, itemHeight float32, fn func(i int), opts ...layout.Option) *VirtualListW {
	w := &VirtualListW{c: c, count: max(0, n), itemH: max(1, itemHeight), overscan: 3, fn: fn}
	b := c.box("vlist", layout.Column, opts)
	b.n.ln.ScrollY = true
	b.n.setRole(a11y.RoleList).fill(w)
	w.sb = c.finishScroll(b, nil)
	w.z = c.lazy(b.n, w.build)
	return w
}

// semantics reports the size of the whole data set (only the rows in view
// are built).
func (w *VirtualListW) semantics(s *a11y.Node) { s.RowCount = w.count }

// A11yLabel sets the accessible name of the list.
func (w *VirtualListW) A11yLabel(s string) *VirtualListW { w.sb.n.setName(s); return w }

// Height fixes the viewport height.
func (w *VirtualListW) Height(px float32) *VirtualListW { w.sb.n.ln.Height = px; return w }

// Layout applies layout options to the viewport.
func (w *VirtualListW) Layout(opts ...layout.Option) *VirtualListW {
	flexbox.Apply(&w.sb.n.ln.Style, opts...)
	return w
}

// Overscan sets how many extra rows are built above and below the
// viewport (default 3).
func (w *VirtualListW) Overscan(rows int) *VirtualListW { w.overscan = max(0, rows); return w }

// ScrollToIndex scrolls so row i is at the top (clamped to the list).
func (w *VirtualListW) ScrollToIndex(i int) *VirtualListW {
	i = max(0, min(i, w.count-1))
	w.sb.s.off.Y = float64(i) * float64(w.itemH)
	w.clamp()
	w.c.invalid = true
	return w
}

// ScrollIntoView scrolls the minimum amount that makes row i fully visible.
func (w *VirtualListW) ScrollIntoView(i int) *VirtualListW {
	i = max(0, min(i, w.count-1))
	top := float64(i) * float64(w.itemH)
	s := w.sb.s
	view := float64(w.viewH())
	switch {
	case top < s.off.Y:
		s.off.Y = top
	case top+float64(w.itemH) > s.off.Y+view:
		s.off.Y = top + float64(w.itemH) - view
	}
	w.clamp()
	w.c.invalid = true
	return w
}

// Offset returns the vertical scroll offset.
func (w *VirtualListW) Offset() float64 { return w.sb.s.off.Y }

// Range returns the half-open range [first, end) of rows built this frame.
func (w *VirtualListW) Range() (first, end int) { w.z.run(); return w.first, w.end }

// ID returns the scroll container's ID.
func (w *VirtualListW) ID() ID { return w.sb.n.id }

func (w *VirtualListW) viewH() float32 {
	if h := w.sb.s.view.H; h > 0 {
		return h
	}
	ln := &w.sb.n.ln
	switch {
	case ln.Height > 0:
		return ln.Height
	case ln.MaxH > 0:
		return min(ln.MaxH, w.c.size.H)
	}
	return w.c.size.H
}

func (w *VirtualListW) total() float64 { return float64(w.count) * float64(w.itemH) }

func (w *VirtualListW) clamp() {
	s := w.sb.s
	s.off.Y = max(0, min(s.off.Y, w.total()-float64(w.viewH())))
}

// build lays out only the rows in view. The rows are laid out from 0 and
// the scroll state's base is set to the first row's content position, so
// row coordinates stay small (exact in float32) however far the list is
// scrolled; the base Scroll provides the draggable scrollbar.
func (w *VirtualListW) build() {
	c := w.c
	s := w.sb.s
	s.extent.Y = w.total()
	if w.count == 0 {
		return
	}
	w.clamp()
	ih := float64(w.itemH)
	off, view := s.off.Y, float64(w.viewH())
	first := int(math.Floor(off/ih)) - w.overscan
	end := int(math.Ceil((off+view)/ih)) + w.overscan
	first = max(0, first)
	end = min(w.count, end)
	w.first, w.end = first, end
	s.base.Y = float64(first) * ih
	for i := first; i < end; i++ {
		Key(c, i, func() {
			row := Column(c, func() { w.fn(i) }, layout.H(w.itemH))
			row.n.setRole(a11y.RoleListItem)
			if c.a11yOn {
				// Position in the full data set, not among the built rows.
				pos, size := i+1, w.count
				row.n.fill(semFunc(func(s *a11y.Node) { s.PosInSet, s.SetSize = pos, size }))
			}
		})
	}
}
