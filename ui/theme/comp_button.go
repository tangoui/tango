package theme

import "github.com/tangoui/tango/ui/paint"

// ButtonToken styles Button.
type ButtonToken struct {
	Alias
	FontWeight      int     // label weight
	PaddingInline   float32 // horizontal padding, middle size
	PaddingInlineSM float32
	PaddingInlineLG float32
	ContentFontSize float32 // label size, middle (small and large use the Alias FontSizeSM / FontSizeLG)

	DefaultBg               Color // default / outline button fill
	DefaultColor            Color `contrast:"text:DefaultBg"`
	DefaultBorderColor      Color
	DefaultHoverBg          Color
	DefaultHoverColor       Color `contrast:"text:DefaultHoverBg"`
	DefaultHoverBorderColor Color

	SecondaryBg      Color // "secondary" variant fill
	SecondaryColor   Color `contrast:"text:SecondaryBg"`
	SecondaryHoverBg Color

	PrimaryColor Color `contrast:"text:@Primary"` // label on the primary fill
	DangerBg     Color // solid danger fill
	DangerColor  Color `contrast:"text:DangerBg"` // label on the solid danger fill

	TextHoverBg Color // text (ghost) button under the pointer
	LinkHoverBg Color // link button under the pointer (zero: none)

	DefaultShadow []paint.Shadow
	PrimaryShadow []paint.Shadow
	DangerShadow  []paint.Shadow
}

func deriveButton(t *Theme) ButtonToken {
	k := ButtonToken{
		FontWeight:    t.ButtonFontWeight,
		PaddingInline: t.ButtonPaddingH, PaddingInlineSM: t.ButtonPaddingHSM, PaddingInlineLG: t.ButtonPaddingHLG,
		ContentFontSize:    t.FontSize,
		DefaultBg:          t.BgContainer,
		DefaultColor:       t.Text,
		DefaultBorderColor: t.Border,
		DefaultHoverBg:     t.BgContainer,
		SecondaryBg:        t.SecondaryBg, SecondaryColor: t.SecondaryText, SecondaryHoverBg: t.SecondaryBgHover,
		PrimaryColor: t.TextOnPrimary,
		DangerBg:     t.ButtonDangerBg, DangerColor: t.TextOnSolid,
		TextHoverBg:   t.BgTextHover,
		DefaultShadow: t.ButtonShadow, PrimaryShadow: t.PrimaryShadow, DangerShadow: t.DangerShadow,
	}
	switch t.Flavor {
	case Element:
		// el-button: secondary-text label, hover tints with the brand.
		k.DefaultColor = t.TextSecondary
		k.DefaultHoverBg, k.DefaultHoverColor, k.DefaultHoverBorderColor = t.Primary.Bg, t.Primary.Base, t.Primary.Shades[2]
		k.DefaultShadow, k.PrimaryShadow, k.DangerShadow = nil, nil, nil
	case StyleShadcn:
		// outline: input background, accent on hover; ghost uses accent.
		k.DefaultBg, k.DefaultHoverBg = t.ControlBg, t.ControlBgHover
		k.PaddingInlineLG = 24 // new-york v4 lg: px-6
		k.DefaultHoverColor, k.DefaultHoverBorderColor = t.Text, t.Border
		k.TextHoverBg = t.ItemHoverBg
	default:
		k.DefaultHoverColor, k.DefaultHoverBorderColor = t.Primary.Hover, t.Primary.Hover
	}
	return k
}
