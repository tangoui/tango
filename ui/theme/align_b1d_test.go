package theme

import "testing"

// TestAlignB1DDatePickerTokens checks the date picker token values read
// from the reference sources (2026-11-14):
//   - antd@6.7.0 es/date-picker/style/token.js: cellWidth =
//     controlHeightSM*1.5 (36), cellHeight = controlHeightSM (24, the
//     inner square), pickerCellPaddingVertical = paddingXXS*1.5 (6),
//     textHeight = controlHeightLG (40), timeColumnWidth =
//     controlHeightLG*1.4 (56), timeColumnHeight = 28*8, timeCellHeight
//     28, presetsWidth 120, pickerDatePanelPaddingHorizontal 18,
//     pickerYearMonthCellWidth = controlHeightLG*1.5 (60);
//     es/date-picker/style/panel.js: a row is cellHeight + 2 *
//     pickerCellPaddingVertical (36).
//   - element-plus@2.14.8 theme-chalk/src/date-picker/picker-panel.scss:
//     .el-picker-panel__sidebar width 110px.
func TestAlignB1DDatePickerTokens(t *testing.T) {
	a := AntDesign().Components.DatePicker
	check := func(name string, got, want float32) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("ant CellWidth", a.CellWidth, 36)
	check("ant CellHeight", a.CellHeight, 36)
	check("ant CellInnerSize", a.CellInnerSize, 24)
	check("ant HeaderHeight", a.HeaderHeight, 40)
	check("ant MonthCellWidth", a.MonthCellWidth, 60)
	check("ant PanelPaddingInline", a.PanelPaddingInline, 18)
	check("ant PresetsWidth", a.PresetsWidth, 120)
	check("ant TimeColumnWidth", a.TimeColumnWidth, 56)
	check("ant TimeColumnHeight", a.TimeColumnHeight, 224)
	check("ant TimeCellHeight", a.TimeCellHeight, 28)

	e := ElementPlus().Components.DatePicker
	check("element PresetsWidth", e.PresetsWidth, 110)
	ratio := func(name string, got, want float64) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	ratio("element NavDisabledOpacity", e.NavDisabledOpacity, 1)
	if e.NavDisabledColor != ElementPlus().TextDisabled {
		t.Errorf("element NavDisabledColor = %v", e.NavDisabledColor)
	}
	ratio("ant NavDisabledOpacity", a.NavDisabledOpacity, 0.25)
	// shadcn keeps its size-8 day cells (calendar.tsx
	// [--cell-size:--spacing(8)]) and dims disabled arrows to 50%
	// (button_previous aria-disabled:opacity-50).
	s := Shadcn().Components.DatePicker
	check("shadcn CellHeight", s.CellHeight, 32)
	ratio("shadcn NavDisabledOpacity", s.NavDisabledOpacity, 0.5)
}

// TestAlignB1DColorPickerTokens checks the color picker token values read
// from antd@6.7.0 es/color-picker/style/index.js (2026-11-14):
// colorPickerWidth 234 (+ 2 * padding 12), colorPickerHandlerSize 16,
// colorPickerSliderHeight 8, colorPickerPresetColorSize 24.
func TestAlignB1DColorPickerTokens(t *testing.T) {
	k := AntDesign().Components.ColorPicker
	if k.PanelWidth != 234+24 || k.PanelPadding != 12 || k.HandleSize != 16 || k.RailHeight != 8 || k.SwatchSize != 24 {
		t.Fatalf("color picker tokens %+v", k)
	}
}
