package theme

// AnchorToken styles Anchor.
type AnchorToken struct {
	Alias
	LinkPaddingBlock       float32 // a link is 22px plus this above and below
	LinkPaddingInlineStart float32 // also the indent per nesting level
	LinkColor              Color   `contrast:"text:@Page"`
	LinkHoverColor         Color   `contrast:"text:@Page"`
	LinkActiveColor        Color   `contrast:"text:@Page"`
	RailColor              Color   // track along the start edge (horizontal: the bottom line)
	RailWidth              float32
	InkBarColor            Color   `contrast:"nontext:@Page"` // marker of the active link
	PaddingBlock           float32 // above and below the vertical list
	HorizontalGap          float32 // between links of a horizontal anchor
}

func deriveAnchor(t *Theme) AnchorToken {
	k := AnchorToken{
		LinkPaddingBlock: 4, LinkPaddingInlineStart: 16,
		LinkColor: t.Text, LinkHoverColor: t.Primary.Base, LinkActiveColor: t.Primary.Base,
		RailColor: t.Split, RailWidth: 2, InkBarColor: t.Primary.Base,
		PaddingBlock: 4, HorizontalGap: 16,
	}
	switch t.Flavor {
	case Element:
		k.LinkColor, k.HorizontalGap = t.TextSecondary, 20
	case StyleShadcn:
		// The docs' "On this page" list: muted-foreground links, the
		// current one and the hovered one in foreground (shadcn's primary
		// is near-black, so primary would not mark the current link).
		k.LinkColor, k.LinkHoverColor, k.LinkActiveColor = t.TextTertiary, t.Text, t.Text
	}
	return k
}
