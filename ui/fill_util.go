package ui

// Shared helpers of the fill_* components (VirtualSelect, VirtualTree,
// DatePickerPanel, ColorPickerPanel, Masonry, Listy, BorderBeam). Every
// unexported name here starts with "fill" to stay clear of other
// component families.

import (
	"math"
	"slices"
	"sort"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
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

// ---------------------------------------------------------------------------
// Windowing inside an existing scroll box (dropdown option lists, Command,
// Cascader columns)

// fillWinOverscan is how many rows a fillWin builds past each viewport edge.
const fillWinOverscan = 3

// fillWin windows n rows inside a vertical ScrollBox (fillViewport): only
// the rows that intersect the viewport, plus fillWinOverscan on each side,
// are built. They are laid out from the content top with the scroll base
// set to the first one and the extent covering every row, so the scroll
// range, the bar and each row's position are those of the full list. Rows
// have the fixed height rowH, or the heights given by the prefix sums offs
// (len n+1) when offs is set. The box's vertical padding stays around the
// rows.
type fillWin struct {
	rowH       float64   // fixed row height (offs nil)
	offs       []float64 // prefix sums of the row heights (variable rows)
	first, end int       // rows built: [first, end)
	base       float64   // top of row first, from the top of row 0
}

// top is the offset of row i from the top of row 0.
func (w *fillWin) top(i int) float64 {
	if w.offs != nil {
		return w.offs[i]
	}
	return float64(i) * w.rowH
}

// span sets sb's extent and base for n rows and picks the window from the
// current offset and viewport (clamping the offset to the rows).
func (w *fillWin) span(sb *ScrollBox, n int) {
	w.first, w.end, w.base = 0, 0, 0
	pad := sb.n.ln.Padding
	s := sb.s
	s.extent.Y = w.top(max(0, n)) + float64(pad.Vertical())
	s.base.Y = 0
	if n <= 0 {
		return
	}
	view := fillViewH(sb, 0)
	fillClampOff(sb, s.extent.Y, view)
	off := s.off.Y - float64(pad.Top)
	if w.offs != nil {
		w.first = max(0, fillFind(w.offs, off)-fillWinOverscan)
		w.end = min(n, fillFind(w.offs, off+view)+1+fillWinOverscan)
	} else {
		w.first, w.end = fillSpan(off, view, w.rowH, n, fillWinOverscan)
	}
	w.base = w.top(w.first)
	s.base.Y = w.base
}

// build windows n rows into sb: row(i) adds the node of row i to the
// current parent and returns it. Row keep (the keyboard or focused row; -1
// none) is built even outside the window, placed absolutely at its own
// position, so ActiveDescendant and the focus keep a node to point at.
func (w *fillWin) build(c *Context, sb *ScrollBox, n, keep int, row func(i int) *node) {
	w.span(sb, n)
	pad := sb.n.ln.Padding
	sb.Body(func() {
		if keep >= 0 && keep < w.first && keep < n {
			w.place(row(keep), keep, pad)
		}
		for i := w.first; i < w.end; i++ {
			row(i)
		}
		if keep >= w.end && keep < n {
			w.place(row(keep), keep, pad)
		}
	})
}

// buildKeeps is build with several rows that are always built (the
// current, focused or edited rows of a list; -1 and duplicates are
// ignored). The ones outside the window are placed like build's keep row,
// in row order before or after the window.
func (w *fillWin) buildKeeps(c *Context, sb *ScrollBox, n int, keeps []int, row func(i int) *node) {
	w.span(sb, n)
	pad := sb.n.ln.Padding
	ks := make([]int, 0, len(keeps))
	for _, k := range keeps {
		if k >= 0 && k < n && (k < w.first || k >= w.end) && !slices.Contains(ks, k) {
			ks = append(ks, k)
		}
	}
	slices.Sort(ks)
	sb.Body(func() {
		for _, k := range ks {
			if k < w.first {
				w.place(row(k), k, pad)
			}
		}
		for i := w.first; i < w.end; i++ {
			row(i)
		}
		for _, k := range ks {
			if k >= w.end {
				w.place(row(k), k, pad)
			}
		}
	})
}

// place lays nd, the node of row i built outside the window, out
// absolutely at the row's position inside a parent with padding pad whose
// first laid-out row is the window's first.
func (w *fillWin) place(nd *node, i int, pad geom.Insets) {
	if nd == nil {
		return
	}
	nd.ln.Absolute = true
	nd.ln.Pos = flexbox.Pos{Left: pad.Left, Right: pad.Right, Top: pad.Top + float32(w.top(i)-w.base), Bottom: layout.Unset}
}

// fillStrut adds a hidden node of zero height and width wd: a windowed
// list whose rows size its width is then as wide as its widest row, built
// or not.
func fillStrut(c *Context, wd float32) {
	n := c.add("fill-strut", "")
	n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: wd} }
	fillHidden(c, n)
}

// fillWidthCache is the retained result of fillWidest.
type fillWidthCache struct {
	key   any
	theme *theme.Theme
	w     float32
}

// fillWidest returns the largest rowW(i) of n rows, measured once and
// remembered for the list id while key and the theme stay the same. key
// identifies the row data (a comparable value such as the slice's first
// element pointer and length); nil measures every call. The theme is
// compared by pointer: a theme (font size, tokens) mutated in place keeps
// the old width until the key or the theme pointer changes. A key holding
// a pointer to the caller's data (&src[0]) keeps that slice reachable,
// not collected, until the key is replaced or the list's state is
// dropped: the price of never matching a new slice that reuses the old
// one's address. Callers include the component's DataVersion in key so
// in-place edits can refresh the width.
func fillWidest(c *Context, id ID, key any, n int, rowW func(i int) float32) float32 {
	st := State[fillWidthCache](c, hashID(id, "fill-widest"), nil)
	if key != nil && st.key == key && st.theme == c.Theme {
		return st.w
	}
	wd := float32(0)
	for i := range n {
		wd = max(wd, rowW(i))
	}
	st.key, st.theme, st.w = key, c.Theme, wd
	return wd
}

// fillIDs records the node of each row built this frame by row index (all
// rows of a full list, the window and keep row of a windowed one).
type fillIDs struct {
	rows []int
	ids  []ID
}

func (f *fillIDs) reset() { f.rows, f.ids = f.rows[:0], f.ids[:0] }

func (f *fillIDs) add(i int, id ID) {
	f.rows = append(f.rows, i)
	f.ids = append(f.ids, id)
}

// at returns the node of row i (0 when it was not built).
func (f *fillIDs) at(i int) ID {
	if len(f.rows) == 0 {
		return 0
	}
	if k := i - f.rows[0]; k >= 0 && k < len(f.rows) && f.rows[k] == i {
		return f.ids[k]
	}
	for k, r := range f.rows {
		if r == i {
			return f.ids[k]
		}
	}
	return 0
}
