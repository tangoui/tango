package ui

// VirtualTree: a tree over TreeNode data that flattens the visible rows
// once per change and only builds the rows inside the viewport, so 100k
// nodes expand, check and scroll at full speed (Element Plus el-tree-v2,
// Ant Design Tree virtual). Switchers, titles and check boxes reuse the
// Tree drawing (paintSwitcher, drawCheckbox) so both trees look alike.

import (
	"github.com/tangoui/tango/ui/a11y"
	"strings"
	"unsafe"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// fillVTItem is a node of the preorder index.
type fillVTItem struct {
	n      *TreeNode
	depth  int32
	parent int32 // -1 for roots
	end    int32 // preorder index after the subtree
}

type fillVTData struct {
	ptr     unsafe.Pointer
	n       int
	version int
	gen     int // bumped when lazy loading finished
}

type fillVTRowsKey struct {
	data   fillVTData
	exp    int
	filter int
}

type fillVTState struct {
	data  fillVTData
	items []fillVTItem
	index map[string]int32
	lower []string

	expandedKeys []string // used when the app binds nothing
	expanded     map[string]bool
	expSeen      []string
	expVer       int

	checkedKeys  []string
	full, half   []bool
	chkSeen      []string
	chkData      fillVTData
	chkStrict    bool
	currentKey   string
	query        string
	filterVer    int
	visible      []bool
	rowsKey      fillVTRowsKey
	rows         []int32
	loading      map[string]bool
	loaded       map[string]bool
	gen          int
	cursor       string
	reveal       bool
	revealCenter bool
}

// VirtualTreeW is a virtualized tree. Configure it with modifiers and
// call Show.
type VirtualTreeW struct {
	c         *Context
	nodes     []TreeNode
	st        *fillVTState
	expanded  *[]string
	checked   *[]string
	current   *string
	checkable bool
	strict    bool
	expClick  bool
	expSet    bool
	chkClick  bool
	loadData  func(key string) bool
	query     string
	filterFn  func(query string, n *TreeNode) bool
	render    func(n *TreeNode, depth int)
	height    float32
	version   int
	scrollTo  string

	sb                *ScrollBox
	box               *Box
	first, end        int
	checkCh, expCh    bool
	curCh             bool
	focusVis          bool
	rowH, gap, indent float32
	switchW           float32
	paint             *TreeW // borrowed Tree drawing helpers
	clickedKey        string
	clickedNode       bool
}

// VirtualTree starts a virtualized tree over nodes (keys unique in the
// whole tree); configure with modifiers and call Show.
//
// Keyboard (the tree is one Tab stop): Up/Down/PageUp/PageDown/Home/End
// move the cursor row, Right expands or enters the first child, Left
// collapses or returns to the parent, Enter makes the row current, Space
// checks it when Checkable.
//
//	ui.VirtualTree(c, nodes).Checkable(&checked).Height(400).Show()
func VirtualTree(c *Context, nodes []TreeNode) *VirtualTreeW {
	id := c.newID("fill-vtree", "")
	st := State(c, id, func() fillVTState {
		return fillVTState{expanded: map[string]bool{}, loading: map[string]bool{}, loaded: map[string]bool{}}
	})
	w := &VirtualTreeW{c: c, nodes: nodes, st: st, height: 200}
	w.expanded, w.checked, w.current = &st.expandedKeys, &st.checkedKeys, &st.currentKey
	return w
}

// Expanded binds the expanded keys to the app.
func (w *VirtualTreeW) Expanded(keys *[]string) *VirtualTreeW { w.expanded = keys; return w }

// Checkable shows check boxes bound to checked keys (fully checked keys,
// parents included; nil keeps them inside the tree).
func (w *VirtualTreeW) Checkable(checked *[]string) *VirtualTreeW {
	w.checkable = true
	if checked != nil {
		w.checked = checked
	}
	return w
}

// CheckStrictly makes parent and child check states independent.
func (w *VirtualTreeW) CheckStrictly() *VirtualTreeW { w.strict = true; return w }

// CheckOnClick toggles the check box when a row is clicked.
func (w *VirtualTreeW) CheckOnClick() *VirtualTreeW { w.chkClick = true; return w }

// Current binds the current (highlighted) node key.
func (w *VirtualTreeW) Current(key *string) *VirtualTreeW { w.current = key; return w }

// ExpandOnClick toggles expansion when a row is clicked (Element's
// expand-on-click-node; on by default with the Element theme).
func (w *VirtualTreeW) ExpandOnClick(on bool) *VirtualTreeW {
	w.expClick, w.expSet = on, true
	return w
}

// LoadData enables lazy loading: expanding a non-leaf node without
// children calls fn(key) every frame until it returns false (still
// loading = true). Add the children to the node meanwhile; the tree
// re-indexes when loading finishes.
func (w *VirtualTreeW) LoadData(fn func(key string) bool) *VirtualTreeW { w.loadData = fn; return w }

// Filter shows only nodes matching query (case-insensitive title match)
// and their ancestors, expanding the ancestors when the query changes.
func (w *VirtualTreeW) Filter(query string) *VirtualTreeW { w.query = query; return w }

// FilterMethod replaces the title match used by Filter.
func (w *VirtualTreeW) FilterMethod(fn func(query string, n *TreeNode) bool) *VirtualTreeW {
	w.filterFn = fn
	return w
}

// NodeRender builds each row's content with fn instead of the title.
func (w *VirtualTreeW) NodeRender(fn func(n *TreeNode, depth int)) *VirtualTreeW {
	w.render = fn
	return w
}

// Height sets the viewport height (default 200, Element's default).
func (w *VirtualTreeW) Height(px float32) *VirtualTreeW { w.height = px; return w }

// DataVersion tells the tree that nodes were modified in place (the index
// is keyed by the slice identity and this number).
func (w *VirtualTreeW) DataVersion(v int) *VirtualTreeW { w.version = v; return w }

// ScrollToKey expands the ancestors of key and scrolls its row into view.
func (w *VirtualTreeW) ScrollToKey(key string) *VirtualTreeW { w.scrollTo = key; return w }

// CheckChanged / ExpandChanged / CurrentChanged report changes in Show.
func (w *VirtualTreeW) CheckChanged() bool   { return w.checkCh }
func (w *VirtualTreeW) ExpandChanged() bool  { return w.expCh }
func (w *VirtualTreeW) CurrentChanged() bool { return w.curCh }

// NodeClicked returns the key of the row clicked in Show.
func (w *VirtualTreeW) NodeClicked() (string, bool) { return w.clickedKey, w.clickedNode }

// HalfChecked returns the half-checked keys (after Show).
func (w *VirtualTreeW) HalfChecked() []string {
	var out []string
	for i, h := range w.st.half {
		if h {
			out = append(out, w.st.items[i].n.Key)
		}
	}
	return out
}

// VisibleCount returns the number of rows (expanded and not filtered out).
func (w *VirtualTreeW) VisibleCount() int { return len(w.st.rows) }

// Range returns the rows [first, end) built by Show.
func (w *VirtualTreeW) Range() (first, end int) { return w.first, w.end }

// ID returns the scroll viewport's ID (after Show).
func (w *VirtualTreeW) ID() ID {
	if w.sb == nil {
		return 0
	}
	return w.sb.n.id
}

// ---------------------------------------------------------------------------
// Index

func (w *VirtualTreeW) dataKey() fillVTData {
	k := fillVTData{n: len(w.nodes), version: w.version, gen: w.st.gen}
	if len(w.nodes) > 0 {
		k.ptr = unsafe.Pointer(&w.nodes[0])
	}
	return k
}

// fillVTIndex builds the preorder index of nodes.
func fillVTIndex(nodes []TreeNode) ([]fillVTItem, map[string]int32) {
	var items []fillVTItem
	var rec func(ns []TreeNode, depth, parent int32)
	rec = func(ns []TreeNode, depth, parent int32) {
		for i := range ns {
			me := int32(len(items))
			items = append(items, fillVTItem{n: &ns[i], depth: depth, parent: parent})
			rec(ns[i].Children, depth+1, me)
			items[me].end = int32(len(items))
		}
	}
	rec(nodes, 0, -1)
	index := make(map[string]int32, len(items))
	for i := range items {
		index[items[i].n.Key] = int32(i)
	}
	return items, index
}

func (w *VirtualTreeW) prepare() {
	st := w.st
	k := w.dataKey()
	if k == st.data && st.items != nil {
		return
	}
	st.data = k
	st.items, st.index = fillVTIndex(w.nodes)
	st.lower = nil
	st.filterVer++
	st.query = "\x00" // recompute the filter
}

// syncExpanded imports the app's expanded keys when they changed.
func (w *VirtualTreeW) syncExpanded() {
	st := w.st
	app := *w.expanded
	if fillSameSlice(app, st.expSeen) {
		return
	}
	st.expanded = make(map[string]bool, len(app))
	for _, k := range app {
		st.expanded[k] = true
	}
	st.expSeen = app
	st.expVer++
}

// fillSameSlice reports whether a and b share length and backing array.
func fillSameSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || &a[0] == &b[0]
}

func (w *VirtualTreeW) setExpanded(key string, on bool) {
	st := w.st
	if st.expanded[key] == on {
		return
	}
	if on {
		st.expanded[key] = true
		*w.expanded = append((*w.expanded)[:len(*w.expanded):len(*w.expanded)], key)
	} else {
		delete(st.expanded, key)
		*w.expanded = remove(*w.expanded, key)
	}
	st.expSeen = *w.expanded
	st.expVer++
	w.expCh = true
	w.c.invalid = true
}

func (w *VirtualTreeW) expandable(i int32) bool {
	n := w.st.items[i].n
	if len(n.Children) > 0 {
		return true
	}
	return w.loadData != nil && !n.IsLeaf && !w.st.loaded[n.Key]
}

func (w *VirtualTreeW) toggleExpand(i int32, on bool) {
	n := w.st.items[i].n
	w.setExpanded(n.Key, on)
	if on && len(n.Children) == 0 && w.loadData != nil && !n.IsLeaf && !w.st.loaded[n.Key] {
		w.st.loading[n.Key] = true
	}
}

// ---------------------------------------------------------------------------
// Checking

// syncChecked imports the app's checked keys and conducts them.
func (w *VirtualTreeW) syncChecked() {
	st := w.st
	app := *w.checked
	if fillSameSlice(app, st.chkSeen) && st.chkData == st.data && st.chkStrict == w.strict && st.full != nil {
		return
	}
	n := len(st.items)
	st.full, st.half = make([]bool, n), make([]bool, n)
	for _, k := range app {
		if i, ok := st.index[k]; ok {
			st.full[i] = true
		}
	}
	if !w.strict {
		for i := range st.items {
			if p := st.items[i].parent; p >= 0 && st.full[p] && treeCheckable(st.items[i].n) {
				st.full[i] = true
			}
		}
		w.conductUp()
	}
	st.chkSeen, st.chkData, st.chkStrict = app, st.data, w.strict
}

// conductUp recomputes parents from their checkable children in one
// reverse preorder pass: full when all are full, half when some are.
func (w *VirtualTreeW) conductUp() {
	st := w.st
	n := len(st.items)
	cnt := make([]int32, n)
	notAll := make([]bool, n)
	some := make([]bool, n)
	for i := n - 1; i >= 0; i-- {
		it := &st.items[i]
		st.half[i] = false
		if cnt[i] > 0 {
			if !notAll[i] && treeCheckable(it.n) {
				st.full[i] = true
			} else {
				st.full[i] = false
				st.half[i] = some[i]
			}
		}
		if p := it.parent; p >= 0 && treeCheckable(it.n) {
			cnt[p]++
			if !st.full[i] {
				notAll[p] = true
			}
			if st.full[i] || st.half[i] {
				some[p] = true
			}
		}
	}
}

// toggleCheck checks or unchecks item i and writes the keys back.
func (w *VirtualTreeW) toggleCheck(i int32, on bool) {
	st := w.st
	it := st.items[i]
	if !treeCheckable(it.n) {
		return
	}
	if w.strict {
		st.full[i] = on
	} else {
		for j := i; j < it.end; j++ {
			if j == i || treeCheckable(st.items[j].n) {
				st.full[j] = on
			}
		}
		if !on {
			for p := it.parent; p >= 0; p = st.items[p].parent {
				st.full[p] = false
			}
		}
		w.conductUp()
	}
	keys := make([]string, 0, len(*w.checked)+1)
	for j, f := range st.full {
		if f {
			keys = append(keys, st.items[j].n.Key)
		}
	}
	*w.checked = keys
	st.chkSeen = keys
	w.checkCh = true
	w.c.invalid = true
}

// ---------------------------------------------------------------------------
// Filtering and rows

func (w *VirtualTreeW) match(i int) bool {
	st := w.st
	if w.filterFn != nil {
		return w.filterFn(w.query, st.items[i].n)
	}
	if st.lower == nil {
		st.lower = make([]string, len(st.items))
		for j := range st.items {
			st.lower[j] = strings.ToLower(st.items[j].n.Title)
		}
	}
	return strings.Contains(st.lower[i], strings.ToLower(w.query))
}

// applyFilter recomputes the visible set when the query changed and
// expands the ancestors of every match.
func (w *VirtualTreeW) applyFilter() {
	st := w.st
	if w.query == st.query {
		return
	}
	st.query = w.query
	st.filterVer++
	if w.query == "" {
		st.visible = nil
		return
	}
	vis := make([]bool, len(st.items))
	var open []string
	for i := range st.items {
		if !w.match(i) {
			continue
		}
		vis[i] = true
		for p := st.items[i].parent; p >= 0 && !vis[p]; p = st.items[p].parent {
			vis[p] = true
			open = append(open, st.items[p].n.Key)
		}
	}
	st.visible = vis
	for _, k := range open {
		if !st.expanded[k] {
			st.expanded[k] = true
			*w.expanded = append((*w.expanded)[:len(*w.expanded):len(*w.expanded)], k)
			w.expCh = true
		}
	}
	if w.expCh {
		st.expSeen = *w.expanded
		st.expVer++
	}
}

// visibleRows returns the preorder indexes of the shown rows (cached).
func (w *VirtualTreeW) visibleRows() []int32 {
	st := w.st
	k := fillVTRowsKey{data: st.data, exp: st.expVer, filter: st.filterVer}
	if k == st.rowsKey && st.rows != nil {
		return st.rows
	}
	st.rowsKey = k
	rows := st.rows[:0]
	n := int32(len(st.items))
	for i := int32(0); i < n; {
		it := &st.items[i]
		if st.visible != nil && !st.visible[i] {
			i = it.end
			continue
		}
		rows = append(rows, i)
		if it.end > i+1 && st.expanded[it.n.Key] {
			i++
		} else {
			i = it.end
		}
	}
	if rows == nil {
		rows = []int32{}
	}
	st.rows = rows
	return rows
}

func fillVTRowOf(rows []int32, item int32) int {
	for r, i := range rows {
		if i == item {
			return r
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Show

// Show builds the tree.
func (w *VirtualTreeW) Show() *VirtualTreeW {
	c, st := w.c, w.st
	w.paint = &TreeW{c: c, search: w.query}
	w.paint.useTheme()
	if !w.expSet {
		w.expClick = w.paint.el()
	}
	w.rowH, w.gap, w.indent, w.switchW = w.paint.metrics()
	w.prepare()
	w.syncExpanded()
	w.applyFilter()
	// Lazy loading: poll loaders; re-index once a load finishes.
	if w.loadData != nil {
		for k := range st.loading {
			if !w.loadData(k) {
				delete(st.loading, k)
				st.loaded[k] = true
				st.gen++
			}
		}
		if len(st.loading) > 0 {
			c.WakeAt(c.now)
		}
		w.prepare()
	}
	if w.checkable {
		w.syncChecked()
	}
	if w.scrollTo != "" {
		if i, ok := st.index[w.scrollTo]; ok {
			for p := st.items[i].parent; p >= 0; p = st.items[p].parent {
				w.setExpanded(st.items[p].n.Key, true)
			}
			st.cursor, st.reveal, st.revealCenter = w.scrollTo, true, true
		}
	}
	w.box = c.box("fill-vtree", layout.Column, nil)
	ia := c.interact(w.box.n, true, input.CursorDefault)
	fillSem(c, w.box.n, a11y.RoleTree, "")
	w.box.n.ownKeys = true
	w.focusVis = ia.FocusVisible
	if keys := c.KeysFor(w.box.n.id); len(keys) > 0 {
		w.handleKeys(keys)
	}
	w.box.Body(w.build)
	return w
}

func (w *VirtualTreeW) rowHeight() float32 { return w.rowH + w.gap }

func (w *VirtualTreeW) build() {
	c, st := w.c, w.st
	rows := w.visibleRows()
	if len(rows) == 0 {
		Column(c, func() { Empty(c).Simple() }, layout.H(w.height), layout.Center())
		return
	}
	rh := float64(w.rowHeight())
	total := float64(len(rows)) * rh
	w.sb = fillViewport(c, "fill-vtree-list", []layout.Option{layout.H(w.height)})
	w.sb.s.extent.Y = total
	view := fillViewH(w.sb, w.height)
	if st.reveal {
		if i, ok := st.index[st.cursor]; ok {
			if r := fillVTRowOf(rows, i); r >= 0 {
				top := float64(r) * rh
				if st.revealCenter {
					w.sb.s.off.Y = top - (view-rh)/2
					c.invalid = true
				} else {
					fillReveal(c, w.sb, top, rh, view)
				}
			}
		}
		st.reveal, st.revealCenter = false, false
	}
	fillClampOff(w.sb, total, view)
	w.first, w.end = fillSpan(w.sb.s.off.Y, view, rh, len(rows), 3)
	w.sb.s.base.Y = float64(w.first) * rh
	w.sb.Body(func() {
		for r := w.first; r < w.end; r++ {
			w.row(rows[r])
		}
	})
}

func (w *VirtualTreeW) handleKeys(keys []input.KeyEvent) {
	c, st := w.c, w.st
	rows := w.visibleRows()
	if len(rows) == 0 {
		return
	}
	idx := -1
	if i, ok := st.index[st.cursor]; ok {
		idx = fillVTRowOf(rows, i)
	}
	page := max(1, int(w.height/w.rowHeight())-1)
	for _, k := range keys {
		if !k.Down {
			continue
		}
		if idx < 0 {
			idx = 0
			if i, ok := st.index[*w.current]; ok {
				if r := fillVTRowOf(rows, i); r >= 0 {
					idx = r
				}
			}
			if k.Key == input.KeyUp || k.Key == input.KeyDown {
				continue
			}
		}
		item := rows[idx]
		nd := st.items[item].n
		switch treeArrow(c, k.Key) { // RTL: Left expands, Right collapses
		case input.KeyDown:
			idx = min(idx+1, len(rows)-1)
		case input.KeyUp:
			idx = max(idx-1, 0)
		case input.KeyPageDown:
			idx = min(idx+page, len(rows)-1)
		case input.KeyPageUp:
			idx = max(idx-page, 0)
		case input.KeyHome:
			idx = 0
		case input.KeyEnd:
			idx = len(rows) - 1
		case input.KeyRight:
			if w.expandable(item) && !st.expanded[nd.Key] {
				w.toggleExpand(item, true)
			} else if st.expanded[nd.Key] && len(nd.Children) > 0 {
				rows = w.visibleRows()
				idx = min(fillVTRowOf(rows, item)+1, len(rows)-1)
			}
		case input.KeyLeft:
			if st.expanded[nd.Key] && (len(nd.Children) > 0 || st.loading[nd.Key]) {
				w.toggleExpand(item, false)
			} else if p := st.items[item].parent; p >= 0 {
				if r := fillVTRowOf(rows, p); r >= 0 {
					idx = r
				}
			}
		case input.KeyEnter:
			w.setCurrent(nd)
		case input.KeySpace:
			if w.checkable {
				w.toggleCheck(item, !st.full[item])
			} else {
				w.setCurrent(nd)
			}
		default:
			continue
		}
		rows = w.visibleRows()
		idx = max(0, min(idx, len(rows)-1))
	}
	if idx >= 0 && idx < len(rows) {
		st.cursor = st.items[rows[idx]].n.Key
		st.reveal = true
		c.invalid = true
	}
}

func (w *VirtualTreeW) setCurrent(nd *TreeNode) {
	if nd.Disabled || *w.current == nd.Key {
		return
	}
	*w.current = nd.Key
	w.curCh = true
	w.c.invalid = true
}

func (w *VirtualTreeW) row(item int32) {
	c, th, tk, st := w.c, w.paint.th, w.paint.tk, w.st
	it := st.items[item]
	nd := it.n
	el := th.Flavor == theme.Element
	Key(c, nd.Key, func() {
		key := nd.Key
		expandable := w.expandable(item)
		open := st.expanded[key]
		loading := st.loading[key]
		cur := *w.current == key
		cursorRow := w.focusVis && st.cursor == key
		rb := c.box("fill-vtree-row", layout.Row, []layout.Option{
			layout.H(w.rowHeight()), layout.Middle(),
			layout.Padding(geom.Insets{Left: float32(it.depth) * w.indent, Bottom: w.gap}),
		})
		rb.n.label = nd.Title // Driver.Find / ClickLabel target
		if nd.Disabled {
			rb.n.disabled = true
		}
		rowIA := c.interact(rb.n, false, input.CursorPointer)
		if nd.Disabled {
			rb.n.cursor = input.CursorNotAllowed
		}
		fillSem(c, rb.n, a11y.RoleTreeItem, nd.Title, a11y.Selectable|fillIf(cur, a11y.Selected)|
			fillIf(expandable, a11y.Expandable)|fillIf(expandable && open, a11y.Expanded)|fillIf(loading, a11y.Busy)|
			fillIf(w.checkable, a11y.Checkable)|fillIf(w.checkable && st.full[item], a11y.Checked)|
			fillIf(w.checkable && st.half[item] && !st.full[item], a11y.Mixed)|fillIf(nd.Disabled, a11y.Disabled))
		fillKeyboard(c, rb.n)
		var swIA, cbIA Interaction
		var title *node
		rb.Body(func() {
			sn := c.add("fill-vtree-switch", "switch:"+key)
			sn.ln.Width, sn.ln.Height, sn.ln.Shrink = w.switchW, w.rowH, 0
			if expandable && !loading {
				swIA = c.interact(sn, false, input.CursorPointer)
			}
			fillHidden(c, sn) // the row carries Expandable / Expanded
			t := c.Anim(sn.id, "rot", b2f(open), th.DurationMid)
			sn.paint = func(l *paint.List, r geom.Rect) { w.paint.paintSwitcher(l, r, expandable, loading, t) }
			if w.checkable {
				cb := c.add("fill-vtree-check", "check:"+key)
				bs := c.boxSize()
				cb.ln.Width, cb.ln.Height, cb.ln.Shrink = bs, bs, 0
				cb.ln.Margin = geom.Insets{Right: 8}
				dis := !treeCheckable(nd)
				if dis {
					cb.disabled = true
				}
				cbIA = c.interact(cb, false, input.CursorPointer)
				fillHidden(c, cb) // the row carries Checkable / Checked / Mixed
				if dis {
					cb.cursor = input.CursorNotAllowed
				}
				full, half := st.full[item], st.half[item]
				cb.paint = func(l *paint.List, r geom.Rect) {
					drawCheckbox(c, l, r, checkLook{checked: full, indeterminate: half && !full, disabled: dis, hovered: cbIA.Hovered, t: 1})
				}
			}
			if w.render != nil {
				Row(c, func() { w.render(nd, int(it.depth)) }, layout.Grow(1), layout.MinW(0), layout.H(w.rowH), layout.Middle())
				return
			}
			title = c.add("fill-vtree-title", nd.Title)
			title.ln.Height = w.rowH
			title.ln.Grow = 1
			title.ln.MinW = 0
			st0 := c.style(th.FontSize, text.Regular)
			st0.LineHeight = w.rowH
			title.ln.Measure = func(_, _ float32) geom.Size {
				return geom.Size{W: text.Width(nd.Title, st0) + 8, H: w.rowH}
			}
			dis := nd.Disabled
			q := w.query
			title.paint = func(l *paint.List, r geom.Rect) {
				// The title highlight and the selected weight come from the
				// Tree token, as in Tree (shadcn: rounded-md, font-medium).
				ts := st0
				if cur && !dis {
					ts.Weight = themeWeight(tk.NodeSelectedFontWeight)
				}
				if !el {
					tr := r
					tr.W = min(r.W, text.Width(nd.Title, ts)+8)
					tr = treeMirror(tr, r, c.IsRTL()) // hugs the title at the start edge
					rad := geom.Rad(tk.NodeBorderRadius)
					if cur && !dis {
						l.Fill(tr, rad, tk.NodeSelectedBg)
					} else if (rowIA.Hovered || cursorRow) && !dis {
						l.Fill(tr, rad, tk.NodeHoverBg)
					}
				}
				col := tk.TitleColor
				if cur {
					col = tk.NodeSelectedColor
				}
				if dis {
					col = th.TextDisabled
				}
				fillVTTitle(c, l, nd.Title, q, ts, r.X+4, r, col, tk.FilterHighlightColor)
			}
		})
		rb.Paint(func(l *paint.List, r geom.Rect) {
			if !el {
				return
			}
			switch {
			case cur:
				l.Fill(r, geom.Radii{}, tk.NodeSelectedBg)
			case (rowIA.Hovered || cursorRow) && !nd.Disabled:
				l.Fill(r, geom.Radii{}, tk.NodeHoverBg)
			}
		})
		if cursorRow {
			rb.PaintOver(func(l *paint.List, r geom.Rect) {
				if !el {
					return
				}
				l.Stroke(r, geom.Radii{}, 1, th.Primary.Border)
			})
		}
		switch {
		case swIA.Clicked:
			w.toggleExpand(item, !open)
		case cbIA.Clicked && treeCheckable(nd):
			w.toggleCheck(item, !st.full[item])
		case rowIA.Clicked && !nd.Disabled:
			w.clickedKey, w.clickedNode = key, true
			w.setCurrent(nd)
			st.cursor = key
			c.SetFocus(w.box.n.id)
			if w.expClick && expandable {
				w.toggleExpand(item, !open)
			}
			if w.chkClick && w.checkable && treeCheckable(nd) {
				w.toggleCheck(item, !st.full[item])
			}
		}
	})
}

// fillVTTitle draws a title with the case-insensitive match of q
// highlighted in hi (TreeToken.FilterHighlightColor).
func fillVTTitle(c *Context, l *paint.List, s, q string, st text.Style, x float32, r geom.Rect, col, hi theme.Color) {
	i := -1
	if q != "" {
		ls := strings.ToLower(s)
		if len(ls) == len(s) {
			i = strings.Index(ls, strings.ToLower(q))
		}
	}
	if i < 0 || i+len(q) > len(s) {
		treeText(c, l, r, x, s, st, r.Right()-x, r.Y, col)
		return
	}
	if c.IsRTL() {
		treeMarkedText(l, r, x, s, i, i+len(q), st, st, 0, r.Y, col, hi, nil)
		return
	}
	for _, p := range []struct {
		s   string
		col theme.Color
	}{{s[:i], col}, {s[i : i+len(q)], hi}, {s[i+len(q):], col}} {
		if p.s == "" {
			continue
		}
		lay := text.Shape(p.s, st, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(x, r.Y), lay.Glyphs, p.col)
		x += lay.Width
	}
}
