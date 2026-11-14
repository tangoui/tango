package theme

// ItemToken styles Item and ItemGroup.
type ItemToken struct {
	Alias
	Radius              float32
	PaddingBlock        float32 // default size
	PaddingInline       float32
	PaddingBlockSM      float32 // small size
	PaddingInlineSM     float32
	Gap                 float32 // between media, content and actions
	GapSM               float32
	ContentGap          float32 // title to description
	TitleFontSize       float32
	TitleFontWeight     int   // zero: regular
	TitleColor          Color `contrast:"text:@Page"`
	DescriptionColor    Color `contrast:"text:@Page"`
	DescriptionSize     float32
	BorderColor         Color // outline variant
	MutedBg             Color // muted variant
	HoverBg             Color // clickable item under the pointer (zero: none)
	MediaIconBox        float32
	MediaIconSize       float32
	MediaIconRadius     float32
	MediaIconBg         Color // zero: none
	MediaIconBorder     Color // zero: none
	MediaIconColor      Color `contrast:"nontext:MediaIconBg"`
	ActionsGap          float32
	GroupSeparator      Color   // rule between the items of a separated group
	GroupSeparatorInset float32 // vertical space around the rule
}

func deriveItem(t *Theme) ItemToken {
	k := ItemToken{
		Radius:       t.RadiusLG,
		PaddingBlock: 16, PaddingInline: 16, PaddingBlockSM: 10, PaddingInlineSM: 12,
		Gap: 16, GapSM: 10, ContentGap: 4,
		TitleFontSize: t.FontSize, TitleFontWeight: t.FontWeightStrong, TitleColor: t.Text,
		DescriptionColor: t.TextSecondary, DescriptionSize: t.FontSize,
		BorderColor: t.BorderSecondary, MutedBg: t.FillQuaternary, HoverBg: t.ItemHoverBg,
		MediaIconBox: 36, MediaIconSize: 18, MediaIconRadius: t.Radius,
		MediaIconBg: t.Primary.Bg, MediaIconColor: t.Primary.Base,
		ActionsGap: 8, GroupSeparator: t.Split,
	}
	switch t.Flavor {
	case Element:
		k.Radius, k.TitleFontWeight = t.Radius, 500
		k.BorderColor, k.MutedBg = t.BorderSecondary, t.FillAlter
		k.DescriptionColor = t.TextSecondary
		k.MediaIconRadius = t.RadiusSM
	case StyleShadcn:
		// Item: rounded-md border-transparent text-sm, p-4 gap-4 (sm: py-3
		// px-4 gap-2.5); ItemTitle font-medium; ItemDescription
		// muted-foreground; ItemMedia variant="icon": size-8 rounded-sm
		// border bg-muted with a size-4 icon; outline: border-border;
		// muted: bg-muted/50; a link item hovers bg-accent/50.
		k.Radius = t.Radius
		k.PaddingBlockSM, k.PaddingInlineSM = 12, 16
		k.TitleFontWeight, k.DescriptionColor = 500, t.TextTertiary
		k.BorderColor = t.Border
		k.MutedBg = Fade(t.FillSecondary, .5)
		k.HoverBg = Fade(t.ItemHoverBg, .5)
		k.MediaIconBox, k.MediaIconSize, k.MediaIconRadius = 32, 16, t.RadiusSM
		k.MediaIconBg, k.MediaIconBorder, k.MediaIconColor = t.FillSecondary, t.Border, t.Text
		k.GroupSeparator = t.Border
	}
	return k
}
