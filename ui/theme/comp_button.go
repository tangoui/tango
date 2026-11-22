package theme

import "github.com/tangoui/tango/ui/paint"

// ButtonToken styles Button.
type ButtonToken struct {
	Alias
	FontWeight                           int     // label weight
	PaddingInline                        float32 // horizontal padding, middle size
	PaddingInlineSM                      float32
	PaddingInlineLG                      float32
	ContentFontSize                      float32 // label size, middle (small and large use the Alias FontSizeSM / FontSizeLG)
	ContentFontSizeSM, ContentFontSizeLG float32 // nonzero overrides for those sizes
	RadiusSM, RadiusLG                   float32 // positive overrides for those sizes

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

	// Color "default" with the solid appearance (Ant color="default"
	// variant="solid"): a dark fill in light themes, a light one in dark.
	DefaultSolidBg       Color
	DefaultSolidHoverBg  Color
	DefaultSolidActiveBg Color
	DefaultSolidColor    Color `contrast:"text:DefaultSolidBg"`

	// PresetTextContrast is the minimum contrast of a label drawn in a
	// preset color (yellow, lime, gold …) against what is behind it; paler
	// labels are deepened towards the text color (0: keep the preset).
	PresetTextContrast float32
	// GroupDividerOpacity is the opacity of the label color for the seam
	// between two solid buttons of a ButtonGroup.
	GroupDividerOpacity float32
	// AutoInsertSpace is the default of Button.AutoInsertSpace: a space
	// between the two characters of a two-Chinese-character label (Ant
	// ConfigProvider button.autoInsertSpace defaults to true; Element
	// auto-insert-space to false; shadcn has none). Being a bool, false
	// cannot be set through Components overrides (a zero field inherits):
	// turn it off for a scope with ConfigProvider
	// Defaults("Button", Props{"autoInsertSpace": false}), or per button
	// with ButtonW.AutoInsertSpace(false). Precedence: the method, then
	// Defaults, then this token.
	AutoInsertSpace bool
}

// ButtonPalette derives the interaction palette (hover, active, tints) of
// an arbitrary button color with t's design-system algorithm, as Button
// does for the preset and hex colors of Button.Color.
func (t *Theme) ButtonPalette(c Color) Palette { return t.flavorPalette(c) }

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
		PresetTextContrast: 3, GroupDividerOpacity: 0.3,
		AutoInsertSpace: t.Flavor == Ant,
	}
	solid := Over(t.Text, t.BgContainer)
	k.DefaultSolidBg, k.DefaultSolidColor = solid, t.BgContainer
	k.DefaultSolidHoverBg, k.DefaultSolidActiveBg = Mix(t.BgContainer, solid, 0.75), Mix(t.BgContainer, solid, 0.95)
	switch t.Flavor {
	case Element:
		// el-button: secondary-text label, hover tints with the brand.
		k.DefaultColor = t.TextSecondary
		k.DefaultHoverBg, k.DefaultHoverColor, k.DefaultHoverBorderColor = t.Primary.Bg, t.Primary.Base, t.Primary.Shades[2]
		k.DefaultShadow, k.PrimaryShadow, k.DangerShadow = nil, nil, nil
		k.GroupDividerOpacity = 0.5 // --el-button-divide-border-color
		if t.elementControls {
			// common/var.scss $button-font-size and $button-border-radius.
			k.ContentFontSizeSM, k.ContentFontSizeLG = 12, t.FontSize
			k.RadiusSM, k.RadiusLG = max(0, t.Radius-1), t.Radius
			k.PaddingInlineSM, k.PaddingInlineLG = 11, 19 // excludes the 1px border
		}
	case StyleShadcn:
		// outline: input background, accent on hover; ghost uses accent.
		k.DefaultBg, k.DefaultHoverBg = t.ControlBg, t.ControlBgHover
		k.PaddingInlineLG = 24 // new-york v4 lg: px-6
		k.DefaultHoverColor, k.DefaultHoverBorderColor = t.Text, t.Border
		k.TextHoverBg = t.ItemHoverBg
		k.GroupDividerOpacity = 0.2
	default:
		k.DefaultHoverColor, k.DefaultHoverBorderColor = t.Primary.Hover, t.Primary.Hover
	}
	return k
}
