package theme

// NotificationCenterToken styles NotificationCenter: the bell trigger with
// its unread badge and the panel listing the notifications.
type NotificationCenterToken struct {
	Alias
	TriggerSize     float32 // square trigger button
	TriggerIconSize float32

	PanelWidth     float32
	PanelMaxHeight float32 // the list scrolls beyond it

	HeaderPaddingBlock  float32
	HeaderPaddingInline float32
	HeaderFontSize      float32
	HeaderFontWeight    int   // zero: regular
	HeaderColor         Color `contrast:"text:@Elevated"`
	HeaderBorder        Color // rule under the header (zero: none)

	GroupFontSize     float32
	GroupColor        Color `contrast:"text:@Elevated"`
	GroupPaddingBlock float32

	ItemPaddingBlock  float32
	ItemPaddingInline float32
	ItemGap           float32 // icon to text
	ItemHoverBg       Color   // zero: none
	ItemUnreadBg      Color   // unread items (zero: none)
	ItemDivider       Color   // between items (zero: none)
	IconSize          float32
	TitleFontSize     float32
	TitleColor        Color `contrast:"text:@Elevated"`
	UnreadFontWeight  int   // title weight of unread items
	BodyColor         Color `contrast:"text:@Elevated"`
	TimeFontSize      float32
	TimeColor         Color `contrast:"text:@Elevated"`
	UnreadDot         Color `contrast:"nontext:@Elevated"`
	UnreadDotSize     float32
	ActionsGap        float32
}

func deriveNotificationCenter(t *Theme) NotificationCenterToken {
	k := NotificationCenterToken{
		TriggerSize: 28, TriggerIconSize: 16,
		PanelWidth: 360, PanelMaxHeight: 420,
		HeaderPaddingBlock: 10, HeaderPaddingInline: 16, HeaderFontSize: t.FontSize,
		HeaderFontWeight: t.FontWeightStrong, HeaderColor: t.Text, HeaderBorder: t.Split,
		GroupFontSize: t.FontSizeSM, GroupColor: t.TextTertiary, GroupPaddingBlock: 6,
		ItemPaddingBlock: 10, ItemPaddingInline: 16, ItemGap: 10,
		ItemHoverBg: t.ItemHoverBg, ItemUnreadBg: Color{}, ItemDivider: Color{},
		IconSize: 16, TitleFontSize: t.FontSize, TitleColor: t.Text, UnreadFontWeight: t.FontWeightStrong,
		BodyColor: t.TextSecondary, TimeFontSize: t.FontSizeSM, TimeColor: t.TextTertiary,
		UnreadDot: t.Primary.Base, UnreadDotSize: 6, ActionsGap: 8,
	}
	switch t.Flavor {
	case Element:
		k.HeaderBorder, k.HeaderFontWeight = t.BorderSecondary, 500
		k.ItemDivider = t.BorderSecondary
	case StyleShadcn:
		// Popover card: p-0, header text-sm font-medium with border-b; rows
		// hover bg-accent, muted-foreground descriptions.
		k.HeaderFontSize, k.HeaderFontWeight, k.HeaderBorder = t.FontSize, 500, t.Border
		k.ItemHoverBg = t.ItemHoverBg
		k.UnreadFontWeight = 500
		k.BodyColor, k.TimeColor, k.GroupColor = t.TextTertiary, t.TextTertiary, t.TextTertiary
		k.UnreadDot = t.Primary.Base
	}
	return k
}
