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
	c        *Context
	n        int
	header   func()
	footer   func()
	bordered bool
	size     ControlSize
	split    bool
	loading  bool
	page     *int
	pageSize int
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
}

// List starts a list of n items; render each with Item.
//
// ConfigProvider.Defaults("List", ...) keys: "size", "bordered", "split".
func List(c *Context, n int) *ListW {
	w := &ListW{c: c, n: n, split: true, size: c.defaultSize(), empty: c.Locale().NoData, th: c.ThemeFor(theme.CompList)}
	w.tk = &w.th.Components.List
	if s, ok := dataPropSize(c, "List"); ok {
		w.size = s
	}
	w.bordered = Prop(c, "List", "bordered", false)
	w.split = Prop(c, "List", "split", true)
	return w
}

func (w *ListW) Header(fn func()) *ListW   { w.header = fn; return w }
func (w *ListW) Footer(fn func()) *ListW   { w.footer = fn; return w }
func (w *ListW) Bordered() *ListW          { w.bordered = true; return w }
func (w *ListW) Size(s ControlSize) *ListW { w.size = s; return w }
func (w *ListW) Split(on bool) *ListW      { w.split = on; return w }
func (w *ListW) Loading(on bool) *ListW    { w.loading = on; return w }
func (w *ListW) EmptyText(s string) *ListW { w.empty = s; return w }
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
// page's items; *page is 1-based and owned by the app.
func (w *ListW) Pagination(page *int, pageSize int) *ListW {
	w.page, w.pageSize = page, max(1, pageSize)
	return w
}

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

func (w *ListW) pad() (v, h float32) {
	tk := w.tk
	switch w.size {
	case Small:
		v, h = tk.ItemPaddingBlockSM, tk.ItemPaddingInlineSM
	case Large:
		v, h = tk.ItemPaddingBlockLG, tk.ItemPaddingInlineLG
	default:
		v, h = tk.ItemPaddingBlock, tk.ItemPaddingInline
	}
	if !w.bordered {
		h = 0
	}
	return
}

// Item builds the list, calling fn for every visible item index.
func (w *ListW) Item(fn func(i int)) *ListW {
	c, tk := w.c, w.tk
	pv, ph := w.pad()
	a, b := 0, w.n
	if w.page != nil {
		a, b = listRange(w.n, *w.page, w.pageSize)
	}
	hasFooter := w.footer != nil
	inner := func() {
		if w.header != nil {
			hb := Column(c, w.header, layout.PadVH(pv, ph))
			if tk.HeaderBg.A > 0 {
				hb.Bg(tk.HeaderBg)
			}
			hb.Paint(func(l *paint.List, r geom.Rect) {
				if w.split || w.bordered {
					dataHLine(l, r.X, r.Bottom()-1, r.W, tk.SplitColor)
				}
			})
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
			for i := a; i < b; i++ {
				i := i
				lastRow := i == b-1
				Key(c, i, func() {
					row := Row(c, func() { fn(i) }, layout.PadVH(pv, ph), layout.Middle(), layout.Gap(16))
					w.itemSem(row.n, i)
					if w.split && (!lastRow || (hasFooter && !w.bordered) || w.more != "") {
						row.Paint(func(l *paint.List, r geom.Rect) {
							dataHLine(l, r.X, r.Bottom()-1, r.W, tk.SplitColor)
						})
					}
				})
			}
		}
		list := func() {
			lb := Column(c, body)
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
			}, layout.PadVH(pv, ph))
			if tk.FooterBg.A > 0 {
				fb.Bg(tk.FooterBg)
			}
			if w.bordered {
				fb.Paint(func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Y, r.W, tk.SplitColor) })
			}
		}
	}
	w.box = Column(c, inner)
	if w.bordered {
		w.box.Border(1, tk.BorderColor).Radius(tk.Radius)
	}
	if w.page != nil && w.n > w.pageSize {
		Row(c, func() {
			w.pager = Pagination(c, w.page, w.n, w.pageSize).Show()
		}, layout.End(), layout.Margin(geom.Insets{Top: 24}))
	}
	return w
}

func (w *ListW) gridItems(a, b int, fn func(i int)) {
	c := w.c
	cols := w.gridAt.resolve(c, w.grid)
	gap := w.gridGap
	Column(c, func() {
		for r := a; r < b; r += cols {
			r := r
			Row(c, func() {
				for i := r; i < r+cols; i++ {
					i := i
					if i >= b {
						Column(c, nil, layout.Grow(1))
						continue
					}
					Key(c, i, func() { w.itemSem(Column(c, func() { fn(i) }, layout.Grow(1), layout.MinW(0)).n, i) })
				}
			}, layout.Gap(gap), layout.AlignItems(layout.AlignStretch))
		}
	}, layout.Gap(gap))
}

// itemSem marks n as list item i; with pagination it carries its
// position in the whole data set.
func (w *ListW) itemSem(n *node, i int) {
	n.setRole(a11y.RoleListItem)
	if w.page != nil && w.c.a11yOn {
		total := w.n
		n.fill(semFunc(func(s *a11y.Node) { s.PosInSet, s.SetSize = i+1, total }))
	}
}

// ListItemMeta is the avatar + title + description block of a list item.
// avatar may be nil. It grows to take the row's free space.
func ListItemMeta(c *Context, avatar func(), title, desc string) *Box {
	tk := &c.ThemeFor(theme.CompList).Components.List
	return Row(c, func() {
		if avatar != nil {
			Row(c, avatar, layout.Margin(geom.Insets{Right: tk.AvatarMarginInlineEnd}))
		}
		Column(c, func() {
			if title != "" {
				t := Text(c, title).Color(tk.TitleColor)
				if tk.TitleFontWeight != 0 {
					t.Weight(text.Weight(tk.TitleFontWeight))
				}
				if desc != "" {
					t.Layout(layout.Margin(geom.Insets{Bottom: tk.TitleMarginBottom}))
				}
			}
			if desc != "" {
				Text(c, desc).Color(tk.DescriptionColor)
			}
		}, layout.Grow(1), layout.MinW(0))
	}, layout.Grow(1), layout.MinW(0), layout.AlignItems(layout.AlignStart))
}

// ListActions lays out item actions at the end of a list row, separated by
// short vertical bars (Ant's actions).
func ListActions(c *Context, fn func()) *Box {
	tk := &c.ThemeFor(theme.CompList).Components.List
	b := Row(c, fn, layout.Gap(17), layout.Middle(), layout.Margin(geom.Insets{Left: tk.ActionsMarginInlineStart}))
	b.n.ln.Shrink = 0
	b.PaintOver(func(l *paint.List, r geom.Rect) {
		kids := b.n.kids
		for i := 0; i+1 < len(kids); i++ {
			kr := kids[i].abs
			x := kr.Right() + 8 // centred in the 17px gap after the action
			if c.IsRTL() {
				x = kr.X - 9
			}
			dataVLine(l, x, r.Y+(r.H-14)/2, 14, tk.ActionsSplitColor)
		}
	})
	return b
}
