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

// TransferItem is an entry of a Transfer.
type TransferItem struct {
	Key, Title string
	Disabled   bool
}

type tfState struct {
	checked [2]map[string]bool
	query   [2]string
	inited  bool

	// p2b_transfer.go: pages, tree source, virtual keyboard focus.
	page      [2]int
	treeExp   []string
	treeKeys  []string
	treeInit  bool
	focusKey  [2]string // virtual lists: keyboard item of each side
	focusID   [2]ID     // its row node at the last build
	focusPend [2]bool   // move focus to it once built
}

// TransferW is a two-list shuttle: items move between the source (left)
// and the target (right) list.
type TransferW struct {
	c        *Context
	th       *theme.Theme         // ThemeFor(CompTransfer)
	tk       *theme.TransferToken // &th.Components.Transfer
	n        *node
	items    []TransferItem
	target   *[]string
	st       *tfState
	titles   [2]string
	ops      [2]string
	listW    float32
	listH    float32
	search   bool
	disabled bool
	status   string
	changed  bool
	z        *lazy

	render   func(o Option) // custom row content (Render)
	pageSize int
	oneWay   bool
	tree     []TreeNode
	virtual  int // 0 auto (long lists), 1 always, -1 never
}

// Transfer adds a transfer bound to the keys of the right-hand list.
func Transfer(c *Context, items []TransferItem, target *[]string) *TransferW {
	w := &TransferW{c: c, items: items, target: target, status: c.itemStatus, titles: [2]string{c.Locale().Transfer.Source, c.Locale().Transfer.Target}}
	w.th = c.ThemeFor(theme.CompTransfer)
	w.tk = &w.th.Components.Transfer
	w.n = c.add("transfer", "")
	w.n.ln.Dir = layout.Row
	w.n.ln.Align = layout.AlignCenter
	w.n.ln.NoStretch = true
	w.st = State(c, w.n.id, func() tfState {
		return tfState{checked: [2]map[string]bool{{}, {}}}
	})
	w.transferDefaults()
	w.z = c.lazy(w.n, w.build)
	return w
}

// Titles sets the list titles.
func (w *TransferW) Titles(left, right string) *TransferW {
	w.titles = [2]string{left, right}
	return w
}

// Operations labels the move buttons (default: icons only).
func (w *TransferW) Operations(toRight, toLeft string) *TransferW {
	w.ops = [2]string{toRight, toLeft}
	return w
}

// ListSize sets the size of each list box.
func (w *TransferW) ListSize(wd, ht float32) *TransferW { w.listW, w.listH = wd, ht; return w }

// Searchable adds a filter input to each list.
func (w *TransferW) Searchable() *TransferW { w.search = true; return w }

// Status sets the validation status ("error", "warning").
func (w *TransferW) Status(s string) *TransferW { w.status = s; return w }

// Disabled disables the whole transfer.
func (w *TransferW) Disabled(on bool) *TransferW {
	if on {
		w.disabled = true
	}
	return w
}

// DefaultChecked checks keys when the widget is first shown.
func (w *TransferW) DefaultChecked(keys ...string) *TransferW {
	if !w.st.inited {
		for _, k := range keys {
			side := 0
			if contains(*w.target, k) {
				side = 1
			}
			w.st.checked[side][k] = true
		}
	}
	return w
}

// Changed reports items moved this frame.
func (w *TransferW) Changed() bool { w.z.run(); return w.changed }

// Checked returns the checked keys of a side (0 left, 1 right).
func (w *TransferW) Checked(side int) []string {
	var out []string
	for _, it := range w.items {
		if w.st.checked[side][it.Key] {
			out = append(out, it.Key)
		}
	}
	return out
}

func (w *TransferW) side(it TransferItem) int {
	if contains(*w.target, it.Key) {
		return 1
	}
	return 0
}

// list returns the (filtered) items of a side.
func (w *TransferW) list(side int, filtered bool) []TransferItem {
	q := strings.ToLower(strings.TrimSpace(w.st.query[side]))
	var out []TransferItem
	for _, it := range w.items {
		if w.side(it) != side {
			continue
		}
		if filtered && q != "" && !strings.Contains(strings.ToLower(it.Title), q) {
			continue
		}
		out = append(out, it)
	}
	return out
}

func (w *TransferW) move(from int) {
	var keys []string
	for _, it := range w.list(from, false) {
		if w.st.checked[from][it.Key] && !it.Disabled {
			keys = append(keys, it.Key)
			delete(w.st.checked[from], it.Key)
		}
	}
	if len(keys) == 0 {
		return
	}
	if from == 0 {
		*w.target = append(*w.target, keys...)
	} else {
		for _, k := range keys {
			*w.target = remove(*w.target, k)
		}
	}
	w.changed = true
	w.c.invalid = true
}

func (w *TransferW) build() {
	c := w.c
	w.st.inited = true
	w.syncTreeItems()
	Disabled(c, w.disabled, func() {
		dis := w.disabled || w.n.disabled
		// Drop checks of items that moved or vanished.
		for side := 0; side < 2; side++ {
			for k := range w.st.checked[side] {
				found := false
				for _, it := range w.items {
					if it.Key == k && w.side(it) == side {
						found = true
					}
				}
				if !found {
					delete(w.st.checked[side], k)
				}
			}
		}
		w.listBox(0, dis)
		w.buttons(dis)
		w.listBox(1, dis)
	})
}

func (w *TransferW) buttons(dis bool) {
	c := w.c
	el := w.th.Flavor == theme.Element
	has := [2]bool{}
	for side := 0; side < 2; side++ {
		for _, it := range w.list(side, false) {
			if w.st.checked[side][it.Key] && !it.Disabled {
				has[side] = true
			}
		}
	}
	m := float32(8)
	if el {
		m = 30
	}
	Column(c, func() {
		btn := func(side int) {
			ic := icon.Right
			if side == 1 {
				ic = icon.Left
			}
			b := Button(c, w.ops[side]).Disabled(dis || !has[side])
			if w.th.Flavor != theme.StyleShadcn {
				b.Primary() // shadcn: outline icon buttons (size-8)
			}
			if w.ops[side] == "" {
				b.Icon(ic)
				loc := c.Locale()
				if side == 0 {
					b.A11yLabel(loc.A11y.MoveToTarget)
				} else {
					b.A11yLabel(loc.A11y.MoveToSource)
				}
			} else if side == 0 {
				b.IconRight(ic)
			} else {
				b.Icon(ic)
			}
			if !el {
				b.Small()
			}
			if b.Clicked() {
				w.move(side)
			}
		}
		switch {
		case w.oneWay:
			btn(0)
		case el:
			btn(1)
			btn(0)
		default:
			btn(0)
			btn(1)
		}
	}, layout.Gap(4), layout.Margin(geom.Insets{Left: m, Right: m}), layout.Middle())
}

func (w *TransferW) listBox(side int, dis bool) {
	c := w.c
	th, tk := w.th, w.tk
	el := th.Flavor == theme.Element
	lw, lh := w.listW, w.listH
	if lw == 0 {
		lw = tk.ListWidth
	}
	if lh == 0 {
		lh = tk.ListHeight
	}
	shad := th.Flavor == theme.StyleShadcn
	all := w.list(side, false)
	shown := w.list(side, true)
	checked := w.st.checked[side]
	border := tk.BorderColor
	rad := tk.ListBorderRadius
	switch w.status {
	case "error":
		border = th.Error.Base
	case "warning":
		border = th.Warning.Base
	}
	box := Column(c, func() {
		// Header.
		nChecked, nEnabled := 0, 0
		for _, it := range shown {
			if it.Disabled {
				continue
			}
			nEnabled++
			if checked[it.Key] {
				nChecked++
			}
		}
		total := 0
		for _, it := range all {
			if checked[it.Key] {
				total++
			}
		}
		hdr := Row(c, func() {
			v := nEnabled > 0 && nChecked == nEnabled
			if w.oneWay && side == 1 {
				// One-way target: no checks, items have remove buttons.
			} else if Checkbox(c, "", &v).Indeterminate(nChecked > 0 && !v).Disabled(dis || nEnabled == 0).A11yLabel(c.Locale().SelectAll).Changed() {
				for _, it := range shown {
					if !it.Disabled {
						if v {
							checked[it.Key] = true
						} else {
							delete(checked, it.Key)
						}
					}
				}
			}
			cnt := c.Locale().Items(len(all))
			if total > 0 {
				cnt = itoaEnt(total) + "/" + cnt
			}
			switch {
			case el:
				Text(c, w.titles[side]).Size(16).NoWrap().Color(th.Text).Layout(layout.Margin(geom.Insets{Left: 8}))
				Spacer(c)
				Text(c, itoaEnt(total)+"/"+itoaEnt(len(all))).Size(12).NoWrap().Color(th.TextTertiary)
			case shad:
				// shadcn: title text-sm font-medium, count muted-foreground.
				Text(c, w.titles[side]).NoWrap().Weight(text.Medium).Color(th.Text).Layout(layout.Margin(geom.Insets{Left: 8}))
				Spacer(c)
				Text(c, cnt).NoWrap().Color(th.TextTertiary)
			default:
				Text(c, cnt).NoWrap().Layout(layout.Margin(geom.Insets{Left: 8}))
				Spacer(c)
				Text(c, w.titles[side]).NoWrap()
			}
		}, layout.H(tk.HeaderHeight), layout.PadVH(0, 12), layout.Middle())
		if tk.HeaderBg.A > 0 {
			hdr.Bg(tk.HeaderBg).Radii(geom.Radii{TL: rad, TR: rad})
		}
		if el {
			hdr.n.ln.Padding.Left, hdr.n.ln.Padding.Right = 15, 15
		}
		hdr.Paint(func(l *paint.List, r geom.Rect) {
			l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, tk.DividerColor)
		})
		if w.search {
			Div(c, layout.Padding(geom.All(12))).Body(func() {
				in := Input(c, &w.st.query[side]).Placeholder(c.Locale().Transfer.SearchPlaceholder).Prefix(icon.Search).AllowClear()
				if el {
					in.Size(Small)
				}
			})
		}
		if side == 0 && w.tree != nil {
			w.treeBody(dis)
			return
		}
		if len(shown) == 0 {
			Column(c, func() { Empty(c).Simple().Description(c.Locale().NoData) }, layout.Grow(1), layout.Center())
			return
		}
		shown = w.pageOf(side, shown)
		if w.virtualOn(len(shown)) {
			w.virtualBody(side, shown, dis)
			w.pager(side, dis)
			return
		}
		nodes := make([]*node, 0, len(shown))
		sb := Scroll(c, func() {
			for _, it := range shown {
				it := it
				Key(c, it.Key, func() { nodes = append(nodes, w.item(side, it, dis)) })
			}
		}, layout.Grow(1), layout.Padding(geom.Insets{Top: 4, Bottom: 4}))
		// A multi-selectable listbox of checkable options: one Tab stop,
		// arrow keys move, Space / Enter toggle the focused item.
		sb.n.setRole(a11y.RoleListbox).setName(w.titles[side]).setState(a11y.Multiselectable)
		cur := -1
		ids := make([]ID, len(nodes))
		for i, n := range nodes {
			ids[i] = n.id
			if c.focus == n.id {
				cur = i
			}
		}
		rovingTabStop(nodes, cur)
		if to := rovingStep(c, ids, func(i int) bool { return !nodes[i].disabled }, rovingVertical); to >= 0 {
			navFocus(c, ids[to])
			c.ScrollIntoView(sb.n.id, ids[to])
		}
		w.pager(side, dis)
	}, layout.Size(lw, lh))
	if len(tk.ListShadow) > 0 {
		box.Shadow(tk.ListShadow) // shadcn: shadow-xs on the rounded-lg list card
	}
	box.Radius(rad).Bg(tk.ListBg).Clip().PaintOver(func(l *paint.List, r geom.Rect) {
		l.Stroke(r, geom.Rad(rad), 1, border)
	})
}

func (w *TransferW) item(side int, it TransferItem, dis bool) *node {
	c := w.c
	th, tk := w.th, w.tk
	n := c.add("tf-item", it.Title)
	h := tk.ItemHeight
	n.ln.Height = h
	shad := th.Flavor == theme.StyleShadcn
	if shad {
		n.ln.Margin = geom.Insets{Left: 4, Right: 4} // items inset in p-1, rounded-sm
	}
	off := it.Disabled || dis
	if off {
		n.disabled = true
	}
	cur := input.CursorPointer
	if off {
		cur = input.CursorNotAllowed
	}
	ia := c.interact(n, !off, cur)
	checked := w.st.checked[side]
	if w.oneWay && side == 1 {
		return w.oneWayItem(n, it, ia, off, h)
	}
	if w.render != nil {
		w.renderItem(n, it)
	}
	if ia.Clicked && !off {
		if checked[it.Key] {
			delete(checked, it.Key)
		} else {
			checked[it.Key] = true
		}
		c.invalid = true
	}
	on := checked[it.Key]
	n.setRole(a11y.RoleOption).setState(a11y.Selectable|a11y.Checkable).stateIf(on, a11y.Selected|a11y.Checked)
	n.paint = func(l *paint.List, r geom.Rect) {
		col := tk.ItemColor
		x := r.X + tk.ItemPaddingInline
		if ia.Hovered && !off && tk.ItemHoverColor.A > 0 {
			col = tk.ItemHoverColor
		}
		rr := geom.Rad(tk.ItemBorderRadius)
		switch {
		case on && !off:
			bg := tk.ItemSelectedBg
			if ia.Hovered {
				bg = tk.ItemSelectedHoverBg
			}
			if bg.A > 0 {
				l.Fill(r, rr, bg)
			}
		case ia.Hovered && !off && tk.ItemHoverBg.A > 0:
			l.Fill(r, rr, tk.ItemHoverBg)
		}
		if off {
			col = th.TextDisabled
		}
		// Checkbox and title are placed as in a left-to-right row and
		// mirrored in RTL.
		bs := c.boxSize()
		drawCheckbox(c, l, treeMirror(geom.Rect{X: x, Y: r.Y + (r.H-bs)/2, W: bs, H: bs}, r, c.IsRTL()), checkLook{checked: on, disabled: off, hovered: ia.Hovered && !off, t: 1})
		x += bs + 8
		if w.render == nil {
			treeText(c, l, r, x, it.Title, c.style(th.FontSize, text.Regular), r.Right()-x-12, treeMid, col)
		}
		if c.focus == n.id && c.focusVisible {
			entFocusRing(c, l, r.Inset(geom.All(3)), th.RadiusSM)
		}
	}
	return n
}
