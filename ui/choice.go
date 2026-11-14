package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"math"
	"sync/atomic"

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
	_ [0]func()

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
	_ [0]func()

	c             *Context
	n             *node
	label         string
	val           *bool
	indeterminate bool
	disabled      bool
	changed       bool
	status        string // "error": invalid (shadcn destructive border)
	size          ControlSize
	look          choiceLook
	// first / last: a button-style checkbox rounds its inline-start /
	// inline-end corners (both when standalone; the group clears the
	// inner ones).
	first, last bool
	ia          Interaction
	z           *lazy
}

// choiceLook is the frame style of a Checkbox / Radio.
type choiceLook uint8

const (
	lookPlain  choiceLook = iota
	lookBorder            // a bordered box around control and label
	lookButton            // a segmented button (checkbox button)
)

// Checkbox adds a checkbox bound to *val.
func Checkbox(c *Context, label string, val *bool) *CheckboxW {
	w := &CheckboxW{c: c, label: label, val: val, status: c.itemStatus, size: c.defaultSize(), first: true, last: true}
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

// Size sets the checkbox size: the label font, the box (Element shrinks
// it when small) and the height of the Border / Button styles.
func (w *CheckboxW) Size(s ControlSize) *CheckboxW { w.size = s; return w }

// Border draws a frame around the box and the label (Element
// el-checkbox border); a checked one gets a brand-colored frame.
func (w *CheckboxW) Border() *CheckboxW { w.look = lookBorder; return w }

// Button renders the checkbox as a toggle button without a box (Element
// el-checkbox-button); checked buttons take the brand color. Inside
// CheckboxGroup(...).Buttons() the buttons join into a segmented row.
func (w *CheckboxW) Button() *CheckboxW { w.look = lookButton; return w }

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

// boxSizeOf is the checkbox square of a checkbox of size s.
func (c *Context) boxSizeOf(s ControlSize) float32 {
	tk := &c.ThemeFor(theme.CompCheckbox).Components.Checkbox
	if s == Small {
		return tk.ControlInteractiveSizeSM
	}
	return tk.ControlInteractiveSize
}

func (w *CheckboxW) measure(_, _ float32) geom.Size {
	c := w.c
	th := c.ThemeFor(theme.CompCheckbox)
	tk := &th.Components.Checkbox
	switch w.look {
	case lookButton:
		pad := tk.ButtonPaddingInline
		if w.size == Small {
			pad = tk.ButtonPaddingInlineSM
		}
		return geom.Size{W: text.Width(w.label, c.style(controlFontOf(th, w.size), text.Regular)) + 2*pad, H: controlHeightOf(th, w.size)}
	case lookBorder:
		sz := choiceContentSize(c, w.label, c.boxSizeOf(w.size), w.size)
		return geom.Size{W: tk.BorderedPaddingStart + sz.W + tk.BorderedPaddingEnd, H: controlHeightOf(th, w.size)}
	}
	return choiceContentSize(c, w.label, c.boxSizeOf(w.size), w.size)
}

// choiceContentSize is a control mark of size bs followed by an optional
// label (plain Checkbox / Radio).
func choiceContentSize(c *Context, label string, bs float32, s ControlSize) geom.Size {
	if label == "" {
		return geom.Size{W: bs, H: 22}
	}
	return geom.Size{W: bs + 8 + text.Width(label, c.choiceFontOf(s)), H: 22}
}

func (c *Context) choiceFont() text.Style { return c.choiceFontOf(Middle) }

// choiceFontOf is the label font of a Checkbox / Radio of size s.
func (c *Context) choiceFontOf(s ControlSize) text.Style {
	th := c.ThemeFor(theme.CompCheckbox)
	fs := th.FontSize
	if s == Small {
		fs = th.FontSizeSM
	}
	st := c.style(fs, text.Regular)
	st.Weight = themeWeight(th.Components.Checkbox.LabelFontWeight)
	return st
}

// drawChoiceFrame paints the frame of a bordered Checkbox / Radio (r is
// the whole control) and, when focused, the focus ring around it.
func drawChoiceFrame(l *paint.List, th *theme.Theme, r geom.Rect, size ControlSize, checked, disabled, hovered, focused bool, checkedCol theme.Color) {
	rad := geom.Rad(controlRadiusOf(th, size))
	shad := th.Flavor == theme.StyleShadcn
	bc := th.Border
	switch {
	case disabled && !shad:
		l.Fill(r, rad, th.BgContainerDisabled)
		bc = th.BorderSecondary
	case checked:
		bc = checkedCol
	case hovered && !shad:
		bc = th.Primary.Base
	}
	if disabled && shad {
		l.Opacity(0.5)
		defer l.Pop()
	}
	l.Stroke(r, rad, 1, bc)
	if focused {
		if shad {
			shadFocusRing(l, th, r, rad, false)
		} else {
			l.Stroke(r.Expand(2), rad.Grow(2), 2, th.FocusRing)
		}
	}
}

// segRadii are the corner radii of a button in a segmented row: the first
// button rounds its inline-start corners, the last its inline-end ones.
func segRadii(c *Context, th *theme.Theme, size ControlSize, first, last bool) geom.Radii {
	rad := controlRadiusOf(th, size)
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

// segLook is the state of a segmented choice button.
type segLook struct {
	selected, disabled, solid bool
	hover                     float32 // hover animation 0..1
	focused                   bool
}

// segColors are the token colors of a segmented choice button.
type segColors struct {
	bg, checkedBg, fg, checkedFg, solidBg, solidFg theme.Color
	filledChecked                                  bool // checkedFg on a brand fill (Element)
}

// drawSegButton paints a radio / checkbox button with its label centered.
func drawSegButton(c *Context, l *paint.List, th *theme.Theme, r geom.Rect, radii geom.Radii, label string, font text.Style, s segLook, k segColors) {
	hov := s.hover
	bg, border, fg := k.bg, th.Border, k.fg
	shad := th.Flavor == theme.StyleShadcn
	switch {
	case shad && s.disabled:
		fg = th.TextDisabled
		if s.selected {
			bg = k.checkedBg
		}
	case shad && s.selected && s.solid:
		bg, border, fg = k.solidBg, k.solidBg, k.solidFg
	case shad && s.selected:
		bg, fg = k.checkedBg, k.checkedFg
	case shad:
		bg = theme.Lerp(k.bg, th.ItemHoverBg, hov)
	case s.disabled:
		bg, fg = th.BgContainerDisabled, th.TextDisabled
	case s.selected && s.solid:
		bg = theme.Lerp(k.solidBg, th.Primary.Hover, hov)
		border, fg = bg, k.solidFg
	case s.selected:
		border = theme.Lerp(th.Primary.Base, th.Primary.Hover, hov)
		bg, fg = k.checkedBg, border
		if k.filledChecked {
			fg = k.checkedFg // white on the brand fill
		}
	default:
		fg = theme.Lerp(k.fg, th.Primary.Base, hov)
	}
	l.Fill(r, radii, bg)
	if s.disabled && s.selected && !shad {
		l.Fill(r, radii, th.FillSecondary) // a disabled selection stays visible
	}
	l.Stroke(r, radii, 1, border)
	lay := text.Shape(label, font, c.textOpts(text.Options{NoWrap: true}))
	l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
	if s.focused {
		if shad {
			shadFocusRing(l, th, r, radii, false)
		} else {
			l.Stroke(r.Expand(2), radii.Grow(2), 2, th.FocusRing)
		}
	}
}

// The box sits at the inline start, the label after it.
func (w *CheckboxW) paint(l *paint.List, r geom.Rect) {
	c, th := w.c, w.c.ThemeFor(theme.CompCheckbox)
	tk := &th.Components.Checkbox
	dis := w.disabled || w.n.disabled
	if w.look == lookButton {
		k := segColors{bg: tk.ButtonBg, checkedBg: tk.ButtonCheckedBg, fg: tk.ButtonColor, checkedFg: tk.ButtonCheckedColor,
			solidBg: tk.ButtonCheckedBg, solidFg: tk.ButtonCheckedColor, filledChecked: th.Flavor == theme.Element}
		drawSegButton(c, l, th, r, segRadii(c, th, w.size, w.first, w.last), w.label, c.style(controlFontOf(th, w.size), text.Regular), segLook{
			selected: *w.val, disabled: dis, focused: w.ia.FocusVisible,
			hover: c.Anim(w.n.id, "hover", b2f(w.ia.Hovered && !dis), th.DurationMid),
		}, k)
		return
	}
	focusBox := w.ia.FocusVisible
	if w.look == lookBorder {
		drawChoiceFrame(l, th, r, w.size, *w.val, dis, w.ia.Hovered && !dis, w.ia.FocusVisible, tk.BorderedCheckedColor)
		focusBox = false
		r = c.inline(r).Rect(tk.BorderedPaddingStart, r.Y, r.W-tk.BorderedPaddingStart, r.H)
	}
	bs := c.boxSizeOf(w.size)
	s := c.inline(r)
	box := s.Rect(0, r.Y+(r.H-bs)/2, bs, bs)
	drawCheckbox(c, l, box, checkLook{
		checked:       *w.val,
		indeterminate: w.indeterminate,
		disabled:      dis,
		hovered:       w.ia.Hovered && !dis,
		focused:       focusBox,
		invalid:       w.status == "error",
		t:             c.Anim(w.n.id, "check", b2f(*w.val), th.DurationMid),
	})
	if w.label != "" {
		st := c.choiceFontOf(w.size)
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

// CheckboxGroupW is the checkbox group CheckboxGroup returns.
type CheckboxGroupW struct {
	_        [0]func()
	c        *Context
	n        *node
	sel      *[]string
	opts     []Option
	disabled bool
	min, max int
	look     choiceLook
	size     ControlSize
	changed  bool
	z        *lazy
}

// CheckboxGroup renders one checkbox per option bound to the selected
// values. Changed on the result reports a selection change; the builder
// also takes Disabled, Min/Max, Border, Buttons and Size.
func CheckboxGroup(c *Context, selected *[]string, opts []Option) *CheckboxGroupW {
	g := &CheckboxGroupW{c: c, sel: selected, opts: opts, size: c.defaultSize()}
	g.n = c.add("checkboxgroup", "")
	g.n.setRole(a11y.RoleGroup)
	g.n.ln.Dir = layout.Row
	g.n.ln.Align = layout.AlignCenter
	g.n.ln.NoStretch = true
	g.z = c.lazy(g.n, g.build)
	return g
}

// Disabled disables every checkbox of the group.
func (g *CheckboxGroupW) Disabled(on bool) *CheckboxGroupW { g.disabled = on; return g }

// Min keeps at least n values selected: while only n are checked they
// cannot be unchecked (0 = no minimum).
func (g *CheckboxGroupW) Min(n int) *CheckboxGroupW { g.min = n; return g }

// Max allows at most n selected values: once n are checked the others are
// disabled (0 = no maximum).
func (g *CheckboxGroupW) Max(n int) *CheckboxGroupW { g.max = n; return g }

// Border draws every checkbox with a frame (see CheckboxW.Border).
func (g *CheckboxGroupW) Border() *CheckboxGroupW { g.look = lookBorder; return g }

// Buttons renders the options as a row of joined toggle buttons (Element
// el-checkbox-button).
func (g *CheckboxGroupW) Buttons() *CheckboxGroupW { g.look = lookButton; return g }

// Size sets the size of the checkboxes.
func (g *CheckboxGroupW) Size(s ControlSize) *CheckboxGroupW { g.size = s; return g }

// A11yLabel names the group for screen readers.
func (g *CheckboxGroupW) A11yLabel(s string) *CheckboxGroupW { g.n.setName(s); return g }

// Changed reports a selection change this frame.
func (g *CheckboxGroupW) Changed() bool { g.z.run(); return g.changed }

func (g *CheckboxGroupW) build() {
	c := g.c
	if g.disabled || g.n.disabled {
		c.disabledDepth++
		defer func() { c.disabledDepth-- }()
	}
	switch {
	case g.look == lookButton:
		g.n.ln.Gap = 0
	case g.look == lookBorder:
		g.n.ln.Gap, g.n.ln.Wrap = 8, true
		if c.Theme.Flavor == theme.Element {
			g.n.ln.Gap = 10
		}
	default:
		g.n.ln.Gap, g.n.ln.Wrap = 8, true
		if c.Theme.Flavor == theme.Element {
			g.n.ln.Gap = 30
		}
	}
	count := 0
	for _, o := range g.opts {
		if contains(*g.sel, o.Value) {
			count++
		}
	}
	boxes := make([]*CheckboxW, len(g.opts))
	for i, o := range g.opts {
		on := contains(*g.sel, o.Value)
		dis := o.Disabled || (g.max > 0 && !on && count >= g.max) || (g.min > 0 && on && count <= g.min)
		cb := Checkbox(c, o.Label, &on).Disabled(dis).Size(g.size)
		boxes[i] = cb
		switch g.look {
		case lookBorder:
			cb.Border()
		case lookButton:
			cb.Button()
			cb.first, cb.last = i == 0, i == len(g.opts)-1
			if i > 0 {
				cb.n.ln.Margin.Left = -1
			}
		}
		if cb.Changed() {
			g.changed = true
			if on {
				*g.sel = append(*g.sel, o.Value)
			} else {
				*g.sel = remove(*g.sel, o.Value)
			}
		}
	}
	if g.look != lookButton {
		return
	}
	// Checked buttons' borders must sit above their neighbours' (they
	// overlap by 1px), so redraw them after all children.
	g.n.over = func(l *paint.List, _ geom.Rect) {
		th := c.ThemeFor(theme.CompCheckbox)
		if th.Flavor == theme.StyleShadcn {
			return // shadcn marks the selection with a fill
		}
		for i, cb := range boxes {
			if !*cb.val || cb.disabled || cb.n.disabled {
				continue
			}
			col := th.Components.Checkbox.ButtonCheckedBorder
			if c.hovered(cb.n.id) {
				col = th.Primary.Hover
			}
			l.Stroke(cb.n.abs, segRadii(c, th, g.size, i == 0, i == len(boxes)-1), 1, col)
		}
	}
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
	_ [0]func()

	c        *Context
	n        *node
	label    string
	checked  bool
	disabled bool
	status   string // "error": invalid (shadcn destructive border)
	size     ControlSize
	border   bool
	ia       Interaction
}

// Radio adds a radio button; it reports Clicked so the caller can select it.
func Radio(c *Context, label string, checked bool) *RadioW {
	w := &RadioW{c: c, label: label, checked: checked, status: c.itemStatus, size: c.defaultSize()}
	w.n = c.add("radio", label)
	w.n.setRole(a11y.RoleRadio).setState(a11y.Checkable).stateIf(checked, a11y.Checked)
	w.n.ln.NoStretch = true
	w.n.ln.Measure = w.measure
	w.n.paint = w.paint
	w.ia = c.interact(w.n, true, input.CursorPointer)
	return w
}

func (w *RadioW) measure(_, _ float32) geom.Size {
	c := w.c
	th := c.ThemeFor(theme.CompRadio)
	tk := &th.Components.Radio
	if w.label == "" && !w.border {
		return geom.Size{W: tk.RadioSize, H: 22}
	}
	sz := geom.Size{W: tk.RadioSize + 8 + text.Width(w.label, c.choiceFontOf(w.size)), H: 22}
	if w.border {
		sz = geom.Size{W: tk.BorderedPaddingStart + sz.W + tk.BorderedPaddingEnd, H: controlHeightOf(th, w.size)}
	}
	return sz
}

// Size sets the radio size: the label font and the height of the Border
// style.
func (w *RadioW) Size(s ControlSize) *RadioW { w.size = s; return w }

// Border draws a frame around the circle and the label (Element el-radio
// border); a checked one gets a brand-colored frame.
func (w *RadioW) Border() *RadioW { w.border = true; return w }

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
	focusDot := w.ia.FocusVisible
	if w.border {
		drawChoiceFrame(l, th, r, w.size, w.checked, dis, w.ia.Hovered && !dis, w.ia.FocusVisible, tk.BorderedCheckedColor)
		focusDot = false
		r = c.inline(r).Rect(tk.BorderedPaddingStart, r.Y, r.W-tk.BorderedPaddingStart, r.H)
	}
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
	if focusDot {
		if th.Flavor == theme.StyleShadcn {
			box := geom.Rect{X: cx - bs/2, Y: cy - bs/2, W: bs, H: bs}
			shadFocusRing(l, th, box, geom.Rad(bs/2), w.status == "error")
		} else {
			l.StrokePath(paint.NewPath().Circle(cx, cy, bs/2+2), paint.StrokeStyle{Width: 2}, th.FocusRing)
		}
	}
	if w.label != "" {
		lay := text.Shape(w.label, c.choiceFontOf(w.size), c.textOpts(text.Options{NoWrap: true}))
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
	_ [0]func()

	c       *Context
	n       *node
	val     *string
	opts    []Option
	buttons bool
	solid   bool
	size    ControlSize
	// disabled disables the whole group; border frames plain radios.
	disabled bool
	border   bool
	changed  bool
	z        *lazy
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

// Size sets the size of the buttons (and of the radios: label font and
// Border height).
func (g *RadioGroupW) Size(s ControlSize) *RadioGroupW { g.size = s; return g }

// Disabled disables every option of the group (radios or buttons).
func (g *RadioGroupW) Disabled(on bool) *RadioGroupW { g.disabled = on; return g }

// Border draws each radio with a frame (see RadioW.Border); it has no
// effect on the button style.
func (g *RadioGroupW) Border() *RadioGroupW { g.border = true; return g }

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
	if g.disabled || g.n.disabled {
		c.disabledDepth++
		defer func() { c.disabledDepth-- }()
	}
	if !g.buttons {
		g.n.ln.Gap = 8
		if c.Theme.Flavor == theme.Element {
			g.n.ln.Gap = 30
			if g.border {
				g.n.ln.Gap = 10
			}
		}
		g.n.ln.Wrap = true
		nodes := make([]*node, len(g.opts))
		for i, o := range g.opts {
			r := Radio(c, o.Label, *g.val == o.Value).Disabled(o.Disabled).Size(g.size)
			if g.border {
				r.Border()
			}
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
	ends := func(first, last bool) geom.Radii { return segRadii(c, th, g.size, first, last) }
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
			k := segColors{bg: tk.ButtonBg, checkedBg: tk.ButtonCheckedBg, fg: tk.ButtonColor, checkedFg: tk.ButtonCheckedColor,
				solidBg: tk.ButtonSolidCheckedBg, solidFg: tk.ButtonSolidCheckedColor, filledChecked: th.Flavor == theme.Element}
			drawSegButton(c, l, th, r, ends(first, last), o.Label, c.style(controlFontOf(th, g.size), text.Regular), segLook{
				selected: sel, disabled: n.disabled, solid: g.solid, focused: ia.FocusVisible,
				hover: c.Anim(n.id, "hover", b2f(ia.Hovered && !n.disabled), th.DurationMid),
			}, k)
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
	_ [0]func()

	c        *Context
	n        *node
	val      *bool
	disabled bool
	loading  bool
	small    bool
	large    bool
	width    float32 // track width (0 = minimum width / fit the texts)
	onText   string
	offText  string
	onIcon   *icon.Icon
	offIcon  *icon.Icon
	outerOn  string // OuterTexts: label after the track
	outerOff string // OuterTexts: label before the track
	set      func(on bool)
	before   func(next bool) bool
	beforeAs func(next bool, done func(ok bool))
	pending  bool // an async BeforeChange is running
	changed  bool
	ia       Interaction
	z        *lazy
}

// switchBefore is the state of an asynchronous BeforeChange.
type switchBefore struct {
	res  atomic.Int32 // switchIdle / switchWaiting / switchAccepted / switchRejected
	next bool
}

const (
	switchIdle int32 = iota
	switchWaiting
	switchAccepted
	switchRejected
)

// Switch adds a switch bound to *val.
func Switch(c *Context, val *bool) *SwitchW {
	s := c.defaultSize()
	w := &SwitchW{c: c, val: val, small: s == Small, large: s == Large}
	w.configDefaults()
	w.n = c.add("switch", "")
	w.n.setRole(a11y.RoleSwitch).setState(a11y.Checkable).fill(w)
	w.n.ln.NoStretch = true
	w.n.ln.AlignSelf = layout.AlignAuto
	w.n.ln.Measure = w.measure
	w.n.paint = w.paint
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.z = c.lazy(w.n, w.finish)
	return w
}

// SwitchValue adds a switch bound to an arbitrary value (Element
// active-value / inactive-value): it is on while *val == on, and toggling
// writes on or off into *val. Any other value shows the switch off.
func SwitchValue[T comparable](c *Context, val *T, on, off T) *SwitchW {
	b := new(bool)
	*b = *val == on
	w := Switch(c, b)
	w.set = func(v bool) {
		if v {
			*val = on
		} else {
			*val = off
		}
	}
	return w
}

func (w *SwitchW) Small() *SwitchW { w.small, w.large = true, false; return w }

// Large selects the large switch.
func (w *SwitchW) Large() *SwitchW { w.small, w.large = false, true; return w }

// Size sets the switch size (Small, Middle or Large).
func (w *SwitchW) Size(s ControlSize) *SwitchW { w.small, w.large = s == Small, s == Large; return w }

// Width sets the track width in px (Element width); by default the track
// has the theme's minimum width and grows to fit the inline texts.
func (w *SwitchW) Width(px float32) *SwitchW { w.width = px; return w }

func (w *SwitchW) Loading(on bool) *SwitchW { w.loading = on; return w }

// Texts shows on / off text inside the track (Ant checkedChildren, Element
// inline-prompt).
func (w *SwitchW) Texts(on, off string) *SwitchW { w.onText, w.offText = on, off; return w }

// Icons shows on / off icons inside the track (Element inline-prompt with
// active-icon / inactive-icon). Either may be nil.
func (w *SwitchW) Icons(on, off *icon.Icon) *SwitchW { w.onIcon, w.offIcon = on, off; return w }

// OuterTexts shows labels beside the track (Element active-text /
// inactive-text): off before it, on after it; the label of the current
// state takes the brand color. Clicking a label toggles the switch.
func (w *SwitchW) OuterTexts(on, off string) *SwitchW { w.outerOn, w.outerOff = on, off; return w }

// BeforeChange is asked before every toggle with the value the switch
// would take; returning false keeps the current value.
func (w *SwitchW) BeforeChange(fn func(next bool) bool) *SwitchW { w.before = fn; return w }

// BeforeChangeAsync asks fn before every toggle; the switch shows loading
// and ignores clicks until fn calls done (from any goroutine, once): with
// true the switch takes next, with false it stays. done wakes the window.
func (w *SwitchW) BeforeChangeAsync(fn func(next bool, done func(ok bool))) *SwitchW {
	w.beforeAs = fn
	return w
}

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

func (w *SwitchW) busy() bool { return w.loading || w.pending }

func (w *SwitchW) apply(next bool) {
	*w.val = next
	if w.set != nil {
		w.set(next)
	}
	w.changed = true
	w.c.invalid = true
}

func (w *SwitchW) finish() {
	c := w.c
	var st *switchBefore
	if w.beforeAs != nil {
		st = State[switchBefore](c, hashID(w.n.id, "sw-before"), nil)
		switch st.res.Load() {
		case switchWaiting:
			w.pending = true
		case switchAccepted:
			st.res.Store(switchIdle)
			w.apply(st.next)
		case switchRejected:
			st.res.Store(switchIdle)
		}
	}
	if !w.ia.Clicked || w.disabled || w.busy() || w.n.disabled {
		return
	}
	next := !*w.val
	if w.before != nil && !w.before(next) {
		return
	}
	if st == nil {
		w.apply(next)
		return
	}
	st.next = next
	st.res.Store(switchWaiting)
	wake := c.Invalidator()
	w.beforeAs(next, func(ok bool) {
		v := switchRejected
		if ok {
			v = switchAccepted
		}
		if st.res.CompareAndSwap(switchWaiting, v) {
			wake()
		}
	})
	// done may have run synchronously.
	switch st.res.Load() {
	case switchWaiting:
		w.pending = true
	case switchAccepted:
		st.res.Store(switchIdle)
		w.apply(next)
	default:
		st.res.Store(switchIdle)
	}
}

func (w *SwitchW) semantics(s *a11y.Node) {
	if *w.val {
		s.States |= a11y.Checked
		s.Value = w.onText
		if s.Value == "" {
			s.Value = w.outerOn
		}
	} else {
		s.Value = w.offText
		if s.Value == "" {
			s.Value = w.outerOff
		}
	}
	if w.busy() {
		s.States |= a11y.Busy
	}
}

func (w *SwitchW) dims() (h, minW, handle float32) {
	tk := &w.c.ThemeFor(theme.CompSwitch).Components.Switch
	switch {
	case w.small:
		return tk.TrackHeightSM, tk.TrackMinWidthSM, tk.HandleSizeSM
	case w.large:
		return tk.TrackHeightLG, tk.TrackMinWidthLG, tk.HandleSizeLG
	}
	return tk.TrackHeight, tk.TrackMinWidth, tk.HandleSize
}

// innerFont is the font of the inline texts.
func (w *SwitchW) innerFont() text.Style {
	tk := &w.c.ThemeFor(theme.CompSwitch).Components.Switch
	if w.large {
		return w.c.style(tk.InnerFontSizeLG, text.Regular)
	}
	return w.c.style(tk.InnerFontSize, text.Regular)
}

// outerFont is the font of the OuterTexts labels.
func (w *SwitchW) outerFont() text.Style {
	return w.c.style(w.c.ThemeFor(theme.CompSwitch).Components.Switch.LabelFontSize, text.Regular)
}

// trackW is the track width: Width, else the minimum width grown to fit
// the inline texts / icons.
func (w *SwitchW) trackW() float32 {
	h, mw, hs := w.dims()
	if w.width > 0 {
		return max(w.width, h)
	}
	st := w.innerFont()
	cw := max(text.Width(w.onText, st), text.Width(w.offText, st))
	if w.onIcon != nil || w.offIcon != nil {
		cw = max(cw, st.Size+2)
	}
	if cw == 0 {
		return mw
	}
	return max(mw, cw+hs+2+9+6)
}

// outerW returns the widths of the off (before) and on (after) labels,
// each including its gap to the track (0 when absent).
func (w *SwitchW) outerW() (before, after float32) {
	tk := &w.c.ThemeFor(theme.CompSwitch).Components.Switch
	st := w.outerFont()
	if w.outerOff != "" {
		before = text.Width(w.outerOff, st) + tk.LabelGap
	}
	if w.outerOn != "" {
		after = text.Width(w.outerOn, st) + tk.LabelGap
	}
	return before, after
}

func (w *SwitchW) measure(_, _ float32) geom.Size {
	h, _, _ := w.dims()
	b, a := w.outerW()
	if b > 0 || a > 0 {
		h = max(h, 22)
	}
	return geom.Size{W: b + w.trackW() + a, H: h}
}

// The thumb rests at the inline start when off and moves to the end when
// on (to the left in RTL); the inline text or icon sits on the other side.
// OuterTexts put the off label before the track and the on label after it.
func (w *SwitchW) paint(l *paint.List, r geom.Rect) {
	c, th := w.c, w.c.ThemeFor(theme.CompSwitch)
	tk := &th.Components.Switch
	h, _, hs := w.dims()
	t := c.Anim(w.n.id, "on", b2f(*w.val), th.DurationMid)
	dis := w.disabled || w.n.disabled
	hov := w.ia.Hovered && !w.disabled
	off, on := tk.OffBg, tk.OnBg
	if hov {
		off, on = tk.OffHoverBg, tk.OnHoverBg
	}
	before, after := w.outerW()
	rs := c.inline(r)
	track := rs.Rect(before, r.Y+(r.H-h)/2, w.trackW(), h)
	if dis || w.busy() {
		op := float32(0.65)
		if th.Flavor == theme.StyleShadcn {
			op = 0.5
		}
		l.Opacity(op)
		defer l.Pop()
	}
	if before > 0 || after > 0 {
		st := w.outerFont()
		label := func(s string, off float32, active bool) {
			if s == "" {
				return
			}
			lay := text.Shape(s, st, c.textOpts(text.Options{NoWrap: true}))
			col := th.Text
			if active {
				col = th.Primary.Base
			}
			l.Glyphs(geom.Pt(rs.X(off, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
		}
		label(w.outerOff, 0, !*w.val)
		label(w.outerOn, before+track.W+tk.LabelGap, *w.val)
	}
	if len(tk.TrackShadow) > 0 {
		l.Shadows(track, geom.Rad(h/2), tk.TrackShadow)
	}
	bg := theme.Lerp(off, on, t)
	l.Fill(track, geom.Rad(h/2), bg)
	pad := (h - hs) / 2
	s := c.inline(track)
	hr := s.Rect(pad+(track.W-2*pad-hs)*t, track.Y+pad, hs, hs)
	if len(tk.HandleShadow) > 0 {
		l.Shadows(hr, geom.Rad(hs/2), tk.HandleShadow)
	}
	// shadcn dark: foreground thumb when off.
	l.Fill(hr, geom.Rad(hs/2), theme.Lerp(tk.HandleOffBg, tk.HandleBg, t))
	if w.busy() {
		drawIconRot(l, hr.Expand(-3), th.Primary.Base, spinAngle(c))
	}
	label, ic := w.offText, w.offIcon
	if *w.val {
		label, ic = w.onText, w.onIcon
	}
	fg := th.TextOn(bg, th.TextOnPrimary)
	lo := hs + pad + 6
	if *w.val {
		lo = 9
	}
	if label != "" {
		lay := text.Shape(label, w.innerFont(), c.textOpts(text.Options{NoWrap: true}))
		l.Glyphs(geom.Pt(s.X(lo, lay.Width), track.Y+(h-lay.Height)/2), lay.Glyphs, fg)
	} else if ic != nil {
		is := min(w.innerFont().Size+2, h-4)
		c.paintIcon(l, ic, s.Rect(lo, track.Y+(h-is)/2, is, is), fg, 0)
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
