package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// The virtual form of List (Height + Virtual): only the items in the
// viewport are built, with heights measured as they come into view and
// cached by RowKey, as Listy does. The windowing is fillWin's.

// listVirtState is the retained state of a virtual List, keyed by its
// viewport.
type listVirtState struct {
	key     listVirtKey
	keys    []any           // RowKeys in the order of the height table
	hs      []float32       // unit heights (item rows, or grid rows plus their margin)
	offs    []float64       // prefix sums of hs, len units+1
	byKey   map[any]float32 // measured unit heights by RowKey (margin excluded)
	byIdx   map[int]float32 // measured unit heights by first item (no RowKey)
	built   []listBuilt     // units built by the last build
	laidOut bool            // built matches the last frame's rectangles
	keep    int             // unit that held the focus at the end of the last build (-1 none)
	kept    any             // its key

	pending       listScroll // last ScrollToKey (re-applied while heights settle)
	pendingFrames int
}

// listVirtKey is what the unit table depends on.
type listVirtKey struct {
	a, b, cols        int
	est, extra, itemH float32
	keyed             bool
}

type listBuilt struct {
	u   int
	key any
	id  ID
}

// virtualOn reports whether the items are windowed.
func (w *ListW) virtualOn() bool { return w.virtual && w.height > 0 }

func (w *ListW) keyOf(i int) any {
	if w.rowKey != nil {
		return w.rowKey(i)
	}
	return i
}

// estH is the assumed height of an item not measured yet.
func (w *ListW) estH() float32 {
	if w.est > 0 {
		return w.est
	}
	pv, _ := w.pad()
	return w.th.LineHeightFor(w.th.FontSize) + 2*pv
}

// extraH is the space below a unit outside its box (a grid row's margin).
func (w *ListW) extraH() float32 {
	if w.grid > 0 {
		return w.tk.GridItemMarginBottom
	}
	return 0
}

// listGridRow keys a windowed grid row by its first item.
type listGridRow struct{ first int }

// virtItems windows items [a, b) into sb. A unit is an item, or a grid
// row of cols items.
func (w *ListW) virtItems(sb *ScrollBox, a, b int, after bool, fn func(i int)) {
	c := w.c
	cols, extra := 1, w.extraH()
	if w.grid > 0 {
		cols = w.gridAt.resolve(c, w.grid)
	}
	m := (b - a + cols - 1) / cols
	first := func(u int) int { return a + u*cols }
	st := State(c, hashID(sb.n.id, "list-virt"), func() listVirtState {
		return listVirtState{byKey: map[any]float32{}, byIdx: map[int]float32{}, keep: -1}
	})
	est := w.estH()
	k := listVirtKey{a: a, b: b, cols: cols, est: est, extra: extra, itemH: w.itemH, keyed: w.rowKey != nil}
	var win fillWin
	var view float64
	top := func(u int) (y, h float64) { return float64(u) * win.rowH, win.rowH }
	if w.itemH > 0 {
		win.rowH = float64(w.itemH + extra)
		st.key, st.hs, st.offs = k, nil, nil
		st.keys = nil
	} else {
		w.measure(st, k, m, est, extra)
		win.offs = st.offs
		top = func(u int) (y, h float64) { return st.offs[u], st.offs[u+1] - st.offs[u] }
		over := sb.n.over
		sb.n.over = func(l *paint.List, r geom.Rect) {
			st.laidOut = true
			over(l, r)
		}
	}
	view = fillViewH(sb, w.height)
	w.applyScroll(st, sb, a, b, cols, m, top, view)
	keep := -1
	if st.keep >= 0 && st.keep < m && w.keyOf(first(st.keep)) == st.kept {
		keep = st.keep
	}
	st.built = st.built[:0]
	var nodes []*node
	win.build(c, sb, m, keep, func(u int) *node {
		var nd *node
		i := first(u)
		if w.grid > 0 {
			Key(c, listGridRow{i}, func() { nd = w.gridRow(i, b, cols, fn) })
		} else {
			nd = w.itemRow(i, fn, i == b-1, after)
		}
		st.built = append(st.built, listBuilt{u: u, key: w.keyOf(i), id: nd.id})
		nodes = append(nodes, nd)
		return nd
	})
	st.keep, st.kept = -1, nil
	for j, nd := range nodes {
		if c.focusWithin(nd) {
			st.keep, st.kept = st.built[j].u, st.built[j].key
			break
		}
	}
	w.first, w.end = first(win.first), min(b, first(win.end))
	if w.itemH > 0 {
		st.built = st.built[:0] // nothing to measure
	}
}

// measure brings the unit heights up to date: the units built by the last
// frame are measured, and the prefix sums are recomputed from the first
// one whose height changed. A new unit layout or RowKey order rebuilds
// the table using the measurements saved by key or index.
func (w *ListW) measure(st *listVirtState, k listVirtKey, m int, est, extra float32) {
	c := w.c
	dirty := m
	same := k == st.key && len(st.hs) == m
	if same && k.keyed {
		for u, key := range st.keys {
			if w.keyOf(k.a+u*k.cols) != key {
				same = false
				break
			}
		}
	}
	built := st.built
	if !st.laidOut {
		// Followers can trigger another build before the new rows have
		// rectangles. Those still belong to the old keys at each position.
		built = nil
	}
	st.laidOut = false
	for _, bt := range built {
		r, ok := c.rects[bt.id]
		if !ok || r.H <= 0 {
			continue
		}
		if st.key.keyed {
			st.byKey[bt.key] = r.H
		} else {
			st.byIdx[bt.key.(int)] = r.H
		}
		if same && bt.u < m && st.hs[bt.u] != r.H+extra {
			st.hs[bt.u] = r.H + extra
			dirty = min(dirty, bt.u)
		}
	}
	if !same {
		st.key = k
		st.keys = st.keys[:0]
		st.hs = st.hs[:0]
		for u := 0; u < m; u++ {
			i := k.a + u*k.cols
			var h float32
			var ok bool
			if k.keyed {
				key := w.keyOf(i)
				st.keys = append(st.keys, key)
				h, ok = st.byKey[key]
			} else {
				h, ok = st.byIdx[i]
			}
			if !ok {
				h = est
			}
			st.hs = append(st.hs, h+extra)
		}
		st.offs = append(st.offs[:0], make([]float64, m+1)...)
		dirty = 0
	}
	for u := dirty; u < m; u++ {
		st.offs[u+1] = st.offs[u] + float64(st.hs[u])
	}
}

// applyScroll applies a ScrollToKey request; under Virtual it is re-applied
// for a few frames while the heights around the target are measured.
func (w *ListW) applyScroll(st *listVirtState, sb *ScrollBox, a, b, cols, m int, top func(u int) (y, h float64), view float64) {
	q := w.req
	if q.set {
		st.pending, st.pendingFrames = q, 4
		if w.itemH > 0 {
			st.pendingFrames = 0
		}
	} else if st.pendingFrames > 0 {
		q = st.pending
		st.pendingFrames--
	} else {
		return
	}
	i := w.findKey(q.key, a, b)
	if i < 0 {
		return
	}
	y, h := top((i - a) / cols)
	total := float64(m) * float64(w.itemH+w.extraH())
	if w.itemH <= 0 {
		total = st.offs[m]
	}
	listScrollTo(w.c, sb, q, y, h, total, view)
}

// listScrollTo moves sb so that the content range [y, y+h) is aligned as
// q asks (Listy's alignments).
func listScrollTo(c *Context, sb *ScrollBox, q listScroll, y, h, total, view float64) {
	s := sb.s
	var target float64
	switch q.align {
	case AlignTop:
		target = y
	case AlignBottom:
		target = y + h - view
	default:
		target = s.off.Y
		if y < target {
			target = y
		} else if y+h > target+view {
			target = y + h - view
		}
	}
	target += q.offset
	s.off.Y = max(0, min(target, total-view))
	c.invalid = true
}

// findKey returns the item of [a, b) with key (-1 when none).
func (w *ListW) findKey(key any, a, b int) int {
	if w.rowKey == nil {
		if i, ok := key.(int); ok && i >= a && i < b {
			return i
		}
		return -1
	}
	for i := a; i < b; i++ {
		if w.rowKey(i) == key {
			return i
		}
	}
	return -1
}

// scrollFull applies a ScrollToKey request to the non-virtual viewport
// from the rows painted in the last frame.
func (w *ListW) scrollFull(sb *ScrollBox, a, b int) {
	c := w.c
	st := State(c, hashID(sb.n.id, "list-scroll"), func() listScroll { return listScroll{} })
	if w.req.set {
		*st = w.req
	}
	if !st.set {
		return
	}
	i := w.findKey(st.key, a, b)
	if i < 0 {
		st.set = false
		return
	}
	rows := sb.n.kids
	if len(rows) == 1 && w.grid > 0 {
		rows = rows[0].kids // gridItems' column
	}
	u := i - a
	if w.grid > 0 {
		u /= w.gridAt.resolve(c, w.grid)
	}
	if u >= len(rows) || sb.s.view.H <= 0 {
		c.WakeAt(c.now) // not laid out yet
		return
	}
	vr, ok1 := c.rects[sb.n.id]
	ir, ok2 := c.rects[rows[u].id]
	if !ok1 || !ok2 {
		c.WakeAt(c.now)
		return
	}
	y := float64(ir.Y-vr.Y) + sb.s.painted.Y
	listScrollTo(c, sb, *st, y, float64(ir.H), sb.s.content.Y, float64(sb.s.view.H))
	st.set = false
}
