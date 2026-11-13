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
	MediaBg            Color   // shadcn icon tile (zero: the picture is an illustration)
	MediaColor         Color   `contrast:"nontext:MediaBg"` // its icon
	MediaRadius        float32
}

func deriveEmpty(t *Theme) EmptyToken {
	k := EmptyToken{
		ImageHeight: 100, ImageHeightSM: 40, DescriptionColor: t.TextTertiary, DescriptionColorSM: t.TextQuaternary,
		SimpleFill: t.FillQuaternary, SimpleLine: t.Illustration.Line, SimpleShadow: t.FillTertiary,
	}
	switch t.Flavor {
	case Element:
		k.ImageHeight = 160
	case StyleShadcn:
		// shadcn Empty: EmptyMedia variant="icon" is a size-10 rounded-lg
		// bg-muted tile with a foreground icon instead of an illustration.
		k.ImageHeight = 40
		k.MediaBg, k.MediaColor, k.MediaRadius = t.FillSecondary, t.Text, t.RadiusLG
	}
	return k
}
