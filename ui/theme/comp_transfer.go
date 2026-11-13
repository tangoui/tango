package theme

import "github.com/tangoui/tango/ui/paint"

// TransferToken styles the Transfer lists.
type TransferToken struct {
	Alias
	ListWidth           float32
	ListHeight          float32
	ListBorderRadius    float32
	ListBg              Color
	ListShadow          []paint.Shadow
	BorderColor         Color `contrast:"nontext:@Page"` // list outline
	HeaderHeight        float32
	HeaderBg            Color // zero: none
	DividerColor        Color // under the header, above the pager
	ItemHeight          float32
	ItemPaddingInline   float32 // start of the checkbox
	ItemBorderRadius    float32
	ItemColor           Color `contrast:"text:ListBg"`
	ItemHoverColor      Color // zero: unchanged
	ItemHoverBg         Color // zero: none
	ItemSelectedBg      Color // checked item (zero: none)
	ItemSelectedHoverBg Color // checked item under the pointer (zero: none)
}

func deriveTransfer(t *Theme) TransferToken {
	k := TransferToken{
		ListWidth: 200, ListHeight: 240, ListBorderRadius: t.RadiusLG, ListBg: t.BgContainer,
		BorderColor: t.Border, HeaderHeight: 40, DividerColor: t.BorderSecondary,
		ItemHeight: 32, ItemPaddingInline: 12,
		ItemColor: t.Text, ItemHoverBg: t.ItemHoverBg,
		ItemSelectedBg: t.ItemSelectedBg, ItemSelectedHoverBg: t.Primary.BgHover,
	}
	switch t.Flavor {
	case Element:
		// el-transfer-panel: filled header, no item fills; the label turns
		// brand-colored under the pointer.
		k.ListHeight, k.ListBorderRadius, k.BorderColor = 280, t.Radius, t.BorderSecondary
		k.HeaderBg = t.FillTertiary
		k.ItemHeight, k.ItemPaddingInline = 30, 15
		k.ItemColor, k.ItemHoverColor = t.TextSecondary, t.Primary.Base
		k.ItemHoverBg, k.ItemSelectedBg, k.ItemSelectedHoverBg = Color{}, Color{}, Color{}
	case StyleShadcn:
		// Card lists (rounded-lg, shadow-xs) with px-2 rounded-sm items.
		k.ListShadow = t.CardShadow
		k.ItemPaddingInline, k.ItemBorderRadius = 8, t.RadiusSM
		k.ItemSelectedHoverBg = t.ItemSelectedBg
	}
	return k
}
