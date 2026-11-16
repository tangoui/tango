package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/internal/flexbox"
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
//
// Pinned, the bar is frosted: the page under it is blurred
// (paint.BackdropBlur) and shows through a translucent background
// (FooterToolbarToken: BlurRadius, FrostedBg per design system). In flow,
// the bar draws the token background. High-contrast themes and renderers
// without backdrop blur use an opaque BgContainer only while pinned. The blurred region follows the bar's rectangle, in
// right-to-left layouts too (nothing is mirrored).

// FooterToolbarW builds a FooterToolbar.
type FooterToolbarW struct {
	_ [0]func()

	c      *Context
	extra  func()
	sticky bool
	fixed  bool
	target ID
	opts   []layout.Option
	box    *Box
	affix  *AffixW // Sticky: the Affix holding the bar
}

// FooterToolbar starts a bottom action bar; chain options, then Body with
// the right-hand buttons.
//
// ConfigProvider.Defaults("FooterToolbar", ...) keys: "fixed" (bool, default true), "sticky" (legacy bool).
func FooterToolbar(c *Context) *FooterToolbarW {
	w := &FooterToolbarW{c: c, fixed: Prop(c, "FooterToolbar", "fixed", true)}
	if v, ok := c.ConfigProp("FooterToolbar", "sticky"); ok {
		if on, ok := v.(bool); ok && on {
			w.fixed = false
			w.sticky = true
		}
	}
	return w
}

// Extra builds content at the left of the bar.
func (f *FooterToolbarW) Extra(fn func()) *FooterToolbarW { f.extra = fn; return f }

// Sticky pins the bar to the bottom of the viewport while the content above
// scrolls (Affix with offset bottom 0).
func (f *FooterToolbarW) Sticky() *FooterToolbarW { f.sticky = true; f.fixed = false; return f }

// Fixed controls whether the bar is pinned to the bottom (default true).
// Fixed(false) places it in normal flow. Sticky remains an alias for enabling it.
func (f *FooterToolbarW) Fixed(on bool) *FooterToolbarW { f.fixed = on; f.sticky = false; return f }

// Target makes Sticky pin to the bottom of a scroll box (ScrollBox.ID())
// instead of the window.
func (f *FooterToolbarW) Target(id ID) *FooterToolbarW {
	f.target = id
	if !f.fixed {
		f.sticky = true
	}
	return f
}

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
	if f.fixed {
		f.fixedBar(fn)
		return f
	}
	if !f.sticky {
		f.bar(fn)
		return f
	}
	a := Affix(c).OffsetBottom(0)
	if f.target != 0 {
		a.Target(f.target)
	}
	f.affix = a
	a.Body(func() { f.bar(fn) })
	return f
}

// fixedBar reserves the bar's measured height in flow and positions the
// overlay after the current frame's container layout has been solved.
func (f *FooterToolbarW) fixedBar(fn func()) {
	c := f.c
	tk := &c.ThemeFor(theme.CompFooterToolbar).Components.FooterToolbar
	marker := c.box("footer-toolbar-marker", layout.Column, []layout.Option{layout.MinH(tk.MinHeight)})
	var clip *node
	ov := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
		clip = c.add("footer-toolbar-clip", "")
		clip.ln.Absolute = true
		clip.clip = true
		c.body(clip, func() { f.bar(fn) })
	})
	ov.n.hit, ov.n.blocks = false, false
	bar := f.box.n
	bar.ln.Absolute = true
	bar.hit, bar.blocks = true, true
	marker.n.ln.Measure = func(maxW, maxH float32) geom.Size {
		link(bar)
		return flexbox.SolveAt(&bar.ln, maxW, geom.Inf, maxW < geom.Inf, false)
	}
	ov.n.paint = func(_ *paint.List, _ geom.Rect) {
		view := geom.Rect{W: c.size.W, H: c.size.H}
		if f.target != 0 {
			if r, ok := c.anchorRect(f.target); ok {
				view = r
			}
		}
		anchor := navAbsRect(marker.n)
		x := max(view.X, anchor.X)
		width := max(float32(0), min(view.Right(), anchor.Right())-x)
		clip.ln.Rect = view
		flexbox.SolveAt(&bar.ln, width, geom.Inf, true, false)
		bar.ln.Rect.X, bar.ln.Rect.Y = x-view.X, view.H-bar.ln.Rect.H
	}
}

func (f *FooterToolbarW) bar(fn func()) {
	c, th := f.c, f.c.Theme
	tk := &c.ThemeFor(theme.CompFooterToolbar).Components.FooterToolbar
	opts := append([]layout.Option{layout.MinH(tk.MinHeight), layout.PadVH(0, tk.PaddingInline), layout.Middle(), layout.Gap(8)}, f.opts...)
	b := pro2Box(c, "pro2-footer-toolbar", "", layout.Row, opts)
	f.box = b
	b.Semantics(a11y.RoleToolbar, c.Locale().PageContainer.FooterToolbar)
	f.surface(b)
	if pro2Shad(c) {
		// shadcn's sticky bars: a top border and no shadow.
		b.Paint(shad2dTopLine(th.BorderSecondary))
		f.barBody(fn)
		return
	}
	// An upward shadow, like Pro's bar.
	b.Shadow([]paint.Shadow{
		{Y: -6, Blur: 16, Spread: -8, Color: theme.RGBA(0, 0, 0, .08)},
		{Y: -9, Blur: 28, Color: theme.RGBA(0, 0, 0, .05)},
	})
	split := th.Split
	b.Paint(func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Y, r.W, split) })
	f.barBody(fn)
}

// surface sets the bar's background: frosted while pinned (see
// FooterToolbar), else the plain one.
func (f *FooterToolbarW) surface(b *Box) {
	c := f.c
	th := c.ThemeFor(theme.CompFooterToolbar)
	tk := &th.Components.FooterToolbar
	if f.fixed || f.affix != nil && f.affix.Affixed() {
		if th.HighContrast || c.noBlur {
			b.Bg(th.BgContainer)
			return
		}
		if tk.BlurRadius > 0 {
			b.Bg(tk.FrostedBg)
			b.blur = tk.BlurRadius
			return
		}
	}
	b.Bg(tk.Bg)
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
	if pc := proLayoutContext(c); pc.depth > 0 {
		pc.hasPage = true
		if !p.ghost {
			pc.filled = true
		}
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
			ft := FooterToolbar(c).Extra(p.footerExtra).Fixed(!p.fill)

			ft.Body(p.footer)
			p.footerBar = ft
		}
	})
	return p
}

// main builds header and content.
func (p *PageContainerW) main(fn func()) {
	c := p.c
	tk := &c.ThemeFor(theme.CompLayout).Components.Layout
	rc := RouteContext(c)
	if p.title == "" {
		p.title = rc.PageTitle
	}
	if p.title == "" && p.sub == "" && len(p.crumbs) == 0 && len(rc.BreadcrumbItems) == 0 && !p.back && p.tags == nil && p.extra == nil && p.content == nil && p.extraCont == nil && len(p.tabs) == 0 {
		p.noHeader = true
	}
	if !p.noHeader {
		hdr := Column(c, func() { p.buildHeader() })
		if !p.ghost {
			hdr.Bg(tk.PageBg)
		}
	}
	pad := geom.Insets{Left: tk.PagePaddingInline, Right: tk.PagePaddingInline, Bottom: tk.PagePaddingBlock}
	if p.noHeader {
		pad.Top = tk.PagePaddingBlock
	}
	if p.ghost {
		pad = geom.Insets{Top: tk.PagePaddingBlock / 2}
	}
	if p.loading {
		PageLoading(c).Show()
		return
	}
	Column(c, fn, layout.Padding(pad), layout.Grow(1), layout.MinW(0))
}

func (p *PageContainerW) buildHeader() {
	c := p.c
	tk := &c.ThemeFor(theme.CompLayout).Components.Layout
	ph := PageHeader(c, p.title).SubTitle(p.sub)
	if p.crumbs != nil {
		ph.Breadcrumb(p.crumbs...)
	} else {
		ph.BreadcrumbItems(RouteContext(c).BreadcrumbItems)
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
	padH := tk.PagePaddingInline
	tabs := len(p.tabs) > 0 && p.tabActive != nil
	headerPad := geom.Insets{Top: tk.PageHeaderPaddingTop, Bottom: tk.PageHeaderPaddingBottom, Left: padH, Right: padH}
	if tabs {
		headerPad.Bottom = 0
	}
	if p.ghost {
		padH = 0
		headerPad = geom.Insets{}
	}
	ph.Padding(headerPad)
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
