package ui

import (
	"unicode"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ControlSize is the size variant of form controls.
//
// Append-only: never reorder or insert; the zero value stays Middle.
type ControlSize uint8

const (
	Middle ControlSize = iota
	Small
	Large
)

// defaultSize is the size a control starts with when the caller does not
// pick one: the innermost ConfigProvider's size, else Middle. Controls set
// it in their constructor so explicit Size/Small/Large still win.
func (c *Context) defaultSize() ControlSize {
	s, _ := c.ComponentSize()
	return s
}

// sizeOr returns the ConfigProvider size when one is configured, else def
// (components whose own default is not Middle: Descriptions, Table...).
func (c *Context) sizeOr(def ControlSize) ControlSize {
	if s, ok := c.ComponentSize(); ok {
		return s
	}
	return def
}

func (c *Context) controlHeight(s ControlSize) float32 { return controlHeightOf(c.Theme, s) }

func (c *Context) controlRadius(s ControlSize) float32 { return controlRadiusOf(c.Theme, s) }

func (c *Context) controlFont(s ControlSize) float32 { return controlFontOf(c.Theme, s) }

// controlHeightOf is the control height of size s in th (pass a
// component's theme from c.ThemeFor so its Alias applies).
func controlHeightOf(th *theme.Theme, s ControlSize) float32 {
	switch s {
	case Small:
		return th.ControlHeightSM
	case Large:
		return th.ControlHeightLG
	}
	return th.ControlHeight
}

// controlRadiusOf is the corner radius of a control of size s in th.
func controlRadiusOf(th *theme.Theme, s ControlSize) float32 {
	if th.Flavor == theme.StyleShadcn {
		return th.Radius // new-york: rounded-md at every size
	}
	switch s {
	case Small:
		return th.RadiusSM
	case Large:
		return th.RadiusLG
	}
	return th.Radius
}

// controlFontOf is the text size of a control of size s in th.
func controlFontOf(th *theme.Theme, s ControlSize) float32 {
	if s == Large && th.Flavor != theme.StyleShadcn { // shadcn: text-sm at every size
		return th.FontSizeLG
	}
	return th.FontSize
}

// ButtonVariant is the visual type of a button.
//
// Append-only: never reorder or insert; the zero value stays VariantDefault.
type ButtonVariant uint8

const (
	VariantDefault ButtonVariant = iota
	VariantPrimary
	VariantDashed
	VariantText
	VariantLink
	// VariantSecondary is a neutral filled button (shadcn "secondary", Ant
	// variant="filled", Element type="info" plain).
	VariantSecondary
)

// ButtonW is a button under construction; chain modifiers then query.
type ButtonW struct {
	_ [0]func()

	c        *Context
	n        *node
	th       *theme.Theme       // Button theme (its Alias applied)
	tk       *theme.ButtonToken // &th.Components.Button
	label    string
	variant  ButtonVariant
	danger   bool
	ghost    bool
	size     ControlSize
	disabled bool
	loading  bool
	ic       *icon.Icon
	icRight  bool
	block    bool
	round    bool
	circle   bool
	ia       Interaction
	color    ButtonColor      // "" = from variant / danger
	appear   ButtonAppearance // "" = from variant
	grp      *ButtonGroupW    // enclosing ButtonGroup (direct child), nil otherwise
	// autoSpace: AutoInsertSpace (Defaults "autoInsertSpace").
	autoSpace bool
}

// Button adds a button.
func Button(c *Context, label string) *ButtonW {
	b := &ButtonW{c: c, label: label, size: c.defaultSize()}
	b.th = c.ThemeFor(theme.CompButton)
	b.tk = &b.th.Components.Button
	b.n = c.add("button", label)
	b.n.setRole(a11y.RoleButton)
	b.n.ln.Measure = b.measure
	b.n.ln.NoStretch = true
	b.n.paint = b.paint
	b.ia = c.interact(b.n, true, input.CursorPointer)
	b.configDefaults()
	b.joinGroup()
	return b
}

func (b *ButtonW) Primary() *ButtonW { b.variant = VariantPrimary; return b }
func (b *ButtonW) Dashed() *ButtonW  { b.variant = VariantDashed; return b }
func (b *ButtonW) Text() *ButtonW    { b.variant = VariantText; return b }
func (b *ButtonW) Link() *ButtonW    { b.variant = VariantLink; return b }

func (b *ButtonW) Danger() *ButtonW { b.danger = true; return b }
func (b *ButtonW) Ghost() *ButtonW  { b.ghost = true; return b }
func (b *ButtonW) Small() *ButtonW  { b.size = Small; return b }
func (b *ButtonW) Large() *ButtonW  { b.size = Large; return b }
func (b *ButtonW) Round() *ButtonW  { b.round = true; return b }

// Secondary makes a neutral filled button (shadcn "secondary").
func (b *ButtonW) Secondary() *ButtonW { b.variant = VariantSecondary; return b }

// Variant sets the variant explicitly.
func (b *ButtonW) Variant(v ButtonVariant) *ButtonW { b.variant = v; return b }

// Color sets the button color (Ant v6 color): a role, an Ant preset or a
// hex color ("#13c2c2"). Combine it with Appearance, or with the variant
// modifiers: Primary is solid, Dashed dashed, Text text, Link link,
// Secondary filled. With neither, the success / warning / info colors are
// solid (Element type="success") and the other colors outlined.
//
//	ui.Button(c, "紫色").Color(ui.ButtonColorPurple).Appearance(ui.ButtonFilled)
func (b *ButtonW) Color(col ButtonColor) *ButtonW { b.color = col; return b }

// Appearance sets how the button wears its color (Ant v6 variant, plus
// Element's plain).
func (b *ButtonW) Appearance(a ButtonAppearance) *ButtonW { b.appear = a; return b }

// Success makes a solid success button (Element type="success"); add
// Plain for the plain style.
func (b *ButtonW) Success() *ButtonW { b.color = ButtonColorSuccess; return b }

// Warning makes a solid warning button (Element type="warning").
func (b *ButtonW) Warning() *ButtonW { b.color = ButtonColorWarning; return b }

// Info makes a solid info button (Element type="info").
func (b *ButtonW) Info() *ButtonW { b.color = ButtonColorInfo; return b }

// Plain uses Element's plain style: a light tint of the color with a
// tinted border, filled with the color under the pointer. It combines with
// every color: Plain() alone, Primary().Plain(), Success().Plain(),
// Danger().Plain(), Color(ui.ButtonColorPurple).Plain() …
func (b *ButtonW) Plain() *ButtonW { b.appear = ButtonPlain; return b }

// Size sets the control size.
func (b *ButtonW) Size(s ControlSize) *ButtonW { b.size = s; return b }

// AutoInsertSpace sets whether a label of exactly two Chinese characters
// is drawn with a space between them ("确定" shows as "确 定"; Ant / Element
// ConfigProvider button.autoInsertSpace). It applies only to buttons
// without an icon whose appearance is not text or link; the accessible
// name keeps the label as given. Off by default in every design system
// (Element's default); Defaults("Button", Props{"autoInsertSpace": true})
// turns it on for a scope, as Ant does by default.
func (b *ButtonW) AutoInsertSpace(on bool) *ButtonW { b.autoSpace = on; return b }

// shownLabel is the label as drawn (AutoInsertSpace applied).
func (b *ButtonW) shownLabel() string {
	if !b.autoSpace || b.ic != nil {
		return b.label
	}
	if a := b.appearance(); a == ButtonText || a == ButtonLink {
		return b.label
	}
	r := []rune(b.label)
	if len(r) == 2 && unicode.Is(unicode.Han, r[0]) && unicode.Is(unicode.Han, r[1]) {
		return string(r[0]) + " " + string(r[1])
	}
	return b.label
}

// Circle makes an icon-only round button.
func (b *ButtonW) Circle() *ButtonW { b.circle = true; return b }

// Icon puts an icon before the label.
func (b *ButtonW) Icon(ic *icon.Icon) *ButtonW { b.ic = ic; return b }

// IconRight puts the icon after the label.
func (b *ButtonW) IconRight(ic *icon.Icon) *ButtonW { b.ic, b.icRight = ic, true; return b }

// Block stretches the button to the container width.
func (b *ButtonW) Block() *ButtonW {
	b.block = true
	b.n.ln.NoStretch = false
	b.n.ln.AlignSelf = layout.AlignStretch
	return b
}

// Disabled disables the button when on.
func (b *ButtonW) Disabled(on bool) *ButtonW {
	if on {
		b.disabled = true
		b.n.focus = false
		b.n.cursor = input.CursorNotAllowed
	}
	return b
}

// Loading shows a spinner and blocks clicks.
func (b *ButtonW) Loading(on bool) *ButtonW { b.loading = on; b.n.stateIf(on, a11y.Busy); return b }

// A11yLabel sets the accessible name read by screen readers. Icon-only
// buttons (no label) must set one, from the Locale: Button(c, "").Icon(ic).A11yLabel(loc.Search).
func (b *ButtonW) A11yLabel(s string) *ButtonW { b.n.setName(s); return b }

// Layout applies layout options.
func (b *ButtonW) Layout(opts ...layout.Option) *ButtonW {
	flexbox.Apply(&b.n.ln.Style, opts...)
	return b
}

// Clicked reports activation (never when disabled or loading).
func (b *ButtonW) Clicked() bool { return b.ia.Clicked && !b.isDisabled() && !b.loading }

// Hovered reports hover.
func (b *ButtonW) Hovered() bool { return b.ia.Hovered }

// ID returns the widget ID.
func (b *ButtonW) ID() ID { return b.n.id }

func (b *ButtonW) isDisabled() bool { return b.disabled || b.n.disabled }

func (b *ButtonW) font() text.Style {
	size := controlFontOf(b.th, b.size)
	if b.size == Middle {
		size = b.tk.ContentFontSize
	}
	st := b.c.style(size, text.Regular)
	st.Weight = themeWeight(b.tk.FontWeight)
	return st
}

// themeWeight converts a CSS font-weight token (400, 500 …) to a text weight.
func themeWeight(w int) text.Weight {
	if w <= 0 {
		return text.Regular
	}
	return text.Weight(w)
}

func (b *ButtonW) padX() float32 {
	switch b.size {
	case Small:
		return b.tk.PaddingInlineSM
	case Large:
		return b.tk.PaddingInlineLG
	}
	return b.tk.PaddingInline
}

func (b *ButtonW) shad() bool { return b.th.Flavor == theme.StyleShadcn }

func (b *ButtonW) iconSize() float32 {
	if b.shad() {
		return shadIconSize
	}
	return controlFontOf(b.th, b.size)
}

// gap is the space between icon and label (shadcn sm: gap-1.5).
func (b *ButtonW) gap() float32 {
	if b.shad() && b.size == Small {
		return 6
	}
	return 8
}

func (b *ButtonW) measure(_, _ float32) geom.Size {
	h := controlHeightOf(b.th, b.size)
	if b.circle || (b.label == "" && b.ic != nil) {
		return geom.Size{W: h, H: h}
	}
	w := text.Width(b.shownLabel(), b.font())
	if b.ic != nil || b.loading {
		w += b.iconSize() + b.gap()
	}
	pad := b.padX()
	if b.th.Flavor == theme.Element && b.appearance() == ButtonLink {
		pad = 0
	}
	if (b.ic != nil || b.loading) && b.label != "" && b.shad() {
		pad -= shadButtonIconPadCut(b.size) // has-[>svg]:px-3 / px-2.5 / px-4
	}
	return geom.Size{W: w + 2*pad + 2, H: h}
}

type btnLook struct {
	bg, border, fg theme.Color
	dashed         bool
	shadow         []paint.Shadow
	opacity        float32 // < 1: whole button faded (shadcn disabled)
	underline      bool
}

func (b *ButtonW) look() btnLook {
	v, danger, ok := b.legacy()
	if !ok {
		return b.lookPalette()
	}
	return b.lookFor(v, danger)
}

// lookFor is the original look of variant v (with Danger when danger):
// Primary / Dashed / Text / Link / Secondary / default, Danger and Ghost.
func (b *ButtonW) lookFor(variant ButtonVariant, danger bool) btnLook {
	c, th, tk := b.c, b.th, b.tk
	pal := th.Primary
	if danger {
		pal = th.Error
	}
	el := th.Flavor == theme.Element
	hover := c.Anim(b.n.id, "hover", b2f(b.ia.Hovered && !b.isDisabled()), th.DurationMid)
	press := b.ia.Pressed && !b.isDisabled()
	var lk btnLook
	mix := func(a, h, p theme.Color) theme.Color {
		if press {
			return p
		}
		return theme.Lerp(a, h, hover)
	}
	if th.Flavor == theme.StyleShadcn {
		return b.lookShadcn(variant, danger, pal, hover)
	}
	if variant == VariantSecondary {
		if b.isDisabled() {
			lk.bg, lk.fg = th.BgContainerDisabled, th.TextDisabled
			return lk
		}
		lk.bg, lk.fg = mix(tk.SecondaryBg, tk.SecondaryHoverBg, tk.SecondaryHoverBg), tk.SecondaryColor
		if danger {
			lk.bg, lk.fg = mix(pal.Bg, pal.BgHover, pal.BgHover), pal.Base
		}
		return lk
	}
	if b.isDisabled() {
		switch {
		case variant == VariantText || variant == VariantLink:
			lk.fg = th.TextDisabled
		case el && variant == VariantPrimary:
			lk.bg, lk.border, lk.fg = pal.Shades[3], pal.Shades[3], th.TextOnPrimary
		case el:
			lk.bg, lk.border, lk.fg = th.BgContainer, th.BorderSecondary, th.TextDisabled
		default:
			lk.bg, lk.border, lk.fg = th.BgContainerDisabled, th.Border, th.TextDisabled
		}
		lk.dashed = variant == VariantDashed
		return lk
	}
	switch variant {
	case VariantPrimary:
		base, fg, sh := pal.Base, tk.PrimaryColor, tk.PrimaryShadow
		if danger {
			base, fg, sh = tk.DangerBg, tk.DangerColor, tk.DangerShadow
		}
		lk.bg = mix(base, pal.Hover, pal.Active)
		lk.border = lk.bg
		lk.fg = fg
		if b.ghost {
			lk.bg = theme.Color{}
			lk.fg = lk.border
		}
		lk.shadow = sh
	case VariantText:
		fg := th.Text
		if danger {
			fg = pal.Base
		}
		lk.fg = fg
		if el && !danger {
			lk.fg = mix(th.Text, pal.Base, pal.Active)
		}
		lk.bg = mix(theme.Color{}, tk.TextHoverBg, th.BgTextActive)
		if danger && !el {
			lk.bg = mix(theme.Color{}, pal.Bg, pal.BgHover)
		}
	case VariantLink:
		lk.fg = mix(pal.Base, pal.TextHover, pal.Active)
		if !el {
			lk.fg = mix(pal.Base, pal.Shades[4], pal.Active)
		}
	default: // default and dashed
		lk.dashed = variant == VariantDashed
		if el {
			lk.fg = mix(tk.DefaultColor, tk.DefaultHoverColor, pal.Active)
			lk.border = mix(tk.DefaultBorderColor, tk.DefaultHoverBorderColor, pal.Active)
			lk.bg = mix(tk.DefaultBg, tk.DefaultHoverBg, tk.DefaultHoverBg)
			if danger {
				lk.bg = mix(pal.Bg, pal.Base, pal.Active)
				lk.border = mix(pal.Border, pal.Base, pal.Active)
				lk.fg = mix(pal.Base, th.TextOnPrimary, th.TextOnPrimary)
			}
		} else {
			if danger {
				lk.fg = mix(pal.Base, pal.Hover, pal.Active)
				lk.border = lk.fg
				lk.shadow = tk.DangerShadow
			} else {
				lk.fg = mix(tk.DefaultColor, tk.DefaultHoverColor, pal.Active)
				lk.border = mix(tk.DefaultBorderColor, tk.DefaultHoverBorderColor, pal.Active)
				lk.shadow = tk.DefaultShadow
			}
			lk.bg = mix(tk.DefaultBg, tk.DefaultHoverBg, tk.DefaultHoverBg)
			if b.ghost {
				lk.bg = theme.Color{}
			}
		}
	}
	return lk
}

// lookShadcn follows the shadcn/ui button variants: default (Primary),
// secondary, outline (the default button), ghost (Text), link and
// destructive (Danger). Hover darkens or tints; disabled fades the whole
// button to 50%.
func (b *ButtonW) lookShadcn(variant ButtonVariant, danger bool, pal theme.Palette, hover float32) btnLook {
	th, tk := b.th, b.tk
	if b.isDisabled() {
		hover = 0
	}
	var lk btnLook
	switch variant {
	case VariantPrimary:
		lk.bg, lk.fg = theme.Lerp(pal.Base, pal.Hover, hover), tk.PrimaryColor
		lk.shadow = tk.PrimaryShadow
		if danger {
			// destructive: bg-destructive (dark /60), hover /90.
			base := tk.DangerBg
			lk.bg = theme.Lerp(base, theme.Over(theme.Fade(base, .9), th.BgContainer), hover)
			lk.fg, lk.shadow = tk.DangerColor, tk.DangerShadow
		}
		if b.ghost {
			lk.bg, lk.border, lk.fg = theme.Color{}, pal.Base, pal.Base
		}
	case VariantSecondary:
		lk.bg, lk.fg = theme.Lerp(tk.SecondaryBg, tk.SecondaryHoverBg, hover), tk.SecondaryColor
		if danger {
			lk.bg, lk.fg = theme.Lerp(pal.Bg, pal.BgHover, hover), pal.Base
		}
	case VariantText:
		lk.fg = th.Text
		lk.bg = theme.Lerp(theme.Color{}, tk.TextHoverBg, hover)
		if danger {
			lk.fg, lk.bg = pal.Base, theme.Lerp(theme.Color{}, pal.Bg, hover)
		}
	case VariantLink:
		lk.fg = th.TextLink
		if danger {
			lk.fg = pal.Base
		}
		lk.underline = hover > 0.5
	default: // outline, dashed
		lk.dashed = variant == VariantDashed
		lk.border, lk.fg = tk.DefaultBorderColor, theme.Lerp(tk.DefaultColor, tk.DefaultHoverColor, hover)
		lk.bg = theme.Lerp(tk.DefaultBg, tk.DefaultHoverBg, hover)
		lk.shadow = tk.DefaultShadow
		if danger {
			lk.fg = pal.Base
			lk.bg = theme.Lerp(tk.DefaultBg, pal.Bg, hover)
		}
		if b.ghost {
			lk.bg = theme.Lerp(theme.Color{}, tk.TextHoverBg, hover)
		}
	}
	if b.isDisabled() {
		lk.opacity = 0.5
		lk.shadow = nil
	}
	return lk
}

func (b *ButtonW) radius(r geom.Rect) geom.Radii {
	rad := geom.Rad(controlRadiusOf(b.th, b.size))
	if b.circle || b.round {
		rad = geom.Rad(r.H / 2)
	}
	if b.grp != nil {
		rad = b.grp.corners(b, rad)
	}
	return rad
}

func (b *ButtonW) paint(l *paint.List, r geom.Rect) {
	lk := b.look()
	rad := b.radius(r)
	if b.loading {
		l.Opacity(0.65)
	}
	if lk.opacity > 0 && lk.opacity < 1 {
		l.Opacity(lk.opacity)
		defer l.Pop()
	}
	if len(lk.shadow) > 0 && lk.bg.A > 0 {
		l.Shadows(r, rad, lk.shadow)
	}
	l.Fill(r, rad, lk.bg)
	b.strokeBorder(l, r, rad, lk)
	if b.grp != nil {
		b.grp.divider(l, b, r, lk)
	}
	st := b.font()
	lay := text.Shape(b.shownLabel(), st, b.c.textOpts(text.Options{NoWrap: true}))
	is := b.iconSize()
	ic := b.ic
	if b.loading {
		ic = icon.Loading
	}
	contentW := lay.Width
	if b.label == "" {
		contentW = 0
	}
	if ic != nil {
		if contentW > 0 {
			contentW += b.gap()
		}
		contentW += is
	}
	// Content runs from the inline start: the leading icon sits right of
	// the label in RTL.
	s := b.c.inline(r)
	off := (r.W - contentW) / 2
	ty := r.Y + (r.H-lay.Height)/2
	iy := r.Y + (r.H-is)/2
	gap := b.gap()
	drawIc := func(off float32) {
		ang := 0.0
		if b.loading {
			ang = spinAngle(b.c)
		}
		b.c.paintIcon(l, ic, s.Rect(off, iy, is, is), lk.fg, ang)
	}
	if ic != nil && !b.icRight {
		drawIc(off)
		off += is + gap
	}
	if b.label != "" {
		x := s.X(off, lay.Width)
		l.Glyphs(geom.Pt(x, ty), lay.Glyphs, lk.fg)
		if lk.underline && len(lay.Lines) > 0 {
			l.Fill(geom.Rect{X: x, Y: ty + lay.Lines[0].Baseline + 3, W: lay.Width, H: 1}, geom.Radii{}, lk.fg)
		}
		off += lay.Width + gap
	}
	if ic != nil && b.icRight {
		drawIc(off)
	}
	if b.loading {
		l.Pop()
	}
	if b.grp == nil { // a group raises the ring above the neighbours
		b.focusRing(l, r, rad)
	}
}

// strokeBorder draws the 1px border of lk (dashed for dashed buttons).
func (b *ButtonW) strokeBorder(l *paint.List, r geom.Rect, rad geom.Radii, lk btnLook) {
	if lk.border.A == 0 {
		return
	}
	if lk.dashed {
		l.StrokeDashed(r, rad, 1, lk.border, []float32{3.5, 2.5})
	} else {
		l.Stroke(r, rad, 1, lk.border)
	}
}

// focusRing draws the :focus-visible ring when the button shows one.
func (b *ButtonW) focusRing(l *paint.List, r geom.Rect, rad geom.Radii) {
	if !b.ia.FocusVisible || b.isDisabled() {
		return
	}
	if b.th.Flavor == theme.StyleShadcn {
		v, danger, _ := b.legacy()
		shadFocusRing(l, b.th, r, rad, danger && v == VariantPrimary)
	} else {
		drawFocusRing(l, b.th, r, rad)
	}
}

// drawFocusRing paints the :focus-visible ring of the active design system
// around r: Ant's 4px halo, Element's 2px outline, shadcn's 2px ring with a
// 2px offset (theme FocusOutline / FocusRingWidth / FocusRingOffset).
func drawFocusRing(l *paint.List, th *theme.Theme, r geom.Rect, rad geom.Radii) {
	g := th.FocusRingOffset + th.FocusRingWidth
	l.Stroke(r.Expand(g), rad.Grow(g), th.FocusRingWidth, th.FocusOutline)
}

// IconButtonW is a borderless square icon button (close buttons, toolbar
// toggles, calendar navigation).
type IconButtonW struct {
	_ [0]func()

	c      *Context
	n      *node
	ic     *icon.Icon
	box    float32
	is     float32
	col    theme.Color
	colSet bool
	noBg   bool
	ia     Interaction
}

// IconButton adds an icon button; label is used for tests and accessibility.
func IconButton(c *Context, ic *icon.Icon, label string) *IconButtonW {
	b := &IconButtonW{c: c, ic: ic, box: 22, is: 16}
	b.n = c.add("iconbutton", label)
	b.n.setRole(a11y.RoleButton)
	b.n.ln.NoStretch = true
	b.n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: b.box, H: b.box} }
	b.ia = c.interact(b.n, true, input.CursorPointer)
	th := c.Theme
	b.n.paint = func(l *paint.List, r geom.Rect) {
		h := c.Anim(b.n.id, "hover", b2f(b.ia.Hovered), th.DurationMid)
		if !b.noBg && h > 0 {
			l.Fill(r, geom.Rad(th.RadiusSM), theme.Fade(th.BgTextHover, float64(h)))
		}
		col := theme.Lerp(th.TextTertiary, th.Text, h)
		if b.colSet {
			col = b.col
		}
		c.paintIcon(l, b.ic, geom.Rect{X: r.X + (r.W-b.is)/2, Y: r.Y + (r.H-b.is)/2, W: b.is, H: b.is}, col, 0)
		if b.ia.FocusVisible {
			l.Stroke(r, geom.Rad(th.RadiusSM), 2, th.FocusRing)
		}
	}
	return b
}

// Size sets the square box size and the icon size.
func (b *IconButtonW) Size(box, iconSize float32) *IconButtonW { b.box, b.is = box, iconSize; return b }

// Color fixes the icon color.
func (b *IconButtonW) Color(col theme.Color) *IconButtonW { b.col, b.colSet = col, true; return b }

// NoHoverBg disables the hover background.
func (b *IconButtonW) NoHoverBg() *IconButtonW { b.noBg = true; return b }

// A11yLabel replaces the accessible name (by default the label argument).
func (b *IconButtonW) A11yLabel(s string) *IconButtonW { b.n.setName(s); return b }

// Clicked reports activation.
func (b *IconButtonW) Clicked() bool { return b.ia.Clicked }

// Hovered reports hover.
func (b *IconButtonW) Hovered() bool { return b.ia.Hovered }

// ---------------------------------------------------------------------------
// Color × appearance (Ant v6 color / variant, Element type / plain)

// ButtonColor names a button color: a role (default, primary, danger,
// success, warning, info), an Ant Design preset (blue … gold) or a hex
// color such as "#13c2c2". The set is open; unknown names fall back to
// primary.
type ButtonColor string

const (
	ButtonColorDefault ButtonColor = "default"
	ButtonColorPrimary ButtonColor = "primary"
	ButtonColorDanger  ButtonColor = "danger"
	ButtonColorSuccess ButtonColor = "success" // Element type="success"
	ButtonColorWarning ButtonColor = "warning" // Element type="warning"
	ButtonColorInfo    ButtonColor = "info"    // Element type="info"

	ButtonColorBlue     ButtonColor = "blue"
	ButtonColorPurple   ButtonColor = "purple"
	ButtonColorCyan     ButtonColor = "cyan"
	ButtonColorGreen    ButtonColor = "green"
	ButtonColorMagenta  ButtonColor = "magenta"
	ButtonColorPink     ButtonColor = "pink"
	ButtonColorRed      ButtonColor = "red"
	ButtonColorOrange   ButtonColor = "orange"
	ButtonColorYellow   ButtonColor = "yellow"
	ButtonColorVolcano  ButtonColor = "volcano"
	ButtonColorGeekblue ButtonColor = "geekblue"
	ButtonColorLime     ButtonColor = "lime"
	ButtonColorGold     ButtonColor = "gold"
)

// ButtonPresetColors lists the Ant Design preset colors in display order.
var ButtonPresetColors = []ButtonColor{
	ButtonColorBlue, ButtonColorPurple, ButtonColorCyan, ButtonColorGreen, ButtonColorMagenta,
	ButtonColorPink, ButtonColorRed, ButtonColorOrange, ButtonColorYellow, ButtonColorVolcano,
	ButtonColorGeekblue, ButtonColorLime, ButtonColorGold,
}

// ButtonAppearance is how a button wears its color (Ant v6 variant, plus
// Element's plain).
type ButtonAppearance string

const (
	ButtonSolid    ButtonAppearance = "solid"    // filled with the color
	ButtonOutlined ButtonAppearance = "outlined" // colored border and label
	ButtonDashed   ButtonAppearance = "dashed"   // outlined with a dashed border
	ButtonFilled   ButtonAppearance = "filled"   // light tint, no border
	ButtonText     ButtonAppearance = "text"     // label only, tint under the pointer
	ButtonLink     ButtonAppearance = "link"     // link-styled label
	ButtonPlain    ButtonAppearance = "plain"    // Element plain: tint + tinted border, solid on hover
)

// combo resolves the color and appearance of a button that set either;
// ok is false for buttons using only the variant modifiers.
func (b *ButtonW) combo() (col ButtonColor, ap ButtonAppearance, ok bool) {
	if b.color == "" && b.appear == "" {
		return "", "", false
	}
	col = b.color
	if col == "" {
		switch {
		case b.danger:
			col = ButtonColorDanger
		case b.variant == VariantPrimary:
			col = ButtonColorPrimary
		default:
			col = ButtonColorDefault
		}
	}
	ap = b.appear
	if ap == "" {
		ap = appearanceOfVariant(b.variant)
		if b.variant == VariantDefault && (col == ButtonColorSuccess || col == ButtonColorWarning || col == ButtonColorInfo) {
			ap = ButtonSolid // Element types are solid
		}
	}
	return col, ap, true
}

// appearance is the effective appearance of b.
func (b *ButtonW) appearance() ButtonAppearance {
	if _, ap, ok := b.combo(); ok {
		return ap
	}
	return appearanceOfVariant(b.variant)
}

func appearanceOfVariant(v ButtonVariant) ButtonAppearance {
	switch v {
	case VariantPrimary:
		return ButtonSolid
	case VariantDashed:
		return ButtonDashed
	case VariantText:
		return ButtonText
	case VariantLink:
		return ButtonLink
	case VariantSecondary:
		return ButtonFilled
	}
	return ButtonOutlined
}

func variantOfAppearance(a ButtonAppearance) ButtonVariant {
	switch a {
	case ButtonSolid:
		return VariantPrimary
	case ButtonDashed:
		return VariantDashed
	case ButtonText:
		return VariantText
	case ButtonLink:
		return VariantLink
	case ButtonFilled:
		return VariantSecondary
	}
	return VariantDefault
}

// legacy returns the variant and danger flag that draw b with the original
// looks; ok is false when its color × appearance needs lookPalette. The
// default and danger colors (except default solid and plain) and the solid
// primary keep the exact look of the variant modifiers.
func (b *ButtonW) legacy() (v ButtonVariant, danger, ok bool) {
	col, ap, set := b.combo()
	if !set {
		return b.variant, b.danger, true
	}
	if ap == ButtonPlain {
		return 0, false, false
	}
	switch col {
	case ButtonColorDefault:
		if ap == ButtonSolid {
			return 0, false, false
		}
		return variantOfAppearance(ap), false, true
	case ButtonColorDanger:
		return variantOfAppearance(ap), true, true
	case ButtonColorPrimary:
		if ap == ButtonSolid {
			return VariantPrimary, false, true
		}
	}
	return 0, false, false
}

// palette returns the palette of a non-default color and whether it is a
// preset or hex color (labels drawn in it get PresetTextContrast).
func (b *ButtonW) palette(col ButtonColor) (theme.Palette, bool) {
	th := b.th
	switch col {
	case ButtonColorPrimary, ButtonColorDefault:
		return th.Primary, false
	case ButtonColorDanger:
		return th.Error, false
	case ButtonColorSuccess:
		return th.Success, false
	case ButtonColorWarning:
		return th.Warning, false
	case ButtonColorInfo:
		return th.Info, false
	}
	hx := string(col)
	if p, ok := antPreset[hx]; ok {
		hx = p
	}
	base, err := theme.ParseHex(hx)
	if err != nil {
		return th.Primary, false
	}
	return th.ButtonPalette(base), true
}

// lookPalette draws the color × appearance combinations the variant
// modifiers do not cover, from the color's palette.
func (b *ButtonW) lookPalette() btnLook {
	col, ap, _ := b.combo()
	th, tk := b.th, b.tk
	dis := b.isDisabled()
	shad := th.Flavor == theme.StyleShadcn
	if dis && !shad {
		return b.lookPaletteDisabled(col, ap)
	}
	hover := b.c.Anim(b.n.id, "hover", b2f(b.ia.Hovered && !dis), th.DurationMid)
	press := b.ia.Pressed && !dis && !shad // shadcn has no pressed shade
	mix := func(a, h, p theme.Color) theme.Color {
		if press {
			return p
		}
		return theme.Lerp(a, h, hover)
	}
	var lk btnLook
	page := tk.DefaultBg
	switch {
	case col == ButtonColorDefault && ap == ButtonSolid:
		lk.bg = mix(tk.DefaultSolidBg, tk.DefaultSolidHoverBg, tk.DefaultSolidActiveBg)
		lk.border, lk.fg = lk.bg, tk.DefaultSolidColor
		if b.ghost {
			lk.bg, lk.fg = theme.Color{}, lk.border
		}
	case col == ButtonColorDefault: // plain
		pal := th.Primary
		lk.bg = tk.DefaultBg
		lk.border = mix(tk.DefaultBorderColor, pal.Base, pal.Active)
		lk.fg = mix(tk.DefaultColor, pal.Base, pal.Active)
		if shad { // the neutral primary: outline that darkens
			lk.border, lk.fg = tk.DefaultBorderColor, mix(tk.DefaultColor, tk.DefaultHoverColor, tk.DefaultHoverColor)
			lk.bg = mix(tk.DefaultBg, tk.DefaultHoverBg, tk.DefaultHoverBg)
		}
	default:
		pal, preset := b.palette(col)
		readable := func(fg, bg theme.Color) theme.Color {
			if preset && tk.PresetTextContrast > 0 {
				return theme.EnsureContrast(fg, theme.Over(bg, page), float64(tk.PresetTextContrast), th.Text)
			}
			return fg
		}
		onSolid := th.TextOn(pal.Base, th.TextOnSolid)
		switch ap {
		case ButtonSolid:
			lk.bg = mix(pal.Base, pal.Hover, pal.Active)
			lk.border, lk.fg = lk.bg, onSolid
			if col == ButtonColorPrimary {
				lk.fg, lk.shadow = tk.PrimaryColor, tk.PrimaryShadow
			}
			if b.ghost {
				lk.bg, lk.fg = theme.Color{}, readable(lk.border, page)
			}
		case ButtonOutlined, ButtonDashed:
			lk.dashed = ap == ButtonDashed
			lk.border = mix(pal.Base, pal.Hover, pal.Active)
			lk.bg = tk.DefaultBg
			if shad {
				lk.bg = theme.Lerp(tk.DefaultBg, pal.Bg, hover)
			}
			if b.ghost {
				lk.bg = theme.Color{}
			}
			lk.fg = readable(lk.border, page)
		case ButtonFilled:
			lk.bg = mix(pal.Bg, pal.BgHover, pal.Border)
			lk.fg = readable(pal.Base, lk.bg)
		case ButtonText:
			lk.bg = mix(theme.Color{}, pal.Bg, pal.BgHover)
			lk.fg = readable(mix(pal.Base, pal.Hover, pal.Active), page)
		case ButtonLink:
			lk.fg = readable(mix(pal.Base, pal.Hover, pal.Active), page)
			if shad {
				lk.fg, lk.underline = readable(pal.Base, page), hover > 0.5
			}
		default: // plain
			lk.bg = mix(pal.Bg, pal.Base, pal.Active)
			lk.border = mix(pal.Border, pal.Base, pal.Active)
			lk.fg = mix(readable(pal.Base, pal.Bg), onSolid, onSolid)
		}
	}
	if dis {
		lk.opacity, lk.shadow = 0.5, nil
	}
	return lk
}

// lookPaletteDisabled is the disabled look of lookPalette (Ant and
// Element; shadcn fades the normal look).
func (b *ButtonW) lookPaletteDisabled(col ButtonColor, ap ButtonAppearance) btnLook {
	th := b.th
	el := th.Flavor == theme.Element
	var lk btnLook
	lk.dashed = ap == ButtonDashed
	if el && col != ButtonColorDefault {
		// el-button disabled: the light-5 / light-8 / light-9 steps.
		pal, _ := b.palette(col)
		faint := pal.Shades[3]
		switch ap {
		case ButtonSolid:
			lk.bg, lk.border, lk.fg = faint, faint, th.TextOn(faint, th.TextOnSolid) // white on light-5 would read below 1.6:1
		case ButtonPlain:
			lk.bg, lk.border, lk.fg = pal.Bg, pal.Shades[1], faint
		case ButtonFilled:
			lk.bg, lk.fg = pal.Bg, faint
		case ButtonText, ButtonLink:
			lk.fg = faint
		default:
			lk.bg, lk.border, lk.fg = th.BgContainer, pal.Shades[2], faint
		}
		return lk
	}
	switch {
	case ap == ButtonText || ap == ButtonLink:
		lk.fg = th.TextDisabled
	case ap == ButtonFilled:
		lk.bg, lk.fg = th.BgContainerDisabled, th.TextDisabled
	case el:
		lk.bg, lk.border, lk.fg = th.BgContainer, th.BorderSecondary, th.TextDisabled
	default:
		lk.bg, lk.border, lk.fg = th.BgContainerDisabled, th.Border, th.TextDisabled
	}
	return lk
}
