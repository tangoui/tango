package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// shadcn Sidebar-block shell of ProLayout: a sidebar surface with a brand
// row (rounded-lg primary square + semibold name), the menu, and the user
// row at the bottom; the inset column holds a header with the sidebar
// trigger, a vertical separator and the breadcrumb. Collapsed, the sidebar
// becomes a 48px icon rail.

// shadSide is the "side" layout.
func (pl *proLayout) shadSide() {
	c, th := pl.c, pl.th
	tk := &th.Components.Layout
	cfg := pl.cfg
	pl.breakpoint()
	collapsed := pl.collapsed()
	bg, _, _, _, _, _ := shad2dSidebar(th)
	Layout(c, func() {
		sider := Sider(c, pl.siderWidth(), collapsed, func() {
			pl.shadBrand(collapsed)
			Scroll(c, func() {
				Menu(c, cfg.Menu, cfg.Selected).Collapsed(collapsed).Show()
			}, layout.Grow(1))
			if cfg.UserName != "" {
				pl.shadUser(collapsed)
			}
		})
		sider.Bg(bg)
		Column(c, func() {
			header := func() {
				hdr := Row(c, func() {
					if cfg.Collapsed != nil {
						if shad2dTrigger(c, collapsed) {
							*cfg.Collapsed = !*cfg.Collapsed
							c.invalid = true
						}
					}
					if len(cfg.Breadcrumb) > 0 {
						if cfg.Collapsed != nil {
							Div(c, layout.W(1), layout.H(16), layout.Margin(geom.Insets{Left: 4, Right: 4})).Bg(th.BorderSecondary)
						}
						Breadcrumb(c, cfg.Breadcrumb...)
					}
					Spacer(c)
					if cfg.Actions != nil {
						cfg.Actions()
					}
				}, layout.H(tk.ProHeaderHeight), layout.PadVH(0, 16), layout.Gap(8), layout.Middle())
				hdr.Bg(tk.ProHeaderBg).Paint(shad2dBottomLine(th.BorderSecondary))
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

// shadHeader is the full-width header of the top and mix layouts: brand,
// optional horizontal menu, actions and the user avatar, on the page
// background with a bottom border.
func (pl *proLayout) shadHeader(menu bool) {
	c, th := pl.c, pl.th
	tk := &th.Components.Layout
	cfg := pl.cfg
	hdr := Row(c, func() {
		Row(c, func() { pl.shadBrandMark(false) }, layout.Gap(8), layout.Middle())
		if menu {
			Row(c, func() {
				Menu(c, cfg.Menu, cfg.Selected).Mode(MenuHorizontal).Show()
			}, layout.Grow(1), layout.MinW(0), layout.Margin(geom.Insets{Left: 24}))
		} else {
			Spacer(c)
		}
		if cfg.Actions != nil {
			cfg.Actions()
		}
		if cfg.UserName != "" {
			Avatar(c, string([]rune(cfg.UserName)[:1])).Size(32).A11yLabel(cfg.UserName)
		}
	}, layout.H(tk.ProHeaderHeight), layout.PadVH(0, 16), layout.Gap(12), layout.Middle())
	hdr.Bg(tk.ProHeaderBg).Paint(shad2dBottomLine(th.BorderSecondary))
	if menu {
		for _, k := range hdr.n.kids {
			if k.ln.Grow == 1 && len(k.kids) == 1 {
				k.ln.Align = layout.AlignStretch
				k.ln.Height = tk.ProHeaderHeight
			}
		}
	}
}

// shadBrand is the SidebarHeader: p-2 around a 48px brand row.
func (pl *proLayout) shadBrand(collapsed bool) {
	c := pl.c
	pad := layout.Pad(8)
	Column(c, func() {
		Row(c, func() { pl.shadBrandMark(collapsed) }, layout.H(48), layout.PadVH(0, map[bool]float32{true: 0, false: 8}[collapsed]),
			layout.Gap(8), layout.Middle())
	}, pad)
}

// shadBrandMark is the rounded-lg primary square with the logo and, when
// expanded, the title.
func (pl *proLayout) shadBrandMark(collapsed bool) {
	c, th := pl.c, pl.th
	pr, prFg := shad2dPrimary(th)
	if v := th.Shadcn; v != nil {
		pr, prFg = v.SidebarPrimary, v.SidebarPrimaryForeground
	}
	logo := c.add("logo", "")
	logo.ln.Width, logo.ln.Height = 32, 32
	logo.ln.NoStretch = true
	ic := pl.cfg.Logo
	rad := th.RadiusLG
	logo.paint = func(l *paint.List, r geom.Rect) {
		l.Fill(r, geom.Rad(rad), pr)
		if ic != nil {
			c.drawIconAt(l, ic, r.X+8, r.Y+8, 16, prFg)
			return
		}
		shad2dLogo(l, r.Inset(geom.All(7)), prFg)
	}
	if !collapsed && pl.cfg.Title != "" {
		Text(c, pl.cfg.Title).Weight(text.Semibold).NoWrap().Ellipsis().Layout(layout.MinW(0))
	}
}

// shadUser is the SidebarFooter user row (NavUser): avatar, name and the
// up-down chevrons; only the avatar on the icon rail.
func (pl *proLayout) shadUser(collapsed bool) {
	c, th := pl.c, pl.th
	_, _, accent, _, fg, muted := shad2dSidebar(th)
	name := pl.cfg.UserName
	Column(c, func() {
		b := Row(c, func() {
			Avatar(c, string([]rune(name)[:1])).Size(32).Square().A11yLabel(name)
			if !collapsed {
				Text(c, name).Weight(text.Medium).Color(fg).NoWrap().Ellipsis().Layout(layout.MinW(0), layout.Grow(1))
				shad2dIconNode(c, 16, shad2dIconChevrons, muted)
			}
		}, layout.H(48), layout.PadVH(0, map[bool]float32{true: 0, false: 8}[collapsed]), layout.Gap(8), layout.Middle())
		b.Radius(th.Radius)
		if b.Hovered() {
			b.Bg(accent)
		}
	}, layout.Pad(8))
}

// shad2dTrigger is the ghost 28px SidebarTrigger (panel icon); it
// reports a click.
func shad2dTrigger(c *Context, collapsed bool) bool {
	th := c.Theme
	loc := c.Locale()
	label := loc.CollapseMenu
	if collapsed {
		label = loc.ExpandMenu
	}
	n := c.add("sidebar-trigger", label)
	n.setRole(a11y.RoleButton).setState(a11y.Expandable).stateIf(!collapsed, a11y.Expanded)
	n.ln.Width, n.ln.Height = 28, 28
	n.ln.NoStretch = true
	ia := c.interact(n, true, input.CursorPointer)
	_, _, accent, _, _, _ := shad2dSidebar(th)
	if v := th.Shadcn; v != nil {
		accent = v.Accent
	}
	fg := th.Text
	n.paint = func(l *paint.List, r geom.Rect) {
		if ia.Hovered || ia.Pressed {
			l.Fill(r, geom.Rad(th.Radius), accent)
		}
		c.shad2dIconPanelDir(l, geom.Rect{X: r.X + 6, Y: r.Y + 6, W: 16, H: 16}, fg)
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Rad(th.Radius))
		}
	}
	return ia.Clicked
}

// shad2dLogo paints the framework mark (two diamonds) in one color.
func shad2dLogo(l *paint.List, r geom.Rect, col theme.Color) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	s := r.W / 32
	a := paint.NewPath().MoveTo(cx-6*s, cy-11*s).LineTo(cx+5*s, cy).LineTo(cx-6*s, cy+11*s).LineTo(cx-15*s, cy+2*s).LineTo(cx-15*s, cy-2*s).Close()
	b := paint.NewPath().MoveTo(cx+6*s, cy-11*s).LineTo(cx+15*s, cy-2*s).LineTo(cx+15*s, cy+2*s).LineTo(cx+6*s, cy+11*s).LineTo(cx-5*s, cy).Close()
	l.Path(a, col)
	l.Path(b, theme.Fade(col, .6))
}
