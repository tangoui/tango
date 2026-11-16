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

	// ControlsPosition("sides") / Mode("spinner"): − and + buttons at
	// both ends. SideHandleBg zero: transparent (antd spinner actions).
	SideHandleBg         Color
	SideHandleColor      Color `contrast:"nontext:SideHandleBg"`
	SideHandleHoverColor Color
	SideHandleFontSize   float32
	// SideHandleWidth* is the width of each side button per size; zero:
	// as wide as the control is high (Element).
	SideHandleWidth   float32
	SideHandleWidthSM float32
	SideHandleWidthLG float32

	// HideControlsInactive is 1 when a disabled or read-only field leaves
	// out its step buttons (antd), 0 when it keeps them, inactive
	// (Element). An int, so it is also a CSS variable.
	HideControlsInactive int
}

func deriveInputNumber(t *Theme) InputNumberToken {
	k := InputNumberToken{
		ControlWidth: 90,
		HandleWidth:  22, HandleFontSize: 10,
		HandleColor: t.TextTertiary, HandleHoverColor: t.Primary.Hover, HandleDisabledColor: t.TextDisabled,
		HandleBorderColor: t.Border,
		SideHandleBg:      t.FillTertiary, SideHandleColor: t.TextSecondary, SideHandleHoverColor: t.Primary.Base,
		SideHandleFontSize: 14,
		// antd/input-number/index.js: mergedControls = controls &&
		// !mergedDisabled && !readOnly.
		HideControlsInactive: 1,
	}
	if t.Flavor == Ant {
		// antd 6 mode="spinner" (es/input-number/style/index.js "Spinner
		// Mode"): transparent actions in colorText, colorPrimary on hover;
		// each is the inherited font size wide plus paddingInline on both
		// sides and a 1px divider: 14+2*11+1, small 14+2*7+1, large
		// 16+2*11+1.
		k.SideHandleBg = Color{}
		k.SideHandleColor = t.Text
		k.SideHandleWidth = t.FontSize + 2*t.ControlPaddingH + t.LineWidth
		k.SideHandleWidthSM = t.FontSize + 2*t.ControlPaddingHSM + t.LineWidth
		k.SideHandleWidthLG = t.FontSizeLG + 2*t.ControlPaddingH + t.LineWidth
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
		// el-input-number__increase / __decrease: font-size 13px.
		k.SideHandleFontSize = 13
		// el-input-number keeps − / + when disabled or readonly
		// (is-disabled, no response).
		k.HideControlsInactive = 0
	case StyleShadcn:
		// Always visible lucide chevrons that turn foreground on an
		// accent background under the pointer.
		k.HandleFontSize = 12
		k.HandleHoverColor, k.HandleHoverBg = t.Text, t.ItemHoverBg
	}
	return k
}
