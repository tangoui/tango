package theme

import "github.com/tangoui/tango/ui/paint"

// MessageToken styles Message (shadcn: Sonner toasts).
type MessageToken struct {
	Alias
	ContentBg     Color // zero: the status tint (Element)
	Color         Color `contrast:"text:ContentBg"` // zero: the status color (Element)
	PaddingBlock  float32
	PaddingInline float32
	Width         float32 // zero: fit the content
	Radius        float32
	BorderColor   Color // zero: none (Element: the status border)
	Shadow        []paint.Shadow
}

func deriveMessage(t *Theme) MessageToken {
	k := MessageToken{
		ContentBg: t.BgElevated, Color: t.Text, PaddingBlock: 9, PaddingInline: 12,
		Radius: t.RadiusLG, Shadow: t.Shadow,
	}
	switch t.Flavor {
	case Element:
		k.ContentBg, k.Color, k.Shadow = Color{}, Color{}, nil
		k.PaddingBlock, k.PaddingInline, k.Radius = 11, 15, t.Radius
	case StyleShadcn:
		// Sonner: 356px cards, p-4, border, shadow-lg.
		k.PaddingBlock, k.PaddingInline, k.Width = 16, 16, 356
		k.BorderColor, k.Shadow = t.PopupBorder, t.ShadowSecondary
	}
	return k
}
