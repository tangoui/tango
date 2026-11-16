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
		// antd 6.7 es/breadcrumb/style/index.js: separatorColor
		// colorTextDescription, separatorMargin marginXS (8) = 4 here plus
		// the link's paddingXXS (antd pulls the link back by marginXXS).
		SeparatorColor: t.TextTertiary, SeparatorMargin: 4, IconSeparatorMargin: 8,
		Radius: t.RadiusSM,
	}
	switch t.Flavor {
	case Element:
		// el-breadcrumb: bold links that turn brand on hover, regular
		// secondary-text current page.
		k.ItemColor, k.LastItemColor, k.LinkHoverColor, k.LinkHoverBg = t.Text, t.TextSecondary, t.Primary.Base, Color{}
		k.LinkFontWeight = 700
		// breadcrumb-item.scss: __separator margin 0 9px in
		// text-color-placeholder (an icon separator 0 6px); links have no
		// padding.
		k.LinkPaddingInline, k.SeparatorMargin, k.IconSeparatorMargin = 0, 9, 6
		k.SeparatorColor = t.TextQuaternary
	case StyleShadcn:
		// breadcrumb.tsx BreadcrumbList gap-1.5 sm:gap-2.5 (10: desktop
		// widths are past sm), muted links that turn foreground on hover,
		// ChevronRight size-3.5 separators.
		k.LinkHoverBg, k.LinkPaddingInline, k.ItemHeight = Color{}, 0, 0
		k.ItemGap, k.SeparatorMargin, k.IconSeparatorMargin = 10, 0, 0
		k.SeparatorColor = t.TextTertiary
	}
	return k
}
