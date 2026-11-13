package ui

// Masonry (Ant Design 6): a waterfall layout that places each item in the
// currently shortest column, with responsive column counts, gutters and
// optional virtualization (only items near the viewport are built).

import (
	"github.com/tangoui/tango/ui/a11y"
	"sort"

	"github.com/tangoui/tango/ui/layout"
)

type fillMasonryState struct {
	heights map[any]float32 // measured item heights (previous frames)
	ids     map[any]ID      // item boxes of the previous build
	ver     int             // bumped when a measured height changes

	// Placement cache.
	key    fillMasonryKey
	hs     []float32
	col    []int
	top    []float32
	colH   []float32
	perCol [][]int32
}

type fillMasonryKey struct {
	n, cols       int
	gv, est       float32
	ver           int
	fixed, pinned bool
}

// MasonryW is a masonry (waterfall) layout. Configure it and call Show.
type MasonryW struct {
	c        *Context
	count    int
	render   func(i int)
	height   func(i int) float32
	key      func(i int) any
	pin      func(i int) int
	cols     int
	colsAt   [6]int
	gh, gv   float32
	est      float32
	virtual  bool
	viewH    float32
	overscan float32
	st       *fillMasonryState

	assign []int
	sb     *ScrollBox
	built  int
}

// Masonry lays out n items built by render(i) in columns, each item going
// to the shortest column so far.
//
//	ui.Masonry(c, len(cards), func(i int) { card(i) }).Columns(4).Gutter(16).Show()
func Masonry(c *Context, n int, render func(i int)) *MasonryW {
	id := c.newID("fill-masonry", "")
	st := State(c, id, func() fillMasonryState {
		return fillMasonryState{heights: map[any]float32{}, ids: map[any]ID{}}
	})
	w := &MasonryW{c: c, count: max(0, n), render: render, cols: 3, est: 120, overscan: 200, st: st}
	// ConfigProvider.Defaults("Masonry", ...) keys: "columns" (int or
	// map[Breakpoint]int), "gutter" (px).
	var resp dataRespCols
	dataPropCols(c, "Masonry", "columns", &w.cols, &resp)
	w.colsAt = resp
	if v, ok := p2bNum(c, "Masonry", "gutter"); ok {
		w.gh, w.gv = float32(v), float32(v)
	}
	return w
}

// Columns sets the column count (default 3).
func (w *MasonryW) Columns(n int) *MasonryW { w.cols = max(1, n); return w }

// ColumnsAt sets the column count from breakpoint bp upwards (Ant
// columns={{ xs: 1, sm: 2, md: 3 }}); the largest set breakpoint not above
// the window width wins, Columns is the fallback.
func (w *MasonryW) ColumnsAt(bp Breakpoint, n int) *MasonryW {
	if int(bp) < len(w.colsAt) {
		w.colsAt[bp] = max(1, n)
	}
	return w
}

// Gutter sets the horizontal and vertical spacing.
func (w *MasonryW) Gutter(px float32) *MasonryW { w.gh, w.gv = px, px; return w }

// Gutters sets horizontal and vertical spacing separately.
func (w *MasonryW) Gutters(h, v float32) *MasonryW { w.gh, w.gv = h, v; return w }

// ItemHeight gives known item heights (Ant item.height); items then get
// that fixed height. Without it heights are measured after the first
// paint (EstimatedHeight is used until then).
func (w *MasonryW) ItemHeight(fn func(i int) float32) *MasonryW { w.height = fn; return w }

// EstimatedHeight is the height assumed for items not measured yet.
func (w *MasonryW) EstimatedHeight(px float32) *MasonryW { w.est = max(1, px); return w }

// ItemKey gives items stable identities (default: the index).
func (w *MasonryW) ItemKey(fn func(i int) any) *MasonryW { w.key = fn; return w }

// ItemColumn pins items to a column (0-based; return -1 for automatic),
// so existing items keep their column when more are added.
func (w *MasonryW) ItemColumn(fn func(i int) int) *MasonryW { w.pin = fn; return w }

// Virtual scrolls the layout inside a viewport of the given height and
// builds only the items near it.
func (w *MasonryW) Virtual(height float32) *MasonryW { w.virtual, w.viewH = true, height; return w }

// Assignment returns the column of every item (after Show).
func (w *MasonryW) Assignment() []int { return w.assign }

// Built returns how many items were built by Show (virtual mode builds
// only the visible ones).
func (w *MasonryW) Built() int { return w.built }

// ID returns the viewport's ID in virtual mode (after Show).
func (w *MasonryW) ID() ID {
	if w.sb == nil {
		return 0
	}
	return w.sb.n.id
}

func (w *MasonryW) columns() int {
	bp := ScreenBreakpoint(w.c)
	for b := int(bp); b >= 0; b-- {
		if w.colsAt[b] > 0 {
			return w.colsAt[b]
		}
	}
	return w.cols
}

func (w *MasonryW) keyOf(i int) any {
	if w.key != nil {
		return w.key(i)
	}
	return i
}

// fillMasonryPlace assigns items to columns greedily: the pinned column if
// valid, else the shortest one (leftmost on ties). It returns each item's
// column and top, and the column heights.
func fillMasonryPlace(heights []float32, cols int, gap float32, pin func(i int) int) (col []int, top []float32, colH []float32) {
	col = make([]int, len(heights))
	top = make([]float32, len(heights))
	colH = make([]float32, cols)
	for i, h := range heights {
		k := -1
		if pin != nil {
			if p := pin(i); p >= 0 && p < cols {
				k = p
			}
		}
		if k < 0 {
			k = 0
			for j := 1; j < cols; j++ {
				if colH[j] < colH[k] {
					k = j
				}
			}
		}
		col[i], top[i] = k, colH[k]
		colH[k] += h + gap
	}
	return col, top, colH
}

// Show builds the layout.
func (w *MasonryW) Show() *MasonryW {
	c, st := w.c, w.st
	cols := w.columns()
	// Measured heights of the items built last frame.
	if w.height == nil {
		for k, id := range st.ids {
			if r, ok := c.rects[id]; ok && r.H > 0 && st.heights[k] != r.H {
				st.heights[k] = r.H
				st.ver++
			}
		}
	}
	clear(st.ids)
	hs := st.hs[:0]
	same := true
	if w.height != nil {
		for i := 0; i < w.count; i++ {
			h := w.height(i)
			if i >= len(st.hs) || st.hs[i] != h {
				same = false
			}
			hs = append(hs, h)
		}
	}
	key := fillMasonryKey{n: w.count, cols: cols, gv: w.gv, est: w.est, ver: st.ver, fixed: w.height != nil, pinned: w.pin != nil}
	if !same || key != st.key || st.perCol == nil || w.pin != nil {
		if w.height == nil {
			for i := 0; i < w.count; i++ {
				h, ok := st.heights[w.keyOf(i)]
				if !ok {
					h = w.est
				}
				hs = append(hs, h)
			}
		}
		st.hs = hs
		st.col, st.top, st.colH = fillMasonryPlace(hs, cols, w.gv, w.pin)
		st.perCol = make([][]int32, cols)
		for i, k := range st.col {
			st.perCol[k] = append(st.perCol[k], int32(i))
		}
		st.key = key
	}
	hs, top, colH := st.hs, st.top, st.colH
	w.assign = st.col
	var lo, hi float64 = -1, -1 // visible content range (virtual)
	content := func() {
		Row(c, func() {
			for k := 0; k < cols; k++ {
				items := st.perCol[k]
				Column(c, func() {
					from := 0
					if lo >= 0 {
						from = sort.Search(len(items), func(j int) bool {
							i := items[j]
							return float64(top[i]+hs[i]) >= lo
						})
					}
					for j := from; j < len(items); j++ {
						i := int(items[j])
						if lo >= 0 && float64(top[i]) > hi {
							break
						}
						if j == from && lo >= 0 && top[i] > 0 {
							pad := c.add("fill-masonry-pad", "")
							pad.ln.Height = top[i] - w.gv
						}
						key := w.keyOf(i)
						Key(c, key, func() {
							b := Column(c, func() { w.render(i) }, layout.MinW(0))
							b.Semantics(a11y.RoleListItem, "")
							if w.height != nil {
								b.Style(layout.H(hs[i]))
							}
							st.ids[key] = b.ID()
						})
						w.built++
					}
				}, layout.Grow(1), layout.MinW(0), layout.Gap(w.gv), layout.Self(layout.AlignStart))
			}
		}, layout.Gap(w.gh), layout.MinH(fillMaxF(colH)-w.gv))
	}
	if !w.virtual {
		content()
		return w
	}
	w.sb = fillViewport(c, "fill-masonry", []layout.Option{layout.H(w.viewH)})
	total := float64(max(0, fillMaxF(colH)-w.gv))
	view := fillViewH(w.sb, w.viewH)
	w.sb.s.extent.Y = total
	fillClampOff(w.sb, total, view)
	lo = max(0, w.sb.s.off.Y-float64(w.overscan))
	hi = w.sb.s.off.Y + view + float64(w.overscan)
	w.sb.Body(content)
	return w
}

func fillMaxF(v []float32) float32 {
	m := float32(0)
	for _, x := range v {
		m = max(m, x)
	}
	return m
}
