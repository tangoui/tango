package theme

// ListToken styles List (and ListItemMeta / ListActions).
type ListToken struct {
	Alias
	HeaderBg                 Color // zero: none
	FooterBg                 Color // zero: none
	SplitColor               Color // rules between items, header and footer
	BorderColor              Color // bordered outline
	Radius                   float32
	ItemPaddingBlock         float32 // Middle
	ItemPaddingBlockSM       float32
	ItemPaddingBlockLG       float32
	ItemPaddingInline        float32 // bordered lists only (Middle)
	ItemPaddingInlineSM      float32
	ItemPaddingInlineLG      float32
	EmptyTextPadding         float32
	AvatarMarginInlineEnd    float32 // ListItemMeta avatar to text
	TitleColor               Color   `contrast:"text:@Page"`
	TitleFontWeight          int     // zero: regular
	TitleMarginBottom        float32 // title to description
	DescriptionColor         Color   `contrast:"text:@Page"`
	ActionsMarginInlineStart float32 // ListActions
	ActionsSplitColor        Color   // bars between actions
}

func deriveList(t *Theme) ListToken {
	k := ListToken{
		SplitColor: t.Split, BorderColor: t.Border, Radius: t.RadiusLG,
		ItemPaddingBlock: 12, ItemPaddingBlockSM: 8, ItemPaddingBlockLG: 16,
		ItemPaddingInline: 24, ItemPaddingInlineSM: 16, ItemPaddingInlineLG: 24,
		EmptyTextPadding: 16, AvatarMarginInlineEnd: 16,
		TitleColor: t.Text, TitleMarginBottom: 4, DescriptionColor: t.TextTertiary,
		ActionsMarginInlineStart: 48, ActionsSplitColor: t.Split,
	}
	switch t.Flavor {
	case Element:
		k.BorderColor, k.Radius, k.TitleFontWeight = t.BorderSecondary, t.Radius, 500
	case StyleShadcn:
		k.TitleFontWeight = 500 // font-medium title
	}
	return k
}
