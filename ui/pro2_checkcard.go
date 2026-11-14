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

	c          *Context
	title      string
	sub        string
	value      string
	defChecked bool
	isChecked  bool
	content    func()
	actions    []func()
	checked    *bool
	desc       string
	avatar     string
	avIcon     *icon.Icon
	extra      func()
	cover      func()
	disabled   bool
	size       ControlSize
	sizeSet    bool
	loading    bool
	bordered   bool
	width      float32
	role       a11y.Role
	changed    bool
	ia         Interaction
	id         ID
}

// CheckCard starts a checkable card bound to checked; chain options, then
// Show.
func CheckCard(c *Context, title string, checked *bool) *CheckCardW {
	w := &CheckCardW{c: c, title: title, checked: checked, size: c.defaultSize(), bordered: true, role: a11y.RoleCheckbox}
	// ConfigProvider.Defaults("CheckCard", ...) keys: "size", "bordered",
	// "disabled", "loading", "defaultChecked".
	if sz, ok := proPropSize(c, "CheckCard"); ok {
		w.size = sz
	}
	w.bordered = Prop(c, "CheckCard", "bordered", true)
	w.disabled = Prop(c, "CheckCard", "disabled", false)
	w.loading = Prop(c, "CheckCard", "loading", false)
	w.defChecked = Prop(c, "CheckCard", "defaultChecked", false)
	return w
}

// DefaultChecked sets the initial checked state when checked is nil.
func (w *CheckCardW) DefaultChecked(on bool) *CheckCardW { w.defChecked = on; return w }

// IsChecked reports the current selection after Show.
func (w *CheckCardW) IsChecked() bool { return w.isChecked }

// SubTitle sets text beside the title.
func (w *CheckCardW) SubTitle(s string) *CheckCardW { w.sub = s; return w }

// Value sets this card's key when built in CheckCardGroup.Body.
func (w *CheckCardW) Value(s string) *CheckCardW { w.value = s; return w }

// Content builds body content below the card's metadata.
func (w *CheckCardW) Content(fn func()) *CheckCardW { w.content = fn; return w }

// Actions builds the bottom action bar.
func (w *CheckCardW) Actions(fns ...func()) *CheckCardW { w.actions = fns; return w }

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
func (w *CheckCardW) Size(s ControlSize) *CheckCardW { w.size, w.sizeSet = s, true; return w }

// Small is Size(Small).
func (w *CheckCardW) Small() *CheckCardW { return w.Size(Small) }

// Large is Size(Large).
func (w *CheckCardW) Large() *CheckCardW { return w.Size(Large) }

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
	group := checkCardCurrentGroup(c)
	if group != nil {
		w.disabled = w.disabled || group.disabled
		w.loading = w.loading || group.loading
		w.bordered = w.bordered && group.bordered
		if !w.sizeSet {
			w.size = group.size
		}
		if !group.isMulti() {
			w.role = a11y.RoleRadio
		}
	}
	var on bool
	if w.checked != nil {
		on = *w.checked
	}
	b := pro2Box(c, "pro2-checkcard", w.title, layout.Column, nil)
	w.id = b.n.id
	state := State[checkCardState](c, b.n.id, func() checkCardState { return checkCardState{checked: w.defChecked} })
	if w.checked == nil {
		on = state.checked
	}
	if group != nil {
		found := false
		for _, v := range group.order {
			if v == w.value {
				found = true
				break
			}
		}
		if !found {
			group.order = append(group.order, w.value)
		}
		on = group.contains(w.value)
		if w.loading {
			on = false
		}
	}
	w.changed = false
	b.n.ln.Width = w.cardWidth()
	b.n.ln.NoStretch = true
	disabled := b.n.disabled || w.disabled
	if disabled || w.loading {
		b.n.disabled = true
	}
	ia := c.interact(b.n, true, input.CursorPointer)
	if disabled {
		b.n.cursor = input.CursorNotAllowed
	}
	w.ia = ia
	if ia.Clicked && !disabled {
		on = !on
		switch {
		case group != nil:
			group.pending = &checkCardToggle{value: w.value, on: on}
		case w.checked != nil:
			*w.checked = on
		default:
			state.checked = on
		}
		w.changed = true
		c.invalid = true
	}
	w.isChecked = on
	b.n.stateIf(w.loading, a11y.Busy)
	b.n.setRole(w.role).stateIf(on, a11y.Checked).stateIf(on && w.role == a11y.RoleRadio, a11y.Selected)
	if w.desc != "" {
		desc := w.desc
		b.A11y(func(s *a11y.Node) { s.Description = desc })
	}

	rad := th.Radius
	hov := c.Anim(b.n.id, "hover", b2f(ia.Hovered && !disabled), th.DurationMid)
	chk := c.Anim(b.n.id, "check", b2f(on), th.DurationMid)
	b.Radius(rad)
	b.Clip()
	bg := th.BgContainer
	switch {
	case disabled:
		bg = th.BgContainerDisabled
	case on:
		bg = th.Primary.Bg
	}
	b.Bg(bg)
	bordered := w.bordered
	b.PaintOver(func(l *paint.List, r geom.Rect) {
		col := th.Border
		if !bordered {
			col.A = 0
		}
		if !disabled {
			col = theme.Lerp(col, th.Primary.Base, max(hov, chk))
		}
		if col.A > 0 {
			l.Stroke(r, geom.Rad(rad), th.LineWidth, col)
		}
		if chk > 0 {
			// Checked corner: a triangle filling the top-end corner.
			s := 2 * (th.Radius + 4)
			// CSS borders form a 2s triangle inset 2px at the top-end.
			rr := geom.Rect{X: r.Right() - 2 - s, Y: r.Y + 2, W: s, H: s}
			tri := paint.NewPath().MoveTo(rr.X, rr.Y).LineTo(rr.Right()-rad, rr.Y).QuadTo(rr.Right(), rr.Y, rr.Right(), rr.Y+rad).LineTo(rr.Right(), rr.Bottom()).Close()
			if c.IsRTL() {
				rr.X = r.X + 2
				tri = paint.NewPath().MoveTo(rr.Right(), rr.Y).LineTo(rr.X+rad, rr.Y).QuadTo(rr.X, rr.Y, rr.X, rr.Y+rad).LineTo(rr.X, rr.Bottom()).Close()
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

	pad := th.Padding
	padding := geom.Insets{Top: pad, Bottom: pad, Left: th.PaddingSM, Right: th.PaddingSM}
	b.Body(func() {
		if w.loading {
			Column(c, func() {
				Skeleton(c).Active().NoTitle().Rows(checkCardLoadingRows).Show().Label("pro2-checkcard-loading")
			}, layout.Pad(th.PaddingMD))
		} else if w.cover != nil {
			Column(c, w.cover, layout.Pad(th.PaddingXXS))
		} else {
			Row(c, func() {
				if w.avatar != "" || w.avIcon != nil {
					w.avatarBox(disabled)
				}
				Column(c, func() {
					hasTitle := w.title != "" || w.extra != nil
					if hasTitle {
						Row(c, func() {
							t := Text(c, w.title).Weight(text.Medium).Color(th.TextHeading).Ellipsis().Layout(layout.MinW(0))
							if disabled {
								t.Color(th.TextDisabled)
							}
							if w.sub != "" {
								Text(c, w.sub).Color(th.TextSecondary).NoWrap().Layout(layout.Margin(geom.Insets{Left: th.PaddingSM}))
							}
							Div(c, layout.Grow(1))
							if w.extra != nil {
								Row(c, w.extra, layout.Gap(8), layout.Middle())
							}
						}, layout.Middle(), layout.MinH(th.LineHeightFor(th.FontSize)))
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
			}, layout.Padding(padding), layout.AlignItems(func() layout.Align {
				if (w.avatar != "" || w.avIcon != nil) && w.desc == "" {
					return layout.AlignCenter
				}
				return layout.AlignStart
			}()))
		}
		if w.content != nil {
			Column(c, w.content, layout.Padding(padding))
		}
		if len(w.actions) > 0 {
			(&ProCardW{c: c, actions: w.actions}).actionBar(th.BorderSecondary)
		}
	})
	return w
}

// avatarBox adds the avatar column.
func (w *CheckCardW) avatarBox(disabled bool) {
	c, th := w.c, w.c.Theme
	sz := float32(48)
	b := Column(c, func() {
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
		Avatar(c, string([]rune(w.avatar)[:1])).Size(sz).Square()
	}, layout.Margin(geom.Insets{Right: 8}))
	if disabled {
		b.Opacity(0.25)
	}
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
	Size        ControlSize // zero without SizeSet: inherit group size
	SizeSet     bool        // allows explicit Middle to override the group size
	Cover       func()
	Children    []CheckCardOption
}

// checkCardState is persistent only when no application binding is supplied.
type checkCardState struct{ checked bool }

const checkCardLoadingRows = 4

type checkCardToggle struct {
	value string
	on    bool
}

type checkCardGroupState struct {
	value  string
	values []string
}
type checkCardGroupStack struct{ groups []*CheckCardGroupW }

var checkCardGroupStackID = hashID("pro2-checkcard-group-stack")

func checkCardCurrentGroup(c *Context) *CheckCardGroupW {
	s := State[checkCardGroupStack](c, checkCardGroupStackID, nil)
	if len(s.groups) > 0 {
		return s.groups[len(s.groups)-1]
	}
	return nil
}

// CheckCardGroupW builds a single- or multiple-choice card group.
type CheckCardGroupW struct {
	_         [0]func()
	c         *Context
	opts      []CheckCardOption
	value     *string
	values    *[]string
	multiple  bool
	defValue  string
	defValues []string
	st        *checkCardGroupState
	size      ControlSize
	disabled  bool
	width     float32
	loading   bool
	bordered  bool
	changed   bool
	layout    []layout.Option
	order     []string
	pending   *checkCardToggle
}

// CheckCardGroup starts a group. Value / Values bind application state;
// without a binding the group owns its state, initialized by DefaultValue
// or DefaultValues. Values also selects multiple-choice mode.
func CheckCardGroup(c *Context, opts []CheckCardOption) *CheckCardGroupW {
	g := &CheckCardGroupW{c: c, opts: opts, size: c.defaultSize(), bordered: Prop(c, "CheckCardGroup", "bordered", true)}
	if sz, ok := proPropSize(c, "CheckCard"); ok {
		g.size = sz
	}
	if sz, ok := proPropSize(c, "CheckCardGroup"); ok {
		g.size = sz
	}
	g.multiple = Prop(c, "CheckCardGroup", "multiple", false)
	g.disabled = Prop(c, "CheckCardGroup", "disabled", false)
	g.loading = Prop(c, "CheckCardGroup", "loading", false)
	g.defValue = Prop(c, "CheckCardGroup", "defaultValue", "")
	g.defValues = append([]string(nil), Prop(c, "CheckCardGroup", "defaultValue", []string(nil))...)
	return g
}

// Value binds the single choice. Re-clicking a selected card clears it.
func (g *CheckCardGroupW) Value(v *string) *CheckCardGroupW { g.value = v; return g }

// Values binds multiple choices, maintained in option order.
func (g *CheckCardGroupW) Values(v *[]string) *CheckCardGroupW {
	g.values = v
	g.multiple = true
	return g
}

// Multiple enables multiple choices with internally owned state.
func (g *CheckCardGroupW) Multiple() *CheckCardGroupW { g.multiple = true; return g }

// DefaultValue sets the initial single selection, ignored when bound.
func (g *CheckCardGroupW) DefaultValue(v string) *CheckCardGroupW { g.defValue = v; return g }

// DefaultValues sets initial multiple selections, ignored when bound.
func (g *CheckCardGroupW) DefaultValues(v ...string) *CheckCardGroupW {
	g.defValues = append([]string(nil), v...)
	return g
}

// Selected reports the single selection after Show / Body.
func (g *CheckCardGroupW) Selected() string {
	if g.value != nil {
		return *g.value
	}
	if g.st != nil {
		return g.st.value
	}
	return ""
}

// SelectedValues reports a copy of the multiple selections after Show / Body.
func (g *CheckCardGroupW) SelectedValues() []string {
	if g.values != nil {
		return append([]string(nil), (*g.values)...)
	}
	if g.st != nil {
		return append([]string(nil), g.st.values...)
	}
	return nil
}

// Size sets the group size.
func (g *CheckCardGroupW) Size(s ControlSize) *CheckCardGroupW { g.size = s; return g }

// Disabled disables every card.
func (g *CheckCardGroupW) Disabled(on bool) *CheckCardGroupW { g.disabled = on; return g }

// Width overrides every option card's width.
func (g *CheckCardGroupW) Width(px float32) *CheckCardGroupW { g.width = px; return g }

// Loading shows noninteractive, unchecked loading placeholders.
func (g *CheckCardGroupW) Loading(on bool) *CheckCardGroupW { g.loading = on; return g }

// Borderless removes option card borders.
func (g *CheckCardGroupW) Borderless() *CheckCardGroupW { g.bordered = false; return g }

// Layout sets wrapping-row layout options.
func (g *CheckCardGroupW) Layout(opts ...layout.Option) *CheckCardGroupW {
	g.layout = append(g.layout, opts...)
	return g
}

// Changed reports a user selection change this frame.
func (g *CheckCardGroupW) Changed() bool { return g.changed }
func (g *CheckCardGroupW) isMulti() bool { return g.multiple || g.values != nil }
func (g *CheckCardGroupW) contains(v string) bool {
	if !g.isMulti() {
		return g.Selected() == v
	}
	for _, x := range g.SelectedValues() {
		if x == v {
			return true
		}
	}
	return false
}
func (g *CheckCardGroupW) toggle(v string, on bool) {
	g.changed = true
	if !g.isMulti() {
		next := ""
		if on {
			next = v
		}
		if g.value != nil {
			*g.value = next
		} else {
			g.st.value = next
		}
		return
	}
	current := g.SelectedValues()
	var next []string
	for _, key := range g.order {
		selected := false
		for _, old := range current {
			if old == key {
				selected = true
				break
			}
		}
		if key == v {
			selected = on
		}
		if selected {
			next = append(next, key)
		}
	}
	if g.values != nil {
		*g.values = next
	} else {
		g.st.values = next
	}
}
func (g *CheckCardGroupW) collect(opts []CheckCardOption) {
	for _, o := range opts {
		if len(o.Children) > 0 {
			g.collect(o.Children)
		} else {
			g.order = append(g.order, o.Value)
		}
	}
}
func (g *CheckCardGroupW) render(opts []CheckCardOption) {
	c := g.c
	for _, option := range opts {
		o := option
		Key(c, o.Value, func() {
			title := o.Title
			if title == "" {
				title = o.Value
			}
			if len(o.Children) > 0 {
				Column(c, func() {
					n := c.add("checkcard-subgroup", title)
					st := State[struct{ collapsed bool }](c, n.id, nil)
					n.ln.Dir = layout.Row
					n.ln.Padding.Top = c.Theme.PaddingXS
					n.ln.Padding.Bottom = c.Theme.PaddingXS
					n.ln.Gap = 4
					n.ln.Align = layout.AlignCenter
					n.setRole(a11y.RoleButton).setName(title).setState(a11y.Expandable).stateIf(!st.collapsed, a11y.Expanded)
					n.disabled = g.disabled || g.loading || o.Disabled
					ia := c.interact(n, true, input.CursorPointer)
					if ia.FocusVisible {
						n.over = func(l *paint.List, r geom.Rect) { drawFocusRing(l, c.Theme, r, geom.Rad(c.Theme.Radius)) }
					}
					if ia.Clicked {
						st.collapsed = !st.collapsed
						c.invalid = true
					}
					c.body(n, func() {
						ic := icon.Right
						if !st.collapsed {
							ic = icon.Down
						}
						Icon(c, ic).Size(c.Theme.FontSize)
						Text(c, title)
					})
					if !st.collapsed {
						Row(c, func() { g.render(o.Children) }, layout.Gap(16), layout.WrapItems())
					}
				}, layout.Gap(8))
				return
			}
			cc := CheckCard(c, title, nil).Value(o.Value).Description(o.Description).Disabled(o.Disabled).Width(g.width)
			if o.SizeSet || o.Size != Middle {
				cc.Size(o.Size)
			}
			if o.Avatar != "" {
				cc.Avatar(o.Avatar)
			}
			if o.Icon != nil {
				cc.AvatarIcon(o.Icon)
			}
			if o.Cover != nil {
				cc.Cover(o.Cover)
			}
			cc.Show()
		})
	}
}

// Show renders the configured options.
func (g *CheckCardGroupW) Show() *CheckCardGroupW { return g.Body(func() { g.render(g.opts) }) }

// Body builds custom CheckCards linked to this group by Value.
func (g *CheckCardGroupW) Body(fn func()) *CheckCardGroupW {
	c := g.c
	opts := append([]layout.Option{layout.Gap(16), layout.WrapItems(), layout.AlignItems(layout.AlignStart)}, g.layout...)
	b := pro2Box(c, "pro2-checkcard-group", "", layout.Row, opts)
	g.st = State[checkCardGroupState](c, b.n.id, func() checkCardGroupState {
		return checkCardGroupState{value: g.defValue, values: append([]string(nil), g.defValues...)}
	})
	g.changed = false
	g.pending = nil
	g.order = nil
	g.collect(g.opts)
	stack := State[checkCardGroupStack](c, checkCardGroupStackID, nil)
	stack.groups = append(stack.groups, g)
	b.Body(fn)
	if g.pending != nil {
		g.toggle(g.pending.value, g.pending.on)
	}
	stack.groups = stack.groups[:len(stack.groups)-1]
	if g.isMulti() {
		b.Semantics(a11y.RoleGroup, "")
	} else {
		b.Semantics(a11y.RoleRadioGroup, "")
	}
	return g
}
