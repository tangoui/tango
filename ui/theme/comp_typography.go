package theme

// TypographyToken styles the Typography decorations of Text / Link:
// Code, Mark, links, the copy / edit actions, and the RichText highlight.
type TypographyToken struct {
	Alias
	MarkBg          Color // Label.Mark highlight
	MarkColor       Color `contrast:"text:MarkBg"` // zero: keep the text color
	HighlightBg     Color // RichText Mark tint (follows the theme's lightness)
	HighlightColor  Color `contrast:"text:HighlightBg"` // zero: keep the span color
	CodeBg          Color
	CodeBorderColor Color // zero: none
	CodeRadius      float32
	LinkColor       Color `contrast:"text:@Page"`
	LinkHoverColor  Color `contrast:"text:@Page"`
	LinkActiveColor Color `contrast:"text:@Page"`
	// LinkTextColor, LinkTextHoverColor and LinkTextActiveColor are the
	// Link component's own colors at rest, under the pointer and pressed.
	// LinkColor stays the link color of other links (Paragraph expand,
	// copy / edit icons): Element's el-link defaults to regular text.
	LinkTextColor       Color   `contrast:"text:@Page"`
	LinkTextHoverColor  Color   `contrast:"text:@Page"`
	LinkTextActiveColor Color   `contrast:"text:@Page"`
	LinkFontWeight      int     // Link text weight
	LinkUnderline       float32 // Link underline at rest (zero: none)
	// LinkHoverUnderline is the Link underline under the pointer (zero:
	// none).
	LinkHoverUnderline float32
	CopyIconColor      Color `contrast:"nontext:@Page"`
	EditIconColor      Color `contrast:"nontext:@Page"`
	ActionHoverColor   Color // copy / edit icon under the pointer
	CopiedIconColor    Color // the check shown after copying
}

func deriveTypography(t *Theme) TypographyToken {
	k := TypographyToken{
		MarkBg: t.MarkBg, MarkColor: t.MarkText, HighlightBg: t.Warning.Shades[2],
		CodeBg: t.FillTertiary, CodeBorderColor: t.Split, CodeRadius: 3,
		LinkColor: t.TextLink, LinkHoverColor: t.TextLinkHover, LinkActiveColor: t.TextLinkActive,
		CopyIconColor: t.TextLink, EditIconColor: t.TextLink, ActionHoverColor: t.TextLinkHover,
		CopiedIconColor: t.Success.Base,
		// antd typography: links inherit the weight and are not
		// underlined (linkDecoration / linkHoverDecoration none).
		LinkFontWeight: 400,
	}
	k.LinkTextColor, k.LinkTextHoverColor, k.LinkTextActiveColor = k.LinkColor, k.LinkHoverColor, k.LinkActiveColor
	switch t.Flavor {
	case Element:
		// el-link: --el-link-font-weight is font-weight-primary (500),
		// underline="hover" by default.
		k.LinkFontWeight, k.LinkHoverUnderline = 500, 1
		// el-link type="default": --el-link-text-color is
		// text-color-regular, hover-text-color color-primary; no separate
		// pressed color (theme-chalk/src/common/var.scss $link).
		k.LinkTextColor, k.LinkTextHoverColor, k.LinkTextActiveColor = t.TextSecondary, t.Primary.Base, t.Primary.Base
	case StyleShadcn:
		// typography: font-medium underline underline-offset-4.
		k.LinkFontWeight, k.LinkUnderline, k.LinkHoverUnderline = 500, 1, 1
	}
	// MarkText is chosen for MarkBg; on the tint it is used only where it
	// reads, else body text.
	if t.MarkText.A > 0 {
		k.HighlightColor = t.MarkText
		if ContrastRatio(k.HighlightColor, k.HighlightBg) < 4.5 {
			k.HighlightColor = t.Text
		}
	}
	if t.Flavor == StyleShadcn {
		k.CodeBg, k.CodeBorderColor, k.CodeRadius = t.FillSecondary, Color{}, t.RadiusSM // bg-muted, no border
	}
	return k
}
