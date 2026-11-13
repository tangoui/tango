package theme

// BreadcrumbToken styles Breadcrumb.
type BreadcrumbToken struct {
	Alias
	IconFontSize        float32 // item icons and icon separators
	ItemColor           Color   `contrast:"text:@Page"` // links before the current page
	LastItemColor       Color   `contrast:"text:@Page"` // the current page
	LinkHoverColor      Color   `contrast:"text:LinkHoverBg"`
	LinkHoverBg         Color   // zero: none
	LinkFontWeight      int     // links before the current page (zero: regular)
	LinkPaddingInline   float32 // inline padding of a link (its hover background)
	ItemHeight          float32 // link height (zero: the line height)
	ItemGap             float32 // gap between items and separators
	SeparatorColor      Color
	SeparatorMargin     float32 // inline margin of a text separator
	IconSeparatorMargin float32 // inline margin of an icon separator
	Radius              float32 // link hover background
}

func deriveBreadcrumb(t *Theme) BreadcrumbToken {
	k := BreadcrumbToken{
		IconFontSize: 14,
		ItemColor:    t.TextTertiary, LastItemColor: t.Text, LinkHoverColor: t.Text, LinkHoverBg: t.BgTextHover,
		LinkPaddingInline: 4, ItemHeight: 22,
		SeparatorColor: t.TextQuaternary, SeparatorMargin: 4, IconSeparatorMargin: 8,
		Radius: t.RadiusSM,
	}
	switch t.Flavor {
	case Element:
		// el-breadcrumb: bold links that turn brand on hover, regular
		// secondary-text current page.
		k.ItemColor, k.LastItemColor, k.LinkHoverColor, k.LinkHoverBg = t.Text, t.TextSecondary, t.Primary.Base, Color{}
		k.LinkFontWeight = 700
	case StyleShadcn:
		// BreadcrumbList gap-1.5 / sm:gap-2.5 (8 here), muted links that
		// turn foreground on hover, chevron separators.
		k.LinkHoverBg, k.LinkPaddingInline, k.ItemHeight = Color{}, 0, 0
		k.ItemGap, k.SeparatorMargin, k.IconSeparatorMargin = 8, 0, 0
		k.SeparatorColor = t.TextTertiary
	}
	return k
}
