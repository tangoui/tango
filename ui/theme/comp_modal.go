package theme

import "github.com/tangoui/tango/ui/paint"

// ModalToken styles Modal and the Confirm / MessageBox dialogs.
type ModalToken struct {
	Alias
	ContentBg          Color
	HeaderBg           Color // zero: none
	FooterBg           Color // zero: none
	TitleColor         Color `contrast:"text:ContentBg"`
	TitleFontSize      float32
	TitleFontWeight    int     // zero: regular
	TitleLineHeight    float32 // zero: the theme line height of TitleFontSize
	Padding            float32 // inline padding of the panel
	PaddingBlock       float32
	HeaderMarginBottom float32
	FooterMarginTop    float32
	CloseIconColor     Color // zero: the icon button default
	Radius             float32
	BorderColor        Color // zero: none
	MaskBg             Color
	Shadow             []paint.Shadow
}

func deriveModal(t *Theme) ModalToken {
	k := ModalToken{
		ContentBg: t.BgElevated, TitleColor: t.Text, TitleFontSize: t.FontSizeLG, TitleFontWeight: t.FontWeightStrong,
		Padding: 24, PaddingBlock: 20, HeaderMarginBottom: 8, FooterMarginTop: 12,
		Radius: t.RadiusLG, BorderColor: t.ModalBorder, MaskBg: t.BgMask, Shadow: t.ShadowSecondary,
	}
	switch t.Flavor {
	case Element:
		k.TitleFontSize, k.TitleFontWeight, k.TitleLineHeight = 18, 0, 24
		k.Padding, k.PaddingBlock, k.HeaderMarginBottom, k.FooterMarginTop = 16, 16, 16, 16
		k.Radius = t.Radius
	case StyleShadcn:
		// DialogContent p-6, DialogTitle text-lg font-semibold.
		k.TitleFontSize, k.TitleFontWeight = 18, 600
		k.Padding, k.PaddingBlock, k.HeaderMarginBottom, k.FooterMarginTop = 24, 24, 16, 16
		k.CloseIconColor = Fade(t.Text, .7)
	}
	return k
}
