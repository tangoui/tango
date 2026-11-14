package ui

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// tblRowVis is the visual state of a body row shared by its cells.
type tblRowVis struct {
	pos    int
	hov    bool
	sel    bool
	cur    bool
	stripe bool
	last   bool
	drag   bool
	style  *RowStyle // RowClass; nil: default look
}

const tblOverscan = 3

// setupScroll restores the body scroll offset, clamps it to the new
// content and resolves the visible row range. It runs before the header so
// header and body share one horizontal offset.
func (f *tblFrame) setupScroll(frameID ID) {
	t := f.t
	f.scroll = State[scrollState](f.c, hashID(frameID, "tbl-scroll"), nil)
	s := f.scroll
	f.bounded = t.maxH > 0
	f.viewH = s.view.H
	if f.viewH <= 0 || !f.bounded {
		f.viewH = t.maxH
	}
	f.offsets()
	contentH := f.offsetOf(f.n)
	s.off.X = max(0, min(s.off.X, float64(f.totalW-f.viewW)))
	if f.bounded {
		s.off.Y = max(0, min(s.off.Y, contentH-float64(f.viewH)))
	} else {
		s.off.Y = 0
	}
	f.sx, f.sy = float32(s.off.X), s.off.Y
	f.updateRange()
	if f.vh != nil {
		f.measureWindow()
	}
}

// varMode reports rows of variable height (a wrapping column).
func (f *tblFrame) varMode() bool {
	for _, li := range f.data {
		if col := f.leaves[li].col; col != nil && col.Wrap {
			return true
		}
	}
	return false
}

// varOffsets prepares the variable-height slots: measured heights are
// dropped when wrap widths or the data change, slots are rebuilt when the
// displayed rows or the detail panels change.
func (f *tblFrame) varOffsets() {
	t, st := f.t, f.st
	v := &st.vh
	f.vh = v
	var sb strings.Builder
	for _, li := range f.data {
		if lf := &f.leaves[li]; lf.col.Wrap {
			sb.WriteString(lf.key)
			sb.WriteByte(':')
			sb.WriteString(strconv.Itoa(int(lf.w)))
			sb.WriteByte(',')
		}
	}
	fmt.Fprintf(&sb, "|%d|%d|%g|%g|%g", t.rows, t.revision, f.fs, f.pv, f.rh)
	if sig := sb.String(); sig != v.sig {
		v.reset(t.rows)
		v.sig = sig
	}
	sb.Reset()
	fmt.Fprintf(&sb, "%d,%d,%t,%t|", f.start, f.n, f.ids != nil, f.tree != nil)
	if f.ids != nil || f.tree != nil {
		sb.WriteString(st.cacheSig)
	}
	if t.expRender != nil {
		for _, k := range *f.expKeys() {
			fmt.Fprintf(&sb, "|%s:%g", k, f.panelH(k))
		}
	}
	if ord := sb.String(); ord != v.ord || len(v.slot) != f.n {
		est := v.est(f.rh)
		v.build(f.n, func(p int) float32 {
			r := f.row(p)
			h := v.measured(r.data)
			if h == 0 {
				h = est
			}
			return h + f.panelExtra(r)
		})
		v.ord = ord
	}
}

// panelExtra is the detail panel height under an expanded row.
func (f *tblFrame) panelExtra(r tblRow) float32 {
	if f.t.expRender != nil && f.expSet[r.key] {
		return f.panelH(r.key)
	}
	return 0
}

// measureWindow measures the rows entering the viewport and keeps the
// first visible row in place when rows above it change height.
func (f *tblFrame) measureWindow() {
	v, s := f.vh, f.scroll
	// Scrolled to the end: stay there as rows turn out taller or shorter.
	atEnd := f.bounded && f.sy > 0 && f.sy >= v.offset(f.n)-float64(f.viewH)-0.5
	for iter := 0; iter < 4; iter++ {
		anchor := v.at(f.sy)
		delta := f.sy - v.offset(max(0, anchor))
		changed := false
		for pos := f.first; pos <= f.last; pos++ {
			r := f.row(pos)
			if v.measured(r.data) > 0 {
				continue
			}
			h := f.measureRow(r)
			v.record(r.data, h)
			v.set(pos, h+f.panelExtra(r))
			changed = true
		}
		if !changed {
			return
		}
		if f.bounded && f.sy > 0 && anchor >= 0 {
			sy := v.offset(anchor) + delta
			if atEnd {
				sy = v.offset(f.n) - float64(f.viewH)
			}
			sy = max(0, min(sy, v.offset(f.n)-float64(f.viewH)))
			s.off.Y, f.sy = sy, sy
		}
		f.updateRange()
	}
}

// measureRow returns the height of a row: the base height or the tallest
// wrapped cell.
func (f *tblFrame) measureRow(r tblRow) float32 {
	h := f.rh
	ts := f.font(text.Regular)
	for _, li := range f.data {
		lf := &f.leaves[li]
		col := lf.col
		if !col.Wrap || col.Text == nil || col.Render != nil {
			continue
		}
		w := lf.w - 2*f.ph
		if lf.di == 0 && f.tree != nil {
			w -= f.treeIndent(r)
		}
		s := col.Text(r.data)
		if s == "" {
			continue
		}
		lay := text.Shape(s, ts, text.Options{MaxWidth: max(1, w)})
		h = max(h, lay.Height+2*f.pv+1)
	}
	if d := r.data; d >= 0 && d < len(f.vh.rnd) && f.vh.rnd[d] > 0 {
		h = max(h, f.vh.rnd[d]+2*f.pv+1)
	}
	return float32(math.Ceil(float64(h)))
}

// treeIndent is the width of a tree row's indent and toggle.
func (f *tblFrame) treeIndent(r tblRow) float32 {
	sz := float32(17)
	if f.el {
		sz = 16
	}
	return float32(r.depth)*f.t.indent + sz + 8
}

// rowHeight is the height of the row at position pos (without its panel).
func (f *tblFrame) rowHeight(pos int) float32 {
	if f.vh == nil {
		return f.rh
	}
	if h := f.vh.measured(f.row(pos).data); h > 0 {
		return h
	}
	return f.vh.est(f.rh)
}

// noteRendered records the content height of a wrapped Render cell; a
// taller (or shorter) content re-measures the row next frame.
func (f *tblFrame) noteRendered(d int, h float32) {
	v := f.vh
	if v == nil || d < 0 {
		return
	}
	if d >= len(v.rnd) {
		v.rnd = append(v.rnd, make([]float32, d+1-len(v.rnd))...)
	}
	if math.Abs(float64(v.rnd[d]-h)) <= 0.5 {
		return
	}
	v.rnd[d] = h
	if old := v.measured(d); old > 0 {
		v.sum -= float64(old)
		v.cnt--
		v.h[d] = 0
	}
	f.c.animating = true // re-layout with the measured height
}

// window lays out only rows [first, last]: they start at 0 inside the
// body and the scroll state's base is the first row's content offset, so
// row positions stay exact at any scroll depth.
func (f *tblFrame) window() {
	s := f.scroll
	s.base, s.extent = vec64{}, vec64{}
	if f.bounded && f.n > 0 {
		s.base.Y = f.offsetOf(f.first)
		s.extent.Y = f.offsetOf(f.n)
	}
	f.ry = float32(f.sy - s.base.Y)
}

func (f *tblFrame) updateRange() {
	if f.bounded && f.vh != nil {
		f.first, f.last = f.vh.rangeOf(f.sy, f.viewH, tblOverscan)
	} else if f.bounded {
		f.first, f.last = tblRange(f.n, f.rh, f.offs, f.sy, f.viewH, tblOverscan)
	} else {
		f.first, f.last = 0, f.n-1
	}
}

// offsets builds prefix offsets when expanded detail panels make row
// heights uneven.
func (f *tblFrame) offsets() {
	f.offs, f.vh = nil, nil
	if f.varMode() {
		f.varOffsets()
		return
	}
	if f.t.expRender == nil || len(f.expSet) == 0 || f.n == 0 {
		return
	}
	offs := make([]float64, f.n+1)
	y := float64(0)
	for i := 0; i < f.n; i++ {
		offs[i] = y
		y += float64(f.rh)
		if r := f.row(i); f.expSet[r.key] {
			y += float64(f.panelH(r.key))
		}
	}
	offs[f.n] = y
	f.offs = offs
}

func (f *tblFrame) panelH(key string) float32 {
	if h, ok := f.st.panelH[key]; ok {
		return h
	}
	return 80
}

func (f *tblFrame) offsetOf(i int) float64 {
	if f.vh != nil {
		return f.vh.offset(i)
	}
	return tblOffset(i, f.rh, f.offs)
}

// posAt returns the row position at content y.
func (f *tblFrame) posAt(y float64) int {
	if f.n == 0 {
		return -1
	}
	var p int
	if f.vh != nil {
		p = f.vh.at(y)
	} else if f.offs == nil {
		p = int(math.Floor(y / float64(f.rh)))
	} else {
		p = sort.Search(f.n, func(i int) bool { return f.offs[i+1] > y })
	}
	return max(0, min(p, f.n-1))
}

// ensureVisible scrolls so row pos and leaf li are inside the viewport.
func (f *tblFrame) ensureVisible(pos, li int) {
	s := f.scroll
	if f.bounded && pos >= 0 && pos < f.n {
		top := f.offsetOf(pos)
		rh := f.rowHeight(pos)
		if top < s.off.Y {
			s.off.Y = top
		} else if top+float64(rh) > s.off.Y+float64(f.viewH) {
			s.off.Y = top + float64(rh-f.viewH)
		}
	}
	if li >= 0 && li < len(f.leaves) && f.leaves[li].fixed == tfNone {
		lf := f.leaves[li]
		lw, rw := f.fixedWidths()
		if float64(lf.x-lw) < s.off.X {
			s.off.X = float64(max(0, lf.x-lw))
		} else if float64(lf.x+lf.w) > s.off.X+float64(f.viewW-rw) {
			s.off.X = float64(min(f.totalW-f.viewW, lf.x+lf.w-f.viewW+rw))
		}
	}
	if float32(s.off.X) != f.sx || s.off.Y != f.sy {
		f.sx, f.sy = float32(max(0, s.off.X)), max(0, s.off.Y)
		f.updateRange()
		f.c.invalid = true
	}
}

func (f *tblFrame) fixedWidths() (l, r float32) {
	for _, lf := range f.leaves {
		switch lf.fixed {
		case tfLeft:
			l += lf.w
		case tfRight:
			r += lf.w
		}
	}
	return
}

// body builds the scrolling body.
func (f *tblFrame) body() {
	c, t, th, st := f.c, f.t, f.th, f.st
	b := c.box("tbody", layout.Column, nil)
	b.n.ln.ScrollX = true
	if f.bounded {
		b.n.ln.ScrollY = true
		b.n.ln.MaxH = t.maxH
	}
	b.n.scroll = f.scroll
	// The table sets the scroll state itself: hand it the body's layout
	// node so scroll.go maps the logical offset in RTL (offset 0 shows the
	// right end, wheel and scrollbar deltas flip).
	f.scroll.ln = &b.n.ln
	if t.bordered && !f.el && len(t.summary) == 0 {
		b.n.radii = geom.Radii{BL: th.RadiusLG, BR: th.RadiusLG}
	}
	f.rowPaint = t.span == nil
	f.bodyID = b.n.id
	f.bodyRect = c.rects[b.n.id]
	f.dragUpdate()
	f.window()
	b.Body(func() {
		if f.n == 0 {
			f.empty()
			return
		}
		cells := f.spanWindow()
		k := 0
		for pos := f.first; pos <= f.last; pos++ {
			// cells is ordered by owner position; owners above the window
			// come first and attach to the first row.
			j := k
			for j < len(cells) && (cells[j].pos <= pos) {
				j++
			}
			rowCells := cells[k:j]
			k = j
			r := f.row(pos)
			Key(c, r.key, func() { f.buildRow(pos, r, rowCells) })
			if t.expRender != nil && f.expSet[r.key] {
				Key(c, r.key, func() { f.panel(r) })
			}
		}
		f.scrollThumbs()
	})
	if st.drag.on {
		b.PaintOver(func(l *paint.List, r geom.Rect) {
			tp := f.dragTarget()
			if tp < 0 {
				return
			}
			yc := f.offsetOf(tp)
			if tp > st.drag.fromPos {
				yc += float64(f.rowHeight(tp))
			}
			y := r.Y + float32(yc-f.sy)
			l.Fill(geom.Rect{X: r.X, Y: y - 1, W: r.W, H: 2}, geom.Radii{}, th.Primary.Base)
		})
	}
}

// empty draws the empty state across the visible width.
func (f *tblFrame) empty() {
	c, th, t := f.c, f.th, f.t
	box := Column(c, func() {
		if f.el {
			desc := t.emptyText
			if desc == "" {
				desc = c.Locale().NoData
			}
			Text(c, desc).Color(th.TextTertiary)
			return
		}
		e := Empty(c).Simple()
		if t.emptyText != "" {
			e.Description(t.emptyText)
		}
	}, layout.W(f.viewW), layout.Center(), layout.PadVH(16, 0), layout.MinH(60))
	box.n.ln.Margin.Left = f.sx
	if !f.el {
		box.Style(layout.PadVH(32, 0))
	}
	box.Paint(func(l *paint.List, r geom.Rect) {
		if !f.el {
			l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, f.spl)
		}
	})
}

// spanWindow resolves the cells to draw for the visible rows.
func (f *tblFrame) spanWindow() []tblCell {
	t := f.t
	nl := len(f.leaves)
	if t.span == nil {
		return tblSpanCells(f.first, f.last, nl, nil, 0)
	}
	cache := map[int]tblRow{}
	rowAt := func(p int) tblRow {
		if r, ok := cache[p]; ok {
			return r
		}
		r := f.row(p)
		cache[p] = r
		return r
	}
	fn := func(pos, li int) (int, int) {
		lf := &f.leaves[li]
		if lf.kind != tkData {
			return 1, 1
		}
		rs, cs := t.span(TableSpan{Row: rowAt(pos).data, Pos: pos, Col: lf.di, Key: lf.key})
		if rs > 1 {
			rs = min(rs, f.n-pos)
		}
		return rs, cs
	}
	cells := tblSpanCells(f.first, f.last, nl, fn, 64)
	sort.SliceStable(cells, func(i, j int) bool { return cells[i].pos < cells[j].pos })
	return cells
}

func (f *tblFrame) rowBg(rv *tblRowVis, sorted bool) theme.Color {
	th, tk, t := f.th, f.tk, f.t
	switch {
	case rv.sel && tk.RowSelectedBg.A > 0:
		if rv.hov && tk.RowSelectedHoverBg.A > 0 {
			return tk.RowSelectedHoverBg
		}
		return tk.RowSelectedBg
	case rv.cur:
		return tk.RowCurrentBg
	case rv.hov && !t.noHover:
		return tk.RowHoverBg
	case rv.style != nil && rv.style.Bg.A > 0:
		return rv.style.Bg
	case rv.style != nil && rv.style.Status != "":
		return th.ColorOf(rv.style.Status).Bg
	case rv.stripe:
		return tk.RowStripedBg
	case sorted && !f.el:
		return tk.BodySortBg
	}
	return theme.Color{}
}

// rowStyle returns the RowClass style of data row d (nil without one or
// for the default look).
func (f *tblFrame) rowStyle(d int) *RowStyle {
	if f.t.rowStyle == nil {
		return nil
	}
	s := f.t.rowStyle(d)
	if s == (RowStyle{}) {
		return nil
	}
	return &s
}

// buildRow builds one body row: a fixed-height strip whose cells are
// absolutely positioned (normal cells first, then sticky ones on top).
func (f *tblFrame) buildRow(pos int, r tblRow, cells []tblCell) {
	c, t, st := f.c, f.t, f.st
	n := c.add("tr", "")
	n.label = "row:" + r.key
	n.ln.Width = max(f.totalW, 1)
	n.ln.Height = f.rowHeight(pos)
	ia := c.interact(n, false, input.CursorDefault)
	// Row semantics: field stores only; the position (and tree level) is
	// attached when accessibility is on. The name comes from the cells.
	n.setRole(a11y.RoleRow).setName("")
	if f.tree != nil && r.kids {
		n.setState(a11y.Expandable).stateIf(r.open, a11y.Expanded)
	} else if t.expRender != nil && f.canExpand(r) {
		n.setState(a11y.Expandable).stateIf(f.expSet[r.key], a11y.Expanded)
	}
	if c.a11yOn {
		lv := 0
		if f.tree != nil {
			lv = r.depth + 1
		}
		n.fill(tblSem{row: f.depth + pos + 1, level: lv})
	}
	f.rowToggle = 0
	rv := &tblRowVis{
		pos:    pos,
		hov:    ia.Hovered && !t.noHover && !st.drag.on,
		sel:    f.hasSelection() && f.selected(r),
		cur:    t.highlight && st.curOn && st.cur == r.key,
		stripe: t.striped && pos%2 == 1,
		last:   pos == f.n-1,
		drag:   st.drag.on && st.drag.fromPos == pos,
	}
	rv.style = f.rowStyle(r.data)
	if f.hasSelection() {
		n.setState(a11y.Selectable).stateIf(rv.sel, a11y.Selected)
	}
	if f.rowPaint {
		n.paint = func(l *paint.List, rr geom.Rect) { l.Fill(rr, geom.Radii{}, f.rowBg(rv, false)) }
		n.over = func(l *paint.List, rr geom.Rect) {
			l.Fill(geom.Rect{X: rr.X, Y: rr.Bottom() - 1, W: rr.W, H: 1}, geom.Radii{}, f.spl)
		}
	}
	var cellIDs []ID
	var cellLeaf []int
	c.body(n, func() {
		for pass := int8(0); pass < 3; pass++ {
			for _, ce := range cells {
				fx := f.leaves[ce.col].fixed
				if (pass == 0 && fx != tfNone) || (pass == 1 && fx != tfLeft) || (pass == 2 && fx != tfRight) {
					continue
				}
				top := float32(0)
				owner := r
				opos := pos
				if ce.pos != pos {
					top = float32(f.offsetOf(ce.pos) - f.offsetOf(pos))
					owner = f.row(ce.pos)
					opos = ce.pos
				}
				var orv *tblRowVis
				if opos == pos {
					orv = rv
				} else {
					orv = &tblRowVis{pos: opos, sel: f.hasSelection() && f.selected(owner), stripe: t.striped && opos%2 == 1, style: f.rowStyle(owner.data)}
				}
				if id, ok := f.cell(owner, opos, ce, top, orv); ok {
					cellIDs = append(cellIDs, id)
					cellLeaf = append(cellLeaf, ce.col)
				}
			}
		}
	})
	// Expand / Collapse actions on the row click its toggle.
	n.aTarget = f.rowToggle
	if ia.PressedNow {
		// Activate the cell under the pointer (fixed cells sit on top, so
		// search from the end).
		for i := len(cellIDs) - 1; i >= 0; i-- {
			if rr, ok := c.rects[cellIDs[i]]; ok && rr.Contains(c.pointer) {
				st.actPos, st.actCol, st.actOn, st.kbd = pos, cellLeaf[i], true, false
				break
			}
		}
		c.SetFocus(f.focusID())
	}
	if ia.Clicked {
		t.clickRow = r.data
		if ia.Clicks == 2 {
			t.dblRow = r.data
		}
		if t.highlight {
			st.cur, st.curOn = r.key, true
		}
		if st.actOn && st.actPos == pos {
			f.maybeStartEdit(r, st.actCol)
		}
		c.invalid = true
	}
}

// cell builds one body cell; it returns the cell's ID unless culled.
func (f *tblFrame) cell(r tblRow, pos int, ce tblCell, top float32, rv *tblRowVis) (ID, bool) {
	c, t, th, st := f.c, f.t, f.th, f.st
	lf := &f.leaves[ce.col]
	w := f.spanW(ce.col, ce.cs)
	if f.culled(ce.col, w) {
		return 0, false
	}
	h := f.rowHeight(ce.pos)
	if ce.rs > 1 {
		h = float32(f.offsetOf(ce.pos+ce.rs) - f.offsetOf(ce.pos))
		if f.offs != nil {
			// Detail panels inside a merged range are not covered.
			h = float32(ce.rs) * f.rh
		} else if f.vh != nil {
			h = 0
			for p := ce.pos; p < min(f.n, ce.pos+ce.rs); p++ {
				h += f.rowHeight(p)
			}
		}
	}
	col, arg, rowHead := lf.col, r.data, false
	if t.tp != nil && col != nil {
		// Transposed: the field column of this row, called with the
		// record of this column (table_transpose.go).
		col, arg, rowHead = t.tp.tpCell(lf, r.data)
	}
	n := c.add("td", "")
	if rowHead {
		n.setRole(a11y.RoleRowHeader)
	} else {
		n.setRole(a11y.RoleCell)
	}
	if c.a11yOn {
		n.fill(tblSem{row: f.depth + ce.pos + 1, col: ce.col + 1})
	}
	if st.actOn && st.actPos == pos && st.actCol >= ce.col && st.actCol < ce.col+ce.cs {
		f.actID = n.id
	}
	n.ln.Absolute = true
	n.ln.Pos = flexbox.Pos{Left: f.cx(ce.col), Top: top, Right: layout.Unset, Bottom: layout.Unset}
	n.ln.Width, n.ln.Height = w, h
	n.ln.Dir = layout.Row
	n.ln.Align = layout.AlignCenter
	n.ln.Padding = geom.VH(0, f.ph)
	n.ln.Gap = 8
	align := AlignStart
	if col != nil {
		align = col.Align
	}
	if lf.kind != tkData {
		align = AlignCenter
	}
	n.ln.Justify = f.justify(align)
	sorted := col != nil && f.sortOf[col.Key] != SortNone
	lastLeaf := ce.col+ce.cs-1 == len(f.leaves)-1
	fixed := lf.fixed != tfNone
	var txt string
	var txtX float32 // extra start offset for painted text (tree prefix)
	painted := false
	editing := false
	c.body(n, func() {
		switch lf.kind {
		case tkSelect:
			f.selectCell(r, pos)
		case tkExpand:
			f.expandCell(r)
		case tkDrag:
			f.dragHandle(r, pos)
		case tkData:
			if lf.di == 0 && f.tree != nil {
				n.ln.Gap = 0
				txtX = f.treePrefix(r)
			}
			if col.Editor != nil && f.isEditing(r, col) {
				editing = true
				f.editor(r, col, w-2*f.ph-txtX, h)
				return
			}
			if col.Render != nil {
				if col.Wrap && f.vh != nil {
					in := c.add("td-wrap", "")
					in.ln.Dir = layout.Row
					in.ln.Wrap = true
					in.ln.Gap = 8
					in.ln.Align = layout.AlignCenter
					in.ln.Width = max(1, w-2*f.ph-txtX)
					c.body(in, func() { col.Render(r.data) })
					if rr, ok := c.rects[in.id]; ok && rr.H > 0 {
						f.noteRendered(r.data, rr.H)
					}
					return
				}
				col.Render(arg)
				return
			}
			if col.Text != nil {
				txt = col.Text(arg)
				painted = true
			}
		}
	})
	if len(n.kids) > 0 && !painted {
		n.clip = true
	}
	ts := f.font(text.Regular)
	tc := f.tk.CellColor
	if rowHead {
		ts, tc = f.font(themeWeight(f.tk.HeaderFontWeight)), f.tk.HeaderColor
	}
	if rs := rv.style; rs != nil {
		if rs.Text.A > 0 {
			tc = rs.Text
		}
		if rs.Bold {
			ts = f.font(text.Medium)
		}
	}
	innerW := w - 2*f.ph - txtX
	wrap := painted && col.Wrap
	if painted && col.Ellipsis && !wrap && rv.hov && !editing {
		if rr, ok := c.rects[n.id]; ok && rr.Contains(c.pointer) && f.truncated(txt, innerW, ts) {
			f.tip(n.id, txt, false)
		}
	}
	if painted {
		n.label = txt
	}
	active := func() bool {
		return f.focused && st.kbd && st.actOn && st.actPos == pos && st.actCol >= ce.col && st.actCol < ce.col+ce.cs
	}
	rowPaint := f.rowPaint && ce.pos == pos
	n.paint = func(l *paint.List, rr geom.Rect) {
		switch {
		case rowHead:
			f.tpHeadPaint(l, rr, ce.pos)
		case fixed || !rowPaint:
			if fixed {
				l.Fill(rr, geom.Radii{}, th.BgContainer)
			}
			l.Fill(rr, geom.Radii{}, f.rowBg(rv, sorted))
			l.Fill(geom.Rect{X: rr.X, Y: rr.Bottom() - 1, W: rr.W, H: 1}, geom.Radii{}, f.spl)
		case sorted:
			if f.rowBg(rv, false).A == 0 {
				l.Fill(geom.Rect{X: rr.X, Y: rr.Y, W: rr.W, H: rr.H - 1}, geom.Radii{}, f.rowBg(rv, true))
			}
		}
		s := f.inl(rr)
		if t.bordered && !lastLeaf {
			l.Fill(geom.Rect{X: s.X(rr.W-1, 1), Y: rr.Y, W: 1, H: rr.H}, geom.Radii{}, f.spl)
		}
		if painted {
			op := false
			if rv.drag {
				l.Opacity(0.45)
				op = true
			}
			tx := s.X(f.ph+txtX, innerW)
			if wrap {
				f.drawWrapped(l, txt, tx, rr.Y, innerW, rr.H, align, ts, tc)
			} else {
				f.drawText(l, txt, tx, rr.Y, innerW, rr.H, align, ts, tc)
			}
			if op {
				l.Pop()
			}
		}
	}
	n.over = func(l *paint.List, rr geom.Rect) {
		if active() {
			l.Stroke(rr.Inset(geom.All(0.5)), geom.Radii{}, 2, th.Primary.Base)
		}
	}
	return n.id, true
}

// selectCell builds the checkbox or radio of a row.
func (f *tblFrame) selectCell(r tblRow, pos int) {
	c, t := f.c, f.t
	dis := !f.selectable(r)
	on := f.selected(r)
	if t.single {
		rw := Radio(c, "", on).Disabled(dis)
		f.rowControl(rw.n, pos)
		if rw.Clicked() && !on {
			f.setSelected(r, true)
		}
		return
	}
	v := on
	half := f.linked && !on && f.half[r.data]
	cb := Checkbox(c, "", &v).Indeterminate(half).Disabled(dis)
	f.rowControl(cb.n, pos)
	if cb.Changed() {
		f.setSelected(r, v)
	}
}

// rowControl annotates a row's selection checkbox or radio: it is not a
// Tab stop of its own (the grid's Space toggles the selection of the
// active row) and is named after the row when accessibility is on.
func (f *tblFrame) rowControl(n *node, pos int) {
	n.noTab, n.aKbd = true, true
	if f.c.a11yOn {
		n.setName(locFmt(f.c.Locale().Table.SelectRowFmt, f.start+pos+1))
	}
}

func (f *tblFrame) canExpand(r tblRow) bool {
	return f.t.expAllow == nil || f.t.expAllow(r.data)
}

// expandCell builds the expand toggle.
func (f *tblFrame) expandCell(r tblRow) {
	c, t := f.c, f.t
	if !f.canExpand(r) {
		return
	}
	open := f.expSet[r.key]
	n := c.add("expand-icon", "expand:"+r.key)
	n.ln.Width, n.ln.Height = 17, 17
	ia := c.interact(n, false, input.CursorPointer)
	if ia.Clicked {
		ks := f.expKeys()
		*ks = tblToggle(*ks, r.key, !open)
		open = !open
		f.expSet = keySet(*ks)
		t.expCh = true
		c.invalid = true
	}
	f.toggleSem(n, open)
	a := c.Anim(n.id, "open", b2f(open), f.th.DurationMid)
	n.paint = func(l *paint.List, rr geom.Rect) { tblDrawExpand(f.th, l, rr, a, ia.Hovered, f.rtl) }
}

// toggleSem annotates an expand / tree toggle. It is pointer-only: the
// grid's Enter toggles the active row, and the row's Expand / Collapse
// actions click it (aTarget).
func (f *tblFrame) toggleSem(n *node, open bool) {
	loc := f.c.Locale()
	name := loc.ExpandRow
	if open {
		name = loc.CollapseRow
	}
	n.setRole(a11y.RoleButton).setName(name).setState(a11y.Expandable).stateIf(open, a11y.Expanded)
	n.aKbd = true
	f.rowToggle = n.id
}

// treePrefix builds indent and toggle of a tree row and returns its width.
func (f *tblFrame) treePrefix(r tblRow) float32 {
	c, t, st := f.c, f.t, f.st
	ind := float32(r.depth) * t.indent
	if ind > 0 {
		Gap(c, ind)
	}
	sz := float32(17)
	if f.el {
		sz = 16
	}
	n := c.add("tree-toggle", "tree:"+r.key)
	n.ln.Width, n.ln.Height = sz, sz
	n.ln.Margin.Right = 8
	open := r.open
	loading := false
	if r.kids {
		ia := c.interact(n, false, input.CursorPointer)
		if ia.Clicked {
			ks := f.openKeys()
			*ks = tblToggle(*ks, r.key, !open)
			open = !open
			t.expCh = true
			c.invalid = true
		}
		f.toggleSem(n, open)
		if open && t.treeLoad != nil && len(t.treeKids(r.data)) == 0 {
			if !st.loadReq[r.key] {
				st.loadReq[r.key] = true
				t.treeLoad(r.data)
				c.invalid = true
			}
			loading = len(t.treeKids(r.data)) == 0
		} else if len(t.treeKids(r.data)) > 0 {
			delete(st.loadReq, r.key)
		}
		a := c.Anim(n.id, "open", b2f(open), f.th.DurationMid)
		hov := ia.Hovered
		n.paint = func(l *paint.List, rr geom.Rect) {
			if loading {
				drawSpinner(c, l, rr.Inset(geom.All(1.5)))
				return
			}
			tblDrawExpand(f.th, l, rr, a, hov, f.rtl)
		}
	}
	return ind + sz + 8
}

// panel builds the expanded detail panel under a row.
func (f *tblFrame) panel(r tblRow) {
	c, th, t, st := f.c, f.th, f.t, f.st
	p := c.box("tr-expand", layout.Column, nil)
	p.n.ln.Width = f.viewW
	p.n.ln.Margin.Left = f.sx
	if f.el {
		p.n.ln.Padding = geom.VH(20, 50)
	} else {
		p.n.ln.Padding = geom.VH(f.pv, f.ph)
	}
	p.Body(func() { t.expRender(r.data) })
	p.Paint(func(l *paint.List, rr geom.Rect) {
		l.Fill(rr, geom.Radii{}, th.BgContainer)
		if !f.el {
			l.Fill(rr, geom.Radii{}, th.FillAlter)
		}
		l.Fill(geom.Rect{X: rr.X, Y: rr.Bottom() - 1, W: rr.W, H: 1}, geom.Radii{}, f.spl)
	})
	if rr, ok := c.rects[p.n.id]; ok && rr.H > 0 {
		if old, ok := st.panelH[r.key]; !ok || math.Abs(float64(old-rr.H)) > 0.5 {
			st.panelH[r.key] = rr.H
			c.animating = true // re-run with the measured height
		}
	}
}

// dragHandle builds the drag-sort holder of a row.
func (f *tblFrame) dragHandle(r tblRow, pos int) {
	c, th, st := f.c, f.th, f.st
	n := c.add("drag-handle", "drag:"+r.key)
	n.setRole(a11y.RoleButton).setName(c.Locale().A11y.DragHandle)
	n.aKbd = true // Alt+Up / Alt+Down on the grid move the active row
	n.ln.Width, n.ln.Height = 20, 20
	ia := c.interact(n, false, input.CursorMove)
	if ia.PressedNow {
		st.drag = tblDrag{on: true, id: n.id, from: r.data, fromPos: pos}
	}
	n.paint = func(l *paint.List, rr geom.Rect) {
		col := th.TextTertiary
		if ia.Hovered {
			col = th.Text
		}
		icon.Draw(l, icon.Holder, rr.Inset(geom.All(2)), col, 0)
	}
}

// dragTarget is the row position under the pointer while dragging.
func (f *tblFrame) dragTarget() int {
	br := f.bodyRect
	if br.Empty() {
		return -1
	}
	return f.posAt(float64(f.c.pointer.Y-br.Y) + f.sy)
}

// dragUpdate finishes a drag-sort when the pointer is released.
func (f *tblFrame) dragUpdate() {
	c, st, t := f.c, f.st, f.t
	if !st.drag.on {
		return
	}
	if c.active == st.drag.id {
		f.autoScroll() // still dragging
		return
	}
	st.drag.last = time.Time{}
	to := f.dragTarget()
	st.drag.on = false
	if to >= 0 && to != st.drag.fromPos && t.dragSort != nil {
		t.dragSort(st.drag.from, f.row(to).data)
		t.movedCh = true
		c.invalid = true
	}
}

// autoScroll scrolls while a dragged row is held near the top or bottom
// edge of the scrolling area (the bounded body, or the scroll container of
// an unbounded table); the speed grows towards the edge.
func (f *tblFrame) autoScroll() {
	c, st := f.c, f.st
	const edge, speed = 40, 1200 // px, px per second at the edge
	var s *scrollState
	var top, bot float32
	maxY := float64(0)
	if f.bounded {
		s, top, bot = f.scroll, f.bodyRect.Y, f.bodyRect.Bottom()
		maxY = f.offsetOf(f.n) - float64(f.viewH)
	} else if p := scrollAncestor(f.frameN); p != nil {
		v, ok := c.rects[p.id]
		if !ok {
			return
		}
		s, top, bot = p.scroll, v.Y, v.Bottom()
		maxY = s.maxY()
	} else {
		return
	}
	if bot-top < 2*edge {
		return
	}
	py := c.pointer.Y
	d := float32(0)
	switch {
	case py < top+edge:
		d = -min(1, (top+edge-py)/edge)
	case py > bot-edge:
		d = min(1, (py-(bot-edge))/edge)
	}
	now := c.Now()
	if d == 0 {
		st.drag.last = time.Time{}
		return
	}
	c.WakeAt(now.Add(16 * time.Millisecond))
	if st.drag.last.IsZero() {
		st.drag.last = now
		return
	}
	dt := now.Sub(st.drag.last).Seconds()
	if dt <= 0 {
		return
	}
	st.drag.last = now
	dt = min(dt, 0.1)
	ny := max(0, min(maxY, s.off.Y+float64(d)*speed*dt))
	if ny == s.off.Y {
		return
	}
	s.off.Y = ny
	if f.bounded {
		f.sy = ny
		f.updateRange()
		if f.vh != nil {
			f.measureWindow()
		}
	}
	c.invalid = true
}

// scrollThumbs paints scrollbars and lets the user drag them.
func (f *tblFrame) scrollThumbs() {
	c, th, s := f.c, f.th, f.scroll
	contentH := f.offsetOf(f.n)
	vw, vh := f.viewW, f.viewH
	if !f.bounded {
		vh = float32(contentH)
	}
	pIn := f.bodyRect.Contains(c.pointer) && c.pointerIn
	thumb := func(kind string, vertical bool) {
		var track float32
		var content, off float64
		if vertical {
			track, content, off = vh, contentH, f.sy
		} else {
			track, content, off = vw, float64(f.totalW), float64(f.sx)
		}
		if content <= float64(track)+0.5 || track <= 0 {
			return
		}
		size := max(24, float32(float64(track)*float64(track)/content))
		pos := (track - size) * float32(off/(content-float64(track)))
		n := c.add(kind, "")
		n.aHidden = true // the grid scrolls with the keyboard (arrows, PageUp/PageDown)
		n.ln.Absolute = true
		if vertical {
			n.ln.Pos = flexbox.Pos{Left: f.sx + vw - 10, Top: f.ry + pos, Right: layout.Unset, Bottom: layout.Unset}
			n.ln.Width, n.ln.Height = 10, size
		} else {
			n.ln.Pos = flexbox.Pos{Left: f.sx + pos, Top: f.ry + vh - 10, Right: layout.Unset, Bottom: layout.Unset}
			n.ln.Width, n.ln.Height = size, 10
		}
		ia := c.interact(n, false, input.CursorDefault)
		st := State[tblThumb](c, n.id, nil)
		if ia.PressedNow {
			st.start, st.off = c.pointer, off
		}
		drag := c.Dragging(n.id)
		if drag {
			d := c.pointer.Y - st.start.Y
			if !vertical {
				// The thumb tracks the logical offset: in RTL it starts at
				// the right end and moves left as the offset grows.
				d = (c.pointer.X - st.start.X) * f.inl(geom.Rect{}).Sign()
			}
			span := content - float64(track)
			nv := max(0, min(span, st.off+float64(d)*span/float64(track-size)))
			if vertical {
				s.off.Y = nv
			} else {
				s.off.X = nv
			}
			c.invalid = true
		}
		show := c.Anim(n.id, "show", b2f(pIn || drag), th.DurationMid)
		n.paint = func(l *paint.List, rr geom.Rect) {
			if show <= 0.01 {
				return
			}
			a := float64(show) * 0.5
			if drag || ia.Hovered {
				a = 0.75
			}
			col := theme.Fade(th.TextTertiary, a)
			if vertical {
				// 1px from the box's outer edge (the table's end edge).
				l.Fill(geom.Rect{X: f.inl(rr).X(3, 6), Y: rr.Y + 2, W: 6, H: rr.H - 4}, geom.Rad(3), col)
			} else {
				l.Fill(geom.Rect{X: rr.X + 2, Y: rr.Y + 3, W: rr.W - 4, H: 6}, geom.Rad(3), col)
			}
		}
	}
	if f.bounded {
		thumb("vthumb", true)
	}
	thumb("hthumb", false)
}

type tblThumb struct {
	start geom.Point
	off   float64
}

// summaryBar builds the summary rows under the body (horizontally synced).
func (f *tblFrame) summaryBar() {
	c, t, th := f.c, f.t, f.th
	rows := t.vis
	for si, fn := range t.summary {
		vals := make([]string, len(f.leaves))
		for li := range f.leaves {
			if f.leaves[li].col != nil {
				vals[li] = fn(f.leaves[li].key, rows)
			}
		}
		// The first non-empty text may spill over leading special columns.
		bar := c.box("tfoot", layout.Column, nil)
		bar.n.setRole(a11y.RoleRow)
		if c.a11yOn {
			bar.n.fill(tblSem{row: f.depth + f.n + si + 1})
		}
		bar.n.ln.Height = f.rh
		bar.n.clip = true
		top := si == 0
		bar.Body(func() {
			for pass := int8(0); pass < 3; pass++ {
				for li := range f.leaves {
					lf := &f.leaves[li]
					if (pass == 0 && lf.fixed != tfNone) || (pass == 1 && lf.fixed != tfLeft) || (pass == 2 && lf.fixed != tfRight) {
						continue
					}
					if f.culled(li, lf.w) {
						continue
					}
					v := vals[li]
					align := AlignStart
					if lf.col != nil {
						align = lf.col.Align
					}
					last := li == len(f.leaves)-1
					n := c.add("tf", v)
					n.setRole(a11y.RoleCell)
					if c.a11yOn {
						n.fill(tblSem{row: f.depth + f.n + si + 1, col: li + 1})
					}
					n.ln.Absolute = true
					n.ln.Pos = flexbox.Pos{Left: f.cx(li) - f.sx, Top: 0, Right: layout.Unset, Bottom: layout.Unset}
					n.ln.Width, n.ln.Height = lf.w, f.rh
					n.paint = func(l *paint.List, rr geom.Rect) {
						// Opaque: fixed cells cover the scrolled ones.
						l.Fill(rr, geom.Radii{}, f.tk.FooterBg)
						l.Fill(geom.Rect{X: rr.X, Y: rr.Bottom() - 1, W: rr.W, H: 1}, geom.Radii{}, f.spl)
						if t.bordered && !last {
							l.Fill(geom.Rect{X: f.inl(rr).X(rr.W-1, 1), Y: rr.Y, W: 1, H: rr.H}, geom.Radii{}, f.spl)
						}
						tc := f.tk.FooterColor
						f.drawText(l, v, rr.X+f.ph, rr.Y, rr.W-2*f.ph, rr.H, align, f.font(text.Regular), tc)
					}
				}
			}
		})
		bar.Bg(th.BgContainer)
		if f.footY < 0 {
			bar.Transform(geom.Translate(0, f.footY))
		}
		if top && (f.bounded || f.footY < 0) {
			bar.PaintOver(func(l *paint.List, rr geom.Rect) {
				if f.footY < 0 || f.sy < f.offsetOf(f.n)-float64(f.viewH)-0.5 {
					l.Fill(geom.Rect{X: rr.X, Y: rr.Y, W: rr.W, H: 1}, geom.Radii{}, f.spl)
				}
			})
		}
	}
}
