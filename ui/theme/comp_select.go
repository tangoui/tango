package theme

import "github.com/tangoui/tango/ui/paint"

// SelectToken styles Select and the selector family (Cascader, TreeSelect,
// AutoComplete panels).
type SelectToken struct {
	Alias
	SelectorBg               Color
	ActiveBorderColor        Color `contrast:"nontext:SelectorBg"`
	HoverBorderColor         Color
	ActiveOutlineColor       Color // focus halo (zero: none)
	OptionActiveBg           Color // option under the pointer / keyboard cursor
	OptionSelectedBg         Color
	OptionSelectedColor      Color `contrast:"text:OptionSelectedBg"`
	OptionSelectedFontWeight int
	OptionHeight             float32
	OptionFontSize           float32
	OptionPadding            float32 // horizontal option padding
	MultipleItemBg           Color   // tag of a multiple select
	MultipleItemColor        Color   `contrast:"text:MultipleItemBg"`
	MultipleItemHeight       float32 // middle size
	MultipleItemHeightSM     float32
	MultipleItemHeightLG     float32
	ClearBg                  Color
	PopupBg                  Color
	PopupBorderColor         Color // panel outline (zero: none)
	PopupRadius              float32
	PopupShadow              []paint.Shadow
	LoadingColor             Color // text of the loading row (remote search)
	LoadingIconColor         Color // spinner of the loading row
}

func deriveSelect(t *Theme) SelectToken {
	k := SelectToken{
		SelectorBg: t.ControlBg, ActiveBorderColor: t.ControlFocusBorder, HoverBorderColor: t.BorderHover,
		ActiveOutlineColor: t.ControlOutline,
		OptionActiveBg:     t.ItemHoverBg, OptionSelectedBg: t.ItemSelectedBg, OptionSelectedColor: t.Text,
		OptionSelectedFontWeight: t.FontWeightStrong,
		OptionHeight:             t.ControlHeight, OptionFontSize: t.FontSize, OptionPadding: t.ControlPaddingH + 1,
		MultipleItemBg: t.FillSecondary, MultipleItemColor: t.Text,
		MultipleItemHeight: 24, MultipleItemHeightSM: 16, MultipleItemHeightLG: 32,
		ClearBg: t.BgContainer, PopupBg: t.BgElevated, PopupBorderColor: t.PopupBorder,
		PopupRadius: t.RadiusLG, PopupShadow: t.Shadow,
		LoadingColor: t.TextTertiary, LoadingIconColor: t.Primary.Base,
	}
	switch t.Flavor {
	case Element:
		k.OptionHeight, k.OptionPadding = 34, 20
		k.OptionSelectedFontWeight = 700
		k.OptionSelectedBg = Color{}
		k.OptionSelectedColor = t.Primary.Base
		k.MultipleItemBg, k.MultipleItemColor = t.Info.Bg, t.Info.Base
		k.PopupBorderColor = t.Split
		k.PopupRadius = t.Radius
	case StyleShadcn:
		k.OptionHeight, k.OptionPadding = 32, 8
		k.OptionSelectedFontWeight = 400
		k.MultipleItemBg = t.SecondaryBg
		k.MultipleItemColor = t.SecondaryText
		k.HoverBorderColor = t.Border // new-york: no hover border
		k.PopupRadius = t.Radius      // rounded-md panels
	}
	return k
}
