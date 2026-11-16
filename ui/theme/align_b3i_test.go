package theme

import (
	"testing"
	"time"
)

// Batch 3, group I (docs/specs/align/3-menu.md): token values read from
// the reference sources (2026-11-15). Every expected value is the source
// value; paths are relative to /tmp/tango-ref.

func TestAlignB3IMenuTokens(t *testing.T) {
	ant := AntDesign()
	m := ant.Components.Menu
	// antd/package/es/menu/style/index.js prepareComponentToken;
	// rc-menu/package/es/Menu.js defaults.
	for _, c := range []struct {
		name      string
		got, want float32
	}{
		{"collapsedWidth", m.CollapsedWidth, 80},                 // controlHeightLG * 2
		{"collapsedIconSize", m.CollapsedIconSize, 16},           // fontSizeLG
		{"itemMarginBlock", m.ItemMarginBlock, 4},                // marginXXS
		{"itemMarginInline", m.ItemMarginInline, 4},              // marginXXS
		{"inlineIndent", m.InlineIndent, 24},                     // rc-menu inlineIndent
		{"groupTitleHeight", m.GroupTitleHeight, 38},             // paddingXS*2 + 22
		{"horizontalLineHeight", m.HorizontalHeight, 46},         // controlHeightLG * 1.15
		{"itemPaddingInline", m.HorizontalItemPaddingInline, 16}, // padding
		{"activeBarHeight", m.HorizontalActiveBarHeight, 2},      // lineWidthBold
	} {
		if c.got != c.want {
			t.Errorf("antd Menu %s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if m.SubMenuItemBg != ant.FillAlter {
		t.Errorf("antd Menu subMenuItemBg = %v, want colorFillAlter", m.SubMenuItemBg)
	}
	if m.DangerItemColor != ant.Error.Base || m.DangerItemSelectedBg != ant.Error.Bg {
		t.Error("antd Menu danger colors: want colorError / colorErrorBg")
	}
	if m.SubMenuOpenDelay != 100*time.Millisecond || m.SubMenuCloseDelay != 100*time.Millisecond {
		t.Errorf("antd subMenuOpenDelay / subMenuCloseDelay = %v / %v, want 0.1 s", m.SubMenuOpenDelay, m.SubMenuCloseDelay)
	}
	if m.PopupIconSize != 14 || m.PopupShortcutGap != 16 {
		t.Errorf("antd Menu popup icon %v extra gap %v, want 14 / 16 (iconSize fontSize, extra padding)", m.PopupIconSize, m.PopupShortcutGap)
	}

	// ep/package/es/components/menu/src/menu.mjs, theme-chalk/src/menu.scss.
	el := ElementPlus().Components.Menu
	if el.SubMenuOpenDelay != 300*time.Millisecond || el.SubMenuCloseDelay != 300*time.Millisecond {
		t.Errorf("Element show-timeout / hide-timeout = %v / %v, want 300 ms", el.SubMenuOpenDelay, el.SubMenuCloseDelay)
	}
	if el.PopupOffset != 6 || el.HorizontalPopupOffset != 6 {
		t.Errorf("Element popper-offset = %v / %v, want 6", el.PopupOffset, el.HorizontalPopupOffset)
	}
	if el.HorizontalHeight != 60 || el.InlineIndent != 20 || el.CollapsedWidth != 64 || el.ItemHeight != 56 {
		t.Errorf("Element menu metrics %v %v %v %v, want 60 / 20 / 64 / 56", el.HorizontalHeight, el.InlineIndent, el.CollapsedWidth, el.ItemHeight)
	}

	// shadcn sidebar.tsx / navigation-menu.tsx; radix-nav (Radix
	// NavigationMenu 1.2.14) dist/index.mjs.
	sh := Shadcn()
	s := sh.Components.Menu
	if m.SubMenuItemHeight != 0 || s.SubMenuGuideInset != 22 || s.SubMenuItemInset != 33 || s.IconGap != 8 || s.ItemSelectedFontWeight != 500 {
		t.Error("Sidebar sub column: antd sub items follow ItemHeight; shadcn guide 8+14, items 8+14+1+10, gap-2, active font-medium")
	}
	if !s.HorizontalArrowTurn || el.HorizontalArrowTurn || s.HorizontalArrowGap != 4 || s.HorizontalItemSelectedBg != Fade(sh.ItemHoverBg, .5) {
		t.Error("NavigationMenu chevron ml-1 rotate-180 (Element still), active link bg-accent/50")
	}
	if m.DarkDangerItemColor != ant.Error.Base || m.DarkDangerItemSelectedBg != ant.Error.Base {
		t.Error("antd darkDangerItemColor / darkDangerItemSelectedBg: want colorError")
	}
	if s.CollapsedWidth != 48 || s.ItemHeight != 32 || s.SubMenuItemHeight != 28 || s.ItemMarginBlock != 2 || s.ItemMarginInline != 8 {
		t.Errorf("shadcn Sidebar metrics %v %v %v %v %v, want 48 (3rem) / 32 (h-8) / 28 (h-7) / 2 (gap-1) / 8 (p-2)",
			s.CollapsedWidth, s.ItemHeight, s.SubMenuItemHeight, s.ItemMarginBlock, s.ItemMarginInline)
	}
	if s.GroupTitleHeight != 32 || s.GroupTitleFontSize != 12 || s.GroupTitleFontWeight != 500 || s.GroupTitleColor != Fade(sh.SiderText, .7) {
		t.Error("shadcn SidebarGroupLabel: want h-8 text-xs font-medium text-sidebar-foreground/70")
	}
	if s.HorizontalHeight != 36 || s.HorizontalItemPaddingInline != 16 || s.HorizontalItemGap != 4 || s.HorizontalFontWeight != 500 ||
		s.HorizontalItemRadius != sh.Radius || s.HorizontalArrowSize != 12 || s.HorizontalPopupOffset != 6 {
		t.Error("shadcn NavigationMenu: want h-9 px-4 gap-1 font-medium rounded-md, chevron size-3, mt-1.5")
	}
	if s.SubMenuOpenDelay != 200*time.Millisecond || s.SubMenuCloseDelay != 150*time.Millisecond {
		t.Errorf("Radix NavigationMenu delays %v / %v, want 200 / 150 ms", s.SubMenuOpenDelay, s.SubMenuCloseDelay)
	}
}

func TestAlignB3IDropdownTokens(t *testing.T) {
	// antd/package/es/dropdown/style/index.js, es/dropdown/dropdown.js.
	a := AntDesign().Components.Dropdown
	if a.IconSize != 12 || a.ShortcutFontSize != 12 || a.ShortcutGap != 16 {
		t.Errorf("antd dropdown icon %v extra %v / %v, want fontSizeSM 12, extra fontSizeSM after padding 16", a.IconSize, a.ShortcutFontSize, a.ShortcutGap)
	}
	if a.SubmenuArrowSize != 12 || a.SubmenuArrowInset != 8 {
		t.Errorf("antd submenu expand icon %v at %v, want fontSizeIcon 12 at paddingXS 8", a.SubmenuArrowSize, a.SubmenuArrowInset)
	}
	if a.MouseEnterDelay != 150*time.Millisecond || a.MouseLeaveDelay != 100*time.Millisecond || a.PopupOffset != 4 {
		t.Errorf("antd mouseEnterDelay / mouseLeaveDelay / offset = %v / %v / %v", a.MouseEnterDelay, a.MouseLeaveDelay, a.PopupOffset)
	}
	if a.ItemHeight != 32 || a.PaddingBlock != 4 { // paddingBlock (32-22)/2 + 22, dropdownEdgeChildPadding paddingXXS
		t.Errorf("antd dropdown item %v padding %v", a.ItemHeight, a.PaddingBlock)
	}
	// ep/package/es/components/dropdown/src/dropdown.mjs.
	e := ElementPlus().Components.Dropdown
	if e.MouseEnterDelay != 150*time.Millisecond || e.MouseLeaveDelay != 150*time.Millisecond {
		t.Errorf("Element show-timeout / hide-timeout = %v / %v, want 150 / 150", e.MouseEnterDelay, e.MouseLeaveDelay)
	}
	// shadcn dropdown-menu.tsx.
	s := Shadcn().Components.Dropdown
	if s.SubmenuArrowSize != 16 || s.IndicatorSize != 16 || s.IndicatorWidth != 24 || s.PopupOffset != 4 || s.IconSize != 16 {
		t.Error("shadcn DropdownMenu: want chevron size-4, indicator size-4 in pl-8, sideOffset 4, size-4 icons")
	}
}

func TestAlignB3IMenubarTokens(t *testing.T) {
	// shadcn menubar.tsx.
	sh := Shadcn()
	m := sh.Components.Menubar
	if m.PopupMinWidth != 192 || m.PopupOffset != 8 || m.PopupAlignOffset != -4 {
		t.Errorf("shadcn MenubarContent min-w %v sideOffset %v alignOffset %v, want 192 / 8 / -4", m.PopupMinWidth, m.PopupOffset, m.PopupAlignOffset)
	}
	if m.TriggerHoverBg.A != 0 || m.TriggerFocusBg != sh.ItemHoverBg || m.TriggerOpenBg != sh.ItemHoverBg {
		t.Error("shadcn MenubarTrigger: no hover fill, focus / open bg-accent")
	}
	if m.Height != 36 || m.TriggerHeight != 28 || m.TriggerPaddingInline != 8 || m.Gap != 4 {
		t.Error("shadcn Menubar: want h-9, trigger py-1 (28) px-2, gap-1")
	}
	if a := AntDesign().Components.Menubar; a.TriggerFocusBg.A != 0 || a.PopupMinWidth != 180 {
		t.Error("antd Menubar look changed")
	}
}

func TestAlignB3ICommandTokens(t *testing.T) {
	// shadcn command.tsx.
	sh := Shadcn()
	k := sh.Components.Command
	if k.InputHeight != 36 || k.InputPaddingInline != 12 || k.InputIconSize != 16 || k.Radius != sh.Radius {
		t.Error("shadcn CommandInput: want h-9 px-3 size-4 icon, rounded-md box")
	}
	if k.ListMaxHeight != 300 || k.ListPadding != 4 || k.GroupTitleHeight != 28 || k.GroupTitleFontSize != 12 || k.GroupTitleFontWeight != 500 {
		t.Error("shadcn CommandList / CommandGroup: want max-h-[300px], p-1, heading py-1.5 text-xs font-medium")
	}
	if k.ItemHeight != 32 || k.ItemPaddingInline != 8 || k.ItemGap != 8 || k.DisabledOpacity != .5 || k.EmptyPaddingBlock != 24 {
		t.Error("shadcn CommandItem / CommandEmpty: want py-1.5 px-2 gap-2, opacity-50, py-6")
	}
	if k.ShortcutFontSize != 12 || k.ShortcutLetterSpacing != 1.2 {
		t.Errorf("shadcn CommandShortcut %v / %v, want text-xs tracking-widest (0.1em)", k.ShortcutFontSize, k.ShortcutLetterSpacing)
	}
	if a := AntDesign().Components.Command; a.InputHeight != 44 || a.Radius != AntDesign().RadiusLG {
		t.Error("antd Command look changed")
	}
}
