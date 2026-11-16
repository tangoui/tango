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
	HoleRadius                  float32 // corners of the hole around the target (gap.radius)
	// FitContent: Width is a maximum and the panel shrinks to its
	// content (antd max-width: fit-content); false: a fixed width.
	FitContent         bool
	HeaderMarginBottom float32 // title row to description
	FooterMarginTop    float32 // description to indicators / buttons
}

func deriveTour(t *Theme) TourToken {
	k := TourToken{
		// antd 6.7 es/tour/style/index.js: width 520 with max-width
		// fit-content, boxShadowTertiary, header padding-bottom paddingXS,
		// footer padding-top paddingXS, indicator background colorFill;
		// @rc-component/tour useTarget gap radius 2.
		Width: 520, FitContent: true, PanelPadding: 16, Radius: t.RadiusLG, Shadow: t.ShadowTertiary,
		HeaderMarginBottom: 8, FooterMarginTop: 8,
		Bg: t.BgElevated, TitleColor: t.Text, DescriptionColor: t.TextSecondary,
		TitleFontSize: t.FontSize, TitleFontWeight: t.FontWeightStrong, CounterFontSize: t.FontSizeSM,
		PrimaryBg: t.Primary.Base, PrimaryColor: t.TextOnPrimary, PrimaryDescriptionColor: RGBA(255, 255, 255, .85),
		IndicatorSize: 6, IndicatorGap: 6, IndicatorColor: t.Fill, IndicatorActiveColor: t.Primary.Base,
		PrimaryIndicatorColor: RGBA(255, 255, 255, .15), PrimaryIndicatorActiveColor: Hex("#ffffff"),
		CloseBtnSize: 22, CloseIconSize: 14, MaskColor: t.BgMask, HoleRadius: 2,
	}
	if t.Flavor == StyleShadcn {
		k.Shadow = t.ShadowSecondary // no shadcn tour: keep the popup shadow
	}
	if t.Flavor == Element {
		// el-tour ($tour in theme-chalk var.scss): 520px wide, 12px
		// padding, 16px regular title, base radius.
		// header padding-bottom / footer padding-top: padding-primary.
		k.Width, k.FitContent, k.PanelPadding = 520, false, 12
		k.HeaderMarginBottom, k.FooterMarginTop = 12, 12
		k.IndicatorColor = t.FillSecondary
		k.TitleFontSize, k.TitleFontWeight, k.Radius = 16, 0, t.Radius
	}
	return k
}
