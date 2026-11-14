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

// ---------------------------------------------------------------------------
// CheckCard / CheckCardGroup (Ant Design Pro CheckCard)
//
//	ui.CheckCard(c, "Option", &on).Description("…").Avatar("A").Show()
//	ui.CheckCardGroup(c, opts).Value(&key).Show()      // single choice
//	ui.CheckCardGroup(c, opts).Values(&keys).Show()    // multiple choice

// CheckCardW builds a checkable card.
type CheckCardW struct {
	_ [0]func()

	c        *Context
	title    string
	checked  *bool
	desc     string
	avatar   string
	avIcon   *icon.Icon
	extra    func()
	cover    func()
	disabled bool
	size     ControlSize
	loading  bool
	bordered bool
	width    float32
	role     a11y.Role
	changed  bool
	ia       Interaction
	id       ID
}

// CheckCard starts a checkable card bound to checked; chain options, then
// Show.
func CheckCard(c *Context, title string, checked *bool) *CheckCardW {
	w := &CheckCardW{c: c, title: title, checked: checked, size: c.defaultSize(), bordered: true, role: a11y.RoleCheckbox}
	// ConfigProvider.Defaults("CheckCard", ...) keys: "size", "bordered".
	if sz, ok := proPropSize(c, "CheckCard"); ok {
		w.size = sz
	}
	w.bordered = Prop(c, "CheckCard", "bordered", true)
	return w
}

// Description sets the secondary text under the title.
func (w *CheckCardW) Description(s string) *CheckCardW { w.desc = s; return w }

// Avatar shows a text avatar (first character) left of the title.
func (w *CheckCardW) Avatar(s string) *CheckCardW { w.avatar = s; return w }

// AvatarIcon shows an icon avatar left of the title.
func (w *CheckCardW) AvatarIcon(ic *icon.Icon) *CheckCardW { w.avIcon = ic; return w }

// Extra builds content at the top right.
func (w *CheckCardW) Extra(fn func()) *CheckCardW { w.extra = fn; return w }

// Cover replaces the content with fn (an image or any block).
func (w *CheckCardW) Cover(fn func()) *CheckCardW { w.cover = fn; return w }

// Disabled disables the card.
func (w *CheckCardW) Disabled(on bool) *CheckCardW { w.disabled = on; return w }

// Size sets the size: Small (212px), Middle (320px) or Large (440px).
func (w *CheckCardW) Size(s ControlSize) *CheckCardW { w.size = s; return w }

// Small is Size(Small).
func (w *CheckCardW) Small() *CheckCardW { w.size = Small; return w }

// Large is Size(Large).
func (w *CheckCardW) Large() *CheckCardW { w.size = Large; return w }

// Loading shows a skeleton instead of the content.
func (w *CheckCardW) Loading(on bool) *CheckCardW { w.loading = on; return w }

// Borderless removes the border (the checked state still colors it).
func (w *CheckCardW) Borderless() *CheckCardW { w.bordered = false; return w }

// Width overrides the width of the size.
func (w *CheckCardW) Width(px float32) *CheckCardW { w.width = px; return w }

// Changed reports a toggle by the user this frame (after Show).
func (w *CheckCardW) Changed() bool { return w.changed }

// Clicked reports a click this frame (after Show).
func (w *CheckCardW) Clicked() bool { return w.ia.Clicked }

// ID returns the card's widget ID (after Show).
func (w *CheckCardW) ID() ID { return w.id }

func (w *CheckCardW) cardWidth() float32 {
	if w.width > 0 {
		return w.width
	}
	switch w.size {
	case Small:
		return 212
	case Large:
		return 440
	}
	return 320
}

// Show builds the card and toggles it on click / Enter / Space.
func (w *CheckCardW) Show() *CheckCardW {
	c, th := w.c, w.c.Theme
	el := pro2El(c)
	var on bool
	if w.checked != nil {
		on = *w.checked
	}
	b := pro2Box(c, "pro2-checkcard", w.title, layout.Column, nil)
	w.id = b.n.id
	b.n.ln.Width = w.cardWidth()
	b.n.ln.NoStretch = true
	if w.disabled {
		b.n.disabled = true
	}
	disabled := b.n.disabled
	ia := c.interact(b.n, true, input.CursorPointer)
	if disabled {
		b.n.cursor = input.CursorNotAllowed
	}
	w.ia = ia
	if ia.Clicked && !disabled && w.checked != nil {
		*w.checked = !*w.checked
		on = *w.checked
		w.changed = true
		c.invalid = true
	}
	b.n.setRole(w.role).stateIf(on, a11y.Checked).stateIf(on && w.role == a11y.RoleRadio, a11y.Selected)
	if w.desc != "" {
		desc := w.desc
		b.A11y(func(s *a11y.Node) { s.Description = desc })
	}

	rad := th.RadiusLG
	if el {
		rad = th.Radius
	}
	hov := c.Anim(b.n.id, "hover", b2f(ia.Hovered && !disabled), th.DurationMid)
	chk := c.Anim(b.n.id, "check", b2f(on), th.DurationMid)
	b.Radius(rad)
	b.Clip()
	bg := th.BgContainer
	switch {
	case disabled:
		bg = th.FillTertiary
	case on:
		bg = th.Primary.Bg
	}
	b.Bg(bg)
	bordered := w.bordered
	// shadcn choice card: a checkbox (radio dot in a group with single
	// choice) at the top right instead of Pro's corner triangle.
	shad := pro2Shad(c)
	shadInd := shad && w.extra == nil && w.cover == nil
	radio := w.role == a11y.RoleRadio
	b.PaintOver(func(l *paint.List, r geom.Rect) {
		col := th.Border
		if !bordered {
			col.A = 0
		}
		if !disabled {
			col = theme.Lerp(col, th.Primary.Base, max(hov, chk))
		} else if on {
			col = th.Primary.BorderHover
		}
		if col.A > 0 {
			l.Stroke(r, geom.Rad(rad), 1, col)
		}
		if shadInd {
			shad2dCheckIndicator(l, th, r, chk, radio, disabled, c.IsRTL())
		} else if chk > 0 && !shad {
			// Checked corner: a triangle filling the top-end corner.
			s := float32(12)
			tri := paint.NewPath().MoveTo(r.Right()-s, r.Y).LineTo(r.Right(), r.Y).LineTo(r.Right(), r.Y+s).Close()
			if c.IsRTL() {
				tri = paint.NewPath().MoveTo(r.X+s, r.Y).LineTo(r.X, r.Y).LineTo(r.X, r.Y+s).Close()
			}
			mark := th.Primary.Base
			if disabled {
				mark = th.TextDisabled
			}
			l.Path(tri, theme.Fade(mark, float64(chk)))
		}
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Rad(rad))
		}
	})

	pad := float32(16)
	if w.size == Small {
		pad = 12
	} else if w.size == Large {
		pad = 20
	}
	b.Body(func() {
		if w.cover != nil && !w.loading {
			Column(c, w.cover, layout.Pad(pad/2))
			return
		}
		Row(c, func() {
			if w.loading {
				Skeleton(c).Active().NoTitle().Rows(3).Show().Style(layout.Grow(1))
				return
			}
			if w.avatar != "" || w.avIcon != nil {
				w.avatarBox(disabled)
			}
			Column(c, func() {
				hasTitle := w.title != "" || w.extra != nil
				if hasTitle {
					Row(c, func() {
						t := Text(c, w.title).Weight(text.Medium).Ellipsis().Layout(layout.Grow(1), layout.MinW(0))
						if w.size == Large {
							t.Size(th.FontSizeLG)
						}
						if disabled {
							t.Color(th.TextDisabled)
						}
						if w.extra != nil {
							Row(c, w.extra, layout.Gap(8), layout.Middle())
						}
					}, layout.Middle(), layout.Gap(8), layout.MinH(24), layout.Margin(geom.Insets{Right: map[bool]float32{true: 20}[shadInd]}))
				}
				if w.desc != "" {
					d := Text(c, w.desc).Color(th.TextSecondary)
					if disabled {
						d.Color(th.TextDisabled)
					}
					if hasTitle {
						d.Layout(layout.Margin(geom.Insets{Top: 4}))
					}
				}
			}, layout.Grow(1), layout.MinW(0))
		}, layout.Pad(pad), layout.AlignItems(layout.AlignStart))
	})
	return w
}

// avatarBox adds the avatar column.
func (w *CheckCardW) avatarBox(disabled bool) {
	c, th := w.c, w.c.Theme
	sz := float32(40)
	if w.size == Small {
		sz = 32
	} else if w.size == Large {
		sz = 48
	}
	Column(c, func() {
		if w.avIcon != nil {
			ic := w.avIcon
			col := th.Primary.Base
			if disabled {
				col = th.TextDisabled
			}
			Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: sz, H: sz} }, func(l *paint.List, r geom.Rect) {
				l.Fill(r, geom.Rad(sz/2), th.Primary.Bg)
				s := sz * 0.55
				c.paintIcon(l, ic, geom.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}, col, 0)
			})
			return
		}
		Avatar(c, string([]rune(w.avatar)[:1])).Size(sz)
	}, layout.Margin(geom.Insets{Right: 8}))
}

// CheckCardOption is one card of a CheckCardGroup.
type CheckCardOption struct {
	_ [0]func()

	Value       string
	Title       string
	Description string
	Avatar      string     // text avatar (first character)
	Icon        *icon.Icon // icon avatar
	Disabled    bool
}

// CheckCardGroupW builds a group of CheckCards with single or multiple
// choice.
type CheckCardGroupW struct {
	_ [0]func()

	c        *Context
	opts     []CheckCardOption
	value    *string
	values   *[]string
	size     ControlSize
	disabled bool
	width    float32
	loading  bool
	changed  bool
	layout   []layout.Option
}

// CheckCardGroup starts a group of cards built from opts; bind it with
// Value (single choice) or Values (multiple choice), then Show.
func CheckCardGroup(c *Context, opts []CheckCardOption) *CheckCardGroupW {
	g := &CheckCardGroupW{c: c, opts: opts, size: c.defaultSize()}
	if sz, ok := proPropSize(c, "CheckCard"); ok {
		g.size = sz
	}
	return g
}

// Value binds the single choice ("" = none). Clicking the checked card
// again clears it, as in Pro.
func (g *CheckCardGroupW) Value(v *string) *CheckCardGroupW { g.value = v; return g }

// Values binds the multiple choice (kept in option order).
func (g *CheckCardGroupW) Values(v *[]string) *CheckCardGroupW { g.values = v; return g }

// Size sets every card's size.
func (g *CheckCardGroupW) Size(s ControlSize) *CheckCardGroupW { g.size = s; return g }

// Disabled disables every card.
func (g *CheckCardGroupW) Disabled(on bool) *CheckCardGroupW { g.disabled = on; return g }

// Width overrides every card's width.
func (g *CheckCardGroupW) Width(px float32) *CheckCardGroupW { g.width = px; return g }

// Loading shows every card as loading.
func (g *CheckCardGroupW) Loading(on bool) *CheckCardGroupW { g.loading = on; return g }

// Layout applies layout options to the group row.
func (g *CheckCardGroupW) Layout(o ...layout.Option) *CheckCardGroupW {
	g.layout = append(g.layout, o...)
	return g
}

// Changed reports a selection change this frame (after Show).
func (g *CheckCardGroupW) Changed() bool { return g.changed }

func (g *CheckCardGroupW) has(v string) bool {
	if g.values == nil {
		return false
	}
	for _, s := range *g.values {
		if s == v {
			return true
		}
	}
	return false
}

// Show builds the cards in a wrapping row.
func (g *CheckCardGroupW) Show() *CheckCardGroupW {
	c := g.c
	multi := g.values != nil
	opts := append([]layout.Option{layout.Gap(16), layout.WrapItems(), layout.AlignItems(layout.AlignStart)}, g.layout...)
	row := Row(c, func() {
		for _, o := range g.opts {
			Key(c, o.Value, func() {
				on := false
				if multi {
					on = g.has(o.Value)
				} else if g.value != nil {
					on = *g.value == o.Value
				}
				cc := CheckCard(c, o.Title, &on).Description(o.Description).Size(g.size).
					Disabled(g.disabled || o.Disabled).Loading(g.loading).Width(g.width)
				if !multi {
					cc.role = a11y.RoleRadio
				}
				if o.Avatar != "" {
					cc.Avatar(o.Avatar)
				}
				if o.Icon != nil {
					cc.AvatarIcon(o.Icon)
				}
				cc.Show()
				if !cc.Changed() {
					return
				}
				g.changed = true
				switch {
				case multi:
					var out []string
					for _, p := range g.opts {
						if (p.Value == o.Value && on) || (p.Value != o.Value && g.has(p.Value)) {
							out = append(out, p.Value)
						}
					}
					*g.values = out
				case g.value != nil:
					if on {
						*g.value = o.Value
					} else {
						*g.value = ""
					}
				}
			})
		}
	}, opts...)
	if multi {
		row.Semantics(a11y.RoleGroup, "")
	} else {
		row.Semantics(a11y.RoleRadioGroup, "")
	}
	return g
}
