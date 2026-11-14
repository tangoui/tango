package ui

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// tblState is the retained per-table state.
type tblState struct {
	viewW, viewH float32
	widths       map[string]float32
	sort         []SortState
	sortInit     bool
	filters      map[string][]string
	sel          []string
	expanded     []string
	treeOpen     []string
	loadReq      map[string]bool
	panelH       map[string]float32
	colState     TableColumnState

	// Filter dropdown.
	fOpen   string
	fDraft  []string
	fQuery  string
	fPanel  ID
	fAnchor ID

	// Column resizing.
	rzKey      string
	rzX, rzW   float32
	rzHandleID ID

	// Keyboard focus cell.
	actPos, actCol int
	actOn          bool
	kbd            bool // active cell moved by keyboard: show its ring
	cur            string
	curOn          bool

	edit      tblEdit
	rowDrafts map[string]map[string]*tblDraft // row key → column key → draft

	filterInit bool   // DefaultFilter applied
	expInit    bool   // DefaultExpandAllRows applied
	lastSel    string // key of the last row checked by pointer (Shift+click range)

	drag tblDrag
	vh   tblVarH // variable row heights (Wrap columns)

	// View cache.
	cacheSig  string
	cacheIDs  []int
	cacheTree []tblRow
	cacheN    int
	selSig    string
	selAll    int
	selCount  int
}

type tblDrag struct {
	on       bool
	id       ID
	from     int // data row
	fromPos  int
	pointerY float32
	last     time.Time // previous auto-scroll step
}

func newTblState() tblState {
	return tblState{widths: map[string]float32{}, loadReq: map[string]bool{}, panelH: map[string]float32{}}
}

// tblFrame is the per-Show working set.
type tblFrame struct {
	t    *TableW
	c    *Context
	th   *theme.Theme      // ThemeFor(CompTable): the table's Alias applied
	tk   *theme.TableToken // &th.Components.Table
	st   *tblState
	rtl  bool // right-to-left scope: start is the right edge
	el   bool
	shad bool // shadcn data-table metrics and sort control
	pv   float32
	ph   float32
	rh   float32 // body row height
	hh   float32 // header row height
	fs   float32 // font size
	lh   float32 // line height
	spl  theme.Color

	cols   []TableColumn
	leaves []tblLeaf
	heads  []tblHead
	depth  int
	ndata  int
	data   []int // leaf index of each data column

	totalW, viewW float32
	anyLeft       bool
	anyRight      bool
	overflow      bool

	// Displayed rows of the current page.
	ids   []int    // sorted/filtered data rows (nil: identity)
	tree  []tblRow // flattened tree (nil: flat)
	start int      // page start into ids / identity
	n     int      // rows on the page
	allN  int      // rows before paging (pager total)

	selInts map[int]bool
	selKeys map[string]bool
	expSet  map[string]bool
	openSet map[string]bool
	editSet map[string]bool

	sorts    []SortState
	sortOf   map[string]SortOrder
	filterOf map[string][]string

	bounded  bool
	scroll   *scrollState
	sx       float32
	sy       float64 // content offset (float64: exact for millions of rows)
	ry       float32 // sy relative to the laid-out row window (scroll base)
	viewH    float32
	offs     []float64
	first    int
	last     int
	bodyID   ID
	bodyRect geom.Rect
	frameID  ID
	focused  bool
	rowPaint bool // rows paint their background and bottom line once (no merged cells)
	tips     []tblTip

	frameN  *node
	vh      *tblVarH     // variable row heights; nil: every row is rh tall
	linked  bool         // tree selection with parent/child linkage
	treePar map[int]int  // tree node → parent (linked selection)
	half    map[int]bool // half-checked tree nodes
	headY   float32      // sticky header shift
	footY   float32      // sticky summary shift (<= 0)
	stickyS *scrollState // container of a sticky table

	byKey bool // the selection change comes from the keyboard (no Shift range)

	// Accessibility: the active cell (ActiveDescendant of the grid) and
	// the expand / tree toggle of the row being built (its Expand action).
	actID     ID
	rowToggle ID
}

// tblSem is the grid position of a header, row or cell. It is attached
// only when accessibility is on (converting it to semFiller allocates).
type tblSem struct {
	row, col int // 1-based; 0 = not applicable
	level    int // tree depth (1 = top level)
	sort     a11y.Sort
}

func (p tblSem) semantics(s *a11y.Node) {
	s.RowIndex, s.ColIndex, s.Level, s.Sort = p.row, p.col, p.level, p.sort
}

// semantics completes the grid node: its full size (virtualized rows are
// not built) and the active cell.
func (f *tblFrame) semantics(s *a11y.Node) {
	s.RowCount = f.depth + f.n + len(f.t.summary)
	s.ColCount = len(f.leaves)
	if f.actID != 0 && f.st.actOn {
		s.ActiveDescendant = a11y.NodeID(f.actID)
	}
}

type tblTip struct {
	anchor ID
	text   string
	err    bool
}

// Show builds the table.
func (t *TableW) Show() *TableW {
	if t.tp != nil && t.tp.on && !t.tp.view {
		return t.showTransposed()
	}
	c := t.c
	root := c.box("table", layout.Column, nil)
	t.id = root.n.id
	st := State[tblState](c, t.id, newTblState)
	t.st = st
	th := c.ThemeFor(theme.CompTable)
	f := &tblFrame{t: t, c: c, th: th, tk: &th.Components.Table, st: st, rtl: c.IsRTL()}
	root.Body(func() { f.build() })
	return t
}

func (f *tblFrame) metrics() {
	t, th, tk := f.t, f.th, f.tk
	f.el = th.Flavor == theme.Element
	f.shad = th.Flavor == theme.StyleShadcn
	f.fs = tk.CellFontSize
	f.spl = tk.BorderColor
	small := t.sizeSet && t.size == Small
	if small {
		f.fs = tk.CellFontSizeSM
	} else if t.sizeSet && t.size == Middle {
		f.fs = tk.CellFontSizeMD
	}
	f.lh = tk.CellLineHeight
	if f.lh <= 0 {
		f.lh = 22
	}
	if f.el {
		// Element: the default density is Middle.
		switch {
		case small:
			f.pv, f.ph = tk.CellPaddingBlockSM, tk.CellPaddingInlineSM
		case t.sizeSet && t.size == Large:
			f.pv, f.ph = tk.CellPaddingBlock, tk.CellPaddingInline
		default:
			f.pv, f.ph = tk.CellPaddingBlockMD, tk.CellPaddingInlineMD
		}
	} else {
		switch {
		case t.sizeSet && t.size == Middle:
			f.pv, f.ph = tk.CellPaddingBlockMD, tk.CellPaddingInlineMD
		case small:
			f.pv, f.ph = tk.CellPaddingBlockSM, tk.CellPaddingInlineSM
		default:
			f.pv, f.ph = tk.CellPaddingBlock, tk.CellPaddingInline
		}
	}
	f.rh = f.lh + 2*f.pv + 1
	f.hh = f.rh
	if f.shad {
		f.shad2bMetrics()
	}
	if t.virtual {
		// Element Plus Table V2 row-height / header-height (50 / 50).
		if tk.VirtualRowHeight > 0 {
			f.rh = tk.VirtualRowHeight
		}
		if tk.VirtualHeaderHeight > 0 {
			f.hh = tk.VirtualHeaderHeight
		}
	}
	if t.rowH > 0 {
		f.rh = t.rowH
	}
	if t.headH > 0 {
		f.hh = t.headH
	}
}

func (f *tblFrame) font(w text.Weight) text.Style {
	st := f.c.style(f.fs, w)
	st.LineHeight = f.lh
	return st
}

// columns resolves leaves and widths for the viewport width.
func (f *tblFrame) columns() {
	t, st := f.t, f.st
	cs := t.colState
	if cs == nil {
		cs = &st.colState
	}
	f.cols = tblArrange(t.cols, cs)
	if t.ellipsisAll {
		f.cols = tblEllipsisAll(f.cols)
	}
	var specials []tblLeaf
	if t.dragSort != nil {
		specials = append(specials, tblLeaf{kind: tkDrag, w: 40, key: "__drag"})
	}
	// Ant Design puts the expand column first and the selection column
	// right after it (useSelection: "Always after expand icon").
	if t.expRender != nil && !t.hideExpandCol {
		specials = append(specials, tblLeaf{kind: tkExpand, w: f.tk.ExpandColumnWidth, key: "__expand"})
	}
	if t.selection != nil || t.selKeys != nil {
		// antd selection.js: selectionColumnWidth (32), + fontSizeIcon +
		// padding/4 (16) with the selections dropdown, + 2 × paddingXS
		// (16) in a bordered table. Element Plus' selection column stays
		// 48 whatever the border (structural, kept as before).
		w := f.tk.SelectionColumnWidth
		if t.selMenu && !t.single && !t.hideAll {
			w += 16 // the selections menu arrow
		}
		if t.bordered && !f.el {
			w += 16
		}
		if t.selColW > 0 {
			w = t.selColW
		}
		specials = append(specials, tblLeaf{kind: tkSelect, w: w, key: "__select"})
	}
	if t.selFixed {
		for i := range specials {
			specials[i].fixed = tfLeft
		}
	}
	f.leaves, f.heads, f.depth = tblFlatten(f.cols, specials)
	widths := st.widths
	if cs.Widths != nil {
		for k, v := range cs.Widths {
			if _, ok := widths[k]; !ok {
				widths[k] = v
			}
		}
	}
	f.totalW = tblWidths(f.leaves, max(f.viewW, t.scrollW), widths)
	f.overflow = f.totalW > f.viewW+0.5
	f.data = f.data[:0]
	for i := range f.leaves {
		switch f.leaves[i].fixed {
		case tfLeft:
			f.anyLeft = true
		case tfRight:
			f.anyRight = true
		}
		if f.leaves[i].kind == tkData {
			f.data = append(f.data, i)
		}
	}
	f.ndata = len(f.data)
}

// ---------------------------------------------------------------------------
// Sorting, filtering and the displayed rows.

func (f *tblFrame) colByKey(key string) *TableColumn {
	for i := range f.leaves {
		if f.leaves[i].col != nil && f.leaves[i].key == key {
			return f.leaves[i].col
		}
	}
	return nil
}

func (f *tblFrame) loadSorts() {
	t, st := f.t, f.st
	switch {
	case t.multiSort != nil:
		f.sorts = *t.multiSort
	case t.sort != nil:
		if t.sort.Order != SortNone && t.sort.Column != "" {
			f.sorts = []SortState{*t.sort}
		}
	default:
		if !st.sortInit {
			st.sortInit = true
			for _, lf := range f.leaves {
				if lf.col != nil && lf.col.DefaultSort != SortNone {
					st.sort = append(st.sort, SortState{lf.key, lf.col.DefaultSort})
				}
			}
		}
		f.sorts = st.sort
	}
	f.sortOf = map[string]SortOrder{}
	for _, s := range f.sorts {
		f.sortOf[s.Column] = s.Order
	}
	if t.filters != nil {
		if *t.filters == nil {
			*t.filters = map[string][]string{}
		}
		f.filterOf = *t.filters
	} else {
		if st.filters == nil {
			st.filters = map[string][]string{}
		}
		f.filterOf = st.filters
	}
}

// clickSort applies a sorter click on col.
func (f *tblFrame) clickSort(col *TableColumn) {
	t, st := f.t, f.st
	next := f.nextOrder(col)
	multi := col.SortMultiple > 0 && (t.multiSort != nil || (t.sort == nil && t.multiSort == nil))
	var list []SortState
	if multi {
		for _, s := range f.sorts {
			if s.Column == col.Key {
				continue
			}
			if c := f.colByKey(s.Column); c != nil && c.SortMultiple > 0 {
				list = append(list, s)
			}
		}
	}
	if next != SortNone {
		list = append(list, SortState{col.Key, next})
	}
	switch {
	case t.multiSort != nil:
		*t.multiSort = list
	case t.sort != nil:
		t.sort.Column, t.sort.Order = col.Key, next
	default:
		st.sort = list
	}
	f.sorts = list
	f.sortOf = map[string]SortOrder{}
	for _, s := range list {
		f.sortOf[s.Column] = s.Order
	}
	t.sortedCh = true
	f.resetPage()
	f.c.invalid = true
}

func (f *tblFrame) resetPage() {
	if f.t.page != nil && !f.t.serverPage {
		*f.t.page = 1
	}
	if f.t.page != nil && f.t.serverPage {
		*f.t.page = 1
		f.t.pageCh = true
	}
}

// localSpecs returns the comparators and filters the table applies itself.
func (f *tblFrame) localSpecs() ([]tblSortSpec, []tblFilterSpec, string) {
	var sig strings.Builder
	type pri struct {
		spec tblSortSpec
		p    int
		i    int
	}
	var ps []pri
	for i, s := range f.sorts {
		col := f.colByKey(s.Column)
		if col == nil || col.Compare == nil || s.Order == SortNone {
			continue
		}
		ps = append(ps, pri{tblSortSpec{col.Compare, s.Order == SortDesc}, col.SortMultiple, i})
		sig.WriteString(s.Column)
		sig.WriteByte(byte('0' + s.Order))
	}
	sort.SliceStable(ps, func(a, b int) bool { return ps[a].p > ps[b].p })
	specs := make([]tblSortSpec, len(ps))
	for i, p := range ps {
		specs[i] = p.spec
	}
	sig.WriteByte('|')
	var fs []tblFilterSpec
	keys := make([]string, 0, len(f.filterOf))
	for k := range f.filterOf {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vals := f.filterOf[k]
		col := f.colByKey(k)
		if col == nil || col.OnFilter == nil || len(vals) == 0 {
			continue
		}
		fs = append(fs, tblFilterSpec{col.OnFilter, vals})
		sig.WriteString(k)
		sig.WriteByte('=')
		sig.WriteString(strings.Join(vals, "\x00"))
		sig.WriteByte(';')
	}
	return specs, fs, sig.String()
}

func keySet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, k := range s {
		m[k] = true
	}
	return m
}

func (f *tblFrame) openKeys() *[]string {
	if f.t.treeOpen != nil {
		return f.t.treeOpen
	}
	return &f.st.treeOpen
}

func (f *tblFrame) expKeys() *[]string {
	if f.t.expKeys != nil {
		return f.t.expKeys
	}
	return &f.st.expanded
}

// view computes the displayed rows (cached across frames).
func (f *tblFrame) view() {
	t, st := f.t, f.st
	specs, filters, sig := f.localSpecs()
	f.openSet = keySet(*f.openKeys())
	f.expSet = keySet(*f.expKeys())
	if t.editRows != nil {
		f.editSet = keySet(*t.editRows)
	}
	prep := func(ids []int) []int {
		ids = tblFilterRows(ids, filters)
		tblSortRows(ids, specs)
		return ids
	}
	if t.treeKids != nil {
		roots := t.treeRoots
		var ob strings.Builder
		for _, k := range *f.openKeys() {
			ob.WriteString(k)
			ob.WriteByte(0)
		}
		full := sig + "|" + strconv.Itoa(len(roots)) + "|" + strconv.FormatUint(t.revision, 10) + "|" + ob.String()
		if full != st.cacheSig || st.cacheTree == nil {
			rs := prep(append([]int(nil), roots...))
			st.cacheIDs = rs
			st.cacheSig = full
			st.cacheTree = []tblRow{}
		}
		rs := st.cacheIDs
		f.allN = len(rs)
		a, b := 0, len(rs)
		if t.page != nil && !t.serverPage {
			a, b, *t.page = tblPage(len(rs), *t.page, f.pageSize())
		}
		has := t.treeHas
		f.tree = tblTree(rs[a:b], t.treeKids, has, func(id int) bool { return f.openSet[tblKey(t.rowKey, id)] }, func(id int) string { return tblKey(t.rowKey, id) }, prep)
		f.n = len(f.tree)
		return
	}
	if len(specs) == 0 && len(filters) == 0 {
		f.ids = nil
		f.allN = t.rows
	} else {
		full := sig + "|" + strconv.Itoa(t.rows) + "|" + strconv.FormatUint(t.revision, 10)
		if full != st.cacheSig || st.cacheIDs == nil || st.cacheTree != nil {
			ids := make([]int, t.rows)
			for i := range ids {
				ids[i] = i
			}
			st.cacheIDs = prep(ids)
			st.cacheSig = full
			st.cacheTree = nil
		}
		f.ids = st.cacheIDs
		f.allN = len(f.ids)
	}
	f.start, f.n = 0, f.allN
	if t.page != nil && !t.serverPage {
		a, b, p := tblPage(f.allN, *t.page, f.pageSize())
		*t.page = p
		f.start, f.n = a, b-a
	}
}

func (f *tblFrame) pageSize() int {
	if f.t.pageSize == nil || *f.t.pageSize <= 0 {
		return 10
	}
	return *f.t.pageSize
}

// row returns the displayed row at position i of the page.
func (f *tblFrame) row(i int) tblRow {
	switch {
	case f.tree != nil:
		return f.tree[i]
	case f.ids != nil:
		d := f.ids[f.start+i]
		return tblRow{data: d, key: tblKey(f.t.rowKey, d)}
	}
	d := f.start + i
	return tblRow{data: d, key: tblKey(f.t.rowKey, d)}
}

// dataRows lists the data rows of the page.
func (f *tblFrame) dataRows() []int {
	out := make([]int, f.n)
	for i := range out {
		switch {
		case f.tree != nil:
			out[i] = f.tree[i].data
		case f.ids != nil:
			out[i] = f.ids[f.start+i]
		default:
			out[i] = f.start + i
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Selection.

func (f *tblFrame) selKeyList() *[]string {
	if f.t.selKeys != nil {
		return f.t.selKeys
	}
	return &f.st.sel
}

func (f *tblFrame) loadSelection() {
	t := f.t
	if t.selection != nil {
		f.selInts = make(map[int]bool, len(*t.selection))
		for _, v := range *t.selection {
			f.selInts[v] = true
		}
	} else if t.selKeys != nil {
		f.selKeys = keySet(*t.selKeys)
	}
}

func (f *tblFrame) hasSelection() bool { return f.t.selection != nil || f.t.selKeys != nil }

func (f *tblFrame) selected(r tblRow) bool {
	if f.t.selection != nil {
		return f.selInts[r.data]
	}
	return f.selKeys[r.key]
}

func (f *tblFrame) selectable(r tblRow) bool {
	return f.t.selDisabled == nil || !f.t.selDisabled(r.data)
}

func (f *tblFrame) setSelected(r tblRow, on bool) {
	t := f.t
	if f.linked {
		f.linkedCheck([]int{r.data}, on)
		return
	}
	if !t.single && !f.byKey {
		if f.c.PressMods().Has(input.ModShift) && f.rangeSelect(r) {
			return
		}
		f.st.lastSel = ""
		if on {
			f.st.lastSel = r.key
		}
	}
	if t.selection != nil {
		s := *t.selection
		if t.single {
			s = s[:0]
			if on {
				s = append(s, r.data)
			}
		} else if on && !f.selInts[r.data] {
			s = append(s, r.data)
		} else if !on {
			out := s[:0]
			for _, v := range s {
				if v != r.data {
					out = append(out, v)
				}
			}
			s = out
		}
		*t.selection = s
		f.selInts = make(map[int]bool, len(s))
		for _, v := range s {
			f.selInts[v] = true
		}
	} else if t.selKeys != nil {
		if t.single {
			*t.selKeys = (*t.selKeys)[:0]
		}
		*t.selKeys = tblToggle(*t.selKeys, r.key, on)
		f.selKeys = keySet(*t.selKeys)
	}
	t.selCh = true
	f.c.invalid = true
}

// pageSelState counts the selectable and selected rows of the page; huge
// pages are cached by selection size.
func (f *tblFrame) pageSelState() (all, some bool) {
	t, st := f.t, f.st
	ln := 0
	if t.selection != nil {
		ln = len(*t.selection)
	} else if t.selKeys != nil {
		ln = len(*t.selKeys)
	}
	sig := strconv.Itoa(ln) + "|" + strconv.Itoa(f.start) + "|" + strconv.Itoa(f.n) + "|" + st.cacheSig
	if f.n > 2000 && sig == st.selSig {
		return tblSelState(st.selAll, st.selCount)
	}
	total, sel := 0, 0
	if t.selDisabled == nil && f.tree == nil && f.ids == nil && t.selection != nil && f.start == 0 && f.n == t.rows {
		total = f.n
		for v := range f.selInts {
			if v >= 0 && v < f.n {
				sel++
			}
		}
	} else {
		for i := 0; i < f.n; i++ {
			r := f.row(i)
			if !f.selectable(r) {
				continue
			}
			total++
			if f.selected(r) {
				sel++
			}
		}
	}
	st.selSig, st.selAll, st.selCount = sig, total, sel
	return tblSelState(total, sel)
}

// selectPage selects or clears every selectable row of the page (other
// pages' selections are kept).
func (f *tblFrame) selectPage(on bool) {
	t := f.t
	if f.linked {
		var ids []int
		for i := 0; i < f.n; i++ {
			if r := f.row(i); f.selectable(r) {
				ids = append(ids, r.data)
			}
		}
		f.linkedCheck(ids, on)
		return
	}
	if t.selection != nil {
		s := *t.selection
		if on {
			for i := 0; i < f.n; i++ {
				r := f.row(i)
				if f.selectable(r) && !f.selInts[r.data] {
					s = append(s, r.data)
					f.selInts[r.data] = true
				}
			}
		} else {
			drop := map[int]bool{}
			for i := 0; i < f.n; i++ {
				r := f.row(i)
				if f.selectable(r) {
					drop[r.data] = true
				}
			}
			out := s[:0]
			for _, v := range s {
				if !drop[v] {
					out = append(out, v)
				}
			}
			s = out
		}
		*t.selection = s
	} else if t.selKeys != nil {
		s := *t.selKeys
		set := keySet(s)
		if on {
			for i := 0; i < f.n; i++ {
				r := f.row(i)
				if f.selectable(r) && !set[r.key] {
					s = append(s, r.key)
					set[r.key] = true
				}
			}
		} else {
			drop := map[string]bool{}
			for i := 0; i < f.n; i++ {
				r := f.row(i)
				if f.selectable(r) {
					drop[r.key] = true
				}
			}
			out := s[:0]
			for _, v := range s {
				if !drop[v] {
					out = append(out, v)
				}
			}
			s = out
		}
		*t.selKeys = s
	}
	f.loadSelection()
	t.selCh = true
	f.c.invalid = true
}

// loadLinkage prepares linked tree selection: the parent map and the
// half-checked nodes.
func (f *tblFrame) loadLinkage() {
	t := f.t
	f.linked = t.treeKids != nil && !t.strict && !t.single && f.hasSelection()
	if !f.linked {
		return
	}
	f.treePar = tblTreeParents(t.treeRoots, t.treeKids)
	f.half = tblTreeHalf(t.treeRoots, t.treeKids, f.linkSel())
}

// linkSel returns the checked tree nodes by data row.
func (f *tblFrame) linkSel() map[int]bool {
	sel := map[int]bool{}
	for id := range f.treePar {
		if f.selected(tblRow{data: id, key: tblKey(f.t.rowKey, id)}) {
			sel[id] = true
		}
	}
	return sel
}

// linkedCheck checks or unchecks tree nodes with linkage and writes the
// result back in the bound selection (order kept, new nodes appended).
func (f *tblFrame) linkedCheck(ids []int, on bool) {
	t := f.t
	sel := f.linkSel()
	var dis func(int) bool
	if t.selDisabled != nil {
		dis = t.selDisabled
	}
	for _, id := range ids {
		tblTreeCheck(sel, id, on, t.treeKids, f.treePar, dis)
	}
	// Tree order for appended nodes.
	var order []int
	seen := map[int]bool{}
	var walk func(ids []int)
	walk = func(ids []int) {
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			order = append(order, id)
			walk(t.treeKids(id))
		}
	}
	walk(t.treeRoots)
	if t.selection != nil {
		var out []int
		have := map[int]bool{}
		for _, v := range *t.selection {
			if _, inTree := f.treePar[v]; (!inTree || sel[v]) && !have[v] {
				out = append(out, v)
				have[v] = true
			}
		}
		for _, id := range order {
			if sel[id] && !have[id] {
				out = append(out, id)
				have[id] = true
			}
		}
		*t.selection = out
	} else if t.selKeys != nil {
		idOf := map[string]int{}
		keyOf := map[int]string{}
		for id := range f.treePar {
			k := tblKey(t.rowKey, id)
			idOf[k], keyOf[id] = id, k
		}
		var out []string
		have := map[string]bool{}
		for _, k := range *t.selKeys {
			if have[k] {
				continue
			}
			id, inTree := idOf[k]
			if !inTree || sel[id] {
				out = append(out, k)
				have[k] = true
			}
		}
		for _, id := range order {
			if k := keyOf[id]; sel[id] && !have[k] {
				out = append(out, k)
				have[k] = true
			}
		}
		*t.selKeys = out
	}
	f.loadSelection()
	f.half = tblTreeHalf(t.treeRoots, t.treeKids, f.linkSel())
	t.selCh = true
	f.c.invalid = true
}

// ---------------------------------------------------------------------------
// Frame.

func (f *tblFrame) build() {
	c, t, th, st := f.c, f.t, f.th, f.st
	f.metrics()
	if t.title != nil {
		f.titleBar(t.title, false)
	}
	var topPager *Box
	if t.page != nil {
		if top, _ := f.pagerPlaces(); top != "" {
			// Filled after the rows are known (Body can run later).
			topPager = c.box("table-pager-top", layout.Column, nil)
		}
	}
	frame := c.box("table-frame", layout.Column, nil)
	border := float32(0)
	if t.bordered {
		border = 2
	}
	vw := st.viewW
	if r, ok := c.rects[frame.n.id]; ok && r.W > 0 {
		vw = r.W - border
	}
	if vw <= 0 {
		vw = max(200, c.size.W-48-border) // first frame guess; corrected next frame
	}
	if vw != st.viewW {
		c.animating = true // re-layout once with the real width
		st.viewW = vw
	}
	f.viewW = vw
	f.columns()
	if (t.tp != nil && t.tp.header == nil) || t.noHeader {
		f.depth, f.heads = 0, nil // HideHeader, or Transposed without TransposedHeader: no header row
	}
	f.loadSorts()
	f.initFilters()
	f.initExpanded()
	if t.showSum && len(t.summary) == 0 {
		// Element Plus renders summary-method or the default sums, never
		// both (table-footer/index.mjs:49-53).
		t.summary = append(t.summary, f.defaultSummary)
	}
	if t.curKey != nil {
		st.cur, st.curOn = *t.curKey, *t.curKey != ""
	}
	prevCur, prevCurOn := st.cur, st.curOn
	f.view()
	f.loadSelection()
	f.loadLinkage()
	t.fr = f
	t.vis = nil
	if len(t.summary) > 0 {
		t.vis = f.dataRows()
	}
	f.frameID = frame.n.id
	frame.n.ownKeys = true
	// A grid: Tab reaches the frame, arrows move the active cell.
	frame.n.setRole(a11y.RoleGrid).fill(f)
	if t.a11yName != "" {
		frame.n.setName(t.a11yName)
	}
	frame.n.stateIf(t.loading, a11y.Busy).stateIf(f.hasSelection() && !t.single, a11y.Multiselectable)
	fia := c.interact(frame.n, true, input.CursorDefault)
	f.focused = fia.Focused
	f.setupScroll(frame.n.id)
	f.keys()
	f.frameN = frame.n
	f.stickyShift(frame.n)
	frame.Body(func() {
		if t.sticky {
			// The header paints above the rows it covers: build it last,
			// absolutely positioned, with a spacer keeping its place.
			sp := c.add("thead-space", "")
			sp.ln.Height = f.hh * float32(f.depth)
			f.body()
			if len(t.summary) > 0 {
				f.summaryBar()
			}
			f.header()
			return
		}
		f.header()
		f.body()
		if len(t.summary) > 0 {
			f.summaryBar()
		}
	})
	// Corners: the header's top radius, the bordered bottom radius (antd
	// radius.js); a Title / Footer bar takes the outer corners instead.
	rad := f.tk.HeaderBorderRadius
	radii := geom.Radii{TL: rad, TR: rad}
	if t.bordered {
		radii.BL, radii.BR = f.tk.BottomRadius, f.tk.BottomRadius
	}
	if t.title != nil {
		radii.TL, radii.TR = 0, 0
	}
	if t.footer != nil {
		radii.BL, radii.BR = 0, 0
	}
	frame.Radii(radii)
	// Background below the header as plain rectangles (cheap to raster);
	// only the bottom corner strip of a bordered table is rounded.
	frame.Paint(func(l *paint.List, r geom.Rect) {
		top := r.Y + f.hh*float32(f.depth)
		bot := r.Bottom()
		if br := radii.BL; br > 0 {
			bot -= br
			l.Fill(geom.Rect{X: r.X, Y: bot - br, W: r.W, H: 2 * br}, geom.Radii{BL: br, BR: br}, th.BgContainer)
		}
		l.Fill(geom.Rect{X: r.X, Y: top, W: r.W, H: max(0, bot-top)}, geom.Radii{}, th.BgContainer)
	})
	// Outer border, Element's bottom edge and the loading mask paint above
	// the cells (opaque sticky cells would otherwise hide the border).
	frame.PaintOver(func(l *paint.List, r geom.Rect) {
		// The start-fixed group casts its shadow toward the end and the
		// end-fixed group toward the start (mirrored in RTL).
		s := f.inl(r)
		if f.shadowL() {
			k := f.lastLeft()
			f.fixedShadow(l, s.X(f.leaves[k].x+f.leaves[k].w, 0), r.Y, r.H, !f.rtl)
		}
		if f.shadowR() {
			f.fixedShadow(l, s.X(f.cx(f.firstRight())-f.sx, 0), r.Y, r.H, f.rtl)
		}
		if t.bordered {
			l.Stroke(r, radii, 1, f.spl)
		} else if f.el {
			l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, f.spl)
		}
		if t.loading {
			l.Fill(r, radii, theme.Fade(th.BgContainer, 0.5))
			drawSpinner(c, l, geom.Rect{X: r.X + r.W/2 - 10, Y: r.Y + min(r.H/2, 160) - 10, W: 20, H: 20})
		}
	})
	if t.loading {
		frame.n.hit, frame.n.blocks = true, true
	}
	if t.footer != nil {
		f.titleBar(t.footer, true)
	}
	if t.page != nil {
		top, bottom := f.pagerPlaces()
		if topPager != nil {
			topPager.Body(func() { f.pager(top) })
		}
		if bottom != "" {
			f.pager(bottom)
		}
	}
	f.expandClicked()
	if st.curOn && (!prevCurOn || st.cur != prevCur) {
		// Element Plus current-change: the current row moved (click or
		// Enter), bound or not.
		t.curCh = true
		if t.curKey != nil {
			*t.curKey = st.cur
		}
	}
	if t.selCh {
		f.pruneSelection()
	}
	if t.scrollTop && (t.sortedCh || t.filterCh || t.pageCh) && f.scroll != nil {
		f.scroll.off.Y = 0
	}
	f.flushTips()
}

// pager builds the pager at placement place (PageTopStart …). Like Ant
// Design's table it is hidden when there are no rows, has no total unless
// PaginationConfig asks for one and is small for Small and Middle tables
// (getPaginationSize).
func (f *tblFrame) pager(place string) {
	c, t := f.c, f.t
	total := f.allN
	if t.serverPage {
		total = t.total
	}
	if total <= 0 {
		return
	}
	if t.pageSize == nil {
		ps := 10
		t.pageSize = &ps
	}
	just := layout.End()
	switch place {
	case PageTopStart, PageBottomStart:
		just = layout.JustifyContent(layout.JustifyStart)
	case PageTopCenter, PageBottomCenter:
		just = layout.JustifyContent(layout.JustifyCenter)
	}
	m := geom.Insets{Top: 16}
	if strings.HasPrefix(place, "top") {
		m = geom.Insets{Bottom: 16}
	}
	Row(c, func() {
		before, beforeSize := *t.page, *t.pageSize
		p := Pagination(c, t.page, total, f.pageSize())
		if t.sizeSet && (t.size == Small || t.size == Middle) {
			p.Small()
		}
		if len(t.pageSizes) > 0 || total > 50 {
			p.SizeChanger(t.pageSize)
		}
		if t.pageCfg != nil {
			t.pageCfg(p)
		}
		p.Show()
		if *t.page != before || *t.pageSize != beforeSize {
			t.pageCh = true
			f.st.actOn = false
			c.invalid = true
		}
	}, just, layout.Margin(m))
}

// ---------------------------------------------------------------------------
// Geometry helpers.
//
// Column positions (tblLeaf.x, cx) are logical: offsets from the inline
// start edge. Cells are absolutely positioned with Pos.Left = x, which the
// layout mirrors in RTL, and the body's scroll state (scrollState.ln) maps
// the logical horizontal offset to the mirrored content, so FixedLeft /
// FixedRight act as start / end like Ant's RTL table. Paint code inside a
// cell maps its own offsets with f.inl.

// inl is the inline span of r in the table's direction.
func (f *tblFrame) inl(r geom.Rect) inlineSpan { return inlineSpan{r: r, rtl: f.rtl} }

// logicalAlign turns a column alignment into start / center / end:
// AlignLeft / AlignRight are start / end; the physical alignments depend on
// the direction.
func (f *tblFrame) logicalAlign(a TextAlign) TextAlign {
	switch a {
	case AlignPhysicalLeft:
		if f.rtl {
			return AlignEnd
		}
		return AlignStart
	case AlignPhysicalRight:
		if f.rtl {
			return AlignStart
		}
		return AlignEnd
	}
	return a
}

// justify is the main-axis justification of a Render cell for alignment a.
func (f *tblFrame) justify(a TextAlign) layout.Justify {
	switch f.logicalAlign(a) {
	case AlignCenter:
		return layout.JustifyCenter
	case AlignEnd:
		return layout.JustifyEnd
	}
	return layout.JustifyStart
}

// cx returns a leaf's x in body content coordinates (sticky columns follow
// the scroll offset).
func (f *tblFrame) cx(li int) float32 {
	lf := &f.leaves[li]
	switch lf.fixed {
	case tfLeft:
		return lf.x + f.sx
	case tfRight:
		if f.overflow {
			return lf.x - (f.totalW - f.viewW) + f.sx
		}
	}
	return lf.x
}

// spanW is the width of leaves [li, li+n).
func (f *tblFrame) spanW(li, n int) float32 {
	e := min(len(f.leaves), li+n) - 1
	return f.leaves[e].x + f.leaves[e].w - f.leaves[li].x
}

// culled reports a non-fixed span outside the viewport.
func (f *tblFrame) culled(li int, w float32) bool {
	if f.leaves[li].fixed != tfNone {
		return false
	}
	x := f.leaves[li].x
	return x+w < f.sx-1 || x > f.sx+f.viewW+1
}

func (f *tblFrame) lastLeft() int {
	k := -1
	for i := range f.leaves {
		if f.leaves[i].fixed == tfLeft {
			k = i
		}
	}
	return k
}

func (f *tblFrame) firstRight() int {
	for i := range f.leaves {
		if f.leaves[i].fixed == tfRight {
			return i
		}
	}
	return -1
}

// shadowL / shadowR report whether the fixed-column shadows show.
func (f *tblFrame) shadowL() bool { return f.anyLeft && f.sx > 0.5 }
func (f *tblFrame) shadowR() bool {
	return f.anyRight && f.overflow && f.sx < f.totalW-f.viewW-0.5
}

// fixedShadow paints the blurred shadow a fixed column group casts over
// the scrolled cells: a box shadow of the group, clipped to a strip outside
// its edge x (right of it for left-fixed columns, left of it otherwise),
// once for the whole table height.
func (f *tblFrame) fixedShadow(l *paint.List, x, y, h float32, right bool) {
	a := f.tk.FixedShadowAlpha
	const w = 14
	strip := geom.Rect{X: x, Y: y, W: w, H: h}
	src := geom.Rect{X: x - 40, Y: y - 40, W: 40, H: h + 80}
	if !right {
		strip.X = x - w
		src.X = x
	}
	l.Clip(strip, geom.Radii{})
	l.Shadows(src, geom.Radii{}, []paint.Shadow{{Blur: 12, Color: theme.Fade(theme.Hex("#000000"), a)}})
	l.Pop()
}

// stickyShift computes the sticky header and summary offsets from the
// table's rect in its scroll container (last paint, corrected by the
// scrolling since).
func (f *tblFrame) stickyShift(frame *node) {
	t, c := f.t, f.c
	f.headY, f.footY = 0, 0
	if !t.sticky && !t.stickySum {
		return
	}
	cid := t.stickyIn
	var s *scrollState
	if cid != 0 {
		s = scrollStateOf(c, cid)
	} else if p := scrollAncestor(frame); p != nil {
		cid, s = p.id, p.scroll
	}
	f.stickyS = s
	fr, ok := c.rects[frame.id]
	if !ok {
		return
	}
	v := geom.Rect{W: c.size.W, H: c.size.H}
	if cid != 0 {
		if v, ok = c.rects[cid]; !ok {
			return
		}
	}
	dy := float32(0)
	if s != nil {
		dy = float32(max(0, min(s.off.Y, s.maxY())) - s.painted.Y)
	}
	top, bot := fr.Y-dy, fr.Bottom()-dy
	room := max(0, fr.H-f.hh*float32(f.depth)-f.rh*float32(len(t.summary)))
	if t.sticky {
		f.headY = max(0, min(v.Y+t.stickyTop-top, room))
	}
	if t.stickySum && len(t.summary) > 0 {
		f.footY = min(0, max(v.Bottom()-t.stickyBot-bot, -room))
	}
}

// textCell draws single-line text with ellipsis inside r (padding applied
// by the caller) and reports truncation.
func (f *tblFrame) drawText(l *paint.List, s string, x, y, w, h float32, align TextAlign, st text.Style, col theme.Color) {
	f.drawTextOpts(l, s, x, y, w, h, align, st, col, text.Options{})
}

// drawTextOpts is drawText with paragraph options (o.Direction: interface
// text such as header titles follows the scope; cell data stays DirAuto).
// Start / end alignment follows the table's direction, so every cell of a
// column lines up whatever its content's script.
func (f *tblFrame) drawTextOpts(l *paint.List, s string, x, y, w, h float32, align TextAlign, st text.Style, col theme.Color, o text.Options) {
	if s == "" || w <= 0 {
		return
	}
	o.MaxWidth, o.NoWrap, o.Ellipsis = w, true, true
	lay := text.Shape(s, st, o)
	x += alignLine(align, w, lay.Width, f.rtl)
	l.Glyphs(geom.Pt(float32(math.Round(float64(x))), y+(h-lay.Height)/2), lay.Glyphs, col)
}

// drawWrapped draws text wrapped to w, vertically centered in h.
func (f *tblFrame) drawWrapped(l *paint.List, s string, x, y, w, h float32, align TextAlign, st text.Style, col theme.Color) {
	if s == "" || w <= 0 {
		return
	}
	lay := text.Shape(s, st, text.Options{MaxWidth: w})
	x += alignLine(align, w, lay.Width, f.rtl)
	l.Glyphs(geom.Pt(float32(math.Round(float64(x))), y+float32(math.Round(float64((h-lay.Height)/2)))), lay.Glyphs, col)
}

func (f *tblFrame) truncated(s string, w float32, st text.Style) bool {
	if s == "" {
		return false
	}
	return text.Width(s, st) > w+0.5
}

// tip queues a tooltip anchored to a widget (ellipsis, validation).
func (f *tblFrame) tip(anchor ID, s string, err bool) {
	f.tips = append(f.tips, tblTip{anchor, s, err})
}

func (f *tblFrame) flushTips() {
	c, th := f.c, f.th
	for _, tp := range f.tips {
		tp := tp
		a := c.AnimFrom(tp.anchor, "tbl-tip", 0, 1, th.DurationFast)
		place := Top
		if tp.err {
			place = Bottom
		}
		ov := Overlay(c, OverlayOptions{Anchor: tp.anchor, Place: place, Offset: 6, Flip: true}, func() {
			col := th.TextOnSpotlight
			if tp.err {
				col = th.Error.Base
			}
			Text(c, tp.text).Color(col).Layout(layout.MaxW(360))
		}, layout.PadVH(6, 8))
		bg := th.BgSpotlight
		if tp.err {
			bg = th.BgElevated
		}
		if f.el {
			ov.n.ln.Padding = geom.VH(5, 11)
		}
		ov.Bg(bg).Radius(th.Radius).Shadow(th.Shadow).Opacity(a)
		ov.n.hit = false
		ov.n.over = func(l *paint.List, r geom.Rect) {
			ar, ok := c.anchorRect(tp.anchor)
			if !ok {
				return
			}
			cx := max(r.X+12, min(ar.X+ar.W/2, r.Right()-12))
			p := paint.NewPath()
			if r.Bottom() <= ar.Y+1 {
				p.MoveTo(cx-6, r.Bottom()-0.5).LineTo(cx, r.Bottom()+5.5).LineTo(cx+6, r.Bottom()-0.5).Close()
			} else {
				p.MoveTo(cx-6, r.Y+0.5).LineTo(cx, r.Y-5.5).LineTo(cx+6, r.Y+0.5).Close()
			}
			l.Path(p, bg)
		}
	}
}

var _ = time.Second

// ---------------------------------------------------------------------------
// Ant Design / Element Plus behaviours (spec alignment, batch 2).

// sortDirs returns the sort cycle of col.
func (f *tblFrame) sortDirs(col *TableColumn) []SortOrder {
	if len(col.SortDirections) > 0 {
		return col.SortDirections
	}
	return f.t.sortDirs
}

// nextOrder is the order a sorter click on col applies.
func (f *tblFrame) nextOrder(col *TableColumn) SortOrder {
	return tblNextOrderIn(f.sortDirs(col), f.sortOf[col.Key])
}

// sorterTip returns the tooltip of a sortable header: the action of the
// next click (Ant Design triggerAsc / triggerDesc / cancelSort) and
// whether it shows (ShowSorterTooltip; Element Plus draws none — a
// structural difference kept from before until the table token has a
// switch).
func (f *tblFrame) sorterTip(col *TableColumn) (string, bool) {
	if !f.t.sortTip || f.el {
		return "", false
	}
	loc := f.c.Locale()
	switch f.nextOrder(col) {
	case SortAsc:
		return loc.SortAsc, true
	case SortDesc:
		return loc.SortDesc, true
	}
	return loc.CancelSort, true
}

// filterResetValues is what the filter's reset button applies: nothing,
// or DefaultFilter with FilterResetToDefault.
func (f *tblFrame) filterResetValues(col *TableColumn) []string {
	if col.FilterResetToDefault && len(col.DefaultFilter) > 0 {
		return append([]string(nil), col.DefaultFilter...)
	}
	return nil
}

// initFilters applies DefaultFilter once, to columns the filter state has
// no entry for.
func (f *tblFrame) initFilters() {
	st := f.st
	if st.filterInit {
		return
	}
	st.filterInit = true
	for _, lf := range f.leaves {
		if lf.col == nil || len(lf.col.DefaultFilter) == 0 {
			continue
		}
		if _, ok := f.filterOf[lf.key]; !ok {
			f.filterOf[lf.key] = append([]string(nil), lf.col.DefaultFilter...)
		}
	}
}

// initExpanded applies DefaultExpandAllRows once to the table-owned
// expansion and tree state.
func (f *tblFrame) initExpanded() {
	t, st := f.t, f.st
	if !t.expandAll || st.expInit {
		return
	}
	st.expInit = true
	if t.treeKids != nil && t.treeOpen == nil {
		var keys []string
		seen := map[int]bool{}
		var walk func(ids []int)
		walk = func(ids []int) {
			for _, id := range ids {
				if seen[id] {
					continue
				}
				seen[id] = true
				if kids := t.treeKids(id); len(kids) > 0 {
					keys = append(keys, tblKey(t.rowKey, id))
					walk(kids)
				}
			}
		}
		walk(t.treeRoots)
		st.treeOpen = keys
	}
	if t.expRender != nil && t.expKeys == nil {
		var keys []string
		for i := 0; i < t.rows; i++ {
			if t.expAllow == nil || t.expAllow(i) {
				keys = append(keys, tblKey(t.rowKey, i))
			}
		}
		st.expanded = keys
	}
}

// posOf returns the page position of the row with key, or -1.
func (f *tblFrame) posOf(key string) int {
	for i := 0; i < f.n; i++ {
		if f.row(i).key == key {
			return i
		}
	}
	return -1
}

// rangeSelect handles Shift+click on a row checkbox (Ant Design
// useMultipleSelect): the rows between the last checked row and r are all
// checked, or all unchecked when every one of them already is. Disabled
// rows keep their state. It reports false when there is no range.
func (f *tblFrame) rangeSelect(r tblRow) bool {
	st := f.st
	if st.lastSel == "" {
		return false
	}
	a, b := f.posOf(st.lastSel), f.posOf(r.key)
	if a < 0 || b < 0 || a == b {
		return false
	}
	if a > b {
		a, b = b, a
	}
	var rows []tblRow
	on := false
	for i := a; i <= b; i++ {
		rr := f.row(i)
		if !f.selectable(rr) {
			continue
		}
		rows = append(rows, rr)
		if !f.selected(rr) {
			on = true
		}
	}
	f.setRows(rows, on)
	st.lastSel = ""
	if on {
		st.lastSel = f.row(b).key
	}
	return true
}

// setRows checks or unchecks rows of a multi-select (no tree linkage).
func (f *tblFrame) setRows(rows []tblRow, on bool) {
	t := f.t
	if t.selection != nil {
		s := *t.selection
		if on {
			for _, r := range rows {
				if !f.selInts[r.data] {
					s = append(s, r.data)
					f.selInts[r.data] = true
				}
			}
		} else {
			drop := map[int]bool{}
			for _, r := range rows {
				drop[r.data] = true
			}
			out := s[:0]
			for _, v := range s {
				if !drop[v] {
					out = append(out, v)
				}
			}
			s = out
		}
		*t.selection = s
	} else if t.selKeys != nil {
		s := *t.selKeys
		for _, r := range rows {
			s = tblToggle(s, r.key, on)
		}
		*t.selKeys = s
	}
	f.loadSelection()
	t.selCh = true
	f.c.invalid = true
}

// pruneSelection drops selected rows that are not in the data (Ant Design
// without preserveSelectedRowKeys): data indices outside 0..rows-1 or off
// the tree, keys of no such row.
func (f *tblFrame) pruneSelection() {
	t := f.t
	if t.preserveSel || !f.hasSelection() {
		return
	}
	var inTree map[int]bool
	if t.treeKids != nil {
		inTree = map[int]bool{}
		var walk func(ids []int)
		walk = func(ids []int) {
			for _, id := range ids {
				if !inTree[id] {
					inTree[id] = true
					walk(t.treeKids(id))
				}
			}
		}
		walk(t.treeRoots)
	}
	valid := func(row int) bool {
		if inTree != nil {
			return inTree[row]
		}
		return row >= 0 && row < t.rows
	}
	if t.selection != nil {
		s := *t.selection
		out := s[:0]
		for _, v := range s {
			if valid(v) {
				out = append(out, v)
			}
		}
		*t.selection = out
	} else {
		keys := map[string]bool{}
		if inTree != nil {
			for id := range inTree {
				keys[tblKey(t.rowKey, id)] = true
			}
		} else {
			for i := 0; i < t.rows; i++ {
				keys[tblKey(t.rowKey, i)] = true
			}
		}
		s := *t.selKeys
		out := s[:0]
		for _, k := range s {
			if keys[k] {
				out = append(out, k)
			}
		}
		*t.selKeys = out
	}
	f.loadSelection()
}

// expandClicked toggles the clicked row's detail panel or tree children
// (ExpandRowByClick).
func (f *tblFrame) expandClicked() {
	t := f.t
	if !t.expandByClick || t.clickRow < 0 {
		return
	}
	key := tblKey(t.rowKey, t.clickRow)
	pos := f.posOf(key)
	if pos < 0 {
		return
	}
	r := f.row(pos)
	switch {
	case f.tree != nil && r.kids:
		ks := f.openKeys()
		*ks = tblToggle(*ks, r.key, !r.open)
	case t.expRender != nil && f.canExpand(r):
		ks := f.expKeys()
		*ks = tblToggle(*ks, r.key, !f.expSet[r.key])
		f.expSet = keySet(*ks)
	default:
		return
	}
	t.expCh = true
	f.c.invalid = true
}

// defaultSummary is Element Plus' show-summary row (ShowSummary).
func (f *tblFrame) defaultSummary(key string, rows []int) string {
	if len(f.data) > 0 && f.leaves[f.data[0]].key == key {
		return f.c.Locale().Table.SumText
	}
	col := f.colByKey(key)
	if col == nil || col.Text == nil {
		return ""
	}
	texts := make([]string, len(rows))
	for i, r := range rows {
		texts[i] = col.Text(r)
	}
	return tblSumTexts(texts)
}

// titleBar builds the Ant Design title / footer bar: cell padding, the
// header colors for the footer, a border around it in a bordered table.
func (f *tblFrame) titleBar(fn func(), footer bool) {
	c, t, tk := f.c, f.t, f.tk
	kind := "table-title"
	if footer {
		kind = "table-footer"
	}
	b := c.box(kind, layout.Column, nil)
	b.n.ln.Padding = geom.VH(f.pv, f.ph)
	if footer {
		// antd footerBg / footerColor (Element and shadcn: no tint).
		b.Bg(tk.FooterAreaBg)
	}
	rad := tk.HeaderBorderRadius
	radii := geom.Radii{TL: rad, TR: rad}
	if footer {
		radii = geom.Radii{}
		if t.bordered {
			radii = geom.Radii{BL: tk.BottomRadius, BR: tk.BottomRadius}
		}
	}
	b.Radii(radii)
	if t.bordered {
		// antd bordered.js: the title has no bottom border and the footer
		// no top border (the table frame draws that edge).
		b.PaintOver(func(l *paint.List, r geom.Rect) {
			l.Stroke(r, radii, 1, f.spl)
			edge := r.Y
			if !footer {
				edge = r.Bottom() - 1
			}
			l.Fill(geom.Rect{X: r.X + 1, Y: edge, W: r.W - 2, H: 1}, geom.Radii{}, tk.FooterAreaBg)
		})
	}
	// The bar's text color (antd footerColor / colorTextHeading, Element
	// text-color-regular, shadcn caption muted-foreground): a scoped theme
	// whose Text is the token, cached per base theme.
	base := c.Theme
	if col := tk.FooterAreaColor; col != (theme.Color{}) && col != base.Text {
		cc := State[tblBarTheme](c, hashID(b.n.id, "bar-theme"), nil)
		if cc.base != base || cc.col != col {
			cp := *base
			cp.Text = col
			cc.base, cc.col, cc.th = base, col, &cp
		}
		b.Body(func() { ConfigProvider(c).Theme(cc.th).Body(fn) })
		return
	}
	b.Body(fn)
}

// tblBarTheme caches the Title / Footer bar theme.
type tblBarTheme struct {
	base, th *theme.Theme
	col      theme.Color
}

// pagerPlaces resolves PaginationPlacement into the top and bottom
// placements ("" = none there).
func (f *tblFrame) pagerPlaces() (top, bottom string) {
	pl := f.t.pagePlace
	if len(pl) == 0 {
		return "", PageBottomEnd
	}
	none := true
	for _, p := range pl {
		switch {
		case strings.HasPrefix(p, "top") && top == "":
			top = p
		case strings.HasPrefix(p, "bottom") && bottom == "":
			bottom = p
		}
		if p != PageNone {
			none = false
		}
	}
	if top == "" && bottom == "" && !none {
		bottom = PageBottomEnd
	}
	return top, bottom
}
