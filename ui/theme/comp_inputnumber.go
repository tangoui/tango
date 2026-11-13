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
	HandleBg            Color // resting background (zero: none)
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
	switch t.Flavor {
	case Element:
		// el-input-number: 150px wide ($input-number-width), step buttons
		// as wide as the control is high (32px). Its default puts them at
		// the sides (InputNumber's ControlsPosition default). Stacked at
		// the right they are always shown, on --el-fill-color-light in
		// --el-text-color-regular, primary on hover (input-number.scss).
		k.ControlWidth, k.HandleWidth = 150, 32
		k.HandleBg, k.HandleColor, k.HandleHoverColor = t.FillTertiary, t.TextSecondary, t.Primary.Base
	case StyleShadcn:
		// Always visible lucide chevrons that turn foreground on an
		// accent background under the pointer.
		k.HandleFontSize = 12
		k.HandleHoverColor, k.HandleHoverBg = t.Text, t.ItemHoverBg
	}
	return k
}
