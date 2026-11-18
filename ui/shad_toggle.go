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

// ToggleW is a two-state button (shadcn/ui Toggle): transparent when off,
// accent-filled when on.
type ToggleW struct {
	_ [0]func()

	c        *Context
	n        *node
	label    string
	ic       *icon.Icon
	val      *bool
	outline  bool
	size     ControlSize
	disabled bool
	changed  bool
	radii    *geom.Radii
	ia       Interaction
	z        *lazy
	radio    bool // member of a single-choice ToggleGroup: a radio
	// grouped: a ToggleGroup item (px-3, no min-width, no shadow when
	// the items touch).
	grouped  bool
	noShadow bool
}

// Toggle adds a toggle button bound to *on. label may be "" for an
// icon-only toggle (it still names the control for tests).
func Toggle(c *Context, label string, on *bool) *ToggleW {
	w := &ToggleW{c: c, label: label, val: on, size: c.defaultSize(), outline: Prop(c, "Toggle", "variant", "") == "outline"}
	if sz, ok := s2cSize(c, "Toggle"); ok { // ConfigProvider.Defaults("Toggle", ...): size, variant
		w.size = sz
	}
	w.n = c.add("toggle", label)
	w.n.setRole(a11y.RoleButton).fill(w)
	w.n.ln.NoStretch = true
	w.n.ln.Measure = w.measure
	w.n.paint = w.paint
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.z = c.lazy(w.n, func() {
		if w.ia.Clicked && !w.disabled && !w.n.disabled {
			*w.val = !*w.val
			w.changed = true
			c.invalid = true
		}
	})
	return w
}

// semantics exposes the toggle as a pressed / not pressed button (a
// checked radio inside a single-choice group).
func (w *ToggleW) semantics(s *a11y.Node) {
	if w.radio {
		s.States |= a11y.Checkable
		if *w.val {
			s.States |= a11y.Checked
		}
		return
	}
	s.States |= a11y.Toggleable
	if *w.val {
		s.States |= a11y.Pressed
	}
}

// A11yLabel sets the accessible name (icon-only toggles built with an
// empty label).
func (w *ToggleW) A11yLabel(s string) *ToggleW { w.n.setName(s); return w }

// Icon puts an icon before the label (or alone when the label is shown
// only to tests: use IconOnly).
func (w *ToggleW) Icon(ic *icon.Icon) *ToggleW { w.ic = ic; return w }

// IconOnly draws just the icon; the label stays the accessible name.
func (w *ToggleW) IconOnly(ic *icon.Icon) *ToggleW { w.ic = ic; w.label = ""; return w }

// Outline adds the input border and shadow (shadcn variant="outline").
func (w *ToggleW) Outline() *ToggleW { w.outline = true; return w }

// Variant sets the shadcn variant: "default" (transparent) or "outline".
// Other values are ignored.
func (w *ToggleW) Variant(v string) *ToggleW {
	switch v {
	case "default":
		w.outline = false
	case "outline":
		w.outline = true
	}
	return w
}

// Size sets the control size.
func (w *ToggleW) Size(s ControlSize) *ToggleW { w.size = s; return w }

// Disabled disables the toggle when on.
func (w *ToggleW) Disabled(on bool) *ToggleW {
	if on {
		w.disabled, w.n.focus, w.n.cursor = true, false, input.CursorNotAllowed
	}
	return w
}

// Changed reports a flip this frame.
func (w *ToggleW) Changed() bool { w.z.run(); return w.changed }

// ID returns the widget ID.
func (w *ToggleW) ID() ID { return w.n.id }

func (w *ToggleW) font() text.Style {
	st := w.c.style(w.c.Theme.FontSize, text.Regular)
	st.Weight = themeWeight(w.c.Theme.ButtonFontWeight)
	return st
}

// padX is the inline padding: toggle.tsx px-1.5 / px-2 / px-2.5 for sm /
// default / lg; toggle-group items px-3.
func (w *ToggleW) padX() float32 {
	switch {
	case w.grouped:
		return 12
	case w.size == Small:
		return 6
	case w.size == Large:
		return 10
	}
	return 8
}

func (w *ToggleW) measure(_, _ float32) geom.Size {
	h := w.c.controlHeight(w.size)
	pad := w.padX()
	cw := float32(0)
	if w.label != "" {
		cw = text.Width(w.label, w.font())
	}
	if w.ic != nil {
		if cw > 0 {
			cw += 8
		}
		cw += 16
	}
	minW := h // min-w-9 / min-w-8 / min-w-10; group items min-w-0
	if w.grouped {
		minW = 0
	}
	return geom.Size{W: max(minW, cw+2*pad), H: h}
}

func (w *ToggleW) paint(l *paint.List, r geom.Rect) {
	c, th := w.c, w.c.Theme
	dis := w.disabled || w.n.disabled
	rad := geom.Rad(c.controlRadius(w.size))
	if w.radii != nil {
		rad = *w.radii
	}
	hov := c.Anim(w.n.id, "hover", b2f(w.ia.Hovered && !dis), th.DurationFast)
	on := c.Anim(w.n.id, "on", b2f(*w.val), th.DurationFast)
	if dis {
		l.Opacity(0.5)
		defer l.Pop()
	}
	bg := theme.Lerp(theme.Color{}, th.FillSecondary, hov)
	fg := theme.Lerp(th.Text, th.TextTertiary, hov)
	bg = theme.Lerp(bg, th.ItemSelectedBg, on)
	fg = theme.Lerp(fg, th.ItemSelected, on)
	if w.outline {
		if len(th.InputShadow) > 0 && !w.noShadow {
			l.Shadows(r, rad, th.InputShadow)
		}
		if bg.A < 255 {
			l.Fill(r, rad, th.BgContainer)
		}
	}
	l.Fill(r, rad, bg)
	if w.outline {
		l.Stroke(r, rad, 1, th.Border)
	}
	st := w.font()
	cw := float32(0)
	var lay *text.Layout
	if w.label != "" {
		lay = text.Shape(w.label, st, w.c.textOpts(text.Options{NoWrap: true}))
		cw = lay.Width
	}
	if w.ic != nil {
		if cw > 0 {
			cw += 8
		}
		cw += 16
	}
	// Icon then label from the inline start, centred as a group.
	s := w.c.inline(r)
	off := (r.W - cw) / 2
	if w.ic != nil {
		w.c.drawIconAt(l, w.ic, s.X(off, 16), r.Y+(r.H-16)/2, 16, fg)
		off += 24
	}
	if lay != nil {
		l.Glyphs(geom.Pt(s.X(off, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
	}
	if w.ia.FocusVisible && !dis {
		drawFocusRing(l, th, r, rad)
	}
}

// ToggleGroupW is a set of toggles (shadcn/ui ToggleGroup).
type ToggleGroupW struct {
	_ [0]func()

	c       *Context
	n       *node
	sel     *[]string
	opts    []Option
	icons   map[string]*icon.Icon
	single  bool
	outline bool
	size    ControlSize
	spacing int
	changed bool
	z       *lazy
	nodes   []*node
}

// ToggleGroup adds one toggle per option bound to the selected values
// (multiple selection; Single keeps at most one). The toggles touch, with
// only the outer corners rounded, unless Spacing separates them.
//
// ConfigProvider.Defaults("ToggleGroup", ...) keys: "size", "variant"
// ("default" | "outline"), "spacing" (int).
func ToggleGroup(c *Context, selected *[]string, opts []Option) *ToggleGroupW {
	g := &ToggleGroupW{c: c, sel: selected, opts: opts, size: c.defaultSize(), outline: Prop(c, "ToggleGroup", "variant", "") == "outline",
		spacing: Prop(c, "ToggleGroup", "spacing", 0)}
	if sz, ok := s2cSize(c, "ToggleGroup"); ok { // ConfigProvider.Defaults("ToggleGroup", ...): size, variant
		g.size = sz
	}
	g.n = c.add("togglegroup", "")
	g.n.ln.Dir = layout.Row
	g.n.ln.Align = layout.AlignCenter
	g.n.ln.NoStretch = true
	g.z = c.lazy(g.n, g.build)
	return g
}

// A11yLabel sets the accessible name of the group.
func (g *ToggleGroupW) A11yLabel(s string) *ToggleGroupW { g.n.setName(s); return g }

// Single makes the group behave like radio buttons that can be cleared.
func (g *ToggleGroupW) Single() *ToggleGroupW { g.single = true; return g }

// Outline joins the toggles into one bordered strip.
func (g *ToggleGroupW) Outline() *ToggleGroupW { g.outline = true; return g }

// Variant sets the variant of every toggle: "default" or "outline".
// Other values are ignored.
func (g *ToggleGroupW) Variant(v string) *ToggleGroupW {
	switch v {
	case "default":
		g.outline = false
	case "outline":
		g.outline = true
	}
	return g
}

// Spacing sets the gap between the toggles in Tailwind spacing units of
// 4px (shadcn toggle-group spacing, default 0). With 0 the toggles touch
// and only the group's outer corners are rounded; otherwise every toggle
// is rounded (and bordered when outlined) on its own. Negative values are
// treated as 0.
func (g *ToggleGroupW) Spacing(n int) *ToggleGroupW { g.spacing = max(0, n); return g }

// Size sets the size of every toggle.
func (g *ToggleGroupW) Size(s ControlSize) *ToggleGroupW { g.size = s; return g }

// Icons draws icon-only toggles; the option labels stay their names.
func (g *ToggleGroupW) Icons(m map[string]*icon.Icon) *ToggleGroupW { g.icons = m; return g }

// Changed reports a selection change this frame.
func (g *ToggleGroupW) Changed() bool { g.z.run(); return g.changed }

func (g *ToggleGroupW) build() {
	c := g.c
	joined := g.spacing <= 0
	g.n.ln.Gap = float32(max(0, g.spacing)) * 4
	rad := c.controlRadius(g.size)
	// A single-choice group is a radio group, a multiple one a toolbar;
	// either way it is one Tab stop and the arrow keys move between the
	// toggles (Space / Enter flip the focused one).
	if g.single {
		g.n.setRole(a11y.RoleRadioGroup)
	} else {
		g.n.setRole(a11y.RoleToolbar)
	}
	g.n.setState(a11y.Horizontal)
	g.nodes = g.nodes[:0]
	cur := -1
	in := newSelIndex(g.sel)
	for i, o := range g.opts {
		on := in.has(o.Value)
		t := Toggle(c, o.Label, &on).Size(g.size).Disabled(o.Disabled)
		t.grouped = true
		g.nodes = append(g.nodes, t.n)
		if on && cur < 0 {
			cur = i
		}
		if g.single {
			t.radio = true
			t.n.setRole(a11y.RoleRadio)
		}
		if ic := g.icons[o.Value]; ic != nil {
			t.IconOnly(ic)
		}
		if g.outline {
			t.Outline()
		}
		if joined {
			// rounded-none, first:rounded-l-md last:rounded-r-md (the
			// inline start / end), shadow-none; outlined items share
			// their borders (border-l-0).
			t.noShadow = true
			var r geom.Radii
			left, right := i == 0, i == len(g.opts)-1
			if c.IsRTL() {
				left, right = right, left
			}
			if left {
				r.TL, r.BL = rad, rad
			}
			if right {
				r.TR, r.BR = rad, rad
			}
			t.radii = &r
			if g.outline && i > 0 {
				t.n.ln.Margin.Left = -1
			}
		}
		if t.Changed() {
			g.changed = true
			switch {
			case on && g.single:
				*g.sel = []string{o.Value}
				in.reset(o.Value)
			case on:
				*g.sel = append(*g.sel, o.Value)
				in.add(o.Value)
			default:
				*g.sel = remove(*g.sel, o.Value)
				in.del(o.Value)
			}
		}
	}
	g.keys(cur)
}

// keys applies the roving Tab stop and the arrow keys of the group.
func (g *ToggleGroupW) keys(cur int) {
	c := g.c
	ids := make([]ID, len(g.nodes))
	for i, n := range g.nodes {
		ids[i] = n.id
		if n.id == c.focus {
			cur = i
		}
	}
	rovingTabStop(g.nodes, cur)
	if to := rovingStep(c, ids, func(i int) bool { return !g.opts[i].Disabled }, rovingHorizontal); to >= 0 {
		navFocus(c, ids[to])
	}
}
