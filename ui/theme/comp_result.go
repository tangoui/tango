package theme

// ResultToken styles Result.
type ResultToken struct {
	Alias
	IconFontSize       float32 // status icon
	IconMarginBottom   float32
	SuccessIconColor   Color
	InfoIconColor      Color
	WarningIconColor   Color
	ErrorIconColor     Color
	IconGlyphColor     Color // mark inside the status circle
	TitleFontSize      float32
	TitleLineHeight    float32
	TitleColor         Color `contrast:"text:@Page"`
	SubtitleFontSize   float32
	SubtitleColor      Color `contrast:"text:@Page"`
	SubtitleMarginTop  float32
	SubtitleLineHeight float32 // zero: the theme line height
	ExtraMargin        float32 // above the actions
	PaddingBlock       float32
	PaddingInline      float32
	ContentBg          Color // Body panel (zero: none)
	ContentRadius      float32
}

func deriveResult(t *Theme) ResultToken {
	k := ResultToken{
		IconFontSize: 72, IconMarginBottom: 24,
		SuccessIconColor: t.Success.Base, InfoIconColor: t.Primary.Base, WarningIconColor: t.Warning.Base, ErrorIconColor: t.Error.Base,
		IconGlyphColor: t.BgContainer,
		TitleFontSize:  24, TitleLineHeight: 32, TitleColor: t.TextHeading,
		// antd es/result/style/index.js: title marginBlock marginXS 8 (block
		// margins collapse with the icon's 24 and the extra's 24), body
		// colorFillAlter without a radius.
		SubtitleFontSize: t.FontSize, SubtitleColor: t.TextTertiary, SubtitleMarginTop: 8,
		ExtraMargin: 24, PaddingBlock: 48, PaddingInline: 32, ContentBg: t.FillAlter, ContentRadius: 0,
	}
	switch t.Flavor {
	case Element:
		k.IconFontSize, k.IconMarginBottom, k.InfoIconColor = 64, 20, t.Info.Base
		// theme-chalk/src/result.scss: title and subtitle line-height 1.3.
		k.TitleFontSize, k.TitleLineHeight, k.SubtitleLineHeight = 20, 26, 18.2
		k.SubtitleColor, k.SubtitleMarginTop, k.ExtraMargin = t.TextSecondary, 10, 30
		k.PaddingBlock, k.PaddingInline, k.ContentBg = 40, 30, Color{}
	case StyleShadcn:
		k.InfoIconColor, k.IconGlyphColor = t.Info.Base, t.TextOnSolid // white marks in both modes
	}
	return k
}
