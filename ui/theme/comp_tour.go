package theme

import "github.com/tangoui/tango/ui/paint"

// TourToken styles Tour.
type TourToken struct {
	Alias
	Width                       float32
	PanelPadding                float32
	Radius                      float32
	Shadow                      []paint.Shadow
	Bg                          Color
	TitleColor                  Color `contrast:"text:Bg"`
	DescriptionColor            Color `contrast:"text:Bg"` // description, close button, step counter
	TitleFontSize               float32
	TitleFontWeight             int     // zero: regular
	CounterFontSize             float32 // "1 / 3" step counter (Element)
	PrimaryBg                   Color   // panel of the primary type
	PrimaryColor                Color   `contrast:"text:PrimaryBg"`
	PrimaryDescriptionColor     Color   `contrast:"text:PrimaryBg"`
	IndicatorSize               float32 // step dots
	IndicatorGap                float32
	IndicatorColor              Color
	IndicatorActiveColor        Color `contrast:"nontext:Bg"`
	PrimaryIndicatorColor       Color
	PrimaryIndicatorActiveColor Color `contrast:"nontext:PrimaryBg"`
	CloseBtnSize                float32
	CloseIconSize               float32
	MaskColor                   Color
	HoleRadius                  float32 // corners of the hole around the target
}

func deriveTour(t *Theme) TourToken {
	k := TourToken{
		Width: 320, PanelPadding: 16, Radius: t.RadiusLG, Shadow: t.ShadowSecondary,
		Bg: t.BgElevated, TitleColor: t.Text, DescriptionColor: t.TextSecondary,
		TitleFontSize: t.FontSize, TitleFontWeight: t.FontWeightStrong, CounterFontSize: t.FontSizeSM,
		PrimaryBg: t.Primary.Base, PrimaryColor: t.TextOnPrimary, PrimaryDescriptionColor: RGBA(255, 255, 255, .85),
		IndicatorSize: 6, IndicatorGap: 6, IndicatorColor: t.FillSecondary, IndicatorActiveColor: t.Primary.Base,
		PrimaryIndicatorColor: RGBA(255, 255, 255, .15), PrimaryIndicatorActiveColor: Hex("#ffffff"),
		CloseBtnSize: 22, CloseIconSize: 14, MaskColor: t.BgMask, HoleRadius: t.Radius,
	}
	if t.Flavor == Element {
		// el-tour ($tour in theme-chalk var.scss): 520px wide, 12px
		// padding, 16px regular title, base radius.
		k.Width, k.PanelPadding = 520, 12
		k.TitleFontSize, k.TitleFontWeight, k.Radius = 16, 0, t.Radius
	}
	return k
}
