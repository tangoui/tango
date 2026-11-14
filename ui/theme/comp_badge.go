package theme

// BadgeToken styles Badge and Badge.Ribbon.
type BadgeToken struct {
	Alias
	IndicatorHeight          float32 // count bubble
	IndicatorHeightSM        float32
	IndicatorPaddingInline   float32 // count bubble side padding (multi-character)
	IndicatorPaddingInlineSM float32
	DotSize                  float32
	TextFontSize             float32
	TextFontSizeSM           float32
	TextFontWeight           int   // zero: regular
	CountBg                  Color // bubble of a badge without Color
	TextColor                Color `contrast:"text:CountBg"`
	RingColor                Color // 1px ring separating the bubble from the content
	StatusSize               float32
	StatusDefaultColor       Color // Status("default") dot
	StatusTextColor          Color `contrast:"text:@Page"`
	RibbonBg                 Color // ribbon without a color
	RibbonColor              Color `contrast:"text:RibbonBg"`
	RibbonHeight             float32
	RibbonRadius             float32
	RibbonFontSize           float32
}

func deriveBadge(t *Theme) BadgeToken {
	// antd 6.7 es/badge/style/index.js prepareComponentToken:
	// indicatorHeight round(fontSize × lineHeight) − 2 × lineWidth = 20,
	// indicatorHeightSM fontSize = 14, dotSize fontSizeSM / 2 = 6,
	// multiple-words paddingInline paddingXS = 8 (both sizes).
	k := BadgeToken{
		IndicatorHeight: 20, IndicatorHeightSM: 14, IndicatorPaddingInline: 8, IndicatorPaddingInlineSM: 8,
		DotSize: 6, TextFontSize: t.FontSizeSM, TextFontSizeSM: t.FontSizeSM,
		CountBg: t.Error.Base, TextColor: t.TextOnSolid, RingColor: t.BgContainer,
		StatusSize: 6, StatusDefaultColor: t.StatusDefault, StatusTextColor: t.Text,
		RibbonBg: t.Primary.Base, RibbonColor: Hex("#ffffff"), RibbonHeight: 22, RibbonRadius: t.RadiusSM, RibbonFontSize: t.FontSize,
	}
	switch t.Flavor {
	case Element:
		// theme-chalk var.scss $badge: size 18, padding 6; is-dot 8.
		k.IndicatorHeight, k.DotSize = 18, 8
		k.IndicatorPaddingInline, k.IndicatorPaddingInlineSM = 6, 4
	case StyleShadcn:
		// No shadcn counterpart for the count bubble: keep the earlier
		// 6 / 4 side padding.
		k.TextFontWeight = 600 // font-semibold
		k.IndicatorPaddingInline, k.IndicatorPaddingInlineSM = 6, 4
	}
	return k
}
