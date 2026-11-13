package ui

// Transfer: custom row rendering, pagination, one-way mode, a tree as the
// source list (Ant tree transfer) and virtual lists for long data.

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Render builds each row's content with fn instead of the title (the row
// background and checkbox stay). The option carries the item's Title as
// Label, Key as Value and Disabled.
//
//	ui.Transfer(c, items, &keys).Render(func(o ui.Option) {
//		ui.Row(c, func() { ui.Icon(c, icon.User); ui.Text(c, o.Label) }, layout.Gap(6))
//	})
func (w *TransferW) Render(fn func(o Option)) *TransferW { w.render = fn; return w }

// Pagination shows pageSize items per page with a pager under each list
// (Ant pagination).
func (w *TransferW) Pagination(pageSize int) *TransferW { w.pageSize = pageSize; return w }

// OneWay makes the transfer one-directional (Ant oneWay): only the move
// to the right remains, and right-hand items have a remove button.
func (w *TransferW) OneWay() *TransferW { w.oneWay = true; return w }

// TreeSource shows nodes as a checkable tree on the left (Ant tree
// transfer). Leaves are the transferable items (their Key / Title); leaves
// already on the right are shown checked and disabled. The items passed
// to Transfer are ignored.
func (w *TransferW) TreeSource(nodes []TreeNode) *TransferW { w.tree = nodes; return w }

// Virtual forces virtual lists on or off (default: more than 100 items
// on a page).
func (w *TransferW) Virtual(on bool) *TransferW {
	w.virtual = -1
	if on {
		w.virtual = 1
	}
	return w
}

// transferDefaults applies ConfigProvider.Defaults("Transfer", ...):
// showSearch, oneWay, pageSize, listWidth, listHeight, virtual, disabled,
// status ("error" | "warning"), titles ([2]string or []string).
func (w *TransferW) transferDefaults() {
	c := w.c
	if v, ok := p2bBool(c, "Transfer", "showSearch"); ok {
		w.search = v
	}
	if v, ok := p2bBool(c, "Transfer", "oneWay"); ok {
		w.oneWay = v
	}
	if v, ok := p2bNum(c, "Transfer", "pageSize"); ok {
		w.pageSize = int(v)
	}
	if v, ok := p2bNum(c, "Transfer", "listWidth"); ok {
		w.listW = float32(v)
	}
	if v, ok := p2bNum(c, "Transfer", "listHeight"); ok {
		w.listH = float32(v)
	}
	if v, ok := p2bBool(c, "Transfer", "virtual"); ok && !v {
		w.virtual = -1
	}
	if v, ok := p2bBool(c, "Transfer", "disabled"); ok && v {
		w.disabled = true
	}
	if v, ok := p2bStr(c, "Transfer", "status"); ok && w.status == "" {
		w.status = v
	}
	if v, ok := c.ConfigProp("Transfer", "titles"); ok {
		switch t := v.(type) {
		case [2]string:
			w.titles = t
		case []string:
			if len(t) == 2 {
				w.titles = [2]string{t[0], t[1]}
			}
		}
	}
}

func (w *TransferW) virtualOn(n int) bool {
	switch {
	case w.virtual > 0:
		return true
	case w.virtual < 0:
		return false
	}
	return n > p2bVirtualMin
}

func (w *TransferW) rowH() float32 { return w.tk.ItemHeight }

// ---------------------------------------------------------------------------
// Pagination

// pages returns the page count of a side.
func (w *TransferW) pages(side int) int {
	if w.pageSize <= 0 {
		return 1
	}
	n := len(w.list(side, true))
	return max(1, (n+w.pageSize-1)/w.pageSize)
}

// pageOf returns the items of the current page of a side.
func (w *TransferW) pageOf(side int, shown []TransferItem) []TransferItem {
	if w.pageSize <= 0 {
		return shown
	}
	st := w.st
	pages := max(1, (len(shown)+w.pageSize-1)/w.pageSize)
	st.page[side] = max(1, min(st.page[side], pages))
	a := (st.page[side] - 1) * w.pageSize
	return shown[a:min(len(shown), a+w.pageSize)]
}

// pager builds the pager under a list: a small simple Pagination
// ("‹ [1] / 3 ›", the current page editable), right-aligned like Ant's.
func (w *TransferW) pager(side int, dis bool) {
	if w.pageSize <= 0 {
		return
	}
	c, tk := w.c, w.tk
	st := w.st
	total := len(w.list(side, true))
	st.page[side] = max(1, min(st.page[side], w.pages(side)))
	bar := Row(c, func() {
		Spacer(c)
		Pagination(c, &st.page[side], total, w.pageSize).Simple().Small().Disabled(dis).Show()
	}, layout.H(40), layout.PadVH(0, 8), layout.Gap(8), layout.Middle())
	bar.Paint(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, tk.DividerColor)
	})
}

// ---------------------------------------------------------------------------
// Rows

// renderItem builds the custom content of row n.
func (w *TransferW) renderItem(n *node, it TransferItem) {
	c := w.c
	n.ln.Dir = layout.Row
	n.ln.Align = layout.AlignCenter
	pad := float32(12)
	if w.th.Flavor == theme.Element {
		pad = 15
	}
	n.ln.Padding = geom.Insets{Left: pad + c.boxSize() + 8, Right: 12}
	n.ln.Measure = nil
	o := Option{Label: it.Title, Value: it.Key, Disabled: it.Disabled}
	c.body(n, func() { w.render(o) })
}

// oneWayItem finishes a right-hand row of a one-way transfer: no checkbox,
// a remove button at the end.
func (w *TransferW) oneWayItem(n *node, it TransferItem, ia Interaction, off bool, h float32) *node {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	n.ln.Dir = layout.Row
	n.ln.Align = layout.AlignCenter
	n.ln.Padding = geom.Insets{Left: 12, Right: 8}
	if el {
		n.ln.Padding.Left = 15
	}
	n.ln.Measure = nil
	n.setRole(a11y.RoleOption).setState(a11y.Selectable)
	c.body(n, func() {
		if w.render != nil {
			w.render(Option{Label: it.Title, Value: it.Key, Disabled: it.Disabled})
		} else {
			t := Text(c, it.Title).NoWrap()
			if off {
				t.Color(th.TextDisabled)
			}
			t.n.aHidden = true
		}
		Spacer(c)
		if !off {
			b := IconButton(c, icon.Delete, c.Locale().RemoveItem(it.Title))
			if b.Clicked() {
				*w.target = remove(*w.target, it.Key)
				w.changed = true
				c.invalid = true
			}
		}
	})
	n.paint = func(l *paint.List, r geom.Rect) {
		if ia.Hovered && !off && tk.ItemHoverBg.A > 0 {
			l.Fill(r, geom.Radii{}, tk.ItemHoverBg)
		}
		if c.focus == n.id && c.focusVisible {
			entFocusRing(c, l, r.Inset(geom.All(3)), th.RadiusSM)
		}
	}
	// Delete / Backspace on the focused row removes it.
	for _, k := range c.KeysFor(n.id) {
		if k.Down && (k.Key == input.KeyDelete || k.Key == input.KeyBackspace) && !off {
			*w.target = remove(*w.target, it.Key)
			w.changed = true
			c.invalid = true
		}
	}
	_ = h
	return n
}

// virtualBody builds the visible window of a long list into a viewport;
// the keyboard moves a focus index over all items (rows outside the
// window are built when reached).
func (w *TransferW) virtualBody(side int, shown []TransferItem, dis bool) {
	c, st := w.c, w.st
	rh := w.rowH()
	total := float64(len(shown)) * float64(rh)
	sb := fillViewport(c, "tf-vlist", []layout.Option{layout.Grow(1), layout.Padding(geom.Insets{Top: 4, Bottom: 4})})
	sb.n.setRole(a11y.RoleListbox).setName(w.titles[side]).setState(a11y.Multiselectable)
	sb.s.extent.Y = total + 8
	view := fillViewH(sb, 200)
	fillClampOff(sb, total+8, view)
	// The keyboard item and its index.
	cur := -1
	for i, it := range shown {
		if st.focusKey[side] == it.Key {
			cur = i
			break
		}
	}
	first, end := fillSpan(max(0, sb.s.off.Y-4), view, float64(rh), len(shown), 3)
	sb.s.base.Y = 4 + float64(first)*float64(rh)
	tab := cur
	if tab < first || tab >= end {
		tab = -1
	}
	st.focusID[side] = 0
	sb.Body(func() {
		for i := first; i < end; i++ {
			i, it := i, shown[i]
			Key(c, it.Key, func() {
				n := w.item(side, it, dis)
				n.aKbd = true // arrow keys reach every item
				if c.a11yOn {
					pos, size := i+1, len(shown)
					n.fill(semFunc(func(s *a11y.Node) { s.PosInSet, s.SetSize = pos, size }))
				}
				if c.focus == n.id {
					st.focusKey[side] = it.Key // focused with the pointer
					tab = i
				}
				// One Tab stop: the keyboard item, else the first row.
				if (tab >= 0 && i != tab) || (tab < 0 && i != first) {
					n.noTab = true
				}
				if st.focusKey[side] == it.Key {
					st.focusID[side] = n.id
					if st.focusPend[side] {
						st.focusPend[side] = false
						navFocus(c, n.id)
					}
				}
			})
		}
	})
	// Arrow keys of the focused row (after the rows read Space / Enter)
	// move the keyboard item; rows outside the window are built and
	// focused in the next pass, after the list follows.
	cur = -1
	for i := first; i < end; i++ {
		if shown[i].Key == st.focusKey[side] {
			cur = i
		}
	}
	if cur >= 0 && st.focusID[side] != 0 && c.focus == st.focusID[side] {
		to := cur
		page := max(1, int(view/float64(rh))-1)
		for _, k := range c.KeysFor(st.focusID[side]) {
			if !k.Down {
				continue
			}
			switch k.Key {
			case input.KeyDown:
				to = tfStep(shown, to, 1)
			case input.KeyUp:
				to = tfStep(shown, to, -1)
			case input.KeyHome:
				to = tfStep(shown, -1, 1)
			case input.KeyEnd:
				to = tfStep(shown, len(shown), -1)
			case input.KeyPageDown:
				to = tfStep(shown, min(len(shown), to+page)-1, 1)
			case input.KeyPageUp:
				to = tfStep(shown, max(-1, to-page)+1, -1)
			}
		}
		if to != cur {
			st.focusKey[side], st.focusPend[side] = shown[to].Key, true
			fillReveal(c, sb, 4+float64(to)*float64(rh), float64(rh), view)
			c.invalid = true
		}
	}
}

// tfStep moves index i by dir to the next enabled item (clamped).
func tfStep(items []TransferItem, i, dir int) int {
	for j := i + dir; j >= 0 && j < len(items); j += dir {
		if !items[j].Disabled {
			return j
		}
	}
	return max(0, min(i, len(items)-1))
}

// ---------------------------------------------------------------------------
// Tree source

// syncTreeItems derives the items from the tree leaves.
func (w *TransferW) syncTreeItems() {
	if w.tree == nil {
		return
	}
	var items []TransferItem
	treeWalk(w.tree, func(n *TreeNode, _ []string) {
		if len(n.Children) == 0 {
			items = append(items, TransferItem{Key: n.Key, Title: n.Title, Disabled: n.Disabled})
		}
	})
	w.items = items
	if !w.st.treeInit {
		w.st.treeInit = true
		treeWalk(w.tree, func(n *TreeNode, _ []string) {
			if len(n.Children) > 0 {
				w.st.treeExp = append(w.st.treeExp, n.Key)
			}
		})
	}
}

// tfMarkTree copies nodes marking the leaves in target as checked-only.
func tfMarkTree(nodes []TreeNode, target map[string]bool) []TreeNode {
	out := make([]TreeNode, len(nodes))
	for i, n := range nodes {
		out[i] = n
		if len(n.Children) > 0 {
			out[i].Children = tfMarkTree(n.Children, target)
		} else if target[n.Key] {
			out[i].DisableCheckbox = true
		}
	}
	return out
}

// treeBody builds the left list as a checkable tree.
func (w *TransferW) treeBody(dis bool) {
	c, st := w.c, w.st
	target := map[string]bool{}
	for _, k := range *w.target {
		target[k] = true
	}
	nodes := tfMarkTree(w.tree, target)
	keys := st.treeKeys[:0]
	for _, k := range *w.target {
		keys = append(keys, k)
	}
	for _, it := range w.items {
		if st.checked[0][it.Key] {
			keys = append(keys, it.Key)
		}
	}
	st.treeKeys = keys
	var sel []string
	Scroll(c, func() {
		Div(c, layout.Padding(geom.VH(4, 8))).Body(func() {
			t := Tree(c, nodes).Checkable(&st.treeKeys).Expanded(&st.treeExp).Selected(&sel).BlockNode().A11yLabel(w.titles[0])
			if q := st.query[0]; q != "" {
				t.Search(q).Filter()
			}
			t.Show()
			if dis {
				return
			}
			if t.CheckChanged() {
				on := map[string]bool{}
				for _, k := range st.treeKeys {
					on[k] = true
				}
				for _, it := range w.items {
					if target[it.Key] || it.Disabled {
						continue
					}
					if on[it.Key] {
						st.checked[0][it.Key] = true
					} else {
						delete(st.checked[0], it.Key)
					}
				}
				c.invalid = true
			}
			// Clicking a leaf title toggles its check (Ant's tree transfer).
			if t.SelectChanged() {
				for _, k := range sel {
					for _, it := range w.items {
						if it.Key == k && !target[k] && !it.Disabled {
							if st.checked[0][k] {
								delete(st.checked[0], k)
							} else {
								st.checked[0][k] = true
							}
						}
					}
				}
				c.invalid = true
			}
		})
	}, layout.Grow(1))
}

// tfTextW is the width of a row title (measure of custom rows).
func tfTextW(c *Context, s string) float32 {
	return entTextW(c, s, c.ThemeFor(theme.CompTransfer).FontSize, text.Regular)
}
