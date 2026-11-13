package theme

// ResultToken styles Result.
type ResultToken struct {
	Alias
	IconFontSize      float32 // status icon
	IconMarginBottom  float32
	SuccessIconColor  Color
	InfoIconColor     Color
	WarningIconColor  Color
	ErrorIconColor    Color
	IconGlyphColor    Color // mark inside the status circle
	TitleFontSize     float32
	TitleLineHeight   float32
	TitleColor        Color `contrast:"text:@Page"`
	SubtitleFontSize  float32
	SubtitleColor     Color `contrast:"text:@Page"`
	SubtitleMarginTop float32
	ExtraMargin       float32 // above the actions
	PaddingBlock      float32
	PaddingInline     float32
	ContentBg         Color // Body panel (zero: none)
	ContentRadius     float32
}

func deriveResult(t *Theme) ResultToken {
	k := ResultToken{
		IconFontSize: 72, IconMarginBottom: 24,
		SuccessIconColor: t.Success.Base, InfoIconColor: t.Primary.Base, WarningIconColor: t.Warning.Base, ErrorIconColor: t.Error.Base,
		IconGlyphColor: t.BgContainer,
		TitleFontSize:  24, TitleLineHeight: 32, TitleColor: t.TextHeading,
		SubtitleFontSize: t.FontSize, SubtitleColor: t.TextTertiary, SubtitleMarginTop: 4,
		ExtraMargin: 24, PaddingBlock: 48, PaddingInline: 32, ContentBg: t.FillAlter, ContentRadius: t.RadiusSM,
	}
	switch t.Flavor {
	case Element:
		k.IconFontSize, k.IconMarginBottom, k.InfoIconColor = 64, 20, t.Info.Base
		k.TitleFontSize, k.TitleLineHeight = 20, 28
		k.SubtitleColor, k.SubtitleMarginTop, k.ExtraMargin = t.TextSecondary, 10, 30
		k.PaddingBlock, k.PaddingInline, k.ContentBg = 40, 30, Color{}
	case StyleShadcn:
		k.InfoIconColor, k.IconGlyphColor = t.Info.Base, t.TextOnSolid // white marks in both modes
	}
	return k
}
