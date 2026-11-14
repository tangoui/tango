package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// FooterToolbar (Ant Design Pro FooterToolbar)
//
//	ui.FooterToolbar(c).Extra(func() { ui.Text(c, "2 selected") }).Body(func() {
//		ui.Button(c, "Cancel").Clicked()
//		ui.Button(c, "Submit").Primary().Clicked()
//	})
//
// The bar spans the width of its parent (the content area). Placed as the
// last child of a column that fills the window (or PageContainer.Fill) it
// sits at the bottom; Sticky pins it to the bottom edge of the window (or
// of Target's scroll box) while the page scrolls under it.

// FooterToolbarW builds a FooterToolbar.
type FooterToolbarW struct {
	_ [0]func()

	c      *Context
	extra  func()
	sticky bool
	target ID
	opts   []layout.Option
	box    *Box
}

// FooterToolbar starts a bottom action bar; chain options, then Body with
// the right-hand buttons.
//
// ConfigProvider.Defaults("FooterToolbar", ...) keys: "sticky" (bool).
func FooterToolbar(c *Context) *FooterToolbarW {
	return &FooterToolbarW{c: c, sticky: Prop(c, "FooterToolbar", "sticky", false)}
}

// Extra builds content at the left of the bar.
func (f *FooterToolbarW) Extra(fn func()) *FooterToolbarW { f.extra = fn; return f }

// Sticky pins the bar to the bottom of the viewport while the content above
// scrolls (Affix with offset bottom 0).
func (f *FooterToolbarW) Sticky() *FooterToolbarW { f.sticky = true; return f }

// Target makes Sticky pin to the bottom of a scroll box (ScrollBox.ID())
// instead of the window.
func (f *FooterToolbarW) Target(id ID) *FooterToolbarW { f.sticky, f.target = true, id; return f }

// Layout applies layout options to the bar.
func (f *FooterToolbarW) Layout(o ...layout.Option) *FooterToolbarW {
	f.opts = append(f.opts, o...)
	return f
}

// ID returns the bar's widget ID (after Body).
func (f *FooterToolbarW) ID() ID {
	if f.box == nil {
		return 0
	}
	return f.box.n.id
}

// Body builds the bar with fn as the right-hand actions.
func (f *FooterToolbarW) Body(fn func()) *FooterToolbarW {
	c := f.c
	if !f.sticky {
		f.bar(fn)
		return f
	}
	a := Affix(c).OffsetBottom(0)
	if f.target != 0 {
		a.Target(f.target)
	}
	a.Body(func() { f.bar(fn) })
	return f
}

func (f *FooterToolbarW) bar(fn func()) {
	c, th := f.c, f.c.Theme
	el := pro2El(c)
	h := float32(56)
	padH := float32(24)
	if el {
		h, padH = 60, 20
	}
	opts := append([]layout.Option{layout.H(h), layout.PadVH(0, padH), layout.Middle(), layout.Gap(8)}, f.opts...)
	b := pro2Box(c, "pro2-footer-toolbar", "", layout.Row, opts)
	f.box = b
	b.Semantics(a11y.RoleToolbar, c.Locale().PageContainer.FooterToolbar)
	if pro2Shad(c) {
		// No backdrop blur primitive: shadcn's sticky bars use
		// bg-background/95 with a top border and no shadow.
		b.Bg(theme.Fade(th.BgContainer, 0.95)).Paint(shad2dTopLine(th.BorderSecondary))
		f.barBody(fn)
		return
	}
	// A slightly translucent surface with an upward shadow, like Pro's
	// frosted bar.
	b.Bg(theme.Fade(th.BgContainer, 0.96))
	b.Shadow([]paint.Shadow{
		{Y: -6, Blur: 16, Spread: -8, Color: theme.RGBA(0, 0, 0, .08)},
		{Y: -9, Blur: 28, Color: theme.RGBA(0, 0, 0, .05)},
	})
	split := th.Split
	b.Paint(func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Y, r.W, split) })
	f.barBody(fn)
}

func (f *FooterToolbarW) barBody(fn func()) {
	c := f.c
	f.box.Body(func() {
		Row(c, func() {
			if f.extra != nil {
				f.extra()
			}
		}, layout.Grow(1), layout.MinW(0), layout.Middle(), layout.Gap(8))
		if fn != nil {
			Row(c, fn, layout.Middle(), layout.Gap(8))
		}
	})
}

// ---------------------------------------------------------------------------
// PageContainer (Ant Design Pro PageContainer)
//
//	ui.PageContainer(c, "Title").Breadcrumb("Home", "List").SubTitle("sub").
//		Extra(func() { ui.Button(c, "Action").Primary() }).
//		Tabs(&tab, []ui.TabItem{{Key: "a", Label: "A"}, {Key: "b", Label: "B"}}).
//		Footer(func() { ui.Button(c, "Submit").Primary() }).
//		Body(func() { ... })
//
// The header (breadcrumb, back, title, sub title, tags, extra, content,
// extra content, tab list) is a PageHeader; the body follows on the layout
// background; Footer builds a FooterToolbar. It fits inside ProLayout's
// content area (Ghost drops the header surface and paddings there).

// PageContainerW builds a PageContainer.
type PageContainerW struct {
	_ [0]func()

	c           *Context
	title, sub  string
	crumbs      []string
	back        bool
	tags        func()
	extra       func()
	content     func()
	extraCont   func()
	tabs        []TabItem
	tabActive   *string
	tabExtra    func()
	tabsCard    bool
	footer      func()
	footerExtra func()
	loading     bool
	ghost       bool
	fill        bool
	noHeader    bool
	opts        []layout.Option

	header     *PageHeaderW
	tabChanged bool
	scrollID   ID
	footerBar  *FooterToolbarW
}

// PageContainer starts a page container titled title; chain options, then
// Body with the page content.
//
// ConfigProvider.Defaults("PageContainer", ...) keys: "ghost", "fill",
// "tabsCard" (bool).
func PageContainer(c *Context, title string) *PageContainerW {
	return &PageContainerW{c: c, title: title, ghost: Prop(c, "PageContainer", "ghost", false),
		fill: Prop(c, "PageContainer", "fill", false), tabsCard: Prop(c, "PageContainer", "tabsCard", false)}
}

// SubTitle sets the secondary title.
func (p *PageContainerW) SubTitle(s string) *PageContainerW { p.sub = s; return p }

// Breadcrumb shows a breadcrumb above the title.
func (p *PageContainerW) Breadcrumb(items ...string) *PageContainerW { p.crumbs = items; return p }

// Back shows the back button (query BackClicked).
func (p *PageContainerW) Back() *PageContainerW { p.back = true; return p }

// Tags builds tags after the title.
func (p *PageContainerW) Tags(fn func()) *PageContainerW { p.tags = fn; return p }

// Extra builds the actions at the right of the title.
func (p *PageContainerW) Extra(fn func()) *PageContainerW { p.extra = fn; return p }

// Content builds the description area under the title.
func (p *PageContainerW) Content(fn func()) *PageContainerW { p.content = fn; return p }

// ExtraContent builds a block at the right of Content.
func (p *PageContainerW) ExtraContent(fn func()) *PageContainerW { p.extraCont = fn; return p }

// Tabs shows a tab list at the bottom of the header bound to active.
func (p *PageContainerW) Tabs(active *string, items []TabItem) *PageContainerW {
	p.tabActive, p.tabs = active, items
	return p
}

// TabExtra builds content at the right of the tab list.
func (p *PageContainerW) TabExtra(fn func()) *PageContainerW { p.tabExtra = fn; return p }

// TabsCard uses card-style tabs.
func (p *PageContainerW) TabsCard() *PageContainerW { p.tabsCard = true; return p }

// Footer builds the right-hand buttons of a FooterToolbar.
func (p *PageContainerW) Footer(fn func()) *PageContainerW { p.footer = fn; return p }

// FooterExtra builds the left side of the FooterToolbar.
func (p *PageContainerW) FooterExtra(fn func()) *PageContainerW { p.footerExtra = fn; return p }

// Loading shows a centred spinner instead of the body.
func (p *PageContainerW) Loading(on bool) *PageContainerW { p.loading = on; return p }

// Ghost drops the header background and the content padding (for use inside
// a padded content area). Inside ProLayout a non-ghost PageContainer
// switches the content area to NoPadding, so the header spans it.
func (p *PageContainerW) Ghost() *PageContainerW { p.ghost = true; return p }

// Fill makes the container take its parent's free height: header and body
// scroll inside it and the footer toolbar stays at the bottom.
func (p *PageContainerW) Fill() *PageContainerW { p.fill = true; return p }

// NoHeader hides the page header (body and footer only).
func (p *PageContainerW) NoHeader() *PageContainerW { p.noHeader = true; return p }

// Layout applies layout options to the container.
func (p *PageContainerW) Layout(o ...layout.Option) *PageContainerW {
	p.opts = append(p.opts, o...)
	return p
}

// BackClicked reports a click on the back button.
func (p *PageContainerW) BackClicked() bool { return p.header != nil && p.header.BackClicked() }

// CrumbClicked returns the index of a clicked breadcrumb item or -1.
func (p *PageContainerW) CrumbClicked() int {
	if p.header == nil {
		return -1
	}
	return p.header.CrumbClicked()
}

// TabChanged reports a tab switch this frame.
func (p *PageContainerW) TabChanged() bool { return p.tabChanged }

// ScrollID returns the scroll box of a Fill container (0 otherwise).
func (p *PageContainerW) ScrollID() ID { return p.scrollID }

// Body builds the container with fn as the page content.
func (p *PageContainerW) Body(fn func()) *PageContainerW {
	c := p.c
	opts := []layout.Option{layout.MinW(0)}
	if p.fill {
		opts = append(opts, layout.Grow(1), layout.MinH(0))
	}
	opts = append(opts, p.opts...)
	if pc := proLayoutContext(c); pc.depth > 0 && !p.ghost {
		pc.filled = true // ProLayout drops its content padding (NoPadding)
	}
	root := pro2Box(c, "pro2-page-container", p.title, layout.Column, opts)
	root.Semantics(a11y.RoleGroup, p.title)
	root.Body(func() {
		if p.fill {
			sb := Scroll(c, func() { p.main(fn) }, layout.Grow(1), layout.MinH(0))
			p.scrollID = sb.ID()
		} else {
			p.main(fn)
		}
		if p.footer != nil || p.footerExtra != nil {
			ft := FooterToolbar(c).Extra(p.footerExtra)
			if !p.fill {
				ft.Sticky()
			}
			ft.Body(p.footer)
			p.footerBar = ft
		}
	})
	return p
}

// main builds header and content.
func (p *PageContainerW) main(fn func()) {
	c, th := p.c, p.c.Theme
	el := pro2El(c)
	if !p.noHeader {
		hdr := Column(c, func() { p.buildHeader() })
		if !p.ghost {
			hdr.Bg(th.BgContainer)
			if len(p.tabs) == 0 {
				split := th.Split
				hdr.Paint(func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Bottom()-1, r.W, split) })
			}
		}
	}
	pad := geom.All(24)
	if el {
		pad = geom.All(20)
	}
	if p.ghost {
		pad = geom.Insets{Top: 16}
	}
	if p.loading {
		Column(c, func() { Spin(c).Size(Large) }, layout.Padding(pad), layout.Center(), layout.MinH(200)).
			Semantics(a11y.RoleStatus, c.Locale().Loading).A11yState(a11y.Busy)
		return
	}
	Column(c, fn, layout.Padding(pad), layout.Grow(1), layout.MinW(0))
}

func (p *PageContainerW) buildHeader() {
	c := p.c
	el := pro2El(c)
	ph := PageHeader(c, p.title).SubTitle(p.sub)
	if len(p.crumbs) > 0 {
		ph.Breadcrumb(p.crumbs...)
	}
	if p.back {
		ph.Back()
	}
	if p.tags != nil {
		ph.Tags(p.tags)
	}
	if p.extra != nil {
		ph.Extra(p.extra)
	}
	if p.content != nil || p.extraCont != nil {
		ph.Content(func() {
			Row(c, func() {
				Column(c, func() {
					if p.content != nil {
						p.content()
					}
				}, layout.Grow(1), layout.MinW(0))
				if p.extraCont != nil {
					Column(c, p.extraCont, layout.Margin(geom.Insets{Left: 24}))
				}
			}, layout.AlignItems(layout.AlignStart))
		})
	}
	padH := float32(24)
	if el {
		padH = 20
	}
	if p.ghost {
		padH = 0
	}
	// PageHeader pads itself with 16/24 (Ant) and nothing (Element); the
	// container aligns it with the content and the tab list.
	tabs := len(p.tabs) > 0 && p.tabActive != nil
	switch {
	case p.ghost:
		ph.Padding(geom.Insets{})
	case el:
		ph.Padding(geom.VH(20, padH))
	case tabs:
		ph.Padding(geom.Insets{Top: 16, Left: 24, Right: 24})
	}
	if tabs && el && !p.ghost {
		ph.Padding(geom.Insets{Top: 20, Left: padH, Right: padH})
	}
	Column(c, func() { ph.Show() })
	p.header = ph
	if tabs {
		Column(c, func() {
			tb := Tabs(c, p.tabActive, p.tabs).NoMargin()
			if p.tabsCard {
				tb.Card()
			}
			if p.tabExtra != nil {
				tb.Extra(p.tabExtra)
			}
			tb.Show()
			p.tabChanged = tb.Changed()
		}, layout.Padding(geom.Insets{Top: 8, Left: padH, Right: padH}))
	}
}
