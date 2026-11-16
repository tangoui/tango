package theme

// PageHeaderToken styles PageHeader (Pro Components v3
// layout/components/PageHeader, EP page-header).
type PageHeaderToken struct {
	Alias
	PaddingBlock         float32 // top / bottom padding
	PaddingInline        float32
	PaddingTopBreadcrumb float32 // top padding with a breadcrumb
	BreadcrumbGap        float32 // breadcrumb to heading
	BackGap              float32 // back button (or Element divider) to the title
	BackIconSize         float32
	BackIconGap          float32 // Element: back arrow to its label
	BackColor            Color   `contrast:"text:@Page"`
	TitleFontSize        float32
	TitleLineHeight      float32
	TitleFontWeight      int     // zero: regular
	TitleColor           Color   `contrast:"text:@Page"`
	TitleMarginEnd       float32 // after the title, subtitle and avatar
	SubTitleFontSize     float32
	SubTitleColor        Color `contrast:"text:@Page"`
	ContentMarginTop     float32
	ContentPaddingTop    float32
	ContentBorderColor   Color // line above the content (zero: none)
	FooterMarginTop      float32
	Bg                   Color   // Bordered (ghost=false) background
	BorderColor          Color   // Bordered bottom line (zero: none)
	HeadingMarginBlock   float32 // above and below the title row (Pro heading-left / extra)
}

func derivePageHeader(t *Theme) PageHeaderToken {
	// Pro Components 3.1 layout/components/PageHeader/style/index.js:
	// padding (4+2) 16, paddingSM top with a breadcrumb, breadcrumb +
	// heading marginXS, back marginInlineEnd margin (16) at 16px in
	// colorTextHeading, title fontSizeHeading4 / 600 / line height
	// controlHeight / marginInlineEnd marginSM, subtitle 14
	// colorTextSecondary, heading-left / extra marginBlock marginXS/2,
	// content paddingSM, footer margin; a non-ghost
	// header is colorWhite (container) without a border.
	k := PageHeaderToken{
		PaddingBlock: 6, PaddingInline: 16, PaddingTopBreadcrumb: 12, BreadcrumbGap: 8,
		BackGap: 16, BackIconSize: 16, BackIconGap: 10, BackColor: t.TextHeading,
		TitleFontSize: t.FontSizeXL, TitleLineHeight: t.ControlHeight, TitleFontWeight: 600, TitleColor: t.TextHeading,
		TitleMarginEnd: 12, SubTitleFontSize: 14, SubTitleColor: t.TextSecondary,
		ContentMarginTop: 12, FooterMarginTop: 16, Bg: t.BgContainer, HeadingMarginBlock: 4,
	}
	if t.Flavor == Element {
		// page-header.scss: no padding; the back icon 16px, 10px before
		// "返回"; the divider margin 0 16px; content 18px text-primary;
		// breadcrumb margin-bottom 16px; the main area border-top
		// border-color-light with margin-top 16px.
		k.PaddingBlock, k.PaddingInline, k.PaddingTopBreadcrumb, k.BreadcrumbGap = 0, 0, 0, 16
		k.HeadingMarginBlock = 0
		k.BackColor = t.Text
		k.TitleFontSize, k.TitleLineHeight, k.TitleFontWeight, k.TitleColor = 18, 28, 0, t.Text
		k.ContentMarginTop, k.ContentBorderColor = 16, t.Split
	}
	return k
}
