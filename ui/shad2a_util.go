package ui

// Shared helpers of the shadcn/ui (new-york v4) look of the data-entry
// controls: Button, Checkbox, Radio, Switch, Input, Select, OTP, Rate …
//
// new-york v4 marks keyboard focus with "focus-visible:border-ring
// focus-visible:ring-[3px] focus-visible:ring-ring/50": a 3px halo of the
// ring color at half opacity right outside the border box, plus the border
// itself switching to the ring color. Invalid controls (aria-invalid) use a
// destructive border and a destructive/20 halo.

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// shad reports whether the active theme is the shadcn/ui flavor.
func (c *Context) shad() bool { return c.Theme.Flavor == theme.StyleShadcn }

// shadRingW is the width of the ring-[3px] focus halo.
const shadRingW = 3

// shadFocusRing paints the new-york focus indication around r: the 3px
// ring/50 halo (destructive/20 when invalid) and a 1px border in the ring
// color. The border uses th.FocusRing, which the theme lifts to 3:1
// against the page (WCAG 1.4.11) so the indicator stays visible even
// though the halo itself is faint.
func shadFocusRing(l *paint.List, th *theme.Theme, r geom.Rect, rad geom.Radii, invalid bool) {
	halo, border := th.ControlOutline, th.FocusRing
	if invalid {
		halo, border = th.ControlOutlineError, th.Error.Base
	}
	l.Stroke(r.Expand(shadRingW), rad.Grow(shadRingW), shadRingW, halo)
	l.Stroke(r, rad, 1, border)
}

// shadInvalidBorder paints the destructive border of an invalid control
// (aria-invalid:border-destructive) that is not focused.
func shadInvalidBorder(l *paint.List, th *theme.Theme, r geom.Rect, rad geom.Radii) {
	l.Stroke(r, rad, 1, th.Error.Base)
}

// shadIconSize is the lucide icon size inside shadcn controls (size-4).
const shadIconSize = 16

// shadButtonPadLG is the horizontal padding of a large shadcn button
// (new-york v4 lg: px-6; the theme's ButtonPaddingHLG keeps v3's px-8 for
// other readers).
const shadButtonPadLG = 24

// shadButtonIconPadCut is how much narrower the padding of a shadcn button
// with an icon is: has-[>svg]:px-3 (default), px-2.5 (sm), px-4 (lg).
func shadButtonIconPadCut(s ControlSize) float32 {
	switch s {
	case Small:
		return 2
	case Large:
		return 8
	}
	return 4
}

// paintHandlesShadcn draws the InputNumber steppers the shadcn way (no
// official counterpart): always visible, a border-colored divider, lucide
// chevrons in the muted foreground that turn foreground on an accent
// background under the pointer; disabled at a limit.
func (w *InputNumberW) paintHandlesShadcn(l *paint.List, r geom.Rect) {
	th, tk := w.th, w.tk // Input theme (frame radius), InputNumber token
	dis := w.box.disabled
	// The handles sit at the inline end: divider toward the value, rounded
	// hover corners on the outer side.
	s := w.c.inline(r)
	l.Fill(s.Rect(0, r.Y+1, 1, r.H-2), geom.Radii{}, tk.HandleBorderColor)
	l.Fill(s.Rect(1, r.Y+r.H/2, r.W-2, 1), geom.Radii{}, tk.HandleBorderColor)
	rad := controlRadiusOf(th, w.size)
	rtl := w.c.IsRTL()
	for i := range 2 {
		// The step nodes get their rectangles when the paint walk reaches
		// them (after this parent), so split the handles' own rect.
		hr := geom.Rect{X: r.X, Y: r.Y + float32(i)*r.H/2, W: r.W, H: r.H / 2}
		hov := !dis && ((i == 0 && w.upIA.Hovered) || (i == 1 && w.dnIA.Hovered))
		col := tk.HandleColor
		if hov && !w.atLimit(i == 0) {
			bg := s.Rect(1, hr.Y+1, hr.W-2, hr.H-1)
			var rr geom.Radii
			if i == 0 {
				bg.Y, bg.H = hr.Y+1, hr.H-1
				if rtl {
					rr.TL = rad - 1
				} else {
					rr.TR = rad - 1
				}
			} else {
				bg.Y, bg.H = hr.Y+1, hr.H-2
				if rtl {
					rr.BL = rad - 1
				} else {
					rr.BR = rad - 1
				}
			}
			if tk.HandleHoverBg.A > 0 {
				l.Fill(bg, rr, tk.HandleHoverBg)
			}
			col = tk.HandleHoverColor
		}
		if w.atLimit(i == 0) || dis {
			col = tk.HandleDisabledColor
		}
		ic := icon.Up
		if i == 1 {
			ic = icon.Down
		}
		is := tk.HandleFontSize
		drawIcon(l, ic, hr.X+(hr.W-is)/2, hr.Y+(hr.H-is)/2, is, col)
	}
}

// paintShadcn draws a CheckableTag as a shadcn Badge: checked is the
// default (primary, or the Type palette) badge, unchecked the outline
// badge whose background turns accent under the pointer; disabled fades
// to 50%.
func (w *CheckableTagW) paintShadcn(l *paint.List, r geom.Rect, font text.Style, padX, is float32) {
	th := w.c.Theme
	rad := geom.Rad(th.Radius)
	on := *w.val
	dis := w.isDisabled()
	if dis {
		l.Opacity(0.5)
		defer l.Pop()
	}
	pal := th.Primary
	if w.typ != "" && w.typ != "primary" {
		pal = th.ColorOf(w.typ)
	}
	hov := w.ia.Hovered && !dis
	var bg, fg theme.Color
	if on {
		bg, fg = pal.Base, th.TextOnPrimary
		if w.typ != "" && w.typ != "primary" {
			fg = th.TextOnSolid
		}
		if hov {
			bg = theme.Over(theme.Fade(pal.Base, .9), th.BgContainer)
		}
		l.Fill(r, rad, bg)
	} else {
		fg = th.Text
		if hov {
			l.Fill(r, rad, th.ItemHoverBg)
		}
		l.Stroke(r, rad, 1, th.BorderSecondary)
	}
	s := w.c.inline(r) // icon then label from the inline start
	off := padX
	if w.ic != nil {
		w.c.drawIconAt(l, w.ic, s.X(off, is), r.Y+(r.H-is)/2, is, fg)
		off += is + 4
	}
	if w.label != "" {
		lay := text.Shape(w.label, font, w.c.textOpts(text.Options{NoWrap: true}))
		l.Glyphs(geom.Pt(s.X(off, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
	}
	if w.ia.FocusVisible {
		shadFocusRing(l, th, r, rad, false)
	}
}
