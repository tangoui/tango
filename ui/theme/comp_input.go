package theme

import "github.com/tangoui/tango/ui/paint"

// InputToken styles Input and the input-like controls (InputNumber,
// TextArea, pickers' fields).
type InputToken struct {
	Alias
	PaddingInline   float32
	PaddingInlineSM float32
	PaddingInlineLG float32
	InputFontSize   float32
	InputFontSizeSM float32
	InputFontSizeLG float32

	Bg                Color // resting field fill
	ActiveBg          Color // focused field fill
	HoverBg           Color
	ActiveBorderColor Color `contrast:"nontext:ActiveBg"` // focused field border (the focus indicator)
	HoverBorderColor  Color
	PlaceholderColor  Color `contrast:"text:ActiveBg"`
	AddonBg           Color
	AddonColor        Color `contrast:"text:AddonBg"`

	// Focus halo (Ant activeShadow): color, width and gap; zero color: none.
	ActiveShadowColor        Color
	ActiveShadowWidth        float32
	ActiveShadowOffset       float32
	ErrorActiveShadowColor   Color
	WarningActiveShadowColor Color

	Shadow []paint.Shadow // resting field shadow (shadcn shadow-xs)
}

func deriveInput(t *Theme) InputToken {
	k := InputToken{
		// The text inset is the same in every design system (Ant's 11px,
		// 7px when small).
		PaddingInline: 11, PaddingInlineSM: 7, PaddingInlineLG: 11,
		InputFontSize: t.FontSize, InputFontSizeSM: t.FontSize, InputFontSizeLG: t.FontSizeLG,
		Bg: t.ControlBg, ActiveBg: t.ControlBg, HoverBg: t.ControlBg,
		ActiveBorderColor: t.ControlFocusBorder, HoverBorderColor: t.BorderHover,
		PlaceholderColor: t.TextQuaternary,
		AddonBg:          t.AddonBg, AddonColor: t.AddonText,
		ActiveShadowColor: t.ControlOutline, ActiveShadowWidth: t.ControlOutlineWidth, ActiveShadowOffset: t.ControlOutlineOffset,
		ErrorActiveShadowColor: t.ControlOutlineError, WarningActiveShadowColor: t.ControlOutlineWarning,
		Shadow: t.InputShadow,
	}
	if t.Flavor == StyleShadcn {
		// new-york: text-sm at every size, no hover border.
		k.InputFontSizeLG = t.FontSize
		k.HoverBorderColor = t.Border
	}
	return k
}
