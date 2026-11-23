package theme

import "github.com/tangoui/tango/ui/paint"

// SegmentedToken styles Segmented.
type SegmentedToken struct {
	Alias
	TrackBg        Color
	TrackPadding   float32
	ItemColor      Color `contrast:"text:TrackBg"`
	ItemHoverColor Color
	ItemHoverBg    Color
	ItemActiveBg   Color
	// ItemSelectedBorder optionally outlines the selected item.
	ItemSelectedBorder Color
	// FocusOutlineColor overrides the Element Plus keyboard focus outline.
	FocusOutlineColor   Color
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
		if t.UsesElementPlusControls() {
			// Element Plus 2.14.8 common/var.scss $segmented.
			k.ItemSelectedBg = t.Primary.Base
			k.FocusOutlineColor = t.Primary.Base
			k.ItemHoverBg = Hex("#ebedf0")
			k.ItemActiveBg = Hex("#e6e8eb")
			if t.Dark {
				k.TrackBg = Hex("#262727")
				k.ItemHoverBg = Hex("#39393a")
				k.ItemActiveBg = Hex("#424243")
			}
		}
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
