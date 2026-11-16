package theme

// ListToken styles List (and ListItemMeta / ListActions) and Listy.
type ListToken struct {
	Alias
	HeaderBg                 Color // zero: none
	FooterBg                 Color // zero: none
	SplitColor               Color // rules between items, header and footer
	BorderColor              Color // bordered outline
	Radius                   float32
	HeaderPaddingBlock       float32 // header and footer (a bordered small / large list uses the item padding)
	ItemPaddingBlock         float32 // Middle
	ItemPaddingBlockSM       float32
	ItemPaddingBlockLG       float32
	ItemPaddingInline        float32 // Middle, bordered lists only
	ItemPaddingInlineSM      float32 // Small, with or without a border
	ItemPaddingInlineLG      float32 // Large, with or without a border
	EmptyTextPadding         float32
	AvatarMarginInlineEnd    float32 // ListItemMeta avatar to text
	TitleColor               Color   `contrast:"text:@Page"`
	TitleFontWeight          int     // zero: regular
	TitleMarginBottom        float32 // title to description
	DescriptionColor         Color   `contrast:"text:@Page"`
	ActionsMarginInlineStart float32 // ListActions
	ActionsSplitColor        Color   // bars between actions
	// ActionsGap separates actions (antd li padding 0 paddingXS); the bar
	// sits in the middle and the last action keeps half a gap after it.
	ActionsGap           float32
	PaginationMarginTop  float32 // pager under the list
	GridItemMarginBottom float32 // space under every grid row (antd grid item marginBlockEnd; gutter is horizontal only)
	// Bordered lists keep the pager inside the outline with this margin.
	PaginationMarginBlockBordered  float32
	PaginationMarginInlineBordered float32
	// Vertical item layout (List.ItemLayout("vertical")).
	VerticalMetaMarginBottom       float32 // under ListItemMeta
	VerticalTitleFontSize          float32
	VerticalTitleMarginBottom      float32
	VerticalActionsMarginTop       float32
	VerticalActionsGap             float32 // between actions (bar in the middle)
	VerticalExtraMarginInlineStart float32 // List.ItemExtra

	// Listy (antd 6.7 es/listy/style).
	ListyItemPaddingBlock        float32
	ListyItemPaddingInline       float32
	ListyItemHoverBg             Color
	ListySplitColor              Color
	ListyGroupHeaderPaddingBlock float32
	ListyGroupHeaderBg           Color // over BgContainer
	ListyGroupHeaderColor        Color `contrast:"text:@Page"`
	ListyGroupHeaderFontWeight   int
}

func deriveList(t *Theme) ListToken {
	// antd es/list/style/index.js prepareComponentToken: itemPadding
	// "paddingContentVertical 0" (12 0), itemPaddingSM "8 16", itemPaddingLG
	// "16 24"; header / footer paddingBlock paddingSM; a bordered list pads
	// paddingLG inline.
	k := ListToken{
		SplitColor: t.Split, BorderColor: t.Border, Radius: t.RadiusLG,
		HeaderPaddingBlock: 12,
		ItemPaddingBlock:   12, ItemPaddingBlockSM: 8, ItemPaddingBlockLG: 16,
		ItemPaddingInline: 24, ItemPaddingInlineSM: 16, ItemPaddingInlineLG: 24,
		EmptyTextPadding: 16, AvatarMarginInlineEnd: 16,
		TitleColor: t.Text, TitleMarginBottom: 4, DescriptionColor: t.TextTertiary,
		ActionsMarginInlineStart: 48, ActionsSplitColor: t.Split, ActionsGap: 16,
		PaginationMarginTop: 24, GridItemMarginBottom: 16, PaginationMarginBlockBordered: 16, PaginationMarginInlineBordered: 24,
		VerticalMetaMarginBottom: 16, VerticalTitleFontSize: t.FontSizeLG, VerticalTitleMarginBottom: 12,
		VerticalActionsMarginTop: 16, VerticalActionsGap: 32, VerticalExtraMarginInlineStart: 24,
		ListyItemPaddingBlock: 12, ListyItemPaddingInline: 16, ListyItemHoverBg: t.ItemHoverBg,
		ListySplitColor: t.Split, ListyGroupHeaderPaddingBlock: 8, ListyGroupHeaderBg: t.FillAlter,
		ListyGroupHeaderColor: t.TextTertiary, ListyGroupHeaderFontWeight: t.FontWeightStrong,
	}
	switch t.Flavor {
	case Element:
		k.BorderColor, k.Radius, k.TitleFontWeight = t.BorderSecondary, t.Radius, 500
	case StyleShadcn:
		k.TitleFontWeight = 500 // font-medium title
	}
	return k
}
