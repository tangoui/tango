package theme

import "github.com/tangoui/tango/ui/paint"

// NotificationToken styles Notification (shadcn: Sonner toasts).
type NotificationToken struct {
	Alias
	Bg                 Color
	TitleColor         Color `contrast:"text:Bg"`
	DescriptionColor   Color `contrast:"text:Bg"`
	TitleFontSize      float32
	TitleFontWeight    int     // zero: regular
	TitleLineHeight    float32 // zero: the theme line height of TitleFontSize
	Width              float32
	Padding            float32 // inline padding (end side)
	PaddingInlineStart float32
	PaddingBlock       float32
	Radius             float32
	BorderColor        Color // zero: none
	Shadow             []paint.Shadow
}

func deriveNotification(t *Theme) NotificationToken {
	k := NotificationToken{
		Bg: t.BgElevated, TitleColor: t.Text, DescriptionColor: t.Text, TitleFontSize: t.FontSizeLG, TitleLineHeight: 24,
		Width: 384, Padding: 24, PaddingInlineStart: 24, PaddingBlock: 20,
		Radius: t.RadiusLG, Shadow: t.ShadowSecondary,
	}
	switch t.Flavor {
	case Element:
		k.DescriptionColor, k.TitleFontWeight = t.TextSecondary, 700
		k.Width, k.Padding, k.PaddingInlineStart, k.PaddingBlock = 330, 15, 13, 14
		k.BorderColor, k.Shadow = t.BorderSecondary, t.Shadow
	case StyleShadcn:
		// Sonner: 356px cards, p-4, font-medium title, muted description.
		k.DescriptionColor, k.TitleFontSize, k.TitleFontWeight, k.TitleLineHeight = t.TextTertiary, t.FontSize, 500, 0
		k.Width, k.Padding, k.PaddingInlineStart, k.PaddingBlock = 356, 16, 16, 16
		k.BorderColor = t.PopupBorder
	}
	return k
}
