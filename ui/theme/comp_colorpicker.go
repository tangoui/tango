package theme

// ColorPickerToken styles ColorPicker and ColorPickerPanel (the trigger
// frame uses the Input frame).
type ColorPickerToken struct {
	Alias
	PanelWidth         float32
	PanelPadding       float32
	PaletteHeight      float32 // saturation / value square
	PaletteRadius      float32
	BarHeight          float32 // hue / alpha slider box (Element: the bar itself)
	RailHeight         float32 // painted rail inside the slider box
	HandleSize         float32 // round handle (Ant / shadcn; Element draws a bar thumb)
	HandleColor        Color   // its ring
	HandleShadow       Color   // its outer edge
	SwatchSize         float32 // preset swatches
	SwatchGap          float32
	SwatchRadius       float32
	SwatchBorder       Color // zero: none
	SwatchActiveBorder Color // ring around the selected preset (Element)
	SwatchHoverBorder  Color
	BorderColor        Color // outline of the current-color swatch and the clear button
	ClearColor         Color // slash of the "no color" swatch

	// Trigger.
	TriggerSwatchRadius float32
	TriggerSwatchBorder Color

	// Gradient stop bar (ColorPickerGradient).
	GradientBarHeight  float32 // the slider box; its rail is RailHeight (Element: the whole box)
	GradientActiveRing Color   // ring around the selected stop's handle
}

func deriveColorPicker(t *Theme) ColorPickerToken {
	k := ColorPickerToken{
		PanelWidth: 234 + 24, PanelPadding: 12,
		PaletteHeight: 160, PaletteRadius: t.RadiusSM,
		BarHeight: 12, RailHeight: 8,
		HandleSize: 16, HandleColor: Hex("#ffffff"), HandleShadow: RGBA(0, 0, 0, .06),
		SwatchSize: 24, SwatchGap: 6, SwatchRadius: t.RadiusSM, SwatchBorder: t.Split,
		SwatchActiveBorder: t.Primary.Base, SwatchHoverBorder: RGBA(0, 0, 0, .15),
		BorderColor: t.Split, ClearColor: t.Error.Base,
		TriggerSwatchRadius: t.RadiusSM, TriggerSwatchBorder: t.Split,
		GradientBarHeight: 20, GradientActiveRing: t.Primary.Base,
	}
	if t.Flavor == Element {
		// el-color-picker: square palette, 12px bars with a white thumb,
		// ringed presets, a 2px-rounded trigger swatch.
		k.PanelWidth, k.PanelPadding = 312, 6
		k.PaletteHeight, k.PaletteRadius = 180, 0
		k.RailHeight = 12
		k.SwatchSize, k.SwatchGap, k.SwatchRadius = 20, 8, 4
		k.SwatchBorder = Color{}
		k.TriggerSwatchRadius, k.TriggerSwatchBorder = 2, t.TextTertiary
	}
	return k
}
