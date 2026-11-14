package theme

// TagToken styles Tag. The label uses the Alias FontSizeSM (FontSize for
// large tags outside Element).
type TagToken struct {
	Alias
	DefaultBg       Color // tag without a color
	DefaultColor    Color `contrast:"text:DefaultBg"`
	BorderColor     Color // default tag outline (zero: none)
	SolidTextColor  Color // label on solid preset colors
	CloseIconColor  Color // zero: the label color
	FontWeight      int
	Radius          float32
	Height          float32
	HeightSM        float32
	HeightLG        float32
	PaddingInline   float32
	PaddingInlineSM float32
	PaddingInlineLG float32
	// Outlined makes "outlined" the default variant (Element's light
	// effect: tinted with a border); otherwise tags are "filled" (antd 6
	// default, border transparent).
	Outlined bool
	// DisabledBg, DisabledColor and DisabledBorderColor style a disabled
	// tag (antd colorBgContainerDisabled / colorTextDisabled; the border
	// of an outlined disabled tag).
	DisabledBg          Color
	DisabledColor       Color
	DisabledBorderColor Color
	// SolidBg is a solid tag without a color (antd colorBgSolid:
	// colorTextBase at 100%, 95% in dark mode).
	SolidBg Color
	// DestructiveBg is the shadcn destructive badge (Color("destructive")):
	// bg-destructive, dark:bg-destructive/60 like the destructive button.
	DestructiveBg Color
}

func deriveTag(t *Theme) TagToken {
	// antd 6.7 es/tag/style/index.js: defaultBg = colorFillTertiary over
	// colorBgContainer; tagPaddingHorizontal 8 minus the 1px border; line
	// height lineHeightSM × fontSizeSM = 20 plus the border = 22.
	k := TagToken{
		DefaultBg: Over(t.FillTertiary, t.BgContainer), DefaultColor: t.Text, BorderColor: t.Border, SolidTextColor: t.TextOnSolid,
		CloseIconColor: t.TextTertiary, FontWeight: 400, Radius: t.RadiusSM,
		Height: 22, HeightSM: 20, HeightLG: 28, PaddingInline: 7, PaddingInlineSM: 5, PaddingInlineLG: 10,
		DisabledBg: Over(t.BgContainerDisabled, t.BgContainer), DisabledColor: t.TextDisabled, DisabledBorderColor: t.Border,
		DestructiveBg: t.Error.Base,
	}
	solid := t.Text
	solid.A = 255
	if t.Dark {
		solid.A = 242 // 0.95
	}
	k.SolidBg = Over(solid, t.BgContainer)
	switch t.Flavor {
	case Element:
		// el-tag: light effect (tinted, light-8 border) is the default.
		k.DefaultBg, k.DefaultColor, k.BorderColor = t.Primary.Bg, t.Primary.Base, t.Primary.BgHover
		k.CloseIconColor = Color{}
		k.Height, k.HeightLG, k.PaddingInline = 24, 32, 9
		k.Outlined = true
		k.SolidBg = t.Primary.Base // effect="dark" with the default type primary
	case StyleShadcn:
		// new-york-v4 badge.tsx: rounded-full border border-transparent
		// px-2 py-0.5 text-xs font-medium; default variant bg-primary;
		// destructive bg-destructive, dark:bg-destructive/60
		// (= ButtonDangerBg).
		k.DefaultBg, k.DefaultColor, k.BorderColor = t.Primary.Base, t.TextOnPrimary, Color{}
		k.CloseIconColor, k.FontWeight, k.Radius = Color{}, 500, 9999
		k.PaddingInline = 7
		k.DestructiveBg = t.ButtonDangerBg
		k.SolidBg = t.Primary.Base
	}
	return k
}
