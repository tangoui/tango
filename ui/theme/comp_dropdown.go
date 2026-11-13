package theme

import "github.com/tangoui/tango/ui/paint"

// DropdownToken styles Dropdown menus and the pop-out panels they share
// with Menu.
type DropdownToken struct {
	Alias
	PopupBg              Color
	PopupBorderColor     Color // zero: none
	PopupRadius          float32
	PopupShadow          []paint.Shadow
	PaddingBlock         float32 // panel padding
	PaddingInline        float32
	ItemHeight           float32
	ItemPaddingInline    float32
	ItemBorderRadius     float32
	ItemColor            Color `contrast:"text:PopupBg"`
	ItemHoverBg          Color // zero: none
	ItemHoverColor       Color `contrast:"text:ItemHoverBg"`
	ItemSelectedBg       Color // zero: none
	ItemSelectedColor    Color `contrast:"text:ItemSelectedBg"`
	ItemIconColor        Color // zero: the label color
	IconSize             float32
	DangerColor          Color `contrast:"text:PopupBg"`
	DangerHoverBg        Color // zero: the regular hover
	DangerHoverColor     Color `contrast:"text:DangerHoverBg"`
	SubmenuArrowColor    Color `contrast:"nontext:PopupBg"`
	GroupTitleHeight     float32
	GroupTitleFontSize   float32
	GroupTitleFontWeight int
	GroupTitleColor      Color `contrast:"text:PopupBg"`
	DividerColor         Color
	// Shortcut column (MenuItem.Shortcut): drawn at the item's inline end
	// in its own color and size, at least ShortcutGap after the label.
	// ShortcutLetterSpacing adds tracking after every glyph (CSS
	// letter-spacing; shadcn tracking-widest).
	ShortcutColor         Color `contrast:"text:PopupBg"`
	ShortcutFontSize      float32
	ShortcutLetterSpacing float32
	ShortcutGap           float32
}

func deriveDropdown(t *Theme) DropdownToken {
	k := DropdownToken{
		PopupBg: t.BgElevated, PopupRadius: t.RadiusLG, PopupShadow: t.Shadow,
		PaddingBlock: 4, PaddingInline: 4,
		ItemHeight: 32, ItemPaddingInline: 12, ItemBorderRadius: t.RadiusSM,
		ItemColor: t.Text, ItemHoverBg: t.ItemHoverBg, ItemHoverColor: t.Text,
		ItemSelectedBg: t.Primary.Bg, ItemSelectedColor: t.Primary.Base,
		IconSize:    14,
		DangerColor: t.Error.Base, DangerHoverBg: t.Error.Base, DangerHoverColor: t.TextOnPrimary,
		SubmenuArrowColor: t.TextTertiary,
		GroupTitleHeight:  32, GroupTitleFontSize: t.FontSize, GroupTitleFontWeight: 400, GroupTitleColor: t.TextTertiary,
		DividerColor:  t.Split,
		ShortcutColor: t.TextTertiary, ShortcutFontSize: t.FontSize, ShortcutGap: 24,
	}
	switch t.Flavor {
	case Element:
		// el-dropdown-menu: bordered panel, 16px items tinted with the brand
		// under the pointer, no selected fill, plain danger items.
		k.PopupBorderColor, k.PopupRadius = t.BorderSecondary, t.Radius
		k.PaddingBlock, k.PaddingInline = 5, 0
		k.ItemPaddingInline, k.ItemBorderRadius = 16, 0
		k.ItemColor, k.ItemHoverBg, k.ItemHoverColor = t.TextSecondary, t.Primary.Bg, t.Primary.Base
		k.ItemSelectedBg = Color{}
		k.DangerHoverBg, k.DangerHoverColor = Color{}, Color{}
		k.GroupTitleHeight, k.GroupTitleFontSize = 30, t.FontSizeSM
	case StyleShadcn:
		// DropdownMenuContent: bordered rounded-md panel, px-2 items,
		// muted size-4 icons, a destructive tint on danger items, medium
		// labels.
		k.PopupBorderColor, k.PopupRadius = t.PopupBorder, t.Radius
		k.ItemPaddingInline = 8
		k.ItemSelectedBg, k.ItemSelectedColor = t.ItemSelectedBg, t.ItemSelected
		k.ItemIconColor, k.IconSize = t.TextTertiary, 16
		k.DangerHoverBg, k.DangerHoverColor = t.Error.Bg, t.Error.Base
		k.GroupTitleFontWeight, k.GroupTitleColor = 500, t.Text
		// DropdownMenuShortcut: ml-auto text-xs tracking-widest
		// text-muted-foreground, after the item's gap-2.
		k.ShortcutFontSize, k.ShortcutLetterSpacing, k.ShortcutGap = t.FontSizeSM, t.FontSizeSM/10, 8
	}
	return k
}
