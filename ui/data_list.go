package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ListW renders a list of n items (Ant Design List).
type ListW struct {
	_ [0]func()

	c        *Context
	n        int
	header   func()
	footer   func()
	bordered bool
	size     ControlSize
	split    bool
	loading  bool
	vertical bool
	extra    func(i int)
	page     *int
	pageSize int
	pagerPos string
	pagerAl  string
	pager    *PaginationW
	grid     int
	gridGap  float32
	more     string
	moreHit  bool
	moreBusy bool
	box      *Box
	empty    string
	gridAt   dataRespCols
	th       *theme.Theme
	tk       *theme.ListToken

	// Viewport and virtual form (Height, Virtual, ItemHeight,
	// EstimatedItemHeight, RowKey, ScrollToKey).
	height     float32
	virtual    bool
	itemH      float32
	est        float32
	rowKey     func(i int) any
	req        listScroll
	first, end int
	view       ID // the items' viewport (Height)
}

// listScroll is a ScrollToKey request.
type listScroll struct {
	set    bool
	key    any
	align  string
	offset float64
}

// List starts a list of n items; render each with Item.
//
// ConfigProvider.Defaults("List", ...) keys: "size", "bordered", "split",
// "itemLayout" ("horizontal" | "vertical").
func List(c *Context, n int) *ListW {
	w := &ListW{c: c, n: max(0, n), split: true, size: c.defaultSize(), empty: c.Locale().NoData, th: c.ThemeFor(theme.CompList)}
	w.tk = &w.th.Components.List
	if s, ok := dataPropSize(c, "List"); ok {
		w.size = s
	}
	w.bordered = Prop(c, "List", "bordered", false)
	w.split = Prop(c, "List", "split", true)
	w.vertical = Prop(c, "List", "itemLayout", "horizontal") == "vertical"
	return w
}

func (w *ListW) Header(fn func()) *ListW   { w.header = fn; return w }
func (w *ListW) Footer(fn func()) *ListW   { w.footer = fn; return w }
func (w *ListW) Bordered() *ListW          { w.bordered = true; return w }
func (w *ListW) Size(s ControlSize) *ListW { w.size = s; return w }
func (w *ListW) Split(on bool) *ListW      { w.split = on; return w }
func (w *ListW) Loading(on bool) *ListW    { w.loading = on; return w }
func (w *ListW) EmptyText(s string) *ListW { w.empty = s; return w }

// ItemLayout sets the item layout (antd itemLayout): "horizontal" (the
// default: content and actions on one line) or "vertical" (actions under
// the content, ItemExtra in a column at the inline end; ListItemMeta gets
// the large title and ListActions moves under the content).
func (w *ListW) ItemLayout(s string) *ListW { w.vertical = s == "vertical"; return w }

// ItemExtra builds extra content of item i (antd List.Item extra): at the
// inline end of the row, or in its own column beside the content and
// actions with the vertical layout.
func (w *ListW) ItemExtra(fn func(i int)) *ListW { w.extra = fn; return w }

// Grid lays the items out in columns separated by gap px (antd grid
// gutter); every grid row keeps GridItemMarginBottom below it.
func (w *ListW) Grid(columns int, gap float32) *ListW {
	w.grid, w.gridGap = max(1, columns), gap
	return w
}

// GridAt sets the grid column count from breakpoint bp upwards (Ant
// grid={{ xs: 1, sm: 2, md: 4 }}); the largest set breakpoint not above
// the window width wins, Grid's count is the fallback. Without Grid the
// list becomes a grid with a 16px gap.
func (w *ListW) GridAt(bp Breakpoint, columns int) *ListW {
	if w.grid == 0 {
		w.grid, w.gridGap = 1, 16
	}
	w.gridAt.set(bp, columns)
	return w
}

// Pagination shows a pager under the list and only renders the current
// page's items; *page is 1-based and owned by the app (out-of-range pages
// show the nearest page).
func (w *ListW) Pagination(page *int, pageSize int) *ListW {
	w.page, w.pageSize = page, max(1, pageSize)
	return w
}

// PaginationPosition places the pager (antd pagination.position):
// "bottom" (default), "top" or "both".
func (w *ListW) PaginationPosition(pos string) *ListW { w.pagerPos = pos; return w }

// PaginationAlign aligns the pager (antd pagination.align): "start",
// "center" or "end" (default).
func (w *ListW) PaginationAlign(a string) *ListW { w.pagerAl = a; return w }

// LoadMore shows a centered "load more" button under the items; query the
// click with LoadMoreClicked. busy shows the button in its loading state.
func (w *ListW) LoadMore(label string, busy bool) *ListW {
	w.more, w.moreBusy = label, busy
	return w
}

// LoadMoreClicked reports a click on the load-more button in the last Item call.
func (w *ListW) LoadMoreClicked() bool { return w.moreHit }

// PageChanged reports a page change in the last Item call.
func (w *ListW) PageChanged() bool { return w.pager != nil && w.pager.Changed() }

// Layout applies layout options to the list (call after Item).
func (w *ListW) Layout(o ...layout.Option) *ListW {
	if w.box != nil {
		w.box.Style(o...)
	}
	return w
}

// Height fixes the height of the items' viewport (antd height); longer
// content scrolls inside it. Header, Footer, LoadMore and the pager stay
// outside it. Default 0: the list grows with its items.
func (w *ListW) Height(px float32) *ListW { w.height = max(0, px); return w }

// Virtual builds only the items in the viewport (and a few past each
// edge) instead of every item; it needs Height and is ignored without it.
// With Pagination it windows the current page; with Grid it windows rows
// of columns items (a row is as high as its tallest item). The item that
// holds the keyboard focus is always built. Items scrolled out of view
// are not built, so state kept inside them (State, an input's text)
// is dropped after a while and they are missing from the accessibility
// tree, FindText and the Tab order, as with Listy and VirtualList.
// Heights are measured as items come into view and cached by RowKey; an
// item never measured counts EstimatedItemHeight, so the scroll bar can
// adjust the first time a taller item appears.
func (w *ListW) Virtual() *ListW { w.virtual = true; return w }

// ItemHeight fixes every item's height, padding included (the fast path
// of Virtual: nothing is measured). Default 0: items size to their content.
func (w *ListW) ItemHeight(px float32) *ListW { w.itemH = max(0, px); return w }

// EstimatedItemHeight is the height Virtual assumes for an item not
// measured yet. Default: one line of text plus the item padding (46 at
// the middle size, as Listy).
func (w *ListW) EstimatedItemHeight(px float32) *ListW { w.est = max(1, px); return w }

// RowKey gives items stable identities for the heights Virtual measures
// (default: the index). It does not change how items are keyed; wrap the
// item content in Key for state that follows the data.
func (w *ListW) RowKey(fn func(i int) any) *ListW { w.rowKey = fn; return w }

// ScrollToKey scrolls the viewport (Height) to the item with key (RowKey;
// the index without one) using align (AlignTop, AlignBottom or AlignAuto)
// plus offset pixels. An item of the current page only; under Virtual an
// item not measured yet is placed by its estimate first and corrected over
// the next frames, as Listy.
func (w *ListW) ScrollToKey(key any, align string, offset float64) *ListW {
	w.req = listScroll{set: true, key: key, align: align, offset: offset}
	return w
}

// Range returns the item indexes [first, end) built by the last Item
// call: the current page, or under Virtual the window (the focused item
// aside, which is built even outside it).
func (w *ListW) Range() (first, end int) { return w.first, w.end }

// listRange returns the half-open item range shown for the current page.
func listRange(n, page, pageSize int) (int, int) {
	if pageSize <= 0 {
		return 0, n
	}
	pages := max(1, (n+pageSize-1)/pageSize)
	page = max(1, min(page, pages))
	a := (page - 1) * pageSize
	return a, min(n, a+pageSize)
}

// pad returns the item padding: antd pads items 12/0 (middle), 8/16
// (small) and 16/24 (large); a bordered middle list pads 24 inline.
func (w *ListW) pad() (v, h float32) {
	tk := w.tk
	switch w.size {
	case Small:
		return tk.ItemPaddingBlockSM, tk.ItemPaddingInlineSM
	case Large:
		return tk.ItemPaddingBlockLG, tk.ItemPaddingInlineLG
	}
	if w.bordered {
		return tk.ItemPaddingBlock, tk.ItemPaddingInline
	}
	return tk.ItemPaddingBlock, 0
}

// edgePad returns the header and footer padding: paddingSM block, and
// inline only with a border (a bordered small / large list pads them like
// its items).
func (w *ListW) edgePad() (v, h float32) {
	if w.bordered {
		if w.size == Small || w.size == Large {
			return w.pad()
		}
		return w.tk.HeaderPaddingBlock, w.tk.ItemPaddingInline
	}
	return w.tk.HeaderPaddingBlock, 0
}

// Item builds the list, calling fn for every visible item index.
func (w *ListW) Item(fn func(i int)) *ListW {
	c, tk := w.c, w.tk
	ev, eh := w.edgePad()
	a, b := 0, w.n
	if w.page != nil {
		a, b = listRange(w.n, *w.page, w.pageSize)
	}
	showPager := w.page != nil && w.n > w.pageSize
	pagerTop := showPager && (w.pagerPos == "top" || w.pagerPos == "both")
	pagerBottom := showPager && w.pagerPos != "top"
	// antd .ant-list-something-after-last-item: the last item keeps its
	// rule when a footer, load-more or pager is there (list/index.js:91: any
	// pagination, top or bottom).
	after := w.footer != nil || (w.more != "" && w.n > 0) || showPager
	pager := func(top bool) {
		m := geom.Insets{Top: tk.PaginationMarginTop}
		if w.bordered {
			bv, bh := tk.PaginationMarginBlockBordered, tk.PaginationMarginInlineBordered
			m = geom.Insets{Top: bv, Bottom: bv, Left: bh, Right: bh}
		}
		just := layout.JustifyEnd
		switch w.pagerAl {
		case "start":
			just = layout.JustifyStart
		case "center":
			just = layout.JustifyCenter
		}
		Row(c, func() {
			pw := Pagination(c, w.page, w.n, w.pageSize).Show()
			if w.pager == nil || pw.Changed() {
				w.pager = pw
			}
		}, layout.JustifyContent(just), layout.Margin(m))
	}
	items := func() {
		for i := a; i < b; i++ {
			w.itemRow(i, fn, i == b-1, after)
		}
	}
	inner := func() {
		if w.header != nil {
			hb := Column(c, w.header, layout.PadVH(ev, eh))
			if tk.HeaderBg.A > 0 {
				hb.Bg(tk.HeaderBg)
			}
			if w.split {
				hb.Paint(func(l *paint.List, r geom.Rect) {
					dataHLine(l, r.X, r.Bottom()-1, r.W, tk.SplitColor)
				})
			}
		}
		if pagerTop {
			pager(true)
		}
		body := func() {
			if w.n == 0 {
				Column(c, func() { Empty(c).Simple().Description(w.empty) }, layout.PadVH(tk.EmptyTextPadding, 0), layout.Center())
				return
			}
			if w.grid > 0 {
				w.gridItems(a, b, fn)
				return
			}
			w.itemScope(items)
		}
		w.first, w.end = a, b
		list := func() {
			var lb *Box
			switch {
			case w.height > 0 && w.virtual && w.n > 0:
				sb := fillViewport(c, "list-view", []layout.Option{layout.H(w.height)})
				if w.grid > 0 {
					w.virtItems(sb, a, b, after, fn)
				} else {
					w.itemScope(func() { w.virtItems(sb, a, b, after, fn) })
				}
				lb, w.view = sb.Box, sb.n.id
				n := w.n
				lb.n.fill(semFunc(func(s *a11y.Node) { s.RowCount = n }))
			case w.height > 0:
				sb := Scroll(c, body, layout.H(w.height))
				w.scrollFull(sb, a, b)
				lb, w.view = sb.Box, sb.n.id
			default:
				lb = Column(c, body)
			}
			if w.n > 0 {
				lb.n.setRole(a11y.RoleList)
			}
		}
		if w.loading {
			Loading(c, true).Body(list)
		} else {
			list()
		}
		if w.more != "" && w.n > 0 {
			Row(c, func() {
				if Button(c, w.more).Loading(w.moreBusy).Clicked() {
					w.moreHit = true
				}
			}, layout.Center(), layout.Margin(geom.Insets{Top: 12, Bottom: 12}))
		}
		if w.footer != nil {
			fb := Column(c, func() {
				Column(c, w.footer)
			}, layout.PadVH(ev, eh))
			if tk.FooterBg.A > 0 {
				fb.Bg(tk.FooterBg)
			}
			if w.split && w.n == 0 {
				// antd .ant-list-split.ant-list-empty .ant-list-footer
				fb.Paint(func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Y, r.W, tk.SplitColor) })
			}
		}
		if pagerBottom && w.bordered {
			pager(false)
		}
	}
	w.box = Column(c, inner)
	if w.bordered {
		w.box.Border(1, tk.BorderColor).Radius(tk.Radius)
	}
	if pagerBottom && !w.bordered {
		pager(false)
	}
	return w
}

// itemScope builds fn with the item layout visible to ListItemMeta and
// ListActions (Defaults "ListItem" "itemLayout"); a horizontal list
// nested in a vertical one resets it.
func (w *ListW) itemScope(fn func()) {
	c := w.c
	inherited := Prop(c, "ListItem", "itemLayout", "horizontal") == "vertical"
	if w.vertical == inherited {
		fn()
		return
	}
	mode := "horizontal"
	if w.vertical {
		mode = "vertical"
	}
	ConfigProvider(c).Defaults("ListItem", Props{"itemLayout": mode}).Body(fn)
}

func (w *ListW) gridItems(a, b int, fn func(i int)) {
	c := w.c
	cols := w.gridAt.resolve(c, w.grid)
	Column(c, func() {
		for r := a; r < b; r += cols {
			w.gridRow(r, b, cols, fn)
		}
	})
}

// itemRow builds the row of item i (keyed by its index) and returns it;
// lastRow and after (something follows the items) decide its divider.
func (w *ListW) itemRow(i int, fn func(i int), lastRow, after bool) *node {
	c, tk := w.c, w.tk
	pv, ph := w.pad()
	var row *Box
	Key(c, i, func() {
		var hopt []layout.Option
		if w.itemH > 0 {
			hopt = []layout.Option{layout.H(w.itemH)}
		}
		if w.vertical {
			// antd vertical: item-main (content + actions) and
			// item-extra side by side, aligned to the top.
			row = Row(c, func() {
				Column(c, func() { fn(i) }, layout.Grow(1), layout.MinW(0))
				if w.extra != nil {
					Column(c, func() { w.extra(i) }, layout.Margin(geom.Insets{Left: tk.VerticalExtraMarginInlineStart}))
				}
			}, append([]layout.Option{layout.PadVH(pv, ph), layout.AlignItems(layout.AlignStart)}, hopt...)...)
		} else {
			row = Row(c, func() {
				fn(i)
				if w.extra != nil {
					w.extra(i)
				}
			}, append([]layout.Option{layout.PadVH(pv, ph), layout.Middle(), layout.JustifyContent(layout.JustifySpaceBetween)}, hopt...)...)
		}
		w.itemSem(row.n, i)
		if w.split && (!lastRow || after) {
			row.Paint(func(l *paint.List, r geom.Rect) {
				dataHLine(l, r.X, r.Bottom()-1, r.W, tk.SplitColor)
			})
		}
	})
	return row.n
}

// gridRow builds the grid row of items [r, r+cols) (cut at b) and returns it.
func (w *ListW) gridRow(r, b, cols int, fn func(i int)) *node {
	c := w.c
	var hopt []layout.Option
	if w.itemH > 0 {
		hopt = []layout.Option{layout.H(w.itemH)}
	}
	return Row(c, func() {
		for i := r; i < r+cols; i++ {
			i := i
			if i >= b {
				Column(c, nil, layout.Grow(1))
				continue
			}
			Key(c, i, func() {
				w.itemSem(Column(c, func() { fn(i) }, append([]layout.Option{layout.Grow(1), layout.MinW(0)}, hopt...)...).n, i)
			})
		}
	}, layout.Gap(w.gridGap), layout.AlignItems(layout.AlignStretch), layout.Margin(geom.Insets{Bottom: w.tk.GridItemMarginBottom})).n
}

// itemSem marks n as list item i; with pagination it carries its
// position in the whole data set.
func (w *ListW) itemSem(n *node, i int) {
	n.setRole(a11y.RoleListItem)
	if (w.page != nil || w.virtualOn()) && w.c.a11yOn {
		total := w.n
		n.fill(semFunc(func(s *a11y.Node) { s.PosInSet, s.SetSize = i+1, total }))
	}
}

// ListItemMetaW is the block ListItemMeta returns; it embeds the row's
// Box.
type ListItemMetaW struct {
	_ [0]func()

	*Box
}

// ListItemMeta is the avatar + title + description block of a list item.
// avatar may be nil. It grows to take the row's free space.
//
// In a vertical list (List.ItemLayout("vertical")) the title is large
// with a 12px gap under it and the block keeps 16px below.
func ListItemMeta(c *Context, avatar func(), title, desc string) *ListItemMetaW {
	tk := &c.ThemeFor(theme.CompList).Components.List
	vertical := Prop(c, "ListItem", "itemLayout", "horizontal") == "vertical"
	opts := []layout.Option{layout.Grow(1), layout.MinW(0), layout.AlignItems(layout.AlignStart)}
	if vertical {
		opts = []layout.Option{layout.MinW(0), layout.AlignItems(layout.AlignStart), layout.Margin(geom.Insets{Bottom: tk.VerticalMetaMarginBottom})}
	}
	b := Row(c, func() {
		if avatar != nil {
			Row(c, avatar, layout.Margin(geom.Insets{Right: tk.AvatarMarginInlineEnd}))
		}
		Column(c, func() {
			if title != "" {
				t := Text(c, title).Color(tk.TitleColor)
				if tk.TitleFontWeight != 0 {
					t.Weight(text.Weight(tk.TitleFontWeight))
				}
				mb := tk.TitleMarginBottom
				if vertical {
					t.Size(tk.VerticalTitleFontSize)
					mb = tk.VerticalTitleMarginBottom
				}
				if desc != "" {
					t.Layout(layout.Margin(geom.Insets{Bottom: mb}))
				}
			}
			if desc != "" {
				Text(c, desc).Color(tk.DescriptionColor)
			}
		}, layout.Grow(1), layout.MinW(0))
	}, opts...)
	return &ListItemMetaW{Box: b}
}

// ListActionsW is the action row ListActions returns; it embeds the row's
// Box.
type ListActionsW struct {
	_ [0]func()

	*Box
}

// ListActions lays out item actions at the end of a list row, separated by
// short vertical bars (Ant's actions). In a vertical list they sit under
// the content, wider apart.
func ListActions(c *Context, fn func()) *ListActionsW {
	tk := &c.ThemeFor(theme.CompList).Components.List
	gap := tk.ActionsGap
	m := geom.Insets{Left: tk.ActionsMarginInlineStart}
	if Prop(c, "ListItem", "itemLayout", "horizontal") == "vertical" {
		gap = tk.VerticalActionsGap
		m = geom.Insets{Top: tk.VerticalActionsMarginTop}
	}
	m.Right = gap / 2 // antd: the last li keeps its end padding
	b := Row(c, fn, layout.Gap(gap), layout.Middle(), layout.Margin(m))
	b.n.ln.Shrink = 0
	b.n.ln.NoStretch = true
	b.PaintOver(func(l *paint.List, r geom.Rect) {
		kids := b.n.kids
		for i := 0; i+1 < len(kids); i++ {
			kr := kids[i].abs
			x := kr.Right() + gap/2 // the bar sits at the end of the action's li
			if c.IsRTL() {
				x = kr.X - gap/2 - 1
			}
			dataVLine(l, x, r.Y+(r.H-14)/2, 14, tk.ActionsSplitColor)
		}
	})
	return &ListActionsW{Box: b}
}
