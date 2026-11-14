package mobile

import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// drawSonner draws the shadcn toast: a popover card (border, rounded-lg,
// shadow-lg, p-4) as wide as the screen allows (at most Sonner's 356px),
// the icon inline before a medium-weight message. Bottom and top toasts
// slide in from their edge.
func drawSonner(c *ui.Context, th *Theme, id ui.ID, msg, kind string, pos Position, v float32) {
	scr := c.Size()
	safe := SafeAreaInsets(c)
	w := min(scr.W-32, 356)
	pad := th.ToastPad
	fs := th.Font(th.FontMain, text.Medium)
	fs.LineHeight = 20
	is := float32(0)
	if kind != "" {
		is = 16
	}
	textX := pad
	if is > 0 {
		textX += is + 8
	}
	lay := text.Shape(msg, fs, text.Options{MaxWidth: w - textX - pad})
	h := max(lay.Height, is) + 2*pad
	ang := 0.0
	if kind == "loading" {
		ang = spinAngle(c, id)
	}
	slide := (1 - v) * 16
	place, margin, shift := ui.ScreenCenter, float32(0), float32(0)
	switch pos {
	case PosTop:
		place, margin, shift = ui.ScreenTop, safe.Top+16, -slide
	case PosBottom:
		shift = (scr.H-h)/2 - 16 - safe.Bottom + slide
	}
	rad := geom.Rad(max(0, th.RadiusLarge-4))
	rtl := c.IsRTL()
	ov := ui.Overlay(c, ui.OverlayOptions{Place: place, Margin: margin, ShiftY: shift}, func() {
		detached(c, func() {
			b := ui.Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: w, H: h} }, nop)
			role := a11y.RoleStatus
			if kind == "fail" {
				role = a11y.RoleAlert
			}
			b.Semantics(role, msg).A11yState(stateIf(kind == "loading", a11y.Busy))
			b.Paint(func(l *paint.List, r geom.Rect) {
				l.Shadows(r, rad, th.Base.ShadowSecondary)
				l.Fill(r, rad, th.ToastBg)
				l.Stroke(r, rad, 1, th.ToastBorder)
				// The icon leads the message: at the right in RTL.
				sp := spanOf(r, rtl)
				lay := text.Shape(msg, fs, sp.opts(text.Options{MaxWidth: r.W - textX - pad}))
				if is > 0 {
					ir := sp.R(geom.Rect{X: r.X + pad, Y: r.Y + (r.H-is)/2, W: is, H: is})
					switch kind {
					case "success":
						icon.Draw(l, icon.CheckCircle, ir, th.Success, 0)
					case "fail":
						icon.Draw(l, icon.CloseCircle, ir, th.Danger, 0)
					case "loading":
						drawSpinner(l, ir.X+is/2, ir.Y+is/2, is/2-1, ang, th.TextWeak)
					}
				}
				l.Glyphs(geom.Pt(sp.X(r.X+textX, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.ToastText)
			})
		})
	})
	ov.Opacity(v)
}
