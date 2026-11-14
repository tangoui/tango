package ui

import (
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

// ---------------------------------------------------------------------------
// Layout (Ant Layout / Element el-container)
//
//	ui.Layout(c, func() {
//		ui.Header(c, header)
//		ui.Layout(c, func() {
//			ui.Sider(c, 200, false, menu)
//			ui.Content(c, page)
//		})
//		ui.Footer(c, footer)
//	})
//
// A Layout lays its regions out horizontally when it directly holds a
// Sider and vertically otherwise; Content and nested Layouts take the free
// space. Region defaults follow the theme: Ant (64px dark header with 50px
// padding, dark sider, layout background) or Element (60px header/footer
// with 20px padding, 20px main padding, no backgrounds). Every region
// returns its *Box, so Bg/Style override the defaults.

// Region labels (internal) used to recognise the regions of a Layout.
const (
	layoutTag  = "ui-layout"
	headerTag  = "ui-layout-header"
	siderTag   = "ui-layout-sider"
	contentTag = "ui-layout-content"
	footerTag  = "ui-layout-footer"
)

func layoutRegion(c *Context, tag string, dir layout.Direction, def, opts []layout.Option, fn func()) *Box {
	b := c.box(tag, dir, nil)
	b.n.label = tag
	flexbox.Apply(&b.n.ln.Style, def...)
	flexbox.Apply(&b.n.ln.Style, opts...)
	return b.Body(fn)
}

// Layout is the page frame holding Header / Sider / Content / Footer (or
// nested Layouts). It fills the free space of its parent.
func Layout(c *Context, fn func(), opts ...layout.Option) *Box {
	b := c.box(layoutTag, layout.Column, nil)
	b.n.label = layoutTag
	b.n.ln.Grow = 1
	b.n.ln.MinW = 0
	flexbox.Apply(&b.n.ln.Style, opts...)
	b.Body(fn)
	for _, k := range b.n.kids {
		if k.label == siderTag {
			b.n.ln.Dir = layout.Row
			break
		}
	}
	for _, k := range b.n.kids {
		if k.label == contentTag || k.label == layoutTag {
			k.ln.Grow = 1
		}
	}
	// Ant: the layout background; shadcn: bg-background; Element: none.
	if bg := c.ThemeFor(theme.CompLayout).Components.Layout.BodyBg; bg.A > 0 {
		b.Bg(bg)
	}
	return b
}

// Header is the top bar.
func Header(c *Context, fn func(), opts ...layout.Option) *Box {
	th := c.ThemeFor(theme.CompLayout)
	tk := &th.Components.Layout
	if shad2d(c) {
		// shadcn site header: h-14 on the page background, border-b.
		h := tk.HeaderHeight
		if v, ok := p2bNum(c, "Header", "height"); ok && v > 0 {
			h = float32(v)
		}
		b := layoutRegion(c, headerTag, layout.Row, []layout.Option{layout.H(h), layout.PadVH(0, tk.HeaderPaddingInline), layout.Gap(8), layout.Middle()}, opts, fn)
		return b.Bg(tk.HeaderBg).Paint(shad2dBottomLine(th.BorderSecondary))
	}
	b := layoutRegion(c, headerTag, layout.Row, []layout.Option{layout.H(tk.HeaderHeight), layout.PadVH(0, tk.HeaderPaddingInline), layout.Middle()}, opts, fn)
	if tk.HeaderBg.A > 0 {
		b.Bg(tk.HeaderBg) // Ant: dark bar; Element: none
	}
	return b
}

// SiderBoxW is the side column Sider returns; it embeds the column's Box.
type SiderBoxW struct {
	_ [0]func()

	*Box
}

// Sider is a side column of fixed width (the theme's collapsed width when
// collapsed, animated). CollapsibleSider adds the collapse trigger and
// breakpoint collapsing.
func Sider(c *Context, width float32, collapsed bool, fn func(), opts ...layout.Option) *SiderBoxW {
	cw := c.ThemeFor(theme.CompLayout).Components.Layout.SiderCollapsedWidth
	if v, ok := p2bNum(c, "Sider", "collapsedWidth"); ok {
		cw = float32(v)
	}
	b := siderBox(c, width, collapsed, cw, fn, opts)
	if shad2d(c) {
		_, border, _, _, _, _ := shad2dSidebar(c.Theme)
		b.Paint(shad2dEndLine(c, border)) // sidebar border-r (border-l in RTL)
	}
	return &SiderBoxW{Box: b}
}

func siderBox(c *Context, width float32, collapsed bool, collapsedW float32, fn func(), opts []layout.Option) *Box {
	th := c.ThemeFor(theme.CompLayout)
	id := hashID(c.scope, "sider")
	w := width
	if collapsed {
		w = collapsedW
	}
	w = c.Anim(id, "w", w, th.DurationMid)
	b := layoutRegion(c, siderTag, layout.Column, []layout.Option{layout.W(w)}, opts, fn)
	// The sider usually holds the main menu: a navigation landmark (apps
	// may override it with Semantics on the returned box).
	b.n.setRole(a11y.RoleNavigation).setName(c.Locale().A11y.Navigation)
	if !navEl(c) {
		b.Bg(th.Components.Layout.SiderBg)
	}
	return b.Clip()
}

// SiderW builds a collapsible Sider (Ant Layout.Sider collapsible).
//
//	ui.CollapsibleSider(c, 200, &collapsed).Breakpoint(ui.LG).Body(menu)
//
// ConfigProvider.Defaults("Sider", ...) keys: "breakpoint" (Breakpoint),
// "collapsedWidth" (float32), "theme" ("light" | "dark").
type SiderW struct {
	_ [0]func()

	c          *Context
	th         *theme.Theme // Layout tokens (c.ThemeFor(theme.CompLayout))
	width      float32
	collapsed  *bool
	collapsedW float32
	bp         Breakpoint
	trigger    bool
	reverse    bool
	light      bool
	changed    bool
	broken     bool
}

type siderState struct {
	seen   bool
	broken bool
}

// CollapsibleSider starts a sider whose collapsed state lives in
// *collapsed: a trigger bar at the bottom toggles it and Breakpoint
// collapses it on narrow windows. Chain options, then Body.
func CollapsibleSider(c *Context, width float32, collapsed *bool) *SiderW {
	th := c.ThemeFor(theme.CompLayout)
	s := &SiderW{c: c, th: th, width: width, collapsed: collapsed, collapsedW: th.Components.Layout.SiderCollapsedWidth, trigger: true}
	s.bp = Prop(c, "Sider", "breakpoint", XS)
	s.collapsedW = Prop(c, "Sider", "collapsedWidth", s.collapsedW)
	s.light = Prop(c, "Sider", "theme", "dark") == "light" || navEl(c) || c.Theme.Flavor == theme.StyleShadcn
	return s
}

// Breakpoint collapses the sider when the window becomes narrower than bp
// and expands it when it grows past it again (Ant breakpoint).
func (s *SiderW) Breakpoint(bp Breakpoint) *SiderW { s.bp = bp; return s }

// CollapsedWidth sets the width when collapsed (default the theme's).
func (s *SiderW) CollapsedWidth(px float32) *SiderW { s.collapsedW = px; return s }

// NoTrigger hides the bottom trigger (collapse from elsewhere).
func (s *SiderW) NoTrigger() *SiderW { s.trigger = false; return s }

// ReverseArrow flips the trigger arrow (a sider on the end side: the
// right, or the left in a right-to-left scope).
func (s *SiderW) ReverseArrow() *SiderW { s.reverse = true; return s }

// Light uses the light sider colors (Ant theme="light").
func (s *SiderW) Light() *SiderW { s.light = true; return s }

// Changed reports a collapse change by the trigger or the breakpoint.
func (s *SiderW) Changed() bool { return s.changed }

// Broken reports whether the window is narrower than the breakpoint.
func (s *SiderW) Broken() bool { return s.broken }

// Body builds the sider with fn as its content.
func (s *SiderW) Body(fn func(), opts ...layout.Option) *Box {
	c, th := s.c, s.th
	var dummy bool
	if s.collapsed == nil {
		s.collapsed = &dummy
	}
	if s.bp > XS {
		st := State[siderState](c, hashID(c.scope, "sider-bp"), nil)
		broken := ScreenBreakpoint(c) < s.bp
		if !st.seen || broken != st.broken {
			st.seen, st.broken = true, broken
			if *s.collapsed != broken {
				*s.collapsed = broken
				s.changed = true
				c.invalid = true
			}
		}
		s.broken = broken
	}
	collapsed := *s.collapsed
	b := siderBox(c, s.width, collapsed, s.collapsedW, func() {
		Column(c, fn, layout.Grow(1), layout.MinH(0))
		if s.trigger && s.collapsedW > 0 {
			s.triggerBar(collapsed)
		}
	}, opts)
	if s.light {
		b.Bg(th.Components.Layout.LightSiderBg)
		split := th.Split
		if shad2d(c) {
			_, split, _, _, _, _ = shad2dSidebar(th)
		}
		// The split line is on the end edge (towards the content).
		b.Paint(func(l *paint.List, r geom.Rect) {
			l.Fill(geom.Rect{X: c.inline(r).X(r.W-1, 1), Y: r.Y, W: 1, H: r.H}, geom.Radii{}, split)
		})
	}
	return b
}

// triggerBar is the 48px collapse toggle at the bottom of the sider.
func (s *SiderW) triggerBar(collapsed bool) {
	c, th := s.c, s.th
	tk := &th.Components.Layout
	loc := c.Locale()
	label := loc.CollapseMenu
	if collapsed {
		label = loc.ExpandMenu
	}
	n := c.add("sider-trigger", label)
	n.setRole(a11y.RoleButton).setState(a11y.Expandable).stateIf(!collapsed, a11y.Expanded)
	n.ln.Height = tk.TriggerHeight
	ia := c.interact(n, true, input.CursorPointer)
	if ia.Clicked {
		*s.collapsed = !*s.collapsed
		s.changed = true
		c.invalid = true
	}
	// Left (towards the start) collapses, Right expands; both mirror in
	// RTL, where the sider sits on the right.
	ic := icon.Left
	if collapsed != s.reverse {
		ic = icon.Right
	}
	light := s.light
	if shad2d(c) {
		// SidebarFooter: a ghost rounded-md button inset by p-2.
		bg, border, accent, _, fg, _ := shad2dSidebar(th)
		n.paint = func(l *paint.List, r geom.Rect) {
			l.Fill(r, geom.Radii{}, bg)
			dataHLine(l, r.X, r.Y, r.W, border)
			in := r.Inset(geom.All(8))
			if ia.Hovered || ia.Pressed {
				l.Fill(in, geom.Rad(th.Radius), accent)
			}
			c.shad2dIconPanelDir(l, geom.Rect{X: r.X + (r.W-16)/2, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, fg)
			if ia.FocusVisible {
				drawFocusRing(l, th, in, geom.Rad(th.Radius))
			}
		}
		return
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		bg, fg := tk.TriggerBg, tk.TriggerColor
		if light {
			bg, fg = tk.LightTriggerBg, tk.LightTriggerColor
		}
		l.Fill(r, geom.Radii{}, bg)
		if ia.Hovered && light {
			l.Fill(r, geom.Radii{}, th.ItemHoverBg)
		}
		if light {
			l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, th.Split)
		}
		c.drawIconAt(l, ic, r.X+(r.W-14)/2, r.Y+(r.H-14)/2, 14, fg)
		if ia.FocusVisible {
			drawFocusRing(l, th, r.Inset(geom.All(2)), geom.Rad(th.RadiusSM))
		}
	}
}

// Content is the main area; it takes the free space.
func Content(c *Context, fn func(), opts ...layout.Option) *Box {
	def := []layout.Option{layout.Grow(1), layout.MinW(0)}
	if navEl(c) {
		def = append(def, layout.Pad(20))
	}
	return layoutRegion(c, contentTag, layout.Column, def, opts, fn)
}

// Footer is the bottom bar (Ant: centred, 24px/50px padding; Element: 60px
// high, 20px padding).
func Footer(c *Context, fn func(), opts ...layout.Option) *Box {
	tk := &c.ThemeFor(theme.CompLayout).Components.Layout
	pad := layout.PadVH(tk.FooterPadding, tk.FooterPaddingInline)
	if navEl(c) {
		return layoutRegion(c, footerTag, layout.Row, []layout.Option{layout.H(60), pad, layout.Middle()}, opts, fn)
	}
	if shad2d(c) {
		b := layoutRegion(c, footerTag, layout.Row, []layout.Option{pad, layout.Center(), layout.Gap(8)}, opts, fn)
		return b.Paint(shad2dTopLine(c.Theme.BorderSecondary))
	}
	return layoutRegion(c, footerTag, layout.Row, []layout.Option{pad, layout.Center()}, opts, fn)
}

// ProLayoutConfig configures the admin shell.
type ProLayoutConfig struct {
	Title      string
	Logo       *icon.Icon
	Menu       []MenuItem
	Selected   *string
	Collapsed  *bool
	LightSider bool     // light sider (default dark, classic Pro)
	Breadcrumb []string // shown in the header
	UserName   string
	Actions    func() // extra header actions (left of the avatar)
	Footer     string
	PageTitle  string
	// Layout is "side" (default: sider with logo and menu, header with
	// the collapse toggle), "top" (header with logo and a horizontal
	// menu, no sider) or "mix" (full-width header with logo and actions
	// above a sider with the menu and a collapse trigger).
	Layout string
	// NoPadding drops the 24px padding of the content area, so a
	// PageContainer (or any page) fills it edge to edge.
	NoPadding bool
	// Breakpoint collapses the sider when the window is narrower (side
	// and mix layouts; XS = never).
	Breakpoint Breakpoint
	// SiderWidth overrides the expanded sider width (0: the theme's).
	SiderWidth float32
	// ContentWidth is "Fluid" (default) or "Fixed": the content is then
	// centred in a 1200px GridContent (Ant contentWidth).
	ContentWidth string
	// NoFixedHeader lets the header scroll away with the content (Ant
	// fixedHeader=false); by default it stays on top.
	NoFixedHeader bool
}

// ProLayout builds an Ant Design Pro style admin shell: sider with logo and
// menu, header with collapse toggle, breadcrumb and user, page title, then
// content, then footer. cfg.Layout selects the top and mix layouts.
//
// ConfigProvider.Defaults("ProLayout", ...) keys: "layout" (string),
// "noPadding" (bool), "siderWidth" (number), "breakpoint" (Breakpoint),
// "title", "footer" (string), "navTheme" ("light" | "dark"),
// "contentWidth" ("Fluid" | "Fixed"), "fixedHeader" (bool).
func ProLayout(c *Context, cfg ProLayoutConfig, content func()) {
	th := c.Theme
	if cfg.Layout == "" {
		cfg.Layout = Prop(c, "ProLayout", "layout", "side")
	}
	if !cfg.NoPadding {
		cfg.NoPadding = Prop(c, "ProLayout", "noPadding", false)
	}
	if cfg.Breakpoint == XS {
		cfg.Breakpoint = Prop(c, "ProLayout", "breakpoint", XS)
	}
	if cfg.Title == "" {
		cfg.Title = Prop(c, "ProLayout", "title", "")
	}
	if cfg.Footer == "" {
		cfg.Footer = Prop(c, "ProLayout", "footer", "")
	}
	if !cfg.LightSider {
		cfg.LightSider = Prop(c, "ProLayout", "navTheme", "") == "light"
	}
	if cfg.ContentWidth == "" {
		cfg.ContentWidth = Prop(c, "ProLayout", "contentWidth", "")
	}
	if !cfg.NoFixedHeader {
		cfg.NoFixedHeader = !Prop(c, "ProLayout", "fixedHeader", true)
	}
	pl := &proLayout{c: c, th: c.ThemeFor(theme.CompLayout), cfg: cfg, content: content}
	// shadcn has a single (light) Sidebar look driven by sidebar tokens.
	pl.dark = !cfg.LightSider && th.Flavor != theme.StyleShadcn
	switch cfg.Layout {
	case "top":
		pl.top()
	case "mix":
		pl.mix()
	default:
		if th.Flavor == theme.StyleShadcn {
			pl.shadSide()
			return
		}
		pl.side()
	}
}

type proLayout struct {
	c            *Context
	th           *theme.Theme // Layout tokens (c.ThemeFor(theme.CompLayout))
	cfg          ProLayoutConfig
	content      func()
	dark         bool
	scrollHeader func() // NoFixedHeader: header built inside the scroll
}

func (pl *proLayout) collapsed() bool { return pl.cfg.Collapsed != nil && *pl.cfg.Collapsed }

// siderWidth is the expanded sider width: ProLayoutConfig.SiderWidth, the
// "siderWidth" default, else the theme's.
func (pl *proLayout) siderWidth() float32 {
	if pl.cfg.SiderWidth > 0 {
		return pl.cfg.SiderWidth
	}
	if v, ok := p2bNum(pl.c, "ProLayout", "siderWidth"); ok && v > 0 {
		return float32(v)
	}
	return pl.th.Components.Layout.SiderWidth
}

// breakpoint collapses the sider when the window crosses cfg.Breakpoint.
func (pl *proLayout) breakpoint() {
	c := pl.c
	if pl.cfg.Breakpoint <= XS || pl.cfg.Collapsed == nil {
		return
	}
	st := State[siderState](c, hashID(c.scope, "prolayout-bp"), nil)
	broken := ScreenBreakpoint(c) < pl.cfg.Breakpoint
	if !st.seen || broken != st.broken {
		st.seen, st.broken = true, broken
		if *pl.cfg.Collapsed != broken {
			*pl.cfg.Collapsed = broken
			c.invalid = true
		}
	}
}

// logo builds the logo mark and title (title hidden when collapsed).
func (pl *proLayout) logo(collapsed, onDark bool) {
	c, th := pl.c, pl.th
	Row(c, func() {
		logo := c.add("logo", "")
		logo.ln.Width, logo.ln.Height = 32, 32
		if pl.cfg.Logo != nil {
			ic := pl.cfg.Logo
			logo.paint = func(l *paint.List, r geom.Rect) { c.drawIconAt(l, ic, r.X+4, r.Y+4, 24, th.Primary.Base) }
		} else {
			logo.paint = func(l *paint.List, r geom.Rect) { drawLogo(l, r, th) }
		}
		if !collapsed {
			t := Text(c, pl.cfg.Title).Size(18).Weight(text.Semibold).NoWrap()
			if onDark {
				t.Color(theme.Hex("#ffffff"))
			}
		}
	}, layout.H(64), layout.PadVH(0, 16), layout.Gap(12), layout.Middle())
}

// user builds the actions and the avatar with the user name.
func (pl *proLayout) user(onDark bool) {
	c, th := pl.c, pl.th
	if pl.cfg.Actions != nil {
		pl.cfg.Actions()
	}
	if pl.cfg.UserName != "" {
		Row(c, func() {
			Avatar(c, string([]rune(pl.cfg.UserName)[:1])).Size(24).Bg(th.Primary.Base)
			t := Text(c, pl.cfg.UserName).NoWrap()
			if onDark {
				t.Color(theme.Hex("#ffffff"))
			}
		}, layout.Gap(8), layout.Middle(), layout.PadVH(0, 8))
	}
}

// proLayoutCtx marks the content area of a ProLayout being built: a
// PageContainer inside it reports itself, and the content area then drops
// its padding (NoPadding) so the page header spans it edge to edge.
type proLayoutCtx struct {
	depth  int
	filled bool // a PageContainer was built directly in the content area
	info   RouteInfo
}

var proLayoutCtxKey = hashID("prolayout-content-ctx")

func proLayoutContext(c *Context) *proLayoutCtx { return State[proLayoutCtx](c, proLayoutCtxKey, nil) }

// main builds the scrolling content area (breadcrumb on top for the top
// and mix layouts, page title, content, footer).
func (pl *proLayout) main(crumbs bool) {
	c := pl.c
	pad := layout.Pad(24)
	if pl.cfg.NoPadding {
		pad = layout.Pad(0)
	}
	pctx := proLayoutContext(c)
	saved := *pctx
	pctx.depth, pctx.filled = pctx.depth+1, false
	pctx.info = pl.routeInfo()
	inner := func() {
		if crumbs && len(pl.cfg.Breadcrumb) > 0 {
			Row(c, func() { Breadcrumb(c, pl.cfg.Breadcrumb...) }, layout.Margin(geom.Insets{Bottom: 16}))
		}
		if pl.cfg.PageTitle != "" {
			Title(c, pl.cfg.PageTitle, 4).Layout(layout.Margin(geom.Insets{Bottom: 16}))
		}
		if pl.cfg.ContentWidth == "Fixed" {
			GridContent(c).ContentWidth("Fixed").Body(pl.content)
		} else {
			pl.content()
		}
		if pl.cfg.Footer != "" {
			Row(c, func() { Text(c, pl.cfg.Footer).Tertiary() }, layout.PadVH(24, 0), layout.Center())
		}
	}
	var padded *Box
	if pl.scrollHeader != nil {
		// NoFixedHeader: the header scrolls with the content.
		Scroll(c, func() {
			pl.scrollHeader()
			padded = Column(c, inner, pad, layout.Grow(1), layout.MinW(0))
		}, layout.Grow(1))
	} else {
		padded = Scroll(c, inner, layout.Grow(1), pad).Box
	}
	pctx = proLayoutContext(c)
	if pctx.filled {
		// A PageContainer fills the area itself (same as NoPadding).
		padded.n.ln.Padding = geom.Insets{}
	}
	*pctx = saved
}

// routeInfo describes the shell for RouteContext.
func (pl *proLayout) routeInfo() RouteInfo {
	cfg := pl.cfg
	ri := RouteInfo{InLayout: true, Layout: cfg.Layout, Collapsed: pl.collapsed(), Breadcrumb: cfg.Breadcrumb,
		PageTitle: cfg.PageTitle, ContentWidth: cfg.ContentWidth, FixedHeader: !cfg.NoFixedHeader,
		HasSiderMenu: cfg.Layout != "top", IsMobile: ScreenBreakpoint(pl.c) < MD}
	if ri.ContentWidth == "" {
		ri.ContentWidth = "Fluid"
	}
	if cfg.Selected != nil {
		ri.Selected = *cfg.Selected
	}
	return ri
}

func (pl *proLayout) side() {
	c, th := pl.c, pl.th
	tk := &th.Components.Layout
	cfg := pl.cfg
	pl.breakpoint()
	collapsed := pl.collapsed()
	dark := pl.dark
	siderBg := tk.SiderBg
	if !dark {
		siderBg = tk.LightSiderBg
	}
	Layout(c, func() {
		sider := Sider(c, pl.siderWidth(), collapsed, func() {
			pl.logo(collapsed, dark)
			Scroll(c, func() {
				m := Menu(c, cfg.Menu, cfg.Selected).Collapsed(collapsed)
				if dark {
					m.Dark()
				}
				m.Show()
			}, layout.Grow(1))
		})
		sider.Bg(siderBg)
		if !dark {
			// Split line on the end edge (towards the content).
			sider.Paint(func(l *paint.List, r geom.Rect) {
				l.Fill(geom.Rect{X: c.inline(r).X(r.W-1, 1), Y: r.Y, W: 1, H: r.H}, geom.Radii{}, th.Split)
			})
		} else {
			// The shadow falls towards the content.
			sx := float32(2)
			if c.IsRTL() {
				sx = -2
			}
			sider.Shadow([]paint.Shadow{{X: sx, Blur: 8, Color: theme.RGBA(29, 35, 41, .05)}})
		}
		Column(c, func() {
			header := func() {
				hdr := Row(c, func() {
					ic := icon.MenuFold
					if collapsed {
						ic = icon.MenuUnfold
					}
					label := c.Locale().CollapseMenu
					if collapsed {
						label = c.Locale().ExpandMenu
					}
					ib := IconButton(c, ic, label).Size(32, 16).Color(th.Text)
					ib.n.setState(a11y.Expandable).stateIf(!collapsed, a11y.Expanded) // the sider
					if ib.Clicked() && cfg.Collapsed != nil {
						*cfg.Collapsed = !*cfg.Collapsed
						c.invalid = true
					}
					if len(cfg.Breadcrumb) > 0 {
						Gap(c, 8)
						Breadcrumb(c, cfg.Breadcrumb...)
					}
					Spacer(c)
					pl.user(false)
				}, layout.H(tk.ProHeaderHeight), layout.PadVH(0, 16), layout.Gap(8), layout.Middle())
				hdr.Bg(tk.ProHeaderBg).Paint(func(l *paint.List, r geom.Rect) {
					l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.Split)
				})
			}
			if cfg.NoFixedHeader {
				pl.scrollHeader = header
			} else {
				header()
			}
			pl.main(false)
		}, layout.Grow(1), layout.MinW(0))
	})
}

// header builds the full-width header of the top and mix layouts.
func (pl *proLayout) header(menu bool) {
	c, th := pl.c, pl.th
	tk := &th.Components.Layout
	cfg := pl.cfg
	dark := pl.dark
	hdr := Row(c, func() {
		pl.logo(false, dark)
		if menu {
			Row(c, func() {
				m := Menu(c, cfg.Menu, cfg.Selected).Mode(MenuHorizontal)
				if dark {
					m.Dark()
				}
				m.Show()
			}, layout.Grow(1), layout.MinW(0), layout.Margin(geom.Insets{Left: 24}))
		} else {
			Spacer(c)
		}
		pl.user(dark)
	}, layout.H(tk.ProHeaderHeight), layout.PadVH(0, 16), layout.Gap(8), layout.Middle())
	if dark {
		hdr.Bg(tk.SiderBg)
	} else {
		hdr.Bg(tk.ProHeaderBg).Paint(func(l *paint.List, r geom.Rect) {
			l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.Split)
		})
	}
	if menu {
		// The horizontal menu fills the header height.
		for _, k := range hdr.n.kids {
			if k.ln.Grow == 1 && len(k.kids) == 1 {
				k.ln.Align = layout.AlignStretch
				k.ln.Height = tk.ProHeaderHeight
			}
		}
	}
}

func (pl *proLayout) top() {
	c := pl.c
	Layout(c, func() {
		header := func() {
			if shad2d(c) {
				pl.shadHeader(true)
			} else {
				pl.header(true)
			}
		}
		if pl.cfg.NoFixedHeader {
			pl.scrollHeader = header
		} else {
			header()
		}
		Column(c, func() { pl.main(true) }, layout.Grow(1), layout.MinH(0))
	})
}

func (pl *proLayout) mix() {
	c := pl.c
	cfg := pl.cfg
	var local bool
	coll := cfg.Collapsed
	if coll == nil {
		coll = &local
	}
	Layout(c, func() {
		if shad2d(c) {
			pl.shadHeader(false)
		} else {
			pl.header(false)
		}
		Layout(c, func() {
			s := CollapsibleSider(c, pl.siderWidth(), coll).Light()
			if cfg.Breakpoint > XS {
				s.Breakpoint(cfg.Breakpoint)
			}
			s.Body(func() {
				Scroll(c, func() { Menu(c, cfg.Menu, cfg.Selected).Collapsed(*coll).Show() }, layout.Grow(1), layout.Pad(0))
			})
			Column(c, func() { pl.main(true) }, layout.Grow(1), layout.MinW(0))
		})
	})
}

// drawLogo paints the framework mark: two overlapping rounded diamonds.
func drawLogo(l *paint.List, r geom.Rect, th *theme.Theme) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	s := r.W / 32
	a := paint.NewPath().MoveTo(cx-6*s, cy-11*s).LineTo(cx+5*s, cy).LineTo(cx-6*s, cy+11*s).LineTo(cx-15*s, cy+2*s).LineTo(cx-15*s, cy-2*s).Close()
	b := paint.NewPath().MoveTo(cx+6*s, cy-11*s).LineTo(cx+15*s, cy-2*s).LineTo(cx+15*s, cy+2*s).LineTo(cx+6*s, cy+11*s).LineTo(cx-5*s, cy).Close()
	l.Path(a, th.Primary.Base)
	l.Path(b, th.Primary.Shades[3])
}
