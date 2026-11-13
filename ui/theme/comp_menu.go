package theme

import "github.com/tangoui/tango/ui/paint"

// MenuToken styles Menu and the ProLayout navigation. The Dark* fields
// color the dark sider (and the pop-out submenus of a dark Menu). The
// Popup* fields style the pop-out submenus of a light Menu (vertical,
// horizontal, collapsed); they default to the Dropdown look but are the
// Menu's own, so a Dropdown override does not reach them.
type MenuToken struct {
	Alias
	ItemHeight        float32
	ItemBorderRadius  float32
	ItemMarginInline  float32
	IconSize          float32
	ItemBg            Color
	SubMenuItemBg     Color // open inline submenu (zero: none)
	ItemColor         Color `contrast:"text:ItemBg"`
	ItemHoverBg       Color // zero: none
	ItemHoverColor    Color
	ItemSelectedBg    Color // zero: none
	ItemSelectedColor Color `contrast:"text:ItemSelectedBg"`

	DarkItemBg            Color
	DarkSubMenuItemBg     Color // zero: none
	DarkItemColor         Color `contrast:"text:DarkItemBg"`
	DarkItemHoverBg       Color // zero: none
	DarkItemHoverColor    Color // text of a hovered dark item
	DarkItemSelectedBg    Color // zero: none
	DarkItemSelectedColor Color `contrast:"text:DarkItemSelectedBg"`

	PopupBg                   Color
	PopupBorderColor          Color // zero: none
	PopupRadius               float32
	PopupShadow               []paint.Shadow
	PopupPaddingBlock         float32 // panel padding
	PopupPaddingInline        float32
	PopupItemHeight           float32
	PopupItemPaddingInline    float32
	PopupItemBorderRadius     float32
	PopupItemColor            Color `contrast:"text:PopupBg"`
	PopupItemHoverBg          Color // zero: none
	PopupItemHoverColor       Color `contrast:"text:PopupItemHoverBg"`
	PopupItemSelectedBg       Color // zero: none
	PopupItemSelectedColor    Color `contrast:"text:PopupItemSelectedBg"`
	PopupItemIconColor        Color // zero: the label color
	PopupIconSize             float32
	PopupDangerColor          Color `contrast:"text:PopupBg"`
	PopupDangerHoverBg        Color // zero: the regular hover
	PopupDangerHoverColor     Color `contrast:"text:PopupDangerHoverBg"`
	PopupArrowColor           Color `contrast:"nontext:PopupBg"` // submenu chevron
	PopupGroupTitleHeight     float32
	PopupGroupTitleFontSize   float32
	PopupGroupTitleFontWeight int
	PopupGroupTitleColor      Color `contrast:"text:PopupBg"`
	PopupDividerColor         Color
	// Shortcut column of pop-out submenu items (see DropdownToken).
	PopupShortcutColor         Color `contrast:"text:PopupBg"`
	PopupShortcutFontSize      float32
	PopupShortcutLetterSpacing float32
	PopupShortcutGap           float32
}

func deriveMenu(t *Theme) MenuToken {
	white := Hex("#ffffff")
	k := MenuToken{
		ItemHeight: t.MenuItemHeight, ItemBorderRadius: t.RadiusLG, ItemMarginInline: 4, IconSize: 14,
		ItemBg: t.BgContainer, ItemColor: t.Text,
		ItemHoverBg: t.ItemHoverBg, ItemHoverColor: t.Text,
		ItemSelectedBg: t.ItemSelectedBg, ItemSelectedColor: t.Primary.Base,
		DarkItemBg: t.SiderBg, DarkSubMenuItemBg: t.SiderSubmenuBg, DarkItemColor: t.SiderText,
		DarkItemHoverColor: white, DarkItemSelectedBg: t.Primary.Base, DarkItemSelectedColor: white,
	}
	// Pop-out submenus start from the Dropdown panel look of the same
	// design system (deriveDropdown holds its flavor differences).
	d := deriveDropdown(t)
	k.PopupBg, k.PopupBorderColor, k.PopupRadius, k.PopupShadow = d.PopupBg, d.PopupBorderColor, d.PopupRadius, d.PopupShadow
	k.PopupPaddingBlock, k.PopupPaddingInline = d.PaddingBlock, d.PaddingInline
	k.PopupItemHeight, k.PopupItemPaddingInline, k.PopupItemBorderRadius = d.ItemHeight, d.ItemPaddingInline, d.ItemBorderRadius
	k.PopupItemColor, k.PopupItemHoverBg, k.PopupItemHoverColor = d.ItemColor, d.ItemHoverBg, d.ItemHoverColor
	k.PopupItemSelectedBg, k.PopupItemSelectedColor = d.ItemSelectedBg, d.ItemSelectedColor
	k.PopupItemIconColor, k.PopupIconSize = d.ItemIconColor, d.IconSize
	k.PopupDangerColor, k.PopupDangerHoverBg, k.PopupDangerHoverColor = d.DangerColor, d.DangerHoverBg, d.DangerHoverColor
	k.PopupArrowColor = d.SubmenuArrowColor
	k.PopupGroupTitleHeight, k.PopupGroupTitleFontSize = d.GroupTitleHeight, d.GroupTitleFontSize
	k.PopupGroupTitleFontWeight, k.PopupGroupTitleColor = d.GroupTitleFontWeight, d.GroupTitleColor
	k.PopupDividerColor = d.DividerColor
	k.PopupShortcutColor, k.PopupShortcutFontSize = d.ShortcutColor, d.ShortcutFontSize
	k.PopupShortcutLetterSpacing, k.PopupShortcutGap = d.ShortcutLetterSpacing, d.ShortcutGap
	switch t.Flavor {
	case Element:
		// el-menu: no selected fill, the brand color marks the item.
		k.ItemBorderRadius, k.ItemMarginInline = 0, 0
		k.ItemSelectedBg = Color{}
		k.DarkItemHoverBg, k.DarkItemHoverColor = t.SiderItemHoverBg, t.SiderTextActive
		k.DarkItemSelectedBg, k.DarkItemSelectedColor = Color{}, t.SiderTextActive
	case StyleShadcn:
		// A single Sidebar look (sidebar / sidebar-accent variables); the
		// Dark* fields are not used.
		k.ItemBorderRadius, k.ItemMarginInline, k.IconSize = t.Radius, 8, 16
		k.ItemBg, k.ItemColor, k.ItemHoverColor = t.SiderLightBg, t.SiderText, t.SiderText
		k.ItemHoverBg, k.ItemSelectedBg, k.ItemSelectedColor = t.SiderItemHoverBg, t.SiderItemHoverBg, t.SiderTextActive
	}
	return k
}
