package theme

// PaginationToken styles Pagination. The Item* colors are the plain pager
// (Ant's default; Element without background, shadcn's ghost / outline
// links); the ItemFilled* colors are the background style (Element's
// default, Background(true)).
type PaginationToken struct {
	Alias
	ItemSize              float32
	ItemSizeSM            float32
	ItemBg                Color // inactive page (zero: none)
	ItemColor             Color `contrast:"text:@Page"`
	ItemHoverBg           Color // zero: none
	ItemHoverColor        Color
	ItemActiveBg          Color // zero: none
	ItemActiveColor       Color `contrast:"text:@Page"`
	ItemActiveBorderColor Color // zero: none
	ItemActiveBgDisabled  Color // zero: none
	ItemInputBg           Color
	ItemLinkBg            Color

	ItemFilledBg          Color
	ItemFilledColor       Color `contrast:"text:ItemFilledBg"`
	ItemFilledActiveBg    Color
	ItemFilledActiveColor Color `contrast:"text:ItemFilledActiveBg"`
}

func derivePagination(t *Theme) PaginationToken {
	k := PaginationToken{
		ItemSize: t.ControlHeight, ItemSizeSM: t.ControlHeightSM,
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
	case StyleShadcn:
		// Ghost links, the current page an outline button.
		k.ItemActiveColor, k.ItemActiveBorderColor = t.Text, t.Border
	default:
		k.ItemSize, k.ItemSizeSM = 32, 24
	}
	return k
}
