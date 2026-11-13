package theme

// InputNumberToken styles InputNumber (its frame, text and placeholder
// use the Input token).
type InputNumberToken struct {
	Alias
	ControlWidth        float32 // default width
	HandleWidth         float32 // stacked step buttons at the inline end
	HandleFontSize      float32 // their arrow icons
	HandleColor         Color   `contrast:"nontext:@Page"`
	HandleHoverColor    Color
	HandleHoverBg       Color // zero: none
	HandleDisabledColor Color // at a limit or disabled
	HandleBorderColor   Color // dividers of the step buttons

	// ControlsPosition("sides"): − and + buttons at both ends.
	SideHandleBg         Color
	SideHandleColor      Color `contrast:"nontext:SideHandleBg"`
	SideHandleHoverColor Color
	SideHandleFontSize   float32
}

func deriveInputNumber(t *Theme) InputNumberToken {
	k := InputNumberToken{
		ControlWidth: 90,
		HandleWidth:  22, HandleFontSize: 10,
		HandleColor: t.TextTertiary, HandleHoverColor: t.Primary.Hover, HandleDisabledColor: t.TextDisabled,
		HandleBorderColor: t.Border,
		SideHandleBg:      t.FillTertiary, SideHandleColor: t.TextSecondary, SideHandleHoverColor: t.Primary.Base,
		SideHandleFontSize: 14,
	}
	if t.Flavor == StyleShadcn {
		// Always visible lucide chevrons that turn foreground on an
		// accent background under the pointer.
		k.HandleFontSize = 12
		k.HandleHoverColor, k.HandleHoverBg = t.Text, t.ItemHoverBg
	}
	return k
}
