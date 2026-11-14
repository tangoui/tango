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

// PageHeader starts a page header; chain options, then Show.
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

// Tags adds tags after the title.
func (p *PageHeaderW) Tags(fn func()) *PageHeaderW { p.tags = fn; return p }

// Extra adds actions at the right.
func (p *PageHeaderW) Extra(fn func()) *PageHeaderW { p.extra = fn; return p }

// Content adds the description area below the title.
func (p *PageHeaderW) Content(fn func()) *PageHeaderW { p.content = fn; return p }

// Footer adds a footer (usually Tabs).
func (p *PageHeaderW) Footer(fn func()) *PageHeaderW { p.footer = fn; return p }

// Bordered draws a container background and bottom border (Ant ghost=false).
func (p *PageHeaderW) Bordered() *PageHeaderW { p.ghost = false; return p }

// Padding overrides the header padding (Ant 16px 24px, Element none),
// e.g. to align it with a surrounding container.
func (p *PageHeaderW) Padding(in geom.Insets) *PageHeaderW { p.pad = &in; return p }

// BackClicked reports a click on the back button.
func (p *PageHeaderW) BackClicked() bool { return p.backHit }

// CrumbClicked returns the clicked breadcrumb index or -1.
func (p *PageHeaderW) CrumbClicked() int { return p.crumbClick }

func (p *PageHeaderW) backButton() {
	c, th := p.c, p.c.Theme
	el := navEl(c)
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
			return geom.Size{W: 16 + 8 + text.Width(label, st), H: 24}
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
			col := th.Text
			if h > 0.5 {
				col = th.Primary.Base
			}
			// Arrow at the start, pointing back (mirrored in RTL), then the label.
			s := c.inline(r)
			c.paintIcon(l, icon.ArrowLeft, s.Rect(0, r.Y+(r.H-16)/2, 16, 16), col, 0)
			lay := navLabel(c, label, th.FontSize, text.Medium)
			l.Glyphs(geom.Pt(s.X(24, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
		} else {
			col := th.Text
			if h > 0 {
				l.Fill(r, geom.Rad(th.RadiusSM), theme.Fade(th.BgTextHover, float64(h)))
			}
			c.paintIcon(l, icon.ArrowLeft, geom.Rect{X: r.X + 4, Y: r.Y + 4, W: 16, H: 16}, col, 0)
		}
		if ia.FocusVisible {
			l.Stroke(r.Expand(2), geom.Rad(th.RadiusSM), 2, th.FocusRing)
		}
	}
}

// Show builds the header.
func (p *PageHeaderW) Show() *PageHeaderW {
	c, th := p.c, p.c.Theme
	el := navEl(c)
	pad := geom.VH(16, 24)
	if el {
		pad = geom.Insets{}
	}
	if p.pad != nil {
		pad = *p.pad
	}
	b := Column(c, func() {
		if len(p.crumbs) > 0 {
			ci := Breadcrumb(c, p.crumbs...).Clicked()
			if ci >= 0 {
				p.crumbClick = ci
			}
			Gap(c, map[bool]float32{true: 16, false: 8}[el])
		}
		Row(c, func() {
			Row(c, func() {
				if p.back {
					p.backButton()
					if el {
						d := c.add("page-divider", "")
						d.ln.Width, d.ln.Height = 1, 16
						d.ln.Margin = geom.Insets{Left: 16, Right: 16}
						d.ln.AlignSelf = layout.AlignCenter
						d.aHidden = true
						d.paint = func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, th.Border) }
					} else {
						Gap(c, 12)
					}
				}
				t := Text(c, p.title).NoWrap().Ellipsis()
				t.n.setRole(a11y.RoleHeading).fill(pageTitleLevel)
				if el {
					t.Size(18).LineHeight(28)
				} else {
					t.Size(th.FontSizeXL).LineHeight(32).Strong()
				}
				if p.sub != "" {
					s := Text(c, p.sub).NoWrap().Ellipsis().Color(th.TextTertiary)
					s.Layout(layout.Margin(geom.Insets{Left: 12}))
					if el {
						s.Color(th.TextSecondary)
					}
				}
				if p.tags != nil {
					Row(c, p.tags, layout.Gap(8), layout.Middle(), layout.Margin(geom.Insets{Left: 12}))
				}
			}, layout.Middle(), layout.Grow(1), layout.MinW(0))
			if p.extra != nil {
				Row(c, p.extra, layout.Gap(8), layout.Middle(), layout.Margin(geom.Insets{Left: 16}))
			}
		}, layout.Middle())
		if p.content != nil {
			box := Column(c, p.content, layout.Margin(geom.Insets{Top: 12}))
			if el {
				box.n.ln.Margin.Top = 16
				box.n.ln.Padding.Top = 16
				box.Paint(func(l *paint.List, r geom.Rect) { navHLine(l, r.X, r.Y, r.W, th.BorderSecondary) })
			}
		}
		if p.footer != nil {
			Column(c, p.footer, layout.Margin(geom.Insets{Top: 16}))
		}
	}, layout.Padding(pad))
	if !p.ghost {
		b.Bg(th.BgContainer)
		b.Paint(func(l *paint.List, r geom.Rect) { navHLine(l, r.X, r.Bottom()-1, r.W, th.Split) })
	}
	return p
}
