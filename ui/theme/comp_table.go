package theme

// TableToken styles Table.
type TableToken struct {
	Alias
	HeaderBg           Color
	HeaderColor        Color `contrast:"text:HeaderBg"`
	HeaderFontWeight   int
	HeaderSortActiveBg Color   // header of the sorted / hovered sortable column (zero: none)
	HeaderSplitColor   Color   // short separator between header cells
	HeaderBorderRadius float32 // top corners of the header (and the bordered frame)
	BodySortBg         Color   // body cells of the sorted column (zero: none)
	RowHoverBg         Color
	RowSelectedBg      Color // zero: selected rows are not tinted (Element)
	RowSelectedHoverBg Color // zero: RowSelectedBg
	RowCurrentBg       Color // HighlightCurrentRow
	RowStripedBg       Color // every other row of a Striped table
	CellColor          Color `contrast:"text:@Page"` // text of plain body cells
	BorderColor        Color // row lines, cell borders and the outer border
	FooterBg           Color // summary cells
	FooterColor        Color `contrast:"text:FooterBg"`
	SorterIdleColor    Color // sorter / filter icon when inactive
	FixedShadowAlpha   float64
	CellFontSize       float32
	CellFontSizeSM     float32 // Small size
	// Cell paddings per size: the plain fields are the default (Ant
	// "large") density, MD the Middle size and SM the Small size. Element's
	// default density is MD; its Large size uses the plain fields.
	CellPaddingBlock    float32
	CellPaddingInline   float32
	CellPaddingBlockMD  float32
	CellPaddingInlineMD float32
	CellPaddingBlockSM  float32
	CellPaddingInlineSM float32
}

func deriveTable(t *Theme) TableToken {
	k := TableToken{
		HeaderBg: t.TableHeaderBg, HeaderColor: t.TableHeaderText, HeaderFontWeight: t.TableHeaderWeight,
		HeaderSortActiveBg: t.TableHeaderActiveBg, HeaderSplitColor: t.BorderSecondary, HeaderBorderRadius: t.RadiusLG,
		BodySortBg: t.TableSortedBg, RowHoverBg: t.TableRowHoverBg,
		RowSelectedBg: t.Primary.Bg, RowSelectedHoverBg: t.Primary.BgHover, RowCurrentBg: t.Primary.Bg,
		RowStripedBg: t.FillAlter, CellColor: t.Text,
		BorderColor: t.BorderSecondary, FooterBg: t.BgContainer, FooterColor: t.Text,
		SorterIdleColor: t.TableSorterIdle, FixedShadowAlpha: t.TableFixedShadowAlpha,
		CellFontSize: t.FontSize, CellFontSizeSM: t.FontSize,
		CellPaddingBlock: t.TablePaddingV, CellPaddingInline: t.TablePaddingH,
		CellPaddingBlockMD: 12, CellPaddingInlineMD: 8, CellPaddingBlockSM: 8, CellPaddingInlineSM: 8,
	}
	switch t.Flavor {
	case Element:
		k.CellPaddingInline = 16
		k.CellPaddingBlockMD, k.CellPaddingInlineMD, k.CellPaddingBlockSM, k.CellPaddingInlineSM = 8, 12, 4, 8
		k.CellFontSizeSM = t.FontSizeSM
		k.HeaderBorderRadius = 0
		k.RowSelectedBg, k.RowSelectedHoverBg = Color{}, Color{} // Element keeps selected rows plain
		k.CellColor, k.FooterColor = t.TextSecondary, t.TextSecondary
		k.FooterBg = t.TableRowHoverBg
	case StyleShadcn:
		// One density (p-2); Small trims the vertical padding.
		k.CellPaddingBlockMD, k.CellPaddingInlineMD = t.TablePaddingV, t.TablePaddingH
		k.CellPaddingBlockSM, k.CellPaddingInlineSM = 6, t.TablePaddingH
		k.RowSelectedBg, k.RowSelectedHoverBg = t.Primary.Bg, t.Primary.Bg // selected rows stay bg-muted on hover
	}
	return k
}
