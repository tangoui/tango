package theme

import "time"

// AnchorToken styles Anchor.
type AnchorToken struct {
	Alias
	LinkPaddingBlock       float32 // a link is 22px plus this above and below
	LinkPaddingBlockNested float32 // the same for nested links
	LinkPaddingInlineStart float32 // also the indent per nesting level
	LinkColor              Color   `contrast:"text:@Page"`
	LinkHoverColor         Color   `contrast:"text:@Page"`
	LinkActiveColor        Color   `contrast:"text:@Page"`
	RailColor              Color   // track along the start edge (horizontal: the bottom line)
	RailWidth              float32
	ShowRail               bool    // draw the rail by default (Element type "default" has none)
	InkBarColor            Color   `contrast:"nontext:@Page"` // marker of the active link
	InkWidth               float32 // vertical marker thickness (zero: RailWidth)
	InkHeight              float32 // vertical marker length (zero: the link's title height)
	InkRadius              float32 // clamped to half the marker width
	PaddingBlock           float32 // above and below the vertical list
	HorizontalGap          float32 // between links of a horizontal anchor
	Bounds                 float32 // a section is active once its top is this close to the line
	ScrollDuration         time.Duration
}

func deriveAnchor(t *Theme) AnchorToken {
	// antd 6.7 es/anchor/style/index.js: linkPaddingBlock paddingXXS,
	// nested anchorPaddingBlockSecondary paddingXXS/2, linkPaddingInlineStart
	// padding, the rail lineWidthBold colorSplit, the ink lineWidthBold as
	// tall as the title; titles colorText, the active one colorPrimary (no
	// hover color); Anchor.js bounds 5; _util/scrollTo.js duration 450.
	k := AnchorToken{
		LinkPaddingBlock: 4, LinkPaddingBlockNested: 2, LinkPaddingInlineStart: 16,
		LinkColor: t.Text, LinkHoverColor: t.Text, LinkActiveColor: t.Primary.Base,
		RailColor: t.Split, RailWidth: 2, ShowRail: true, InkBarColor: t.Primary.Base,
		PaddingBlock: 4, HorizontalGap: 16, Bounds: 5, ScrollDuration: 450 * time.Millisecond,
	}
	switch t.Flavor {
	case Element:
		// el-anchor: bound 15, duration 300; type "default" draws no rail
		// and a 4x14 rounded marker; links turn primary on hover.
		k.LinkColor, k.LinkHoverColor, k.HorizontalGap = t.TextSecondary, t.Primary.Base, 20
		k.ShowRail, k.InkWidth, k.InkHeight, k.InkRadius = false, 4, 14, 4 // radius 4px, drawn clamped to 2
		k.Bounds, k.ScrollDuration = 15, 300*time.Millisecond
	case StyleShadcn:
		// The docs' "On this page" list: muted-foreground links, the
		// current one and the hovered one in foreground (shadcn's primary
		// is near-black, so primary would not mark the current link).
		k.LinkColor, k.LinkHoverColor, k.LinkActiveColor = t.TextTertiary, t.Text, t.Text
	}
	return k
}
