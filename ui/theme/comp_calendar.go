package theme

// CalendarToken styles Calendar (the full-page grid and the card).
type CalendarToken struct {
	Alias
	FullBg            Color
	MiniBorderColor   Color // outline of the card calendar
	MiniRadius        float32
	HeaderBorderColor Color // divider under the card / Element header

	FullCellHeight      float32
	MiniCellHeight      float32
	YearControlWidth    float32 // year select of the full calendar header
	YearControlWidthSM  float32 // … of the card
	MonthControlWidth   float32
	MonthControlWidthSM float32

	// Full calendar cells.
	ItemColor       Color `contrast:"text:FullBg"`
	ItemOutColor    Color // days of adjacent months
	ItemActiveBg    Color // selected day / month
	ItemActiveColor Color `contrast:"text:ItemActiveBg"` // selected day and today, their top line
	ItemHoverBg     Color
	ItemDisabledBg  Color
	ItemBorderColor Color // top line of a cell (Element: grid lines)

	// Card calendar days and months.
	DayRadius      float32
	DayActiveBg    Color
	DayActiveColor Color `contrast:"text:DayActiveBg"`
	DayHoverBg     Color
	DayTodayBorder Color `contrast:"nontext:FullBg"` // drawn unless DayTodayBg is set
	DayTodayBg     Color // zero: none
	DayOutColor    Color // zero: ItemOutColor
}

func deriveCalendar(t *Theme) CalendarToken {
	k := CalendarToken{
		FullBg: t.BgContainer, MiniBorderColor: t.BorderSecondary, MiniRadius: t.RadiusLG, HeaderBorderColor: t.BorderSecondary,
		FullCellHeight: 116, MiniCellHeight: 32,
		YearControlWidth: 100, YearControlWidthSM: 92, MonthControlWidth: 80, MonthControlWidthSM: 68,
		ItemColor: t.Text, ItemOutColor: t.TextQuaternary,
		ItemActiveBg: t.Primary.Bg, ItemActiveColor: t.Primary.Base,
		ItemHoverBg: t.ItemHoverBg, ItemDisabledBg: t.BgContainerDisabled, ItemBorderColor: t.Split,
		DayRadius: t.RadiusSM, DayActiveBg: t.Primary.Base, DayActiveColor: t.TextOnPrimary,
		DayHoverBg: t.ItemHoverBg, DayTodayBorder: t.Primary.Base,
	}
	switch t.Flavor {
	case Element:
		// el-calendar: bordered table cells tinted with the brand.
		k.FullCellHeight = 85
		k.ItemHoverBg, k.ItemDisabledBg, k.ItemBorderColor = t.Primary.Bg, t.FillTertiary, t.BorderSecondary
	case StyleShadcn:
		// shadcn Calendar: size-8 rounded-md days, mt-2 between the weeks,
		// accent today, muted outside days.
		k.MiniCellHeight = 40
		k.DayRadius = t.Radius
		k.DayTodayBg = t.ItemSelectedBg
		k.DayOutColor = t.TextTertiary
	}
	return k
}
