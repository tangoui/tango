package theme

// CascaderToken styles the Cascader menu columns (the trigger and the
// search result list follow Select).
type CascaderToken struct {
	Alias
	OptionHeight             float32
	OptionPadding            float32 // inline start padding of an option
	ColumnWidth              float32 // minimum column width
	DropdownHeight           float32 // column height
	OptionColor              Color   `contrast:"text:@Elevated"`
	OptionHoverBg            Color
	OptionSelectedBg         Color // option on the expanded path / selected leaf (zero: none)
	OptionSelectedColor      Color `contrast:"text:OptionSelectedBg"`
	OptionSelectedFontWeight int
	ColumnDividerColor       Color
	ExpandIconColor          Color `contrast:"nontext:@Elevated"` // arrow towards the next column
}

func deriveCascader(t *Theme) CascaderToken {
	k := CascaderToken{
		OptionHeight: t.ControlHeight, OptionPadding: 12, ColumnWidth: 111, DropdownHeight: 180,
		OptionColor: t.Text, OptionHoverBg: t.ItemHoverBg, OptionSelectedBg: t.ItemSelectedBg,
		OptionSelectedColor: t.Text, OptionSelectedFontWeight: t.FontWeightStrong,
		ColumnDividerColor: t.Split, ExpandIconColor: t.TextTertiary,
	}
	switch t.Flavor {
	case Element:
		// el-cascader-menu: 180px columns, 34px nodes, the active path in
		// bold brand color without a fill.
		k.OptionHeight, k.OptionPadding, k.ColumnWidth, k.DropdownHeight = 34, 20, 180, 204
		k.OptionColor = t.TextSecondary
		k.OptionSelectedBg, k.OptionSelectedColor, k.OptionSelectedFontWeight = Color{}, t.Primary.Base, 700
		k.ExpandIconColor = t.TextSecondary
	case StyleShadcn:
		// Command-style items (px-2, 32px) with the accent background on
		// the expanded path and under the pointer; w-32 columns.
		k.OptionHeight, k.OptionPadding, k.ColumnWidth = 32, 8, 128
		k.OptionSelectedBg, k.OptionSelectedColor, k.OptionSelectedFontWeight = t.ItemHoverBg, t.ItemSelected, 400
	}
	return k
}
