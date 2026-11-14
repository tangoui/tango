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
	// Top is the default distance from the window edge to the stack
	// (MessageRef.Offset overrides it); MarginInline the distance from
	// the side for the start / end placements.
	Top          float32
	MarginInline float32
	// Gap is the space between stacked messages (antd notification
	// list gap = margin 16; EP GAP 16). Sonner stacks keep their own.
	Gap        float32
	CloseColor Color // close button of ShowClose
	// ContentFontSize is the message text size (zero: the theme font size;
	// shadcn Sonner 13px).
	ContentFontSize float32
	// Plain style (Element plain): neutral background, only the icon
	// in the status color.
	PlainBg          Color
	PlainColor       Color `contrast:"text:PlainBg"`
	PlainBorderColor Color // zero: none
	PlainShadow      []paint.Shadow
}

func deriveMessage(t *Theme) MessageToken {
	k := MessageToken{
		ContentBg: t.BgElevated, Color: t.Text, PaddingBlock: 9, PaddingInline: 12,
		Radius: t.RadiusLG, Shadow: t.Shadow,
		Top: 8, MarginInline: 16, Gap: 16, CloseColor: t.TextTertiary,
		PlainBg: t.BgElevated, PlainColor: t.Text, PlainShadow: t.Shadow,
	}
	switch t.Flavor {
	case Element:
		k.ContentBg, k.Color, k.Shadow = Color{}, Color{}, nil
		k.PaddingBlock, k.PaddingInline, k.Radius = 11, 15, t.Radius
		k.Top = 16 // ElMessage offset 16 (es/components/message/src/message.mjs)
		k.PlainBorderColor = t.BorderSecondary
	case StyleShadcn:
		// Sonner 2.0.7 dist/index.mjs styled toast: 356px, padding 16px,
		// 1px --normal-border, radius var(--radius), shadow 0 4px 12px
		// rgba(0,0,0,.1), 13px text.
		sonner := []paint.Shadow{{Y: 4, Blur: 12, Color: RGBA(0, 0, 0, .1)}}
		k.PaddingBlock, k.PaddingInline, k.Width, k.ContentFontSize = 16, 16, 356, 13
		k.BorderColor, k.Shadow = t.PopupBorder, sonner
		k.PlainBorderColor, k.PlainShadow = t.PopupBorder, sonner
	}
	return k
}
