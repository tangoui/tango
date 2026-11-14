package theme

// RadioToken styles Radio and the button-style radio group.
type RadioToken struct {
	Alias
	RadioSize float32
	// DotSize is the diameter of the centre dot of a checked radio.
	DotSize float32
	// DotColor marks a checked radio: the dot (shadcn) or the filled
	// circle around a background-colored dot (Ant, Element).
	DotColor                Color `contrast:"nontext:@Page"`
	BorderColor             Color `contrast:"nontext:@Page"`
	ButtonBg                Color
	ButtonCheckedBg         Color
	ButtonColor             Color `contrast:"text:ButtonBg"`
	ButtonCheckedColor      Color `contrast:"text:ButtonCheckedBg"`
	ButtonPaddingInline     float32
	ButtonPaddingInlineSM   float32
	ButtonSolidCheckedBg    Color
	ButtonSolidCheckedColor Color `contrast:"text:ButtonSolidCheckedBg"`

	// Bordered radio (Border): inline padding before the circle and after
	// the label, and the frame color of a checked one.
	BorderedPaddingStart float32
	BorderedPaddingEnd   float32
	BorderedCheckedColor Color `contrast:"nontext:@Page"`
}

func deriveRadio(t *Theme) RadioToken {
	k := RadioToken{
		RadioSize: t.CheckboxSize, DotSize: t.CheckboxSize / 2, DotColor: t.Primary.Base, BorderColor: t.Border,
		ButtonBg: t.BgContainer, ButtonCheckedBg: t.BgContainer, ButtonColor: t.Text, ButtonCheckedColor: t.Primary.Base,
		ButtonPaddingInline: 15, ButtonPaddingInlineSM: 7,
		ButtonSolidCheckedBg: t.Primary.Base, ButtonSolidCheckedColor: t.TextOnPrimary,
		BorderedPaddingStart: 9, BorderedPaddingEnd: 15, BorderedCheckedColor: t.Primary.Base,
	}
	switch t.Flavor {
	case Element:
		// el-radio: a small dot on the brand fill; buttons fill when checked.
		k.DotSize = 4
		k.ButtonColor = t.TextSecondary
		k.ButtonCheckedBg, k.ButtonCheckedColor = t.Primary.Base, t.TextOnPrimary
	case StyleShadcn:
		k.DotSize = 8
		k.BorderedPaddingStart, k.BorderedPaddingEnd = 12, 12
		k.ButtonCheckedBg, k.ButtonCheckedColor = t.ItemSelectedBg, t.ItemSelected
	}
	return k
}
