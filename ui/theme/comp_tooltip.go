package theme

import (
	"time"

	"github.com/tangoui/tango/ui/paint"
)

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
	Offset          float32       // bubble distance from the trigger with the arrow
	MouseEnterDelay time.Duration // default hover delay before showing
	MouseLeaveDelay time.Duration // default delay before hiding
}

func deriveTooltip(t *Theme) TooltipToken {
	k := TooltipToken{
		Bg: t.BgSpotlight, Color: t.TextOnSpotlight, ContentFontSize: t.FontSize, MaxWidth: 250,
		PaddingBlock: 6, PaddingInline: 8, MinHeight: 32, Radius: t.Radius, Shadow: t.Shadow,
		// antd 6.7 es/tooltip/index.js: mouseEnterDelay / mouseLeaveDelay
		// default 0.1 s; getPlacements offset marginXXS with arrowWidth
		// sizePopupArrow: _util/placements.js halfArrowWidth + offset =
		// 16/2 + 4 = 12 from the trigger (4 without the arrow).
		Offset: 12, MouseEnterDelay: 100 * time.Millisecond, MouseLeaveDelay: 100 * time.Millisecond,
	}
	switch t.Flavor {
	case Element:
		k.PaddingBlock, k.PaddingInline, k.MinHeight = 5, 11, 0
		// es/hooks/use-delayed-toggle: showAfter 0, hideAfter 200;
		// el-tooltip offset 12.
		k.Offset, k.MouseEnterDelay, k.MouseLeaveDelay = 12, 0, 200*time.Millisecond
	case StyleShadcn:
		// px-3 py-1.5, text-xs, no shadow.
		k.ContentFontSize, k.PaddingBlock, k.PaddingInline, k.MinHeight, k.Shadow = t.FontSizeSM, 6, 12, 0, t.TooltipShadow
		// tooltip.tsx: delayDuration 0; Radix closes at once.
		k.MouseEnterDelay, k.MouseLeaveDelay = 0, 0
		k.Offset = 8 // kept from before (not yet checked against Radix sideOffset + arrow)
	}
	return k
}
