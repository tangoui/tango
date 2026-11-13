package theme

import "github.com/tangoui/tango/ui/paint"

// PopoverToken styles Popover and Popconfirm.
type PopoverToken struct {
	Alias
	Bg                Color
	TitleColor        Color `contrast:"text:Bg"`
	Color             Color `contrast:"text:Bg"`
	TitleFontSize     float32
	TitleFontWeight   int // zero: regular
	TitleMarginBottom float32
	TitleMinWidth     float32
	InnerPadding      float32
	Radius            float32
	BorderColor       Color // zero: none
	Shadow            []paint.Shadow
}

func derivePopover(t *Theme) PopoverToken {
	k := PopoverToken{
		Bg: t.BgElevated, TitleColor: t.Text, Color: t.Text, TitleFontSize: t.FontSize, TitleFontWeight: t.FontWeightStrong,
		TitleMarginBottom: 8, TitleMinWidth: 177, InnerPadding: 12,
		Radius: t.RadiusLG, BorderColor: t.PopupBorder, Shadow: t.Shadow,
	}
	switch t.Flavor {
	case Element:
		k.TitleColor, k.TitleFontSize, k.TitleFontWeight, k.TitleMarginBottom = t.TextHeading, 16, 0, 12
		k.TitleMinWidth, k.Radius = 150, t.Radius
	case StyleShadcn:
		// p-4, title font-medium leading-none.
		k.TitleFontWeight, k.InnerPadding, k.Radius = 500, 16, t.Radius
	}
	return k
}
