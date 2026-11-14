package ui

import (
	"strings"

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

// TreeSelectNode is a node of a TreeSelect tree. Values must be unique.
// With LoadData, a node without Children that is not IsLeaf loads its
// children when expanded.
type TreeSelectNode struct {
	Label, Value string
	Disabled     bool
	Children     []TreeSelectNode
	IsLeaf       bool // never expandable (relevant with LoadData)
}

type tsState struct {
	pop      entPopup
	expanded map[string]bool
	inited   bool
	active   int
	wasOpen  bool

	loading, loaded map[string]bool // LoadData bookkeeping (p2b_treeselect.go)
	// Virtual list: highlight at the last build, list shown, pointer.
	shown     int
	shownH    float32 // row height at the last build (theme switches)
	listShown bool
	ptr       geom.Point
}

type tsRow struct {
	nd     *TreeSelectNode
	depth  int
	parent int
	match  bool
}

// TreeSelectW is a select whose options form a tree.
type TreeSelectW struct {
	_ [0]func()

	c         *Context
	th        *theme.Theme     // ThemeFor(CompTree): the panel's tree rows
	tk        *theme.TreeToken // &th.Components.Tree
	box       *selector
	n         *node // box.n
	val       *string
	vals      *[]string
	nodes     []TreeSelectNode
	st        *tsState
	checkable bool
	expandAll bool
	forceOpen bool
	changed   bool
	z         *lazy
	loadData  func(value string) bool
	strategy  CheckedStrategy
	virtual   int // 0 auto (long lists), 1 always, -1 never
	listH     float32
	chipVals  []string // node value of each tag (checked strategy)
	stratVal  bool     // the bound value follows the checked strategy
	expKeys   *[]string
	expChange bool
	strictly  bool                                         // CheckStrictly
	filterFn  func(query string, node TreeSelectNode) bool // FilterTreeNode
}

// TreeSelect adds a tree select bound to *val (single selection). Use
// Multiple / Checkable for multiple values.
func TreeSelect(c *Context, val *string, nodes []TreeSelectNode) *TreeSelectW {
	w := &TreeSelectW{c: c, val: val, nodes: nodes}
	w.th = c.ThemeFor(theme.CompTree)
	w.tk = &w.th.Components.Tree
	w.box = newSelector(c, "treeselect")
	w.box.placeholder = c.Locale().SelectPlaceholder
	w.n = w.box.n
	w.st = State(c, w.n.id, func() tsState { return tsState{expanded: map[string]bool{}} })
	w.treeSelectDefaults()
	w.z = c.lazy(w.n, w.finish)
	return w
}

// Multiple binds the selection to *vals instead (each click toggles a node).
func (w *TreeSelectW) Multiple(vals *[]string) *TreeSelectW { w.vals = vals; return w }

// Checkable shows checkboxes with parent/child linkage (implies Multiple;
// the value holds the checked leaves).
func (w *TreeSelectW) Checkable() *TreeSelectW {
	w.checkable = true
	if w.vals == nil {
		w.vals = new([]string)
	}
	return w
}

// CheckStrictly unlinks parents and children in Checkable mode (Ant
// treeCheckStrictly, Element check-strictly; implies Checkable): every
// node, parent or leaf, is checked on its own, the value holds exactly the
// checked nodes and each one is a tag. ShowCheckedStrategy and
// StrategyValue do not apply.
func (w *TreeSelectW) CheckStrictly() *TreeSelectW {
	w.strictly = true
	return w.Checkable()
}

// FilterTreeNode replaces the search match (by default the label
// containing the typed text, case-insensitively) with fn, which receives
// the typed text and a node (Ant filterTreeNode, Element
// filter-node-method). Matching nodes are shown with their ancestors;
// the typed text is still highlighted where it appears in a label.
// Implies Searchable.
func (w *TreeSelectW) FilterTreeNode(fn func(query string, node TreeSelectNode) bool) *TreeSelectW {
	w.filterFn, w.box.search = fn, true
	return w
}

func (w *TreeSelectW) Placeholder(s string) *TreeSelectW { w.box.placeholder = s; return w }

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; the placeholder is the last resort).
func (w *TreeSelectW) A11yLabel(s string) *TreeSelectW { w.n.setName(s); return w }
func (w *TreeSelectW) Searchable() *TreeSelectW        { w.box.search = true; return w }
func (w *TreeSelectW) AllowClear() *TreeSelectW        { w.box.allowClear = true; return w }
func (w *TreeSelectW) DefaultExpandAll() *TreeSelectW  { w.expandAll = true; return w }
func (w *TreeSelectW) MaxTagCount(n int) *TreeSelectW  { w.box.maxCount = n; return w }
func (w *TreeSelectW) Status(s string) *TreeSelectW    { w.box.status = s; return w }

// Variant sets the frame look (FieldOutlined by default).
func (w *TreeSelectW) Variant(v FieldVariant) *TreeSelectW { w.box.variant = v; return w }

func (w *TreeSelectW) Size(s ControlSize) *TreeSelectW { w.box.size = s; return w }
func (w *TreeSelectW) Width(px float32) *TreeSelectW   { w.n.ln.Width = px; return w }
func (w *TreeSelectW) Layout(o ...layout.Option) *TreeSelectW {
	flexbox.Apply(&w.n.ln.Style, o...)
	return w
}

// Disabled disables the control.
func (w *TreeSelectW) Disabled(on bool) *TreeSelectW {
	if on {
		w.box.disabled = true
	}
	return w
}

// Open forces the dropdown open (screenshots, tests).
func (w *TreeSelectW) Open() *TreeSelectW { w.forceOpen = true; w.st.pop.open = true; return w }

// IsOpen reports whether the dropdown is shown.
func (w *TreeSelectW) IsOpen() bool { return w.st.pop.open }

// Changed reports a selection change this frame.
func (w *TreeSelectW) Changed() bool { w.z.run(); return w.changed }

func (w *TreeSelectW) multiple() bool { return w.vals != nil }

// parentValue returns the value of v's parent ("" at the top level).
func (w *TreeSelectW) parentValue(v string) string {
	p, _ := tsPath(w.nodes, v)
	if len(p) == 0 {
		return ""
	}
	return p[len(p)-1]
}

// ---------------------------------------------------------------------------
// Tree helpers

func tsFind(ns []TreeSelectNode, v string) *TreeSelectNode {
	for i := range ns {
		if ns[i].Value == v {
			return &ns[i]
		}
		if f := tsFind(ns[i].Children, v); f != nil {
			return f
		}
	}
	return nil
}

// tsPath returns the values of the ancestors of v (root first).
func tsPath(ns []TreeSelectNode, v string) ([]string, bool) {
	for i := range ns {
		if ns[i].Value == v {
			return nil, true
		}
		if p, ok := tsPath(ns[i].Children, v); ok {
			return append([]string{ns[i].Value}, p...), true
		}
	}
	return nil, false
}

// tsLeaves appends the leaves under nd (nd itself for a leaf), skipping
// disabled branches when enabledOnly.
func tsLeaves(nd *TreeSelectNode, enabledOnly bool, out []string) []string {
	if enabledOnly && nd.Disabled {
		return out
	}
	if len(nd.Children) == 0 {
		return append(out, nd.Value)
	}
	for i := range nd.Children {
		out = tsLeaves(&nd.Children[i], enabledOnly, out)
	}
	return out
}

// tsMatches reports whether nd or a descendant matches the search: self
// reports a node's own match.
func tsMatches(nd *TreeSelectNode, self func(*TreeSelectNode) bool) bool {
	if self(nd) {
		return true
	}
	for i := range nd.Children {
		if tsMatches(&nd.Children[i], self) {
			return true
		}
	}
	return false
}

// matcher returns the own-match test of the search text q (lower-cased
// and trimmed; raw is the text as typed, given to FilterTreeNode).
func (w *TreeSelectW) matcher(q, raw string) func(*TreeSelectNode) bool {
	if w.filterFn != nil {
		return func(nd *TreeSelectNode) bool { return w.filterFn(raw, *nd) }
	}
	return func(nd *TreeSelectNode) bool { return strings.Contains(strings.ToLower(nd.Label), q) }
}

func (w *TreeSelectW) expandAllNodes(ns []TreeSelectNode) {
	for i := range ns {
		if len(ns[i].Children) > 0 {
			w.st.expanded[ns[i].Value] = true
			w.expandAllNodes(ns[i].Children)
		}
	}
}

// rows flattens the visible part of the tree. While searching, nodes that
// match or have a matching descendant are shown with their ancestors open.
func (w *TreeSelectW) rows() []tsRow {
	raw := strings.TrimSpace(w.box.st.query)
	q := strings.ToLower(raw)
	self := w.matcher(q, raw)
	var out []tsRow
	var walk func(ns []TreeSelectNode, depth, parent int)
	walk = func(ns []TreeSelectNode, depth, parent int) {
		for i := range ns {
			nd := &ns[i]
			if q != "" && !tsMatches(nd, self) {
				continue
			}
			idx := len(out)
			out = append(out, tsRow{nd: nd, depth: depth, parent: parent, match: q != "" && self(nd)})
			if len(nd.Children) > 0 && (q != "" || w.st.expanded[nd.Value]) {
				walk(nd.Children, depth+1, idx)
			}
		}
	}
	walk(w.nodes, 0, -1)
	return out
}

// Check states returned by checkState.
const (
	tsOff  = 0
	tsOn   = 1
	tsHalf = 2
)

// checkState returns tsOff/tsOn/tsHalf for nd in checkable mode, or
// selected/unselected otherwise.
func (w *TreeSelectW) checkState(nd *TreeSelectNode, set map[string]bool) int {
	if !w.checkable || len(nd.Children) == 0 || w.strictly {
		if (w.multiple() && set[nd.Value]) || (!w.multiple() && *w.val == nd.Value) {
			return tsOn
		}
		return tsOff
	}
	leaves := tsLeaves(nd, true, nil)
	if len(leaves) == 0 {
		leaves = tsLeaves(nd, false, nil)
	}
	on := 0
	for _, v := range leaves {
		if set[v] {
			on++
		}
	}
	switch {
	case on == 0:
		// A disabled checked leaf still makes the parent half-checked.
		for _, v := range tsLeaves(nd, false, nil) {
			if set[v] {
				return tsHalf
			}
		}
		return tsOff
	case on == len(leaves):
		return tsOn
	}
	return tsHalf
}

func (w *TreeSelectW) valueSet() map[string]bool {
	set := map[string]bool{}
	if w.vals != nil {
		for _, v := range *w.vals {
			set[v] = true
		}
	}
	return set
}

// act applies a click / Enter on a row.
func (w *TreeSelectW) act(nd *TreeSelectNode) {
	if nd.Disabled {
		return
	}
	c := w.c
	switch {
	case !w.multiple():
		if *w.val != nd.Value {
			*w.val = nd.Value
			w.changed = true
		}
		w.st.pop.open = false
		w.box.st.query = ""
		c.SetFocus(w.box.ed.id)
	case w.strictly:
		if contains(*w.vals, nd.Value) {
			*w.vals = remove(*w.vals, nd.Value)
		} else {
			*w.vals = append(*w.vals, nd.Value)
		}
		w.changed = true
	case w.checkable:
		set := w.valueSet()
		leaves := tsLeaves(nd, true, nil)
		if w.checkState(nd, set) == tsOn {
			for _, v := range leaves {
				*w.vals = remove(*w.vals, v)
			}
		} else {
			for _, v := range leaves {
				if !set[v] {
					*w.vals = append(*w.vals, v)
				}
			}
		}
		w.changed = true
	default:
		if contains(*w.vals, nd.Value) {
			*w.vals = remove(*w.vals, nd.Value)
		} else {
			*w.vals = append(*w.vals, nd.Value)
		}
		w.changed = true
	}
	if w.multiple() && w.box.st.query != "" {
		w.box.st.query = ""
		w.box.st.edit = editState{}
	}
	c.invalid = true
}

func (w *TreeSelectW) hasValue() bool {
	if w.multiple() {
		return len(*w.vals) > 0
	}
	return *w.val != ""
}

func (w *TreeSelectW) label() string {
	if nd := tsFind(w.nodes, *w.val); nd != nil {
		return nd.Label
	}
	return ""
}

func (w *TreeSelectW) openAt() {
	st := w.st
	st.pop.open = true
	st.wasOpen = true
	w.box.st.query = ""
	st.active = 0
	cur := []string{}
	if w.multiple() {
		cur = append(cur, *w.vals...)
	} else if *w.val != "" {
		cur = append(cur, *w.val)
	}
	for _, v := range cur {
		if p, ok := tsPath(w.nodes, v); ok {
			for _, a := range p {
				st.expanded[a] = true
			}
		}
	}
	if len(cur) > 0 {
		for i, r := range w.rows() {
			if r.nd.Value == cur[0] {
				st.active = i
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Build

func (w *TreeSelectW) finish() {
	if w.expKeys != nil {
		w.expandIn()
	}
	var user *[]string
	if w.stratVal && w.checkable && !w.strictly {
		// Work on the checked leaves; write the strategy values back.
		user = w.vals
		leaves := tsExpandValues(w.nodes, *user)
		w.vals = &leaves
	}
	w.build()
	if user != nil {
		if w.changed {
			_, vals := w.chips()
			*user = vals
			w.chipVals = append([]string(nil), vals...)
		}
		w.vals = user
	}
	if w.expKeys != nil {
		w.expandOut()
	}
}

func (w *TreeSelectW) build() {
	c, b, st := w.c, w.box, w.st
	if !st.inited {
		st.inited = true
		if w.expandAll && (w.expKeys == nil || len(*w.expKeys) == 0) {
			w.expandAllNodes(w.nodes)
		}
	}
	w.pollLoads()
	var labels []string
	w.chipVals = nil
	if w.checkable {
		labels, w.chipVals = w.chips()
		if b.n.ln.Width == 0 {
			b.n.ln.Width = 280
		}
	} else if w.multiple() {
		for _, v := range *w.vals {
			if nd := tsFind(w.nodes, v); nd != nil {
				labels = append(labels, nd.Label)
			} else {
				labels = append(labels, v)
			}
		}
		if b.n.ln.Width == 0 {
			b.n.ln.Width = 280
		}
	} else {
		b.single = w.label()
	}
	b.arrowIcon = nil
	if b.search && st.pop.open && w.th.Flavor == theme.Ant {
		b.arrowIcon = icon.Search
	}
	b.open = st.pop.open
	b.build(labels)
	if b.disabled {
		st.pop.open, b.open = false, false
		return
	}
	switch {
	case w.checkable && b.removed >= 0 && b.removed < len(w.chipVals):
		w.removeChip(w.chipVals[b.removed])
	case w.multiple() && b.removed >= 0 && b.removed < len(*w.vals):
		*w.vals = append((*w.vals)[:b.removed:b.removed], (*w.vals)[b.removed+1:]...)
		w.changed = true
	case b.clearClicked():
		if w.multiple() {
			*w.vals = nil
		} else {
			*w.val = ""
		}
		b.st.query = ""
		w.changed = true
		st.pop.open = false
	case b.clicked():
		if !st.pop.open {
			w.openAt()
		} else if !b.search {
			st.pop.open = false
		}
		c.SetFocus(b.ed.id)
		c.invalid = true
	}
	if !w.forceOpen {
		st.pop.dismiss(c, b.n.id)
	}
	if st.pop.open && c.focus == 0 { // presses inside the panel keep focus
		c.SetFocus(b.ed.id)
	}
	rows := w.rows()
	w.keys(rows)
	if b.edit().changed {
		if !st.pop.open {
			q := b.st.query
			w.openAt()
			b.st.query = q
		}
		st.active = 0
	}
	if !st.pop.open {
		st.wasOpen = false
		b.st.query = ""
		b.open = false
		return
	}
	if !st.wasOpen { // opened by Open(): reveal the current value
		q := b.st.query
		w.openAt()
		b.st.query = q
	}
	b.open = true
	w.dropdown(w.rows())
}

func (w *TreeSelectW) keys(rows []tsRow) {
	c, st, b := w.c, w.st, w.box
	browsing := b.st.query == "" // arrows and Home/End move the caret while typing
	keys := b.takeKeys(func(k input.KeyEvent) bool {
		switch k.Key {
		case input.KeyUp, input.KeyDown, input.KeyEnter:
			return true
		case input.KeyLeft, input.KeyRight, input.KeyHome, input.KeyEnd:
			return st.pop.open && browsing
		case input.KeySpace:
			return !b.search
		case input.KeyBackspace:
			return w.multiple() && browsing
		}
		return false
	})
	for _, k := range keys {
		if k.Key == input.KeyBackspace {
			if w.checkable && len(w.chipVals) > 0 {
				w.removeChip(w.chipVals[len(w.chipVals)-1]) // the last tag, by the checked strategy
				c.invalid = true
				continue
			}
			if n := len(*w.vals); n > 0 {
				*w.vals = append([]string(nil), (*w.vals)[:n-1]...)
				w.changed = true
				c.invalid = true
			}
			continue
		}
		if !st.pop.open {
			switch k.Key {
			case input.KeyDown, input.KeyEnter, input.KeySpace:
				w.openAt()
				c.invalid = true
			}
			continue
		}
		if len(rows) == 0 {
			continue
		}
		st.active = clampIdx(st.active, len(rows)-1)
		row := rows[st.active]
		switch treeArrow(c, k.Key) { // RTL: Left expands, Right collapses
		case input.KeyDown:
			st.active = (st.active + 1) % len(rows)
		case input.KeyUp:
			st.active = (st.active - 1 + len(rows)) % len(rows)
		case input.KeyRight:
			if w.expandable(row.nd) {
				if !st.expanded[row.nd.Value] {
					w.setExpanded(row.nd, true)
				} else if st.active+1 < len(rows) && len(row.nd.Children) > 0 {
					st.active++
				}
			}
		case input.KeyLeft:
			if w.expandable(row.nd) && st.expanded[row.nd.Value] {
				w.setExpanded(row.nd, false)
			} else if row.parent >= 0 {
				st.active = row.parent
			}
		case input.KeyEnter:
			w.act(row.nd)
		case input.KeySpace:
			w.act(row.nd)
		case input.KeyHome:
			st.active = 0
		case input.KeyEnd:
			st.active = len(rows) - 1
		}
		c.invalid = true
	}
}

// Row metrics (Tree token).
func (w *TreeSelectW) rowH() float32 { return w.tk.DropdownNodeHeight }

func (w *TreeSelectW) indent() float32 { return w.tk.IndentSize }

// switcherW is the width of the expand switcher column.
func (w *TreeSelectW) switcherW() float32 { return w.tk.SwitcherSize }

func (w *TreeSelectW) dropdown(rows []tsRow) {
	c, st := w.c, w.st
	el := w.th.Flavor == theme.Element
	set := w.valueSet()
	w.box.listID, w.box.activeID = 0, 0
	ov := entPanel(c, w.box.n.id, &st.pop, true, func() {
		if len(rows) == 0 {
			Empty(c).Simple().Description(c.Locale().NoData)
			return
		}
		if w.virtualOn(len(rows)) {
			w.virtualList(rows, set)
			return
		}
		st.listShown = false
		idx := st.active
		sb := Scroll(c, func() {
			for i, row := range rows {
				i, row := i, row
				Key(c, row.nd.Value, func() {
					w.row(i, row, set)
				})
			}
		}, layout.MaxH(256))
		followRow(c, sb, idx, st.active, 0, w.rowH())
		sb.n.setRole(a11y.RoleTree).setName(w.box.popupName()).stateIf(w.multiple(), a11y.Multiselectable)
		w.box.listID = sb.n.id
	})
	st.ptr = c.pointer
	ov.n.ln.Padding = entListPad(c)
	if !el {
		ov.n.ln.Padding = geom.VH(4, 4)
	}
}

func (w *TreeSelectW) row(i int, row tsRow, set map[string]bool) {
	c, st := w.c, w.st
	th, tk := w.th, w.tk
	el := th.Flavor == theme.Element
	nd := row.nd
	n := c.add("ts-row", nd.Label)
	n.ln.Height = w.rowH()
	hasKids := w.expandable(nd)
	loading := w.isLoading(nd.Value)
	query := w.box.st.query
	expanded := hasKids && (st.expanded[nd.Value] || query != "") && !loading
	shad := shadTree(th)
	padL := float32(0)
	if el {
		padL = 6
	} else if shad {
		padL = 4
	}
	swEnd := padL + float32(row.depth)*w.indent() + w.switcherW()
	n.ln.Measure = func(_, _ float32) geom.Size {
		wd := swEnd + entTextW(c, nd.Label, th.FontSize, text.Regular) + 16
		if w.checkable {
			wd += c.boxSize() + 8
		}
		if shad && !w.checkable {
			wd += shadTreeIcon + 8 // room for the check mark
		}
		return geom.Size{W: wd, H: w.rowH()}
	}
	cur := input.CursorPointer
	if nd.Disabled {
		cur = input.CursorNotAllowed
	}
	ia := c.interact(n, false, cur)
	n.aKbd = true // the trigger's arrow keys move through the rows
	n.setRole(a11y.RoleTreeItem).stateIf(hasKids, a11y.Expandable).stateIf(expanded, a11y.Expanded).stateIf(loading, a11y.Busy)
	if st.active == i {
		w.box.activeID = n.id
	}
	if c.a11yOn {
		n.fill(semFunc(func(s *a11y.Node) {
			s.Level = row.depth + 1
			sibs := w.nodes
			if row.parent >= 0 {
				if p := tsFind(w.nodes, w.parentValue(nd.Value)); p != nil {
					sibs = p.Children
				}
			}
			for k := range sibs {
				if sibs[k].Value == nd.Value {
					s.PosInSet, s.SetSize = k+1, len(sibs)
				}
			}
		}))
	}
	if ia.Hovered {
		st.active = i
	}
	if ia.Clicked {
		x := c.inline(c.rects[n.id]).Off(c.pointer.X) // from the start edge
		if act, _, ok := c.A11yAction(n.id); ok && (act == a11y.Expand || act == a11y.Collapse) {
			x = 0 // expand / collapse from assistive technology: the switcher
		}
		if hasKids && x < swEnd && query == "" {
			w.setExpanded(nd, !st.expanded[nd.Value])
		} else {
			w.act(nd)
		}
	}
	state := w.checkState(nd, set)
	if w.checkable {
		n.setState(a11y.Checkable).stateIf(state == tsOn, a11y.Checked).stateIf(state == tsHalf, a11y.Mixed)
	} else {
		n.setState(a11y.Selectable).stateIf(state == tsOn, a11y.Selected)
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		active := st.active == i
		x := r.X + padL + float32(row.depth)*w.indent()
		if shad {
			q := ""
			if row.match {
				q = query
			}
			shadPaintTreeItem(c, th, l, r, shadTreeLook{
				hiBg: tk.NodeHoverBg, hiCol: th.ItemSelected, muted: tk.SwitcherColor,
				label: nd.Label, query: q, x: x + w.switcherW(),
				highlight: active, disabled: nd.Disabled,
				chevron: hasKids, chevronAt: x + (w.switcherW()-shadTreeIcon)/2, expanded: expanded, loading: loading,
				checkbox: w.checkable, checked: state == tsOn, half: state == tsHalf,
				selected: state == tsOn && !w.checkable, padR: 8 + shadTreeIcon + 8,
			})
			return
		}
		// Parts are placed as in a left-to-right row and mirrored in RTL,
		// where the indentation grows from the right.
		rtl := c.IsRTL()
		mx := func(b geom.Rect) geom.Rect { return treeMirror(b, r, rtl) }
		if el && active && !nd.Disabled {
			l.Fill(r, geom.Radii{}, tk.NodeHoverBg)
		}
		sw := w.switcherW()
		if loading {
			sz := float32(12)
			p2bSpin(c, l, mx(geom.Rect{X: x + (sw-sz)/2, Y: r.Y + (r.H-sz)/2, W: sz, H: sz}), th.Primary.Base)
		} else if hasKids {
			is := float32(10)
			ang := 0.0
			if !expanded {
				ang = -3.14159265 / 2 // points along the reading direction
				if rtl {
					ang = -ang
				}
			}
			if el {
				is = 12
			}
			icon.Draw(l, icon.CaretDown, mx(geom.Rect{X: x + (sw-is)/2, Y: r.Y + (r.H-is)/2, W: is, H: is}), tk.SwitcherColor, ang)
		}
		x += sw
		if w.checkable {
			bs := c.boxSize()
			drawCheckbox(c, l, mx(geom.Rect{X: x, Y: r.Y + (r.H-bs)/2, W: bs, H: bs}), checkLook{checked: state == tsOn, indeterminate: state == tsHalf, disabled: nd.Disabled, hovered: ia.Hovered && !nd.Disabled, t: 1})
			x += bs + 8
			if !el {
				x -= 4
			}
		}
		tw := entTextW(c, nd.Label, th.FontSize, text.Regular)
		title := geom.Rect{X: x, Y: r.Y + (r.H-24)/2, W: tw + 8, H: 24}
		sel := state == tsOn && !w.checkable
		col := tk.TitleColor
		wt := text.Regular
		if el {
			col = th.TextSecondary
			if sel {
				col, wt = th.Primary.Base, text.Bold
			}
		} else {
			switch {
			case sel && !nd.Disabled:
				l.Fill(mx(title), geom.Rad(tk.NodeBorderRadius), tk.NodeSelectedBg)
				col = tk.NodeSelectedColor
			case active && !nd.Disabled:
				l.Fill(mx(title), geom.Rad(tk.NodeBorderRadius), tk.NodeHoverBg)
			}
		}
		if nd.Disabled {
			col = th.TextDisabled
		}
		tx := title.X + 4
		q := strings.TrimSpace(query)
		if row.match && q != "" {
			low := strings.ToLower(nd.Label)
			k := strings.Index(low, strings.ToLower(q))
			if k >= 0 && len(low) == len(nd.Label) {
				a, b := nd.Label[:k], nd.Label[k:k+len(q)]
				hc := th.Error.Base
				if el {
					hc = th.Primary.Base
				}
				if rtl {
					ts := c.style(th.FontSize, wt)
					treeMarkedText(l, geom.Rect{X: r.X, Y: title.Y, W: r.W, H: title.H}, tx, nd.Label, k, k+len(q), ts, ts, 0, treeMid, col, hc, nil)
					return
				}
				tx += entText(c, l, a, tx, title, th.FontSize, wt, col, 0)
				tx += entText(c, l, b, tx, title, th.FontSize, wt, hc, 0)
				entText(c, l, nd.Label[k+len(q):], tx, title, th.FontSize, wt, col, 0)
				return
			}
		}
		// Centered in the title box like entText; mirrored in the row.
		treeText(c, l, geom.Rect{X: r.X, Y: title.Y, W: r.W, H: title.H}, tx, nd.Label, c.style(th.FontSize, wt), r.Right()-tx-4, treeMid, col)
		if el && w.multiple() && !w.checkable && sel {
			ck := mx(geom.Rect{X: r.Right() - 20 - 12, W: 12})
			drawIcon(l, icon.Check, ck.X, r.Y+(r.H-12)/2, 12, th.Primary.Base)
		}
	}
}
