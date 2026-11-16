package theme

// DividerToken styles Divider and VDivider.
type DividerToken struct {
	Alias
	Color                Color   // the rule (Alias LineWidth thick)
	TextColor            Color   `contrast:"text:@Page"`
	PlainTextColor       Color   `contrast:"text:@Page"`
	TextFontSize         float32 // titled divider (Plain: the Alias FontSize)
	TextFontWeight       int     // titled divider (Plain: regular)
	TextPaddingInline    float32 // title to the rule on each side
	OrientationMargin    float32 // start / end titles: the short rule, a fraction of the width
	MarginBlock          float32 // above and below a plain rule
	TextMarginBlock      float32 // above and below a titled divider
	MarginBlockSM        float32 // Size(Small)
	MarginBlockMD        float32 // Size(Middle)
	VerticalMarginInline float32 // VDivider
	VerticalHeight       float32 // VDivider (zero: the height of the row)
}

func deriveDivider(t *Theme) DividerToken {
	k := DividerToken{
		// antd divider/style/index.js: the titled divider is fontSizeLG,
		// 500, margin `margin`; plain titles fontSize; a rule marginLG;
		// size sm marginXS / md margin; vertical 0.9em high, marginXS.
		Color: t.Split, TextColor: t.TextHeading, PlainTextColor: t.Text,
		TextFontSize: t.FontSizeLG, TextFontWeight: 500, TextPaddingInline: 16, OrientationMargin: 0.05,
		MarginBlock: 24, TextMarginBlock: 16, MarginBlockSM: 8, MarginBlockMD: 16,
		VerticalMarginInline: 8, VerticalHeight: 0.9 * t.FontSize,
	}
	switch t.Flavor {
	case Element:
		// el-divider (theme-chalk divider.scss): the rule in
		// --el-border-color, the 14px title 20px from it, 24px above and
		// below with or without a title; vertical 1em high, 0 8px.
		k.Color, k.TextFontSize, k.TextPaddingInline, k.TextMarginBlock = t.Border, 14, 20, 24
		k.VerticalHeight = t.FontSize
		k.MarginBlockSM, k.MarginBlockMD = 24, 24 // el-divider has no size
	case StyleShadcn:
		// separator.tsx: vertical is h-full w-px (the row's height); no
		// titled form, the title stays text-sm.
		k.TextFontSize, k.VerticalHeight = t.FontSize, 0
		k.MarginBlockSM, k.MarginBlockMD = k.MarginBlock, k.MarginBlock // Separator has no size
	}
	return k
}
