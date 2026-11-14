package ui

// Listy (Ant Design 6.6): a data-driven list built for long data: rows
// built by a callback, optional virtual scrolling with measured (variable)
// row heights, grouping with sticky group headers, programmatic scrolling
// and a load-more footer.

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// Scroll alignments of Listy.ScrollToKey / ScrollToGroup.
const (
	AlignAuto   = "auto"
	AlignTop    = "top"
	AlignBottom = "bottom"
)

type fillListyState struct {
	heights map[any]float32   // measured row heights by RowKey
	hIdx    map[int32]float32 // measured row heights by index (no RowKey)
	ids     map[any]ID        // row boxes of the previous build
	ver     int               // bumped when a measured height changes

	rowsKey fillListyRowsKey
	rows    []fillListyRow
	groups  []string
	counts  []int
	offsKey fillListyOffsKey
	offs    []float64 // row tops (prefix sums), len rows+1

	pending       fillListyScroll // last ScrollTo* request (re-applied while heights settle)
	pendingFrames int
}

type fillListyRowsKey struct {
	n, version int
	grouped    bool
}

type fillListyOffsKey struct {
	rows       fillListyRowsKey
	ver        int
	itemH, est float32
}

// fillListyRow is a row: an item (index) or a group header (item -1).
type fillListyRow struct {
	item  int32
	group int32
}

type fillListyScroll struct {
	set     bool
	px      float64
	key     any
	group   string
	byKey   bool
	byGroup bool
	align   string
	offset  float64
	pxOnly  bool
}

// ListyW is a Listy list. Configure it and call Show.
type ListyW struct {
	_ [0]func()

	c         *Context
	count     int
	item      func(i int)
	key       func(i int) any
	groupKey  func(i int) string
	groupHead func(key string, count int)
	sticky    bool
	height    float32
	virtual   bool
	itemH     float32
	est       float32
	hoverable bool
	split     bool
	hasMore   bool
	loading   bool
	more      bool
	version   int
	req       fillListyScroll
	st        *fillListyState

	sb         *ScrollBox
	rows       []fillListyRow
	groups     []string
	counts     []int
	first, end int
	clicked    int
	nearEnd    bool
}

// Listy starts a list of n items; item(i) builds the content of row i
// (padded 12/16 with a divider and hover background).
//
//	ui.Listy(c, len(users), func(i int) { ui.Text(c, users[i].Name) }).
//		Virtual().Height(400).Group(byLetter, letterHeader).Sticky().Show()
func Listy(c *Context, n int, item func(i int)) *ListyW {
	id := c.newID("fill-listy", "")
	st := State(c, id, func() fillListyState {
		return fillListyState{heights: map[any]float32{}, hIdx: map[int32]float32{}, ids: map[any]ID{}}
	})
	w := &ListyW{c: c, count: max(0, n), item: item, est: 46, hoverable: true, split: true, st: st, clicked: -1}
	w.configDefaults() // Defaults("Listy", ...): split, rowHoverable
	return w
}

// RowKey gives rows stable identities (default: the index).
func (w *ListyW) RowKey(fn func(i int) any) *ListyW { w.key = fn; return w }

// Group groups items by key(i) (items with equal keys are gathered in
// order of first appearance) under headers built by title(key, count).
func (w *ListyW) Group(key func(i int) string, title func(key string, count int)) *ListyW {
	w.groupKey, w.groupHead = key, title
	return w
}

// Sticky pins the current group header to the top while scrolling.
func (w *ListyW) Sticky() *ListyW { w.sticky = true; return w }

// Height fixes the viewport height; longer content scrolls.
func (w *ListyW) Height(px float32) *ListyW { w.height = px; return w }

// Virtual builds only the visible rows (needs Height).
func (w *ListyW) Virtual() *ListyW { w.virtual = true; return w }

// DataVersion tells the list that the data behind n, RowKey or Group
// changed without n changing (grouping and offsets are cached by n and
// this number).
func (w *ListyW) DataVersion(v int) *ListyW { w.version = v; return w }

// ItemHeight fixes every item's height (fast path; default: measured).
func (w *ListyW) ItemHeight(px float32) *ListyW { w.itemH = px; return w }

// EstimatedItemHeight is assumed for items not measured yet (default 46).
func (w *ListyW) EstimatedItemHeight(px float32) *ListyW { w.est = max(1, px); return w }

// RowHoverable toggles the row hover background (default on).
func (w *ListyW) RowHoverable(on bool) *ListyW { w.hoverable = on; return w }

// Split toggles the row dividers (default on).
func (w *ListyW) Split(on bool) *ListyW { w.split = on; return w }

// LoadMore adds a footer: a spinner while loading, "no more" when hasMore
// is false. EndReached reports when the user scrolled near it.
func (w *ListyW) LoadMore(hasMore, loading bool) *ListyW {
	w.more, w.hasMore, w.loading = true, hasMore, loading
	return w
}

// ScrollTo scrolls to a vertical pixel offset.
func (w *ListyW) ScrollTo(px float64) *ListyW {
	w.req = fillListyScroll{set: true, px: px, pxOnly: true}
	return w
}

// ScrollToKey scrolls to the item with key (RowKey) using align
// (AlignTop, AlignBottom or AlignAuto) plus offset pixels.
func (w *ListyW) ScrollToKey(key any, align string, offset float64) *ListyW {
	w.req = fillListyScroll{set: true, key: key, byKey: true, align: align, offset: offset}
	return w
}

// ScrollToGroup scrolls to a group header.
func (w *ListyW) ScrollToGroup(group string, align string, offset float64) *ListyW {
	w.req = fillListyScroll{set: true, group: group, byGroup: true, align: align, offset: offset}
	return w
}

// EndReached reports that the viewport is within 100px of the end while
// more data can be loaded (after Show).
func (w *ListyW) EndReached() bool { return w.nearEnd }

// Clicked returns the item clicked in Show.
func (w *ListyW) Clicked() (int, bool) { return w.clicked, w.clicked >= 0 }

// Range returns the rows [first, end) built by Show (headers count as rows).
func (w *ListyW) Range() (first, end int) { return w.first, w.end }

// Offset returns the scroll offset (0 without a viewport).
func (w *ListyW) Offset() float64 {
	if w.sb == nil {
		return 0
	}
	return w.sb.s.off.Y
}

// ID returns the viewport's ID (after Show; 0 without Height).
func (w *ListyW) ID() ID {
	if w.sb == nil {
		return 0
	}
	return w.sb.n.id
}

func (w *ListyW) keyOf(i int) any {
	if w.key != nil {
		return w.key(i)
	}
	return i
}

// fillListyGroups orders items by group (first appearance) and returns
// the rows with headers, the group keys and their sizes.
func fillListyGroups(n int, key func(i int) string) (rows []fillListyRow, groups []string, counts []int) {
	if key == nil {
		rows = make([]fillListyRow, n)
		for i := range rows {
			rows[i] = fillListyRow{item: int32(i), group: -1}
		}
		return rows, nil, nil
	}
	gi := map[string]int{}
	var members [][]int32
	for i := 0; i < n; i++ {
		k := key(i)
		g, ok := gi[k]
		if !ok {
			g = len(groups)
			gi[k] = g
			groups = append(groups, k)
			members = append(members, nil)
		}
		members[g] = append(members[g], int32(i))
	}
	rows = make([]fillListyRow, 0, n+len(groups))
	counts = make([]int, len(groups))
	for g, ms := range members {
		counts[g] = len(ms)
		rows = append(rows, fillListyRow{item: -1, group: int32(g)})
		for _, i := range ms {
			rows = append(rows, fillListyRow{item: i, group: int32(g)})
		}
	}
	return rows, groups, counts
}

func (w *ListyW) headerH() float32 { return 38 }

func (w *ListyW) footerH() float32 { return 48 }

func (w *ListyW) rowKey(r fillListyRow) any {
	if r.item < 0 {
		return fillListyHead{w.groups[r.group]}
	}
	return w.keyOf(int(r.item))
}

type fillListyHead struct{ g string }

// heightOf returns the (measured or assumed) height of row r.
func (w *ListyW) heightOf(r fillListyRow) float32 {
	if r.item < 0 {
		return w.headerH()
	}
	if w.itemH > 0 {
		return w.itemH
	}
	if w.key == nil {
		if h, ok := w.st.hIdx[r.item]; ok {
			return h
		}
		return w.est
	}
	k := w.rowKey(r)
	if h, ok := w.st.heights[k]; ok {
		return h
	}
	return w.est
}

// Show builds the list.
func (w *ListyW) Show() *ListyW {
	c, st := w.c, w.st
	rk := fillListyRowsKey{n: w.count, version: w.version, grouped: w.groupKey != nil}
	if rk != st.rowsKey || st.rows == nil {
		st.rows, st.groups, st.counts = fillListyGroups(w.count, w.groupKey)
		st.rowsKey = rk
		st.offs = nil
	}
	w.rows, w.groups, w.counts = st.rows, st.groups, st.counts
	// Collect heights measured in the last paint.
	if w.itemH <= 0 {
		for k, id := range st.ids {
			r, ok := c.rects[id]
			if !ok || r.H <= 0 {
				continue
			}
			if i, isIdx := k.(int); isIdx && w.key == nil {
				if st.hIdx[int32(i)] != r.H {
					st.hIdx[int32(i)] = r.H
					st.ver++
				}
			} else if st.heights[k] != r.H {
				st.heights[k] = r.H
				st.ver++
			}
		}
	}
	clear(st.ids)
	ok := fillListyOffsKey{rows: rk, ver: st.ver, itemH: w.itemH, est: w.est}
	if ok != st.offsKey || st.offs == nil {
		offs := st.offs[:0]
		offs = append(offs, 0)
		for _, r := range w.rows {
			offs = append(offs, offs[len(offs)-1]+float64(w.heightOf(r)))
		}
		st.offs, st.offsKey = offs, ok
	}
	offs := st.offs
	total := offs[len(offs)-1]
	if w.more {
		total += float64(w.footerH())
	}
	if w.height <= 0 {
		w.first, w.end = 0, len(w.rows)
		Column(c, func() { w.buildRows(0, len(w.rows)) })
		return w
	}
	StackBox(c, func() {
		if w.virtual {
			w.sb = fillViewport(c, "fill-listy", []layout.Option{layout.H(w.height), layout.Grow(1)})
		} else {
			b := c.box("fill-listy", layout.Column, []layout.Option{layout.H(w.height), layout.Grow(1)})
			b.n.ln.ScrollY = true
			w.sb = c.finishScroll(b, nil)
		}
		view := fillViewH(w.sb, w.height)
		w.applyScroll(offs, total, view)
		off := w.sb.s.off.Y
		if w.virtual {
			w.sb.s.extent.Y = total
			fillClampOff(w.sb, total, view)
			off = w.sb.s.off.Y
			w.first = max(0, fillFind(offs, off)-2)
			last := fillFind(offs, off+view)
			w.end = min(len(w.rows), last+3)
			if len(w.rows) == 0 {
				w.first, w.end = 0, 0
			}
			w.sb.s.base.Y = offs[w.first]
		} else {
			w.first, w.end = 0, len(w.rows)
		}
		w.sb.Semantics(a11y.RoleList, "")
		w.sb.Body(func() { w.buildRows(w.first, w.end) })
		w.nearEnd = w.more && w.hasMore && !w.loading && off+view >= total-100
		if w.sticky && len(w.groups) > 0 && off > 0 {
			w.stickyHeader(offs, off)
		}
	}, layout.H(w.height))
	return w
}

// applyScroll applies a pending ScrollTo* request.
func (w *ListyW) applyScroll(offs []float64, total, view float64) {
	q := w.req
	st := w.st
	if q.set {
		// Row heights below estimates are measured once the target is
		// built, so a key / group request is re-applied for a few frames.
		st.pending, st.pendingFrames = q, 4
		if q.pxOnly {
			st.pendingFrames = 0
		}
	} else if st.pendingFrames > 0 {
		q = st.pending
		st.pendingFrames--
	} else {
		return
	}
	s := w.sb.s
	target := -1.0
	var top, h float64
	switch {
	case q.pxOnly:
		target = q.px
	case q.byKey || q.byGroup:
		for i, r := range w.rows {
			hit := false
			if q.byKey {
				hit = r.item >= 0 && w.keyOf(int(r.item)) == q.key
			} else {
				hit = r.item < 0 && w.groups[r.group] == q.group
			}
			if hit {
				top, h = offs[i], offs[i+1]-offs[i]
				target = 0
				break
			}
		}
		if target < 0 {
			return
		}
		switch q.align {
		case AlignTop:
			target = top
		case AlignBottom:
			target = top + h - view
		default:
			target = s.off.Y
			if top < target {
				target = top
			} else if top+h > target+view {
				target = top + h - view
			}
		}
		target += q.offset
	}
	s.off.Y = max(0, min(target, total-view))
	w.c.invalid = true
}

func (w *ListyW) buildRows(first, end int) {
	for i := first; i < end; i++ {
		w.buildRow(w.rows[i])
	}
	if w.more && end == len(w.rows) {
		w.footer()
	}
}

func (w *ListyW) buildRow(r fillListyRow) {
	c, th, st := w.c, w.c.Theme, w.st
	k := w.rowKey(r)
	Key(c, k, func() {
		if r.item < 0 {
			w.header(r.group, false)
			return
		}
		i := int(r.item)
		opts := []layout.Option{layout.PadVH(12, 16)}
		if w.itemH > 0 {
			opts = append(opts, layout.H(w.itemH), layout.JustifyContent(layout.JustifyCenter))
		}
		b := c.box("fill-listy-item", layout.Column, opts)
		b.Semantics(a11y.RoleListItem, "")
		ia := c.interact(b.n, false, input.CursorDefault)
		if ia.Clicked {
			w.clicked = i
		}
		b.Body(func() { w.item(i) })
		hover := w.hoverable && ia.Hovered
		split := w.split
		b.Paint(func(l *paint.List, rr geom.Rect) {
			if hover {
				l.Fill(rr, geom.Radii{}, th.ItemHoverBg)
			}
			if split {
				l.Fill(geom.Rect{X: rr.X, Y: rr.Bottom() - 1, W: rr.W, H: 1}, geom.Radii{}, th.Split)
			}
		})
		st.ids[k] = b.ID()
	})
}

// header builds a group header (also used as the sticky copy).
func (w *ListyW) header(g int32, sticky bool, opts ...layout.Option) {
	c, th := w.c, w.c.Theme
	key := w.groups[g]
	opts = append([]layout.Option{layout.H(w.headerH()), layout.PadVH(0, 16), layout.JustifyContent(layout.JustifyCenter)}, opts...)
	b := c.box("fill-listy-group", layout.Column, opts)
	b.Semantics(a11y.RoleHeading, "")
	if sticky {
		b.A11yHidden() // a copy of the real header
	}
	if sticky {
		b.n.blocks = true
		c.interact(b.n, false, input.CursorDefault)
	}
	b.Body(func() {
		if w.groupHead != nil {
			w.groupHead(key, w.counts[g])
			return
		}
		Text(c, key).Weight(text.Semibold)
	})
	b.Paint(func(l *paint.List, r geom.Rect) {
		l.Fill(r, geom.Radii{}, th.BgContainer)
		l.Fill(r, geom.Radii{}, th.FillQuaternary)
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.Split)
	})
}

// stickyHeader pins the header of the group at the top of the viewport;
// the next header pushes it up as it arrives.
func (w *ListyW) stickyHeader(offs []float64, off float64) {
	row := fillFind(offs, off)
	if row < 0 {
		return
	}
	g := w.rows[row].group
	if g < 0 {
		return
	}
	top := float32(0)
	hh := float64(w.headerH())
	for i := row + 1; i < len(w.rows); i++ {
		if w.rows[i].item < 0 {
			if d := offs[i] - off; d < hh {
				top = float32(d - hh)
			}
			break
		}
	}
	if w.rows[row].item < 0 && offs[row] == off {
		return // the real header sits at the top
	}
	Key(w.c, fillListyHead{"\x00sticky"}, func() {
		w.header(g, true, layout.Abs(0, top, 0, layout.Unset))
	})
}

func (w *ListyW) footer() {
	c, th := w.c, w.c.Theme
	loc := c.Locale()
	Row(c, func() {
		switch {
		case w.loading:
			Icon(c, icon.Loading).Spin().Size(14).Color(th.Primary.Base)
			Text(c, loc.Loading).Tertiary()
		case !w.hasMore:
			Text(c, loc.NoMore).Tertiary()
		}
	}, layout.H(w.footerH()), layout.Center(), layout.Gap(8))
}
