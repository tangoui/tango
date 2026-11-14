package theme

// TitleBarToken styles TitleBar (the self-drawn title bar of a frameless
// window) and WindowControls (its minimize / maximize / close buttons).
type TitleBarToken struct {
	Alias
	Height        float32
	Bg            Color // bar background
	BorderColor   Color // rule under the bar (zero: none)
	PaddingInline float32
	Gap           float32 // between icon, menu, center and tools

	IconSize           float32 // app icon
	TitleFontSize      float32
	TitleFontWeight    int   // zero: regular
	TitleColor         Color `contrast:"text:Bg"`
	TitleInactiveColor Color // title while the window is not focused
	// SystemInset is the room left at the bar's start for window buttons
	// the platform draws itself (macOS traffic lights over a hidden title
	// bar).
	SystemInset float32

	// Windows / Linux caption buttons at the bar's end.
	ControlWidth    float32
	ControlIconSize float32
	ControlColor    Color `contrast:"text:Bg"`
	ControlHoverBg  Color
	ControlActiveBg Color
	CloseHoverBg    Color
	CloseHoverColor Color `contrast:"text:CloseHoverBg"`
	CloseActiveBg   Color
	ControlInactive Color // glyphs while the window is not focused

	// macOS-style traffic lights drawn by WindowControls (when the platform
	// does not draw them). The colors are the macOS system values.
	TrafficSize     float32
	TrafficGap      float32
	TrafficInset    float32 // from the bar's start to the first light
	TrafficClose    Color
	TrafficMinimize Color
	TrafficZoom     Color
	TrafficInactive Color // all three while the window is not focused
	TrafficBorder   Color // hairline around each light (zero: none)
	TrafficGlyph    Color // symbols shown while the pointer is over the group
}

func deriveTitleBar(t *Theme) TitleBarToken {
	k := TitleBarToken{
		Height: 36, Bg: t.BgContainer, BorderColor: t.Split, PaddingInline: 8, Gap: 8,
		IconSize: 16, TitleFontSize: t.FontSizeSM + 1, TitleColor: t.Text, TitleInactiveColor: t.TextTertiary,
		SystemInset:  76,
		ControlWidth: 46, ControlIconSize: 12, ControlColor: t.Text,
		ControlHoverBg: t.BgTextHover, ControlActiveBg: t.BgTextActive,
		CloseHoverBg: t.Error.Base, CloseHoverColor: t.TextOnSolid, CloseActiveBg: t.Error.Active,
		ControlInactive: t.TextTertiary,
		TrafficSize:     12, TrafficGap: 8, TrafficInset: 12,
		TrafficClose:    Color{R: 0xff, G: 0x5f, B: 0x57, A: 0xff},
		TrafficMinimize: Color{R: 0xfe, G: 0xbc, B: 0x2e, A: 0xff},
		TrafficZoom:     Color{R: 0x28, G: 0xc8, B: 0x40, A: 0xff},
		TrafficInactive: t.Fill, TrafficBorder: Fade(t.Text, .12),
		TrafficGlyph: Color{R: 0, G: 0, B: 0, A: 0x8c},
	}
	switch t.Flavor {
	case Element:
		k.Bg, k.BorderColor = t.BgContainer, t.BorderSecondary
		k.TitleFontWeight = 500
	case StyleShadcn:
		// Title area of a shadcn app shell: bg-background with border-b,
		// muted-foreground glyphs that turn foreground on the accent hover.
		k.BorderColor = t.Border
		k.TitleFontWeight = 500
		k.ControlColor = t.TextSecondary
		k.ControlHoverBg, k.ControlActiveBg = t.ItemHoverBg, t.FillSecondary
		k.ControlIconSize = 14
	}
	return k
}
