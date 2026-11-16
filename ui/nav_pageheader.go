package ui

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

// PageHeaderW is the page title bar: back action, breadcrumb, title,
// subtitle, tags, extra actions, content and footer (tabs).
type PageHeaderW struct {
	_ [0]func()

	c          *Context
	title      string
	sub        string
	back       bool
	backText   string
	crumbs     []string
	crumbItems []BreadcrumbItem
	avatar     func()
	backIcon   *icon.Icon
	tags       func()
	extra      func()
	content    func()
	footer     func()
	ghost      bool
	pad        *geom.Insets
	backHit    bool
	crumbClick int
}

// pageTitleLevel makes the page header title a level-1 heading.
var pageTitleLevel = semFunc(func(s *a11y.Node) { s.Level = 1 })

// PageHeader starts a page header; chain options, then Show. Sizes and
// colors come from theme.PageHeaderToken (Pro Components v3 PageHeader,
// Element el-page-header); a Bordered header is the container color, as
// Pro's non-ghost header.
func PageHeader(c *Context, title string) *PageHeaderW {
	// ConfigProvider.Defaults("PageHeader", ...) keys: "ghost" (bool).
	return &PageHeaderW{c: c, title: title, ghost: Prop(c, "PageHeader", "ghost", true), crumbClick: -1}
}

// SubTitle sets the secondary title.
func (p *PageHeaderW) SubTitle(s string) *PageHeaderW { p.sub = s; return p }

// Back shows the back button.
func (p *PageHeaderW) Back() *PageHeaderW { p.back = true; return p }

// BackText overrides the Element back label (default Locale().Back).
func (p *PageHeaderW) BackText(s string) *PageHeaderW { p.back, p.backText = true, s; return p }

// Breadcrumb shows a breadcrumb above the title.
func (p *PageHeaderW) Breadcrumb(items ...string) *PageHeaderW { p.crumbs = items; return p }

// BreadcrumbItems shows a structured breadcrumb above the title (Pro
// breadcrumb {items}); CrumbClicked reports clicks on it.
func (p *PageHeaderW) BreadcrumbItems(items []BreadcrumbItem) *PageHeaderW {
	p.crumbItems = items
	return p
}

// Avatar builds an avatar before the title (Pro avatar), e.g.
// func() { ui.Avatar(c, "U") }.
func (p *PageHeaderW) Avatar(fn func()) *PageHeaderW { p.avatar = fn; return p }

// BackIcon replaces the back arrow and shows the back button (Pro
// backIcon, Element icon).
func (p *PageHeaderW) BackIcon(ic *icon.Icon) *PageHeaderW { p.back, p.backIcon = true, ic; return p }

// Tags adds tags after the title.
func (p *PageHeaderW) Tags(fn func()) *PageHeaderW { p.tags = fn; return p }

// Extra adds actions at the right.
func (p *PageHeaderW) Extra(fn func()) *PageHeaderW { p.extra = fn; return p }

// Content adds the description area below the title.
func (p *PageHeaderW) Content(fn func()) *PageHeaderW { p.content = fn; return p }

// Footer adds a footer (usually Tabs).
func (p *PageHeaderW) Footer(fn func()) *PageHeaderW { p.footer = fn; return p }

// Bordered draws the container background (Pro ghost=false; a bottom
// line only when PageHeaderToken.BorderColor is set).
func (p *PageHeaderW) Bordered() *PageHeaderW { p.ghost = false; return p }

// Ghost toggles the transparent background (Pro ghost, default true).
func (p *PageHeaderW) Ghost(on bool) *PageHeaderW { p.ghost = on; return p }

// Padding overrides the header padding (Ant 16px 24px, Element none),
// e.g. to align it with a surrounding container.
func (p *PageHeaderW) Padding(in geom.Insets) *PageHeaderW { p.pad = &in; return p }

// BackClicked reports a click on the back button.
func (p *PageHeaderW) BackClicked() bool { return p.backHit }

// CrumbClicked returns the clicked breadcrumb index or -1.
func (p *PageHeaderW) CrumbClicked() int { return p.crumbClick }

func (p *PageHeaderW) backButton(tk *theme.PageHeaderToken) {
	c, th := p.c, p.c.Theme
	el := navEl(c) // structural: Element shows an arrow + label, Ant / shadcn an icon button
	ic := icon.ArrowLeft
	if p.backIcon != nil {
		ic = p.backIcon
	}
	is := tk.BackIconSize
	label := p.backText
	if label == "" {
		label = c.Locale().Back
	}
	n := c.add("page-back", label)
	n.setRole(a11y.RoleButton)
	n.ln.NoStretch = true
	st := c.style(th.FontSize, text.Regular)
	n.ln.Measure = func(_, _ float32) geom.Size {
		if el {
			return geom.Size{W: is + tk.BackIconGap + text.Width(label, st), H: 24}
		}
		return geom.Size{W: 24, H: 24}
	}
	n.ln.AlignSelf = layout.AlignCenter
	ia := c.interact(n, true, input.CursorPointer)
	if ia.Clicked {
		p.backHit = true
	}
	id := n.id
	n.paint = func(l *paint.List, r geom.Rect) {
		h := c.Anim(id, "hover", b2f(ia.Hovered), th.DurationMid)
		if el {
			col := tk.BackColor
			if h > 0.5 {
				col = th.Primary.Base
			}
			// Arrow at the start, pointing back (mirrored in RTL), then the label.
			s := c.inline(r)
			c.paintIcon(l, ic, s.Rect(0, r.Y+(r.H-is)/2, is, is), col, 0)
			lay := navLabel(c, label, th.FontSize, text.Medium)
			l.Glyphs(geom.Pt(s.X(is+tk.BackIconGap, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
		} else {
			// Pro: a small text Button with the icon.
			col := tk.BackColor
			if h > 0 {
				l.Fill(r, geom.Rad(th.RadiusSM), theme.Fade(th.BgTextHover, float64(h)))
			}
			c.paintIcon(l, ic, geom.Rect{X: r.X + (r.W-is)/2, Y: r.Y + (r.H-is)/2, W: is, H: is}, col, 0)
		}
		if ia.FocusVisible {
			l.Stroke(r.Expand(2), geom.Rad(th.RadiusSM), 2, th.FocusRing)
		}
	}
}

// Show builds the header.
func (p *PageHeaderW) Show() *PageHeaderW {
	c, th := p.c, p.c.ThemeFor(theme.CompPageHeader)
	tk := &th.Components.PageHeader
	el := navEl(c) // structural: the Element divider after the back label
	hasCrumb := len(p.crumbs) > 0 || len(p.crumbItems) > 0
	pad := geom.VH(tk.PaddingBlock, tk.PaddingInline)
	if hasCrumb {
		pad.Top = tk.PaddingTopBreadcrumb
	}
	if p.footer != nil {
		pad.Bottom = 0 // Pro has-footer
	}
	if p.pad != nil {
		pad = *p.pad
	}
	b := Column(c, func() {
		if hasCrumb {
			var ci int
			if len(p.crumbItems) > 0 {
				ci = BreadcrumbItems(c, p.crumbItems).Show().Clicked()
			} else {
				ci = Breadcrumb(c, p.crumbs...).Clicked()
			}
			if ci >= 0 {
				p.crumbClick = ci
			}
			Gap(c, tk.BreadcrumbGap)
		}
		Row(c, func() {
			Row(c, func() {
				if p.back {
					p.backButton(tk)
					if el {
						d := c.add("page-divider", "")
						d.ln.Width, d.ln.Height = 1, 16
						d.ln.Margin = geom.Insets{Left: tk.BackGap, Right: tk.BackGap}
						d.ln.AlignSelf = layout.AlignCenter
						d.aHidden = true
						d.paint = func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, th.Border) }
					} else {
						Gap(c, tk.BackGap)
					}
				}
				if p.avatar != nil {
					Row(c, p.avatar, layout.Middle(), layout.Margin(geom.Insets{Right: tk.TitleMarginEnd}))
				}
				t := Text(c, p.title).NoWrap().Ellipsis().Size(tk.TitleFontSize).LineHeight(tk.TitleLineHeight).Color(tk.TitleColor)
				t.n.setRole(a11y.RoleHeading).fill(pageTitleLevel)
				if tk.TitleFontWeight >= 600 {
					t.Strong()
				}
				if p.sub != "" {
					s := Text(c, p.sub).NoWrap().Ellipsis().Size(tk.SubTitleFontSize).Color(tk.SubTitleColor)
					s.Layout(layout.Margin(geom.Insets{Left: tk.TitleMarginEnd}))
				}
				if p.tags != nil {
					Row(c, p.tags, layout.Gap(8), layout.Middle(), layout.Margin(geom.Insets{Left: tk.TitleMarginEnd}))
				}
			}, layout.Middle(), layout.Grow(1), layout.MinW(0))
			if p.extra != nil {
				Row(c, p.extra, layout.Gap(8), layout.Middle(), layout.Margin(geom.Insets{Left: 16}))
			}
		}, layout.Middle(), layout.Margin(geom.Insets{Top: tk.HeadingMarginBlock, Bottom: tk.HeadingMarginBlock}))
		if p.content != nil {
			box := Column(c, p.content, layout.Margin(geom.Insets{Top: tk.ContentMarginTop}), layout.Padding(geom.Insets{Top: tk.ContentPaddingTop}))
			if col := tk.ContentBorderColor; col.A > 0 {
				box.Paint(func(l *paint.List, r geom.Rect) { navHLine(l, r.X, r.Y, r.W, col) })
			}
		}
		if p.footer != nil {
			Column(c, p.footer, layout.Margin(geom.Insets{Top: tk.FooterMarginTop}))
		}
	}, layout.Padding(pad))
	if !p.ghost {
		b.Bg(tk.Bg)
		if col := tk.BorderColor; col.A > 0 {
			b.Paint(func(l *paint.List, r geom.Rect) { navHLine(l, r.X, r.Bottom()-1, r.W, col) })
		}
	}
	return p
}
