package theme

import (
	"time"

	"github.com/tangoui/tango/ui/paint"
)

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
	Width             float32       // default panel width (zero: fits the content)
	MouseEnterDelay   time.Duration // hover trigger: delay before opening
	MouseLeaveDelay   time.Duration // hover trigger: delay before closing

	// Popconfirm defaults (Popconfirm.Placement / Width / CancelType
	// override).
	PopconfirmBottom     bool    // placement "bottom" (Element) instead of "top"
	PopconfirmWidth      float32 // zero: fits the content
	PopconfirmCancelText bool    // cancel button of type "text" (Element)

	// Arrow: Popover and Popconfirm show an arrow by default (shadcn has
	// none). Offset / OffsetNoArrow: panel distance from the trigger with
	// and without the arrow.
	Arrow         bool
	Offset        float32
	OffsetNoArrow float32
}

func derivePopover(t *Theme) PopoverToken {
	k := PopoverToken{
		Bg: t.BgElevated, TitleColor: t.Text, Color: t.Text, TitleFontSize: t.FontSize, TitleFontWeight: t.FontWeightStrong,
		TitleMarginBottom: 8, TitleMinWidth: 177, InnerPadding: 12,
		Radius: t.RadiusLG, BorderColor: t.PopupBorder, Shadow: t.Shadow,
		// antd 6.7 es/popover/index.js: mouseEnterDelay / mouseLeaveDelay
		// default 0.1 s.
		MouseEnterDelay: 100 * time.Millisecond, MouseLeaveDelay: 100 * time.Millisecond,
		// antd 6.7 es/popover/index.js -> tooltip getPlacements:
		// _util/placements.js halfArrowWidth (sizePopupArrow 16 / 2) +
		// offset marginXXS 4 = 12; without the arrow arrowWidth 0 -> 4.
		Arrow: true, Offset: 12, OffsetNoArrow: 4,
	}
	switch t.Flavor {
	case Element:
		k.TitleColor, k.TitleFontSize, k.TitleFontWeight, k.TitleMarginBottom = t.TextHeading, 16, 0, 12
		k.TitleMinWidth, k.Radius = 150, t.Radius
		// el-popover width 150 (es/components/popover/src/popover.mjs),
		// use-delayed-toggle showAfter 0 / hideAfter 200.
		k.Width, k.MouseEnterDelay, k.MouseLeaveDelay = 150, 0, 200*time.Millisecond
		// el-popconfirm (es/components/popconfirm/src/popconfirm.mjs):
		// placement bottom, width 150, cancel-button-type text.
		k.PopconfirmBottom, k.PopconfirmWidth, k.PopconfirmCancelText = true, 150, true
		// el-popover / popper offset 12 (es/components/popper/src/content.mjs).
		k.Offset, k.OffsetNoArrow = 12, 12
	case StyleShadcn:
		// p-4, title font-medium leading-none.
		k.TitleFontWeight, k.InnerPadding, k.Radius = 500, 16, t.Radius
		// Radix Popover has no arrow; popover.tsx sideOffset 4.
		k.Arrow, k.Offset, k.OffsetNoArrow = false, 4, 4
		// popover.tsx: w-72. Radix Popover has no hover trigger: a hover
		// Popover is the shadcn HoverCard, whose Root defaults openDelay
		// 700 / closeDelay 300 (@radix-ui/react-hover-card 1.1.15
		// dist/index.mjs).
		k.Width, k.MouseEnterDelay, k.MouseLeaveDelay = 288, 700*time.Millisecond, 300*time.Millisecond
	}
	return k
}
