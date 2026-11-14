package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ButtonGroupW joins buttons into one group (Ant Button.Group, Element
// el-button-group, shadcn ButtonGroup): neighbours share a one-pixel
// border, only the outer corners stay rounded, and size, color,
// appearance or variant can be set once for every button. Buttons keep
// their own modifiers, which win over the group's.
//
//	ui.ButtonGroup(c).Variant(ui.VariantPrimary).Body(func() {
//		ui.Button(c, "上一页").Icon(icon.Left)
//		ui.Button(c, "下一页").IconRight(icon.Right)
//	})
//
// Only buttons that are direct children join; each stays its own Tab stop.
type ButtonGroupW struct {
	_        [0]func()
	c        *Context
	n        *node
	size     ControlSize
	sizeSet  bool
	color    ButtonColor
	appear   ButtonAppearance
	variant  ButtonVariant
	varSet   bool
	vertical bool
	name     string
	opts     []layout.Option
	btns     []*ButtonW
}

// btnGroupLink lets the buttons built inside a group find it: it is kept
// in the retained state of the group node and rewritten every frame.
type btnGroupLink struct {
	g *ButtonGroupW
}

// ButtonGroup starts a button group; chain options, then Body.
func ButtonGroup(c *Context) *ButtonGroupW { return &ButtonGroupW{c: c} }

// Size sets the size of every button.
func (g *ButtonGroupW) Size(s ControlSize) *ButtonGroupW { g.size, g.sizeSet = s, true; return g }

// Color sets the color of every button (see ButtonW.Color).
func (g *ButtonGroupW) Color(col ButtonColor) *ButtonGroupW { g.color = col; return g }

// Appearance sets the appearance of every button (see ButtonW.Appearance).
func (g *ButtonGroupW) Appearance(a ButtonAppearance) *ButtonGroupW { g.appear = a; return g }

// Variant sets the variant of every button (Element group type="primary"
// is Variant(ui.VariantPrimary)).
func (g *ButtonGroupW) Variant(v ButtonVariant) *ButtonGroupW {
	g.variant, g.varSet = v, true
	return g
}

// Vertical stacks the buttons (joined top / bottom edges).
func (g *ButtonGroupW) Vertical() *ButtonGroupW { g.vertical = true; return g }

// A11yLabel names the group for screen readers.
func (g *ButtonGroupW) A11yLabel(s string) *ButtonGroupW { g.name = s; return g }

// Layout applies layout options to the group.
func (g *ButtonGroupW) Layout(o ...layout.Option) *ButtonGroupW {
	g.opts = append(g.opts, o...)
	return g
}

// ID returns the group's widget ID (valid after Body).
func (g *ButtonGroupW) ID() ID {
	if g.n == nil {
		return 0
	}
	return g.n.id
}

// Body adds the buttons.
func (g *ButtonGroupW) Body(fn func()) *ButtonGroupW {
	c := g.c
	dir := layout.Row
	if g.vertical {
		dir = layout.Column
	}
	// NoStretch keeps the natural size inside stretching parents.
	opts := append([]layout.Option{layout.AlignItems(layout.AlignStretch), layout.NoStretch()}, g.opts...)
	b := c.box("buttongroup", dir, opts)
	g.n = b.n
	b.n.setRole(a11y.RoleGroup)
	if g.vertical {
		b.n.setState(a11y.Vertical)
	} else {
		b.n.setState(a11y.Horizontal)
	}
	if g.name != "" {
		b.n.setName(g.name)
	}
	b.n.paint = nil
	b.n.over = g.paintOver
	State[btnGroupLink](c, hashID(b.n.id, "buttongroup"), nil).g = g
	b.Body(fn)
	return g
}

// buttonGroupOf returns the group whose node is p, or nil.
func buttonGroupOf(c *Context, p *node) *ButtonGroupW {
	if p == nil {
		return nil
	}
	e, ok := c.states[hashID(p.id, "buttongroup")]
	if !ok {
		return nil
	}
	lk, ok := e.v.(*btnGroupLink)
	if !ok || lk.g == nil || lk.g.n != p {
		return nil
	}
	return lk.g
}

// joinGroup applies the settings of an enclosing ButtonGroup (before the
// button's own modifiers run) and overlaps it with the previous child.
func (b *ButtonW) joinGroup() {
	g := buttonGroupOf(b.c, b.n.parent)
	if g == nil {
		return
	}
	b.grp = g
	if g.sizeSet {
		b.size = g.size
	}
	if g.color != "" {
		b.color = g.color
	}
	if g.appear != "" {
		b.appear = g.appear
	}
	if g.varSet {
		b.variant = g.variant
	}
	if g.vertical {
		b.Block() // equal widths
	}
	if len(g.n.kids) > 1 {
		if g.vertical {
			b.Layout(layout.Margin(geom.Insets{Top: -1}))
		} else {
			b.Layout(layout.Margin(geom.Insets{Left: -1})) // start edge; mirrored in RTL
		}
	}
	g.btns = append(g.btns, b)
}

// position reports whether b is the first / last child of the group.
func (g *ButtonGroupW) position(b *ButtonW) (first, last bool) {
	kids := g.n.kids
	for i, k := range kids {
		if k == b.n {
			return i == 0, i == len(kids)-1
		}
	}
	return true, true
}

// joined reports which physical sides of b touch a neighbour.
func (g *ButtonGroupW) joined(b *ButtonW) (left, right, top, bottom bool) {
	first, last := g.position(b)
	if g.vertical {
		return false, false, !first, !last
	}
	left, right = !first, !last // previous child at the inline start
	if b.rtl() {
		left, right = right, left
	}
	return left, right, false, false
}

// corners squares the joined corners of rad.
func (g *ButtonGroupW) corners(b *ButtonW, rad geom.Radii) geom.Radii {
	left, right, top, bottom := g.joined(b)
	if left || top {
		rad.TL = 0
	}
	if left || bottom {
		rad.BL = 0
	}
	if right || top {
		rad.TR = 0
	}
	if right || bottom {
		rad.BR = 0
	}
	return rad
}

// divider draws the seam between b and the previous solid button: the
// shared border column takes a light tint of the label instead of the
// fill (Element --el-button-divide-border-color, Ant primary groups).
func (g *ButtonGroupW) divider(l *paint.List, b *ButtonW, r geom.Rect, lk btnLook) {
	if lk.bg.A == 0 || lk.border != lk.bg || lk.dashed {
		return
	}
	first, _ := g.position(b)
	if first {
		return
	}
	col := theme.Over(theme.Fade(lk.fg, float64(b.tk.GroupDividerOpacity)), theme.Over(lk.bg, b.tk.DefaultBg))
	switch {
	case g.vertical:
		l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, col)
	case b.rtl():
		l.Fill(geom.Rect{X: r.Right() - 1, Y: r.Y, W: 1, H: r.H}, geom.Radii{}, col)
	default:
		l.Fill(geom.Rect{X: r.X, Y: r.Y, W: 1, H: r.H}, geom.Radii{}, col)
	}
}

// paintOver raises the border and focus ring of the hovered, pressed or
// focused button above its neighbours (they share a border column).
func (g *ButtonGroupW) paintOver(l *paint.List, _ geom.Rect) {
	for _, b := range g.btns {
		if b.n.parent != g.n || b.isDisabled() {
			continue
		}
		if !b.ia.Hovered && !b.ia.Pressed && !b.ia.FocusVisible {
			continue
		}
		r := b.n.abs
		rad := b.radius(r)
		lk := b.look()
		if lk.opacity == 0 || lk.opacity >= 1 {
			b.strokeBorder(l, r, rad, lk)
		}
		b.focusRing(l, r, rad)
	}
}

// rtl reports the direction of the scope b was built in.
func (b *ButtonW) rtl() bool {
	if e := b.n.env; e != nil {
		return e.rtl
	}
	return b.c.IsRTL()
}
