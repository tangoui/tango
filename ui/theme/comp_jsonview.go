package theme

// JSONViewToken styles JSONView: the rows of the value tree, the value
// colors by type and the search highlight.
type JSONViewToken struct {
	Alias
	RowHeight          float32
	Indent             float32 // per nesting level
	ValueFontSize      float32
	KeyColor           Color `contrast:"text:@Page"` // object keys and array indexes
	StringColor        Color `contrast:"text:@Page"`
	NumberColor        Color `contrast:"text:@Page"`
	BoolColor          Color `contrast:"text:@Page"`
	NullColor          Color `contrast:"text:@Page"`
	OtherColor         Color `contrast:"text:@Page"` // values of ValueOther (functions, pointers …)
	MetaColor          Color `contrast:"text:@Page"` // item counts, types, group ranges
	PunctColor         Color `contrast:"text:@Page"` // ":" and brackets
	SwitcherColor      Color `contrast:"nontext:@Page"`
	RowHoverBg         Color
	RowSelectedBg      Color
	MatchBg            Color   // search match
	MatchCurrentBg     Color   // the match the navigation is on
	MatchCurrentBorder Color   // outline of the current match
	Radius             float32 // row highlight
}

func deriveJSONView(t *Theme) JSONViewToken {
	bg := t.BgContainer
	sc := func(h Color) Color { return SyntaxColor(h, t.Dark, bg) }
	k := JSONViewToken{
		RowHeight: 24, Indent: 16, ValueFontSize: t.FontSize - 1,
		KeyColor: sc(syntaxPurple), StringColor: sc(syntaxGreen), NumberColor: sc(syntaxBlue),
		BoolColor: sc(syntaxVolcano), NullColor: t.TextSecondary, OtherColor: sc(syntaxCyan),
		MetaColor: readable(t.TextTertiary, t.TextSecondary, bg), PunctColor: t.TextSecondary,
		SwitcherColor: t.TextSecondary,
		RowHoverBg:    t.FillTertiary, RowSelectedBg: t.Primary.Bg,
		MatchBg: Fade(t.Warning.Base, 0.3), MatchCurrentBg: Fade(t.Warning.Base, 0.4), MatchCurrentBorder: t.Warning.Base,
		Radius: t.RadiusSM,
	}
	switch t.Flavor {
	case Element:
		k.RowHoverBg, k.RowSelectedBg, k.Radius = t.FillAlter, t.Primary.Bg, 0
	case StyleShadcn:
		k.RowHoverBg, k.RowSelectedBg, k.Radius, k.RowHeight = t.FillSecondary, t.FillSecondary, t.RadiusSM, 28
	}
	return k
}
