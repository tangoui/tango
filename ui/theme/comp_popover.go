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
}

func derivePopover(t *Theme) PopoverToken {
	k := PopoverToken{
		Bg: t.BgElevated, TitleColor: t.Text, Color: t.Text, TitleFontSize: t.FontSize, TitleFontWeight: t.FontWeightStrong,
		TitleMarginBottom: 8, TitleMinWidth: 177, InnerPadding: 12,
		Radius: t.RadiusLG, BorderColor: t.PopupBorder, Shadow: t.Shadow,
		// antd 6.7 es/popover/index.js: mouseEnterDelay / mouseLeaveDelay
		// default 0.1 s.
		MouseEnterDelay: 100 * time.Millisecond, MouseLeaveDelay: 100 * time.Millisecond,
	}
	switch t.Flavor {
	case Element:
		k.TitleColor, k.TitleFontSize, k.TitleFontWeight, k.TitleMarginBottom = t.TextHeading, 16, 0, 12
		k.TitleMinWidth, k.Radius = 150, t.Radius
		// el-popover width 150 (es/components/popover/src/popover.mjs),
		// use-delayed-toggle showAfter 0 / hideAfter 200.
		k.Width, k.MouseEnterDelay, k.MouseLeaveDelay = 150, 0, 200*time.Millisecond
	case StyleShadcn:
		// p-4, title font-medium leading-none.
		k.TitleFontWeight, k.InnerPadding, k.Radius = 500, 16, t.Radius
		// popover.tsx: w-72. Radix Popover has no hover trigger; the hover
		// delays stay as before (HoverCard is aligned in batch 3).
		k.Width, k.MouseEnterDelay, k.MouseLeaveDelay = 288, 0, 100*time.Millisecond
	}
	return k
}
