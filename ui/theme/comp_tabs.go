package theme

// TabsToken styles Tabs.
type TabsToken struct {
	Alias
	CardBg                Color // inactive card tab
	CardHeight            float32
	HeaderBg              Color // header strip of card tabs (zero: none)
	InkBarColor           Color `contrast:"nontext:@Page"`
	ItemColor             Color `contrast:"text:@Page"`
	ItemHoverColor        Color
	ItemSelectedColor     Color `contrast:"text:@Page"`
	ItemActiveColor       Color
	TitleFontSize         float32
	TitleFontSizeSM       float32
	TitleFontSizeLG       float32
	HorizontalItemGutter  float32
	HorizontalItemPadding float32 // vertical padding of a horizontal tab

	// Sizes (antd cardHeightSM / cardHeightLG, horizontalItemPaddingSM /
	// LG, the inline part of cardPadding / cardPaddingSM).
	CardHeightSM            float32
	CardHeightLG            float32
	HorizontalItemPaddingSM float32
	HorizontalItemPaddingLG float32
	CardPaddingH            float32 // inline padding of a card tab (middle and large)
	CardPaddingHSM          float32 // … of a small card tab

	// ItemSelectedBg and ItemSelectedBorder paint the active trigger of
	// the shadcn segmented list (zero border: none).
	ItemSelectedBg     Color
	ItemSelectedBorder Color

	// SegmentedList draws the default bar as a segmented track (the
	// shadcn TabsList); Variant("line") then switches to line tabs.
	// Without it the default bar is already line tabs, so "line" changes
	// nothing.
	SegmentedList bool
}

func deriveTabs(t *Theme) TabsToken {
	// antd 6.7 es/tabs/style/index.js prepareComponentToken.
	k := TabsToken{
		CardBg: t.FillAlter, CardHeight: t.ControlHeightLG, HeaderBg: t.TabsHeaderBg,
		InkBarColor: t.Primary.Base, ItemColor: t.Text, ItemHoverColor: t.Primary.Hover,
		ItemSelectedColor: t.Primary.Base, ItemActiveColor: t.Primary.Active,
		TitleFontSize: t.FontSize, TitleFontSizeSM: t.FontSize, TitleFontSizeLG: t.FontSizeLG,
		HorizontalItemGutter: 32, HorizontalItemPadding: 12,
		CardHeightSM: t.ControlHeight, CardHeightLG: t.ControlHeightLG + 8,
		HorizontalItemPaddingSM: 8, HorizontalItemPaddingLG: 16,
		CardPaddingH: 16, CardPaddingHSM: 8,
		ItemSelectedBg: t.SegmentedSelectedBg,
	}
	switch t.Flavor {
	case Element:
		// el-tabs has one size: 40px items, no vertical padding.
		k.ItemHoverColor, k.HorizontalItemGutter, k.HorizontalItemPadding = t.Primary.Base, 20, 0
		k.CardHeight, k.CardHeightSM, k.CardHeightLG = 40, 40, 40
		k.HorizontalItemPaddingSM, k.HorizontalItemPaddingLG, k.CardPaddingHSM = 0, 0, 16
	case StyleShadcn:
		k.ItemColor, k.ItemHoverColor, k.ItemSelectedColor, k.ItemActiveColor = t.TextTertiary, t.Text, t.Text, t.Text
		k.InkBarColor, k.CardHeight, k.HorizontalItemGutter, k.HorizontalItemPadding = t.Text, 36, 4, 4
		k.SegmentedList = true
		k.CardHeightSM, k.CardHeightLG = 36, 36
		k.HorizontalItemPaddingSM, k.HorizontalItemPaddingLG = 4, 4
		// tabs.tsx TabsTrigger: data-[state=active]:bg-background;
		// dark:data-[state=active]:border-input dark:bg-input/30.
		if t.Shadcn != nil && t.Dark {
			v := t.Shadcn
			k.ItemSelectedBg, k.ItemSelectedBorder = Over(Fade(v.Input, .3), v.Muted), v.Input
		}
	}
	return k
}
