package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Option is an item of a Select / RadioGroup / CheckboxGroup. Group puts
// the option under a titled group in Select / MultiSelect / VirtualSelect
// dropdowns (Ant OptGroup, Element el-option-group); options of one group
// are listed together in order of the group's first appearance.
type Option struct {
	Label, Value string
	Disabled     bool
	Group        string
}

// Opts builds options whose value equals the label.
func Opts(labels ...string) []Option {
	out := make([]Option, len(labels))
	for i, l := range labels {
		out[i] = Option{Label: l, Value: l}
	}
	return out
}

// later runs fn once, with n as the current parent, either when a query
// method calls it or at the end of the build pass. The ConfigProvider
// scope (locale, size, defaults) active at creation is restored while fn
// runs, so deferred builds read the same c.Locale() as the caller; so is
// the node environment (theme, direction), so nodes the deferred build adds
// belong to the scope the widget was created in.
type lazy struct {
	c      *Context
	n      *node
	fn     func()
	ov     ID   // overlay being built when the widget was created
	env    *env // scope the widget was created in
	cfg    navConfig
	hasCfg bool
	done   bool
}

func (c *Context) lazy(n *node, fn func()) *lazy {
	z := &lazy{c: c, n: n, fn: fn, ov: c.curOverlay, env: c.env}
	if cf := c.navConfig(); cf != nil {
		z.cfg, z.hasCfg = *cf, true
	}
	c.deferred = append(c.deferred, z.run)
	return z
}

func (z *lazy) run() {
	if z.done {
		return
	}
	z.done = true
	c := z.c
	prev, prevScope, prevOv := c.cur, c.scope, c.curOverlay
	c.cur, c.scope, c.curOverlay = z.n, z.n.id, z.ov
	if z.env != nil && z.env != c.env {
		pe, pt := c.enterEnv(z.env)
		defer c.leaveEnv(pe, pt)
	}
	if z.hasCfg {
		s := c.navConfigs()
		depth := len(s.frames)
		s.frames = append(s.frames, z.cfg)
		z.fn()
		s = c.navConfigs()
		if len(s.frames) > depth {
			s.frames = s.frames[:depth]
		}
	} else {
		z.fn()
	}
	c.cur, c.scope, c.curOverlay = prev, prevScope, prevOv
}

// ---------------------------------------------------------------------------
// Checkbox

// CheckboxW is a checkbox with a label.
type CheckboxW struct {
	c             *Context
	n             *node
	label         string
	val           *bool
	indeterminate bool
	disabled      bool
	changed       bool
	status        string // "error": invalid (shadcn destructive border)
	ia            Interaction
	z             *lazy
}

// Checkbox adds a checkbox bound to *val.
func Checkbox(c *Context, label string, val *bool) *CheckboxW {
	w := &CheckboxW{c: c, label: label, val: val, status: c.itemStatus}
	w.n = c.add("checkbox", label)
	w.n.setRole(a11y.RoleCheckbox).setState(a11y.Checkable).fill(w)
	w.n.ln.NoStretch = true
	w.n.ln.Measure = w.measure
	w.n.paint = w.paint
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.z = c.lazy(w.n, w.finish)
	return w
}

func (w *CheckboxW) Indeterminate(on bool) *CheckboxW { w.indeterminate = on; return w }

// Status sets the validation status ("error" marks the box invalid; inside
// a failing FormItem it is set automatically).
func (w *CheckboxW) Status(s string) *CheckboxW { w.status = s; return w }
func (w *CheckboxW) Disabled(on bool) *CheckboxW {
	if on {
		w.disabled, w.n.focus, w.n.cursor = true, false, input.CursorNotAllowed
	}
	return w
}

// Changed reports a toggle this frame.
func (w *CheckboxW) Changed() bool { w.z.run(); return w.changed }

// A11yLabel sets the accessible name of a checkbox without a visible label.
func (w *CheckboxW) A11yLabel(s string) *CheckboxW { w.n.setName(s); return w }

func (w *CheckboxW) semantics(s *a11y.Node) {
	switch {
	case w.indeterminate:
		s.States |= a11y.Mixed
	case w.val != nil && *w.val:
		s.States |= a11y.Checked
	}
}

func (w *CheckboxW) finish() {
	if w.ia.Clicked && !w.disabled && !w.n.disabled {
		*w.val = !*w.val
		w.changed = true
		w.c.invalid = true
	}
}

// boxSize is the checkbox square of the current scope (Checkbox token).
func (c *Context) boxSize() float32 {
	return c.ThemeFor(theme.CompCheckbox).Components.Checkbox.ControlInteractiveSize
}

func (w *CheckboxW) measure(_, _ float32) geom.Size {
	bs := w.c.boxSize()
	if w.label == "" {
		return geom.Size{W: bs, H: 22}
	}
	return geom.Size{W: bs + 8 + text.Width(w.label, w.c.choiceFont()), H: 22}
}

func (c *Context) choiceFont() text.Style {
	th := c.ThemeFor(theme.CompCheckbox)
	st := c.style(th.FontSize, text.Regular)
	st.Weight = themeWeight(th.Components.Checkbox.LabelFontWeight)
	return st
}

// The box sits at the inline start, the label after it.
func (w *CheckboxW) paint(l *paint.List, r geom.Rect) {
	c, th := w.c, w.c.ThemeFor(theme.CompCheckbox)
	dis := w.disabled || w.n.disabled
	bs := c.boxSize()
	s := c.inline(r)
	box := s.Rect(0, r.Y+(r.H-bs)/2, bs, bs)
	drawCheckbox(c, l, box, checkLook{
		checked:       *w.val,
		indeterminate: w.indeterminate,
		disabled:      dis,
		hovered:       w.ia.Hovered && !dis,
		focused:       w.ia.FocusVisible,
		invalid:       w.status == "error",
		t:             c.Anim(w.n.id, "check", b2f(*w.val), th.DurationMid),
	})
	if w.label != "" {
		st := c.choiceFont()
		lay := text.Shape(w.label, st, c.textOpts(text.Options{NoWrap: true}))
		col := th.Text
		if th.Flavor == theme.Element {
			col = th.TextSecondary
			if *w.val {
				col = th.Primary.Base
			}
		}
		if dis {
			col = th.TextDisabled
		}
		l.Glyphs(geom.Pt(s.X(bs+8, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	}
}

// checkLook is the state of a checkbox square drawn by drawCheckbox.
type checkLook struct {
	checked       bool
	indeterminate bool // drawn when not checked
	disabled      bool
	hovered       bool
	focused       bool    // keyboard focus ring
	invalid       bool    // aria-invalid (shadcn: destructive border / ring)
	t             float32 // check animation progress 0..1; static rows pass 1
}

// drawCheckbox paints the checkbox square (checked, half-checked or empty)
// into box in the active design system's style. Checkbox uses it, and so
// does every widget that paints checkbox rows itself (Cascader, TreeSelect,
// Transfer) so all of them look the same.
func drawCheckbox(c *Context, l *paint.List, box geom.Rect, s checkLook) {
	th := c.ThemeFor(theme.CompCheckbox)
	tk := &th.Components.Checkbox
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	bs := box.W
	rad := geom.Rad(tk.Radius)
	switch {
	case s.disabled && shad:
		// shadcn fades the whole control instead of graying it.
		l.Opacity(0.5)
		defer l.Pop()
		bg := th.BgContainer
		if s.checked || s.indeterminate {
			bg = tk.CheckedBg
		}
		l.Fill(box, rad, bg)
		l.Stroke(box, rad, 1, theme.Lerp(tk.BorderColor, tk.CheckedBg, b2f(s.checked || s.indeterminate)))
	case s.disabled:
		l.Fill(box, rad, th.BgContainerDisabled)
		l.Stroke(box, rad, 1, tk.BorderColor)
	case s.checked:
		bg := tk.CheckedBg
		if s.hovered {
			bg = tk.CheckedHoverBg
		}
		l.Fill(box, rad, theme.Lerp(th.BgContainer, bg, s.t))
		l.Stroke(box, rad, 1, theme.Lerp(tk.BorderColor, bg, s.t))
	default:
		bc := tk.BorderColor
		if (s.hovered || s.focused) && !shad {
			bc = th.Primary.Base
		}
		if shad && s.invalid {
			bc = th.Error.Base
		}
		if len(tk.Shadow) > 0 {
			l.Shadows(box, rad, tk.Shadow)
		}
		l.Fill(box, rad, tk.Bg) // shadcn dark: bg-input/30
		l.Stroke(box, rad, 1, bc)
	}
	mark := tk.CheckColor
	if s.disabled && !shad {
		mark = th.TextDisabled
	}
	if s.checked && s.t > 0 && shad {
		// lucide check, size-3.5 inside the size-4 box.
		l.Opacity(s.t)
		icon.Draw(l, icon.Check, box.Expand(-bs/16), mark, 0)
		l.Pop()
	} else if s.checked && s.t > 0 {
		k := bs / 16
		p := paint.NewPath().MoveTo(box.X+4*k, box.Y+8.2*k).LineTo(box.X+6.9*k, box.Y+11*k).LineTo(box.X+12*k, box.Y+5.4*k)
		l.Opacity(s.t)
		l.StrokePath(p, paint.StrokeStyle{Width: 2 * k, Cap: paint.CapSquare}, mark)
		l.Pop()
	} else if s.indeterminate && !s.checked {
		col := tk.CheckedBg
		if s.disabled && !shad {
			col = th.TextDisabled
		}
		if shad {
			l.Fill(box, rad, col)
			l.Fill(geom.Rect{X: box.X + 4, Y: box.Y + bs/2 - 1, W: bs - 8, H: 2}, geom.Rad(1), tk.CheckColor)
		} else if el {
			l.Fill(box, rad, col)
			l.Fill(geom.Rect{X: box.X + 3, Y: box.Y + bs/2 - 1, W: bs - 6, H: 2}, geom.Radii{}, th.BgContainer)
		} else {
			l.Fill(geom.Rect{X: box.X + bs/4, Y: box.Y + bs/4, W: bs / 2, H: bs / 2}, geom.Radii{}, col)
		}
	}
	if s.focused {
		if shad {
			shadFocusRing(l, th, box, rad, s.invalid)
		} else {
			l.Stroke(box.Expand(3), rad.Grow(3), 2, th.FocusRing)
		}
	}
}

// CheckboxGroup renders one checkbox per option bound to the selected
// values. It returns true when the selection changed.
func CheckboxGroup(c *Context, selected *[]string, opts []Option) bool {
	changed := false
	gap := float32(8)
	if c.Theme.Flavor == theme.Element {
		gap = 30
	}
	Row(c, func() {
		for _, o := range opts {
			on := contains(*selected, o.Value)
			if Checkbox(c, o.Label, &on).Disabled(o.Disabled).Changed() {
				changed = true
				if on {
					*selected = append(*selected, o.Value)
				} else {
					*selected = remove(*selected, o.Value)
				}
			}
		}
	}, layout.Gap(gap), layout.WrapItems(), layout.Middle())
	return changed
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func remove(s []string, v string) []string {
	out := s[:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Radio

// RadioW is a single radio button.
type RadioW struct {
	c        *Context
	n        *node
	label    string
	checked  bool
	disabled bool
	status   string // "error": invalid (shadcn destructive border)
	ia       Interaction
}

// Radio adds a radio button; it reports Clicked so the caller can select it.
func Radio(c *Context, label string, checked bool) *RadioW {
	w := &RadioW{c: c, label: label, checked: checked, status: c.itemStatus}
	w.n = c.add("radio", label)
	w.n.setRole(a11y.RoleRadio).setState(a11y.Checkable).stateIf(checked, a11y.Checked)
	w.n.ln.NoStretch = true
	w.n.ln.Measure = func(_, _ float32) geom.Size {
		bs := c.ThemeFor(theme.CompRadio).Components.Radio.RadioSize
		return geom.Size{W: bs + 8 + text.Width(label, c.choiceFont()), H: 22}
	}
	w.n.paint = w.paint
	w.ia = c.interact(w.n, true, input.CursorPointer)
	return w
}

func (w *RadioW) Disabled(on bool) *RadioW {
	if on {
		w.disabled, w.n.focus, w.n.cursor = true, false, input.CursorNotAllowed
	}
	return w
}

// Status sets the validation status ("error" marks the radio invalid).
func (w *RadioW) Status(s string) *RadioW { w.status = s; return w }

// Clicked reports activation.
func (w *RadioW) Clicked() bool { return w.ia.Clicked && !w.disabled && !w.n.disabled }

// The circle sits at the inline start, the label after it.
func (w *RadioW) paint(l *paint.List, r geom.Rect) {
	c, th := w.c, w.c.ThemeFor(theme.CompRadio)
	tk := &th.Components.Radio
	dis := w.disabled || w.n.disabled
	bs := tk.RadioSize
	s := c.inline(r)
	cx, cy := s.X(0, bs)+bs/2, r.Y+r.H/2
	t := c.Anim(w.n.id, "on", b2f(w.checked), th.DurationMid)
	outer := paint.NewPath().Circle(cx, cy, bs/2)
	inner := paint.NewPath().Circle(cx, cy, bs/2-1)
	hov := w.ia.Hovered && !dis
	border := tk.BorderColor
	if (hov || w.ia.FocusVisible) && th.Flavor != theme.StyleShadcn {
		border = th.Primary.Base
	}
	if th.Flavor == theme.StyleShadcn {
		// shadcn: input-colored ring with a primary dot; disabled fades.
		if dis {
			l.Opacity(0.5)
		}
		if sh := th.Components.Checkbox.Shadow; len(sh) > 0 {
			l.Shadows(geom.Rect{X: cx - bs/2, Y: cy - bs/2, W: bs, H: bs}, geom.Rad(bs/2), sh)
		}
		ring := tk.BorderColor
		if w.status == "error" {
			ring = th.Error.Base
		}
		l.Path(outer, ring)
		l.Path(inner, th.ControlBg)
		if t > 0 {
			l.Path(paint.NewPath().Circle(cx, cy, tk.DotSize/2*t), tk.DotColor)
		}
		if dis {
			l.Pop()
		}
	} else if dis {
		l.Path(outer, tk.BorderColor)
		l.Path(inner, th.BgContainerDisabled)
		if w.checked {
			l.Path(paint.NewPath().Circle(cx, cy, bs/4), th.TextDisabled)
		}
	} else {
		// Ant / Element: the circle fills with DotColor around a
		// background-colored dot.
		on := tk.DotColor
		l.Path(outer, theme.Lerp(border, on, t))
		l.Path(inner, theme.Lerp(th.BgContainer, on, t))
		if t > 0 {
			dot := tk.DotSize / 2 * t
			if th.Flavor == theme.Ant {
				dot = tk.DotSize / 2 * float32(math.Max(0.4, float64(t)))
			}
			l.Path(paint.NewPath().Circle(cx, cy, dot), th.BgContainer)
		}
	}
	if w.ia.FocusVisible {
		if th.Flavor == theme.StyleShadcn {
			box := geom.Rect{X: cx - bs/2, Y: cy - bs/2, W: bs, H: bs}
			shadFocusRing(l, th, box, geom.Rad(bs/2), w.status == "error")
		} else {
			l.StrokePath(paint.NewPath().Circle(cx, cy, bs/2+2), paint.StrokeStyle{Width: 2}, th.FocusRing)
		}
	}
	if w.label != "" {
		lay := text.Shape(w.label, c.choiceFont(), c.textOpts(text.Options{NoWrap: true}))
		col := th.Text
		if th.Flavor == theme.Element {
			col = th.TextSecondary
			if w.checked {
				col = th.Primary.Base
			}
		}
		if dis {
			col = th.TextDisabled
		}
		l.Glyphs(geom.Pt(s.X(bs+8, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	}
}

// RadioGroupW is a group of mutually exclusive options.
type RadioGroupW struct {
	c       *Context
	n       *node
	val     *string
	opts    []Option
	buttons bool
	solid   bool
	size    ControlSize
	changed bool
	z       *lazy
}

// RadioGroup adds radio options bound to *val.
func RadioGroup(c *Context, val *string, opts []Option) *RadioGroupW {
	g := &RadioGroupW{c: c, val: val, opts: opts, size: c.defaultSize()}
	g.configDefaults()
	g.n = c.add("radiogroup", "")
	g.n.setRole(a11y.RoleRadioGroup).setState(a11y.Horizontal)
	g.n.ln.Dir = layout.Row
	g.n.ln.Align = layout.AlignCenter
	g.n.ln.NoStretch = true
	g.z = c.lazy(g.n, g.build)
	return g
}

// Buttons renders the group as segmented buttons.
func (g *RadioGroupW) Buttons() *RadioGroupW { g.buttons = true; return g }

// Solid fills the selected button with the primary color.
func (g *RadioGroupW) Solid() *RadioGroupW { g.buttons, g.solid = true, true; return g }

// Size sets the button size.
func (g *RadioGroupW) Size(s ControlSize) *RadioGroupW { g.size = s; return g }

// Changed reports a selection change this frame.
func (g *RadioGroupW) Changed() bool { g.z.run(); return g.changed }

// A11yLabel names the group for screen readers (inside a FormItem the
// item label is used).
func (g *RadioGroupW) A11yLabel(s string) *RadioGroupW { g.n.setName(s); return g }

// keys moves the selection with the arrow keys (radio groups select on
// focus) and leaves only the selected radio as a Tab stop.
func (g *RadioGroupW) keys(nodes []*node) {
	c := g.c
	ids := make([]ID, len(nodes))
	sel := -1
	for i, n := range nodes {
		ids[i] = n.id
		if g.opts[i].Value == *g.val {
			sel = i
		}
	}
	rovingTabStop(nodes, sel)
	to := rovingStep(c, ids, func(i int) bool { return !g.opts[i].Disabled && !nodes[i].disabled }, rovingBoth)
	if to >= 0 && g.opts[to].Value != *g.val {
		*g.val = g.opts[to].Value
		g.changed = true
	}
	if to >= 0 {
		navFocus(c, ids[to])
	}
}

func (g *RadioGroupW) build() {
	c := g.c
	if !g.buttons {
		g.n.ln.Gap = 8
		if c.Theme.Flavor == theme.Element {
			g.n.ln.Gap = 30
		}
		g.n.ln.Wrap = true
		nodes := make([]*node, len(g.opts))
		for i, o := range g.opts {
			r := Radio(c, o.Label, *g.val == o.Value).Disabled(o.Disabled)
			nodes[i] = r.n
			if r.Clicked() && *g.val != o.Value {
				*g.val = o.Value
				g.changed = true
				c.invalid = true
			}
		}
		g.keys(nodes)
		return
	}
	th := c.ThemeFor(theme.CompRadio)
	tk := &th.Components.Radio
	// Logical end radii: the first button rounds its inline-start corners,
	// the last its inline-end ones.
	ends := func(first, last bool) geom.Radii {
		rad := controlRadiusOf(th, g.size)
		left, right := first, last
		if c.IsRTL() {
			left, right = last, first
		}
		var radii geom.Radii
		if left {
			radii.TL, radii.BL = rad, rad
		}
		if right {
			radii.TR, radii.BR = rad, rad
		}
		return radii
	}
	nodes := make([]*node, 0, len(g.opts))
	for i, o := range g.opts {
		i, o := i, o
		n := c.add("radiobtn", o.Label)
		nodes = append(nodes, n)
		n.setRole(a11y.RoleRadio).setState(a11y.Checkable).stateIf(*g.val == o.Value, a11y.Checked)
		first, last := i == 0, i == len(g.opts)-1
		n.ln.Measure = func(_, _ float32) geom.Size {
			pad := tk.ButtonPaddingInline
			if g.size == Small {
				pad = tk.ButtonPaddingInlineSM
			}
			return geom.Size{W: text.Width(o.Label, c.style(controlFontOf(th, g.size), text.Regular)) + 2*pad, H: controlHeightOf(th, g.size)}
		}
		if !first {
			n.ln.Margin.Left = -1
		}
		ia := c.interact(n, true, input.CursorPointer)
		if o.Disabled {
			n.disabled = true
		}
		sel := *g.val == o.Value
		if ia.Clicked && !n.disabled && !sel {
			*g.val = o.Value
			g.changed = true
			c.invalid = true
			sel = true
		}
		n.paint = func(l *paint.List, r geom.Rect) {
			radii := ends(first, last)
			hov := c.Anim(n.id, "hover", b2f(ia.Hovered && !n.disabled), th.DurationMid)
			bg, border, fg := tk.ButtonBg, th.Border, tk.ButtonColor
			shad := th.Flavor == theme.StyleShadcn
			switch {
			case shad && n.disabled:
				fg = th.TextDisabled
			case shad && sel && g.solid:
				bg, border, fg = tk.ButtonSolidCheckedBg, tk.ButtonSolidCheckedBg, tk.ButtonSolidCheckedColor
			case shad && sel:
				bg, fg = tk.ButtonCheckedBg, tk.ButtonCheckedColor
			case shad:
				bg = theme.Lerp(tk.ButtonBg, th.ItemHoverBg, hov)
			case n.disabled:
				bg, fg = th.BgContainerDisabled, th.TextDisabled
			case sel && g.solid:
				bg = theme.Lerp(tk.ButtonSolidCheckedBg, th.Primary.Hover, hov)
				border, fg = bg, tk.ButtonSolidCheckedColor
			case sel:
				border = theme.Lerp(th.Primary.Base, th.Primary.Hover, hov)
				bg, fg = tk.ButtonCheckedBg, border
				if th.Flavor == theme.Element {
					fg = tk.ButtonCheckedColor // white on the brand fill
				}
			default:
				fg = theme.Lerp(tk.ButtonColor, th.Primary.Base, hov)
			}
			l.Fill(r, radii, bg)
			l.Stroke(r, radii, 1, border)
			lay := text.Shape(o.Label, c.style(controlFontOf(th, g.size), text.Regular), c.textOpts(text.Options{NoWrap: true}))
			l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
			if ia.FocusVisible {
				if shad {
					shadFocusRing(l, th, r, radii, false)
				} else {
					l.Stroke(r.Expand(2), radii.Grow(2), 2, th.FocusRing)
				}
			}
		}
	}
	g.keys(nodes)
	// The selected button's border must sit above its neighbours' (they
	// overlap by 1px), so redraw it after all children.
	g.n.over = func(l *paint.List, _ geom.Rect) {
		if th.Flavor == theme.StyleShadcn {
			return // shadcn marks the selection with a fill, not a border
		}
		for i, k := range g.n.kids {
			if i >= len(g.opts) || g.opts[i].Value != *g.val || k.disabled {
				continue
			}
			radii := ends(i == 0, i == len(g.opts)-1)
			col := th.Primary.Base
			if c.hovered(k.id) {
				col = th.Primary.Hover
			}
			l.Stroke(k.abs, radii, 1, col)
		}
	}
}

func (g *RadioGroupW) selectedLabel() string {
	for _, o := range g.opts {
		if o.Value == *g.val {
			return o.Label
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Switch

// SwitchW is a toggle switch.
type SwitchW struct {
	c        *Context
	n        *node
	val      *bool
	disabled bool
	loading  bool
	small    bool
	onText   string
	offText  string
	changed  bool
	ia       Interaction
	z        *lazy
}

// Switch adds a switch bound to *val.
func Switch(c *Context, val *bool) *SwitchW {
	w := &SwitchW{c: c, val: val, small: c.defaultSize() == Small}
	w.configDefaults()
	w.n = c.add("switch", "")
	w.n.setRole(a11y.RoleSwitch).setState(a11y.Checkable).fill(w)
	w.n.ln.NoStretch = true
	w.n.ln.AlignSelf = layout.AlignAuto
	w.n.ln.Measure = w.measure
	w.n.paint = w.paint
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.z = c.lazy(w.n, func() {
		if w.ia.Clicked && !w.disabled && !w.loading && !w.n.disabled {
			*w.val = !*w.val
			w.changed = true
			c.invalid = true
		}
	})
	return w
}

func (w *SwitchW) Small() *SwitchW               { w.small = true; return w }
func (w *SwitchW) Loading(on bool) *SwitchW      { w.loading = on; return w }
func (w *SwitchW) Texts(on, off string) *SwitchW { w.onText, w.offText = on, off; return w }
func (w *SwitchW) Disabled(on bool) *SwitchW {
	if on {
		w.disabled, w.n.focus, w.n.cursor = true, false, input.CursorNotAllowed
	}
	return w
}
func (w *SwitchW) Changed() bool { w.z.run(); return w.changed }

// A11yLabel sets the accessible name (a switch has no visible label; inside
// a FormItem the item label is used).
func (w *SwitchW) A11yLabel(s string) *SwitchW { w.n.setName(s); return w }

func (w *SwitchW) semantics(s *a11y.Node) {
	if *w.val {
		s.States |= a11y.Checked
		s.Value = w.onText
	} else {
		s.Value = w.offText
	}
	if w.loading {
		s.States |= a11y.Busy
	}
}

func (w *SwitchW) dims() (h, minW, handle float32) {
	tk := &w.c.ThemeFor(theme.CompSwitch).Components.Switch
	if w.small {
		return tk.TrackHeightSM, tk.TrackMinWidthSM, tk.HandleSizeSM
	}
	return tk.TrackHeight, tk.TrackMinWidth, tk.HandleSize
}

func (w *SwitchW) measure(_, _ float32) geom.Size {
	h, mw, hs := w.dims()
	tw := float32(0)
	st := w.c.style(12, text.Regular)
	if w.onText != "" || w.offText != "" {
		tw = max(text.Width(w.onText, st), text.Width(w.offText, st)) + hs + 2 + 9 + 6
	}
	return geom.Size{W: max(mw, tw), H: h}
}

// The thumb rests at the inline start when off and moves to the end when
// on (to the left in RTL); the on/off text sits on the other side.
func (w *SwitchW) paint(l *paint.List, r geom.Rect) {
	c, th := w.c, w.c.ThemeFor(theme.CompSwitch)
	tk := &th.Components.Switch
	h, _, hs := w.dims()
	t := c.Anim(w.n.id, "on", b2f(*w.val), th.DurationMid)
	hov := w.ia.Hovered && !w.disabled
	off, on := tk.OffBg, tk.OnBg
	if hov {
		off, on = tk.OffHoverBg, tk.OnHoverBg
	}
	track := geom.Rect{X: r.X, Y: r.Y + (r.H-h)/2, W: r.W, H: h}
	if w.disabled || w.n.disabled || w.loading {
		op := float32(0.65)
		if th.Flavor == theme.StyleShadcn {
			op = 0.5
		}
		l.Opacity(op)
		defer l.Pop()
	}
	if len(tk.TrackShadow) > 0 {
		l.Shadows(track, geom.Rad(h/2), tk.TrackShadow)
	}
	l.Fill(track, geom.Rad(h/2), theme.Lerp(off, on, t))
	pad := (h - hs) / 2
	s := c.inline(track)
	hr := s.Rect(pad+(track.W-2*pad-hs)*t, track.Y+pad, hs, hs)
	if len(tk.HandleShadow) > 0 {
		l.Shadows(hr, geom.Rad(hs/2), tk.HandleShadow)
	}
	// shadcn dark: foreground thumb when off.
	l.Fill(hr, geom.Rad(hs/2), theme.Lerp(tk.HandleOffBg, tk.HandleBg, t))
	if w.loading {
		drawIconRot(l, hr.Expand(-3), th.Primary.Base, spinAngle(c))
	}
	label := w.offText
	if *w.val {
		label = w.onText
	}
	if label != "" {
		lay := text.Shape(label, c.style(12, text.Regular), c.textOpts(text.Options{NoWrap: true}))
		lo := hs + pad + 6
		if *w.val {
			lo = 9
		}
		l.Glyphs(geom.Pt(s.X(lo, lay.Width), track.Y+(h-lay.Height)/2), lay.Glyphs, th.TextOnPrimary)
	}
	if w.ia.FocusVisible {
		if th.Flavor == theme.StyleShadcn {
			shadFocusRing(l, th, track, geom.Rad(h/2), false)
		} else {
			l.Stroke(track.Expand(2), geom.Rad(h/2+2), 2, th.FocusRing)
		}
	}
}

// Slider: see p2b_slider.go.

func drawIconRot(l *paint.List, r geom.Rect, col theme.Color, ang float64) {
	p := paint.NewPath().Arc(r.X+r.W/2, r.Y+r.H/2, r.W/2-1, ang, 1.5*math.Pi)
	l.StrokePath(p, paint.StrokeStyle{Width: 1.5, Cap: paint.CapRound}, col)
}
