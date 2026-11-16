package theme

// Spec alignment batch 1, group C: Select, Cascader and Transfer token
// values checked against the reference sources (2026-11-14):
//   antd@6.7.0 es/select/style/token.js, es/cascader/style/index.js,
//   es/transfer/style/index.js (prepareComponentToken);
//   element-plus@2.14.8 theme-chalk/src/common/var.scss ($select-option,
//   $select-dropdown, $transfer), cascader-panel.scss;
//   shadcn new-york-v4 ui/select.tsx (SelectItem py-1.5 text-sm = 32px).

import "testing"

func TestAlignB1CSelectTokens(t *testing.T) {
	ant := AntDesign().Components.Select
	// multipleItemHeight = min(controlHeight - 2*paddingXXS, controlHeight - 2*lineWidth)
	if ant.MultipleItemHeight != 24 || ant.MultipleItemHeightSM != 16 || ant.MultipleItemHeightLG != 32 {
		t.Fatalf("Ant multiple item heights %v %v %v, want 24 16 32", ant.MultipleItemHeight, ant.MultipleItemHeightSM, ant.MultipleItemHeightLG)
	}
	// optionHeight = controlHeight, optionPadding = controlPaddingHorizontal (12),
	// optionSelectedFontWeight = fontWeightStrong (600), optionFontSize = fontSize.
	if ant.OptionHeight != 32 || ant.OptionPadding != 12 || ant.OptionSelectedFontWeight != 600 || ant.OptionFontSize != 14 {
		t.Fatalf("Ant option %v %v %v %v", ant.OptionHeight, ant.OptionPadding, ant.OptionSelectedFontWeight, ant.OptionFontSize)
	}
	el := ElementPlus().Components.Select
	// $select-option height 34px; option.scss padding 0 32px 0 20px.
	if el.OptionHeight != 34 || el.OptionPadding != 20 {
		t.Fatalf("Element option %v %v, want 34 20", el.OptionHeight, el.OptionPadding)
	}
	sh := Shadcn().Components.Select
	if sh.OptionHeight != 32 || sh.OptionPadding != 8 {
		t.Fatalf("shadcn option %v %v, want 32 8", sh.OptionHeight, sh.OptionPadding)
	}
	// native-select.tsx / select.tsx trigger: h-9 default, h-8 sm.
	if s := Shadcn(); s.ControlHeight != 36 || s.ControlHeightSM != 32 {
		t.Fatalf("shadcn control heights %v %v, want 36 32", s.ControlHeight, s.ControlHeightSM)
	}
}

func TestAlignB1CCascaderTokens(t *testing.T) {
	ant := AntDesign().Components.Cascader
	// controlItemWidth 111, dropdownHeight 180, optionPadding paddingSM (12).
	if ant.ColumnWidth != 111 || ant.DropdownHeight != 180 || ant.OptionPadding != 12 || ant.OptionHeight != 32 {
		t.Fatalf("Ant cascader %v %v %v %v", ant.ColumnWidth, ant.DropdownHeight, ant.OptionPadding, ant.OptionHeight)
	}
	el := ElementPlus().Components.Cascader
	// cascader-panel.scss: menu min-width 180px, height 204px; node height 34px, padding-left 20px.
	if el.ColumnWidth != 180 || el.DropdownHeight != 204 || el.OptionHeight != 34 || el.OptionPadding != 20 {
		t.Fatalf("Element cascader %v %v %v %v", el.ColumnWidth, el.DropdownHeight, el.OptionHeight, el.OptionPadding)
	}
}

func TestAlignB1CTransferTokens(t *testing.T) {
	ant := AntDesign().Components.Transfer
	// listWidth 180, listHeight 200, headerHeight controlHeightLG, itemHeight controlHeight.
	if ant.ListWidth != 180 || ant.ListHeight != 200 || ant.HeaderHeight != 40 || ant.ItemHeight != 32 {
		t.Fatalf("Ant transfer %v %v %v %v, want 180 200 40 32", ant.ListWidth, ant.ListHeight, ant.HeaderHeight, ant.ItemHeight)
	}
	el := ElementPlus().Components.Transfer
	// $transfer panel-width 200px, header 40px + body 278px, item-height 30px.
	if el.ListWidth != 200 || el.ListHeight != 318 || el.HeaderHeight != 40 || el.ItemHeight != 30 {
		t.Fatalf("Element transfer %v %v %v %v, want 200 318 40 30", el.ListWidth, el.ListHeight, el.HeaderHeight, el.ItemHeight)
	}
}
