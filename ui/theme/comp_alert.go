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
	DescriptionColor             Color   `contrast:"text:InfoBg"` // zero: the status icon color (Element)
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
	// WithDescriptionIconCenter centers the big icon vertically next to
	// title and description (Element: el-alert is align-items: center);
	// false keeps it at the top (antd).
	WithDescriptionIconCenter bool
	// CloseColor and CloseHoverColor color the close icon (antd
	// colorIcon / colorIconHover); CloseIconSize is its size and
	// CloseFontSize the size of a CloseText.
	CloseColor      Color
	CloseHoverColor Color
	CloseIconSize   float32
	CloseFontSize   float32
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
		TitleColor: t.TextHeading, DescriptionColor: t.Text,
		TitleFontSize: t.FontSizeLG, TitleFontWeight: 0, DescriptionFontSize: t.FontSize,
		DefaultPaddingBlock: 8, DefaultPaddingInline: 12, WithDescriptionPaddingBlock: 20, WithDescriptionPaddingInline: 24,
		IconSize: 14, WithDescriptionIconSize: 24, IconMarginInlineEnd: 8, WithDescriptionIconMarginEnd: 12,
		TitleMarginBottom: 8, Radius: t.RadiusLG, // antd &-with-description title: marginBottom marginXS, no fontWeight
		SuccessDarkBg: t.Success.Base, InfoDarkBg: t.Info.Base, WarningDarkBg: t.Warning.Base, ErrorDarkBg: t.Error.Base,
		DarkTextColor: t.TextOnSolid,
		CloseColor:    t.TextTertiary, CloseHoverColor: t.Text, CloseIconSize: 12, CloseFontSize: 12,
	}
	switch t.Flavor {
	case Element:
		k.SuccessBorderColor, k.InfoBorderColor, k.WarningBorderColor, k.ErrorBorderColor = Color{}, Color{}, Color{}, Color{}
		// theme-chalk/src/alert.scss + common/var.scss $alert: title and
		// description in the status color, title 16px (14px alone),
		// description 14px, content gap 4px, padding 8px 16px, icon
		// 16px / 28px, close 16px in the placeholder color.
		k.TitleColor, k.DescriptionColor = Color{}, Color{}
		k.TitleFontSize, k.TitleFontWeight, k.DescriptionFontSize = 16, 0, 14
		k.DefaultPaddingInline, k.WithDescriptionPaddingBlock, k.WithDescriptionPaddingInline = 16, 8, 16
		k.IconSize, k.WithDescriptionIconSize, k.Radius = 16, 28, t.Radius
		k.TitleMarginBottom = 4 // __content gap 4px
		k.WithDescriptionIconCenter = true
		k.CloseColor, k.CloseHoverColor, k.CloseIconSize, k.CloseFontSize = t.TextQuaternary, t.TextQuaternary, 16, 14
	case StyleShadcn:
		// A bordered card (rounded-lg px-4 py-3): only the destructive
		// variant tints its border; the info icon takes the foreground.
		// alert.tsx (v4): bg-card, border (the destructive variant only
		// recolors text and icon), gap-y-0.5 between title and
		// description.
		card := t.BgContainer
		if t.Shadcn != nil {
			card = t.Shadcn.Card
		}
		k.SuccessBg, k.InfoBg, k.WarningBg, k.ErrorBg = card, card, card, card
		k.SuccessBorderColor, k.InfoBorderColor, k.WarningBorderColor = t.BorderSecondary, t.BorderSecondary, t.BorderSecondary
		k.ErrorBorderColor = t.BorderSecondary
		k.InfoIconColor = t.Text
		k.InfoDarkBg = t.Primary.Base // shadcn's solid neutral fill
		k.DescriptionColor = t.TextTertiary
		k.TitleFontSize, k.TitleFontWeight = t.FontSize, 500
		k.DefaultPaddingBlock, k.DefaultPaddingInline, k.WithDescriptionPaddingBlock, k.WithDescriptionPaddingInline = 12, 16, 12, 16
		k.IconSize, k.WithDescriptionIconSize, k.IconMarginInlineEnd = 16, 16, 12
		k.TitleMarginBottom = 2
		k.CloseIconSize, k.CloseFontSize = 16, 14
	}
	return k
}
