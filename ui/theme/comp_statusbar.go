package theme

// StatusBarToken styles StatusBar and its StatusBarItem entries: a compact
// strip at the bottom of a workbench window.
type StatusBarToken struct {
	Alias
	Height        float32
	Bg            Color // bar background
	BorderColor   Color // rule above the bar (zero: none)
	PaddingInline float32
	Gap           float32 // between items of one side

	ItemPaddingInline float32
	ItemGap           float32 // icon, text, badge and progress inside an item
	ItemRadius        float32
	ItemFontSize      float32
	ItemColor         Color `contrast:"text:Bg"`
	ItemHoverBg       Color // zero: none
	ItemHoverColor    Color `contrast:"text:Bg"`
	ItemActiveBg      Color // pressed item (zero: none)
	IconSize          float32

	BadgeHeight   float32
	BadgeFontSize float32
	BadgeBg       Color
	BadgeColor    Color `contrast:"text:BadgeBg"`

	ProgressWidth  float32 // the bar of a determinate progress item
	ProgressHeight float32
	ProgressTrack  Color
	ProgressColor  Color `contrast:"nontext:ProgressTrack"`
}

func deriveStatusBar(t *Theme) StatusBarToken {
	k := StatusBarToken{
		Height: 24, Bg: t.BgLayout, BorderColor: t.Split, PaddingInline: 4, Gap: 0,
		ItemPaddingInline: 6, ItemGap: 4, ItemRadius: 0, ItemFontSize: t.FontSizeSM,
		ItemColor: t.TextSecondary, ItemHoverBg: t.BgTextHover, ItemHoverColor: t.Text,
		ItemActiveBg: t.BgTextActive, IconSize: 14,
		BadgeHeight: 16, BadgeFontSize: 11, BadgeBg: t.Primary.Base, BadgeColor: t.TextOnPrimary,
		ProgressWidth: 48, ProgressHeight: 4, ProgressTrack: t.FillSecondary, ProgressColor: t.Primary.Base,
	}
	switch t.Flavor {
	case Element:
		// Element: el-footer neutrals, a flat strip on the light fill.
		k.Bg, k.BorderColor = t.FillAlter, t.BorderSecondary
		k.ItemHoverColor = t.Primary.Base
	case StyleShadcn:
		// A muted sidebar-like strip: bg-muted/40 border-t, items rounded-sm
		// with the accent hover of ghost buttons.
		k.Bg, k.BorderColor = t.BgLayout, t.Border
		k.PaddingInline, k.Gap = 6, 2
		k.ItemRadius, k.ItemPaddingInline = t.RadiusSM, 6
		k.ItemHoverBg, k.ItemHoverColor = t.ItemHoverBg, t.ItemSelected
		k.BadgeBg, k.BadgeColor = t.Primary.Base, t.TextOnPrimary
	}
	return k
}
