package theme

import "github.com/tangoui/tango/ui/paint"

// CardToken styles Card.
type CardToken struct {
	Alias
	Bg                    Color
	HeaderBg              Color // zero: none
	InnerHeaderBg         Color // header band of an inner card (zero: none)
	HeaderColor           Color `contrast:"text:Bg"`
	HeaderFontSize        float32
	HeaderFontSizeSM      float32
	HeaderFontWeight      int     // zero: regular
	HeaderHeight          float32 // minimum header height (zero: none)
	HeaderHeightSM        float32
	HeaderPaddingBlock    float32
	HeaderPaddingBlockSM  float32
	HeaderPaddingInline   float32
	HeaderPaddingInlineSM float32
	BodyPadding           float32
	BodyPaddingSM         float32
	ActionsBg             Color
	BorderColor           Color // outline and dividers
	Radius                float32
	Shadow                []paint.Shadow // "always" shadow
	HoverShadow           []paint.Shadow // hoverable card under the pointer
}

func deriveCard(t *Theme) CardToken {
	k := CardToken{
		Bg: t.BgContainer, InnerHeaderBg: t.FillAlter, HeaderColor: t.Text,
		HeaderFontSize: t.FontSizeLG, HeaderFontSizeSM: t.FontSize, HeaderFontWeight: t.FontWeightStrong,
		HeaderHeight: 56, HeaderHeightSM: 38, HeaderPaddingInline: 24, HeaderPaddingInlineSM: 12,
		BodyPadding: 24, BodyPaddingSM: 12,
		ActionsBg: t.BgContainer, BorderColor: t.BorderSecondary, Radius: t.CardRadius,
		Shadow: t.ShadowTertiary, HoverShadow: t.Shadow,
	}
	if t.CardBorder.A > 0 {
		k.BorderColor = t.CardBorder
	}
	switch t.Flavor {
	case Element:
		k.HeaderFontWeight = 0
		k.HeaderHeight, k.HeaderHeightSM = 0, 0
		k.HeaderPaddingBlock, k.HeaderPaddingBlockSM, k.HeaderPaddingInline = 18, 10, 20
		k.BodyPadding = 20
		k.HoverShadow = []paint.Shadow{{Y: 0, Blur: 12, Color: RGBA(0, 0, 0, .12)}}
		if k.Radius == 0 {
			k.Radius = t.Radius
		}
	case StyleShadcn:
		// CardHeader p-6 (p-4 small) with a semibold title, no divider.
		k.HeaderHeight, k.HeaderHeightSM = 0, 0
		k.HeaderPaddingBlock, k.HeaderPaddingBlockSM = 24, 16
		k.HeaderPaddingInlineSM, k.BodyPaddingSM = 16, 16
	}
	if len(t.CardShadow) > 0 {
		k.Shadow = t.CardShadow
	}
	if k.Radius == 0 {
		k.Radius = t.RadiusLG
	}
	return k
}
