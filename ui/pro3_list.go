package ui

import (
	"slices"
	"strconv"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ProListMetas maps a row to the parts of a ProList item. Every part is
// optional.
type ProListMetas[T any] struct {
	Title       func(row T) string
	SubTitle    func(c *Context, row T) // next to the title (tags)
	Description func(row T) string
	Avatar      func(c *Context, row T)
	Content     func(c *Context, row T) // middle area
	Actions     func(c *Context, row T) // links at the end, split by bars
	Extra       func(c *Context, row T) // right-most (an image)
}

// ProListW is a data-driven list (Ant Design Pro ProList): items built
// from metas, a toolbar with a title and actions, selection, expandable
// items, pagination, a loading state and a card grid mode.
//
//	ui.ProList(c, rows, metas).HeaderTitle("Projects").RowKey(key).
//		Selection(&sel).Pagination(5).Show()
type ProListW[T any] struct {
	_ [0]func()

	c        *Context
	rows     []T
	metas    ProListMetas[T]
	title    string
	toolbar  func()
	rowKey   func(T) string
	sel      *[]string
	expand   func(c *Context, row T)
	pageSize int
	loading  bool
	grid     int
	bordered bool
	split    bool
	empty    string
	onClick  func(row T)
	request  func(page, size int) ([]T, int, error)

	id      ID
	st      *pro3ListState
	changed bool
	list    *ListW
}

type pro3ListState struct {
	page     int
	expanded map[string]bool

	// Remote data (Request).
	reqPage, reqSize int // page of the last request (0: none yet)
	reload           int
	reqReload        int
	seq              int
	loading          bool
	err              error
	remote           any // []T of the current page
	total            int
	res              pro3Async[pro3ListRes]
}

// pro3ListRes is a delivered page of a ProList request.
type pro3ListRes struct {
	seq   int
	rows  any // []T
	total int
	err   error
}

// ProList starts a list over rows; chain options, then Show.
func ProList[T any](c *Context, rows []T, metas ProListMetas[T]) *ProListW[T] {
	p := &ProListW[T]{c: c, rows: rows, metas: metas, split: true}
	// ConfigProvider.Defaults("ProList", ...) keys: "pagination" (false,
	// a page size, or Props{"pageSize": n}), "split", "bordered" (bool),
	// "grid" (column count).
	if _, ok := c.ConfigProp("ProList", "pagination"); ok {
		p.pageSize, _ = proPropPageSize(c, "ProList", 0)
	}
	p.split = Prop(c, "ProList", "split", p.split)
	p.bordered = Prop(c, "ProList", "bordered", p.bordered)
	if n, ok := proPropInt(c, "ProList", "grid"); ok {
		p.grid = n
	}
	return p
}

// HeaderTitle sets the toolbar title.
func (p *ProListW[T]) HeaderTitle(s string) *ProListW[T] { p.title = s; return p }

// ToolBar builds actions at the right of the toolbar.
func (p *ProListW[T]) ToolBar(fn func()) *ProListW[T] { p.toolbar = fn; return p }

// RowKey gives rows stable keys (selection and expansion follow them;
// default: the row index).
func (p *ProListW[T]) RowKey(f func(T) string) *ProListW[T] { p.rowKey = f; return p }

// Selection adds a checkbox per item bound to the selected keys.
func (p *ProListW[T]) Selection(keys *[]string) *ProListW[T] { p.sel = keys; return p }

// Expandable adds an expand toggle per item showing fn below it.
func (p *ProListW[T]) Expandable(fn func(c *Context, row T)) *ProListW[T] { p.expand = fn; return p }

// Pagination shows n items per page with a pager.
func (p *ProListW[T]) Pagination(n int) *ProListW[T] { p.pageSize = max(1, n); return p }

// Loading shows the loading mask.
func (p *ProListW[T]) Loading(on bool) *ProListW[T] { p.loading = on; return p }

// Grid lays the items out as cards in n columns.
func (p *ProListW[T]) Grid(n int) *ProListW[T] { p.grid = max(1, n); return p }

// Bordered frames the list.
func (p *ProListW[T]) Bordered() *ProListW[T] { p.bordered = true; return p }

// Split toggles the lines between items (default on).
func (p *ProListW[T]) Split(on bool) *ProListW[T] { p.split = on; return p }

// EmptyText replaces the empty-state text.
func (p *ProListW[T]) EmptyText(s string) *ProListW[T] { p.empty = s; return p }

// OnItemClick makes the items (cards in grid mode) clickable.
func (p *ProListW[T]) OnItemClick(fn func(row T)) *ProListW[T] { p.onClick = fn; return p }

// Request loads the list page by page from a server: fn runs on a
// background goroutine with the 1-based page and the page size (set with
// Pagination, default 10) and returns the page's rows and the total count.
// The rows passed to ProList are ignored; the list shows a loading mask
// while a page loads and an error with a retry button when it fails.
func (p *ProListW[T]) Request(fn func(page, size int) ([]T, int, error)) *ProListW[T] {
	p.request = fn
	return p
}

// Reload loads the current page again (valid after Show).
func (p *ProListW[T]) Reload() {
	if p.st != nil {
		p.st.reload++
		p.c.invalid = true
	}
}

// Total returns the total count reported by Request.
func (p *ProListW[T]) Total() int {
	if p.st == nil {
		return 0
	}
	return p.st.total
}

// Err returns the error of the last Request.
func (p *ProListW[T]) Err() error {
	if p.st == nil {
		return nil
	}
	return p.st.err
}

// IsLoading reports a Request in flight.
func (p *ProListW[T]) IsLoading() bool { return p.st != nil && p.st.loading }

// SelectionChanged reports a selection change in the last Show.
func (p *ProListW[T]) SelectionChanged() bool { return p.changed }

// Page returns the current page (1-based).
func (p *ProListW[T]) Page() int {
	if p.st == nil {
		return 1
	}
	return p.st.page
}

// ID returns the widget ID (valid after Show).
func (p *ProListW[T]) ID() ID { return p.id }

func (p *ProListW[T]) key(i int) string {
	if p.rowKey != nil {
		return p.rowKey(p.rows[i])
	}
	return strconv.Itoa(i)
}

func (p *ProListW[T]) selected(k string) bool {
	return p.sel != nil && slices.Contains(*p.sel, k)
}

func (p *ProListW[T]) toggle(k string) {
	if p.sel == nil {
		return
	}
	if i := slices.Index(*p.sel, k); i >= 0 {
		*p.sel = slices.Delete(*p.sel, i, i+1)
	} else {
		*p.sel = append(*p.sel, k)
	}
	p.changed = true
	p.c.invalid = true
}

// Show builds the list.
func (p *ProListW[T]) Show() *ProListW[T] {
	c, th := p.c, p.c.Theme
	root := c.box("pro-list", layout.Column, nil)
	p.id = root.n.id
	st := State[pro3ListState](c, p.id, func() pro3ListState { return pro3ListState{page: 1, expanded: map[string]bool{}} })
	p.st = st
	if p.request != nil {
		p.remoteData()
	} else if p.pageSize > 0 {
		pages := max(1, (len(p.rows)+p.pageSize-1)/p.pageSize)
		st.page = max(1, min(st.page, pages))
	}
	root.Semantics(a11y.RoleList, p.title)
	root.Body(func() {
		if p.title != "" || p.toolbar != nil {
			Row(c, func() {
				if p.title != "" {
					t := Text(c, p.title).Size(th.FontSizeLG).Weight(text.Medium).Color(th.TextHeading)
					if shad2d(c) {
						t.Weight(text.Semibold)
					}
				}
				Spacer(c)
				if p.toolbar != nil {
					Row(c, p.toolbar, layout.Gap(8), layout.Middle())
				}
			}, layout.Middle(), layout.MinH(56)).Semantics(a11y.RoleToolbar, p.title)
		}
		if p.sel != nil && len(*p.sel) > 0 && shad2d(c) {
			if shad2dSelectionBar(c, len(*p.sel), nil, nil) {
				*p.sel = (*p.sel)[:0]
				p.changed = true
				c.invalid = true
			}
		} else if p.sel != nil && len(*p.sel) > 0 {
			Row(c, func() {
				Text(c, c.Locale().Selected(len(*p.sel)))
				if Link(c, c.Locale().ProTable.ClearSelection).Clicked() {
					*p.sel = (*p.sel)[:0]
					p.changed = true
					c.invalid = true
				}
			}, layout.PadVH(9, 24), layout.Gap(16), layout.Middle(), layout.Margin(geom.Insets{Bottom: 16})).
				Bg(th.Primary.Bg).Border(1, th.Primary.Border).Radius(th.RadiusLG)
		}
		if p.request != nil && st.err != nil {
			Row(c, func() {
				Alert(c, "error", locFmt(c.Locale().ProTable.LoadErrorFmt, st.err.Error())).Show()
			}, layout.Margin(geom.Insets{Bottom: 12}))
			Row(c, func() {
				if Button(c, c.Locale().Retry).Small().Icon(icon.Reload).Clicked() {
					p.Reload()
				}
			}, layout.Margin(geom.Insets{Bottom: 12}))
		}
		w := List(c, len(p.rows)).Loading(p.loading || st.loading).Split(p.split && p.grid == 0)
		if p.empty != "" {
			w.EmptyText(p.empty)
		}
		if p.bordered && p.grid == 0 {
			w.Bordered()
		}
		if p.grid > 0 {
			w.Grid(p.grid, 16)
		}
		if p.pageSize > 0 && p.request == nil {
			w.Pagination(&st.page, p.pageSize)
		}
		p.list = w
		w.Item(func(i int) {
			if p.grid > 0 {
				p.card(i)
			} else {
				p.item(i)
			}
		})
		if p.request != nil && st.total > 0 {
			size := p.pageSize
			if size <= 0 {
				size = 10
			}
			Row(c, func() {
				if Pagination(c, &st.page, st.total, size).Show().Changed() {
					c.invalid = true
				}
			}, layout.End(), layout.Margin(geom.Insets{Top: 24}))
		}
	})
	if p.request != nil {
		p.remoteData() // a page change above starts its request now
	}
	return p
}

// remoteData polls the delivered page and requests the current one when
// the page, size or reload count changed.
func (p *ProListW[T]) remoteData() {
	c, st := p.c, p.st
	for {
		r, ok := st.res.poll()
		if !ok {
			break
		}
		if r.seq != st.seq {
			continue
		}
		st.loading, st.err = false, r.err
		if r.err == nil {
			st.remote, st.total = r.rows, r.total
		} else {
			st.remote = nil
		}
		c.invalid = true
	}
	size := p.pageSize
	if size <= 0 {
		size = 10
	}
	if st.total > 0 {
		st.page = max(1, min(st.page, (st.total+size-1)/size))
	}
	if st.reqPage != st.page || st.reqSize != size || st.reqReload != st.reload {
		st.reqPage, st.reqSize, st.reqReload = st.page, size, st.reload
		st.seq++
		st.loading = true
		seq, page, fn, ch, inv := st.seq, st.page, p.request, st.res.chInit(), c.Invalidator()
		go func() {
			rows, total, err := fn(page, size)
			ch <- pro3ListRes{seq: seq, rows: rows, total: total, err: err}
			inv()
		}()
	}
	if rows, ok := st.remote.([]T); ok {
		p.rows = rows
	} else {
		p.rows = nil
	}
}

// expander is the caret toggle of an expandable item.
func (p *ProListW[T]) expander(k string) {
	c, st := p.c, p.st
	loc := c.Locale()
	open := st.expanded[k]
	lbl := loc.Expand
	ic := icon.Right
	if open {
		lbl, ic = loc.Collapse, icon.Down
	}
	if pro3IconBtn(c, lbl, ic, 20) {
		st.expanded[k] = !open
		c.invalid = true
	}
}

// meta builds the avatar + title (+ subtitle) + description block.
func (p *ProListW[T]) meta(row T, grow bool) {
	c, th := p.c, p.c.Theme
	m := p.metas
	opts := []layout.Option{layout.MinW(0), layout.AlignItems(layout.AlignStart)}
	if grow {
		opts = append(opts, layout.Grow(1), layout.Basis(0))
	}
	Row(c, func() {
		if m.Avatar != nil {
			Row(c, func() { m.Avatar(c, row) }, layout.Margin(geom.Insets{Right: 16}))
		}
		Column(c, func() {
			if m.Title != nil || m.SubTitle != nil {
				Row(c, func() {
					if m.Title != nil {
						t := Text(c, m.Title(row)).Color(th.Text).Weight(text.Medium).Ellipsis()
						t.Layout(layout.MinW(0), layout.Shrink(1))
					}
					if m.SubTitle != nil {
						Row(c, func() { m.SubTitle(c, row) }, layout.Gap(4), layout.Middle())
					}
				}, layout.Gap(8), layout.Middle(), layout.MinW(0))
			}
			if m.Description != nil {
				if s := m.Description(row); s != "" {
					Text(c, s).Color(th.TextTertiary).Layout(layout.Margin(geom.Insets{Top: 4}))
				}
			}
		}, layout.Grow(1), layout.MinW(0))
	}, opts...)
}

// item is one row of the list mode.
func (p *ProListW[T]) item(i int) {
	c := p.c
	row := p.rows[i]
	k := p.key(i)
	m := p.metas
	col := Column(c, func() {
		Row(c, func() {
			if p.expand != nil {
				p.expander(k)
			}
			if p.sel != nil {
				on := p.selected(k)
				if Checkbox(c, "", &on).A11yLabel(p.selectName(i, row)).Changed() {
					p.toggle(k)
				}
			}
			p.meta(row, true)
			if m.Content != nil {
				Row(c, func() { m.Content(c, row) }, layout.Grow(1), layout.Basis(0), layout.MinW(0), layout.Middle())
			}
			if m.Actions != nil {
				ListActions(c, func() { m.Actions(c, row) })
			}
			if m.Extra != nil {
				Row(c, func() { m.Extra(c, row) }, layout.Margin(geom.Insets{Left: 24}))
			}
		}, layout.Gap(12), layout.Middle())
		if p.expand != nil && p.st.expanded[k] {
			Column(c, func() { p.expand(c, row) }, layout.Padding(geom.Insets{Top: 12, Left: 32}))
		}
	}, layout.Grow(1), layout.MinW(0))
	col.Semantics(a11y.RoleListItem, p.titleOf(row))
	if p.sel != nil && p.selected(k) {
		col.A11yState(a11y.Selected)
	}
	// A clickable item is a Tab stop: Enter / Space click it.
	if p.onClick != nil && p.itemFocusable(col, p.c.Theme.RadiusSM).Clicked {
		p.onClick(row)
	}
}

// itemFocusable makes an item (row or card) a Tab stop with a focus ring.
func (p *ProListW[T]) itemFocusable(b *Box, rad float32) Interaction {
	th := p.c.Theme
	ia := b.Focusable(input.CursorPointer)
	if ia.FocusVisible {
		b.PaintOver(func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r, geom.Rad(rad)) })
	}
	return ia
}

// selectName is the accessible name of item i's selection checkbox: its
// title, or "Select row n" without one.
func (p *ProListW[T]) selectName(i int, row T) string {
	if t := p.titleOf(row); t != "" {
		return t
	}
	return locFmt(p.c.Locale().Table.SelectRowFmt, i+1)
}

func (p *ProListW[T]) titleOf(row T) string {
	if p.metas.Title != nil {
		return p.metas.Title(row)
	}
	return ""
}

// card is one item of the grid mode.
func (p *ProListW[T]) card(i int) {
	c, th := p.c, p.c.Theme
	row := p.rows[i]
	k := p.key(i)
	m := p.metas
	on := p.selected(k)
	rad := th.RadiusLG
	pad := float32(20)
	switch th.Flavor {
	case theme.Element:
		rad = th.Radius
	case theme.StyleShadcn:
		rad, pad = shad2dCardRadius(th), 24 // Card: rounded-xl, p-6
	}
	b := Column(c, func() {
		Row(c, func() {
			p.meta(row, true)
			if p.sel != nil {
				v := on
				if Checkbox(c, "", &v).A11yLabel(p.selectName(i, row)).Changed() {
					p.toggle(k)
				}
			}
		}, layout.Gap(8), layout.AlignItems(layout.AlignStart))
		if m.Content != nil {
			Column(c, func() { m.Content(c, row) }, layout.Margin(geom.Insets{Top: 12}))
		}
		if p.expand != nil {
			Row(c, func() { p.expander(k) }, layout.Margin(geom.Insets{Top: 8}))
			if p.st.expanded[k] {
				Column(c, func() { p.expand(c, row) }, layout.Margin(geom.Insets{Top: 8}))
			}
		}
		if m.Actions != nil || m.Extra != nil {
			Row(c, func() {
				if m.Actions != nil {
					Row(c, func() { m.Actions(c, row) }, layout.Gap(16), layout.Middle())
				}
				Spacer(c)
				if m.Extra != nil {
					m.Extra(c, row)
				}
			}, layout.Middle(), layout.Margin(geom.Insets{Top: 16}))
		}
	}, layout.Grow(1), layout.Pad(pad), layout.MinW(0))
	// A selectable or clickable card is a Tab stop (Enter / Space toggle
	// the selection / click it); otherwise it only reacts to hover.
	var ia Interaction
	if p.sel != nil || p.onClick != nil {
		ia = p.itemFocusable(b, rad)
	} else {
		ia = b.Interact(input.CursorDefault)
	}
	border := th.BorderSecondary
	switch {
	case on:
		border = th.Primary.Base
	case ia.Hovered:
		border = th.Primary.BorderHover
	}
	b.Bg(th.BgContainer).Radius(rad).Border(1, border)
	if on {
		b.Bg(th.Primary.Bg)
	}
	b.Semantics(a11y.RoleListItem, p.titleOf(row))
	if p.sel != nil {
		st := a11y.State(0)
		if on {
			st = a11y.Selected
		}
		b.A11yState(st)
	}
	if ia.Clicked {
		if p.sel != nil {
			p.toggle(k)
		}
		if p.onClick != nil {
			p.onClick(row)
		}
	}
}
