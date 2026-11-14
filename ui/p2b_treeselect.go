package ui

// TreeSelect: lazy loading (LoadData + TreeSelectNode.IsLeaf, the same
// polling contract as Tree.LoadData), a virtual dropdown for long trees
// (only the visible rows are built, as in VirtualTree) and the checked
// strategy of checkable mode.

import (
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/layout"
)

// LoadData enables lazy loading with Tree's contract: expanding a node
// that has no Children and is not IsLeaf calls fn(value) every frame until
// it returns false (true = still loading); the application appends the
// children to its node slice meanwhile. A spinner replaces the switcher.
func (w *TreeSelectW) LoadData(fn func(value string) bool) *TreeSelectW { w.loadData = fn; return w }

// ShowCheckedStrategy chooses the tags of Checkable mode (default
// ShowChild: checked leaves; ShowParent: a fully checked parent replaces
// its children; ShowAll: every checked node). By default the bound value
// holds the checked leaves and Checked returns the strategy's values; with
// StrategyValue(true) the bound value itself follows the strategy (Ant).
func (w *TreeSelectW) ShowCheckedStrategy(s CheckedStrategy) *TreeSelectW { w.strategy = s; return w }

// StrategyValue makes the bound value of Checkable mode follow the checked
// strategy, as Ant's value does: with ShowParent a fully checked parent is
// stored instead of its leaves, with ShowAll parents and leaves are both
// stored. Values written by the application are read the same way (a
// parent value checks its whole subtree). Default false: the value holds
// the checked leaves whatever the strategy (lossless with lazy loading and
// disabled nodes, and what earlier versions stored).
func (w *TreeSelectW) StrategyValue(on bool) *TreeSelectW { w.stratVal = on; return w }

// ExpandedKeys binds the expanded nodes of the dropdown to *keys
// (controlled treeExpandedKeys): the tree shows exactly these nodes open,
// and expanding or collapsing (pointer, keyboard, opening on the current
// value, search) writes the new list back in tree order. DefaultExpandAll
// only seeds an empty list at the first frame.
func (w *TreeSelectW) ExpandedKeys(keys *[]string) *TreeSelectW { w.expKeys = keys; return w }

// ExpandChanged reports that the bound ExpandedKeys changed this frame.
func (w *TreeSelectW) ExpandChanged() bool { w.z.run(); return w.expChange }

// expandIn replaces the retained expansion with the bound keys.
func (w *TreeSelectW) expandIn() {
	m := make(map[string]bool, len(*w.expKeys))
	for _, k := range *w.expKeys {
		m[k] = true
	}
	w.st.expanded = m
}

// expandOut writes the expansion back to the bound keys: expanded nodes in
// tree order, then keys of nodes not in the tree (yet) in their old order.
func (w *TreeSelectW) expandOut() {
	var out []string
	seen := map[string]bool{}
	var walk func(ns []TreeSelectNode)
	walk = func(ns []TreeSelectNode) {
		for i := range ns {
			if w.st.expanded[ns[i].Value] {
				out = append(out, ns[i].Value)
				seen[ns[i].Value] = true
			}
			walk(ns[i].Children)
		}
	}
	walk(w.nodes)
	for _, k := range *w.expKeys {
		if !seen[k] && w.st.expanded[k] && tsFind(w.nodes, k) == nil {
			out = append(out, k)
			seen[k] = true
		}
	}
	old := *w.expKeys
	same := len(old) == len(out)
	for i := 0; same && i < len(out); i++ {
		same = old[i] == out[i]
	}
	if !same {
		*w.expKeys = out
		w.expChange = true
	}
}

// Virtual forces the virtual dropdown on or off (default: more than 100
// visible rows).
func (w *TreeSelectW) Virtual(on bool) *TreeSelectW {
	w.virtual = -1
	if on {
		w.virtual = 1
	}
	return w
}

// ListHeight sets the maximum dropdown height (Ant listHeight, 256).
func (w *TreeSelectW) ListHeight(px float32) *TreeSelectW { w.listH = px; return w }

// Checked returns the selected values; in Checkable mode they follow the
// checked strategy (the values shown as tags).
func (w *TreeSelectW) Checked() []string {
	w.z.run()
	if w.checkable {
		return append([]string(nil), w.chipVals...)
	}
	if w.multiple() {
		return append([]string(nil), *w.vals...)
	}
	if *w.val == "" {
		return nil
	}
	return []string{*w.val}
}

// treeSelectDefaults applies ConfigProvider.Defaults("TreeSelect", ...):
// size, status, allowClear, showSearch, placeholder, treeDefaultExpandAll,
// maxTagCount, virtual, listHeight, showCheckedStrategy, strategyValue.
func (w *TreeSelectW) treeSelectDefaults() {
	c, b := w.c, w.box
	shad2aTreeSize(c, "TreeSelect", &b.size)
	if v, ok := p2bStr(c, "TreeSelect", "status"); ok && b.status == "" {
		b.status = v
	}
	if v, ok := p2bBool(c, "TreeSelect", "strategyValue"); ok {
		w.stratVal = v
	}
	if v, ok := p2bBool(c, "TreeSelect", "allowClear"); ok {
		b.allowClear = v
	}
	if v, ok := p2bBool(c, "TreeSelect", "showSearch"); ok {
		b.search = v
	}
	if v, ok := p2bStr(c, "TreeSelect", "placeholder"); ok {
		b.placeholder = v
	}
	if v, ok := p2bBool(c, "TreeSelect", "treeDefaultExpandAll"); ok {
		w.expandAll = v
	}
	if v, ok := p2bNum(c, "TreeSelect", "maxTagCount"); ok {
		b.maxCount = int(v)
	}
	if v, ok := p2bBool(c, "TreeSelect", "virtual"); ok {
		w.virtual = -1
		if v {
			w.virtual = 0 // automatic (long lists)
		}
	}
	if v, ok := p2bNum(c, "TreeSelect", "listHeight"); ok {
		w.listH = float32(v)
	}
	w.strategy = p2bStrategy(c, "TreeSelect", w.strategy)
}

// pendingLazy reports a node whose children are still to be loaded.
func (w *TreeSelectW) pendingLazy(nd *TreeSelectNode) bool {
	return w.loadData != nil && !nd.IsLeaf && len(nd.Children) == 0 && !w.st.loaded[nd.Value]
}

// expandable reports whether nd shows a switcher.
func (w *TreeSelectW) expandable(nd *TreeSelectNode) bool {
	return len(nd.Children) > 0 || w.pendingLazy(nd)
}

// isLoading reports a node whose children are being loaded.
func (w *TreeSelectW) isLoading(v string) bool { return w.st.loading[v] }

// setExpanded opens or closes nd, starting a load when needed.
func (w *TreeSelectW) setExpanded(nd *TreeSelectNode, on bool) {
	w.st.expanded[nd.Value] = on
	if on && w.pendingLazy(nd) {
		if w.st.loading == nil {
			w.st.loading = map[string]bool{}
		}
		w.st.loading[nd.Value] = true
	}
	w.c.invalid = true
}

// pollLoads runs LoadData for loading nodes until it reports done.
func (w *TreeSelectW) pollLoads() {
	st := w.st
	if w.loadData == nil || len(st.loading) == 0 {
		return
	}
	if st.loaded == nil {
		st.loaded = map[string]bool{}
	}
	for v := range st.loading {
		nd := tsFind(w.nodes, v)
		if nd == nil || len(nd.Children) > 0 || !w.loadData(v) {
			delete(st.loading, v)
			st.loaded[v] = true
			w.c.invalid = true
			if nd != nil {
				w.inheritCheck(nd)
			}
		}
	}
	if len(st.loading) > 0 {
		w.c.WakeAt(w.c.now.Add(50 * time.Millisecond)) // keep polling
	}
}

// inheritCheck hands the check of a lazy node that was checked before its
// children loaded down to the new children (the node was never a leaf).
func (w *TreeSelectW) inheritCheck(nd *TreeSelectNode) {
	if !w.checkable || w.strictly || len(nd.Children) == 0 || !contains(*w.vals, nd.Value) {
		return
	}
	vals := remove(*w.vals, nd.Value)
	set := map[string]bool{}
	for _, v := range vals {
		set[v] = true
	}
	for _, l := range tsLeaves(nd, true, nil) {
		if !set[l] {
			vals = append(vals, l)
		}
	}
	*w.vals = vals
	w.changed = true
}

// chips returns the tag labels and node values of a checkable selection
// by the checked strategy.
func (w *TreeSelectW) chips() (labels, vals []string) {
	set := w.valueSet()
	if w.strategy == ShowChild || w.strictly {
		for _, v := range *w.vals {
			if nd := tsFind(w.nodes, v); nd != nil {
				labels = append(labels, nd.Label)
			} else {
				labels = append(labels, v)
			}
			vals = append(vals, v)
		}
		return
	}
	var walk func(ns []TreeSelectNode)
	walk = func(ns []TreeSelectNode) {
		for i := range ns {
			nd := &ns[i]
			s := w.checkState(nd, set)
			if s == tsOff {
				continue
			}
			if s == tsOn {
				labels = append(labels, nd.Label)
				vals = append(vals, nd.Value)
				if w.strategy == ShowParent {
					continue
				}
			}
			walk(nd.Children)
		}
	}
	walk(w.nodes)
	return
}

// removeChip unchecks the subtree of tag value v.
func (w *TreeSelectW) removeChip(v string) {
	nd := tsFind(w.nodes, v)
	if nd == nil || w.strictly {
		*w.vals = remove(*w.vals, v)
		w.changed = true
		return
	}
	for _, l := range tsLeaves(nd, false, nil) {
		*w.vals = remove(*w.vals, l)
	}
	w.changed = true
}

func (w *TreeSelectW) listHeight() float32 {
	if w.listH > 0 {
		return w.listH
	}
	return 256
}

// virtualOn reports whether the dropdown builds only its visible rows.
func (w *TreeSelectW) virtualOn(n int) bool {
	switch {
	case w.virtual > 0:
		return true
	case w.virtual < 0:
		return false
	}
	return n > p2bVirtualMin
}

// virtualList builds the visible window of rows into a fixed-height
// viewport (VirtualTree's approach: extent = rows × row height, rows
// placed from base offset).
func (w *TreeSelectW) virtualList(rows []tsRow, set map[string]bool) {
	c, st := w.c, w.st
	rh := w.rowH()
	total := float64(len(rows)) * float64(rh)
	h := min(float64(w.listHeight()), total)
	sb := fillViewport(c, "ts-vlist", []layout.Option{layout.H(float32(h))})
	sb.n.setRole(a11y.RoleTree).setName(w.box.popupName()).stateIf(w.multiple(), a11y.Multiselectable)
	w.box.listID = sb.n.id
	sb.s.extent.Y = total
	view := fillViewH(sb, float32(h))
	fillClampOff(sb, total, view)
	if (!st.listShown || st.active != st.shown || st.shownH != rh) && st.active >= 0 && st.active < len(rows) {
		fillReveal(c, sb, float64(st.active)*float64(rh), float64(rh), view)
	}
	moved := c.pointer != st.ptr
	first, end := fillSpan(sb.s.off.Y, view, float64(rh), len(rows), 3)
	sb.s.base.Y = float64(first) * float64(rh)
	keep := st.active
	sb.Body(func() {
		for i := first; i < end; i++ {
			i, row := i, rows[i]
			Key(c, row.nd.Value, func() { w.row(i, row, set) })
		}
	})
	if !moved {
		st.active = keep // rows scrolling under a still pointer keep the highlight
	}
	st.shown, st.listShown, st.shownH = st.active, true, rh
}
