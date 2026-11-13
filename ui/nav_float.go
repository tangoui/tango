package ui

import (
	"strconv"
	"time"

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
// FloatButton

type floatGroupCtx struct {
	square  bool
	primary bool
	count   int
}

var floatGroupKey = hashID("float-group-ctx")

func (c *Context) floatGroup() *[]*floatGroupCtx {
	return State[[]*floatGroupCtx](c, floatGroupKey, nil)
}

// FloatButtonW is a button floating above the page at a window corner.
type FloatButtonW struct {
	c       *Context
	n       *node
	ic      *icon.Icon
	glyph   *icon.Icon
	desc    string
	tip     string
	primary bool
	square  bool
	badge   int
	dot     bool
	pos     *node
	group   *floatGroupCtx
	index   int
	ia      Interaction
}

// floatLayer holds floating buttons at a window corner. right is the
// distance from the end edge: the layout mirrors the absolute position, so
// in a right-to-left scope the buttons float at the bottom left.
func (c *Context) floatLayer(right, bottom float32, fn func()) *node {
	var holder *node
	ov := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
		holder = c.add("float-holder", "")
		holder.ln.Absolute = true
		holder.ln.Pos = layout.Pos{Left: layout.Unset, Top: layout.Unset, Right: right, Bottom: bottom}
		holder.ln.Align = layout.AlignEnd
		c.body(holder, fn)
	})
	ov.n.hit, ov.n.blocks = false, false
	return holder
}

// FloatButton adds a floating action button (default bottom-right,
// 24px from the right and 48px from the bottom). Inside a
// FloatButtonGroup it becomes a member of the group.
func FloatButton(c *Context) *FloatButtonW {
	// ConfigProvider.Defaults("FloatButton", ...) keys: "shape"
	// ("circle" | "square"), "type" ("default" | "primary").
	f := &FloatButtonW{c: c, ic: icon.Plus, square: Prop(c, "FloatButton", "shape", "") == "square", primary: Prop(c, "FloatButton", "type", "") == "primary"}
	if g := *c.floatGroup(); len(g) > 0 {
		f.group = g[len(g)-1]
		f.square, f.primary = f.group.square, false
		f.index = f.group.count
		f.group.count++
		f.build()
		return f
	}
	f.pos = c.floatLayer(24, 48, f.build)
	return f
}

func (f *FloatButtonW) size() (float32, float32) {
	tk := &f.c.ThemeFor(theme.CompFloatButton).Components.FloatButton
	if f.desc != "" {
		return tk.Size, tk.DescriptionHeight
	}
	return tk.Size, tk.Size
}

func (f *FloatButtonW) build() {
	c := f.c
	th := c.ThemeFor(theme.CompFloatButton)
	tk := &th.Components.FloatButton
	f.n = c.add("float-button", f.tipOr())
	f.n.setRole(a11y.RoleButton).fill(f)
	f.n.ln.NoStretch = true
	f.n.ln.Measure = func(_, _ float32) geom.Size {
		w, h := f.size()
		return geom.Size{W: w, H: h}
	}
	f.ia = c.interact(f.n, true, input.CursorPointer)
	id := f.n.id
	f.n.paint = func(l *paint.List, r geom.Rect) {
		inGroup := f.group != nil && f.group.square
		rad := geom.Rad(r.W / 2)
		if f.square || f.desc != "" {
			rad = geom.Rad(tk.Radius)
		}
		hov := c.Anim(id, "hover", b2f(f.ia.Hovered), th.DurationMid)
		bg, fg := tk.Bg, tk.Color
		if f.primary {
			bg, fg = theme.Lerp(tk.PrimaryBg, tk.PrimaryHoverBg, hov), tk.PrimaryColor
		}
		if !inGroup {
			l.Shadows(r, rad, tk.Shadow)
			l.Fill(r, rad, bg)
		}
		if !f.primary && hov > 0 {
			inner := r
			ir := rad
			if inGroup {
				inner = r.Expand(-tk.GroupPadding)
				ir = geom.Rad(tk.InnerRadius)
			}
			l.Fill(inner, ir, theme.Fade(tk.HoverBg, float64(hov)*1.5))
		}
		is := tk.IconSize
		iy := r.Y + (r.H-is)/2
		if f.desc != "" {
			is = tk.DescriptionIconSize
			iy = r.Y + 8
		}
		ir := geom.Rect{X: r.X + (r.W-is)/2, Y: iy, W: is, H: is}
		if f.glyph != nil {
			icon.Draw(l, f.glyph, ir, fg, 0)
		} else {
			c.paintIcon(l, f.ic, ir, fg, 0)
		}
		if f.desc != "" {
			lay := text.Shape(f.desc, c.style(tk.DescriptionFontSize, text.Regular), c.textOpts(text.Options{NoWrap: true}))
			l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, ir.Bottom()+2), lay.Glyphs, fg)
		}
		if f.ia.FocusVisible {
			l.Stroke(r.Expand(2), rad.Grow(2), 2, th.FocusRing)
		}
		if f.badge > 0 || f.dot {
			navBadge(c, l, geom.Pt(c.inline(r).X(r.W-6, 0), r.Y+6), f.badge, f.dot) // top-end corner
		}
	}
	c.lazy(f.n, func() {
		if f.tip == "" || !f.ia.Hovered {
			return
		}
		navTooltipBubble(c, f.n.id, f.tip, InlineStart) // towards the page
	})
}

func (f *FloatButtonW) tipOr() string { return "float" }

// semantics names the button by its tooltip or description (the label
// "float" is an internal key) and exposes the badge count.
func (f *FloatButtonW) semantics(s *a11y.Node) {
	if s.Name == "float" {
		s.Name = f.tip
		if s.Name == "" {
			s.Name = f.desc
		}
	} else if f.desc != "" && f.desc != s.Name {
		s.Description = f.desc
	}
	if f.badge > 0 {
		s.Value = strconv.Itoa(f.badge)
	}
}

// A11yLabel sets the accessible name of an icon-only button without a
// tooltip.
func (f *FloatButtonW) A11yLabel(s string) *FloatButtonW { f.n.setName(s); return f }

// Icon sets the icon.
func (f *FloatButtonW) Icon(ic *icon.Icon) *FloatButtonW { f.ic = ic; return f }

// Description shows a short caption below the icon (square shape).
func (f *FloatButtonW) Description(s string) *FloatButtonW { f.desc = s; f.square = true; return f }

// Tooltip shows a tip on hover, towards the page (left; right in RTL).
func (f *FloatButtonW) Tooltip(s string) *FloatButtonW { f.tip = s; f.n.label = s; return f }

// Primary uses the brand color.
func (f *FloatButtonW) Primary() *FloatButtonW { f.primary = true; return f }

// Square uses a rounded square instead of a circle.
func (f *FloatButtonW) Square() *FloatButtonW { f.square = true; return f }

// Badge shows a count (dot when n < 0).
func (f *FloatButtonW) Badge(n int) *FloatButtonW {
	if n < 0 {
		f.dot = true
	} else {
		f.badge = n
	}
	return f
}

// Right sets the distance from the right window edge (the end edge: the
// left one in a right-to-left scope).
func (f *FloatButtonW) Right(px float32) *FloatButtonW {
	if f.pos != nil {
		f.pos.ln.Pos.Right = px
	}
	return f
}

// Bottom sets the distance from the bottom window edge.
func (f *FloatButtonW) Bottom(px float32) *FloatButtonW {
	if f.pos != nil {
		f.pos.ln.Pos.Bottom = px
	}
	return f
}

// Clicked reports activation.
func (f *FloatButtonW) Clicked() bool { return f.ia.Clicked }

// ID returns the widget ID.
func (f *FloatButtonW) ID() ID { return f.n.id }

func navBadge(c *Context, l *paint.List, at geom.Point, n int, dot bool) {
	th := c.Theme
	if dot {
		l.Fill(geom.Rect{X: at.X - 3, Y: at.Y - 3, W: 6, H: 6}, geom.Rad(3), th.Error.Base)
		return
	}
	s := strconv.Itoa(n)
	if n > 99 {
		s = "99+"
	}
	lay := text.Shape(s, c.style(th.FontSizeSM, text.Regular), text.Options{NoWrap: true})
	w := max(20, lay.Width+12)
	r := geom.Rect{X: at.X - w/2, Y: at.Y - 10, W: w, H: 20}
	l.Fill(r.Expand(1), geom.Rad(11), th.BgContainer)
	l.Fill(r, geom.Rad(10), th.Error.Base)
	l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.TextOnSolid)
}

// navTooltipBubble shows a dark tip next to anchor.
func navTooltipBubble(c *Context, anchor ID, tip string, place Placement) {
	th := c.Theme
	place = place.Resolve(c.IsRTL()) // InlineEnd → Right / Left (arrow side)
	ov := Overlay(c, OverlayOptions{Anchor: anchor, Place: place, Offset: 8}, func() {
		Text(c, tip).Color(th.TextOnSpotlight).NoWrap()
	}, layout.PadVH(6, 8))
	ov.Bg(th.BgSpotlight).Radius(th.Radius)
	ov.n.setRole(a11y.RoleTooltip)
	if len(th.TooltipShadow) > 0 {
		ov.Shadow(th.TooltipShadow)
	}
	ov.n.hit, ov.n.blocks = false, false
	// Side placements (centred by the overlay) get an arrow.
	if place == Left || place == Right {
		ov.n.paint = func(l *paint.List, r geom.Rect) {
			l.Fill(r, ov.n.radii, ov.bg)
			col := ov.bg
			p := paint.NewPath()
			cy := r.Y + r.H/2
			if place == Left {
				p.MoveTo(r.Right()-0.5, cy-5).LineTo(r.Right()+5, cy).LineTo(r.Right()-0.5, cy+5).Close()
			} else {
				p.MoveTo(r.X+0.5, cy-5).LineTo(r.X-5, cy).LineTo(r.X+0.5, cy+5).Close()
			}
			l.Path(p, col)
		}
	}
}

// ---------------------------------------------------------------------------
// FloatButtonGroup

type floatGroupState struct {
	open bool
	kbd  bool       // opened from the keyboard: hover does not close it
	ptr  geom.Point // pointer position when kbd was set
}

// FloatButtonGroupW stacks float buttons; with a trigger it collapses
// them behind a menu button.
type FloatButtonGroupW struct {
	c       *Context
	square  bool
	trigger string
	open    *bool
	ic      *icon.Icon
	primary bool
	right   float32
	bottom  float32
	st      *floatGroupState
	tip     string
}

// FloatButtonGroup starts a group; chain options, then Body with
// FloatButton calls.
func FloatButtonGroup(c *Context) *FloatButtonGroupW {
	return &FloatButtonGroupW{c: c, right: 24, bottom: 48}
}

// Square joins the buttons in one rounded card with separators.
func (g *FloatButtonGroupW) Square() *FloatButtonGroupW { g.square = true; return g }

// Trigger collapses the group behind a menu button opened by "click" or
// "hover".
func (g *FloatButtonGroupW) Trigger(t string) *FloatButtonGroupW { g.trigger = t; return g }

// Open binds the open state of a triggered group.
func (g *FloatButtonGroupW) Open(open *bool) *FloatButtonGroupW { g.open = open; return g }

// Icon sets the menu button icon (default: a plus that turns into a cross).
func (g *FloatButtonGroupW) Icon(ic *icon.Icon) *FloatButtonGroupW { g.ic = ic; return g }

// Primary colors the menu button.
func (g *FloatButtonGroupW) Primary() *FloatButtonGroupW { g.primary = true; return g }

// Position sets the distance from the right and bottom window edges.
func (g *FloatButtonGroupW) Position(right, bottom float32) *FloatButtonGroupW {
	g.right, g.bottom = right, bottom
	return g
}

// IsOpen reports whether the members are shown.
func (g *FloatButtonGroupW) IsOpen() bool { return g.trigger == "" || g.isOpen() }

func (g *FloatButtonGroupW) isOpen() bool {
	if g.open != nil {
		return *g.open
	}
	return g.st.open
}

func (g *FloatButtonGroupW) setOpen(v bool) {
	if g.open != nil {
		*g.open = v
	}
	g.st.open = v
	g.c.invalid = true
}

// Body builds the members.
func (g *FloatButtonGroupW) Body(fn func()) *FloatButtonGroupW {
	c := g.c
	tk := &c.ThemeFor(theme.CompFloatButton).Components.FloatButton
	gid := hashID(c.scope, "float-group")
	g.st = State[floatGroupState](c, gid, nil)
	stack := c.floatGroup()
	ctx := &floatGroupCtx{square: g.square}
	var members, trig *node
	var trigger *FloatButtonW
	holder := c.floatLayer(g.right, g.bottom, func() {
		col := c.add("float-group", "")
		col.ln.Dir = layout.Column
		col.ln.Align = layout.AlignCenter
		members = col
		gap := tk.GroupGap
		if g.square {
			gap = 0
		}
		col.ln.Gap = gap
		show := g.trigger == "" || g.isOpen()
		if show {
			*stack = append(*stack, ctx)
			c.body(col, fn)
			*stack = (*stack)[:len(*stack)-1]
		}
		if g.square && show {
			p := tk.GroupPadding
			col.ln.Padding = geom.All(p)
			col.paint = func(l *paint.List, r geom.Rect) {
				l.Shadows(r, geom.Rad(tk.Radius), tk.GroupShadow)
				l.Fill(r, geom.Rad(tk.Radius), tk.Bg)
				for i, k := range col.kids {
					if i > 0 {
						navHLine(l, k.abs.X+p, k.abs.Y, k.abs.W-2*p, tk.GroupSplitColor)
					}
				}
			}
		}
		if g.trigger != "" {
			if show {
				Gap(c, tk.GroupGap)
			}
			*stack = append(*stack, &floatGroupCtx{square: false})
			trigger = FloatButton(c)
			*stack = (*stack)[:len(*stack)-1]
			trig = trigger.n
			ic := g.ic
			if ic == nil {
				ic = icon.Plus
			}
			if g.isOpen() {
				ic = icon.Close
			}
			trigger.Icon(ic)
			trigger.square = g.square
			if g.primary {
				trigger.Primary()
			}
			// The trigger is an expand button for the members.
			trig.setName(c.Locale().A11y.MoreActions).setState(a11y.Expandable).stateIf(g.isOpen(), a11y.Expanded)
			if c.a11yOn && g.isOpen() {
				mid := members.id
				trig.fill(semFunc(func(s *a11y.Node) { s.Controls = a11y.NodeID(mid) }))
			}
		}
	})
	_ = holder
	if g.trigger == "click" && trigger != nil && trigger.Clicked() {
		g.setOpen(!g.isOpen())
	}
	if g.trigger == "hover" && trig != nil {
		if g.st.kbd && c.pointer != g.st.ptr {
			g.st.kbd = false
		}
		if trigger.Clicked() && navNonPointer(c, trig.id) {
			// Keyboard / screen reader activation toggles a hover group;
			// it stays as chosen until the pointer moves.
			g.setOpen(!g.isOpen())
			g.st.kbd, g.st.ptr = true, c.pointer
		} else if !g.st.kbd {
			over := c.pointerOver(trig.id, members.id)
			want := HoverDelay(c, gid, over, 100*time.Millisecond, 200*time.Millisecond)
			if want != g.isOpen() {
				g.setOpen(want)
			}
		}
	}
	if g.trigger != "" && g.isOpen() && trig != nil && (navHasNode(members, c.focus) || c.focus == trig.id) &&
		c.EscapeAt(members.id) {
		g.setOpen(false)
		g.st.kbd = false
		navFocus(c, trig.id)
	}
	if g.trigger == "click" && g.isOpen() && c.PressedOutside(members.id, trig.id) {
		g.setOpen(false)
	}
	return g
}

// navNonPointer reports whether the activation of id this frame came from
// the keyboard (Enter / Space on the focused widget) or an accessibility
// action rather than a pointer click.
func navNonPointer(c *Context, id ID) bool {
	if _, _, ok := c.A11yAction(id); ok {
		return true
	}
	if c.focus != id {
		return false
	}
	for _, k := range c.keys {
		if k.Down && (k.Key == input.KeyEnter || k.Key == input.KeySpace) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// BackTop

// BackTopW is the "back to top" floating button of a scroll container.
type BackTopW struct {
	c       *Context
	target  ID
	visible float32
	right   float32
	bottom  float32
	dur     time.Duration
	clicked bool
	content func()
}

// BackTop adds the button for the scroll container target
// (ScrollBox.ID()); it appears once scrolled past VisibilityHeight
// (default 400) and scrolls back smoothly. Call Show.
func BackTop(c *Context, target ID) *BackTopW {
	b := &BackTopW{c: c, target: target, visible: 400, right: 40, bottom: 40, dur: 450 * time.Millisecond}
	if !navEl(c) {
		b.right, b.bottom = 24, 48
	}
	return b
}

// VisibilityHeight sets the scroll distance after which it shows.
func (b *BackTopW) VisibilityHeight(px float32) *BackTopW { b.visible = px; return b }

// Position sets the distance from the right and bottom window edges.
func (b *BackTopW) Position(right, bottom float32) *BackTopW {
	b.right, b.bottom = right, bottom
	return b
}

// Duration sets the scroll animation time.
func (b *BackTopW) Duration(d time.Duration) *BackTopW { b.dur = d; return b }

// Content replaces the default icon with custom content (Element slot).
func (b *BackTopW) Content(fn func()) *BackTopW { b.content = fn; return b }

// Clicked reports a click this frame.
func (b *BackTopW) Clicked() bool { return b.clicked }

// Show builds the button.
func (b *BackTopW) Show() *BackTopW {
	c, th := b.c, b.c.Theme
	navScrollStep(c, b.target)
	s := navScrollState(c, b.target)
	show := s != nil && s.off.Y >= float64(b.visible)
	id := hashID(c.scope, "backtop", b.target)
	a := c.Anim(id, "show", b2f(show), th.DurationMid)
	if a <= 0.01 {
		return b
	}
	var hit Interaction
	pos := c.floatLayer(b.right, b.bottom, func() {
		if b.content != nil {
			box := c.box("backtop-custom", layout.Column, nil)
			box.n.ln.NoStretch = true
			box.Body(b.content)
			hit = box.Focusable(input.CursorPointer)
			box.Semantics(a11y.RoleButton, c.Locale().BackTop)
			focus := hit.FocusVisible
			box.PaintOver(func(l *paint.List, r geom.Rect) {
				if focus {
					drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
				}
			})
			return
		}
		f := FloatButton(c)
		f.glyph = navIcToTop
		if navEl(c) {
			f.glyph = navIcCaretUp
		}
		f.n.label = c.Locale().BackTop
		hit = f.ia
	})
	pos.opacity = max(a, 0.0001)
	if a >= 1 {
		pos.opacity = 0
	}
	if hit.Clicked && show {
		b.clicked = true
		navScrollTo(c, b.target, 0, b.dur)
	}
	return b
}
