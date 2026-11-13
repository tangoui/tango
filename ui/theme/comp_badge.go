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
	k := BadgeToken{
		IndicatorHeight: 20, IndicatorHeightSM: 14, IndicatorPaddingInline: 6, IndicatorPaddingInlineSM: 4,
		DotSize: 6, TextFontSize: t.FontSizeSM, TextFontSizeSM: t.FontSizeSM,
		CountBg: t.Error.Base, TextColor: t.TextOnSolid, RingColor: t.BgContainer,
		StatusSize: 6, StatusDefaultColor: t.StatusDefault, StatusTextColor: t.Text,
		RibbonBg: t.Primary.Base, RibbonColor: Hex("#ffffff"), RibbonHeight: 22, RibbonRadius: t.RadiusSM, RibbonFontSize: t.FontSize,
	}
	switch t.Flavor {
	case Element:
		k.IndicatorHeight, k.DotSize = 18, 8
	case StyleShadcn:
		k.TextFontWeight = 600 // font-semibold
	}
	return k
}
