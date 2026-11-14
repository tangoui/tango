package theme

// AlertToken styles Alert. Ant and Element tint the box with the status
// palette; shadcn draws a bordered card with a status-colored icon.
type AlertToken struct {
	Alias
	SuccessBg                    Color
	SuccessBorderColor           Color // zero: none
	SuccessIconColor             Color
	InfoBg                       Color
	InfoBorderColor              Color
	InfoIconColor                Color
	WarningBg                    Color
	WarningBorderColor           Color
	WarningIconColor             Color
	ErrorBg                      Color
	ErrorBorderColor             Color
	ErrorIconColor               Color
	TitleColor                   Color   `contrast:"text:InfoBg"` // zero: the status icon color (Element)
	DescriptionColor             Color   `contrast:"text:InfoBg"`
	TitleFontSize                float32 // with a description
	TitleFontWeight              int     // with a description (zero: regular)
	DescriptionFontSize          float32
	DefaultPaddingBlock          float32
	DefaultPaddingInline         float32
	WithDescriptionPaddingBlock  float32
	WithDescriptionPaddingInline float32
	IconSize                     float32
	WithDescriptionIconSize      float32
	IconMarginInlineEnd          float32
	WithDescriptionIconMarginEnd float32
	TitleMarginBottom            float32 // title to description
	Radius                       float32
	// Dark effect (Element effect="dark"): the box is filled with the
	// status color and text and icon are drawn in DarkTextColor (through
	// Theme.TextOn, so a light fill gets dark text).
	SuccessDarkBg Color
	InfoDarkBg    Color
	WarningDarkBg Color
	ErrorDarkBg   Color
	DarkTextColor Color
}

func deriveAlert(t *Theme) AlertToken {
	k := AlertToken{
		SuccessBg: t.Success.Bg, SuccessBorderColor: t.Success.Border, SuccessIconColor: t.Success.Base,
		InfoBg: t.Info.Bg, InfoBorderColor: t.Info.Border, InfoIconColor: t.Info.Base,
		WarningBg: t.Warning.Bg, WarningBorderColor: t.Warning.Border, WarningIconColor: t.Warning.Base,
		ErrorBg: t.Error.Bg, ErrorBorderColor: t.Error.Border, ErrorIconColor: t.Error.Base,
		TitleColor: t.Text, DescriptionColor: t.Text,
		TitleFontSize: t.FontSizeLG, TitleFontWeight: t.FontWeightStrong, DescriptionFontSize: t.FontSize,
		DefaultPaddingBlock: 8, DefaultPaddingInline: 12, WithDescriptionPaddingBlock: 20, WithDescriptionPaddingInline: 24,
		IconSize: 14, WithDescriptionIconSize: 24, IconMarginInlineEnd: 8, WithDescriptionIconMarginEnd: 12,
		TitleMarginBottom: 4, Radius: t.RadiusLG,
		SuccessDarkBg: t.Success.Base, InfoDarkBg: t.Info.Base, WarningDarkBg: t.Warning.Base, ErrorDarkBg: t.Error.Base,
		DarkTextColor: t.TextOnSolid,
	}
	switch t.Flavor {
	case Element:
		k.SuccessBorderColor, k.InfoBorderColor, k.WarningBorderColor, k.ErrorBorderColor = Color{}, Color{}, Color{}, Color{}
		k.TitleColor, k.DescriptionColor = Color{}, t.TextSecondary
		k.TitleFontSize, k.TitleFontWeight, k.DescriptionFontSize = t.FontSize, 700, t.FontSizeSM
		k.DefaultPaddingInline, k.WithDescriptionPaddingBlock, k.WithDescriptionPaddingInline = 16, 8, 16
		k.WithDescriptionIconSize, k.Radius = 28, t.Radius
	case StyleShadcn:
		// A bordered card (rounded-lg px-4 py-3): only the destructive
		// variant tints its border; the info icon takes the foreground.
		k.SuccessBg, k.InfoBg, k.WarningBg, k.ErrorBg = t.BgContainer, t.BgContainer, t.BgContainer, t.BgContainer
		k.SuccessBorderColor, k.InfoBorderColor, k.WarningBorderColor = t.BorderSecondary, t.BorderSecondary, t.BorderSecondary
		k.ErrorBorderColor = Fade(t.Error.Base, .5)
		k.InfoIconColor = t.Text
		k.InfoDarkBg = t.Primary.Base // shadcn's solid neutral fill
		k.DescriptionColor = t.TextTertiary
		k.TitleFontSize, k.TitleFontWeight = t.FontSize, 500
		k.DefaultPaddingBlock, k.DefaultPaddingInline, k.WithDescriptionPaddingBlock, k.WithDescriptionPaddingInline = 12, 16, 12, 16
		k.IconSize, k.WithDescriptionIconSize, k.IconMarginInlineEnd = 16, 16, 12
	}
	return k
}
