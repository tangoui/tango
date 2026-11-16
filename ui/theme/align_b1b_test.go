package theme

import "testing"

// Spec alignment batch 1, group B: component token values compared with
// the reference sources (2026-11-14). Each expected value is the source's.

// TestAlignB1bAntTokens: antd 6.7.0 component tokens of the input group.
func TestAlignB1bAntTokens(t *testing.T) {
	for _, dark := range []bool{false, true} {
		var th *Theme
		if dark {
			th = AntDesign(Dark())
		} else {
			th = AntDesign()
		}
		c := th.Components
		check := func(name string, got, want float32) {
			t.Helper()
			if got != want {
				t.Errorf("dark=%v %s = %v, want %v", dark, name, got, want)
			}
		}
		// es/input/style/token.js initComponentToken: paddingInline
		// paddingSM-lineWidth, SM controlPaddingHorizontalSM-lineWidth, LG
		// controlPaddingHorizontal-lineWidth; inputFontSize fontSize,
		// inputFontSizeLG fontSizeLG.
		check("Input.PaddingInline", c.Input.PaddingInline, 11)
		check("Input.PaddingInlineSM", c.Input.PaddingInlineSM, 7)
		check("Input.PaddingInlineLG", c.Input.PaddingInlineLG, 11)
		check("Input.InputFontSize", c.Input.InputFontSize, 14)
		check("Input.InputFontSizeLG", c.Input.InputFontSizeLG, 16)
		if c.Input.ActiveBorderColor != th.Primary.Base || c.Input.HoverBorderColor != th.Primary.Hover {
			t.Errorf("dark=%v Input active / hover border: colorPrimary / colorPrimaryHover", dark)
		}
		// es/input-number/style/token.js: controlWidth 90, handleWidth
		// controlHeightSM - 2*lineWidth.
		check("InputNumber.ControlWidth", c.InputNumber.ControlWidth, 90)
		check("InputNumber.HandleWidth", c.InputNumber.HandleWidth, 24-2)
		// es/input-number/style/index.js "Spinner Mode": transparent actions
		// in colorText, paddingInline on both sides around the 14px icon plus
		// the 1px divider.
		check("InputNumber.SideHandleWidth", c.InputNumber.SideHandleWidth, 14+2*11+1)
		check("InputNumber.SideHandleWidthSM", c.InputNumber.SideHandleWidthSM, 14+2*7+1)
		check("InputNumber.SideHandleWidthLG", c.InputNumber.SideHandleWidthLG, 16+2*11+1)
		if c.InputNumber.SideHandleBg.A != 0 || c.InputNumber.SideHandleColor != th.Text || c.InputNumber.SideHandleHoverColor != th.Primary.Base {
			t.Errorf("dark=%v spinner actions: bg %v color %v hover %v", dark, c.InputNumber.SideHandleBg, c.InputNumber.SideHandleColor, c.InputNumber.SideHandleHoverColor)
		}
		// es/slider/style/index.js prepareComponentToken.
		check("Slider.RailSize", c.Slider.RailSize, 4)
		check("Slider.HandleSize", c.Slider.HandleSize, 40/4)
		check("Slider.HandleSizeHover", c.Slider.HandleSizeHover, 24/2)
		check("Slider.HandleLineWidth", c.Slider.HandleLineWidth, 1+1)
		check("Slider.HandleLineWidthHover", c.Slider.HandleLineWidthHover, 1+1*1.5)
		check("Slider.HandleActiveOutlineWidth", c.Slider.HandleActiveOutlineWidth, 6)
		check("Slider.DotSize", c.Slider.DotSize, 8)
		if c.Slider.TrackBgDisabled != th.BgContainerDisabled {
			t.Errorf("dark=%v Slider.TrackBgDisabled %v, want colorBgContainerDisabled %v", dark, c.Slider.TrackBgDisabled, th.BgContainerDisabled)
		}
		if c.Slider.HandleActiveOutlineColor != Fade(th.Primary.Base, .2) || c.Slider.RailBg != th.FillTertiary || c.Slider.TrackBg != th.Primary.Border {
			t.Errorf("dark=%v slider colors %+v", dark, c.Slider)
		}
		// es/switch/style/index.js: trackHeight fontSize*lineHeight (22),
		// trackHeightSM controlHeight/2, padding 2, handleSize
		// trackHeight-4, trackMinWidth handleSize*2+padding*4,
		// trackMinWidthSM handleSizeSM*2+padding*2.
		check("Switch.TrackHeight", c.Switch.TrackHeight, 22)
		check("Switch.TrackHeightSM", c.Switch.TrackHeightSM, 32/2)
		check("Switch.HandleSize", c.Switch.HandleSize, 22-4)
		check("Switch.HandleSizeSM", c.Switch.HandleSizeSM, 16-4)
		check("Switch.TrackMinWidth", c.Switch.TrackMinWidth, 18*2+2*4)
		check("Switch.TrackMinWidthSM", c.Switch.TrackMinWidthSM, 12*2+2*2)
		check("Switch.TrackPadding", c.Switch.TrackPadding, 2)
		// es/radio/style/index.js prepareComponentToken: radioSize
		// fontSizeLG, dotSize radioSize-(4+lineWidth)*2.
		check("Radio.RadioSize", c.Radio.RadioSize, 16)
		check("Radio.DotSize", c.Radio.DotSize, 16-(4+1)*2)
		// radioColor: colorWhite (non-wireframe).
		if c.Radio.DotInnerColor != Hex("#ffffff") {
			t.Errorf("dark=%v Radio.DotInnerColor %v, want colorWhite", dark, c.Radio.DotInnerColor)
		}
		// es/input-number/index.js: mergedControls = controls &&
		// !mergedDisabled && !readOnly.
		if c.InputNumber.HideControlsInactive != 1 {
			t.Errorf("dark=%v InputNumber.HideControlsInactive %v, want 1", dark, c.InputNumber.HideControlsInactive)
		}
		if c.Radio.DotHoverColor != th.Primary.Hover {
			t.Errorf("dark=%v Radio.DotHoverColor: colorPrimaryHover", dark)
		}
		// es/checkbox/style/index.js: checkboxSize controlInteractiveSize
		// (16), borderRadiusSM (4).
		check("Checkbox.ControlInteractiveSize", c.Checkbox.ControlInteractiveSize, 16)
		check("Checkbox.Radius", c.Checkbox.Radius, 4)
		// es/form/style/index.js .ant-form-item-optional:
		// colorTextDescription.
		if c.Form.LabelOptionalColor != th.TextTertiary {
			t.Errorf("dark=%v Form.LabelOptionalColor %v", dark, c.Form.LabelOptionalColor)
		}
	}
}

// TestAlignB1bElementTokens: element-plus 2.14.8 theme-chalk values.
func TestAlignB1bElementTokens(t *testing.T) {
	for _, th := range []*Theme{ElementPlus(), ElementPlus(Dark())} {
		c := th.Components
		// theme-chalk/src/input-number.scss __increase / __decrease:
		// font-size 13px, as wide as the input is high.
		if c.InputNumber.SideHandleFontSize != 13 || c.InputNumber.SideHandleWidth != 0 {
			t.Errorf("el-input-number side buttons: font %v width %v", c.InputNumber.SideHandleFontSize, c.InputNumber.SideHandleWidth)
		}
		// common/var.scss $slider 'disabled-color':
		// --el-text-color-placeholder.
		if c.Slider.TrackBgDisabled != th.TextQuaternary || c.Slider.HandleColorDisabled != th.TextQuaternary {
			t.Errorf("el-slider disabled color %v / %v, want %v", c.Slider.TrackBgDisabled, c.Slider.HandleColorDisabled, th.TextQuaternary)
		}
		// el-input-number keeps − / + disabled; el-radio inner dot
		// --el-color-white.
		if c.InputNumber.HideControlsInactive != 0 || c.Radio.DotInnerColor != Hex("#ffffff") {
			t.Errorf("el-input-number controls hidden %v, radio inner %v", c.InputNumber.HideControlsInactive, c.Radio.DotInnerColor)
		}
		if c.Slider.HandleSize != 20 || c.Slider.RailSize != 6 {
			t.Errorf("el-slider button-size %v height %v", c.Slider.HandleSize, c.Slider.RailSize)
		}
	}
}

// TestAlignB1bShadcnTokens: new-york-v4 slider.tsx thumb ring-4.
func TestAlignB1bShadcnTokens(t *testing.T) {
	th := Shadcn()
	if w := th.Components.Slider.HandleActiveOutlineWidth; w != 4 {
		t.Errorf("shadcn slider ring %v, want 4 (ring-4)", w)
	}
	// new-york-v4 textarea.tsx: min-h-16 (4rem).
	if h := th.Components.Input.TextAreaMinHeight; h != 64 {
		t.Errorf("shadcn textarea min height %v, want 64", h)
	}
	if h := AntDesign().Components.Input.TextAreaMinHeight; h != 0 {
		t.Errorf("antd textarea min height %v, want 0 (rows decide)", h)
	}
}
