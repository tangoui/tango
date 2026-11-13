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
}

func deriveTag(t *Theme) TagToken {
	k := TagToken{
		DefaultBg: t.FillQuaternary, DefaultColor: t.Text, BorderColor: t.Border, SolidTextColor: t.TextOnSolid,
		CloseIconColor: t.TextTertiary, FontWeight: 400, Radius: t.RadiusSM,
		Height: 22, HeightSM: 20, HeightLG: 28, PaddingInline: 7, PaddingInlineSM: 5, PaddingInlineLG: 10,
	}
	switch t.Flavor {
	case Element:
		k.DefaultBg, k.DefaultColor, k.BorderColor = t.Primary.Bg, t.Primary.Base, t.Primary.BgHover
		k.CloseIconColor = Color{}
		k.Height, k.HeightLG, k.PaddingInline = 24, 32, 9
	case StyleShadcn:
		// shadcn Badge (default variant): primary fill, text-xs
		// font-semibold, rounded-md, px-2.5 minus the 1px border.
		k.DefaultBg, k.DefaultColor, k.BorderColor = t.Primary.Base, t.TextOnPrimary, Color{}
		k.CloseIconColor, k.FontWeight, k.Radius = Color{}, 600, t.Radius
		k.PaddingInline = 9
	}
	return k
}
