package ui

import (
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

// CascaderOption is a node of a Cascader tree. With LoadData, a node
// without Children that is not IsLeaf loads its children when expanded.
type CascaderOption struct {
	Label, Value string
	Disabled     bool
	Children     []CascaderOption
	IsLeaf       bool // never has children (relevant with LoadData)
}

type casState struct {
	pop     entPopup
	path    []string // expanded values, one per open column
	kcol    int      // keyboard column
	sactive int      // active row of the search result list
	lazy    *casLazy // LoadData bookkeeping (p2b_cascader.go)
}

// CascaderW selects a path in a tree through cascading menus.
type CascaderW struct {
	c              *Context
	th             *theme.Theme         // ThemeFor(CompCascader)
	tk             *theme.CascaderToken // &th.Components.Cascader
	box            *selector
	val            *[]string
	multi          *[][]string
	opts           []CascaderOption
	st             *casState
	changeOnSelect bool
	hoverExpand    bool
	forced         bool
	changed        bool
	chipPaths      [][]string
	z              *lazy
	hitIDs         []ID // search result options (ActiveDescendant)
	loadData       func(path []string, done func([]CascaderOption))
	strategy       CheckedStrategy
	stratVal       bool // the bound value follows the checked strategy

	// Set while building the columns (columnsView): the first column's
	// listbox and the option on the keyboard column's expanded path.
	listID, activeID ID
	// focusID takes the focus after a pointer pick: the trigger editor of
	// the popup Cascader, the panel of a CascaderPanel.
	focusID ID
	colH    float32 // column height (0: the token's DropdownHeight)
}

// Cascader adds a cascading select bound to *val (the selected value path).
func Cascader(c *Context, val *[]string, opts []CascaderOption) *CascaderW {
	w := &CascaderW{c: c, val: val, opts: opts, strategy: ShowParent}
	w.th = c.ThemeFor(theme.CompCascader)
	w.tk = &w.th.Components.Cascader
	w.box = newSelector(c, "cascader")
	w.st = State[casState](c, hashID(w.box.n.id, "cas"), nil)
	w.cascaderDefaults()
	w.z = c.lazy(w.box.n, w.finish)
	return w
}

func (w *CascaderW) Placeholder(s string) *CascaderW { w.box.placeholder = s; return w }

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used; the placeholder is the last resort).
func (w *CascaderW) A11yLabel(s string) *CascaderW        { w.box.n.setName(s); return w }
func (w *CascaderW) AllowClear() *CascaderW               { w.box.allowClear = true; return w }
func (w *CascaderW) Status(s string) *CascaderW           { w.box.status = s; return w }
func (w *CascaderW) Size(s ControlSize) *CascaderW        { w.box.size = s; return w }
func (w *CascaderW) Width(px float32) *CascaderW          { w.box.n.ln.Width = px; return w }
func (w *CascaderW) Layout(o ...layout.Option) *CascaderW { w.box.n.ln.Style.Apply(o...); return w }
func (w *CascaderW) MaxTagCount(n int) *CascaderW         { w.box.maxCount = n; return w }
func (w *CascaderW) Searchable() *CascaderW               { w.box.search = true; return w }
func (w *CascaderW) ChangeOnSelect() *CascaderW           { w.changeOnSelect = true; return w }
func (w *CascaderW) ExpandOnHover() *CascaderW            { w.hoverExpand = true; return w }
func (w *CascaderW) Multiple(vals *[][]string) *CascaderW { w.multi = vals; return w }
func (w *CascaderW) Disabled(on bool) *CascaderW {
	if on {
		w.box.disabled = true
	}
	return w
}

// Open forces the menus open (screenshots, tests).
func (w *CascaderW) Open() *CascaderW { w.st.pop.open, w.forced = true, true; return w }

// Expand sets the expanded path of an open menu (screenshots).
func (w *CascaderW) Expand(path ...string) *CascaderW {
	w.st.path = append([]string(nil), path...)
	w.st.kcol = max(0, len(path)-1)
	return w
}

// IsOpen reports whether the menus are shown.
func (w *CascaderW) IsOpen() bool { w.z.run(); return w.st.pop.open }

// Changed reports a new selection this frame.
func (w *CascaderW) Changed() bool { w.z.run(); return w.changed }

// casFind returns the option with value v in opts.
func casFind(opts []CascaderOption, v string) (CascaderOption, int) {
	for i, o := range opts {
		if o.Value == v {
			return o, i
		}
	}
	return CascaderOption{}, -1
}

// labelsOf maps a value path to its labels.
func (w *CascaderW) labelsOf(path []string) []string {
	var out []string
	opts := w.opts
	for k, v := range path {
		o, i := casFind(opts, v)
		if i < 0 {
			out = append(out, v)
			opts = nil
			continue
		}
		out = append(out, o.Label)
		opts = w.kids(o, path[:k+1])
	}
	return out
}

func casKey(path []string) string { return strings.Join(path, "\x00") }

// leaves appends the enabled leaf paths under o (prefix includes o). A
// lazy node whose children are not loaded yet stands for its subtree: it is
// checked as one unit and hands the check to its children once they load
// (pollLoads).
func (w *CascaderW) leaves(o CascaderOption, prefix []string, out [][]string) [][]string {
	if o.Disabled {
		return out
	}
	ks := w.kids(o, prefix)
	if len(ks) == 0 {
		return append(out, append([]string(nil), prefix...))
	}
	for _, ch := range ks {
		out = w.leaves(ch, append(prefix[:len(prefix):len(prefix)], ch.Value), out)
	}
	return out
}

// columns returns the option lists of the open menus.
func (w *CascaderW) columns() [][]CascaderOption {
	cols := [][]CascaderOption{w.opts}
	for i, v := range w.st.path {
		o, k := casFind(cols[i], v)
		if k < 0 {
			w.st.path = w.st.path[:i]
			break
		}
		ks := w.kids(o, w.st.path[:i+1])
		if len(ks) == 0 {
			w.st.path = w.st.path[:i+1]
			break
		}
		cols = append(cols, ks)
	}
	return cols
}

// checked returns the set of checked leaf paths (multiple mode).
func (w *CascaderW) checked() map[string]bool {
	m := map[string]bool{}
	if w.multi != nil {
		for _, p := range *w.multi {
			m[casKey(p)] = true
		}
	}
	return m
}

// checkState reports whether all / some enabled leaves under o are checked.
func (w *CascaderW) checkState(o CascaderOption, path []string, set map[string]bool) (all, some bool) {
	leaves := w.leaves(o, path, nil)
	if len(leaves) == 0 {
		return set[casKey(path)], set[casKey(path)]
	}
	n := 0
	for _, l := range leaves {
		if set[casKey(l)] {
			n++
		}
	}
	return n == len(leaves), n > 0
}

func (w *CascaderW) toggleNode(o CascaderOption, path []string) {
	set := w.checked()
	all, _ := w.checkState(o, path, set)
	leaves := w.leaves(o, path, nil)
	if all {
		for _, l := range leaves {
			delete(set, casKey(l))
		}
	} else {
		for _, l := range leaves {
			set[casKey(l)] = true
		}
	}
	// Keep a stable, tree order.
	var out [][]string
	for _, root := range w.opts {
		for _, l := range w.leaves(root, []string{root.Value}, nil) {
			if set[casKey(l)] {
				out = append(out, l)
			}
		}
	}
	*w.multi = out
	w.changed = true
	w.c.invalid = true
}

// chips computes the multiple-mode tags by the checked strategy (default
// ShowParent: fully checked subtrees collapse into their top node).
func (w *CascaderW) chips() (labels []string, paths [][]string) {
	if w.strategy != ShowParent {
		return w.strategyChips()
	}
	set := w.checked()
	el := w.th.Flavor == theme.Element
	var walk func(opts []CascaderOption, prefix []string)
	walk = func(opts []CascaderOption, prefix []string) {
		for _, o := range opts {
			p := append(append([]string(nil), prefix...), o.Value)
			all, some := w.checkState(o, p, set)
			switch {
			case all && some:
				lb := o.Label
				if el {
					lb = strings.Join(w.labelsOf(p), " / ")
				}
				labels = append(labels, lb)
				paths = append(paths, p)
			case some:
				walk(w.kids(o, p), p)
			}
		}
	}
	walk(w.opts, nil)
	return
}

type casHit struct {
	path   []string
	labels []string
	opt    CascaderOption
}

// searchResults flattens the tree into paths matching the query.
func (w *CascaderW) searchResults(q string) []casHit {
	q = strings.ToLower(q)
	var out []casHit
	var walk func(opts []CascaderOption, path, labels []string, dis bool)
	walk = func(opts []CascaderOption, path, labels []string, dis bool) {
		for _, o := range opts {
			p := append(append([]string(nil), path...), o.Value)
			lb := append(append([]string(nil), labels...), o.Label)
			d := dis || o.Disabled
			ks := w.kids(o, p)
			leaf := len(ks) == 0 && !w.pendingLazy(o, p) // an unloaded parent is not a leaf
			if (leaf || w.changeOnSelect) && strings.Contains(strings.ToLower(strings.Join(lb, " / ")), q) {
				oo := o
				oo.Disabled = d
				out = append(out, casHit{p, lb, oo})
			}
			walk(ks, p, lb, d)
		}
	}
	walk(w.opts, nil, nil, false)
	return out
}

func (w *CascaderW) selectPath(path []string, close bool) {
	if casKey(path) != casKey(*w.val) {
		*w.val = append([]string(nil), path...)
		w.changed = true
	}
	if close {
		w.st.pop.open = false
		if w.box != nil {
			w.box.st.query = ""
		}
	}
	w.c.invalid = true
}

func (w *CascaderW) openMenus() {
	w.st.pop.open = true
	w.st.sactive = 0
	if w.multi == nil {
		w.st.path = append([]string(nil), *w.val...)
	}
	w.st.kcol = max(0, len(w.st.path)-1)
}

func (w *CascaderW) finish() {
	var user *[][]string
	if w.stratVal && w.multi != nil {
		// Work on the checked leaf paths; write the strategy paths back.
		user = w.multi
		leaves := w.casExpandPaths(*user)
		w.multi = &leaves
	}
	w.build()
	if user != nil {
		if w.changed {
			_, paths := w.chips()
			*user = paths
			w.chipPaths = paths
		}
		w.multi = user
	}
}

func (w *CascaderW) build() {
	c, b, st := w.c, w.box, w.st
	w.pollLoads()
	var labels []string
	if w.multi != nil {
		labels, w.chipPaths = w.chips()
	} else {
		b.single = strings.Join(w.labelsOf(*w.val), " / ")
	}
	b.open = st.pop.open
	b.build(labels)
	if b.disabled {
		st.pop.open, b.open = false, false
		return
	}
	switch {
	case b.removed >= 0 && b.removed < len(w.chipPaths):
		p := w.chipPaths[b.removed]
		root, _ := casFind(w.opts, p[0])
		o := root
		for k, v := range p[1:] {
			o, _ = casFind(w.kids(o, p[:k+1]), v)
		}
		set := w.checked()
		if all, _ := w.checkState(o, p, set); all {
			w.toggleNode(o, p)
		}
	case b.clearClicked():
		if w.multi != nil {
			*w.multi = nil
		} else {
			*w.val = nil
		}
		w.changed = true
		st.pop.open = false
		b.st.query = ""
	case b.clicked():
		if !st.pop.open {
			w.openMenus()
		} else if !b.search {
			st.pop.open = false
		}
		c.SetFocus(b.ed.id)
		c.invalid = true
	}
	if !w.forced {
		st.pop.dismiss(c, b.n.id)
	}
	if !st.pop.open {
		b.st.query = ""
	}
	if st.pop.open && c.focus == 0 { // presses inside the panel keep focus
		c.SetFocus(b.ed.id)
	}
	searching := b.search && b.st.query != ""
	keys := b.takeKeys(func(k input.KeyEvent) bool {
		switch k.Key {
		case input.KeyUp, input.KeyDown, input.KeyEnter:
			return true
		case input.KeyLeft, input.KeyRight:
			return st.pop.open && !searching
		case input.KeySpace:
			return !b.search
		case input.KeyBackspace:
			return w.multi != nil && b.st.query == ""
		}
		return false
	})
	for _, k := range keys {
		if !st.pop.open && k.Key != input.KeyBackspace {
			w.openMenus()
			continue
		}
		if searching {
			hits := w.searchResults(b.st.query)
			switch k.Key {
			case input.KeyDown, input.KeyUp:
				if n := len(hits); n > 0 {
					d := 1
					if k.Key == input.KeyUp {
						d = n - 1
					}
					st.sactive = (st.sactive + d) % n
				}
			case input.KeyEnter:
				if st.sactive < len(hits) && !hits[st.sactive].opt.Disabled {
					w.pickHit(hits[st.sactive])
				}
			}
			continue
		}
		w.key(k.Key)
	}
	if b.edit().changed {
		st.pop.open = true
		st.sactive = 0
	}
	b.open = st.pop.open
	if st.pop.open {
		w.loadExpanded()
		w.dropdown()
	}
}

func (w *CascaderW) pickHit(h casHit) {
	if w.multi != nil {
		w.toggleNode(h.opt, h.path)
		w.box.st.query = ""
		return
	}
	w.selectPath(h.path, true)
}

// key handles keyboard navigation in the column menus. The horizontal
// arrows follow the reading direction: Right (Left in RTL) opens the next
// column, Left (Right in RTL) goes back.
func (w *CascaderW) key(k input.Key) {
	k = treeArrow(w.c, k)
	st := w.st
	cols := w.columns()
	col := min(st.kcol, len(cols)-1)
	list := cols[col]
	cur := -1
	if col < len(st.path) {
		_, cur = casFind(list, st.path[col])
	}
	setRow := func(i int) {
		st.path = append(append([]string(nil), st.path[:col]...), list[i].Value)
	}
	switch k {
	case input.KeyDown, input.KeyUp:
		opts := make([]Option, len(list))
		for i, o := range list {
			opts[i] = Option{Label: o.Label, Value: o.Value, Disabled: o.Disabled}
		}
		if cur < 0 && k == input.KeyUp {
			cur = 0
		}
		if cur < 0 {
			cur = -1
		}
		setRow(msStep(opts, cur, k == input.KeyDown))
	case input.KeyRight, input.KeyEnter, input.KeySpace:
		if cur < 0 {
			return
		}
		o := list[cur]
		path := append([]string(nil), st.path[:col+1]...)
		ks := w.kids(o, path)
		if w.pendingLazy(o, path) {
			w.load(path) // children arrive later; the column opens then
			if w.changeOnSelect && k == input.KeyEnter && w.multi == nil {
				w.selectPath(path, false)
			}
			return
		}
		if w.multi != nil && (k == input.KeySpace || (k == input.KeyEnter && len(ks) == 0)) {
			w.toggleNode(o, path)
			return
		}
		if len(ks) > 0 {
			if w.changeOnSelect && k == input.KeyEnter && w.multi == nil {
				w.selectPath(path, false)
			}
			for _, ch := range ks {
				if !ch.Disabled {
					st.path = append(path, ch.Value)
					st.kcol = col + 1
					break
				}
			}
			return
		}
		if k != input.KeyRight && w.multi == nil {
			w.selectPath(path, true)
		}
	case input.KeyLeft:
		if col > 0 {
			st.path = st.path[:col]
			st.kcol = col - 1
		}
	}
}

func (w *CascaderW) colWidth() float32 { return w.tk.ColumnWidth }

func (w *CascaderW) dropdown() {
	c, b, st := w.c, w.box, w.st
	if b.search && b.st.query != "" {
		hits := w.searchResults(b.st.query)
		ov := entPanel(c, b.n.id, &st.pop, true, func() {
			if len(hits) == 0 {
				Empty(c).Simple().Description(c.Locale().NoData)
				return
			}
			set := w.checked()
			msOptionList(c, len(hits), func(i int) (string, string, bool) {
				return casKey(hits[i].path), strings.Join(hits[i].labels, " / "), hits[i].opt.Disabled
			}, &st.sactive, func(i int) bool {
				if w.multi != nil {
					all, _ := w.checkState(hits[i].opt, hits[i].path, set)
					return all
				}
				return casKey(hits[i].path) == casKey(*w.val)
			}, &w.hitIDs, func(i int, l *paint.List, r geom.Rect) {
				h := hits[i]
				sel := casKey(h.path) == casKey(*w.val)
				if w.multi != nil {
					sel, _ = w.checkState(h.opt, h.path, set)
				}
				entPaintOption(c, l, r, "", entOptState{selected: sel, active: st.sactive == i, disabled: h.opt.Disabled, check: w.multi != nil})
				acPaintMatch(c, l, r, strings.Join(h.labels, " / "), b.st.query, h.opt.Disabled)
			}, func(i int) { w.pickHit(hits[i]); c.SetFocus(b.ed.id) })
		})
		ov.Style(layout.Padding(entListPad(c)))
		if len(hits) > 0 {
			ov.n.setRole(a11y.RoleListbox).setName(b.popupName()).stateIf(w.multi != nil, a11y.Multiselectable)
			b.listID, b.activeID = ov.n.id, 0
			if st.sactive >= 0 && st.sactive < len(w.hitIDs) {
				b.activeID = w.hitIDs[st.sactive]
			}
		}
		return
	}
	if b.ed != nil {
		w.focusID = b.ed.id
	}
	entPanel(c, b.n.id, &st.pop, false, func() { w.columnsView(b.popupName()) })
	b.listID, b.activeID = w.listID, w.activeID
}

// columnsView builds the column menus of the expanded path (the popup of
// Cascader, the body of CascaderPanel); name names the first column.
func (w *CascaderW) columnsView(name0 string) {
	c, st := w.c, w.st
	th, tk := w.th, w.tk
	el := th.Flavor == theme.Element
	cols := w.columns()
	set := w.checked()
	colH := tk.DropdownHeight
	if w.colH > 0 {
		colH = w.colH
	}
	w.listID, w.activeID = 0, 0
	Row(c, func() {
		for ci, list := range cols {
			ci, list := ci, list
			prefix := append([]string(nil), st.path[:min(ci, len(st.path))]...)
			pad := geom.All(4)
			if el {
				pad = geom.VH(6, 0)
			}
			act := func() int {
				if ci < len(st.path) {
					_, i := casFind(list, st.path[ci])
					return i
				}
				return -1
			}
			idx := act()
			sb := Scroll(c, func() {
				for _, o := range list {
					o := o
					Key(c, o.Value, func() { w.item(ci, o, prefix, set) })
				}
			}, layout.H(colH), layout.MinW(w.colWidth()), layout.Padding(pad))
			// Each column is a listbox named after the option it expands.
			name := name0
			if ci > 0 {
				if ls := w.labelsOf(prefix); len(ls) > 0 {
					name = ls[len(ls)-1]
				}
			}
			sb.n.setRole(a11y.RoleListbox).setName(name).stateIf(w.multi != nil, a11y.Multiselectable)
			if ci == 0 {
				w.listID = sb.n.id
			}
			followRow(c, sb, idx, act(), pad.Top, tk.OptionHeight)
			if ci < len(cols)-1 {
				sb.Paint(func(l *paint.List, r geom.Rect) {
					// Separator on the column's end edge (the columns
					// run right to left in RTL).
					l.Fill(treeMirror(geom.Rect{X: r.Right() - 1, Y: r.Y, W: 1, H: r.H}, r, c.IsRTL()), geom.Radii{}, tk.ColumnDividerColor)
				})
			}
		}
	})
}

// item builds one menu row.
func (w *CascaderW) item(ci int, o CascaderOption, prefix []string, set map[string]bool) {
	c, st := w.c, w.st
	th, tk := w.th, w.tk
	el := th.Flavor == theme.Element
	path := append(append([]string(nil), prefix...), o.Value)
	it := c.add("cas-option", o.Label)
	it.ln.Height = tk.OptionHeight
	multi := w.multi != nil
	bs := c.boxSize()
	shad := shadTree(th)
	padL, padR := tk.OptionPadding, float32(12)
	if el {
		padR = 30
	} else if shad {
		padR = 8 + shadTreeIcon + 8
	}
	if multi {
		padL += bs + 8
	}
	lazyKids := w.pendingLazy(o, path)
	loading := lazyKids && w.isLoading(path)
	hasKids := len(w.kids(o, path)) > 0 || lazyKids
	it.ln.Measure = func(_, _ float32) geom.Size {
		w := entTextW(c, o.Label, th.FontSize, text.Semibold) + padL + padR
		if hasKids && !el && !shad {
			w += 18
		}
		return geom.Size{W: w, H: tk.OptionHeight}
	}
	cur := input.CursorPointer
	if o.Disabled {
		it.disabled = true
		cur = input.CursorNotAllowed
	}
	ia := c.interact(it, false, cur)
	expanded := ci < len(st.path) && st.path[ci] == o.Value
	it.aKbd = true // arrow keys of the trigger move through the columns
	it.setRole(a11y.RoleOption).setState(a11y.Selectable).stateIf(hasKids, a11y.Expandable).stateIf(hasKids && expanded && !lazyKids, a11y.Expanded).stateIf(loading, a11y.Busy)
	if expanded && ci == min(st.kcol, len(st.path)-1) {
		w.activeID = it.id
	}
	if !o.Disabled {
		expand := func() {
			st.path = path
			st.kcol = ci
			if lazyKids {
				w.load(path)
			}
			c.invalid = true
		}
		if ia.Hovered && w.hoverExpand && hasKids && !expanded {
			expand()
		}
		if ia.Clicked {
			r := c.rects[it.id]
			onBox := c.inline(r).Off(c.pointer.X) < padL
			switch {
			case multi && (!hasKids || onBox) && !loading:
				w.toggleNode(o, path)
				if hasKids {
					expand()
				}
			case hasKids:
				expand()
				if w.changeOnSelect && !multi {
					w.selectPath(path, false)
				}
			default:
				expand()
				w.selectPath(path, true)
			}
			if w.focusID != 0 {
				c.SetFocus(w.focusID)
			}
		}
	}
	selected := !multi && casKey(path) == casKey((*w.val)[:min(len(path), len(*w.val))]) && len(*w.val) > 0
	var all, some bool
	if multi {
		all, some = w.checkState(o, path, set)
		it.setState(a11y.Checkable).stateIf(all, a11y.Checked).stateIf(some && !all, a11y.Mixed)
	}
	it.stateIf(selected && (!hasKids || w.changeOnSelect), a11y.Selected)
	it.paint = func(l *paint.List, r geom.Rect) {
		if shad {
			x := r.X + tk.OptionPadding
			hi := tk.OptionHoverBg
			if expanded || (selected && !hasKids) {
				hi = tk.OptionSelectedBg
			}
			shadPaintTreeItem(c, th, l, r, shadTreeLook{
				hiBg: hi, hiCol: tk.OptionSelectedColor, muted: tk.ExpandIconColor,
				label: o.Label, x: x,
				highlight: expanded || (selected && !hasKids) || ia.Hovered, disabled: o.Disabled,
				chevron: hasKids, chevronAt: r.Right() - 8 - shadTreeIcon, chevronR: true, expanded: false, loading: loading,
				checkbox: multi, checked: all, half: some && !all,
				selected: selected && (!hasKids || w.changeOnSelect), padR: padR,
			})
			return
		}
		rad := geom.Rad(th.RadiusSM)
		if el {
			rad = geom.Radii{}
		}
		col, wt := tk.OptionColor, text.Regular
		active := expanded || (selected && !hasKids)
		switch {
		case o.Disabled:
			col = th.TextDisabled
		case el:
			// The active path is marked by its color; the fill follows
			// the pointer only.
			if ia.Hovered {
				l.Fill(r, rad, tk.OptionHoverBg)
			}
			if active || (multi && some) || selected {
				col, wt = tk.OptionSelectedColor, themeWeight(tk.OptionSelectedFontWeight)
			}
		default:
			if active {
				l.Fill(r, rad, tk.OptionSelectedBg)
				col, wt = tk.OptionSelectedColor, themeWeight(tk.OptionSelectedFontWeight)
			} else if ia.Hovered {
				l.Fill(r, rad, tk.OptionHoverBg)
			}
		}
		// Parts are placed as in a left-to-right row and mirrored in RTL.
		rtl := c.IsRTL()
		x := r.X + tk.OptionPadding
		if multi {
			drawCheckbox(c, l, treeMirror(geom.Rect{X: x, Y: r.Y + (r.H-bs)/2, W: bs, H: bs}, r, rtl), checkLook{checked: all, indeterminate: some, disabled: o.Disabled, t: 1})
			x += bs + 8
		} else if el && selected && !hasKids {
			ck := treeMirror(geom.Rect{X: r.X + 4, W: 12}, r, rtl)
			drawIcon(l, icon.Check, ck.X, r.Y+(r.H-12)/2, 12, tk.OptionSelectedColor)
		}
		treeText(c, l, r, x, o.Label, c.style(th.FontSize, wt), r.Right()-padR-x, treeMid, col)
		if hasKids {
			is := float32(10)
			ac := tk.ExpandIconColor
			if el {
				is = 14
			}
			if o.Disabled {
				ac = th.TextDisabled
			}
			rx := r.Right() - 12 - is
			if el {
				rx = r.Right() - 20 - is + 6
			}
			if loading {
				sz := float32(14)
				p2bSpin(c, l, treeMirror(geom.Rect{X: rx + is - sz, Y: r.Y + (r.H-sz)/2, W: sz, H: sz}, r, rtl), th.Primary.Base)
				return
			}
			// Points toward the next column: left in RTL.
			ar := treeMirror(geom.Rect{X: rx, W: is}, r, rtl)
			c.drawIconAt(l, icon.Right, ar.X, r.Y+(r.H-is)/2, is, ac)
		}
	}
}
