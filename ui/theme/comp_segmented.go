package theme

import "github.com/tangoui/tango/ui/paint"

// SegmentedToken styles Segmented.
type SegmentedToken struct {
	Alias
	TrackBg             Color
	TrackPadding        float32
	ItemColor           Color `contrast:"text:TrackBg"`
	ItemHoverColor      Color
	ItemHoverBg         Color
	ItemActiveBg        Color
	ItemSelectedBg      Color
	ItemSelectedColor   Color          `contrast:"text:ItemSelectedBg"`
	ItemSelectedShadow  []paint.Shadow // the sliding thumb
	ItemFontWeight      int
	ItemPaddingInline   float32 // middle and large
	ItemPaddingInlineSM float32
}

func deriveSegmented(t *Theme) SegmentedToken {
	k := SegmentedToken{
		TrackBg: t.SegmentedBg, TrackPadding: 2,
		ItemColor: t.TextSecondary, ItemHoverColor: t.Text, ItemHoverBg: t.FillSecondary, ItemActiveBg: t.Fill,
		ItemSelectedBg: t.SegmentedSelectedBg, ItemSelectedColor: t.Text, ItemSelectedShadow: t.ShadowTertiary,
		ItemFontWeight: 400, ItemPaddingInline: 11, ItemPaddingInlineSM: 7,
	}
	switch t.Flavor {
	case Element:
		k.ItemSelectedColor = t.TextOnPrimary
		k.ItemHoverBg = t.Fill
		k.ItemSelectedShadow = nil
	case StyleShadcn:
		// TabsList: p-[3px], font-medium px-3 triggers, hover only
		// darkens the text.
		k.ItemColor, k.ItemHoverBg, k.ItemActiveBg = t.TextTertiary, Color{}, Color{}
		k.TrackPadding = 3
		k.ItemFontWeight = 500
		k.ItemPaddingInline = 12
	}
	return k
}
