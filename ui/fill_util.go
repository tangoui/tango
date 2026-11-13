package ui

// Shared helpers of the fill_* components (VirtualSelect, VirtualTree,
// DatePickerPanel, ColorPickerPanel, Masonry, Listy, BorderBeam). Every
// unexported name here starts with "fill" to stay clear of other
// component families.

import (
	"math"
	"sort"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/layout"
)

// fillSpan returns the half-open range [first, end) of fixed-height rows
// that intersect the viewport [off, off+view), widened by overscan rows.
func fillSpan(off, view, rowH float64, count, overscan int) (first, end int) {
	if count <= 0 || rowH <= 0 {
		return 0, 0
	}
	first = int(math.Floor(off/rowH)) - overscan
	end = int(math.Ceil((off+view)/rowH)) + overscan
	return max(0, first), min(count, max(end, 0))
}

// fillFind returns the row whose span [offs[i], offs[i+1]) contains y in a
// prefix-sum table of len n+1 (clamped to [0, n-1]; -1 when empty).
func fillFind(offs []float64, y float64) int {
	n := len(offs) - 1
	if n <= 0 {
		return -1
	}
	i := sort.Search(n, func(i int) bool { return offs[i+1] > y })
	return min(i, n-1)
}

// fillViewport creates a vertically scrolling viewport for virtual
// content; the caller sets the scroll extent and base and builds the
// visible window into it (see VirtualListW.build).
func fillViewport(c *Context, kind string, opts []layout.Option) *ScrollBox {
	b := c.box(kind, layout.Column, opts)
	b.n.ln.ScrollY = true
	return c.finishScroll(b, nil)
}

// fillViewH is the viewport height of sb: the painted height once known,
// else the fixed or maximum height, else fallback.
func fillViewH(sb *ScrollBox, fallback float32) float64 {
	if h := sb.s.view.H; h > 0 {
		return float64(h)
	}
	ln := &sb.n.ln
	switch {
	case ln.Height > 0:
		return float64(ln.Height)
	case ln.MaxH > 0:
		return float64(ln.MaxH)
	}
	return float64(fallback)
}

// fillReveal scrolls sb the minimum amount that makes the content range
// [top, top+h) visible, using view as the viewport height.
func fillReveal(c *Context, sb *ScrollBox, top, h, view float64) {
	s := sb.s
	off := s.off.Y
	switch {
	case top < off:
		off = top
	case top+h > off+view:
		off = top + h - view
	}
	if off != s.off.Y {
		s.off.Y = max(0, off)
		c.invalid = true
	}
}

// fillClampOff keeps the scroll offset inside [0, total-view].
func fillClampOff(sb *ScrollBox, total, view float64) {
	sb.s.off.Y = max(0, min(sb.s.off.Y, total-view))
}

// fillSem annotates node n with a role, name and states through the
// public Box.Semantics / Box.A11yState API.
func fillSem(c *Context, n *node, role a11y.Role, name string, states ...a11y.State) {
	b := &Box{c: c, n: n}
	b.Semantics(role, name)
	for _, s := range states {
		b.A11yState(s)
	}
}

// fillKeyboard tells the accessibility audit that pointer-only node n is
// reachable with the keyboard through its composite (list / tree / grid).
func fillKeyboard(c *Context, n *node) { (&Box{c: c, n: n}).A11yKeyboard() }

// fillHidden hides node n (and its subtree) from the semantics tree.
func fillHidden(c *Context, n *node) { (&Box{c: c, n: n}).A11yHidden() }

// fillIf returns s when on, else 0.
func fillIf(on bool, s a11y.State) a11y.State {
	if on {
		return s
	}
	return 0
}
