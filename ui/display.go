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
// "orientation" ("left" | "right" | "center"), "plain" (bool).
type DividerW struct {
	*Box
	dashed bool
	orient string
	plain  bool
}

// Divider draws a horizontal rule, optionally with a title.
func Divider(c *Context, title ...string) *DividerW {
	th := c.ThemeFor(theme.CompDivider)
	tk := &th.Components.Divider
	label := strings.Join(title, " ")
	b := c.box("divider", layout.Column, nil)
	d := &DividerW{Box: b, dashed: Prop(c, "Divider", "dashed", false), orient: Prop(c, "Divider", "orientation", "center"), plain: Prop(c, "Divider", "plain", false)}
	if Prop(c, "Divider", "variant", "") == "dashed" {
		d.dashed = true
	}
	b.n.setRole(a11y.RoleSeparator).setState(a11y.Horizontal).setName(label)
	b.n.ln.Margin = geom.Insets{Top: tk.MarginBlock, Bottom: tk.MarginBlock}
	if label != "" {
		b.n.ln.Margin = geom.Insets{Top: tk.TextMarginBlock, Bottom: tk.TextMarginBlock}
	}
	font := func() text.Style {
		w := text.Weight(tk.TextFontWeight)
		if d.plain {
			w = text.Regular
		}
		return text.Style{Size: tk.FontSize, Weight: w, LineHeight: th.LineHeightFor(tk.FontSize)}
	}
	pad := tk.TextPaddingInline
	b.n.ln.Measure = func(_, _ float32) geom.Size {
		if label == "" {
			return geom.Size{H: tk.LineWidth}
		}
		return geom.Size{W: text.Width(label, font()) + 2*pad, H: 22}
	}
	b.n.paint = func(l *paint.List, r geom.Rect) {
		cy := r.Y + r.H/2
		lw := tk.LineWidth
		line := func(x0, x1 float32) {
			if x1 <= x0 {
				return
			}
			if d.dashed {
				p := paint.NewPath().MoveTo(x0, cy).LineTo(x1, cy)
				l.StrokePath(p, paint.StrokeStyle{Width: lw, Dash: []float32{4, 3}}, tk.Color)
				return
			}
			l.Fill(geom.Rect{X: x0, Y: cy - lw/2, W: x1 - x0, H: lw}, geom.Radii{}, tk.Color)
		}
		if label == "" {
			line(r.X, r.Right())
			return
		}
		lay := text.Shape(label, font(), c.textOpts(text.Options{NoWrap: true}))
		tx := r.X + (r.W-lay.Width)/2
		// Ant orientationMargin: the short side is 5% of the width. "left"
		// and "right" are logical (start / end) like Ant in RTL.
		switch d.orient {
		case "left", "start":
			tx = r.X + r.W*tk.OrientationMargin + pad
		case "right", "end":
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
		l.Glyphs(geom.Pt(tx, r.Y), lay.Glyphs, col)
	}
	return d
}

// Dashed draws a dashed line.
func (d *DividerW) Dashed() *DividerW { d.dashed = true; return d }

// Orientation places the title: "left", "right" or "center" (default;
// Ant titlePlacement / Element content-position).
func (d *DividerW) Orientation(s string) *DividerW { d.orient = s; return d }

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
	n.paint = func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, tk.Color) }
}

// ---------------------------------------------------------------------------
// Tag

// TagW is a small label.
type TagW struct {
	c        *Context
	n        *node
	label    string
	color    string // "", status names, or "#rrggbb"
	closable bool
	closed   bool
	ic       *icon.Icon
	ia       Interaction
	bordered bool
	kbdClose bool // closed with the keyboard or an accessibility action
	size     ControlSize
	effect   string // "" / "light", "dark", "plain"
	th       *theme.Theme
	tk       *theme.TagToken
}

// Tag adds a tag. color may be "success", "processing", "error",
// "warning", "default", or a hex color.
//
// ConfigProvider.Defaults("Tag", ...) keys: "size" (ControlSize), "effect"
// (string), "bordered" (bool).
func Tag(c *Context, label string) *TagW {
	t := &TagW{c: c, label: label, bordered: Prop(c, "Tag", "bordered", true), size: Middle, effect: Prop(c, "Tag", "effect", ""), color: Prop(c, "Tag", "color", "")}
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
	return t
}

func (t *TagW) Color(col string) *TagW   { t.color = col; return t }
func (t *TagW) Icon(ic *icon.Icon) *TagW { t.ic = ic; return t }
func (t *TagW) Borderless() *TagW        { t.bordered = false; return t }

// Size sets small (20px), middle (default) or large (Element 32px) tags.
func (t *TagW) Size(sz ControlSize) *TagW { t.size = sz; return t }

// Effect sets the Element theme of the tag: "light" (default, tinted),
// "dark" (solid color, white text) or "plain" (container background,
// colored border and text). In Ant these are the filled / solid /
// outlined variants.
func (t *TagW) Effect(e string) *TagW { t.effect = e; return t }

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
	return t
}

// Closed reports a click on the close icon (or a keyboard close).
func (t *TagW) Closed() bool {
	if !t.closable {
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

// effectColors restyles the tint colors for the dark / plain effects.
func (t *TagW) effectColors(bg, border, fg theme.Color) (theme.Color, theme.Color, theme.Color) {
	th := t.th
	switch t.effect {
	case "dark", "solid":
		base := fg
		if t.color == "" || t.color == "default" {
			if th.Flavor == theme.Element {
				base = th.Primary.Base
			} else {
				base = th.TextSecondary
			}
		}
		txt := t.tk.SolidTextColor
		if theme.Luma(base) > 0.75 {
			txt = th.Text
		}
		return base, base, txt
	case "plain", "outlined":
		if th.Flavor == theme.Element {
			// Element plain: light-5 border.
			switch t.color {
			case "", "default", "processing":
				border = th.Primary.Border
			case "success", "warning", "error", "danger", "info", "primary":
				border = th.ColorOf(t.color).Border
			}
		}
		return th.BgContainer, border, fg
	}
	return bg, border, fg
}

func (t *TagW) colors() (bg, border, fg theme.Color) {
	th, tk := t.th, t.tk
	if t.color == "" || t.color == "default" {
		return tk.DefaultBg, tk.BorderColor, tk.DefaultColor
	}
	if th.Flavor == theme.StyleShadcn {
		// shadcn Badge variants: default (primary), secondary, outline,
		// destructive; status colors use a soft tint.
		switch t.color {
		case "primary":
			return th.Primary.Base, theme.Color{}, th.TextOnPrimary
		case "secondary":
			return th.SecondaryBg, theme.Color{}, th.SecondaryText
		case "outline":
			return theme.Color{}, th.Border, th.Text
		case "destructive":
			return th.Error.Base, theme.Color{}, th.TextOnSolid
		case "success", "warning", "error", "danger", "info", "processing":
			p := th.ColorOf(t.color)
			return p.Bg, theme.Color{}, p.Shades[6] // deeper shade reads on the tint
		}
	}
	switch t.color {
	case "processing":
		p := th.Primary
		return p.Bg, p.Border, p.Base
	case "success", "warning", "error", "danger", "info", "primary":
		p := th.ColorOf(t.color)
		if th.Flavor == theme.Element {
			return p.Bg, p.BgHover, p.Base
		}
		return p.Bg, p.Border, p.Base
	}
	if col, err := theme.ParseHex(t.color); err == nil {
		pal := theme.AntPalette(col, th.Dark, th.BgContainer)
		return pal.Bg, pal.Border, pal.Active
	}
	return th.FillQuaternary, th.Border, th.Text
}

func (t *TagW) paint(l *paint.List, r geom.Rect) {
	th := t.th
	bg, border, fg := t.colors()
	if th.Flavor != theme.StyleShadcn {
		bg, border, fg = t.effectColors(bg, border, fg)
	}
	rad := geom.Rad(t.tk.Radius)
	l.Fill(r, rad, bg)
	if t.bordered && border.A > 0 {
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
	l.Glyphs(geom.Pt(s.X(off, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
	if t.closable {
		col := t.tk.CloseIconColor
		if col.A == 0 {
			col = fg
		}
		drawIcon(l, icon.Close, s.X(off+lay.Width+4, 10), r.Y+(r.H-10)/2, 10, col)
		if t.ia.FocusVisible {
			drawFocusRing(l, th, r, rad)
		}
	}
}

// ---------------------------------------------------------------------------
// Avatar

// AvatarW is a round user avatar.
type AvatarW struct {
	n      *node
	size   float32
	square bool
	bg     theme.Color
	bgSet  bool
	img    image.Image
	ic     *icon.Icon
}

// Avatar shows text initials (or an icon when label is "").
func Avatar(c *Context, label string) *AvatarW {
	th := c.ThemeFor(theme.CompAvatar)
	tk := &th.Components.Avatar
	a := &AvatarW{size: tk.ContainerSize, bg: tk.Bg}
	// ConfigProvider.Defaults("Avatar", ...) keys: "size" (float32 px or
	// ControlSize), "shape" ("circle" | "square").
	switch v := Prop[any](c, "Avatar", "size", nil).(type) {
	case float32:
		a.size = v
	case int:
		a.size = float32(v)
	case ControlSize:
		a.size = map[ControlSize]float32{Small: tk.ContainerSizeSM, Middle: tk.ContainerSize, Large: tk.ContainerSizeLG}[v]
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
		rad := geom.Rad(a.size / 2)
		if a.square {
			rad = geom.Rad(th.Radius)
		}
		if a.img != nil {
			l.DrawImage(a.img, r, rad)
			return
		}
		l.Fill(r, rad, a.bg)
		fg := tk.Color
		if a.bgSet {
			fg = th.TextOnSolid // custom colored background
			if theme.Luma(a.bg) > 0.75 {
				fg = th.Text // light custom fill (shadcn dark primary): dark mark
				if theme.Luma(th.Text) > 0.5 {
					fg = th.BgContainer
				}
			}
		}
		if label == "" || a.ic != nil {
			ic := icon.User
			if a.ic != nil {
				ic = a.ic
			}
			s := a.size * 0.56
			c.drawIconAt(l, ic, r.X+(r.W-s)/2, r.Y+(r.H-s)/2, s, fg)
			return
		}
		fs := tk.TextFontSize
		if a.size >= tk.ContainerSizeLG {
			fs = tk.TextFontSizeLG
		} else if a.size <= tk.ContainerSizeSM {
			fs = tk.TextFontSizeSM
		}
		lay := text.Shape(label, c.style(fs, text.Regular), text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
	}
	return a
}

func (a *AvatarW) Size(px float32) *AvatarW       { a.size = px; return a }
func (a *AvatarW) Square() *AvatarW               { a.square = true; return a }
func (a *AvatarW) Bg(col theme.Color) *AvatarW    { a.bg, a.bgSet = col, true; return a }
func (a *AvatarW) Image(img image.Image) *AvatarW { a.img = img; return a }

// Icon shows an icon instead of the text (icon avatar).
func (a *AvatarW) Icon(ic *icon.Icon) *AvatarW { a.ic = ic; return a }

// A11yLabel sets the accessible name (the person's name; the default is
// the avatar text, or "avatar" for an icon or image avatar).
func (a *AvatarW) A11yLabel(s string) *AvatarW { a.n.setName(s); return a }

// ---------------------------------------------------------------------------
// Statistic

// StatisticW shows a labelled number.
type StatisticW struct {
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
}

// Statistic adds a statistic display.
func Statistic(c *Context, title string, value float64, precision int) *StatisticW {
	th := c.ThemeFor(theme.CompStatistic)
	tk := &th.Components.Statistic
	s := &StatisticW{c: c, title: title, value: FormatNumber(value, precision), th: th, tk: tk}
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
func (s *StatisticW) Value(v string) *StatisticW           { s.value = v; return s }

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
func (s *StatisticW) Layout(o ...layout.Option) *StatisticW { s.n.ln.Style.Apply(o...); return s }

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
	th      *theme.Theme
	tk      *theme.TooltipToken
}

type tooltipState struct {
	open    bool // click trigger
	panel   ID
	escShut bool // Escape hid a hover / focus tooltip until the trigger is left
}

// Tooltip wraps fn's content and shows tip above it while hovered (or
// while keyboard focus is inside).
func Tooltip(c *Context, tip string, fn func()) *TooltipW {
	b := c.box("tooltip-anchor", layout.Column, nil)
	b.n.ln.NoStretch = true
	b.Body(fn)
	t := &TooltipW{Box: b, c: c, tip: tip,
		place:   Prop(c, "Tooltip", "placement", "top"),
		trigger: Prop(c, "Tooltip", "trigger", "hover"),
		color:   Prop(c, "Tooltip", "color", ""),
		effect:  Prop(c, "Tooltip", "effect", "dark"),
		arrow:   Prop(c, "Tooltip", "arrow", true),
		enter:   Prop(c, "Tooltip", "mouseEnterDelay", time.Duration(0)),
		leave:   Prop(c, "Tooltip", "mouseLeaveDelay", time.Duration(0)),
	}
	t.th = c.ThemeFor(theme.CompTooltip)
	t.tk = &t.th.Components.Tooltip
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

// MouseEnterDelay waits d before showing on hover (Element show-after).
func (t *TooltipW) MouseEnterDelay(d time.Duration) *TooltipW { t.enter = d; return t }

// MouseLeaveDelay keeps the tooltip for d after the pointer left
// (Element hide-after).
func (t *TooltipW) MouseLeaveDelay(d time.Duration) *TooltipW { t.leave = d; return t }

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
	fg = th.TextOnSolid
	if theme.Luma(bg) > 0.75 {
		fg = th.Text
	}
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
	if show && t.trigger != "click" && c.EscapeAt(st.panel) {
		st.escShut, show = true, false
		c.invalid = true
	}
	t.isOpen = show
	if !show {
		st.panel = 0
		return
	}
	bg, fg, border := t.colors()
	place, _ := c.popPlacement(t.place)
	off := float32(8)
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
