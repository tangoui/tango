package theme

import "testing"

func TestAlignB4ProTokens(t *testing.T) {
	for _, th := range []*Theme{AntDesign(), AntDesign(Dark()), ElementPlus(), ElementPlus(Dark()), Shadcn(), Shadcn(Dark())} {
		// Pro v3 card/components/Card/style.js: small body paddingBlock paddingSM,
		// header paddingBlock paddingXS; LoginForm/style.js: logo 16, weight 600,
		// description marginBlockEnd 40 (2026-11-15).
		if th.Components.Card.ProHeaderPaddingBlockSM != 8 || th.Components.Card.ProBodyPaddingBlockSM != 12 {
			t.Fatal("ProCard small spacing")
		}
		if k := th.Components.Form; k.LoginLogoGap != 16 || k.LoginTitleFontWeight != 600 || k.LoginDescriptionMarginBottom != 40 {
			t.Fatal("LoginForm tokens")
		}
	}
	// ProLayout.js defaults and provider/typing/layoutToken.js.
	for _, th := range []*Theme{AntDesign(), AntDesign(Dark())} {
		k := th.Components.Layout
		if k.ProSiderWidth != 256 || k.ProMixSiderWidth != 215 || k.ProHeaderHeight != 56 || k.PagePaddingInline != 40 || k.PagePaddingBlock != 32 || k.ProFixedWidth != 1152 {
			t.Fatal("Pro layout defaults diverged from v3")
		}
		if k.DefaultFooterLinkGap != 8 || k.DefaultFooterMarginTop != 48 || k.DefaultFooterMarginBottom != 24 {
			t.Fatal("DefaultFooter spacing")
		}
		if th.Components.FooterToolbar.MinHeight != 64 {
			t.Fatal("FooterToolbar height")
		}
		if th.Components.Layout.SiderWidth != th.SiderWidth || th.Components.Layout.HeaderHeight != 64 {
			t.Fatal("basic Layout must remain unchanged")
		}
	}
}
