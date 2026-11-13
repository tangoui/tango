package ui

// Shared helpers of the data-entry components (entry_*.go). Everything here
// is prefixed "ent" to stay clear of other component families. Parts that
// are generally useful (glyph icons, popup state) are candidates for the
// base layer; see the notes on each helper.

import (
	"strings"
	"unicode"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Popup state

// entPopup is the retained open state of a widget with a dropdown panel.
type entPopup struct {
	open  bool
	panel ID // overlay root of the last frame; look its rect up via c.rects
}

// dismiss closes the popup on a press outside the anchor and the panel (and
// any extra widgets) or on Escape. It reports whether it closed.
func (p *entPopup) dismiss(c *Context, anchor ID, extra ...ID) bool {
	if !p.open {
		return false
	}
	if c.Dismissed(p.panel, append([]ID{anchor}, extra...)...) {
		p.open = false
		c.invalid = true
		return true
	}
	return false
}

// entPanel opens a dropdown overlay under anchor styled for the active
// design system and records its ID in p. match makes it at least as wide as
// the anchor.
func entPanel(c *Context, anchor ID, p *entPopup, match bool, fn func()) *Box {
	th := c.Theme
	a := c.AnimFrom(anchor, "ent-open", 0, 1, th.DurationMid)
	ov := Overlay(c, OverlayOptions{Anchor: anchor, Place: BottomStart, Offset: 4, MatchWidth: match, Flip: true, ShiftY: (1 - a) * -4}, fn)
	entStylePanel(c, ov)
	ov.Opacity(a)
	p.panel = ov.n.id
	return ov
}

// entStylePanel gives a floating panel its background, radius, border and
// shadow from the Select token (PopupBg, PopupRadius, PopupShadow), so every select-like dropdown panel
// (Cascader, TreeSelect, AutoComplete, Mentions, VirtualSelect …) follows
// it. Element keeps its lighter --el-border-color-lighter outline on these
// panels (Select's own list uses PopupBorderColor).
func entStylePanel(c *Context, b *Box) {
	th := c.ThemeFor(theme.CompSelect)
	tk := &th.Components.Select
	b.Bg(tk.PopupBg).Radius(tk.PopupRadius).Shadow(tk.PopupShadow)
	if th.Flavor == theme.Element {
		b.Border(1, th.BorderSecondary)
	} else if tk.PopupBorderColor.A > 0 {
		b.Border(1, tk.PopupBorderColor)
	}
	b.n.clip = true
}

// entPopupBorder outlines a floating panel when the theme asks for it
// (shadcn's bordered popovers); Element panels set their own border.
func entPopupBorder(c *Context, b *Box) {
	th := c.Theme
	if th.Flavor != theme.Element && th.PopupBorder.A > 0 {
		b.Border(1, th.PopupBorder)
	}
}

// entListPad is the vertical/horizontal padding of option lists.
func entListPad(c *Context) geom.Insets {
	if c.Theme.Flavor == theme.Element {
		return geom.VH(6, 0)
	}
	return geom.All(4)
}

// ---------------------------------------------------------------------------
// Option rows

// entOptionH is the height of a dropdown option row.
func entOptionH(c *Context) float32 {
	return c.ThemeFor(theme.CompSelect).Components.Select.OptionHeight
}

// entOptState describes how an option row is drawn.
type entOptState struct {
	selected bool // current value (single) or checked (multiple)
	active   bool // keyboard / hover highlight
	disabled bool
	check    bool // multiple mode: draw a check mark when selected
	padL     float32
}

// entPaintOption draws an option row of a dropdown list in r.
// The label starts at the inline start of the row, the check mark sits at
// its end.
func entPaintOption(c *Context, l *paint.List, r geom.Rect, label string, s entOptState) {
	th := c.ThemeFor(theme.CompSelect)
	tk := &th.Components.Select
	if th.Flavor == theme.StyleShadcn {
		entPaintOptionShadcn(c, l, r, label, s)
		return
	}
	el := th.Flavor == theme.Element
	rad := geom.Rad(th.RadiusSM)
	if el {
		rad = geom.Radii{}
	}
	col, wt := th.Text, text.Regular
	if el {
		col = th.TextSecondary
	}
	switch {
	case s.disabled:
		col = th.TextDisabled
	case s.selected && el:
		col, wt = tk.OptionSelectedColor, themeWeight(tk.OptionSelectedFontWeight)
		if s.active {
			l.Fill(r, rad, tk.OptionActiveBg)
		}
	case s.selected:
		l.Fill(r, rad, tk.OptionSelectedBg)
		col, wt = tk.OptionSelectedColor, themeWeight(tk.OptionSelectedFontWeight)
	case s.active:
		l.Fill(r, rad, tk.OptionActiveBg)
	}
	padL := s.padL
	if padL == 0 {
		padL = tk.OptionPadding
	}
	padR := float32(12)
	if s.check {
		padR += 18
	}
	entTextIn(c, l, label, padL, r, tk.OptionFontSize, wt, col, r.W-padL-padR)
	if s.check && s.selected {
		ic := float32(14)
		mc := th.Primary.Base
		if s.disabled {
			mc = th.TextDisabled
		}
		drawIcon(l, icon.Check, c.inline(r).X(r.W-12-ic, ic), r.Y+(r.H-ic)/2, ic, mc)
	}
}

// entPaintOptionShadcn draws a shadcn Select item: accent background only
// under the pointer / keyboard, the selection marked by a check on the right.
func entPaintOptionShadcn(c *Context, l *paint.List, r geom.Rect, label string, s entOptState) {
	th := c.ThemeFor(theme.CompSelect)
	tk := &th.Components.Select
	col := th.Text
	if s.selected {
		col = tk.OptionSelectedColor
	}
	if s.active && !s.disabled {
		l.Fill(r, geom.Rad(th.RadiusSM), tk.OptionActiveBg)
		col = th.ItemSelected
	}
	if s.disabled {
		col = th.TextDisabled
	}
	padL := s.padL
	if padL == 0 {
		padL = tk.OptionPadding
	}
	wt := text.Regular
	if s.selected {
		wt = themeWeight(tk.OptionSelectedFontWeight)
	}
	ic := float32(16)
	padR := float32(8) + ic + 8
	entTextIn(c, l, label, padL, r, tk.OptionFontSize, wt, col, r.W-padL-padR)
	if s.selected {
		drawIcon(l, icon.Check, c.inline(r).X(r.W-8-ic, ic), r.Y+(r.H-ic)/2, ic, col)
	}
}

// entArrowLook returns the size, right inset and color of a selector's
// trailing arrow: shadcn uses a 16px chevron at half opacity.
func entArrowLook(c *Context) (size, right float32, col theme.Color) {
	th := c.ThemeFor(theme.CompSelect)
	switch th.Flavor {
	case theme.Element:
		return 14, 11, th.TextQuaternary
	case theme.StyleShadcn:
		return 16, th.ControlPaddingH, theme.Fade(th.Text, .5)
	}
	return 12, 11, th.TextQuaternary
}

// ---------------------------------------------------------------------------
// Text

// entFont returns a regular/weighted style at size.
func entFont(c *Context, size float32, w text.Weight) text.Style { return c.style(size, w) }

// entText draws a single line vertically centred in r starting at x,
// truncated with an ellipsis at maxW (0 = no limit). It returns the width.
func entText(c *Context, l *paint.List, s string, x float32, r geom.Rect, size float32, w text.Weight, col theme.Color, maxW float32) float32 {
	if s == "" {
		return 0
	}
	opt := text.Options{NoWrap: true}
	if maxW > 0 {
		opt.Ellipsis, opt.MaxWidth = true, maxW
	}
	lay := text.Shape(s, c.style(size, w), opt)
	l.Glyphs(geom.Pt(x, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	return lay.Width
}

// strongDir is the direction of the first strong character of s: -1
// right-to-left, 1 left-to-right, 0 none (digits, punctuation).
func strongDir(s string) int {
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Hebrew, unicode.Arabic, unicode.Syriac, unicode.Thaana, unicode.Nko, unicode.Samaritan, unicode.Mandaic, unicode.Adlam):
			return -1
		case unicode.IsLetter(r):
			return 1
		}
	}
	return 0
}

// contentRTL reports whether user content s lays out right to left: by its
// first strong character (DirAuto), else by the scope's direction.
func (c *Context) contentRTL(s string) bool {
	if d := strongDir(s); d != 0 {
		return d < 0
	}
	return c.IsRTL()
}

// entEditorRect is the rectangle a single-line editor paints into inside
// its box r so that the value (or, when empty, the placeholder and the
// caret) sits at the start of its paragraph direction: r itself for
// left-to-right content, a rectangle hugging r's right edge for
// right-to-left content that fits. The value keeps DirAuto; only an empty
// field takes the scope's direction.
func (c *Context) entEditorRect(r geom.Rect, id ID, val, placeholder string, st text.Style, password bool) geom.Rect {
	show := val
	if password {
		show = strings.Repeat("•", len([]rune(val)))
	}
	if comp, _ := c.CompositionFor(id); comp != "" {
		show += comp
	}
	rtl := c.IsRTL()
	if show != "" {
		rtl = c.contentRTL(show)
	} else if placeholder != "" {
		show = placeholder // painted by paintEditor: keep its room
	}
	if !rtl {
		return r
	}
	tw := text.Width(show, st) + 1
	if tw >= r.W {
		return r
	}
	return geom.Rect{X: r.Right() - tw, Y: r.Y, W: tw, H: r.H}
}

// entPaintEdit paints a single-line editor (paintEditor) whose value
// follows its own paragraph direction (entEditorRect) while the
// placeholder is interface text: shaped in the scope's direction and
// aligned to the scope's start, so an RTL placeholder truncates at its
// logical end.
func (c *Context) entPaintEdit(l *paint.List, r geom.Rect, id ID, e *editState, val string, st text.Style, col, phCol theme.Color, placeholder string, password bool) {
	comp := ""
	if c.focus == id {
		comp, _ = c.CompositionFor(id)
	}
	if val == "" && comp == "" && placeholder != "" {
		pl := text.Shape(placeholder, st, c.textOpts(text.Options{NoWrap: true}))
		x := r.X
		if c.IsRTL() {
			x = r.Right() - pl.Width
		}
		l.Glyphs(geom.Pt(x, r.Y+(r.H-pl.LineHeight)/2), pl.Glyphs, phCol)
		placeholder = ""
	}
	er := c.entEditorRect(r, id, val, placeholder, st, password)
	c.paintEditor(l, er, id, e, val, st, col, "", password, false)
}

// entTextIn is entText placed off from the inline start of r: left-aligned
// in LTR, right-aligned in RTL. It returns the width.
func entTextIn(c *Context, l *paint.List, s string, off float32, r geom.Rect, size float32, w text.Weight, col theme.Color, maxW float32) float32 {
	if s == "" {
		return 0
	}
	if !c.IsRTL() {
		return entText(c, l, s, r.X+off, r, size, w, col, maxW)
	}
	opt := text.Options{NoWrap: true}
	if maxW > 0 {
		opt.Ellipsis, opt.MaxWidth = true, maxW
	}
	lay := text.Shape(s, c.style(size, w), opt)
	l.Glyphs(geom.Pt(c.inline(r).X(off, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	return lay.Width
}

// entTextW measures a single line.
func entTextW(c *Context, s string, size float32, w text.Weight) float32 {
	return text.Width(s, c.style(size, w))
}

func itoaEnt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// ---------------------------------------------------------------------------
// Small drawing helpers

// entArrow draws the dropdown arrow of a selector at the inline end of r
// (rotating under Element when open) and returns its rect.
func entArrow(c *Context, l *paint.List, r geom.Rect, id ID, open bool, ic *icon.Icon) geom.Rect {
	th := c.Theme
	is, right, col := entArrowLook(c)
	ar := c.inline(r).Rect(r.W-right-is, r.Y+(r.H-is)/2, is, is)
	if ic == nil {
		ic = icon.Down
	}
	ang := 0.0
	if th.Flavor == theme.Element && ic == icon.Down {
		ang = float64(c.Anim(id, "ent-arrow", b2f(open), th.DurationMid)) * 3.14159265
	}
	c.paintIcon(l, ic, ar, col, ang)
	return ar
}

// entClearIcon draws the round clear button used by selectors.
func entClearIcon(c *Context, l *paint.List, r geom.Rect) {
	th := c.Theme
	if th.Flavor == theme.StyleShadcn {
		// lucide circle-x in the muted foreground.
		icon.Draw(l, icon.CloseCircle, r, th.TextTertiary, 0)
		return
	}
	drawStatusIcon(l, "error", r, th.TextQuaternary, th.BgContainer)
}

// entFocusRing draws the keyboard focus ring around r.
func entFocusRing(c *Context, l *paint.List, r geom.Rect, rad float32) {
	th := c.Theme
	switch th.Flavor {
	case theme.Element:
		l.Stroke(r.Expand(2), geom.Rad(rad+2), 2, th.Primary.Shades[3])
		return
	case theme.StyleShadcn:
		shadFocusRing(l, th, r, geom.Rad(rad), false)
		return
	}
	l.Stroke(r.Expand(3), geom.Rad(rad+3), 2, th.FocusRing)
}
