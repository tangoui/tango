package theme

import "github.com/tangoui/tango/ui/paint"

// TooltipToken styles Tooltip.
type TooltipToken struct {
	Alias
	Bg              Color
	Color           Color `contrast:"text:Bg"`
	ContentFontSize float32
	MaxWidth        float32
	PaddingBlock    float32
	PaddingInline   float32
	MinHeight       float32
	Radius          float32
	Shadow          []paint.Shadow
}

func deriveTooltip(t *Theme) TooltipToken {
	k := TooltipToken{
		Bg: t.BgSpotlight, Color: t.TextOnSpotlight, ContentFontSize: t.FontSize, MaxWidth: 250,
		PaddingBlock: 6, PaddingInline: 8, MinHeight: 32, Radius: t.Radius, Shadow: t.Shadow,
	}
	switch t.Flavor {
	case Element:
		k.PaddingBlock, k.PaddingInline, k.MinHeight = 5, 11, 0
	case StyleShadcn:
		// px-3 py-1.5, text-xs, no shadow.
		k.ContentFontSize, k.PaddingBlock, k.PaddingInline, k.MinHeight, k.Shadow = t.FontSizeSM, 6, 12, 0, t.TooltipShadow
	}
	return k
}
