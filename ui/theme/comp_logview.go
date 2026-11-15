package theme

// LogViewToken styles LogView: the frame, the monospace lines, the level
// colors of lines without colors of their own, the search highlight and
// the 16 ANSI colors (SGR 30–37 / 90–97). The ANSI colors are derived for
// the background, so they read on light and dark themes alike.
type LogViewToken struct {
	Alias
	Bg                 Color
	BorderColor        Color // zero: none
	Radius             float32
	LineFontSize       float32
	LineHeight         float32
	PaddingInline      float32
	Text               Color `contrast:"text:Bg"`
	GutterColor        Color `contrast:"text:Bg"` // line numbers
	ErrorColor         Color `contrast:"text:Bg"` // lines of level error / fatal
	WarnColor          Color `contrast:"text:Bg"`
	DebugColor         Color `contrast:"text:Bg"` // debug / trace lines
	SelectionBg        Color
	MatchBg            Color
	MatchCurrentBg     Color
	MatchCurrentBorder Color // outline of the current match
	ToolbarGap         float32
	Black              Color
	Red                Color
	Green              Color
	Yellow             Color
	Blue               Color
	Magenta            Color
	Cyan               Color
	White              Color
	BrightBlack        Color
	BrightRed          Color
	BrightGreen        Color
	BrightYellow       Color
	BrightBlue         Color
	BrightMagenta      Color
	BrightCyan         Color
	BrightWhite        Color
}

func deriveLogView(t *Theme) LogViewToken {
	k := LogViewToken{
		Bg: t.BgContainer, BorderColor: t.BorderSecondary, Radius: t.RadiusSM,
		LineFontSize: 12, LineHeight: 18, PaddingInline: 8,
		Text: t.Text, SelectionBg: Fade(t.Primary.Base, 0.22),
		MatchBg: Fade(t.Warning.Base, 0.3), MatchCurrentBg: Fade(t.Warning.Base, 0.4), MatchCurrentBorder: t.Warning.Base,
		ToolbarGap: 8,
	}
	switch t.Flavor {
	case Element:
		k.Radius = t.Radius
	case StyleShadcn:
		k.BorderColor, k.Radius = t.Border, t.Radius
	}
	bg := Over(k.Bg, t.BgContainer)
	k.GutterColor = readable(t.TextTertiary, t.TextSecondary, bg)
	k.DebugColor = t.TextSecondary
	sc := func(h Color) Color { return SyntaxColor(h, t.Dark, bg) }
	k.ErrorColor, k.WarnColor = sc(t.Error.Base), sc(syntaxGold)
	// Normal colors read as text; bright ones are a step toward the
	// background's opposite (lighter on dark, the pure hue on light).
	bright := func(h Color) Color {
		s := AntShades(h, t.Dark, bg)
		if t.Dark {
			return s[7]
		}
		return s[5]
	}
	k.Red, k.BrightRed = sc(syntaxRed), bright(syntaxRed)
	k.Green, k.BrightGreen = sc(syntaxGreen), bright(syntaxGreen)
	k.Yellow, k.BrightYellow = sc(syntaxGold), bright(syntaxGold)
	k.Blue, k.BrightBlue = sc(syntaxDaybreak), bright(syntaxDaybreak)
	k.Magenta, k.BrightMagenta = sc(syntaxMagenta), bright(syntaxMagenta)
	k.Cyan, k.BrightCyan = sc(syntaxCyan), bright(syntaxCyan)
	if t.Dark {
		k.Black, k.BrightBlack = Hex("#3a3a3a"), Over(t.TextTertiary, bg)
		k.White, k.BrightWhite = Over(t.TextSecondary, bg), Hex("#ffffff")
	} else {
		k.Black, k.BrightBlack = Hex("#1f1f1f"), Over(t.TextSecondary, bg)
		k.White, k.BrightWhite = Over(t.TextTertiary, bg), Over(t.TextQuaternary, bg)
	}
	return k
}
