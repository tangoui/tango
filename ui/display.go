package ui

import (
	"image"
	"math"
	"strconv"
	"strings"
	"time"

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

// ---------------------------------------------------------------------------
// Divider

// DividerW is the Box of a Divider with its line options; it embeds the
// *Box, so code using the returned box keeps working.
//
// ConfigProvider.Defaults("Divider", ...) keys: "dashed" (bool),
// "variant" ("solid" | "dashed" | "dotted"), "orientation"
// ("horizontal" | "vertical"; a title placement is still accepted),
// "vertical" (bool), "titlePlacement" ("start" | "end" | "left" | "right"
// | "center"), "plain" (bool), "size" (ControlSize).
type DividerW struct {
	_ [0]func()

	*Box
	c        *Context
	tk       *theme.DividerToken
	variant  string // "solid" | "dashed" | "dotted"
	orient   string // title placement
	plain    bool
	vertical bool
	size     ControlSize
	sizeSet  bool
	titled   bool
}

// Divider draws a horizontal rule, optionally with a title.
func Divider(c *Context, title ...string) *DividerW {
	th := c.ThemeFor(theme.CompDivider)
	tk := &th.Components.Divider
	label := strings.Join(title, " ")
	b := c.box("divider", layout.Column, nil)
	d := &DividerW{Box: b, c: c, tk: tk, variant: "solid", orient: "center", titled: label != "",
		plain: Prop(c, "Divider", "plain", false)}
	b.n.setRole(a11y.RoleSeparator).setState(a11y.Horizontal).setName(label)
	if Prop(c, "Divider", "dashed", false) {
		d.variant = "dashed"
	}
	if v := Prop(c, "Divider", "variant", ""); v != "" {
		d.Variant(v)
	}
	d.Orientation(Prop(c, "Divider", "orientation", ""))
	d.TitlePlacement(Prop(c, "Divider", "titlePlacement", ""))
	if s, ok := c.ComponentSize(); ok { // Ant: the ConfigProvider size applies
		d.size, d.sizeSet = s, true
	}
	if s, ok := c.ConfigProp("Divider", "size"); ok {
		if sz, ok := shad2aParseSize(s); ok { // ControlSize or "small" / "middle" / "large"
			d.Size(sz)
		}
	}
	if Prop(c, "Divider", "vertical", false) {
		d.Vertical()
	}
	d.margins()
	font := func() text.Style {
		size, w := tk.TextFontSize, text.Weight(tk.TextFontWeight)
		if d.plain {
			size, w = tk.FontSize, text.Regular
		}
		return text.Style{Size: size, Weight: w, LineHeight: th.LineHeightFor(size)}
	}
	pad := tk.TextPaddingInline
	b.n.ln.Measure = func(_, _ float32) geom.Size {
		if d.vertical {
			return geom.Size{W: tk.LineWidth, H: tk.VerticalHeight}
		}
		if label == "" {
			return geom.Size{H: tk.LineWidth}
		}
		f := font()
		return geom.Size{W: text.Width(label, f) + 2*pad, H: f.LineHeight}
	}
	b.n.paint = func(l *paint.List, r geom.Rect) {
		lw := tk.LineWidth
		if d.vertical {
			d.rule(l, r.X+r.W/2, r.Y, r.X+r.W/2, r.Bottom(), lw)
			return
		}
		cy := r.Y + r.H/2
		line := func(x0, x1 float32) {
			if x1 > x0 {
				d.rule(l, x0, cy, x1, cy, lw)
			}
		}
		if label == "" {
			line(r.X, r.Right())
			return
		}
		lay := text.Shape(label, font(), c.textOpts(text.Options{NoWrap: true}))
		tx := r.X + (r.W-lay.Width)/2
		// Ant orientationMargin: the short side is 5% of the width. "start"
		// and "end" are logical; "left" and "right" physical (Ant v6).
		switch d.placement() {
		case "start":
			tx = r.X + r.W*tk.OrientationMargin + pad
		case "end":
			tx = r.Right() - r.W*tk.OrientationMargin - pad - lay.Width
		}
		if c.IsRTL() {
			tx = mirrorRect(geom.Rect{X: tx, W: lay.Width}, r).X
		}
		line(r.X, tx-pad)
		line(tx+lay.Width+pad, r.Right())
		col := tk.TextColor
		if d.plain {
			col = tk.PlainTextColor
		}
		l.Glyphs(geom.Pt(tx, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	}
	return d
}

// rule draws one solid, dashed or dotted line from (x0, y0) to (x1, y1).
func (d *DividerW) rule(l *paint.List, x0, y0, x1, y1, lw float32) {
	col := d.tk.Color
	switch d.variant {
	case "dashed":
		p := paint.NewPath().MoveTo(x0, y0).LineTo(x1, y1)
		l.StrokePath(p, paint.StrokeStyle{Width: lw, Dash: []float32{4, 3}}, col)
	case "dotted":
		p := paint.NewPath().MoveTo(x0, y0).LineTo(x1, y1)
		l.StrokePath(p, paint.StrokeStyle{Width: lw, Dash: []float32{lw, lw}}, col)
	default:
		r := geom.Rect{X: x0, Y: y0 - lw/2, W: x1 - x0, H: lw}
		if x0 == x1 {
			r = geom.Rect{X: x0 - lw/2, Y: y0, W: lw, H: y1 - y0}
		}
		l.Fill(r, geom.Radii{}, col)
	}
}

// placement resolves the title placement to "start", "end" or "center":
// "left" and "right" are physical sides (Ant v6 titlePlacement).
func (d *DividerW) placement() string {
	rtl := d.c.IsRTL()
	switch d.orient {
	case "left":
		if rtl {
			return "end"
		}
		return "start"
	case "right":
		if rtl {
			return "start"
		}
		return "end"
	case "start", "end":
		return d.orient
	}
	return "center"
}

// margins sets the block (horizontal rule) or inline (vertical) margins.
func (d *DividerW) margins() {
	tk, ln := d.tk, &d.Box.n.ln
	if d.vertical {
		ln.Margin = geom.Insets{Left: tk.VerticalMarginInline, Right: tk.VerticalMarginInline}
		return
	}
	m := tk.MarginBlock
	if d.titled {
		m = tk.TextMarginBlock
	}
	if d.sizeSet { // Ant size: small 8, middle 16 (large keeps the default)
		switch d.size {
		case Small:
			m = tk.MarginBlockSM
		case Middle:
			m = tk.MarginBlockMD
		}
	}
	ln.Margin = geom.Insets{Top: m, Bottom: m}
}

// Dashed draws a dashed line (Variant("dashed")).
func (d *DividerW) Dashed() *DividerW { d.variant = "dashed"; return d }

// Variant sets the line style: "solid" (default), "dashed" or "dotted"
// (Ant variant, Element border-style). Other values are ignored.
func (d *DividerW) Variant(v string) *DividerW {
	switch v {
	case "solid", "dashed", "dotted":
		d.variant = v
	}
	return d
}

// Orientation sets the direction: "horizontal" (default) or "vertical"
// (Ant v6 orientation). A title placement ("left", "right", "center",
// "start", "end") is still accepted here and sets TitlePlacement, as Ant
// does for the pre-v6 meaning; new code sets the title placement with
// TitlePlacement.
func (d *DividerW) Orientation(s string) *DividerW {
	switch s {
	case "horizontal":
		return d
	case "vertical":
		return d.Vertical()
	}
	return d.TitlePlacement(s)
}

// TitlePlacement places the title: "start", "end" (logical), "left",
// "right" (physical) or "center" (default; Ant titlePlacement, Element
// content-position). Other values are ignored.
func (d *DividerW) TitlePlacement(s string) *DividerW {
	switch s {
	case "left", "right", "center", "start", "end":
		d.orient = s
	}
	return d
}

// Vertical turns the divider into an inline vertical separator like
// VDivider (Ant vertical / orientation="vertical", Element
// direction="vertical"); a title is not shown.
func (d *DividerW) Vertical() *DividerW {
	if d.vertical {
		return d
	}
	d.vertical = true
	n := d.Box.n
	n.aState &^= a11y.Horizontal
	n.setState(a11y.Vertical)
	n.ln.NoStretch = true
	n.ln.AlignSelf = layout.AlignCenter
	if d.tk.VerticalHeight == 0 { // shadcn: h-full
		n.ln.NoStretch = false
		n.ln.AlignSelf = layout.AlignStretch
	}
	d.margins()
	return d
}

// Size sets the block margin of a horizontal divider (Ant size): Small
// 8px, Middle 16px; Large keeps the default.
func (d *DividerW) Size(s ControlSize) *DividerW {
	d.size, d.sizeSet = s, true
	d.margins()
	return d
}

// Plain shows the title in the normal text style (Ant plain).
func (d *DividerW) Plain() *DividerW { d.plain = true; return d }

// VDivider draws a short vertical separator for inline content.
func VDivider(c *Context) {
	n := c.add("vdivider", "")
	n.setRole(a11y.RoleSeparator).setState(a11y.Vertical)
	tk := &c.ThemeFor(theme.CompDivider).Components.Divider
	n.ln.Width, n.ln.Height = tk.LineWidth, tk.VerticalHeight
	n.ln.Margin = geom.Insets{Left: tk.VerticalMarginInline, Right: tk.VerticalMarginInline}
	n.ln.AlignSelf = layout.AlignCenter
	if tk.VerticalHeight == 0 { // shadcn Separator vertical: h-full
		n.ln.Height = 0
		n.ln.AlignSelf = layout.AlignStretch
	}
	n.paint = func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, tk.Color) }
}

// ---------------------------------------------------------------------------
// Tag

// TagW is a small label.
type TagW struct {
	_ [0]func()

	c         *Context
	n         *node
	label     string
	color     string // "", status names, Ant presets, shadcn variants or "#rrggbb"
	closable  bool
	closed    bool
	ic        *icon.Icon
	closeIcon *icon.Icon
	ia        Interaction
	bordered  bool
	kbdClose  bool // closed with the keyboard or an accessibility action
	size      ControlSize
	effect    string // "" / "light", "dark", "plain"
	variant   string // "" (theme default) / "filled", "solid", "outlined"
	varSet    bool   // variant came from Variant (not Defaults): it beats "-inverse"
	disabled  bool
	round     bool
	th        *theme.Theme
	tk        *theme.TagToken
}

// Tag adds a tag. color may be "success", "processing", "error",
// "warning", "default", an Ant preset ("blue", "magenta" ...; with the
// suffix "-inverse", e.g. "blue-inverse", the preset solid unless a
// Variant is set, like antd), a shadcn Badge variant ("primary",
// "secondary", "destructive", "outline", "ghost", "link") or a hex color.
//
// ConfigProvider.Defaults("Tag", ...) keys: "size" (ControlSize), "effect",
// "variant", "color" (string), "bordered" (bool; deprecated, false =
// variant "filled"), "disabled" (bool).
func Tag(c *Context, label string) *TagW {
	t := &TagW{c: c, label: label, bordered: Prop(c, "Tag", "bordered", true), size: Middle,
		effect: Prop(c, "Tag", "effect", ""), variant: Prop(c, "Tag", "variant", ""),
		color: Prop(c, "Tag", "color", ""), disabled: Prop(c, "Tag", "disabled", false)}
	t.th = c.ThemeFor(theme.CompTag)
	t.tk = &t.th.Components.Tag
	if sz, ok := dataPropSize(c, "Tag"); ok {
		t.size = sz
	}
	t.n = c.add("tag", label)
	t.n.setRole(a11y.RoleText)
	t.n.ln.NoStretch = true
	t.n.ln.Measure = t.measure
	t.n.paint = t.paint
	t.applyDisabled() // ConfigProvider.ComponentDisabled / Defaults "disabled"
	return t
}

func (t *TagW) Color(col string) *TagW   { t.color = col; return t }
func (t *TagW) Icon(ic *icon.Icon) *TagW { t.ic = ic; return t }

// Borderless draws the tag without its border.
//
// Deprecated: use Variant("filled") (antd 6 replaced bordered={false}).
func (t *TagW) Borderless() *TagW { t.bordered = false; return t }

// Variant sets the antd 6 variant: "filled" (tinted, no border; the
// default outside Element), "solid" (saturated color, light text) or
// "outlined" (tinted with a border; Element's default light effect).
func (t *TagW) Variant(v string) *TagW { t.variant, t.varSet = v, v != ""; return t }

// Size sets small (20px), middle (default) or large (Element 32px) tags.
func (t *TagW) Size(sz ControlSize) *TagW { t.size = sz; return t }

// Round draws a pill-shaped tag (Element round).
func (t *TagW) Round() *TagW { t.round = true; return t }

// Effect sets the Element theme of the tag: "light" (tinted with a
// border; = Variant("outlined")), "dark" (solid color, white text; =
// Variant("solid")) or "plain" (container background, colored border and
// text). Variant wins when both are set.
func (t *TagW) Effect(e string) *TagW { t.effect = e; return t }

// CloseIcon replaces the close icon (antd closeIcon); it also makes the
// tag closable.
func (t *TagW) CloseIcon(ic *icon.Icon) *TagW {
	t.closeIcon = ic
	if !t.closable {
		t.Closable()
	}
	return t
}

// Disabled greys the tag out (antd 6 disabled); a disabled closable tag
// does not close. ConfigProvider.ComponentDisabled applies too.
func (t *TagW) Disabled(on bool) *TagW {
	t.disabled = on
	t.applyDisabled()
	return t
}

func (t *TagW) isDisabled() bool { return t.disabled || t.n.disabled }

func (t *TagW) applyDisabled() {
	if t.isDisabled() {
		t.n.setState(a11y.Disabled)
		if t.closable {
			t.n.focus, t.n.cursor = false, input.CursorNotAllowed
		}
	}
}

// Closable shows a close icon. The tag becomes a "remove" button for the
// keyboard: Enter, Space, Delete or Backspace close it.
func (t *TagW) Closable() *TagW {
	c := t.c
	t.closable = true
	t.n.setRole(a11y.RoleButton).setName(c.Locale().A11y.Remove + " " + t.label)
	t.ia = c.interact(t.n, true, input.CursorDefault)
	id := t.n.id
	t.kbdClose = t.ia.Clicked && c.clicked[id] == 0 // Enter / Space
	if _, _, ok := c.A11yAction(id); ok && t.ia.Clicked {
		t.kbdClose = true
	}
	for _, k := range c.KeysFor(id) {
		if k.Down && (k.Key == input.KeyDelete || k.Key == input.KeyBackspace) {
			t.kbdClose = true
		}
	}
	t.applyDisabled()
	return t
}

// Closed reports a click on the close icon (or a keyboard close); always
// false while disabled.
func (t *TagW) Closed() bool {
	if !t.closable || t.isDisabled() {
		return false
	}
	if t.kbdClose {
		return true
	}
	if !t.ia.Clicked {
		return false
	}
	// The close icon sits in the last 18px at the inline end.
	r := t.c.rects[t.n.id]
	return t.c.inline(r).Off(t.c.pointer.X) > r.W-18
}

func (t *TagW) font() text.Style {
	th := t.th
	fs := th.FontSizeSM
	if t.size == Large && th.Flavor != theme.Element {
		fs = th.FontSize
	}
	st := t.c.style(fs, text.Regular)
	if t.tk.FontWeight != 0 {
		st.Weight = themeWeight(t.tk.FontWeight)
	}
	return st
}

// tagPad is the horizontal padding inside a tag.
func (t *TagW) tagPad() float32 {
	switch t.size {
	case Small:
		return t.tk.PaddingInlineSM
	case Large:
		return t.tk.PaddingInlineLG
	}
	return t.tk.PaddingInline
}

func (t *TagW) measure(_, _ float32) geom.Size {
	w := text.Width(t.label, t.font())
	if t.ic != nil {
		w += 12 + 4
	}
	if t.closable {
		w += 10 + 4
	}
	pad, h := t.tagPad(), t.tk.Height
	switch t.size {
	case Small:
		h = t.tk.HeightSM
	case Large:
		h = t.tk.HeightLG
	}
	return geom.Size{W: w + 2*pad + 2, H: h}
}

// effVariant resolves the variant: Variant, then an "-inverse" color
// (solid), then the Defaults variant, then Effect, then the deprecated
// Borderless, then the theme default (antd useColor: variant prop, inverse,
// context variant).
func (t *TagW) effVariant() string {
	valid := t.variant == "filled" || t.variant == "solid" || t.variant == "outlined"
	if valid && t.varSet {
		return t.variant
	}
	if strings.HasSuffix(t.color, "-inverse") {
		return "solid"
	}
	if valid {
		return t.variant
	}
	switch t.effect {
	case "dark", "solid":
		return "solid"
	case "plain":
		return "plain"
	case "light", "outlined":
		return "outlined"
	case "filled":
		return "filled"
	}
	if !t.bordered || !t.tk.Outlined {
		return "filled"
	}
	return "outlined"
}

// shadcnVariant reports a shadcn Badge variant color.
func shadcnVariant(col string) bool {
	switch col {
	case "primary", "secondary", "destructive", "outline", "ghost", "link":
		return true
	}
	return false
}

// tint returns the tinted colors (outlined look) and the solid color of
// the tag's color.
func (t *TagW) tint() (bg, border, fg, solid theme.Color) {
	th, tk := t.th, t.tk
	switch t.color {
	case "", "default":
		return tk.DefaultBg, tk.BorderColor, tk.DefaultColor, tk.SolidBg
	case "processing":
		p := th.Primary
		return p.Bg, p.Border, p.Base, p.Base
	case "success", "warning", "error", "danger", "info", "primary":
		p := th.ColorOf(t.color)
		if th.Flavor == theme.Element {
			return p.Bg, p.BgHover, p.Base, p.Base
		}
		return p.Bg, p.Border, p.Base, p.Base
	}
	hx := strings.TrimSuffix(t.color, "-inverse")
	if p, ok := antPreset[hx]; ok {
		hx = p
	}
	if col, err := theme.ParseHex(hx); err == nil {
		// antd presets: lightColor (1), lightBorderColor (3), textColor
		// (7), darkColor (6).
		pal := theme.AntPalette(col, th.Dark, th.BgContainer)
		return pal.Bg, pal.Border, pal.Active, pal.Base
	}
	return th.FillQuaternary, th.Border, th.Text, tk.SolidBg
}

// shadcnColors returns the shadcn Badge variant colors (border-transparent
// unless outline).
func (t *TagW) shadcnColors() (bg, border, fg theme.Color, ok bool) {
	th, tk := t.th, t.tk
	switch t.color {
	case "", "default", "primary":
		return tk.DefaultBg, theme.Color{}, tk.DefaultColor, true
	case "secondary":
		return th.SecondaryBg, theme.Color{}, th.SecondaryText, true
	case "outline":
		return theme.Color{}, th.Border, th.Text, true
	case "ghost":
		return theme.Color{}, theme.Color{}, th.Text, true
	case "link":
		return theme.Color{}, theme.Color{}, th.Primary.Base, true
	case "destructive":
		return tk.DestructiveBg, theme.Color{}, th.TextOnSolid, true
	case "success", "warning", "error", "danger", "info", "processing":
		p := th.ColorOf(t.color)
		return p.Bg, theme.Color{}, p.Shades[6], true // deeper shade reads on the tint
	}
	return
}

// colors returns the background, border (zero: none) and label colors.
func (t *TagW) colors() (bg, border, fg theme.Color) {
	th, tk := t.th, t.tk
	v := t.effVariant()
	if th.Flavor == theme.StyleShadcn && (shadcnVariant(t.color) || (t.variant == "" && t.effect == "")) {
		if b, bo, f, ok := t.shadcnColors(); ok {
			bg, border, fg = b, bo, f
			if t.isDisabled() {
				bg, fg = tk.DisabledBg, tk.DisabledColor
				if border.A > 0 {
					border = tk.DisabledBorderColor
				}
			}
			return
		}
	}
	bg, border, fg, solid := t.tint()
	switch v {
	case "filled":
		border = theme.Color{}
	case "solid":
		bg, border = solid, theme.Color{}
		fg = th.TextOn(solid, tk.SolidTextColor)
	case "plain":
		if th.Flavor == theme.Element {
			// Element plain: light-5 border.
			switch t.color {
			case "processing": // "" and default: TagToken.PlainBorderColor below
				border = th.Primary.Border
			case "success", "warning", "error", "danger", "info", "primary":
				border = th.ColorOf(t.color).Border
			}
		}
		bg = th.BgContainer
		if t.color == "" || t.color == "default" {
			// The default tint's label may be meant for a filled badge
			// (shadcn primary-foreground on primary): the theme says what a
			// plain tag uses.
			fg, border = tk.PlainColor, tk.PlainBorderColor
		}
	}
	if t.isDisabled() {
		bg, fg = tk.DisabledBg, tk.DisabledColor
		if v == "outlined" || v == "plain" {
			border = tk.DisabledBorderColor
		} else {
			border = theme.Color{}
		}
	}
	return
}

func (t *TagW) paint(l *paint.List, r geom.Rect) {
	th := t.th
	bg, border, fg := t.colors()
	rad := geom.Rad(min(t.tk.Radius, r.H/2))
	if t.round {
		rad = geom.Rad(r.H / 2)
	}
	if bg.A > 0 {
		l.Fill(r, rad, bg)
	}
	if border.A > 0 {
		l.Stroke(r, rad, 1, border)
	}
	// Icon, label and close icon run from the inline start.
	s := t.c.inline(r)
	off := t.tagPad() + 1
	if t.ic != nil {
		t.c.drawIconAt(l, t.ic, s.X(off, 12), r.Y+(r.H-12)/2, 12, fg)
		off += 16
	}
	lay := text.Shape(t.label, t.font(), t.c.textOpts(text.Options{NoWrap: true}))
	ty := r.Y + (r.H-lay.Height)/2
	lx := s.X(off, lay.Width)
	l.Glyphs(geom.Pt(lx, ty), lay.Glyphs, fg)
	if t.closable {
		col := t.tk.CloseIconColor
		if col.A == 0 || t.isDisabled() {
			col = fg
		}
		ic := icon.Close
		if t.closeIcon != nil {
			ic = t.closeIcon
		}
		t.c.drawIconAt(l, ic, s.X(off+lay.Width+4, 10), r.Y+(r.H-10)/2, 10, col)
		if t.ia.FocusVisible {
			drawFocusRing(l, th, r, rad)
		}
	}
}

// ---------------------------------------------------------------------------
// Avatar

// AvatarW is a round user avatar.
type AvatarW struct {
	_ [0]func()

	c      *Context
	n      *node
	size   float32
	square bool
	bg     theme.Color
	bgSet  bool
	img    image.Image
	ic     *icon.Icon
	gap    float32
}

// Avatar shows text initials (or an icon when label is "").
//
// ConfigProvider.Defaults("Avatar", ...) keys: "size" (float32 px or
// ControlSize), "shape" ("circle" | "square"), "gap" (px, default 4).
func Avatar(c *Context, label string) *AvatarW {
	th := c.ThemeFor(theme.CompAvatar)
	tk := &th.Components.Avatar
	a := &AvatarW{c: c, size: tk.ContainerSize, bg: tk.Bg, gap: 4}
	switch v := Prop[any](c, "Avatar", "size", nil).(type) {
	case float32:
		a.size = v
	case int:
		a.size = float32(v)
	case ControlSize:
		a.size = avatarSize(tk, v)
	}
	if v, ok := numProp(c.ConfigProp("Avatar", "gap")); ok {
		a.gap = float32(v)
	}
	a.square = Prop(c, "Avatar", "shape", "circle") == "square"
	a.n = c.add("avatar", label)
	a.n.setRole(a11y.RoleImage)
	if label == "" {
		a.n.setName(c.Locale().A11y.Avatar)
	}
	a.n.ln.NoStretch = true
	a.n.ln.AlignSelf = layout.AlignAuto
	a.n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: a.size, H: a.size} }
	a.n.paint = func(l *paint.List, r geom.Rect) {
		// Sizes at or beyond the large / small container pick the large /
		// small text, icon and corner radius.
		fs, is, sq := tk.TextFontSize, tk.IconFontSize, tk.Radius
		if a.size >= tk.ContainerSizeLG {
			fs, is, sq = tk.TextFontSizeLG, tk.IconFontSizeLG, tk.RadiusLG
		} else if a.size <= tk.ContainerSizeSM {
			fs, is, sq = tk.TextFontSizeSM, tk.IconFontSizeSM, tk.RadiusSM
		}
		rad := geom.Rad(a.size / 2)
		if a.square {
			rad = geom.Rad(sq)
		}
		if a.img != nil {
			l.DrawImage(a.img, r, rad)
			return
		}
		l.Fill(r, rad, a.bg)
		fg := tk.Color
		if a.bgSet {
			fg = th.TextOn(a.bg, th.TextOnSolid) // custom colored background
		}
		if label == "" || a.ic != nil {
			ic := icon.User
			if a.ic != nil {
				ic = a.ic
			}
			// Icons keep their token size, scaled with custom avatar sizes.
			if a.size != tk.ContainerSize && a.size != tk.ContainerSizeLG && a.size != tk.ContainerSizeSM {
				is = a.size * tk.IconFontSize / tk.ContainerSize
			}
			c.drawIconAt(l, ic, r.X+(r.W-is)/2, r.Y+(r.H-is)/2, is, fg)
			return
		}
		st := c.style(fs, text.Regular)
		// antd gap: the text is scaled down to fit size − 2 × gap.
		if w := text.Width(label, st); tk.TextAutoScale && a.gap*2 < a.size && w > a.size-2*a.gap && w > 0 {
			st = c.style(fs*(a.size-2*a.gap)/w, text.Regular)
		}
		lay := text.Shape(label, st, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
	}
	return a
}

// avatarSize maps a ControlSize to the container size token.
func avatarSize(tk *theme.AvatarToken, s ControlSize) float32 {
	switch s {
	case Small:
		return tk.ContainerSizeSM
	case Large:
		return tk.ContainerSizeLG
	}
	return tk.ContainerSize
}

func (a *AvatarW) Size(px float32) *AvatarW       { a.size = px; return a }
func (a *AvatarW) Square() *AvatarW               { a.square = true; return a }
func (a *AvatarW) Bg(col theme.Color) *AvatarW    { a.bg, a.bgSet = col, true; return a }
func (a *AvatarW) Image(img image.Image) *AvatarW { a.img = img; return a }

// Small uses the small container size (antd size="small": 24px).
func (a *AvatarW) Small() *AvatarW { return a.sizeOf(Small) }

// Large uses the large container size (antd 40px, Element 56px).
func (a *AvatarW) Large() *AvatarW { return a.sizeOf(Large) }

func (a *AvatarW) sizeOf(s ControlSize) *AvatarW {
	tk := &a.c.ThemeFor(theme.CompAvatar).Components.Avatar
	a.size = avatarSize(tk, s)
	return a
}

// Shape sets "circle" (default) or "square".
func (a *AvatarW) Shape(s string) *AvatarW { a.square = s == "square"; return a }

// Gap sets the space kept on each side of the text; longer text is scaled
// down to fit (antd gap, default 4).
func (a *AvatarW) Gap(px float32) *AvatarW { a.gap = px; return a }

// Icon shows an icon instead of the text (icon avatar).
func (a *AvatarW) Icon(ic *icon.Icon) *AvatarW { a.ic = ic; return a }

// A11yLabel sets the accessible name (the person's name; the default is
// the avatar text, or "avatar" for an icon or image avatar).
func (a *AvatarW) A11yLabel(s string) *AvatarW { a.n.setName(s); return a }

// ---------------------------------------------------------------------------
// Statistic

// StatisticW shows a labelled number.
type StatisticW struct {
	_ [0]func()

	c      *Context
	n      *node
	title  string
	value  string
	prefix string
	suffix string
	color  theme.Color
	pIcon  *icon.Icon
	vst    text.Style
	th     *theme.Theme
	tk     *theme.StatisticToken
	raw    float64
	prec   int
	group  string
	dec    string
	custom bool // Value replaced the formatted number
}

// Statistic adds a statistic display. The value is shown with precision
// decimals, cut (not rounded) like antd and Element Plus; a negative
// precision shows the decimals the value has. NaN and ±Inf are shown as
// "NaN" and "Infinity".
//
// ConfigProvider.Defaults("Statistic", ...) keys: "groupSeparator",
// "decimalSeparator" (string).
func Statistic(c *Context, title string, value float64, precision int) *StatisticW {
	th := c.ThemeFor(theme.CompStatistic)
	tk := &th.Components.Statistic
	s := &StatisticW{c: c, title: title, th: th, tk: tk, raw: value, prec: precision,
		group: Prop(c, "Statistic", "groupSeparator", ","), dec: Prop(c, "Statistic", "decimalSeparator", ".")}
	s.value = statisticFormat(value, precision, s.group, s.dec)
	s.color = tk.ContentColor
	s.n = c.add("statistic", title)
	s.n.setRole(a11y.RoleMeter).fill(s)
	style := func(size float32, w int) text.Style {
		st := text.Style{Size: size, Weight: text.Regular, LineHeight: th.LineHeightFor(size)}
		if w != 0 {
			st.Weight = text.Weight(w)
		}
		return st
	}
	tst := style(tk.TitleFontSize, tk.TitleFontWeight)
	vst := style(tk.ContentFontSize, tk.ContentFontWeight)
	vst.LineHeight = tk.ContentLineHeight
	sst := style(tk.AffixFontSize, 0)
	gap := tk.TitleMarginBottom
	s.vst = vst
	s.n.ln.Measure = func(_, _ float32) geom.Size {
		vst := s.vst
		w := text.Width(s.value, vst) + text.Width(s.prefix, sst) + text.Width(s.suffix, sst) + 8
		if s.pIcon != nil {
			w += 20
		}
		th := tst.LineHeight + gap
		if s.title == "" {
			th = 0
		}
		return geom.Size{W: max(w, text.Width(s.title, tst)), H: th + s.vst.LineHeight}
	}
	s.n.paint = func(l *paint.List, r geom.Rect) {
		// Title, then icon, prefix, value and suffix from the inline start.
		sp := c.inline(r)
		tl := text.Shape(s.title, tst, c.textOpts(text.Options{NoWrap: true}))
		l.Glyphs(geom.Pt(sp.X(0, tl.Width), r.Y), tl.Glyphs, tk.TitleColor)
		y := r.Y + tst.LineHeight + gap
		if s.title == "" {
			y = r.Y
		}
		vst := s.vst
		var x float32
		if s.pIcon != nil {
			c.drawIconAt(l, s.pIcon, sp.X(x, 16), y+(vst.LineHeight-16)/2, 16, s.color)
			x += 20
		}
		if s.prefix != "" {
			pl := text.Shape(s.prefix, sst, text.Options{NoWrap: true})
			l.Glyphs(geom.Pt(sp.X(x, pl.Width), y+(vst.LineHeight-pl.Height)/2+2), pl.Glyphs, s.color)
			x += pl.Width + 4
		}
		vl := text.Shape(s.value, vst, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(sp.X(x, vl.Width), y), vl.Glyphs, s.color)
		x += vl.Width + 4
		if s.suffix != "" {
			sl := text.Shape(s.suffix, sst, text.Options{NoWrap: true})
			l.Glyphs(geom.Pt(sp.X(x, sl.Width), y+(vst.LineHeight-sl.Height)/2+2), sl.Glyphs, s.color)
		}
	}
	return s
}

func (s *StatisticW) Prefix(p string) *StatisticW          { s.prefix = p; return s }
func (s *StatisticW) Suffix(p string) *StatisticW          { s.suffix = p; return s }
func (s *StatisticW) Color(col theme.Color) *StatisticW    { s.color = col; return s }
func (s *StatisticW) PrefixIcon(ic *icon.Icon) *StatisticW { s.pIcon = ic; return s }
func (s *StatisticW) Value(v string) *StatisticW           { s.value, s.custom = v, true; return s }

// GroupSeparator sets the thousands separator (default ",").
func (s *StatisticW) GroupSeparator(sep string) *StatisticW {
	s.group = sep
	return s.reformat()
}

// DecimalSeparator sets the decimal point (default ".").
func (s *StatisticW) DecimalSeparator(sep string) *StatisticW {
	s.dec = sep
	return s.reformat()
}

// Formatter formats the value with fn instead (antd / Element formatter).
func (s *StatisticW) Formatter(fn func(v float64) string) *StatisticW {
	s.value, s.custom = fn(s.raw), true
	return s
}

func (s *StatisticW) reformat() *StatisticW {
	if !s.custom {
		s.value = statisticFormat(s.raw, s.prec, s.group, s.dec)
	}
	return s
}

// statisticFormat formats v like antd's StatisticNumber: the integer part
// grouped by threes with group, the decimals padded or cut to precision
// (negative: as they are) after dec.
func statisticFormat(v float64, precision int, group, dec string) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	}
	neg := v < 0
	raw := strconv.FormatFloat(math.Abs(v), 'f', -1, 64)
	intPart, frac := raw, ""
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		intPart, frac = raw[:i], raw[i+1:]
	}
	if precision >= 0 {
		for len(frac) < precision {
			frac += "0"
		}
		frac = frac[:precision]
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	for i, ch := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteString(group)
		}
		b.WriteRune(ch)
	}
	if frac != "" {
		b.WriteString(dec)
		b.WriteString(frac)
	}
	return b.String()
}

// ValueSize sets the value font size (line height follows).
func (s *StatisticW) ValueSize(px float32) *StatisticW {
	s.vst.Size = px
	s.vst.LineHeight = headingLineHeight(px)
	if px == 30 {
		s.vst.LineHeight = 38
	}
	return s
}

func (s *StatisticW) semantics(n *a11y.Node) {
	v := strings.TrimSpace(s.prefix + " " + s.value + " " + s.suffix)
	n.Value = v
	if n.Name == "" {
		n.Name = v
	}
}

// Layout applies layout options.
func (s *StatisticW) Layout(o ...layout.Option) *StatisticW {
	flexbox.Apply(&s.n.ln.Style, o...)
	return s
}

// FormatNumber formats v with thousands separators.
func FormatNumber(v float64, precision int) string {
	neg := v < 0
	v = math.Abs(v)
	s := strconv.FormatFloat(v, 'f', max(0, precision), 64)
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	var b strings.Builder
	for i, ch := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	out := b.String() + frac
	if neg {
		out = "-" + out
	}
	return out
}

// ---------------------------------------------------------------------------
// Tooltip

// TooltipW is a tooltip wrapped around trigger content. It embeds the
// anchor *Box, so code that used the box Tooltip returned keeps working.
// Options apply when the build finishes (or at IsOpen).
//
// ConfigProvider.Defaults("Tooltip", ...) keys: "placement", "trigger",
// "color", "effect" (string), "arrow" (bool), "mouseEnterDelay",
// "mouseLeaveDelay" (time.Duration).
type TooltipW struct {
	_ [0]func()

	*Box
	c       *Context
	tip     string
	content func()
	place   string
	trigger string
	color   string
	effect  string
	enter   time.Duration
	leave   time.Duration
	arrow   bool
	ia      Interaction
	z       *lazy
	isOpen  bool
	open    *bool // controlled open state (Open)
	changed bool
	off     bool // Disabled
	th      *theme.Theme
	tk      *theme.TooltipToken
}

type tooltipState struct {
	open    bool // click trigger
	panel   ID
	escShut bool // Escape hid a hover / focus tooltip until the trigger is left
	want    bool // the trigger's wish last frame (controlled mode edge detection)
}

// Tooltip wraps fn's content and shows tip above it while hovered (or
// while keyboard focus is inside). The hover delays default to the theme
// (antd 0.1 s / 0.1 s, Element 0 / 0.2 s, shadcn 0 / 0).
func Tooltip(c *Context, tip string, fn func()) *TooltipW {
	b := c.box("tooltip-anchor", layout.Column, nil)
	b.n.ln.NoStretch = true
	b.Body(fn)
	th := c.ThemeFor(theme.CompTooltip)
	tk := &th.Components.Tooltip
	t := &TooltipW{Box: b, c: c, tip: tip, th: th, tk: tk,
		place:   Prop(c, "Tooltip", "placement", "top"),
		trigger: Prop(c, "Tooltip", "trigger", "hover"),
		color:   Prop(c, "Tooltip", "color", ""),
		effect:  Prop(c, "Tooltip", "effect", "dark"),
		arrow:   Prop(c, "Tooltip", "arrow", true),
		enter:   Prop(c, "Tooltip", "mouseEnterDelay", tk.MouseEnterDelay),
		leave:   Prop(c, "Tooltip", "mouseLeaveDelay", tk.MouseLeaveDelay),
	}
	t.ia = b.Interact(input.CursorDefault)
	dataDescribe(c, b.n, func() string { return t.tip })
	t.z = c.lazy(b.n, t.build)
	return t
}

// Placement: top (default), topLeft, topRight, bottom, bottomLeft,
// bottomRight, left, leftTop, leftBottom, right, rightTop, rightBottom.
// Left and right are logical (start / end): mirrored in RTL.
func (t *TooltipW) Placement(s string) *TooltipW { t.place = s; return t }

// Trigger: "hover" (default; keyboard focus shows it too), "click" or
// "focus".
func (t *TooltipW) Trigger(s string) *TooltipW { t.trigger = s; return t }

// Color sets the bubble color: a status ("success", "error" ...), an Ant
// preset name ("blue", "magenta" ...) or "#rrggbb".
func (t *TooltipW) Color(s string) *TooltipW { t.color = s; return t }

// Effect "light" draws a light bubble with a border (Element effect);
// "dark" is the default.
func (t *TooltipW) Effect(s string) *TooltipW { t.effect = s; return t }

// Content replaces the text with rich content built by fn (the tip text
// still describes the trigger to screen readers).
func (t *TooltipW) Content(fn func()) *TooltipW { t.content = fn; return t }

// MouseEnterDelay waits d before showing on hover (Element show-after;
// default: the theme's TooltipToken.MouseEnterDelay).
func (t *TooltipW) MouseEnterDelay(d time.Duration) *TooltipW { t.enter = d; return t }

// MouseLeaveDelay keeps the tooltip for d after the pointer left
// (Element hide-after; default: TooltipToken.MouseLeaveDelay).
func (t *TooltipW) MouseLeaveDelay(d time.Duration) *TooltipW { t.leave = d; return t }

// Open binds the open state (antd open / onOpenChange): *v decides
// whether the tooltip shows, and the trigger writes its wish (hover,
// focus, click) into *v when it changes. OpenChanged reports such a write.
func (t *TooltipW) Open(v *bool) *TooltipW { t.open = v; return t }

// OpenChanged reports that the trigger changed the Open state this frame.
func (t *TooltipW) OpenChanged() bool { t.z.run(); return t.changed }

// Disabled stops the tooltip from showing (Element disabled).
func (t *TooltipW) Disabled(on bool) *TooltipW { t.off = on; return t }

// Arrow shows or hides the arrow (default on).
func (t *TooltipW) Arrow(on bool) *TooltipW { t.arrow = on; return t }

// IsOpen reports whether the tooltip is shown this frame.
func (t *TooltipW) IsOpen() bool { t.z.run(); return t.isOpen }

// colors returns the bubble background, text color and border.
func (t *TooltipW) colors() (bg, fg, border theme.Color) {
	th := t.th
	bg, fg = t.tk.Bg, t.tk.Color
	switch {
	case t.color == "" && t.effect == "light":
		return th.BgElevated, th.Text, th.Border
	case t.color == "":
		return bg, fg, theme.Color{}
	case t.color == "processing":
		bg = th.Primary.Base
	case t.color == "success", t.color == "warning", t.color == "error", t.color == "danger", t.color == "info", t.color == "primary":
		bg = th.ColorOf(t.color).Base
	default:
		hx := t.color
		if p, ok := antPreset[hx]; ok {
			hx = p
		}
		col, err := theme.ParseHex(hx)
		if err != nil {
			return bg, fg, theme.Color{}
		}
		bg = col
	}
	fg = th.TextOn(bg, th.TextOnSolid)
	return bg, fg, theme.Color{}
}

// build decides whether the tooltip shows and builds the bubble.
func (t *TooltipW) build() {
	c, th, tk := t.c, t.th, t.tk
	b := t.Box
	st := State[tooltipState](c, hashID(b.n.id, "tooltip"), nil)
	within := c.focusWithin(b.n)
	var show bool
	switch t.trigger {
	case "click":
		if t.open != nil {
			st.open = *t.open // controlled: the application's value is the state
		}
		if t.ia.Clicked || c.clickedWithin(b.n) || c.popKbdActivated(b.n) {
			st.open = !st.open
			c.invalid = true
		}
		if st.open && c.Dismissed(st.panel, b.n.id) {
			st.open = false
		}
		show = st.open
	case "focus":
		if !within {
			st.escShut = false
		}
		show = within && !st.escShut
	default:
		// Keyboard users see the tip while focus is on the trigger.
		kbd := c.focusVisible && within
		if !t.ia.Hovered && !kbd {
			st.escShut = false
		}
		show = HoverDelay(c, b.n.id, t.ia.Hovered || kbd, t.enter, t.leave) && !st.escShut
	}
	if t.off {
		// Disabled: never shown, and the controlled state is left alone.
		t.isOpen, st.panel, st.want = false, 0, false
		return
	}
	ctrlOpen := t.open != nil && *t.open
	esc := false
	if (show || ctrlOpen) && t.trigger != "click" && c.EscapeAt(st.panel) {
		esc = true
		st.escShut, show = true, false
		c.invalid = true
	}
	if t.open != nil {
		switch {
		case esc && *t.open:
			// Escape closes an application-opened tooltip too.
			*t.open, t.changed = false, true
			st.want = false
		case t.trigger == "click":
			// st.open was synced from *t.open: any difference is this
			// frame's click or dismissal.
			if show != *t.open {
				*t.open, t.changed = show, true
				c.invalid = true
			}
		case show != st.want:
			// Hover / focus: the trigger's wish is written only when it
			// changes, so the application can open or close the tooltip
			// itself.
			st.want = show
			if *t.open != show {
				*t.open, t.changed = show, true
				c.invalid = true
			}
		}
		show = *t.open
	}
	t.isOpen = show
	if !show {
		st.panel = 0
		return
	}
	bg, fg, border := t.colors()
	place, _ := c.popPlacement(t.place)
	off := tk.Offset
	if !t.arrow {
		off = 4
	}
	a := c.AnimFrom(b.n.id, "tip", 0, 1, th.DurationFast)
	ov := Overlay(c, OverlayOptions{Anchor: b.n.id, Place: place, Offset: off, Flip: true}, func() {
		if t.content != nil {
			Column(c, t.content, layout.MaxW(tk.MaxWidth))
			return
		}
		tx := Text(c, t.tip).Color(fg).Layout(layout.MaxW(tk.MaxWidth))
		tx.n.aHidden = true // the tooltip node carries the text
		if tk.ContentFontSize != c.Theme.FontSize {
			tx.Size(tk.ContentFontSize)
		}
	}, layout.PadVH(tk.PaddingBlock, tk.PaddingInline), layout.MinH(tk.MinHeight))
	ov.Bg(bg).Radius(tk.Radius).Shadow(tk.Shadow).Opacity(a)
	if border.A > 0 {
		ov.Border(1, border)
	}
	ov.n.setRole(a11y.RoleTooltip).setName(t.tip)
	ov.n.hit = false
	st.panel = ov.n.id
	if c.a11yOn {
		if trig := dataFirstSem(b.n, true); trig != nil {
			pid := ov.n.id
			trig.fill(semFunc(func(s *a11y.Node) { s.DescribedBy = a11y.NodeID(pid) }))
		}
	}
	if !t.arrow {
		return
	}
	anchor := b.n.id
	ov.n.over = func(l *paint.List, r geom.Rect) {
		ar, ok := c.anchorRect(anchor)
		if !ok {
			return
		}
		tipArrow(l, r, ar, bg, border)
	}
}

// tipArrow draws the tooltip arrow on the side of bubble r facing the
// anchor ar, pointing at the anchor's centre (kept inside the bubble).
func tipArrow(l *paint.List, r, ar geom.Rect, bg, border theme.Color) {
	const half, h = 6, 6
	p := paint.NewPath()
	edge := paint.NewPath()
	switch {
	case r.Bottom() <= ar.Y+1: // bubble above
		cx := max(r.X+8+half, min(ar.X+ar.W/2, r.Right()-8-half))
		p.MoveTo(cx-half, r.Bottom()-0.5).LineTo(cx, r.Bottom()+h-0.5).LineTo(cx+half, r.Bottom()-0.5).Close()
		edge.MoveTo(cx-half, r.Bottom()-0.5).LineTo(cx, r.Bottom()+h-0.5).LineTo(cx+half, r.Bottom()-0.5)
	case r.Y >= ar.Bottom()-1: // below
		cx := max(r.X+8+half, min(ar.X+ar.W/2, r.Right()-8-half))
		p.MoveTo(cx-half, r.Y+0.5).LineTo(cx, r.Y-h+0.5).LineTo(cx+half, r.Y+0.5).Close()
		edge.MoveTo(cx-half, r.Y+0.5).LineTo(cx, r.Y-h+0.5).LineTo(cx+half, r.Y+0.5)
	case r.Right() <= ar.X+1: // left of the anchor
		cy := max(r.Y+6+half, min(ar.Y+ar.H/2, r.Bottom()-6-half))
		p.MoveTo(r.Right()-0.5, cy-half).LineTo(r.Right()+h-0.5, cy).LineTo(r.Right()-0.5, cy+half).Close()
		edge.MoveTo(r.Right()-0.5, cy-half).LineTo(r.Right()+h-0.5, cy).LineTo(r.Right()-0.5, cy+half)
	default: // right of the anchor
		cy := max(r.Y+6+half, min(ar.Y+ar.H/2, r.Bottom()-6-half))
		p.MoveTo(r.X+0.5, cy-half).LineTo(r.X-h+0.5, cy).LineTo(r.X+0.5, cy+half).Close()
		edge.MoveTo(r.X+0.5, cy-half).LineTo(r.X-h+0.5, cy).LineTo(r.X+0.5, cy+half)
	}
	l.Path(p, bg)
	if border.A > 0 {
		l.StrokePath(edge, paint.StrokeStyle{Width: 1}, border)
	}
}
