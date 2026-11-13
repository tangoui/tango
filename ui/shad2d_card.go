package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// shadHeader is the shadcn CardHeader of a ProCard: a column with the
// font-semibold title (tooltip icon after it) and the muted description
// (SubTitle) under it, and the CardAction (Extra) in its own column at
// the top right.
func (w *ProCardW) shadHeader(hd *Box, folded bool) {
	c, th := w.c, w.c.Theme
	_, muted := shad2dMuted(th)
	fs := th.FontSizeLG
	if w.small {
		fs = th.FontSize
	}
	Column(c, func() {
		Row(c, func() {
			if w.collapsible {
				rot := c.Anim(hd.n.id, "rot", map[bool]float32{true: 0, false: 90}[folded], th.DurationMid)
				Icon(c, icon.Right).Size(14).Color(muted).Rotate(float64(rot))
			}
			if w.title != "" {
				Text(c, w.title).Size(fs).LineHeight(fs).Weight(text.Semibold).NoWrap().Ellipsis().Layout(layout.MinW(0))
			}
			if w.tip != "" {
				Tooltip(c, w.tip, func() {
					Icon(c, icon.Question).Size(14).Color(muted)
				}).Semantics(a11y.RoleImage, c.Locale().ProCard.MoreInfo)
			}
		}, layout.Gap(6), layout.Middle(), layout.MinW(0))
		if w.sub != "" {
			Text(c, w.sub).Size(th.FontSize).Color(muted).Ellipsis().Layout(layout.MinW(0))
		}
	}, layout.Gap(6), layout.Grow(1), layout.MinW(0))
	if w.extra != nil {
		// With a description the action sits at the top right (grid row
		// span); a single-line header centres it on the title.
		Row(c, w.extra, layout.Gap(8), layout.Middle())
	}
}

// shad2dCheckIndicator paints the 16px checkbox (or radio) of a shadcn
// choice card at the top end corner of r (top left when rtl); chk animates
// the checked state.
func shad2dCheckIndicator(l *paint.List, th *theme.Theme, r geom.Rect, chk float32, radio, disabled, rtl bool) {
	pr, prFg := shad2dPrimary(th)
	if disabled {
		pr = th.TextDisabled
	}
	box := geom.Rect{X: r.Right() - 16 - 14, Y: r.Y + 16, W: 16, H: 16}
	if rtl {
		box = mirrorRect(box, r)
	}
	rad := geom.Rad(th.CheckboxRadius)
	if radio {
		rad = geom.Rad(8)
	}
	l.Fill(box, rad, th.BgContainer)
	l.Stroke(box, rad, 1, theme.Lerp(th.Border, pr, chk))
	if chk <= 0 {
		return
	}
	if radio {
		cx, cy := box.X+8, box.Y+8
		l.Path(paint.NewPath().Circle(cx, cy, 4*chk), pr)
		return
	}
	l.Fill(box, rad, theme.Fade(pr, float64(chk)))
	shad2dIconCheck(l, box.Inset(geom.All(1.5)), theme.Fade(prFg, float64(chk)))
}
