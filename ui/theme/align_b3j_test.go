package theme

import (
	"testing"
	"time"
)

// Batch 3, group J token values against their sources (2026-11-15,
// /tmp/tango-ref; docs/specs/align/3-nav.md).

func b3jEq(t *testing.T, name string, got, want float32) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func b3jCol(t *testing.T, name string, got, want Color) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %s, want %s", name, HexString(got), HexString(want))
	}
}

// TestAlignB3JPaginationTokens: antd 6.7 es/pagination/style/index.js
// (itemSize controlHeight / SM controlHeightSM / LG controlHeightLG,
// item-spacing marginXS / marginXXS / marginSM, quickJumperInputWidth
// controlHeightLG*1.25 / *1.1, itemActiveColorHover colorPrimaryHover);
// @rc-component/pagination Options.js page sizes; el-pagination
// pager-count 7, page-sizes, 40px large buttons, jumper 56px.
func TestAlignB3JPaginationTokens(t *testing.T) {
	a := AntDesign()
	k := a.Components.Pagination
	b3jEq(t, "ItemSize", k.ItemSize, 32)
	b3jEq(t, "ItemSizeSM", k.ItemSizeSM, 24)
	b3jEq(t, "ItemSizeLG", k.ItemSizeLG, 40)
	b3jEq(t, "ItemSpacing", k.ItemSpacing, 8)
	b3jEq(t, "ItemSpacingSM", k.ItemSpacingSM, 4)
	b3jEq(t, "ItemSpacingLG", k.ItemSpacingLG, 12)
	b3jEq(t, "QuickJumperInputWidth", k.QuickJumperInputWidth, 50)
	b3jEq(t, "QuickJumperInputWidthSM", k.QuickJumperInputWidthSM, 44)
	b3jCol(t, "ItemActiveColorHover", k.ItemActiveColorHover, a.Primary.Hover)
	b3jCol(t, "EllipsisColor", k.EllipsisColor, a.TextDisabled)
	if k.Background || k.PagerCount != 0 || k.PageSizesByTen {
		t.Errorf("ant behaviour tokens: bg %v count %d byTen %v", k.Background, k.PagerCount, k.PageSizesByTen)
	}
	e := ElementPlus().Components.Pagination
	b3jEq(t, "Element ItemSizeLG", e.ItemSizeLG, 40)
	b3jEq(t, "Element QuickJumperInputWidth", e.QuickJumperInputWidth, 56)
	if !e.Background || e.PagerCount != 7 || !e.PageSizesByTen {
		t.Errorf("element behaviour tokens: bg %v count %d byTen %v", e.Background, e.PagerCount, e.PageSizesByTen)
	}
	s := Shadcn().Components.Pagination
	b3jEq(t, "shadcn ItemSpacing (gap-1)", s.ItemSpacing, 4)
	if s.JumpHoverArrow {
		t.Error("shadcn ellipsis shows a hover arrow")
	}
}

// TestAlignB3JStepsTokens: antd 6.7 es/steps/style/status.js filled wait
// icon colorFillTertiary + colorTextLabel, hover colorPrimaryBgHover,
// text hover colorPrimaryHover; outlined colorBgContainer / current
// colorPrimaryBg.
func TestAlignB3JStepsTokens(t *testing.T) {
	a := AntDesign()
	k := a.Components.Steps
	b3jCol(t, "WaitIconBg", k.WaitIconBg, a.FillTertiary)
	b3jCol(t, "WaitIconColor", k.WaitIconColor, a.TextSecondary)
	b3jCol(t, "HoverIconBg", k.HoverIconBg, a.Primary.BgHover)
	b3jCol(t, "HoverColor", k.HoverColor, a.Primary.Hover)
	b3jCol(t, "OutlinedIconBg", k.OutlinedIconBg, a.BgContainer)
	b3jCol(t, "OutlinedActiveIconBg", k.OutlinedActiveIconBg, a.Primary.Bg)
	b3jEq(t, "IconSize", k.IconSize, 32)
	b3jEq(t, "IconSizeSM (fontSizeHeading3)", k.IconSizeSM, 24)
	b3jEq(t, "DotSize (controlHeight/4)", k.DotSize, 8)
	b3jEq(t, "DotCurrentSize (controlHeightLG/4)", k.DotCurrentSize, 10)
}

// TestAlignB3JSplitterTokens: antd 6.7 es/splitter/style/index.js
// (splitBarSize 2, splitTriggerSize 6, splitBarDraggableSize 20; line
// controlItemBgHover / Active / ActiveHover; spinner colorFill; collapse
// bar fontSizeSM x controlHeightSM, borderRadiusXS); el-splitter 2px
// border-color-light, collapse 16x24; shadcn resizable.tsx w-px bg-border,
// withHandle h-4 w-3 rounded-xs, GripVertical size-2.5.
func TestAlignB3JSplitterTokens(t *testing.T) {
	a := AntDesign()
	k := a.Components.Splitter
	b3jEq(t, "SplitBarSize", k.SplitBarSize, 2)
	b3jEq(t, "SplitTriggerSize", k.SplitTriggerSize, 6)
	b3jEq(t, "SplitBarDraggableSize", k.SplitBarDraggableSize, 20)
	b3jEq(t, "BarLineSize", k.BarLineSize, 2)
	b3jCol(t, "BarColor", k.BarColor, a.FillTertiary)
	b3jCol(t, "BarHoverColor", k.BarHoverColor, a.Primary.Bg)
	b3jCol(t, "BarActiveColor", k.BarActiveColor, a.Primary.BgHover)
	b3jCol(t, "HandleColor", k.HandleColor, a.Fill)
	b3jEq(t, "CollapseWidth", k.CollapseWidth, 12)
	b3jEq(t, "CollapseHeight", k.CollapseHeight, 24)
	b3jEq(t, "CollapseRadius", k.CollapseRadius, 2)
	e := ElementPlus()
	b3jCol(t, "Element BarColor", e.Components.Splitter.BarColor, e.Split)
	b3jEq(t, "Element CollapseWidth", e.Components.Splitter.CollapseWidth, 16)
	s := Shadcn()
	sk := s.Components.Splitter
	b3jEq(t, "shadcn BarLineSize", sk.BarLineSize, 1)
	b3jCol(t, "shadcn BarColor", sk.BarColor, s.Border)
	b3jEq(t, "shadcn GripWidth", sk.GripWidth, 12)
	b3jEq(t, "shadcn GripHeight", sk.GripHeight, 16)
	b3jEq(t, "shadcn GripIconSize", sk.GripIconSize, 10)
}

// TestAlignB3JBreadcrumbTokens: antd 6.7 es/breadcrumb/style/index.js
// prepareComponentToken (separatorColor colorTextDescription,
// iconFontSize fontSize); EP breadcrumb-item.scss separator margin 9px in
// text-color-placeholder; shadcn gap 2.5.
func TestAlignB3JBreadcrumbTokens(t *testing.T) {
	a := AntDesign()
	k := a.Components.Breadcrumb
	b3jCol(t, "SeparatorColor", k.SeparatorColor, a.TextTertiary)
	b3jEq(t, "IconFontSize", k.IconFontSize, 14)
	b3jEq(t, "separator margin (marginXS)", k.SeparatorMargin+k.LinkPaddingInline, 8)
	e := ElementPlus()
	b3jEq(t, "Element SeparatorMargin", e.Components.Breadcrumb.SeparatorMargin, 9)
	b3jCol(t, "Element SeparatorColor", e.Components.Breadcrumb.SeparatorColor, e.TextQuaternary)
	b3jEq(t, "shadcn ItemGap", Shadcn().Components.Breadcrumb.ItemGap, 10)
}

// TestAlignB3JPageHeaderTokens: Pro Components 3.1
// layout/components/PageHeader/style/index.js; EP page-header.scss.
func TestAlignB3JPageHeaderTokens(t *testing.T) {
	a := AntDesign()
	k := a.Components.PageHeader
	b3jEq(t, "PaddingBlock", k.PaddingBlock, 6)
	b3jEq(t, "PaddingInline", k.PaddingInline, 16)
	b3jEq(t, "PaddingTopBreadcrumb", k.PaddingTopBreadcrumb, 12)
	b3jEq(t, "BreadcrumbGap", k.BreadcrumbGap, 8)
	b3jEq(t, "BackGap", k.BackGap, 16)
	b3jEq(t, "TitleFontSize", k.TitleFontSize, 20)
	b3jEq(t, "TitleLineHeight", k.TitleLineHeight, 32)
	b3jEq(t, "ContentMarginTop", k.ContentMarginTop, 12)
	b3jEq(t, "FooterMarginTop", k.FooterMarginTop, 16)
	b3jCol(t, "SubTitleColor", k.SubTitleColor, a.TextSecondary)
	if k.TitleFontWeight != 600 || k.BorderColor.A != 0 {
		t.Errorf("title weight %d border %v", k.TitleFontWeight, k.BorderColor)
	}
	e := ElementPlus()
	ek := e.Components.PageHeader
	b3jEq(t, "Element BackIconGap", ek.BackIconGap, 10)
	b3jEq(t, "Element BreadcrumbGap", ek.BreadcrumbGap, 16)
	b3jEq(t, "Element TitleFontSize", ek.TitleFontSize, 18)
	b3jEq(t, "Element ContentMarginTop", ek.ContentMarginTop, 16)
	b3jCol(t, "Element ContentBorderColor", ek.ContentBorderColor, e.Split)
}

// TestAlignB3JAnchorTokens: antd 6.7 es/anchor/style/index.js and
// Anchor.js (bounds 5), _util/scrollTo.js (450ms); el-anchor bound 15,
// duration 300, marker 4x14.
func TestAlignB3JAnchorTokens(t *testing.T) {
	a := AntDesign()
	k := a.Components.Anchor
	b3jEq(t, "LinkPaddingBlock", k.LinkPaddingBlock, 4)
	b3jEq(t, "LinkPaddingBlockNested", k.LinkPaddingBlockNested, 2)
	b3jEq(t, "LinkPaddingInlineStart", k.LinkPaddingInlineStart, 16)
	b3jEq(t, "Bounds", k.Bounds, 5)
	b3jCol(t, "LinkHoverColor (no hover color)", k.LinkHoverColor, a.Text)
	if k.ScrollDuration != 450*time.Millisecond || !k.ShowRail {
		t.Errorf("duration %v rail %v", k.ScrollDuration, k.ShowRail)
	}
	e := ElementPlus().Components.Anchor
	b3jEq(t, "Element Bounds", e.Bounds, 15)
	b3jEq(t, "Element InkWidth", e.InkWidth, 4)
	b3jEq(t, "Element InkHeight", e.InkHeight, 14)
	if e.ScrollDuration != 300*time.Millisecond || e.ShowRail {
		t.Errorf("element duration %v rail %v", e.ScrollDuration, e.ShowRail)
	}
}

// TestAlignB3JBackTopTokens: antd float-button BackTop visibilityHeight
// 400, FloatButton insetInlineEnd 24 / insetBlockEnd 48; el-backtop
// visibility-height 200, right / bottom 40.
func TestAlignB3JBackTopTokens(t *testing.T) {
	k := AntDesign().Components.FloatButton
	b3jEq(t, "BackTopVisibilityHeight", k.BackTopVisibilityHeight, 400)
	b3jEq(t, "BackTopRight", k.BackTopRight, 24)
	b3jEq(t, "BackTopBottom", k.BackTopBottom, 48)
	e := ElementPlus().Components.FloatButton
	b3jEq(t, "Element BackTopVisibilityHeight", e.BackTopVisibilityHeight, 200)
	b3jEq(t, "Element BackTopRight", e.BackTopRight, 40)
	if !e.BackTopCaret || k.BackTopCaret {
		t.Error("BackTopCaret")
	}
}
