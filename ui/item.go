package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ItemVariant is the surface of an Item. Open set: new variants are added
// as new constants.
type ItemVariant string

const (
	ItemVariantDefault ItemVariant = "default" // no surface
	ItemVariantOutline ItemVariant = "outline" // bordered
	ItemVariantMuted   ItemVariant = "muted"   // muted fill
)

// ItemSize is the density of an Item. Open set.
type ItemSize string

const (
	ItemSizeDefault ItemSize = "default"
	ItemSizeSmall   ItemSize = "sm"
)

// itemRole is what an Item does when activated.
type itemRole uint8

const (
	itemStatic itemRole = iota
	itemButton
	itemLink
)

// ItemW is a general list row (shadcn/ui Item): media (an icon tile, an
// avatar, an image), a title with a description, and actions at the end.
type ItemW struct {
	_        [0]func()
	c        *Context
	th       *theme.Theme
	tk       *theme.ItemToken
	box      *Box
	title    string
	desc     string
	ic       *icon.Icon
	media    func()
	content  func()
	actions  func()
	variant  ItemVariant
	size     ItemSize
	role     itemRole
	disabled bool
	inGroup  bool
	ia       Interaction
	z        *lazy
}

// Item adds a list row titled title; chain the parts and options:
//
//	ui.Item(c, "安全设置").Description("双重验证已开启").Icon(icon.Lock).Outline().
//		Actions(func() { ui.Button(c, "管理").Small() })
//	if ui.Item(c, "个人资料").Icon(icon.User).Link().Clicked() { ... }
//
// The row is built at the end of the chain (Clicked, or the end of the
// build).
func Item(c *Context, title string) *ItemW {
	w := &ItemW{c: c, title: title, variant: ItemVariantDefault, size: ItemSizeDefault}
	w.th = c.ThemeFor(theme.CompItem)
	w.tk = &w.th.Components.Item
	w.inGroup = c.cur != nil && c.cur.role == a11y.RoleList
	w.box = c.box("item", layout.Row, []layout.Option{layout.Middle()})
	w.box.n.label = title
	w.box.n.paint = w.paint
	w.box.n.over = w.over
	w.z = c.lazy(w.box.n, w.build)
	return w
}

// Description adds the secondary text under the title (two lines at most).
func (w *ItemW) Description(s string) *ItemW { w.desc = s; return w }

// Icon shows ic in a tile at the start (ItemMedia variant="icon").
func (w *ItemW) Icon(ic *icon.Icon) *ItemW { w.ic = ic; return w }

// Media builds custom media at the start (an Avatar, an Image) in place
// of the icon tile.
func (w *ItemW) Media(fn func()) *ItemW { w.media = fn; return w }

// Content builds extra content under the description (tags, progress).
func (w *ItemW) Content(fn func()) *ItemW { w.content = fn; return w }

// Actions builds controls at the end of the row.
func (w *ItemW) Actions(fn func()) *ItemW { w.actions = fn; return w }

// Variant selects the surface (default, outline, muted).
func (w *ItemW) Variant(v ItemVariant) *ItemW { w.variant = v; return w }

// Outline is Variant(ItemVariantOutline).
func (w *ItemW) Outline() *ItemW { return w.Variant(ItemVariantOutline) }

// Muted is Variant(ItemVariantMuted).
func (w *ItemW) Muted() *ItemW { return w.Variant(ItemVariantMuted) }

// Size selects the density (default, sm).
func (w *ItemW) Size(s ItemSize) *ItemW { w.size = s; return w }

// Small is Size(ItemSizeSmall).
func (w *ItemW) Small() *ItemW { return w.Size(ItemSizeSmall) }

// Clickable makes the whole row a button: focusable, hover highlight,
// Enter / Space or a click report Clicked.
func (w *ItemW) Clickable() *ItemW { return w.activate(itemButton) }

// Link makes the whole row a link (as Clickable, announced as a link;
// the app navigates on Clicked).
func (w *ItemW) Link() *ItemW { return w.activate(itemLink) }

func (w *ItemW) activate(r itemRole) *ItemW {
	if w.role == itemStatic {
		w.ia = w.c.interact(w.box.n, true, input.CursorPointer)
	}
	w.role = r
	return w
}

// Disabled dims the row and stops a clickable one from reacting.
func (w *ItemW) Disabled(on bool) *ItemW { w.disabled = on; return w }

// A11yLabel overrides the accessible name (the title by default).
func (w *ItemW) A11yLabel(s string) *ItemW { w.box.n.setName(s); return w }

// Clicked reports a click (or Enter / Space) on a clickable row.
func (w *ItemW) Clicked() bool {
	w.z.run()
	return w.role != itemStatic && w.ia.Clicked && !w.isDisabled()
}

// ID returns the row's ID.
func (w *ItemW) ID() ID { return w.box.n.id }

func (w *ItemW) isDisabled() bool { return w.disabled || w.box.n.disabled }

// semantics names the row by its title and describes it with the
// description.
func (w *ItemW) semantics(s *a11y.Node) {
	if s.Description == "" {
		s.Description = w.desc
	}
}

func (w *ItemW) build() {
	c, tk := w.c, w.tk
	n := w.box.n
	small := w.size == ItemSizeSmall
	pb, pi, gap := tk.PaddingBlock, tk.PaddingInline, tk.Gap
	if small {
		pb, pi, gap = tk.PaddingBlockSM, tk.PaddingInlineSM, tk.GapSM
	}
	w.box.Style(layout.PadVH(pb, pi), layout.Gap(gap))
	dis := w.isDisabled()
	if dis {
		w.box.Opacity(0.5)
	}
	switch w.role {
	case itemButton:
		n.setRole(a11y.RoleButton)
	case itemLink:
		n.setRole(a11y.RoleLink)
	default:
		if w.inGroup {
			n.setRole(a11y.RoleListItem)
		} else {
			n.setRole(a11y.RoleGroup)
		}
	}
	if !n.aNameSet {
		n.setName(w.title)
	}
	n.fill(w)
	if w.role != itemStatic && dis {
		n.focus, n.cursor = false, input.CursorNotAllowed
		n.setState(a11y.Disabled)
	}
	top := w.desc != "" || w.content != nil
	if w.ic != nil || w.media != nil {
		var opts []layout.Option
		if top {
			opts = append(opts, layout.Self(layout.AlignStart))
		}
		Row(c, func() {
			if w.media != nil {
				w.media()
				return
			}
			w.iconTile()
		}, opts...)
	}
	Column(c, func() {
		t := Text(c, w.title).Size(tk.TitleFontSize).Color(tk.TitleColor).Weight(themeWeight(tk.TitleFontWeight))
		t.n.aHidden = w.role != itemStatic // the row carries the name
		if w.desc != "" {
			d := Text(c, w.desc).Size(tk.DescriptionSize).Color(tk.DescriptionColor).Lines(2)
			d.n.aHidden = w.role != itemStatic
		}
		if w.content != nil {
			Column(c, w.content, layout.Margin(geom.Insets{Top: 4}))
		}
	}, layout.Grow(1), layout.MinW(0), layout.Gap(tk.ContentGap))
	if w.actions != nil {
		b := Row(c, w.actions, layout.Middle(), layout.Gap(tk.ActionsGap))
		b.n.ln.NoStretch = true
	}
}

// iconTile draws the icon media: a tinted square with the icon centred.
func (w *ItemW) iconTile() {
	c, tk, ic := w.c, w.tk, w.ic
	s := tk.MediaIconBox
	Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: s, H: s} }, func(l *paint.List, r geom.Rect) {
		rad := geom.Rad(tk.MediaIconRadius)
		l.Fill(r, rad, tk.MediaIconBg)
		if tk.MediaIconBorder.A > 0 {
			l.Stroke(r, rad, 1, tk.MediaIconBorder)
		}
		is := tk.MediaIconSize
		c.drawIconAt(l, ic, r.X+(r.W-is)/2, r.Y+(r.H-is)/2, is, tk.MediaIconColor)
	}).A11yHidden()
}

func (w *ItemW) paint(l *paint.List, r geom.Rect) {
	c, th, tk := w.c, w.th, w.tk
	rad := geom.Rad(tk.Radius)
	dis := w.isDisabled()
	switch w.variant {
	case ItemVariantMuted:
		l.Fill(r, rad, tk.MutedBg)
	case ItemVariantOutline:
		l.Stroke(r, rad, 1, tk.BorderColor)
	}
	if w.role != itemStatic && !dis {
		h := c.Anim(w.box.n.id, "hover", b2f(w.ia.Hovered), th.DurationFast)
		if h > 0 && tk.HoverBg.A > 0 {
			l.Fill(r, rad, theme.Fade(tk.HoverBg, float64(h)))
		}
	}
}

// over draws the focus ring.
func (w *ItemW) over(l *paint.List, r geom.Rect) {
	if w.role != itemStatic && w.ia.FocusVisible && !w.isDisabled() {
		drawFocusRing(l, w.th, r, geom.Rad(w.tk.Radius))
	}
}

// ItemGroupW stacks Items as a list (shadcn/ui ItemGroup), optionally
// with rules between them (ItemSeparator) and an outline around them.
type ItemGroupW struct {
	_        [0]func()
	c        *Context
	tk       *theme.ItemToken
	sep      bool
	bordered bool
	gap      float32
	label    string
	box      *Box
}

// ItemGroup starts a list of Items; chain options, then Body:
//
//	ui.ItemGroup(c).Separated().Bordered().Body(func() {
//		ui.Item(c, "通知").Description("…")
//		ui.Item(c, "隐私").Description("…")
//	})
func ItemGroup(c *Context) *ItemGroupW {
	return &ItemGroupW{c: c, tk: &c.ThemeFor(theme.CompItem).Components.Item}
}

// Separated draws a rule between consecutive items.
func (g *ItemGroupW) Separated() *ItemGroupW { g.sep = true; return g }

// Bordered outlines the group with rounded corners.
func (g *ItemGroupW) Bordered() *ItemGroupW { g.bordered = true; return g }

// Gap sets the space between items (with Separated, the rule sits in its
// middle).
func (g *ItemGroupW) Gap(px float32) *ItemGroupW { g.gap = px; return g }

// A11yLabel names the list.
func (g *ItemGroupW) A11yLabel(s string) *ItemGroupW { g.label = s; return g }

// Body builds the items.
func (g *ItemGroupW) Body(fn func()) *ItemGroupW {
	c, tk := g.c, g.tk
	gap := g.gap
	if g.sep {
		gap += 1 + 2*tk.GroupSeparatorInset
	}
	g.box = c.box("item-group", layout.Column, []layout.Option{layout.Gap(gap)})
	g.box.n.setRole(a11y.RoleList)
	if g.label != "" {
		g.box.n.setName(g.label)
	}
	if g.bordered {
		g.box.Radius(tk.Radius).Border(1, tk.BorderColor).Clip()
	}
	g.box.Body(fn)
	if g.sep {
		b := g.box
		col := tk.GroupSeparator
		prev := b.n.over
		b.n.over = func(l *paint.List, r geom.Rect) {
			if prev != nil {
				prev(l, r)
			}
			kids := b.n.kids
			for i := 0; i+1 < len(kids); i++ {
				y := (kids[i].abs.Bottom() + kids[i+1].abs.Y) / 2
				l.Fill(geom.Rect{X: r.X, Y: y - 0.5, W: r.W, H: 1}, geom.Radii{}, col)
			}
		}
	}
	return g
}

// ID returns the group's ID.
func (g *ItemGroupW) ID() ID {
	if g.box == nil {
		return 0
	}
	return g.box.n.id
}
