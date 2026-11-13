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
}

func deriveTabs(t *Theme) TabsToken {
	k := TabsToken{
		CardBg: t.FillAlter, CardHeight: 40, HeaderBg: t.TabsHeaderBg,
		InkBarColor: t.Primary.Base, ItemColor: t.Text, ItemHoverColor: t.Primary.Hover,
		ItemSelectedColor: t.Primary.Base, ItemActiveColor: t.Primary.Active,
		TitleFontSize: t.FontSize, TitleFontSizeSM: t.FontSize, TitleFontSizeLG: t.FontSizeLG,
		HorizontalItemGutter: 32, HorizontalItemPadding: 12,
	}
	switch t.Flavor {
	case Element:
		k.ItemHoverColor, k.HorizontalItemGutter, k.HorizontalItemPadding = t.Primary.Base, 20, 0
	case StyleShadcn:
		k.ItemColor, k.ItemHoverColor, k.ItemSelectedColor, k.ItemActiveColor = t.TextTertiary, t.Text, t.Text, t.Text
		k.InkBarColor, k.CardHeight, k.HorizontalItemGutter, k.HorizontalItemPadding = t.Text, 36, 4, 4
	}
	return k
}
