package theme

import (
	"time"

	"github.com/tangoui/tango/ui/paint"
)

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

	// Inline / vertical layout. CollapsedWidth is the width of an
	// inline-collapsed menu (icons only), CollapsedIconSize its icon size.
	// ItemMarginBlock separates items; SubMenuItemHeight is the height of
	// the items of an open inline submenu (zero: ItemHeight; shadcn
	// SidebarMenuSubButton h-7).
	// InlineIndent is the indent per level (Ant inlineIndent, Element
	// --el-menu-level-padding).
	CollapsedWidth    float32
	CollapsedIconSize float32
	ItemMarginBlock   float32
	SubMenuItemHeight float32
	InlineIndent      float32
	// ItemContentInset, when non-zero, is the start inset of every inline
	// item's content instead of the per-level indent (shadcn: nesting
	// comes from the submenu guide). IconGap separates icon and label;
	// ItemSelectedFontWeight is the label weight of the selected item
	// (zero: regular).
	ItemContentInset       float32
	IconGap                float32
	ItemSelectedFontWeight int
	// SubMenuGuideInset, when non-zero, hangs open inline submenus off a
	// guide line SubMenuGuideInset from the menu's start edge (shadcn
	// SidebarMenuSub border-l); their items start SubMenuItemInset from
	// the edge (and end as far from the other edge), the column is padded
	// SubMenuPaddingBlock above and below.
	SubMenuGuideInset   float32
	SubMenuItemInset    float32
	SubMenuPaddingBlock float32
	SubMenuGuideColor   Color
	// GroupTitleNested indents the title of a nested group with its level.
	GroupTitleNested bool
	// Group titles (Ant type="group", shadcn SidebarGroupLabel).
	GroupTitleHeight        float32
	GroupTitleFontSize      float32
	GroupTitleFontWeight    int
	GroupTitleColor         Color `contrast:"text:ItemBg"`
	GroupTitlePaddingInline float32
	// Danger items (MenuItem.Danger; Ant dangerItemColor,
	// dangerItemSelectedBg).
	DangerItemColor      Color `contrast:"text:ItemBg"`
	DangerItemSelectedBg Color // zero: none
	// Dark menu counterparts (antd darkDangerItemColor,
	// darkDangerItemSelectedBg, darkGroupTitleColor).
	DarkDangerItemColor      Color `contrast:"text:DarkItemBg"`
	DarkDangerItemSelectedBg Color // zero: none
	DarkGroupTitleColor      Color `contrast:"text:DarkItemBg"`
	// Extra column (MenuItem.Shortcut shown as Ant's item extra).
	ExtraColor    Color `contrast:"text:ItemBg"`
	ExtraFontSize float32
	// Pop-out submenu timing (Ant subMenuOpenDelay / subMenuCloseDelay,
	// Element show-timeout / hide-timeout, Radix NavigationMenu
	// delayDuration / close timer) and gap to the parent item
	// (PopupOffset: vertical and collapsed; HorizontalPopupOffset:
	// under a horizontal bar).
	SubMenuOpenDelay      time.Duration
	SubMenuCloseDelay     time.Duration
	PopupOffset           float32
	HorizontalPopupOffset float32

	// Horizontal bar (shadcn NavigationMenu triggers). HorizontalActiveBar*
	// is the underline of the active item (zero height: none), inset by
	// HorizontalActiveBarInset from both ends; HorizontalItemHoverBg /
	// HorizontalItemOpenBg / HorizontalItemSelectedBg fill the item (pill
	// of HorizontalItemRadius, zero: none); HorizontalArrowSize is the
	// submenu chevron (zero: none), turned over while open.
	HorizontalHeight            float32
	HorizontalItemPaddingInline float32
	HorizontalItemGap           float32
	HorizontalItemRadius        float32
	HorizontalFontWeight        int
	HorizontalItemHoverBg       Color
	HorizontalItemOpenBg        Color
	HorizontalItemSelectedBg    Color
	HorizontalActiveBarHeight   float32
	HorizontalActiveBarInset    float32
	HorizontalArrowSize         float32
	HorizontalArrowGap          float32 // between label and chevron
	HorizontalArrowTurn         bool    // turn the chevron over while open
	HorizontalBg                Color   // bar background (zero: ItemBg)
	HorizontalBorderColor       Color   // rule under a light bar (zero: none)

	// DarkTheme reports whether Dark() (Defaults theme "dark") switches
	// to the Dark* colors; false ignores it (shadcn: a single Sidebar look
	// that follows the light / dark theme).
	DarkTheme bool

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
	PopupArrowSize            float32
	PopupArrowInset           float32 // chevron distance from the item's inline end
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
		// antd 6.7 es/menu/style/index.js prepareComponentToken:
		// collapsedWidth controlHeightLG*2, collapsedIconSize fontSizeLG,
		// itemMarginBlock marginXXS, subMenuItemBg colorFillAlter, group
		// title padding paddingXS padding (lineHeight 22), colorTextDescription,
		// dangerItemColor colorError, dangerItemSelectedBg colorErrorBg,
		// horizontalLineHeight controlHeightLG*1.15, itemPaddingInline
		// padding, activeBarHeight lineWidthBold; @rc-component/menu 1.6.1
		// es/Menu.js subMenuOpenDelay / subMenuCloseDelay 0.1, inlineIndent 24.
		CollapsedWidth: 80, CollapsedIconSize: t.FontSizeLG, ItemMarginBlock: 4,
		DarkTheme: true, InlineIndent: 24, SubMenuItemBg: t.FillAlter, IconGap: 10, GroupTitleNested: true,
		GroupTitleHeight: 38, GroupTitleFontSize: t.FontSize, GroupTitleFontWeight: 400,
		GroupTitleColor: t.TextTertiary, GroupTitlePaddingInline: 16,
		DangerItemColor: t.Error.Base, DangerItemSelectedBg: t.Error.Bg,
		DarkDangerItemColor: t.Error.Base, DarkDangerItemSelectedBg: t.Error.Base, DarkGroupTitleColor: Fade(t.SiderText, .7),
		HorizontalArrowGap: 6,
		ExtraColor:         t.TextTertiary, ExtraFontSize: t.FontSize,
		SubMenuOpenDelay: 100 * time.Millisecond, SubMenuCloseDelay: 100 * time.Millisecond,
		PopupOffset: 4, HorizontalPopupOffset: 4,
		HorizontalHeight: 46, HorizontalItemPaddingInline: 16, HorizontalFontWeight: 400,
		HorizontalActiveBarHeight: 2, HorizontalActiveBarInset: 16, HorizontalBorderColor: t.Split,
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
	// Menu pop-outs keep the Menu's own icon, extra and chevron metrics
	// (antd Menu: iconSize fontSize, item extra paddingInlineStart padding
	// in the item's font size); shadcn below takes the Dropdown ones.
	k.PopupIconSize, k.PopupArrowSize, k.PopupArrowInset = 14, 10, 12
	k.PopupGroupTitleHeight, k.PopupGroupTitleFontSize = d.GroupTitleHeight, d.GroupTitleFontSize
	k.PopupGroupTitleFontWeight, k.PopupGroupTitleColor = d.GroupTitleFontWeight, d.GroupTitleColor
	k.PopupDividerColor = d.DividerColor
	k.PopupShortcutColor, k.PopupShortcutFontSize = d.ShortcutColor, d.ShortcutFontSize
	k.PopupShortcutLetterSpacing = d.ShortcutLetterSpacing
	k.PopupShortcutFontSize, k.PopupShortcutGap = t.FontSize, 16
	switch t.Flavor {
	case Element:
		// el-menu: no selected fill, the brand color marks the item.
		k.ItemBorderRadius, k.ItemMarginInline = 0, 0
		k.ItemSelectedBg = Color{}
		k.DarkItemHoverBg, k.DarkItemHoverColor = t.SiderItemHoverBg, t.SiderTextActive
		k.DarkItemSelectedBg, k.DarkItemSelectedColor = Color{}, t.SiderTextActive
		// el-menu (theme-chalk/src/menu.scss, es/components/menu/src/menu.mjs):
		// no item margin, --el-menu-level-padding 20, collapsed width
		// icon-width + 2 × base-level-padding (64), item-group title 12px
		// secondary text (30 high), show-timeout / hide-timeout 300,
		// popper-offset 6, --el-menu-horizontal-height 60 with a full-width
		// 2px bar and a hover fill, no submenu background.
		k.CollapsedWidth, k.ItemMarginBlock, k.InlineIndent, k.SubMenuItemBg = t.SiderCollapsed, 0, 20, Color{}
		k.GroupTitleHeight, k.GroupTitleFontSize, k.GroupTitleColor, k.GroupTitlePaddingInline = 30, t.FontSizeSM, t.TextSecondary, 20
		k.DangerItemSelectedBg = Color{}
		k.SubMenuOpenDelay, k.SubMenuCloseDelay = 300*time.Millisecond, 300*time.Millisecond
		k.PopupOffset, k.HorizontalPopupOffset = 6, 6
		k.HorizontalHeight, k.HorizontalItemPaddingInline, k.HorizontalActiveBarInset = 60, 20, 0
		k.HorizontalItemHoverBg, k.HorizontalArrowSize = t.ItemHoverBg, 12
		k.DarkDangerItemSelectedBg = Color{}
		k.PopupShortcutGap = 24
	case StyleShadcn:
		// A single Sidebar look (sidebar / sidebar-accent variables); the
		// Dark* fields are not used.
		k.ItemBorderRadius, k.ItemMarginInline, k.IconSize = t.Radius, 8, 16
		k.ItemBg, k.ItemColor, k.ItemHoverColor = t.SiderLightBg, t.SiderText, t.SiderText
		k.ItemHoverBg, k.ItemSelectedBg, k.ItemSelectedColor = t.SiderItemHoverBg, t.SiderItemHoverBg, t.SiderTextActive
		// sidebar.tsx: SidebarMenu gap-1, SidebarMenuSubButton h-7,
		// SidebarGroupLabel h-8 px-2 text-xs font-medium
		// text-sidebar-foreground/70 (inside the group's p-2), collapsed to
		// --sidebar-width-icon 3rem with size-4 icons; no submenu fill.
		k.CollapsedWidth, k.CollapsedIconSize, k.ItemMarginBlock, k.SubMenuItemHeight = t.SiderCollapsed, 16, 2, 28
		// SidebarMenuButton p-2 gap-2, active font-medium; SidebarMenuSub
		// mx-3.5 border-l px-2.5 py-0.5 inside the group's p-2: the guide
		// at 8+14 = 22, sub buttons from 22+1+10 = 33.
		k.ItemContentInset, k.IconGap, k.ItemSelectedFontWeight = 8, 8, 500
		k.SubMenuGuideInset, k.SubMenuItemInset, k.SubMenuPaddingBlock, k.SubMenuGuideColor = 22, 33, 2, t.Split
		k.GroupTitleNested = false
		k.DarkTheme = false
		k.SubMenuItemBg = Color{}
		k.GroupTitleHeight, k.GroupTitleFontSize, k.GroupTitleFontWeight = 32, t.FontSizeSM, 500
		k.GroupTitleColor, k.GroupTitlePaddingInline = Fade(t.SiderText, .7), 16
		k.ExtraFontSize = t.FontSizeSM
		k.PopupIconSize, k.PopupArrowSize, k.PopupArrowInset = d.IconSize, d.SubmenuArrowSize, d.SubmenuArrowInset
		k.PopupShortcutFontSize, k.PopupShortcutGap = d.ShortcutFontSize, d.ShortcutGap
		// navigation-menu.tsx: triggers h-9 rounded-md px-4 py-2 text-sm
		// font-medium, gap-1, hover bg-accent, open bg-accent/50, ChevronDown
		// size-3 turned 180° while open; the content panel mt-1.5. Radix
		// NavigationMenu 1.2.14: delayDuration 200, close timer 150 ms.
		k.SubMenuOpenDelay, k.SubMenuCloseDelay = 200*time.Millisecond, 150*time.Millisecond
		k.HorizontalPopupOffset = 6
		k.HorizontalHeight, k.HorizontalItemPaddingInline, k.HorizontalItemGap = 36, 16, 4
		k.HorizontalItemRadius, k.HorizontalFontWeight = t.Radius, 500
		// The active link is bg-accent/50 like an open trigger; the chevron
		// is ml-1 and turns 180° while open; the bar sits on the page.
		k.HorizontalItemHoverBg, k.HorizontalItemOpenBg, k.HorizontalItemSelectedBg = t.ItemHoverBg, Fade(t.ItemHoverBg, .5), Fade(t.ItemHoverBg, .5)
		k.HorizontalActiveBarHeight, k.HorizontalArrowSize, k.HorizontalBorderColor = 0, 12, Color{}
		k.HorizontalArrowGap, k.HorizontalArrowTurn, k.HorizontalBg = 4, true, t.BgContainer
	}
	return k
}
