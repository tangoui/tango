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
	InnerBodyPaddingBlock float32 // body block padding of an inner card (inline stays BodyPadding)
	ActionsBg             Color
	BorderColor           Color // outline and dividers
	Radius                float32
	Shadow                []paint.Shadow // "always" shadow (also a borderless Ant card)
	HoverShadow           []paint.Shadow // hoverable card under the pointer
	HoverBorderColor      Color          // outline of a hoverable card under the pointer (zero: hidden)
	// ShadowAlways is the default shadow mode: true "always" (Element
	// el-card shadow="always", shadcn shadow-sm), false "never" (Ant,
	// where only a borderless card keeps Shadow).
	ShadowAlways bool
	// Pro v3 Card/style.js uses independent header/body padding; small
	// header uses paddingXS (8), body paddingSM (12), not antd Card padding.
	ProHeaderPaddingBlock, ProHeaderPaddingInline     float32
	ProHeaderPaddingBlockSM, ProHeaderPaddingInlineSM float32
	ProBodyPaddingBlock, ProBodyPaddingBlockSM        float32
	ProHoverShadow                                    []paint.Shadow
	ProHoverBorderColor                               Color // zero: transparent at full hover
}

func deriveCard(t *Theme) CardToken {
	k := CardToken{
		Bg: t.BgContainer, InnerHeaderBg: t.FillAlter, HeaderColor: t.Text,
		HeaderFontSize: t.FontSizeLG, HeaderFontSizeSM: t.FontSize, HeaderFontWeight: t.FontWeightStrong,
		HeaderHeight: 56, HeaderHeightSM: 38, HeaderPaddingInline: 24, HeaderPaddingInlineSM: 12,
		BodyPadding: 24, BodyPaddingSM: 12, InnerBodyPaddingBlock: 16,
		ActionsBg: t.BgContainer, BorderColor: t.BorderSecondary, Radius: t.CardRadius,
		Shadow: t.ShadowTertiary, HoverShadow: antCardShadow(t),
		ProHeaderPaddingBlock: 16, ProHeaderPaddingInline: 24,
		ProHeaderPaddingBlockSM: 8, ProHeaderPaddingInlineSM: 12,
		ProBodyPaddingBlock: 16, ProBodyPaddingBlockSM: 12,
		ProHoverShadow: t.ShadowTertiary,
	}
	if t.CardBorder.A > 0 {
		k.BorderColor = t.CardBorder
	}
	switch t.Flavor {
	case Element:
		k.ShadowAlways = true
		k.HeaderFontWeight = 0
		k.HeaderHeight, k.HeaderHeightSM = 0, 0
		k.HeaderPaddingBlock, k.HeaderPaddingBlockSM, k.HeaderPaddingInline = 18, 10, 20
		k.BodyPadding = 20
		k.ProHeaderPaddingBlock, k.ProHeaderPaddingInline, k.ProBodyPaddingBlock = 18, 20, 20
		k.ProHoverShadow, k.ProHoverBorderColor = t.Shadow, k.BorderColor
		k.HoverShadow = []paint.Shadow{{Y: 0, Blur: 12, Color: RGBA(0, 0, 0, .12)}}
		if k.Radius == 0 {
			k.Radius = t.Radius
		}
	case StyleShadcn:
		k.ShadowAlways, k.HoverShadow = true, t.Shadow
		k.ProHeaderPaddingBlock, k.ProBodyPaddingBlock = 24, 24
		k.ProHoverShadow, k.ProHoverBorderColor = t.Shadow, k.BorderColor
		// CardHeader p-6 (p-4 small) with a semibold title, no divider.
		k.HeaderHeight, k.HeaderHeightSM = 0, 0
		k.HeaderPaddingBlock, k.HeaderPaddingBlockSM = 24, 16
		k.HeaderPaddingInlineSM, k.BodyPaddingSM = 16, 16
	}
	if t.Flavor == Element || t.Flavor == StyleShadcn {
		k.HoverBorderColor = k.BorderColor      // el-card keeps its border under the hover shadow
		k.InnerBodyPaddingBlock = k.BodyPadding // inner cards are antd only
	}
	if len(t.CardShadow) > 0 {
		k.Shadow = t.CardShadow
	}
	if k.Radius == 0 {
		k.Radius = t.RadiusLG
	}
	return k
}

// antCardShadow is antd's boxShadowCard (theme/util/alias.js): three
// layers of colorShadow (#000 light, rgba(255,255,255,.2) dark) at
// alpha .16 / .12 / .09.
func antCardShadow(t *Theme) []paint.Shadow {
	base, a := RGBA(0, 0, 0, 1), 1.0
	if t.Dark {
		base, a = RGBA(255, 255, 255, 1), .2
	}
	at := func(x float64) Color { return Fade(base, a*x) }
	return []paint.Shadow{
		{Y: 1, Blur: 2, Spread: -2, Color: at(.16)},
		{Y: 3, Blur: 6, Color: at(.12)},
		{Y: 5, Blur: 12, Spread: 4, Color: at(.09)},
	}
}
