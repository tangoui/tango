package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/theme"
)

// TestLinkElementDefaultRegular: Element's el-link defaults to type
// "default" — regular text at rest, color-primary on hover
// (element-plus theme-chalk/src/common/var.scss $link, 2026-11-14) — while
// Ant and shadcn links keep the link color, and other link-colored text
// (Typography.LinkColor) stays link-colored under Element.
func TestLinkElementDefaultRegular(t *testing.T) {
	for _, tc := range []struct {
		name               string
		th                 *theme.Theme
		rest, hover, press func(th *theme.Theme) theme.Color
	}{
		{"element", theme.ElementPlus(),
			func(th *theme.Theme) theme.Color { return th.TextSecondary },
			func(th *theme.Theme) theme.Color { return th.Primary.Base },
			func(th *theme.Theme) theme.Color { return th.Primary.Base }},
		{"antd", theme.AntDesign(),
			func(th *theme.Theme) theme.Color { return th.TextLink },
			func(th *theme.Theme) theme.Color { return th.TextLinkHover },
			func(th *theme.Theme) theme.Color { return th.TextLinkActive }},
		{"shadcn", theme.Shadcn(),
			func(th *theme.Theme) theme.Color { return th.TextLink },
			func(th *theme.Theme) theme.Color { return th.TextLinkHover },
			func(th *theme.Theme) theme.Color { return th.TextLinkActive }},
	} {
		var lw *LinkW
		NewDriver(func(c *Context) { lw = Link(c, "a") }, DriverOptions{Theme: tc.th}).Frame()
		tk := &tc.th.Components.Typography
		base, hover, press := lw.colors(tc.th, tk)
		if base != tc.rest(tc.th) || hover != tc.hover(tc.th) || press != tc.press(tc.th) {
			t.Errorf("%s: link colors %v / %v / %v", tc.name, base, hover, press)
		}
		if tk.LinkColor != tc.th.TextLink {
			t.Errorf("%s: Typography.LinkColor %v, want the link color %v", tc.name, tk.LinkColor, tc.th.TextLink)
		}
	}
}
