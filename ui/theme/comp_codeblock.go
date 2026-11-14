package theme

// CodeBlockToken styles CodeBlock and the code blocks of Markdown: the
// frame, the line-number gutter and the syntax colors a code highlighter's
// token kinds map to (ui.CodeTokenKind).
type CodeBlockToken struct {
	Alias
	Bg              Color
	BorderColor     Color // zero: none
	Radius          float32
	CodeFontSize    float32 // code text
	LineHeight      float32
	PaddingBlock    float32
	PaddingInline   float32
	HeaderColor     Color `contrast:"text:Bg"` // title / language label of the header row
	HeaderBorder    Color // rule under the header row (zero: none)
	GutterColor     Color `contrast:"text:Bg"` // line numbers
	GutterBorder    Color // rule between the gutter and the code (zero: none)
	LineHighlightBg Color // CodeBlockW.HighlightLines
	SelectionBg     Color // selected text
	Text            Color `contrast:"text:Bg"` // plain code
	Keyword         Color `contrast:"text:Bg"`
	String          Color `contrast:"text:Bg"`
	Number          Color `contrast:"text:Bg"`
	Comment         Color `contrast:"text:Bg"`
	Type            Color `contrast:"text:Bg"`
	Function        Color `contrast:"text:Bg"`
	Variable        Color `contrast:"text:Bg"`
	Constant        Color `contrast:"text:Bg"`
	Operator        Color `contrast:"text:Bg"`
	Punctuation     Color `contrast:"text:Bg"`
	Tag             Color `contrast:"text:Bg"`
	Attribute       Color `contrast:"text:Bg"`
	Property        Color `contrast:"text:Bg"`
	Regexp          Color `contrast:"text:Bg"`
	Invalid         Color `contrast:"text:Bg"`
}

// Hues of the syntax colors (Ant Design preset colors; every design
// system derives its shades from them, see SyntaxColor).
var (
	syntaxPurple   = Hex("#722ed1")
	syntaxGreen    = Hex("#52c41a")
	syntaxBlue     = Hex("#2f54eb")
	syntaxCyan     = Hex("#13c2c2")
	syntaxOrange   = Hex("#fa8c16")
	syntaxVolcano  = Hex("#fa541c")
	syntaxRed      = Hex("#f5222d")
	syntaxMagenta  = Hex("#eb2f96")
	syntaxDaybreak = Hex("#1677ff")
	syntaxGold     = Hex("#faad14")
)

// SyntaxColor returns the shade of hue that reads as text on bg (4.5:1
// when any shade does): the hue's own color, then darker shades on light
// backgrounds or lighter ones on dark backgrounds (dark selects Ant's dark
// palette blended onto bg).
func SyntaxColor(hue Color, dark bool, bg Color) Color {
	if bg.A < 255 {
		base := Hex("#ffffff")
		if dark {
			base = Hex("#141414")
		}
		bg = Over(bg, base)
	}
	s := AntShades(hue, dark, bg)
	best := s[5]
	for i := 5; i < len(s); i++ {
		best = s[i]
		if ContrastRatio(s[i], bg) >= 4.5 {
			break
		}
	}
	return best
}

func deriveCodeBlock(t *Theme) CodeBlockToken {
	k := CodeBlockToken{
		Bg: RGBA(150, 150, 150, .1), BorderColor: RGBA(100, 100, 100, .2), Radius: t.RadiusSM + 2,
		CodeFontSize: t.FontSize - 1, LineHeight: t.LineHeightFor(t.FontSize - 1),
		PaddingBlock: 10, PaddingInline: 14,
		HeaderColor: t.TextTertiary, HeaderBorder: RGBA(100, 100, 100, .2),
		GutterColor: t.TextTertiary, SelectionBg: Fade(t.Primary.Base, 0.22),
		LineHighlightBg: Fade(t.Warning.Base, 0.14),
		Text:            t.Text, Comment: t.TextSecondary, Operator: t.Text, Punctuation: t.TextSecondary,
		Variable: t.Text, Invalid: t.Error.Base,
	}
	switch t.Flavor {
	case Element:
		k.Bg, k.BorderColor, k.Radius, k.HeaderBorder = t.FillAlter, t.BorderSecondary, t.Radius, t.BorderSecondary
	case StyleShadcn:
		// bg-muted, no border, rounded-md (the Typography code look).
		k.Bg, k.BorderColor, k.Radius, k.HeaderBorder = t.FillSecondary, Color{}, t.Radius, t.Border
	}
	bg := Over(k.Bg, t.BgContainer)
	k.HeaderColor = readable(k.HeaderColor, t.TextSecondary, bg)
	k.GutterColor = readable(k.GutterColor, t.TextSecondary, bg)
	sc := func(h Color) Color { return SyntaxColor(h, t.Dark, bg) }
	k.Keyword, k.String, k.Number = sc(syntaxPurple), sc(syntaxGreen), sc(syntaxBlue)
	k.Type, k.Function, k.Constant = sc(syntaxCyan), sc(syntaxOrange), sc(syntaxVolcano)
	k.Tag, k.Attribute, k.Property = sc(syntaxRed), sc(syntaxGold), sc(syntaxDaybreak)
	k.Regexp = sc(syntaxMagenta)
	return k
}

// readable returns c when it reads as text on bg (4.5:1), else alt.
func readable(c, alt, bg Color) Color {
	if ContrastRatio(c, bg) >= 4.5 {
		return c
	}
	return alt
}
