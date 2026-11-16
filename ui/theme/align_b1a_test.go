package theme

// Spec alignment batch 1, group A: component token values checked against
// the reference sources (/tmp/tango-ref, 2026-11-14; see
// docs/specs/align/1-basic.md).

import "testing"

func TestAlignB1ADividerTokens(t *testing.T) {
	ant := AntDesign().Components.Divider
	// antd es/divider/style/index.js: with-text fontSize fontSizeLG (16),
	// margin `margin` (16); rule marginLG (24); -sm marginXS (8), -md
	// margin (16); vertical height 0.9em, marginInline marginXS (8);
	// textPaddingInline 1em, orientationMargin 0.05.
	if ant.TextFontSize != 16 || ant.TextFontWeight != 500 || ant.TextMarginBlock != 16 || ant.MarginBlock != 24 ||
		ant.MarginBlockSM != 8 || ant.MarginBlockMD != 16 || ant.VerticalMarginInline != 8 || ant.OrientationMargin != 0.05 {
		t.Errorf("ant divider %+v", ant)
	}
	if h := ant.VerticalHeight; h < 12.59 || h > 12.61 {
		t.Errorf("ant vertical height %v, want 0.9em = 12.6", h)
	}
	// ep theme-chalk/src/divider.scss: text 14px / 500, padding 0 20px,
	// margin 24px 0; vertical 1em high, margin 0 8px.
	el := ElementPlus().Components.Divider
	if el.TextFontSize != 14 || el.TextPaddingInline != 20 || el.TextMarginBlock != 24 || el.VerticalHeight != 14 || el.VerticalMarginInline != 8 {
		t.Errorf("element divider %+v", el)
	}
	// shadcn new-york-v4/ui/separator.tsx: vertical h-full.
	if sh := Shadcn().Components.Divider; sh.VerticalHeight != 0 {
		t.Errorf("shadcn vertical height %v, want 0 (h-full)", sh.VerticalHeight)
	}
}

func TestAlignB1ARateTokens(t *testing.T) {
	// antd es/rate/style/index.js: starSize controlHeight*0.625 (20),
	// SM controlHeightSM*0.625 (15), LG controlHeightLG*0.625 (25).
	ant := AntDesign().Components.Rate
	if ant.StarSize != 20 || ant.StarSizeSM != 15 || ant.StarSizeLG != 25 || ant.StarGap != 8 {
		t.Errorf("ant rate %+v", ant)
	}
	// ep theme-chalk/src/rate.scss: height large 40 / default 32 / small
	// 24, icon 18, small 14, icon margin 6.
	el := ElementPlus().Components.Rate
	if el.StarSize != 18 || el.StarSizeSM != 14 || el.StarSizeLG != 18 || el.Height != 32 || el.HeightSM != 24 || el.HeightLG != 40 || el.StarGap != 6 {
		t.Errorf("element rate %+v", el)
	}
}

func TestAlignB1AButtonTokens(t *testing.T) {
	// antd ConfigProvider button.autoInsertSpace defaults to true; ep
	// button auto-insert-space defaults to false; shadcn has none.
	if !AntDesign().Components.Button.AutoInsertSpace || ElementPlus().Components.Button.AutoInsertSpace || Shadcn().Components.Button.AutoInsertSpace {
		t.Error("AutoInsertSpace defaults: want Ant on, Element and shadcn off")
	}
}

func TestAlignB1ATypographyTokens(t *testing.T) {
	// antd typography: links not decorated (linkDecoration none).
	ant := AntDesign().Components.Typography
	if ant.LinkFontWeight != 400 || ant.LinkUnderline != 0 || ant.LinkHoverUnderline != 0 {
		t.Errorf("ant link weight %v underline %v/%v", ant.LinkFontWeight, ant.LinkUnderline, ant.LinkHoverUnderline)
	}
	// ep theme-chalk common/var.scss $link font-weight font-weight-primary
	// (500); link.vue underline defaults to "hover".
	el := ElementPlus().Components.Typography
	if el.LinkFontWeight != 500 || el.LinkUnderline != 0 || el.LinkHoverUnderline == 0 {
		t.Errorf("element link weight %v underline %v/%v", el.LinkFontWeight, el.LinkUnderline, el.LinkHoverUnderline)
	}
	// shadcn typography link: font-medium underline.
	sh := Shadcn().Components.Typography
	if sh.LinkFontWeight != 500 || sh.LinkUnderline == 0 || sh.LinkHoverUnderline == 0 {
		t.Errorf("shadcn link weight %v underline %v/%v", sh.LinkFontWeight, sh.LinkUnderline, sh.LinkHoverUnderline)
	}
}

func TestAlignB1ALayoutTokens(t *testing.T) {
	// The dark Ant sider is Tango's dark palette, not #001529: its trigger
	// keeps following SiderSubmenuBg.
	if d := AntDesign(Dark()); d.Components.Layout.TriggerBg != d.SiderSubmenuBg {
		t.Errorf("dark trigger %v, want SiderSubmenuBg %v", d.Components.Layout.TriggerBg, d.SiderSubmenuBg)
	}
	// antd es/layout/style/index.js: headerHeight controlHeight*2 (64),
	// headerPadding 0 controlHeightLG*1.25 (50), footerPadding
	// controlHeightSM (24) 50, siderBg / headerBg #001529, triggerBg
	// #002140, triggerHeight controlHeightLG + marginXXS*2 (48),
	// zeroTrigger controlHeightLG (40).
	k := AntDesign().Components.Layout
	if k.HeaderHeight != 64 || k.HeaderPaddingInline != 50 || k.FooterPadding != 24 || k.FooterPaddingInline != 50 ||
		k.SiderBg != Hex("#001529") || k.HeaderBg != Hex("#001529") || k.TriggerBg != Hex("#002140") || k.TriggerHeight != 48 ||
		k.ZeroTriggerWidth != 40 || k.ZeroTriggerHeight != 40 {
		t.Errorf("ant layout %+v", k)
	}
}
