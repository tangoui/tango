package theme

// TableToken styles Table.
//
// Sources (docs/specs/align/2-table-look.md): antd@6.7.0
// es/table/style/index.js prepareComponentToken and the style parts
// (sorter.js, filter.js, expand.js, bordered.js, radius.js, sticky.js);
// element-plus@2.14.8 theme-chalk/src/table.scss, table-column.scss and
// common/var.scss $table / $table-padding / $table-cell-padding;
// shadcn new-york-v4 ui/table.tsx.
type TableToken struct {
	Alias
	HeaderBg           Color
	HeaderColor        Color `contrast:"text:HeaderBg"`
	HeaderFontWeight   int
	HeaderSortActiveBg Color   // header of the sorted sortable column (zero: none)
	HeaderSortHoverBg  Color   // header of a hovered sortable column (zero: HeaderSortActiveBg)
	HeaderSplitColor   Color   // short separator between leaf header cells (zero: none)
	HeaderBorderRadius float32 // top corners of the header (and the bordered frame)
	// HeaderGroupBg is the background of every header cell of a grouped
	// (multi-row) header: Element's thead.is-group tint.
	HeaderGroupBg      Color
	BodySortBg         Color // body cells of the sorted column (zero: none)
	RowHoverBg         Color
	RowSelectedBg      Color // zero: selected rows are not tinted (Element)
	RowSelectedHoverBg Color // zero: RowSelectedBg
	RowCurrentBg       Color // HighlightCurrentRow
	RowStripedBg       Color // every other row of a Striped table
	RowExpandedBg      Color // the expanded detail panel, over BgContainer (zero: none)
	CellColor          Color `contrast:"text:@Page"` // text of plain body cells
	BorderColor        Color // row lines, cell borders and the outer border
	// BottomRadius rounds the bottom corners of a bordered table (antd
	// rounds only the top: radius.js); zero: square.
	BottomRadius     float32
	FooterBg         Color // summary cells
	FooterColor      Color `contrast:"text:FooterBg"`
	FooterFontWeight int   // summary text
	// FooterAreaBg / FooterAreaColor style the Footer area below the
	// table (antd footerBg colorFillAlter on colorBgContainer, footerColor
	// colorTextHeading); shadcn uses it for TableCaption
	// (text-muted-foreground). Zero bg: none.
	FooterAreaBg     Color
	FooterAreaColor  Color `contrast:"text:FooterAreaBg"`
	SorterIdleColor  Color // sorter / filter icon when inactive
	SorterHoverColor Color // inactive sorter while its header is hovered
	// Sorter caret geometry: width and height of each triangle and the
	// gap between the up and down carets.
	SorterArrowWidth  float32
	SorterArrowHeight float32
	SorterGap         float32
	// Filter trigger in the header: icon size, idle / hover colors, hover
	// background and corner radius; FilterArrow draws Element's arrow
	// instead of Ant's funnel.
	FilterIconSize       float32
	FilterIdleColor      Color
	FilterHoverColor     Color
	FilterHoverBg        Color // zero: none
	FilterRadius         float32
	FilterArrow          bool // can only be turned on through Components (false inherits)
	FilterDropdownBg     Color
	FilterDropdownRadius float32
	FilterDropdownBorder Color // zero: none (the shadow outlines it)
	// Expand / tree toggle. Ant's is a bordered plus/minus box:
	// ExpandIconSize is its CSS box, drawn scaled by ExpandIconScale;
	// ExpandIconArrow draws Element's rotating arrow instead.
	ExpandIconSize       float32
	ExpandIconScale      float32
	ExpandIconRadius     float32
	ExpandIconBg         Color
	ExpandIconBorder     Color
	ExpandIconColor      Color
	ExpandIconHoverColor Color
	ExpandIconArrow      bool // can only be turned on through Components (false inherits)
	// Expanded panel padding (zero: the cell paddings).
	ExpandedPaddingBlock  float32
	ExpandedPaddingInline float32
	// IndentSize is the default tree indent per level (antd
	// expandable.indentSize 15, EP el-table indent 16).
	IndentSize float32
	// SelectionPaddingInline is the inline padding of selection cells
	// (antd selection.js paddingXS 8; EP .el-table-column--selection .cell
	// 14px); the checkbox is centred.
	SelectionPaddingInline float32
	// Column widths of the selection and expand columns.
	SelectionColumnWidth float32
	ExpandColumnWidth    float32
	// SelectionBorderedExtra widens the selection column of a bordered
	// table (antd selection.js: + 2 × paddingXS = 16; Element and shadcn
	// 0).
	SelectionBorderedExtra float32
	// SorterTooltip shows the next sort action over a sortable header
	// unless ShowSorterTooltip(false) (antd showSorterTooltip default
	// true; Element Plus has none).
	SorterTooltip bool
	// Scroll thumbs of a bounded table (opacity included).
	StickyScrollBarBg      Color
	StickyScrollBarHoverBg Color
	StickyScrollBarSize    float32
	FixedShadowAlpha       float64
	CellFontSize           float32
	CellFontSizeMD         float32 // Middle size
	CellFontSizeSM         float32 // Small size
	CellLineHeight         float32 // line box of cell text
	// Cell paddings per size: the plain fields are the default (Ant
	// "large") density, MD the Middle size and SM the Small size. Element's
	// default density is MD; its Large size uses the plain fields.
	CellPaddingBlock    float32
	CellPaddingInline   float32
	CellPaddingBlockMD  float32
	CellPaddingInlineMD float32
	CellPaddingBlockSM  float32
	CellPaddingInlineSM float32
	// Fixed row and header heights of a Virtual table (EP el-table-v2
	// row-height / header-height); zero: from the paddings.
	VirtualRowHeight    float32
	VirtualHeaderHeight float32
}

func deriveTable(t *Theme) TableToken {
	idle := t.TableSorterIdle
	k := TableToken{
		HeaderBg: t.TableHeaderBg, HeaderColor: t.TableHeaderText, HeaderFontWeight: t.TableHeaderWeight,
		HeaderSortActiveBg: t.TableHeaderActiveBg, HeaderSortHoverBg: t.TableHeaderActiveBg,
		HeaderSplitColor: t.BorderSecondary, HeaderBorderRadius: t.RadiusLG, HeaderGroupBg: t.TableHeaderBg,
		BodySortBg: t.TableSortedBg, RowHoverBg: t.TableRowHoverBg,
		RowSelectedBg: t.Primary.Bg, RowSelectedHoverBg: t.Primary.BgHover, RowCurrentBg: t.Primary.Bg,
		RowStripedBg: t.FillAlter, RowExpandedBg: t.FillAlter, CellColor: t.Text,
		BorderColor: t.BorderSecondary, FooterBg: t.BgContainer, FooterColor: t.Text, FooterFontWeight: 400,
		// antd headerIconColor = colorIcon × opacityLoading (.65);
		// headerIconHoverColor = colorIconHover (colorText) × .65.
		FooterAreaBg: Over(t.FillAlter, t.BgContainer), FooterAreaColor: t.TextHeading,
		SorterIdleColor: idle, SorterHoverColor: Fade(t.Text, .65),
		SorterArrowWidth: 8, SorterArrowHeight: 4.5, SorterGap: 3,
		// filter.js: fontSizeSM icon, headerIconColor idle, colorIcon and
		// headerFilterHoverBg (colorFillContent) on hover, borderRadius.
		FilterIconSize: t.FontSizeSM, FilterIdleColor: idle, FilterHoverColor: t.TextTertiary,
		FilterHoverBg: t.FillSecondary, FilterRadius: t.Radius,
		FilterDropdownBg: t.BgContainer, FilterDropdownRadius: t.Radius,
		// expand.js: controlInteractiveSize 16 → box (16/2 − 1) × 2 + 3 =
		// 17 scaled by 16/17, border tableBorderColor, borderRadius,
		// expandIconBg colorBgContainer, text color, colorLinkHover hover.
		ExpandIconSize: 17, ExpandIconScale: 16.0 / 17, ExpandIconRadius: t.Radius,
		ExpandIconBg: t.BgContainer, ExpandIconBorder: t.BorderSecondary,
		ExpandIconColor: t.Text, ExpandIconHoverColor: t.TextLinkHover,
		SelectionColumnWidth: 32, SelectionPaddingInline: 8, ExpandColumnWidth: 48, IndentSize: 15,
		SelectionBorderedExtra: 16, SorterTooltip: true,
		// index.js:55 scrollbarColor tableScrollThumbBg = stickyScrollBarBg
		// (colorTextPlaceholder) on a scroll.y body; size
		// tableScrollThumbSize 8. The native bar has no hover color: the
		// grab color is ui/scroll.go's TextTertiary. (sticky.js's ×.65
		// opacity applies only to the sticky bar.)
		StickyScrollBarBg: t.TextQuaternary, StickyScrollBarHoverBg: t.TextTertiary, StickyScrollBarSize: 8,
		FixedShadowAlpha: t.TableFixedShadowAlpha,
		CellFontSize:     t.FontSize, CellFontSizeMD: t.FontSize, CellFontSizeSM: t.FontSize,
		CellLineHeight:   22,
		CellPaddingBlock: t.TablePaddingV, CellPaddingInline: t.TablePaddingH,
		CellPaddingBlockMD: 12, CellPaddingInlineMD: 8, CellPaddingBlockSM: 8, CellPaddingInlineSM: 8,
	}
	switch t.Flavor {
	case Element:
		// $table-cell-padding large / default / small: 0 16 / 0 12 / 0 8;
		// $table-padding 12 / 8 / 4; $table-font-size small extra-small.
		k.CellPaddingInline = 16
		k.CellPaddingBlockMD, k.CellPaddingInlineMD, k.CellPaddingBlockSM, k.CellPaddingInlineSM = 8, 12, 4, 8
		k.CellFontSizeSM = t.FontSizeSM
		k.CellLineHeight = 23 // .cell line-height 23px
		k.HeaderBorderRadius = 0
		k.HeaderSortActiveBg, k.HeaderSortHoverBg, k.BodySortBg = Color{}, Color{}, Color{}
		k.HeaderSplitColor = Color{}
		k.HeaderGroupBg = t.TableRowHoverBg                      // thead.is-group th: fill-color-light
		k.RowSelectedBg, k.RowSelectedHoverBg = Color{}, Color{} // Element keeps selected rows plain
		k.RowExpandedBg = Color{}                                // expanded-cell-bg-color: fill-color-blank
		// --el-fill-color-lighter (striped rows): #fafafa / dark #1d1d1d.
		k.RowStripedBg = Hex("#fafafa")
		if t.Dark {
			k.RowStripedBg = Hex("#1d1d1d")
		}
		k.CellColor, k.FooterColor = t.TextSecondary, t.TextSecondary
		k.FooterBg = t.TableRowHoverBg
		k.FooterAreaBg, k.FooterAreaColor = Color{}, t.TextSecondary // append slot: no tint
		// sort-caret: 5px borders (10 × 5 triangles), carets 2px apart.
		k.SorterArrowWidth, k.SorterArrowHeight, k.SorterGap = 10, 5, 2
		k.SorterHoverColor = idle
		// column-filter-trigger i: color-info, 14px; no hover change.
		k.FilterIconSize, k.FilterIdleColor, k.FilterHoverColor = 14, t.TextTertiary, t.TextTertiary
		k.FilterHoverBg, k.FilterRadius, k.FilterArrow = Color{}, 0, true
		// .el-table-filter: border-color-lighter, radius 2px.
		k.FilterDropdownRadius, k.FilterDropdownBorder = 2, t.BorderSecondary
		// theme-chalk table-column.scss:18 .el-table-filter: #fff, the
		// overlay surface (BgElevated) also in dark mode.
		k.FilterDropdownBg = t.BgElevated
		k.ExpandIconArrow = true
		k.ExpandIconSize, k.ExpandIconScale = 16, 1
		k.ExpandIconColor, k.ExpandIconHoverColor = t.TextSecondary, t.TextSecondary // __expand-icon: text-color-regular
		k.ExpandedPaddingBlock, k.ExpandedPaddingInline = 20, 50                     // __expanded-cell padding 20px 50px
		k.SelectionColumnWidth, k.SelectionPaddingInline = 48, 14
		k.SelectionBorderedExtra, k.SorterTooltip = 0, false
		k.IndentSize = 16 // el-table indent
		// $scrollbar: text-color-secondary at .3, hover .5; bar 6px.
		k.StickyScrollBarBg, k.StickyScrollBarHoverBg, k.StickyScrollBarSize = Fade(t.TextTertiary, .3), Fade(t.TextTertiary, .5), 6
		// el-table-v2 row-height 50, header-height 50.
		k.VirtualRowHeight, k.VirtualHeaderHeight = 50, 50
		if t.elementControls {
			// common/var.scss $table and $scrollbar.
			k.HeaderColor = t.TextSecondary
			k.StickyScrollBarBg, k.StickyScrollBarHoverBg = Fade(t.TextSecondary, .3), Fade(t.TextSecondary, .5)
		}
	case StyleShadcn:
		// One density (p-2); Small trims the vertical padding.
		k.CellPaddingBlockMD, k.CellPaddingInlineMD = t.TablePaddingV, t.TablePaddingH
		k.CellPaddingBlockSM, k.CellPaddingInlineSM = 6, t.TablePaddingH
		k.CellLineHeight = 20 // text-sm leading 20px
		k.HeaderSplitColor = Color{}
		// No expanded panel in table.tsx; has-aria-expanded:bg-muted/50
		// tints the expanded row: FillAlter is muted/50 (unchanged look).
		k.ExpandIconHoverColor = t.Primary.Hover // no shadcn source: keep the previous hover
		// TableHead text-foreground font-medium; TableFooter bg-muted/50
		// font-medium; selected rows bg-muted (also on hover).
		k.HeaderColor = t.Text
		k.FooterBg, k.FooterFontWeight = t.TableRowHoverBg, 500
		k.FooterAreaBg, k.FooterAreaColor = Color{}, t.TextTertiary // TableCaption mt-4 text-sm text-muted-foreground
		k.RowSelectedBg = t.Primary.Bg
		if t.Shadcn != nil {
			k.HeaderColor = t.Shadcn.Foreground
			k.RowSelectedBg = t.Shadcn.Muted
		}
		k.RowSelectedHoverBg = k.RowSelectedBg
		// data-table wrapper: overflow-hidden rounded-md border.
		k.HeaderBorderRadius, k.BottomRadius = t.Radius, t.Radius
		// No shadcn expand control: keep the unscaled 17px box with the
		// small radius (the Ant metric scaled would read as a circle).
		k.ExpandIconScale, k.ExpandIconRadius = 1, t.RadiusSM
		k.SelectionBorderedExtra = 0                                        // one density: a border adds no padding
		k.FilterDropdownBg, k.FilterDropdownRadius = t.BgElevated, t.Radius // popover: bg-popover rounded-md
		// scroll-area: w-2.5 − border-l − p-px × 2 = 7px, bg-border.
		k.StickyScrollBarBg, k.StickyScrollBarHoverBg, k.StickyScrollBarSize = t.BorderSecondary, t.BorderSecondary, 7
		if t.Shadcn != nil {
			k.StickyScrollBarBg, k.StickyScrollBarHoverBg = t.Shadcn.Border, t.Shadcn.Border
		}
	}
	return k
}
