package theme

import "github.com/tangoui/tango/ui/paint"

// DatePickerToken styles the date and time pickers: DatePicker,
// DatePickerPanel, RangePicker, DateTimePicker, TimePicker, TimePanel,
// TimeRangePicker and TimeSelect (their trigger fields use the Input frame).
type DatePickerToken struct {
	Alias
	CellWidth       float32 // day cell box
	CellHeight      float32
	CellInnerSize   float32 // highlighted square inside a day cell
	MonthCellWidth  float32 // highlight of a month / quarter / year cell
	MonthCellHeight float32
	CellRadius      float32 // cell highlight (clamped to half its height)

	CellColor            Color `contrast:"text:@Elevated"`
	CellOutColor         Color // days of adjacent months, years outside the decade
	CellHoverBg          Color // zero: none (the hover only recolors the text)
	CellHoverColor       Color // zero: unchanged
	CellHoverWithRangeBg Color // hovered cell inside the selected range
	CellActiveBg         Color // selected cell
	CellActiveColor      Color `contrast:"text:CellActiveBg"`
	CellActiveFontWeight int   // zero: regular
	CellRangeBg          Color // strip behind the cells of a range
	CellRangeColor       Color // zero: unchanged
	CellRangePreviewBg   Color // hover preview of a range (zero: none)
	CellRangeHoverBorder Color // dashed outline of the preview
	CellTodayBorder      Color `contrast:"nontext:@Elevated"` // zero: none
	CellTodayBg          Color // zero: none
	CellTodayColor       Color // zero: unchanged
	CellTodayFontWeight  int   // zero: regular
	CellDisabledBg       Color // zero: none
	CellDisabledColor    Color

	WeekColor       Color // weekday headers (zero: the text color)
	WeekFontSize    float32
	WeekNumberColor Color // week numbers of the week picker

	HeaderHeight      float32
	HeaderColor       Color `contrast:"text:@Elevated"` // clickable year / month titles
	HeaderHoverColor  Color // zero: unchanged
	HeaderHoverBg     Color // zero: none
	HeaderBorderColor Color // divider under the header (zero: none)

	PanelPaddingInline float32 // around the cell grid
	PanelBg            Color
	PanelRadius        float32 // popup panel
	PanelShadow        []paint.Shadow
	PanelBorderColor   Color // outline of a bordered inline panel
	SplitColor         Color // footer, preset and panel dividers
	PresetsWidth       float32
	FooterHeight       float32
	PresetColor        Color `contrast:"text:@Elevated"`
	PresetHoverColor   Color // zero: unchanged
	PresetHoverBg      Color // zero: none

	TimeColumnWidth          float32
	TimeColumnHeight         float32
	TimeCellHeight           float32
	TimeCellFontSize         float32
	TimeCellRadius           float32
	TimeCellColor            Color `contrast:"text:@Elevated"`
	TimeCellHoverBg          Color
	TimeCellActiveBg         Color // zero: none
	TimeCellActiveColor      Color `contrast:"text:TimeCellActiveBg"`
	TimeCellActiveFontWeight int   // zero: regular
	TimeCellDisabledBg       Color // selected disabled option (zero: none)
	TimeCellDisabledColor    Color
}

func deriveDatePicker(t *Theme) DatePickerToken {
	k := DatePickerToken{
		CellWidth: 36, CellHeight: 32, CellInnerSize: 24, MonthCellWidth: 60, MonthCellHeight: 24,
		CellRadius:   t.RadiusSM,
		CellColor:    t.Text,
		CellOutColor: t.TextQuaternary,
		CellHoverBg:  t.ItemHoverBg, CellHoverWithRangeBg: Over(t.ItemHoverBg, t.Primary.Bg),
		CellActiveBg: t.Primary.Base, CellActiveColor: t.TextOnPrimary,
		CellRangeBg: t.Primary.Bg, CellRangePreviewBg: Fade(t.Primary.Bg, 0.6), CellRangeHoverBorder: t.Primary.Border,
		CellTodayBorder: t.Primary.Base,
		CellDisabledBg:  t.BgContainerDisabled, CellDisabledColor: t.TextDisabled,
		WeekFontSize: t.FontSize, WeekNumberColor: t.TextQuaternary,
		HeaderHeight: 40, HeaderColor: t.Text, HeaderHoverColor: t.Primary.Base, HeaderBorderColor: t.Split,
		PanelPaddingInline: 18,
		PanelBg:            t.BgElevated, PanelRadius: t.RadiusLG, PanelShadow: t.Shadow, PanelBorderColor: t.BorderSecondary,
		SplitColor:   t.Split,
		PresetsWidth: 110, FooterHeight: 40,
		PresetColor: t.Text, PresetHoverBg: t.ItemHoverBg,
		TimeColumnWidth: 56, TimeColumnHeight: 224, TimeCellHeight: 28, TimeCellFontSize: t.FontSize, TimeCellRadius: t.RadiusSM,
		TimeCellColor: t.Text, TimeCellHoverBg: t.ItemHoverBg,
		TimeCellActiveBg: t.Primary.Bg, TimeCellActiveColor: t.Text,
		TimeCellDisabledBg: t.BgContainerDisabled, TimeCellDisabledColor: t.TextDisabled,
	}
	switch t.Flavor {
	case Element:
		// el-date-table: round 24px cells, text-only hover, bold today.
		k.CellWidth, k.CellHeight = 40, 36
		k.MonthCellHeight = 36
		k.CellRadius = 18
		k.CellColor = t.TextSecondary
		k.CellHoverBg, k.CellHoverColor = Color{}, t.Primary.Base
		k.CellActiveFontWeight = 500
		k.CellRangeBg = t.FillTertiary
		k.CellTodayBorder, k.CellTodayColor, k.CellTodayFontWeight = Color{}, t.Primary.Base, 700
		k.WeekColor = t.TextSecondary
		k.HeaderColor = t.TextSecondary
		k.HeaderBorderColor = Color{}
		k.PanelPaddingInline = 12
		k.PresetHoverColor, k.PresetHoverBg = t.Primary.Base, Color{}
		k.TimeColumnWidth, k.TimeColumnHeight, k.TimeCellHeight = 60, 192, 32
		k.TimeCellFontSize, k.TimeCellRadius = t.FontSizeSM, 0
		k.TimeCellColor = t.TextSecondary
		k.TimeCellActiveBg, k.TimeCellActiveColor, k.TimeCellActiveFontWeight = Color{}, t.Primary.Base, 700
		k.TimeCellDisabledBg = Color{}
	case StyleShadcn:
		// shadcn Calendar: size-8 rounded-md days, accent hover, today and
		// range band, muted half-opaque outside and disabled days.
		k.CellWidth, k.CellHeight = 32, 32
		k.MonthCellHeight = 32
		k.CellRadius = t.Radius
		muted := Fade(t.TextTertiary, .5)
		k.CellOutColor = muted
		k.CellHoverColor = t.ItemSelected
		k.CellRangeBg, k.CellRangeColor = t.ItemHoverBg, t.ItemSelected
		k.CellTodayBorder, k.CellTodayBg, k.CellTodayColor, k.CellTodayFontWeight = Color{}, t.ItemHoverBg, t.ItemSelected, 500
		k.CellDisabledBg, k.CellDisabledColor = Color{}, muted
		k.WeekColor, k.WeekFontSize = t.TextTertiary, t.FontSizeSM
		k.HeaderHoverColor, k.HeaderHoverBg = Color{}, t.ItemHoverBg
		k.HeaderBorderColor = Color{}
		k.PanelPaddingInline = 12
		k.FooterHeight = 44
		k.TimeCellActiveBg, k.TimeCellActiveColor = t.Primary.Base, t.TextOnPrimary
	}
	return k
}
