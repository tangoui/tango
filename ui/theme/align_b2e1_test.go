package theme

// Spec alignment batch 2, group E1 (Table appearance and tokens): token
// values compared with the source (antd@6.7.0 es/table/style,
// element-plus@2.14.8 theme-chalk, shadcn new-york-v4 ui/table.tsx) as
// recorded in docs/specs/align/2-table-look.md. The expected values are
// the source values.

import "testing"

func TestAlignB2E1AntTableTokens(t *testing.T) {
	ant := AntDesign()
	tk := ant.Components.Table
	// antd es/table/style/index.js prepareComponentToken:
	// headerBg / rowHoverBg / bodySortBg = colorFillAlter on
	// colorBgContainer = #fafafa; headerColor colorTextHeading.
	if tk.HeaderBg != Hex("#fafafa") || tk.RowHoverBg != Hex("#fafafa") || tk.HeaderColor != ant.TextHeading {
		t.Errorf("Ant header %v hover %v color %v", tk.HeaderBg, tk.RowHoverBg, tk.HeaderColor)
	}
	if Over(tk.BodySortBg, ant.BgContainer) != Hex("#fafafa") {
		t.Errorf("Ant bodySortBg %v, want #fafafa on white", tk.BodySortBg)
	}
	// headerSortActiveBg = headerSortHoverBg = colorFillSecondary
	// (colorFillContent) on colorBgContainer = #f0f0f0; Tango draws the
	// token over the header bg (#fafafa), which composites to the same.
	for _, c := range []Color{tk.HeaderSortActiveBg, tk.HeaderSortHoverBg} {
		if got := Over(c, tk.HeaderBg); got != Hex("#f0f0f0") {
			t.Errorf("Ant sorted header over headerBg %v, want #f0f0f0", got)
		}
	}
	// rowSelectedBg controlItemBgActive (colorPrimaryBg), hover
	// controlItemBgActiveHover (colorPrimaryBgHover); rowExpandedBg
	// colorFillAlter; borderColor / headerSplitColor colorBorderSecondary;
	// headerBorderRadius borderRadiusLG 8.
	if tk.RowSelectedBg != ant.Primary.Bg || tk.RowSelectedHoverBg != ant.Primary.BgHover || tk.RowExpandedBg != ant.FillAlter {
		t.Errorf("Ant selected %v / %v expanded %v", tk.RowSelectedBg, tk.RowSelectedHoverBg, tk.RowExpandedBg)
	}
	if tk.BorderColor != Hex("#f0f0f0") || tk.HeaderSplitColor != Hex("#f0f0f0") || tk.HeaderBorderRadius != 8 {
		t.Errorf("Ant border %v split %v radius %v", tk.BorderColor, tk.HeaderSplitColor, tk.HeaderBorderRadius)
	}
	// radius.js: only the top corners are rounded (footer gets the bottom).
	if tk.BottomRadius != 0 {
		t.Errorf("Ant BottomRadius %v, want 0", tk.BottomRadius)
	}
	// cellPaddingBlock/Inline padding 16; MD paddingSM 12 / paddingXS 8;
	// SM paddingXS 8 / 8; cellFontSize / MD / SM fontSize 14; line 22.
	if tk.CellPaddingBlock != 16 || tk.CellPaddingInline != 16 || tk.CellPaddingBlockMD != 12 || tk.CellPaddingInlineMD != 8 ||
		tk.CellPaddingBlockSM != 8 || tk.CellPaddingInlineSM != 8 {
		t.Errorf("Ant paddings %+v", tk)
	}
	if tk.CellFontSize != 14 || tk.CellFontSizeMD != 14 || tk.CellFontSizeSM != 14 || tk.CellLineHeight != 22 {
		t.Errorf("Ant font %v/%v/%v line %v", tk.CellFontSize, tk.CellFontSizeMD, tk.CellFontSizeSM, tk.CellLineHeight)
	}
	// headerIconColor colorIcon (.45) × opacityLoading .65 = .29;
	// headerIconHoverColor colorIconHover (.88) × .65 = .57.
	if tk.SorterIdleColor != RGBA(0, 0, 0, .29) || tk.SorterHoverColor != RGBA(0, 0, 0, .572) {
		t.Errorf("Ant sorter idle %v hover %v", tk.SorterIdleColor, tk.SorterHoverColor)
	}
	// filter.js: trigger fontSizeSM 12, hover colorIcon + headerFilterHoverBg
	// (colorFillContent .06), borderRadius 6; dropdown filterDropdownBg
	// colorBgContainer, borderRadius 6.
	if tk.FilterIconSize != 12 || tk.FilterHoverColor != ant.TextTertiary || tk.FilterHoverBg != RGBA(0, 0, 0, .06) ||
		tk.FilterRadius != 6 || tk.FilterDropdownBg != ant.BgContainer || tk.FilterDropdownRadius != 6 || tk.FilterArrow {
		t.Errorf("Ant filter %v %v %v %v %v %v", tk.FilterIconSize, tk.FilterHoverColor, tk.FilterHoverBg, tk.FilterRadius, tk.FilterDropdownBg, tk.FilterDropdownRadius)
	}
	// expandIconSize = (controlInteractiveSize 16 / 2 − 1) × 2 + 3 = 17,
	// expandIconScale 16 / 17, borderRadius 6, expandIconBg colorBgContainer.
	if tk.ExpandIconSize != 17 || tk.ExpandIconScale != 16.0/17 || tk.ExpandIconRadius != 6 || tk.ExpandIconBg != ant.BgContainer || tk.ExpandIconArrow {
		t.Errorf("Ant expand icon %v × %v r %v bg %v", tk.ExpandIconSize, tk.ExpandIconScale, tk.ExpandIconRadius, tk.ExpandIconBg)
	}
	// expand.js operationUnit hover colorLinkHover.
	if tk.ExpandIconHoverColor != ant.TextLinkHover {
		t.Errorf("Ant expand hover %v, want colorLinkHover %v", tk.ExpandIconHoverColor, ant.TextLinkHover)
	}
	// selectionColumnWidth controlHeight 32; tableExpandColumnWidth
	// controlInteractiveSize 16 + padding × 2 = 48.
	if tk.SelectionColumnWidth != 32 || tk.ExpandColumnWidth != 48 {
		t.Errorf("Ant selection %v expand %v", tk.SelectionColumnWidth, tk.ExpandColumnWidth)
	}
	// index.js:55 scrollbarColor: tableScrollThumbBg (colorTextPlaceholder
	// .25); tableScrollThumbSize 8; grab color TextTertiary (ui/scroll.go).
	if tk.StickyScrollBarSize != 8 || tk.StickyScrollBarBg != ant.TextQuaternary || tk.StickyScrollBarHoverBg != ant.TextTertiary {
		t.Errorf("Ant sticky bar %v %v %v", tk.StickyScrollBarSize, tk.StickyScrollBarBg, tk.StickyScrollBarHoverBg)
	}
	// summary.js: summary background tableBg (colorBgContainer).
	if tk.FooterBg != ant.BgContainer || tk.FooterFontWeight != 400 {
		t.Errorf("Ant summary bg %v weight %v", tk.FooterBg, tk.FooterFontWeight)
	}
	// .ant-table-footer: footerBg colorFillAlter solid, footerColor colorTextHeading.
	if tk.FooterAreaBg != Hex("#fafafa") || tk.FooterAreaColor != ant.TextHeading {
		t.Errorf("Ant footer area %v %v", tk.FooterAreaBg, tk.FooterAreaColor)
	}
	if tk.VirtualRowHeight != 0 || tk.VirtualHeaderHeight != 0 {
		t.Error("Ant virtual tables keep the padded row height")
	}
}

func TestAlignB2E1AntDarkTableTokens(t *testing.T) {
	d := AntDesign(Dark())
	tk := d.Components.Table
	// dark: colorFillAlter rgba(255,255,255,.04) on #141414 = #1d1d1d.
	if tk.HeaderBg != Hex("#1d1d1d") {
		t.Errorf("Ant dark headerBg %v, want #1d1d1d", tk.HeaderBg)
	}
	// rowHoverBg colorFillAlterSolid (index.js:203) = #1d1d1d; headerIconColor
	// colorIcon (.45) × opacityLoading .65 = rgba(255,255,255,.29).
	if tk.RowHoverBg != Hex("#1d1d1d") || tk.SorterIdleColor != RGBA(255, 255, 255, .29) {
		t.Errorf("Ant dark row hover %v sorter idle %v", tk.RowHoverBg, tk.SorterIdleColor)
	}
	// filterDropdownBg colorBgContainer (#141414, not the elevated bg).
	if tk.FilterDropdownBg != Hex("#141414") {
		t.Errorf("Ant dark filter dropdown %v, want #141414", tk.FilterDropdownBg)
	}
}

func TestAlignB2E1ElementTableTokens(t *testing.T) {
	el := ElementPlus()
	tk := el.Components.Table
	// var.scss $table: header-text-color text-color-secondary,
	// text-color text-color-regular, row-hover-bg fill-color-light,
	// border-color border-color-lighter, current-row color-primary-light-9.
	if tk.HeaderColor != Hex("#909399") || tk.CellColor != Hex("#606266") || tk.RowHoverBg != Hex("#f5f7fa") || tk.BorderColor != Hex("#ebeef5") {
		t.Errorf("Element header %v cell %v hover %v border %v", tk.HeaderColor, tk.CellColor, tk.RowHoverBg, tk.BorderColor)
	}
	if tk.RowCurrentBg != el.Primary.Bg {
		t.Errorf("Element current row %v, want primary-light-9 %v", tk.RowCurrentBg, el.Primary.Bg)
	}
	// table.scss thead th font-weight 600; .cell line-height 23px;
	// thead.is-group th background fill-color-light.
	if tk.HeaderFontWeight != 600 || tk.CellLineHeight != 23 || tk.HeaderGroupBg != Hex("#f5f7fa") {
		t.Errorf("Element weight %v line %v group %v", tk.HeaderFontWeight, tk.CellLineHeight, tk.HeaderGroupBg)
	}
	// --striped: fill-color-lighter #fafafa (dark #1d1d1d).
	if tk.RowStripedBg != Hex("#fafafa") || ElementPlus(Dark()).Components.Table.RowStripedBg != Hex("#1d1d1d") {
		t.Errorf("Element striped %v", tk.RowStripedBg)
	}
	// $table-padding 12 / 8 / 4 and $table-cell-padding 16 / 12 / 8
	// (large / default / small); $table-font-size small extra-small 12.
	if tk.CellPaddingBlock != 12 || tk.CellPaddingInline != 16 || tk.CellPaddingBlockMD != 8 || tk.CellPaddingInlineMD != 12 ||
		tk.CellPaddingBlockSM != 4 || tk.CellPaddingInlineSM != 8 || tk.CellFontSizeSM != 12 {
		t.Errorf("Element paddings %v/%v %v/%v %v/%v font %v", tk.CellPaddingBlock, tk.CellPaddingInline, tk.CellPaddingBlockMD, tk.CellPaddingInlineMD, tk.CellPaddingBlockSM, tk.CellPaddingInlineSM, tk.CellFontSizeSM)
	}
	// No sort tint, no header split, no rounding, selected rows plain,
	// expanded cell fill-color-blank, padding 20px 50px.
	if tk.HeaderSortActiveBg.A != 0 || tk.BodySortBg.A != 0 || tk.HeaderSplitColor.A != 0 || tk.HeaderBorderRadius != 0 || tk.BottomRadius != 0 ||
		tk.RowSelectedBg.A != 0 || tk.RowExpandedBg.A != 0 || tk.ExpandedPaddingBlock != 20 || tk.ExpandedPaddingInline != 50 {
		t.Errorf("Element plain look %+v", tk)
	}
	// sort-caret: border 5px → 10 × 5, carets 2px apart; placeholder color.
	if tk.SorterArrowWidth != 10 || tk.SorterArrowHeight != 5 || tk.SorterGap != 2 || tk.SorterIdleColor != Hex("#a8abb2") {
		t.Errorf("Element sorter %v×%v gap %v idle %v", tk.SorterArrowWidth, tk.SorterArrowHeight, tk.SorterGap, tk.SorterIdleColor)
	}
	// column-filter-trigger i: color-info, 14px; .el-table-filter
	// border-color-lighter, radius 2px.
	if !tk.FilterArrow || tk.FilterIconSize != 14 || tk.FilterIdleColor != Hex("#909399") || tk.FilterDropdownRadius != 2 || tk.FilterDropdownBorder != Hex("#ebeef5") {
		t.Errorf("Element filter %v %v %v %v %v", tk.FilterArrow, tk.FilterIconSize, tk.FilterIdleColor, tk.FilterDropdownRadius, tk.FilterDropdownBorder)
	}
	// __expand-icon text-color-regular; type=selection width 48.
	if !tk.ExpandIconArrow || tk.ExpandIconColor != Hex("#606266") || tk.SelectionColumnWidth != 48 {
		t.Errorf("Element expand %v %v selection %v", tk.ExpandIconArrow, tk.ExpandIconColor, tk.SelectionColumnWidth)
	}
	// $scrollbar opacity .3 / hover .5 of text-color-secondary, 6px.
	if tk.StickyScrollBarSize != 6 || tk.StickyScrollBarBg != Fade(Hex("#909399"), .3) || tk.StickyScrollBarHoverBg != Fade(Hex("#909399"), .5) {
		t.Errorf("Element scrollbar %v %v %v", tk.StickyScrollBarSize, tk.StickyScrollBarBg, tk.StickyScrollBarHoverBg)
	}
	// el-table-v2 row-height 50, header-height 50.
	if tk.VirtualRowHeight != 50 || tk.VirtualHeaderHeight != 50 {
		t.Errorf("Element virtual heights %v / %v", tk.VirtualRowHeight, tk.VirtualHeaderHeight)
	}
}

func TestAlignB2E1ShadcnTableTokens(t *testing.T) {
	for _, dark := range []bool{false, true} {
		var th *Theme
		if dark {
			th = Shadcn(Dark())
		} else {
			th = Shadcn()
		}
		v, tk := th.Shadcn, th.Components.Table
		// table.tsx TableHead: h-10 px-2 font-medium text-foreground.
		if tk.HeaderColor != v.Foreground || tk.HeaderFontWeight != 500 || tk.CellPaddingInline != 8 {
			t.Errorf("shadcn head color %v weight %v px %v", tk.HeaderColor, tk.HeaderFontWeight, tk.CellPaddingInline)
		}
		// TableCell p-2; text-sm (leading 20).
		if tk.CellPaddingBlock != 8 || tk.CellLineHeight != 20 {
			t.Errorf("shadcn cell p %v line %v", tk.CellPaddingBlock, tk.CellLineHeight)
		}
		// TableRow hover:bg-muted/50, data-[state=selected]:bg-muted.
		bg := v.Background
		if tk.RowHoverBg != Mix(bg, v.Muted, .5) || tk.RowSelectedBg != v.Muted || tk.RowSelectedHoverBg != v.Muted {
			t.Errorf("shadcn hover %v selected %v / %v", tk.RowHoverBg, tk.RowSelectedBg, tk.RowSelectedHoverBg)
		}
		// TableFooter bg-muted/50 font-medium.
		if tk.FooterBg != Mix(bg, v.Muted, .5) || tk.FooterFontWeight != 500 {
			t.Errorf("shadcn footer %v weight %v", tk.FooterBg, tk.FooterFontWeight)
		}
		// data-table wrapper rounded-md (radius − 2) border; no split, no
		// sort tint.
		if tk.HeaderBorderRadius != th.RadiusLG-2 || tk.BottomRadius != th.RadiusLG-2 || tk.HeaderSplitColor.A != 0 || tk.BodySortBg.A != 0 {
			t.Errorf("shadcn radius %v / %v split %v", tk.HeaderBorderRadius, tk.BottomRadius, tk.HeaderSplitColor)
		}
		// has-aria-expanded:bg-muted/50: the expanded panel stays muted/50.
		if tk.RowExpandedBg != Mix(bg, v.Muted, .5) {
			t.Errorf("shadcn expanded %v", tk.RowExpandedBg)
		}
		// scroll-area thumb bg-border rounded-full, w-2.5 − border − p-px × 2 = 7.
		if tk.StickyScrollBarBg != v.Border || tk.StickyScrollBarSize != 7 {
			t.Errorf("shadcn scroll thumb %v %v", tk.StickyScrollBarBg, tk.StickyScrollBarSize)
		}
	}
}
