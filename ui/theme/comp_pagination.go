package theme

// PaginationToken styles Pagination. The Item* colors are the plain pager
// (Ant's default; Element without background, shadcn's ghost / outline
// links); the ItemFilled* colors are the background style (Element's
// default, Background(true)).
type PaginationToken struct {
	Alias
	ItemSize                float32
	ItemSizeSM              float32
	ItemSizeLG              float32 // Size(Large)
	ItemSpacing             float32 // between the buttons (Ant marginXS)
	ItemSpacingSM           float32
	ItemSpacingLG           float32
	TotalMarginEnd          float32 // extra space after the total text
	QuickJumperInputWidth   float32 // "go to" box
	QuickJumperInputWidthSM float32
	ItemBg                  Color // inactive page (zero: none)
	ItemColor               Color `contrast:"text:@Page"`
	ItemHoverBg             Color // zero: none
	ItemHoverColor          Color
	ItemActiveBg            Color // zero: none
	ItemActiveColor         Color `contrast:"text:@Page"`
	ItemActiveBorderColor   Color // zero: none
	ItemActiveColorHover    Color // hovered current page (zero: ItemActiveColor)
	ItemActiveBorderHover   Color // hovered current page outline (zero: ItemActiveBorderColor)
	EllipsisColor           Color // the ••• of the jump buttons
	JumpHoverArrow          bool  // hovering a ••• jump shows a double chevron
	JumpArrowColor          Color // that chevron
	ItemActiveBgDisabled    Color // zero: none
	ItemInputBg             Color
	ItemLinkBg              Color

	ItemFilledBg          Color
	ItemFilledColor       Color `contrast:"text:ItemFilledBg"`
	ItemFilledActiveBg    Color
	ItemFilledActiveColor Color `contrast:"text:ItemFilledActiveBg"`

	Background bool // filled buttons by default (Element background)
	PagerCount int  // page buttons at most (Element pager-count; 0: Ant's window)
	// PageSizesByTen gives the size changer 10/20/30/40/50/100 (Element
	// page-sizes) instead of 10/20/50/100 (rc-pagination Options.js).
	PageSizesByTen bool
}

func derivePagination(t *Theme) PaginationToken {
	k := PaginationToken{
		ItemSize: t.ControlHeight, ItemSizeSM: t.ControlHeightSM, ItemSizeLG: t.ControlHeightLG,
		// antd es/pagination/style/index.js: item-spacing-actual marginXS /
		// small marginXXS / large marginSM; quickJumperInputWidth
		// controlHeightLG*1.25, small controlHeightLG*1.1.
		ItemSpacing: 8, ItemSpacingSM: 4, ItemSpacingLG: 12,
		QuickJumperInputWidth: t.ControlHeightLG * 1.25, QuickJumperInputWidthSM: t.ControlHeightLG * 1.1,
		ItemActiveColorHover: t.Primary.Hover, ItemActiveBorderHover: t.Primary.Hover,
		// jump-prev / jump-next: the ellipsis in colorTextDisabled, a
		// colorPrimary double arrow on hover.
		EllipsisColor: t.TextDisabled, JumpHoverArrow: true, JumpArrowColor: t.Primary.Base,
		ItemColor: t.Text, ItemHoverBg: t.BgTextHover, ItemHoverColor: t.Text,
		ItemActiveBg: t.BgContainer, ItemActiveColor: t.Primary.Base, ItemActiveBorderColor: t.Primary.Base,
		ItemActiveBgDisabled: t.BgContainerDisabled, ItemInputBg: t.ControlBg, ItemLinkBg: t.BgContainer,
		ItemFilledBg: t.FillSecondary, ItemFilledColor: t.Text,
		ItemFilledActiveBg: t.Primary.Base, ItemFilledActiveColor: t.TextOnPrimary,
	}
	switch t.Flavor {
	case Element:
		// el-pagination: 32px buttons, 24px when small ($pagination
		// button-height(-small) in theme-chalk var.scss); without
		// background the current page is only colored, the hovered one too.
		k.ItemSize, k.ItemSizeSM = 32, 24
		k.ItemHoverBg, k.ItemHoverColor = Color{}, t.Primary.Base
		k.ItemActiveBg, k.ItemActiveBorderColor, k.ItemActiveBgDisabled = Color{}, Color{}, Color{}
		k.ItemFilledColor = t.TextSecondary
		k.ItemActiveColorHover, k.ItemActiveBorderHover = Color{}, Color{}
		// el-pagination: background buttons margin 0 4px, the parts
		// --el-pagination-item-gap 16px apart; large 40px buttons;
		// jumper editor 56px; pager-count 7; page-sizes [10..100]; the
		// "more" button in the text color, a d-arrow on hover.
		k.ItemSizeLG = 40
		k.ItemSpacing, k.ItemSpacingSM, k.ItemSpacingLG = 8, 8, 8
		k.TotalMarginEnd = 8
		k.QuickJumperInputWidth, k.QuickJumperInputWidthSM = 56, 56
		k.EllipsisColor = t.Text
		k.Background, k.PagerCount = true, 7
		k.PageSizesByTen = true
	case StyleShadcn:
		// Ghost links, the current page an outline button.
		k.ItemActiveColor, k.ItemActiveBorderColor = t.Text, t.Border
		k.ItemActiveColorHover, k.ItemActiveBorderHover = Color{}, Color{}
		// pagination.tsx: PaginationContent gap-1; PaginationEllipsis a
		// plain MoreHorizontal (no hover arrow).
		k.ItemSpacing, k.ItemSpacingSM, k.ItemSpacingLG = 4, 4, 4
		k.EllipsisColor, k.JumpHoverArrow = t.Text, false
	default:
		k.ItemSize, k.ItemSizeSM = 32, 24
	}
	return k
}
