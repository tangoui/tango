package theme

// TypographyToken styles the Typography decorations of Text / Link:
// Code, Mark, links, the copy / edit actions, and the RichText highlight.
type TypographyToken struct {
	Alias
	MarkBg           Color // Label.Mark highlight
	MarkColor        Color `contrast:"text:MarkBg"` // zero: keep the text color
	HighlightBg      Color // RichText Mark tint (follows the theme's lightness)
	HighlightColor   Color `contrast:"text:HighlightBg"` // zero: keep the span color
	CodeBg           Color
	CodeBorderColor  Color // zero: none
	CodeRadius       float32
	LinkColor        Color `contrast:"text:@Page"`
	LinkHoverColor   Color `contrast:"text:@Page"`
	LinkActiveColor  Color `contrast:"text:@Page"`
	CopyIconColor    Color `contrast:"nontext:@Page"`
	EditIconColor    Color `contrast:"nontext:@Page"`
	ActionHoverColor Color // copy / edit icon under the pointer
	CopiedIconColor  Color // the check shown after copying
}

func deriveTypography(t *Theme) TypographyToken {
	k := TypographyToken{
		MarkBg: t.MarkBg, MarkColor: t.MarkText, HighlightBg: t.Warning.Shades[2],
		CodeBg: t.FillTertiary, CodeBorderColor: t.Split, CodeRadius: 3,
		LinkColor: t.TextLink, LinkHoverColor: t.TextLinkHover, LinkActiveColor: t.TextLinkActive,
		CopyIconColor: t.TextLink, EditIconColor: t.TextLink, ActionHoverColor: t.TextLinkHover,
		CopiedIconColor: t.Success.Base,
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
