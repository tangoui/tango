package theme

// StatisticToken styles Statistic (StatisticCard shares its sizes).
type StatisticToken struct {
	Alias
	TitleFontSize     float32
	TitleFontWeight   int   // zero: regular
	TitleColor        Color `contrast:"text:@Page"`
	TitleMarginBottom float32
	ContentFontSize   float32
	ContentLineHeight float32
	ContentFontWeight int     // zero: regular
	ContentColor      Color   `contrast:"text:@Page"`
	AffixFontSize     float32 // prefix and suffix
}

func deriveStatistic(t *Theme) StatisticToken {
	k := StatisticToken{
		TitleFontSize: t.FontSize, TitleColor: t.TextTertiary, TitleMarginBottom: 4,
		ContentFontSize: 24, ContentLineHeight: 32, ContentColor: t.TextHeading, AffixFontSize: t.FontSizeLG,
	}
	switch t.Flavor {
	case Element:
		k.TitleFontSize, k.ContentFontSize, k.ContentLineHeight = t.FontSizeSM, 20, 28
	case StyleShadcn:
		// Dashboard card numbers: text-2xl font-bold, muted label font-medium.
		k.TitleFontWeight, k.ContentFontWeight = 500, 700
	}
	return k
}
