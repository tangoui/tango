package ui

import (
	"unicode/utf8"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Variant sets the look of the frame (Ant Input variant): "outlined"
// (default), "filled" (grey fill, border on focus), "borderless" (no
// frame) or "underlined" (bottom line only).
func (w *InputW) Variant(v string) *InputW { w.variant = v; return w }

// Bg replaces the background color of the frame (a grey search field on a
// white bar). It keeps hover / focus borders unless NoBorder is set.
func (w *InputW) Bg(col theme.Color) *InputW { w.bg = &col; return w }

// NoBorder drops the border and the focus halo but keeps the background
// (Bg or the variant's), e.g. a filled search bar.
func (w *InputW) NoBorder() *InputW { w.noBorder = true; return w }

// ShowCount shows the character count ("3 / 20" with MaxLength) at the end
// of the field (Ant showCount, Element show-word-limit).
func (w *InputW) ShowCount() *InputW { w.showCount = true; return w }

// configDefaults applies ConfigProvider Defaults("Input", ...): "variant"
// (string), "allowClear" and "showCount" (bool), "maxLength" (int).
func (w *InputW) configDefaults() {
	c := w.c
	w.variant = propString(c, "Input", "variant", w.variant)
	w.allowClear = propBool(c, "Input", "allowClear", w.allowClear)
	w.showCount = propBool(c, "Input", "showCount", w.showCount)
	w.maxLen = int(propInt64(c, "Input", "maxLength", int64(w.maxLen)))
}

func (w *InputW) countText() string {
	n := utf8.RuneCountInString(*w.val)
	if w.maxLen > 0 {
		return itoaEnt(n) + " / " + itoaEnt(w.maxLen)
	}
	return itoaEnt(n)
}

// countWidth is the room the count takes at the right (with its gap).
func (w *InputW) countWidth() float32 {
	if !w.showCount {
		return 0
	}
	return text.Width(w.countText(), w.countFont()) + 4
}

func (w *InputW) countFont() text.Style {
	return w.c.style(w.fontSize(), text.Regular)
}

// countNode adds the count as a polite live region at the right edge of
// the frame, so screen readers hear the count change while typing.
func (w *InputW) countNode() {
	s := w.countText()
	n := &node{id: hashID(w.box.id, "count"), parent: w.box, label: s}
	n.ln.Ref = n
	n.ln.Style.Apply(layout.Abs(layout.Unset, 0, w.padX(), 0))
	n.ln.Width = text.Width(s, w.countFont())
	n.setRole(a11y.RoleStatus).setName(s)
	w.box.kids = append(w.box.kids, n)
	th := w.th
	n.paint = func(l *paint.List, r geom.Rect) {
		col := th.TextTertiary
		if th.Flavor == theme.Element {
			col = th.TextSecondary
		}
		if w.box.disabled {
			col = th.TextDisabled
		}
		lay := text.Shape(s, w.countFont(), text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.X, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	}
}

// paintFrameVariant draws the filled / borderless / underlined frames and
// custom backgrounds (paintFrame passes the status colors it resolved).
func (c *Context) paintFrameVariant(l *paint.List, r geom.Rect, rad geom.Radii, id ID, fs frameState, ft frameTok, bg, border, hoverC, focusC, outline theme.Color) {
	th := ft.th
	h := c.Anim(id, "fhover", b2f(fs.hover), th.DurationMid)
	f := c.Anim(id, "ffocus", b2f(fs.focus), th.DurationMid)
	switch fs.variant {
	case "borderless":
		if fs.bg != nil {
			l.Fill(r, rad, *fs.bg)
		}
		return
	case "underlined":
		if fs.bg != nil {
			l.Fill(r, geom.Radii{}, *fs.bg)
		}
		if fs.noBorder {
			return
		}
		col := theme.Lerp(theme.Lerp(border, hoverC, h), focusC, f)
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, col)
		return
	case "filled":
		// Ant filled: tinted fill, fill darkens on hover; focus shows the
		// container background with the focus border.
		fill, fillHover := th.FillTertiary, th.FillSecondary
		switch fs.status {
		case "error":
			fill, fillHover = th.Error.Bg, th.Error.BgHover
		case "warning":
			fill, fillHover = th.Warning.Bg, th.Warning.BgHover
		}
		if fs.bg != nil {
			fill, fillHover = *fs.bg, *fs.bg
		}
		base := theme.Lerp(fill, fillHover, h)
		if fs.noBorder {
			l.Fill(r, rad, base)
			return
		}
		l.Fill(r, rad, theme.Lerp(base, th.BgContainer, f))
		if f > 0 {
			l.Stroke(r, rad, 1, theme.Fade(focusC, float64(f)))
		} else if fs.status != "" {
			l.Stroke(r, rad, 1, border)
		}
		return
	}
	// Outlined with a custom background and / or no border.
	if fs.bg != nil {
		bg = *fs.bg
	}
	if fs.noBorder {
		l.Fill(r, rad, bg)
		return
	}
	col := theme.Lerp(theme.Lerp(border, hoverC, h), focusC, f)
	if f > 0 && outline.A > 0 && ft.outlineW > 0 {
		paintOutlineWith(l, ft.outlineW, ft.outlineOff, r, rad, theme.Fade(outline, float64(f)))
	}
	l.Fill(r, rad, bg)
	l.Stroke(r, rad, 1, col)
}
