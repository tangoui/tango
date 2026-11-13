package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
)

// shadHeader is the login block CardHeader: the logo and the text-2xl
// font-semibold title on one centred line, the muted description under it.
func (w *LoginFormW) shadHeader() {
	c, th := w.c, w.c.Theme
	_, muted := shad2dMuted(th)
	Column(c, func() {
		if w.logo != nil || w.title != "" {
			Row(c, func() {
				if w.logo != nil {
					Row(c, w.logo, layout.Middle())
				}
				if w.title != "" {
					Text(c, w.title).Size(24).Weight(text.Semibold).Color(th.TextHeading).NoWrap()
				}
			}, layout.Gap(8), layout.Middle(), layout.Center())
		}
		if w.subTitle != "" {
			Text(c, w.subTitle).Color(muted).Align(AlignCenter)
		}
	}, layout.AlignItems(layout.AlignCenter), layout.Gap(8), layout.Margin(geom.Insets{Bottom: 24}))
}

// shadOthers is the "Or continue with" separator (a rule with the muted
// label in the middle) above the other login methods.
func (w *LoginFormW) shadOthers() {
	c, th := w.c, w.c.Theme
	loc := c.Locale()
	_, muted := shad2dMuted(th)
	Column(c, func() {
		Row(c, func() {
			Div(c, layout.H(1), layout.Grow(1)).Bg(th.BorderSecondary)
			Text(c, loc.LoginForm.Others).Size(th.FontSize).Color(muted).NoWrap()
			Div(c, layout.H(1), layout.Grow(1)).Bg(th.BorderSecondary)
		}, layout.Gap(8), layout.Middle())
		Row(c, w.actions, layout.Gap(12), layout.Middle(), layout.Center())
	}, layout.Gap(16), layout.Margin(geom.Insets{Top: 24})).Semantics(a11y.RoleGroup, loc.LoginForm.Others)
}
