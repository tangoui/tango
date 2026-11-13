package theme

// DividerToken styles Divider and VDivider.
type DividerToken struct {
	Alias
	Color                Color   // the rule (Alias LineWidth thick)
	TextColor            Color   `contrast:"text:@Page"`
	PlainTextColor       Color   `contrast:"text:@Page"`
	TextFontWeight       int     // titled divider (Plain: regular)
	TextPaddingInline    float32 // title to the rule on each side
	OrientationMargin    float32 // start / end titles: the short rule, a fraction of the width
	MarginBlock          float32 // above and below a plain rule
	TextMarginBlock      float32 // above and below a titled divider
	VerticalMarginInline float32 // VDivider
	VerticalHeight       float32 // VDivider
}

func deriveDivider(t *Theme) DividerToken {
	k := DividerToken{
		Color: t.Split, TextColor: t.TextHeading, PlainTextColor: t.Text,
		TextFontWeight: 500, TextPaddingInline: 16, OrientationMargin: 0.05,
		MarginBlock: 24, TextMarginBlock: 16, VerticalMarginInline: 8, VerticalHeight: 14,
	}
	if t.Flavor == Element {
		// el-divider (theme-chalk divider.scss): the rule in
		// --el-border-color, the title 20px from it, 24px above and below
		// with or without a title.
		k.Color, k.TextPaddingInline, k.TextMarginBlock = t.Border, 20, 24
	}
	return k
}
