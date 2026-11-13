package theme

// EmptyToken styles Empty. The default pictures take their colors from
// Theme.Illustration.
type EmptyToken struct {
	Alias
	ImageHeight        float32 // default picture (Element: its illustration)
	ImageHeightSM      float32 // Simple picture
	DescriptionColor   Color   `contrast:"text:@Page"`
	DescriptionColorSM Color   `contrast:"text:@Page"` // Simple
	SimpleFill         Color   // Simple tray
	SimpleLine         Color   // Simple tray outline
	SimpleShadow       Color   // Simple ground ellipse
}

func deriveEmpty(t *Theme) EmptyToken {
	k := EmptyToken{
		ImageHeight: 100, ImageHeightSM: 40, DescriptionColor: t.TextTertiary, DescriptionColorSM: t.TextQuaternary,
		SimpleFill: t.FillQuaternary, SimpleLine: t.Illustration.Line, SimpleShadow: t.FillTertiary,
	}
	if t.Flavor == Element {
		k.ImageHeight = 160
	}
	return k
}
