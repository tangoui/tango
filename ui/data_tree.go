package ui

import (
	"math"
	"strings"

	"github.com/tangoui/tango/ui/a11y"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// TreeNode is one node of a Tree. Keys must be unique in the whole tree.
type TreeNode struct {
	Key             string
	Title           string
	Children        []TreeNode
	IsLeaf          bool // never expandable (relevant with LoadData)
	Disabled        bool
	DisableCheckbox bool
	Icon            *icon.Icon // shown with ShowIcon; nil uses folder/file
}

// Tree drop positions reported by Dropped and accepted by TreeMove.
const (
	TreeDropBefore = -1
	TreeDropInside = 0
	TreeDropAfter  = 1
)

type treeDrag struct {
	key    string
	src    ID
	active bool
	over   string
	pos    int
}

type treeState struct {
	expanded   []string
	selected   []string
	checked    []string
	loading    map[string]bool
	loaded     map[string]bool
	lastSearch string
	drag       treeDrag
	targets    map[ID]string // click target -> key (previous build)
	rows       map[string]ID // key -> row id (previous build)
	cursor     string        // keyboard focus row
}

// TreeW is a hierarchical list with expand, select, check, drag and search.
type TreeW struct {
	c         *Context
	th        *theme.Theme     // ThemeFor(CompTree)
	tk        *theme.TreeToken // &th.Components.Tree
	nodes     []TreeNode
	st        *treeState
	expanded  *[]string
	selected  *[]string
	checked   *[]string
	checkable bool
	strict    bool
	showLine  bool
	showIcon  bool
	block     bool
	draggable bool
	multiple  bool
	expClick  bool
	expSet    bool
	loadData  func(key string) bool
	search    string
	filter    bool

	full, half map[string]bool
	visible    map[string]bool
	parents    map[string]string
	targets    map[ID]string
	rows       map[string]ID

	selCh, checkCh, expCh bool
	dropped               bool
	dropDrag, dropOn      string
	dropPos               int
	box                   *Box
	focusVis              bool // keyboard focus: draw the cursor row
	aName                 string
	height                float32 // >0: virtual scrolling viewport height
	sb                    *ScrollBox
	built                 int  // rows built by the last Show
	reveal                bool // the keyboard moved the cursor this build
}

// Tree starts a tree over nodes; configure with modifiers and call Show.
//
// The tree is one Tab stop. Keyboard: Up/Down move the focused row,
// Home/End jump to the first/last row, Right expands (or enters the first
// child), Left collapses (or returns to the parent), Enter selects, Space
// checks when Checkable and selects otherwise.
func Tree(c *Context, nodes []TreeNode) *TreeW {
	id := c.newID("tree", "")
	st := State(c, id, func() treeState {
		return treeState{loading: map[string]bool{}, loaded: map[string]bool{}}
	})
	w := &TreeW{c: c, nodes: nodes, st: st}
	w.useTheme()
	w.expanded, w.selected, w.checked = &st.expanded, &st.selected, &st.checked
	// ConfigProvider.Defaults("Tree", ...) keys: "showLine", "showIcon",
	// "blockNode", "checkable", "multiple", "draggable", "height"
	// (virtual scrolling viewport, px) and "virtual" (false turns the
	// height into a plain scroll area).
	w.showLine = Prop(c, "Tree", "showLine", false)
	w.showIcon = Prop(c, "Tree", "showIcon", false)
	w.block = Prop(c, "Tree", "blockNode", false)
	w.checkable = Prop(c, "Tree", "checkable", false)
	w.multiple = Prop(c, "Tree", "multiple", false)
	w.draggable = Prop(c, "Tree", "draggable", false)
	if h, ok := p2bNum(c, "Tree", "height"); ok && h > 0 {
		w.height = float32(h)
	}
	return w
}

// Height turns the tree into a scrolling viewport of px that builds only
// the rows in view (Ant height / virtual): use it for large trees, cost
// then stays proportional to the viewport instead of the node count.
// Expand / collapse jump without the height animation in this mode.
// For more control (custom row renderers, Range) see VirtualTree.
func (w *TreeW) Height(px float32) *TreeW { w.height = max(0, px); return w }

// Built returns how many rows the last Show built (all visible rows
// without Height, only those in the viewport with it).
func (w *TreeW) Built() int { return w.built }

// A11yLabel sets the tree's accessible name (inside a FormItem the item
// label is used).
func (w *TreeW) A11yLabel(s string) *TreeW { w.aName = s; return w }

// Expanded binds the expanded keys to the app.
func (w *TreeW) Expanded(keys *[]string) *TreeW { w.expanded = keys; return w }

// Selected binds the selected keys to the app.
func (w *TreeW) Selected(keys *[]string) *TreeW { w.selected = keys; return w }

// Checkable shows checkboxes bound to checked keys.
func (w *TreeW) Checkable(checked *[]string) *TreeW {
	w.checkable = true
	if checked != nil {
		w.checked = checked
	}
	return w
}

// CheckStrictly disables parent/child check cascading.
func (w *TreeW) CheckStrictly() *TreeW { w.strict = true; return w }

// ShowLine draws connecting lines (Ant).
func (w *TreeW) ShowLine() *TreeW { w.showLine = true; return w }

// ShowIcon draws node icons before titles.
func (w *TreeW) ShowIcon() *TreeW { w.showIcon = true; return w }

// BlockNode stretches titles to the full row width.
func (w *TreeW) BlockNode() *TreeW { w.block = true; return w }

// Draggable enables drag-to-reorder; read the result with Dropped.
func (w *TreeW) Draggable() *TreeW { w.draggable = true; return w }

// Multiple allows selecting several nodes.
func (w *TreeW) Multiple() *TreeW { w.multiple = true; return w }

// ExpandOnClick toggles expansion when the title is clicked (Element's
// expand-on-click-node, on by default with the Element theme).
func (w *TreeW) ExpandOnClick(on bool) *TreeW { w.expClick, w.expSet = on, true; return w }

// LoadData enables lazy loading: expanding a non-leaf node without children
// calls fn(key) every frame until it returns false (still loading = true);
// a spinner replaces the switcher meanwhile.
func (w *TreeW) LoadData(fn func(key string) bool) *TreeW { w.loadData = fn; return w }

// Search highlights titles containing keyword and, when the keyword
// changes, expands the parents of every match.
func (w *TreeW) Search(keyword string) *TreeW { w.search = keyword; return w }

// Filter hides nodes that neither match the search keyword nor contain a
// match (Element's filter-node-method).
func (w *TreeW) Filter() *TreeW { w.filter = true; return w }

// SelectChanged / CheckChanged / ExpandChanged report changes in the last Show.
func (w *TreeW) SelectChanged() bool { return w.selCh }
func (w *TreeW) CheckChanged() bool  { return w.checkCh }
func (w *TreeW) ExpandChanged() bool { return w.expCh }

// Dropped reports a completed drag in the last Show: the dragged key, the
// key it was dropped on and the position (TreeDropBefore/Inside/After).
func (w *TreeW) Dropped() (drag, drop string, pos int, ok bool) {
	return w.dropDrag, w.dropOn, w.dropPos, w.dropped
}

// HalfChecked returns the keys drawn half-checked after the last Show.
func (w *TreeW) HalfChecked() []string {
	var out []string
	treeWalk(w.nodes, func(n *TreeNode, _ []string) {
		if w.half[n.Key] {
			out = append(out, n.Key)
		}
	})
	return out
}

// Layout applies layout options to the tree (after Show).
func (w *TreeW) Layout(o ...layout.Option) *TreeW {
	if w.box != nil {
		w.box.Style(o...)
	}
	return w
}

// ---------------------------------------------------------------------------
// Pure helpers (tested directly).

// treeWalk visits nodes depth-first with the keys of their ancestors.
func treeWalk(nodes []TreeNode, fn func(n *TreeNode, path []string)) {
	var rec func(ns []TreeNode, path []string)
	rec = func(ns []TreeNode, path []string) {
		for i := range ns {
			fn(&ns[i], path)
			if len(ns[i].Children) > 0 {
				rec(ns[i].Children, append(path[:len(path):len(path)], ns[i].Key))
			}
		}
	}
	rec(nodes, nil)
}

func treeCheckable(n *TreeNode) bool { return !n.Disabled && !n.DisableCheckbox }

// treeConduct resolves checked keys with parent/child cascading: checked
// parents check their whole subtree, a parent is checked when all its
// checkable children are, and half-checked when only some descendants are.
func treeConduct(nodes []TreeNode, keys []string) (full, half map[string]bool) {
	in := map[string]bool{}
	for _, k := range keys {
		in[k] = true
	}
	full, half = map[string]bool{}, map[string]bool{}
	var down func(ns []TreeNode, forced bool)
	down = func(ns []TreeNode, forced bool) {
		for i := range ns {
			n := &ns[i]
			on := (forced && treeCheckable(n)) || in[n.Key]
			if on {
				full[n.Key] = true
			}
			down(n.Children, on)
		}
	}
	down(nodes, false)
	var up func(n *TreeNode)
	up = func(n *TreeNode) {
		all, some, cnt := true, false, 0
		for i := range n.Children {
			ch := &n.Children[i]
			up(ch)
			if !treeCheckable(ch) {
				continue
			}
			cnt++
			if full[ch.Key] {
				some = true
			} else {
				all = false
				if half[ch.Key] {
					some = true
				}
			}
		}
		if cnt == 0 {
			return
		}
		if all && treeCheckable(n) {
			full[n.Key] = true
		} else {
			delete(full, n.Key)
			if some {
				half[n.Key] = true
			}
		}
	}
	for i := range nodes {
		up(&nodes[i])
	}
	return full, half
}

// treeToggleCheck returns the new checked keys after toggling key.
func treeToggleCheck(nodes []TreeNode, checked []string, key string, on, strict bool) []string {
	if strict {
		if on {
			if !contains(checked, key) {
				return append(checked, key)
			}
			return checked
		}
		return remove(checked, key)
	}
	full, _ := treeConduct(nodes, checked)
	if on {
		full[key] = true
	} else {
		delete(full, key)
		treeWalk(nodes, func(n *TreeNode, path []string) {
			if n.Key == key {
				for _, p := range path {
					delete(full, p)
				}
			}
			for _, p := range path {
				if p == key {
					delete(full, n.Key)
				}
			}
		})
	}
	var keys []string
	treeWalk(nodes, func(n *TreeNode, _ []string) {
		if full[n.Key] {
			keys = append(keys, n.Key)
		}
	})
	full, _ = treeConduct(nodes, keys)
	keys = keys[:0]
	treeWalk(nodes, func(n *TreeNode, _ []string) {
		if full[n.Key] {
			keys = append(keys, n.Key)
		}
	})
	return keys
}

// treeSearchParents returns the ancestors of every node whose title
// contains keyword, in tree order.
func treeSearchParents(nodes []TreeNode, keyword string) []string {
	if keyword == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	treeWalk(nodes, func(n *TreeNode, path []string) {
		if strings.Contains(n.Title, keyword) {
			for _, p := range path {
				if !seen[p] {
					seen[p] = true
					out = append(out, p)
				}
			}
		}
	})
	return out
}

// TreeMove returns a copy of nodes with the node drag moved before, inside
// (as first child) or after the node drop. Moving a node into its own
// subtree is ignored.
func TreeMove(nodes []TreeNode, drag, drop string, pos int) []TreeNode {
	if drag == drop {
		return nodes
	}
	inside := false
	treeWalk(nodes, func(n *TreeNode, path []string) {
		if n.Key == drop {
			for _, p := range path {
				if p == drag {
					inside = true
				}
			}
		}
	})
	if inside {
		return nodes
	}
	var moved *TreeNode
	var cut func(ns []TreeNode) []TreeNode
	cut = func(ns []TreeNode) []TreeNode {
		out := make([]TreeNode, 0, len(ns))
		for _, n := range ns {
			if n.Key == drag {
				m := n
				moved = &m
				continue
			}
			n.Children = cut(n.Children)
			out = append(out, n)
		}
		return out
	}
	rest := cut(nodes)
	if moved == nil {
		return nodes
	}
	done := false
	var put func(ns []TreeNode) []TreeNode
	put = func(ns []TreeNode) []TreeNode {
		out := make([]TreeNode, 0, len(ns)+1)
		for _, n := range ns {
			if !done && n.Key == drop {
				done = true
				switch pos {
				case TreeDropBefore:
					out = append(out, *moved, n)
				case TreeDropAfter:
					out = append(out, n, *moved)
				default:
					n.Children = append([]TreeNode{*moved}, n.Children...)
					out = append(out, n)
				}
				continue
			}
			n.Children = put(n.Children)
			out = append(out, n)
		}
		return out
	}
	res := put(rest)
	if !done {
		return nodes
	}
	return res
}

// ---------------------------------------------------------------------------
// Build

// useTheme captures the Tree theme and token of the build scope.
func (w *TreeW) useTheme() {
	w.th = w.c.ThemeFor(theme.CompTree)
	w.tk = &w.th.Components.Tree
}

// look returns the captured theme and token (captured now when the TreeW
// was made without Tree, as a painter).
func (w *TreeW) look() (*theme.Theme, *theme.TreeToken) {
	if w.th == nil {
		w.useTheme()
	}
	return w.th, w.tk
}

func (w *TreeW) el() bool {
	th, _ := w.look()
	return th.Flavor == theme.Element
}

func (w *TreeW) shad() bool {
	th, _ := w.look()
	return th.Flavor == theme.StyleShadcn
}

// metrics are the row height, the gap under a row, the indent per level
// and the switcher column width.
func (w *TreeW) metrics() (rowH, gap, indent, sw float32) {
	_, tk := w.look()
	return tk.TitleHeight, tk.NodeGap, tk.IndentSize, tk.SwitcherSize
}

func (w *TreeW) isExpanded(k string) bool { return contains(*w.expanded, k) }

func (w *TreeW) setExpanded(k string, on bool) {
	if on == w.isExpanded(k) {
		return
	}
	if on {
		*w.expanded = append(*w.expanded, k)
	} else {
		*w.expanded = remove(*w.expanded, k)
	}
	w.expCh = true
	w.c.invalid = true
}

func (w *TreeW) expandable(n *TreeNode) bool {
	if len(n.Children) > 0 {
		return true
	}
	return w.loadData != nil && !n.IsLeaf && !w.st.loaded[n.Key]
}

func (w *TreeW) matches(n *TreeNode) bool {
	return w.search != "" && strings.Contains(n.Title, w.search)
}

// Show builds the tree.
func (w *TreeW) Show() *TreeW {
	c := w.c
	st := w.st
	if !w.expSet {
		w.expClick = w.el()
	}
	// Search: auto-expand the parents of matches when the keyword changes.
	if w.search != st.lastSearch {
		st.lastSearch = w.search
		if w.search != "" {
			*w.expanded = treeSearchParents(w.nodes, w.search)
			w.expCh = true
		}
	}
	w.parents = map[string]string{}
	treeWalk(w.nodes, func(n *TreeNode, path []string) {
		if len(path) > 0 {
			w.parents[n.Key] = path[len(path)-1]
		}
	})
	if w.filter && w.search != "" {
		w.visible = map[string]bool{}
		treeWalk(w.nodes, func(n *TreeNode, path []string) {
			if w.matches(n) {
				w.visible[n.Key] = true
				for _, p := range path {
					w.visible[p] = true
				}
			}
		})
	}
	// Lazy loading bookkeeping: expanded nodes without children load.
	if w.loadData != nil {
		for _, k := range *w.expanded {
			if n := w.find(k); n != nil && len(n.Children) == 0 && !n.IsLeaf && !st.loaded[k] {
				st.loading[k] = true
			}
		}
	}
	for k := range st.loading {
		n := w.find(k)
		if n == nil || len(n.Children) > 0 || w.loadData == nil || !w.loadData(k) {
			delete(st.loading, k)
			st.loaded[k] = true
		}
	}
	w.dragBegin()
	if w.checkable {
		w.refreshChecks()
	}
	w.targets, w.rows = map[ID]string{}, map[string]ID{}
	w.box = c.box("tree", layout.Column, nil)
	ia := c.interact(w.box.n, true, input.CursorDefault)
	w.box.n.ownKeys = true
	w.box.n.setRole(a11y.RoleTree).setState(a11y.Vertical).stateIf(w.multiple, a11y.Multiselectable).fill(w)
	if w.aName != "" {
		w.box.n.setName(w.aName)
	}
	w.focusVis = ia.FocusVisible
	if keys := c.KeysFor(w.box.n.id); len(keys) > 0 {
		w.handleKeys(keys)
	}
	if w.height > 0 {
		w.virtualBody()
	} else {
		w.box.Body(func() { w.level(w.nodes, 0, nil) })
	}
	st.targets, st.rows = w.targets, w.rows
	return w
}

// semantics points the tree at its keyboard row (the focus stays on the
// tree box; screen readers follow ActiveDescendant).
func (w *TreeW) semantics(s *a11y.Node) {
	key := w.st.cursor
	if _, ok := w.rows[key]; !ok {
		key = ""
		if len(*w.selected) > 0 {
			key = (*w.selected)[0]
		}
	}
	if id, ok := w.rows[key]; ok {
		s.ActiveDescendant = a11y.NodeID(id)
	}
}

// visibleRows lists the rows currently shown (expanded and not filtered
// out), in display order.
func (w *TreeW) visibleRows() []*TreeNode {
	var out []*TreeNode
	var rec func(ns []TreeNode)
	rec = func(ns []TreeNode) {
		for i := range ns {
			n := &ns[i]
			if w.visible != nil && !w.visible[n.Key] {
				continue
			}
			out = append(out, n)
			if len(n.Children) > 0 && w.isExpanded(n.Key) {
				rec(n.Children)
			}
		}
	}
	rec(w.nodes)
	return out
}

func treeIndex(rows []*TreeNode, key string) int {
	for i, n := range rows {
		if n.Key == key {
			return i
		}
	}
	return -1
}

// treeArrow maps the horizontal arrow keys to their LTR meaning: Right
// expands / enters the children and Left collapses / goes to the parent in
// LTR, the other way round in RTL (the keys follow the reading
// direction). Other keys are returned unchanged.
func treeArrow(c *Context, k input.Key) input.Key {
	switch prev, next := c.inlineKey(k); {
	case next:
		return input.KeyRight
	case prev:
		return input.KeyLeft
	}
	return k
}

// handleKeys applies keyboard navigation to the focused tree.
func (w *TreeW) handleKeys(keys []input.KeyEvent) {
	c, st := w.c, w.st
	rows := w.visibleRows()
	if len(rows) == 0 {
		return
	}
	idx := treeIndex(rows, st.cursor)
	for _, k := range keys {
		if !k.Down {
			continue
		}
		if idx < 0 {
			// First key: land on the selected row (or the first one).
			idx = 0
			for i, n := range rows {
				if contains(*w.selected, n.Key) {
					idx = i
					break
				}
			}
			if k.Key == input.KeyUp || k.Key == input.KeyDown {
				continue
			}
		}
		nd := rows[idx]
		switch treeArrow(c, k.Key) {
		case input.KeyDown:
			idx = min(idx+1, len(rows)-1)
		case input.KeyUp:
			idx = max(idx-1, 0)
		case input.KeyHome:
			idx = 0
		case input.KeyEnd:
			idx = len(rows) - 1
		case input.KeyRight:
			switch {
			case w.expandable(nd) && !w.isExpanded(nd.Key):
				w.toggleExpand(nd, true)
			case w.isExpanded(nd.Key) && len(nd.Children) > 0:
				rows = w.visibleRows()
				idx = min(treeIndex(rows, nd.Key)+1, len(rows)-1)
			}
		case input.KeyLeft:
			if w.isExpanded(nd.Key) && (len(nd.Children) > 0 || w.st.loading[nd.Key]) {
				w.toggleExpand(nd, false)
			} else if p, ok := w.parents[nd.Key]; ok {
				if i := treeIndex(rows, p); i >= 0 {
					idx = i
				}
			}
		case input.KeyEnter:
			w.selectNode(nd)
		case input.KeySpace:
			if w.checkable {
				if !nd.Disabled && !nd.DisableCheckbox {
					on := !w.full[nd.Key]
					*w.checked = treeToggleCheck(w.nodes, *w.checked, nd.Key, on, w.strict)
					w.checkCh = true
					w.refreshChecks()
				}
			} else {
				w.selectNode(nd)
			}
		default:
			continue
		}
		rows = w.visibleRows()
		idx = min(idx, len(rows)-1)
	}
	if idx >= 0 && rows[idx].Key != st.cursor {
		st.cursor = rows[idx].Key
		w.reveal = true
		c.invalid = true
	}
}

// refreshChecks recomputes the drawn check states from *w.checked.
func (w *TreeW) refreshChecks() {
	if w.strict {
		w.full, w.half = map[string]bool{}, map[string]bool{}
		for _, k := range *w.checked {
			w.full[k] = true
		}
		return
	}
	w.full, w.half = treeConduct(w.nodes, *w.checked)
}

// selectNode applies a click/Enter on a row title.
func (w *TreeW) selectNode(nd *TreeNode) {
	if nd.Disabled {
		return
	}
	key := nd.Key
	selected := contains(*w.selected, key)
	if w.multiple {
		if selected {
			*w.selected = remove(*w.selected, key)
		} else {
			*w.selected = append(*w.selected, key)
		}
	} else if selected && !w.el() {
		*w.selected = (*w.selected)[:0]
	} else {
		*w.selected = append((*w.selected)[:0], key)
	}
	w.selCh = true
	w.c.invalid = true
}

func (w *TreeW) find(key string) *TreeNode {
	var out *TreeNode
	treeWalk(w.nodes, func(n *TreeNode, _ []string) {
		if n.Key == key {
			out = n
		}
	})
	return out
}

func (w *TreeW) level(nodes []TreeNode, depth int, lasts []bool) {
	shown := nodes
	if w.visible != nil {
		shown = nil
		for _, n := range nodes {
			if w.visible[n.Key] {
				shown = append(shown, n)
			}
		}
	}
	for i := range shown {
		n := &shown[i]
		l := append(lasts[:len(lasts):len(lasts)], i == len(shown)-1)
		pos := i + 1
		Key(w.c, n.Key, func() {
			w.built++
			w.row(n, depth, l, pos, len(shown))
			if len(n.Children) > 0 {
				collMotion(w.c, "tree-motion", w.isExpanded(n.Key), func() {
					w.level(n.Children, depth+1, l)
				})
			}
		})
	}
}

func (w *TreeW) row(nd *TreeNode, depth int, lasts []bool, pos, size int) {
	c, th, tk := w.c, w.th, w.tk
	el := w.el()
	rowH, gap, indent, sw := w.metrics()
	key := nd.Key
	disabled := nd.Disabled
	expandable := w.expandable(nd)
	open := w.isExpanded(key)
	loading := w.st.loading[key]
	selected := contains(*w.selected, key)

	rowLabel := ""
	if el {
		rowLabel = nd.Title
	}
	rb := collNamedBox(c, "tree-row", rowLabel, layout.Row, layout.Padding(geom.Insets{Bottom: gap}), layout.H(rowH+gap), layout.Middle())
	if disabled {
		rb.n.disabled = true
	}
	w.rows[key] = rb.n.id
	// Semantics: a tree item (the tree box keeps focus and handles the
	// keys); switcher, checkbox and title are parts of it.
	rb.n.setRole(a11y.RoleTreeItem).setName(nd.Title).setState(a11y.Selectable).stateIf(selected, a11y.Selected)
	rb.n.stateIf(expandable, a11y.Expandable).stateIf(expandable && open, a11y.Expanded).stateIf(loading, a11y.Busy)
	if w.checkable {
		rb.n.setState(a11y.Checkable).stateIf(w.full[key], a11y.Checked).stateIf(w.half[key] && !w.full[key], a11y.Mixed)
	}
	rb.n.aKbd = true
	if c.a11yOn {
		lv := depth + 1
		rb.n.fill(semFunc(func(s *a11y.Node) {
			s.Level, s.PosInSet, s.SetSize = lv, pos, size
			if !disabled {
				s.Actions |= a11y.Press
			}
		}))
	}
	// Press selects; Expand / Collapse are clicks on the switcher (aTarget).
	a11yPress := false
	if act, _, ok := c.A11yAction(rb.n.id); ok && act == a11y.Press {
		a11yPress = true
	}
	var rowIA Interaction
	if el {
		rowIA = c.interact(rb.n, false, input.CursorPointer)
		if disabled {
			rb.n.cursor = input.CursorNotAllowed
		}
		w.targets[rb.n.id] = key
	}
	var title *node
	var titleIA Interaction
	var sIA Interaction
	rb.Body(func() {
		if depth > 0 {
			ind := c.add("tree-indent", "")
			ind.ln.Width, ind.ln.Height = float32(depth)*indent, 1
			ind.ln.Shrink = 0
		}
		// Switcher.
		sn := c.add("tree-switcher", "switch:"+key)
		sn.ln.Width, sn.ln.Height, sn.ln.Shrink = sw, rowH, 0
		if expandable && !loading {
			sIA = c.interact(sn, false, input.CursorPointer)
			rb.n.aTarget = sn.id
		}
		sn.aHidden = true
		t := c.Anim(sn.id, "rot", b2f(open), th.DurationMid)
		sn.paint = func(l *paint.List, r geom.Rect) { w.paintSwitcher(l, r, expandable, loading, t) }
		// Checkbox.
		if w.checkable {
			v := w.full[key]
			cb := Checkbox(c, "", &v).Indeterminate(w.half[key] && !v).Disabled(disabled || nd.DisableCheckbox)
			cb.n.ln.Margin = geom.Insets{Right: 8}
			cb.n.aHidden = true // the tree item is Checkable
			if !el {
				cb.n.ln.Margin.Left = 0
			}
			if cb.Changed() {
				*w.checked = treeToggleCheck(w.nodes, *w.checked, key, v, w.strict)
				w.checkCh = true
			}
		}
		// Title.
		title = c.add("tree-title", nd.Title)
		title.aHidden = true // named by the tree item
		title.ln.Height = rowH
		if w.block || el {
			title.ln.Grow = 1
			title.ln.MinW = 0
		}
		st := c.style(th.FontSize, text.Regular)
		st.LineHeight = rowH
		hasIcon := w.showIcon
		title.ln.Measure = func(_, _ float32) geom.Size {
			wd := text.Width(nd.Title, st) + 8
			if hasIcon {
				wd += 20
			}
			return geom.Size{W: wd, H: rowH}
		}
		if !el {
			titleIA = c.interact(title, false, input.CursorPointer)
			if disabled {
				title.cursor = input.CursorNotAllowed
			}
			w.targets[title.id] = key
			if rb.n.aTarget == 0 {
				rb.n.aTarget = title.id
			}
		}
		hov := titleIA.Hovered || (w.focusVis && w.st.cursor == key)
		hovA := c.Anim(title.id, "hover", b2f(hov && !disabled), th.DurationFast)
		if selected && !disabled {
			st.Weight = themeWeight(tk.NodeSelectedFontWeight)
		}
		title.paint = func(l *paint.List, r geom.Rect) {
			if !el {
				rad := geom.Rad(tk.NodeBorderRadius)
				if selected && !disabled {
					l.Fill(r, rad, tk.NodeSelectedBg)
				} else if hovA > 0 {
					l.Fill(r, rad, theme.Fade(tk.NodeHoverBg, float64(hovA)))
				}
			}
			x := r.X + 4
			col := tk.TitleColor
			if selected {
				col = tk.NodeSelectedColor
			}
			if disabled {
				col = th.TextDisabled
			}
			if hasIcon {
				w.paintNodeIcon(l, nd, treeMirror(geom.Rect{X: x, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, r, c.IsRTL()), open, col)
				x += 20
			}
			w.paintTitle(l, nd.Title, st, x, r, col)
		}
	})
	// Interactions.
	clickIA := titleIA
	if el {
		clickIA = rowIA
	}
	if a11yPress {
		// The click went to the switcher (aTarget): select instead.
		sIA.Clicked, clickIA.Clicked = false, true
	}
	if sIA.Clicked && !w.dragSuppress() {
		w.toggleExpand(nd, !open)
		open = !open
	}
	if clickIA.Clicked && !disabled && !w.dragSuppress() {
		w.selectNode(nd)
		w.st.cursor = key
		c.SetFocus(w.box.n.id)
		if w.expClick && expandable {
			w.toggleExpand(nd, !open)
		}
	}
	rowHov := c.Anim(rb.n.id, "hover", b2f((rowIA.Hovered || (w.focusVis && w.st.cursor == key)) && !disabled), th.DurationFast)
	rb.Paint(func(l *paint.List, r geom.Rect) {
		if el {
			if selected {
				l.Fill(r, geom.Rad(tk.NodeBorderRadius), tk.NodeSelectedBg)
			} else if rowHov > 0 {
				l.Fill(r, geom.Rad(tk.NodeBorderRadius), theme.Fade(tk.NodeHoverBg, float64(rowHov)))
			}
		}
		if w.showLine {
			w.paintLines(l, r, nd, depth, lasts, open)
		}
	})
	if w.draggable {
		rb.PaintOver(func(l *paint.List, r geom.Rect) { w.paintDrop(l, r, key, title, indent) })
	}
}

func (w *TreeW) toggleExpand(nd *TreeNode, on bool) {
	w.setExpanded(nd.Key, on)
	if on && len(nd.Children) == 0 && w.loadData != nil && !nd.IsLeaf && !w.st.loaded[nd.Key] {
		w.st.loading[nd.Key] = true
		w.c.invalid = true
	}
}

func (w *TreeW) paintSwitcher(l *paint.List, r geom.Rect, expandable, loading bool, t float32) {
	th, tk := w.look()
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	if loading {
		s := float32(14)
		icon.Draw(l, icon.Loading, geom.Rect{X: cx - s/2, Y: cy - s/2, W: s, H: s}, th.Primary.Base, spinAngle(w.c))
		return
	}
	if !expandable {
		return
	}
	// The carets point along the reading direction when collapsed:
	// mirrored in RTL (turned down when open either way).
	rtl := w.c.IsRTL()
	if w.shad() {
		treeMirrorX(l, cx, rtl, func() { shad2bTreeChevron(l, tk.SwitcherColor, r, t) })
		return
	}
	if w.el() {
		// Filled caret pointing right, rotating down when open.
		p := paint.NewPath().MoveTo(cx-2.5, cy-5).LineTo(cx+3.5, cy).LineTo(cx-2.5, cy+5).Close()
		treeMirrorX(l, cx, rtl, func() { l.Path(p.Rotated(float64(t)*math.Pi/2, cx, cy), tk.SwitcherColor) })
		return
	}
	if w.showLine {
		// Plus / minus square.
		b := geom.Rect{X: cx - 5.5, Y: cy - 5.5, W: 11, H: 11}
		l.Fill(b, geom.Rad(2), th.BgContainer)
		l.Stroke(b, geom.Rad(2), 1, th.TextTertiary)
		l.Fill(geom.Rect{X: cx - 3, Y: cy - 0.5, W: 6, H: 1}, geom.Radii{}, tk.SwitcherColor)
		if t < 0.5 {
			l.Fill(geom.Rect{X: cx - 0.5, Y: cy - 3, W: 1, H: 6}, geom.Radii{}, tk.SwitcherColor)
		}
		return
	}
	// Ant: filled caret pointing down, rotated -90° when collapsed.
	p := paint.NewPath().MoveTo(cx-4.5, cy-2.2).LineTo(cx+4.5, cy-2.2).LineTo(cx, cy+3).Close()
	treeMirrorX(l, cx, rtl, func() { l.Path(p.Rotated(-float64(1-t)*math.Pi/2, cx, cy), tk.SwitcherColor) })
}

func (w *TreeW) paintNodeIcon(l *paint.List, nd *TreeNode, r geom.Rect, open bool, col theme.Color) {
	// paintIcon mirrors directional icons (FileText) in RTL.
	if nd.Icon != nil {
		w.c.paintIcon(l, nd.Icon, r, col, 0)
		return
	}
	switch {
	case len(nd.Children) > 0 && open:
		w.c.paintIcon(l, icon.FolderOpen, r, col, 0)
	case len(nd.Children) > 0 || (!nd.IsLeaf && w.loadData != nil):
		w.c.paintIcon(l, icon.Folder, r, col, 0)
	default:
		w.c.paintIcon(l, icon.FileText, r, col, 0)
	}
}

func (w *TreeW) paintTitle(l *paint.List, s string, st text.Style, x float32, r geom.Rect, col theme.Color) {
	th, tk := w.look()
	y := r.Y
	i := -1
	if w.search != "" {
		i = strings.Index(s, w.search)
	}
	if i < 0 {
		treeText(w.c, l, r, x, s, st, r.Right()-x, y, col)
		return
	}
	hi := tk.SearchHighlightColor
	if hi.A == 0 {
		hi = col // a soft mark behind the match keeps the text color
	}
	mark := tk.SearchHighlightBg
	if w.c.IsRTL() {
		var bg func(geom.Rect)
		if mark.A > 0 {
			bg = func(b geom.Rect) {
				l.Fill(geom.Rect{X: b.X, Y: y + (r.H-20)/2, W: b.W, H: 20}, geom.Rad(th.RadiusXS), mark)
			}
		}
		treeMarkedText(l, r, x, s, i, i+len(w.search), st, st, 0, y, col, hi, bg)
		return
	}
	parts := []struct {
		s   string
		col theme.Color
	}{{s[:i], col}, {w.search, hi}, {s[i+len(w.search):], col}}
	for _, p := range parts {
		if p.s == "" {
			continue
		}
		lay := text.Shape(p.s, st, text.Options{NoWrap: true})
		if mark.A > 0 && p.s == w.search {
			l.Fill(geom.Rect{X: x, Y: y + (r.H-20)/2, W: lay.Width, H: 20}, geom.Rad(th.RadiusXS), mark)
		}
		l.Glyphs(geom.Pt(x, y), lay.Glyphs, p.col)
		x += lay.Width
	}
}

// paintLines draws Ant's showLine connectors for one row.
func (w *TreeW) paintLines(l *paint.List, r geom.Rect, nd *TreeNode, depth int, lasts []bool, open bool) {
	rowH, _, indent, sw := w.metrics()
	_, tk := w.look()
	col := tk.LineColor
	colX := func(d int) float32 { return float32(math.Floor(float64(r.X + float32(d)*indent + sw/2))) }
	// The connectors are computed as in a left-to-right row and mirrored
	// in RTL, where the indentation grows from the right.
	rtl := w.c.IsRTL()
	vline := func(x, y, h float32) {
		if rtl {
			x = 2*r.X + r.W - x - 1
		}
		dataVLine(l, x, y, h, col)
	}
	hline := func(x, y, wd float32) {
		if rtl {
			x = 2*r.X + r.W - x - wd
		}
		dataHLine(l, x, y, wd, col)
	}
	if w.shad() {
		// shadcn SidebarMenuSub: a plain border-l guide per ancestor level.
		for d := 0; d < depth; d++ {
			vline(colX(d), r.Y, r.H)
		}
		return
	}
	mid := r.Y + rowH/2
	for d := 0; d < depth; d++ {
		x := colX(d)
		if d == depth-1 {
			// Parent column: ├ or └ towards this node.
			bottom := r.Bottom()
			if lasts[depth] {
				bottom = mid
			}
			vline(x, r.Y, bottom-r.Y)
			if !w.expandable(nd) {
				hline(x, mid, colX(depth)-x+4)
			} else {
				hline(x, mid, colX(depth)-x-7)
			}
			continue
		}
		if !lasts[d+1] {
			vline(x, r.Y, r.H)
		}
	}
	if open && len(nd.Children) > 0 {
		x := colX(depth)
		vline(x, mid+5.5, r.Bottom()-mid-5.5)
	}
}

// ---------------------------------------------------------------------------
// Drag and drop

func (w *TreeW) dragSuppress() bool { return w.st.drag.active || w.dropped }

func (w *TreeW) dragBegin() {
	if !w.draggable {
		return
	}
	c, st := w.c, w.st
	d := &st.drag
	if d.active {
		if c.active != d.src {
			// Released (or cancelled).
			if c.released[d.src] && d.over != "" {
				w.dropped = true
				w.dropDrag, w.dropOn, w.dropPos = d.key, d.over, d.pos
				c.invalid = true
			}
			*d = treeDrag{}
			return
		}
		w.dragOver()
		return
	}
	if c.active == 0 {
		return
	}
	key, ok := st.targets[c.active]
	if !ok {
		return
	}
	if n := w.find(key); n == nil || n.Disabled {
		return
	}
	dx, dy := c.pointer.X-c.pressPos.X, c.pointer.Y-c.pressPos.Y
	if dx*dx+dy*dy < 16 {
		return
	}
	*d = treeDrag{key: key, src: c.active, active: true}
	w.dragOver()
}

func (w *TreeW) dragOver() {
	c, st := w.c, w.st
	d := &st.drag
	d.over = ""
	rowH, _, _, _ := w.metrics()
	for k, id := range st.rows {
		r, ok := c.rects[id]
		if !ok || !r.Contains(c.pointer) || k == d.key {
			continue
		}
		// Not into its own subtree.
		bad := false
		for p := w.parents[k]; p != ""; p = w.parents[p] {
			if p == d.key {
				bad = true
				break
			}
		}
		if bad {
			continue
		}
		y := c.pointer.Y - r.Y
		switch {
		case y < rowH*0.25:
			d.pos = TreeDropBefore
		case y > rowH*0.75:
			d.pos = TreeDropAfter
		default:
			d.pos = TreeDropInside
		}
		d.over = k
	}
	c.invalid = true
}

func (w *TreeW) paintDrop(l *paint.List, r geom.Rect, key string, title *node, indent float32) {
	d := w.st.drag
	th, tk := w.look()
	if !d.active {
		return
	}
	if key == d.key && title != nil {
		l.Fill(title.abs, geom.Rad(th.RadiusSM), tk.DraggingNodeBg)
	}
	if d.over != key || title == nil {
		return
	}
	rowH, _, _, _ := w.metrics()
	x := title.abs.X
	y := r.Y + rowH
	switch d.pos {
	case TreeDropBefore:
		y = r.Y
	case TreeDropInside:
		x += indent
	}
	col := tk.DropIndicatorColor
	cx := x + 1
	bar := geom.Rect{X: x + 4, Y: y - 1, W: r.Right() - x - 4, H: 2}
	if w.c.IsRTL() {
		// The indicator starts at the title's start edge (its right edge in
		// RTL) and runs to the row's end edge; "inside" indents to the left.
		x = title.abs.Right()
		if d.pos == TreeDropInside {
			x -= indent
		}
		cx = x - 1
		bar = geom.Rect{X: r.X, Y: y - 1, W: x - 4 - r.X, H: 2}
	}
	l.Fill(bar, geom.Radii{}, col)
	l.Path(paint.NewPath().Circle(cx, y, 4), th.BgContainer)
	l.StrokePath(paint.NewPath().Circle(cx, y, 3), paint.StrokeStyle{Width: 2}, col)
}

// ---------------------------------------------------------------------------
// Virtual scrolling (Height)

type treeFlatRow struct {
	nd        *TreeNode
	depth     int
	lasts     []bool
	pos, size int
}

// flatRows lists the visible rows with their depth, connector state and
// position among their siblings.
func (w *TreeW) flatRows() []treeFlatRow {
	var out []treeFlatRow
	var rec func(ns []TreeNode, depth int, lasts []bool)
	rec = func(ns []TreeNode, depth int, lasts []bool) {
		var shown []*TreeNode
		for i := range ns {
			if w.visible == nil || w.visible[ns[i].Key] {
				shown = append(shown, &ns[i])
			}
		}
		for i, n := range shown {
			l := append(lasts[:len(lasts):len(lasts)], i == len(shown)-1)
			out = append(out, treeFlatRow{nd: n, depth: depth, lasts: l, pos: i + 1, size: len(shown)})
			if len(n.Children) > 0 && w.isExpanded(n.Key) {
				rec(n.Children, depth+1, l)
			}
		}
	}
	rec(w.nodes, 0, nil)
	return out
}

// virtualBody builds the tree box as a scroll viewport holding only the
// rows in view (plus a few above and below). Rows are laid out from 0 and
// the scroll base is the first built row's position, like VirtualList.
func (w *TreeW) virtualBody() {
	c := w.c
	rowH, gap, _, _ := w.metrics()
	ih := float64(rowH + gap)
	w.box.n.ln.ScrollY = true
	w.box.n.ln.Height = w.height
	rows := w.flatRows()
	w.sb = c.finishScroll(w.box, func() {
		s := State[scrollState](c, w.box.n.id, nil) // the viewport's scroll state
		s.extent.Y = float64(len(rows)) * ih
		view := float64(w.height)
		if s.view.H > 0 {
			view = float64(s.view.H)
		}
		// Keep the keyboard row in view.
		if i := treeFlatIndex(rows, w.st.cursor); i >= 0 && w.reveal {
			top := float64(i) * ih
			switch {
			case top < s.off.Y:
				s.off.Y = top
			case top+ih > s.off.Y+view:
				s.off.Y = top + ih - view
			}
		}
		s.off.Y = max(0, min(s.off.Y, s.extent.Y-view))
		const overscan = 3
		first := max(0, int(math.Floor(s.off.Y/ih))-overscan)
		end := min(len(rows), int(math.Ceil((s.off.Y+view)/ih))+overscan)
		s.base.Y = float64(first) * ih
		for _, fr := range rows[first:end] {
			fr := fr
			Key(c, fr.nd.Key, func() {
				w.built++
				w.row(fr.nd, fr.depth, fr.lasts, fr.pos, fr.size)
			})
		}
	})
}

func treeFlatIndex(rows []treeFlatRow, key string) int {
	if key == "" {
		return -1
	}
	for i, r := range rows {
		if r.nd.Key == key {
			return i
		}
	}
	return -1
}
