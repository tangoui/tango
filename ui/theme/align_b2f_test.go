package theme

// Spec alignment batch 2, group F: Collapse, Card, Descriptions, List,
// Listy and Tree token values checked against the reference sources
// (2026-11-14):
//   antd@6.7.0 es/collapse/style/index.js, es/card/style/index.js,
//   es/descriptions/style/index.js, es/list/style/index.js,
//   es/listy/style/index.js, es/tree/style/index.js
//   (prepareComponentToken) and es/theme/util/alias.js (boxShadowCard);
//   element-plus@2.14.8 theme-chalk/src/collapse-item.scss,
//   descriptions.scss, descriptions-item.scss, tree.scss and
//   es/components/tree-v2/src/virtual-tree.mjs (indent 16, itemSize 26).

import (
	"testing"

	"github.com/tangoui/tango/ui/paint"
)

func TestAlignB2FCollapseTokens(t *testing.T) {
	ant := AntDesign()
	k := ant.Components.Collapse
	// headerPaddingSM "8 12 8 8", headerPaddingLG "16 24 16 16".
	if k.HeaderPaddingBlockSM != 8 || k.HeaderPaddingInlineSM != 12 || k.HeaderPaddingInlineStartSM != 8 ||
		k.HeaderPaddingBlockLG != 16 || k.HeaderPaddingInlineLG != 24 || k.HeaderPaddingInlineStartLG != 16 {
		t.Fatalf("Ant header paddings %+v", k)
	}
	// contentPadding "16 16", contentPaddingSM = paddingSM, contentPaddingLG = paddingLG.
	if k.ContentPaddingBlock != 16 || k.ContentPaddingInline != 16 || k.ContentPaddingBlockSM != 12 ||
		k.ContentPaddingInlineSM != 12 || k.ContentPaddingBlockLG != 24 || k.ContentPaddingInlineLG != 24 {
		t.Fatalf("Ant content paddings %+v", k)
	}
	// borderlessContentPadding "4 16 16"; ghost body paddingBlock paddingSM.
	if k.BorderlessContentPadding != 4 || k.BorderlessContentPaddingInline != 16 || k.BorderlessContentPaddingBottom != 16 || k.GhostContentPaddingBlock != 12 {
		t.Fatalf("Ant borderless / ghost paddings %+v", k)
	}
	// expandIconPlacement defaults to "start"; fontSizeIcon 12; marginSM 12.
	if k.ExpandIconEnd || k.ArrowSize != 12 || k.HeaderGap != 12 || k.HeaderBg != ant.FillAlter || k.BorderColor != ant.Border {
		t.Fatalf("Ant arrow / colors %+v", k)
	}
	el := ElementPlus()
	e := el.Components.Collapse
	// expand-icon-position default "right"; __arrow inherits the
	// header text color (text-color-primary); header 48px, 13px.
	if !e.ExpandIconEnd || e.ArrowColor != el.Text || e.HeaderHeight != 48 || e.HeaderFontSize != 13 || e.ContentPaddingBlock != 25 {
		t.Fatalf("Element collapse %+v", e)
	}
	sh := Shadcn()
	s := sh.Components.Collapse
	// accordion.tsx: py-4, size-4 chevron at the end, pb-4 content.
	if !s.ExpandIconEnd || s.HeaderPaddingBlock != 16 || s.ArrowSize != 16 || s.ContentPaddingBlock != 16 || s.HeaderFontWeight != 500 {
		t.Fatalf("shadcn accordion %+v", s)
	}
}

func TestAlignB2FCardTokens(t *testing.T) {
	ant := AntDesign()
	k := ant.Components.Card
	// headerHeight = 16*1.5 + 16*2 = 56; headerHeightSM = 14*22/14 + 8*2 = 38;
	// bodyPadding paddingLG, bodyPaddingSM 12, inner body "padding bodyPadding".
	if k.HeaderHeight != 56 || k.HeaderHeightSM != 38 || k.BodyPadding != 24 || k.BodyPaddingSM != 12 || k.InnerBodyPaddingBlock != 16 {
		t.Fatalf("Ant card sizes %+v", k)
	}
	if k.ShadowAlways || k.HoverBorderColor.A != 0 {
		t.Fatalf("Ant card shadow mode %v / hover border %v", k.ShadowAlways, k.HoverBorderColor)
	}
	// boxShadowCard: 0 1px 2px -2px .16, 0 3px 6px 0 .12, 0 5px 12px 4px .09 of #000.
	want := []paint.Shadow{
		{Y: 1, Blur: 2, Spread: -2, Color: RGBA(0, 0, 0, .16)},
		{Y: 3, Blur: 6, Color: RGBA(0, 0, 0, .12)},
		{Y: 5, Blur: 12, Spread: 4, Color: RGBA(0, 0, 0, .09)},
	}
	if len(k.HoverShadow) != 3 {
		t.Fatalf("Ant hover shadow %v", k.HoverShadow)
	}
	for i, s := range want {
		if k.HoverShadow[i] != s {
			t.Fatalf("Ant hover shadow[%d] = %+v, want %+v", i, k.HoverShadow[i], s)
		}
	}
	if !ElementPlus().Components.Card.ShadowAlways || !Shadcn().Components.Card.ShadowAlways {
		t.Fatal("Element (shadow=always) / shadcn (shadow-sm) cards must default to a shadow")
	}
}

func TestAlignB2FDescriptionsTokens(t *testing.T) {
	ant := AntDesign()
	k := ant.Components.Descriptions
	// itemPaddingBottom padding (16), medium paddingSM, small paddingXS;
	// itemPaddingEnd padding; colon margins marginXXS/2 and marginXS.
	if k.ItemPaddingBottom != 16 || k.ItemPaddingBottomMD != 12 || k.ItemPaddingBottomSM != 8 || k.ItemPaddingEnd != 16 ||
		k.ColonMarginInlineStart != 2 || k.LabelMarginInlineEnd != 8 || !k.ShowColon {
		t.Fatalf("Ant descriptions spacing %+v", k)
	}
	// Bordered: border colorSplit, label colorTextSecondary on colorFillAlter.
	if k.BorderColor != ant.Split || k.BorderedLabelColor != ant.TextSecondary || k.LabelColor != ant.TextTertiary || k.LabelBg != ant.FillAlter {
		t.Fatalf("Ant descriptions colors %+v", k)
	}
	if k.TitleMarginBottom != 20 || k.DefaultMiddle {
		t.Fatalf("Ant title margin %v / default size %v", k.TitleMarginBottom, k.DefaultMiddle)
	}
	el := ElementPlus()
	e := el.Components.Descriptions
	// descriptions.scss: header margin 20/16/12, bordered cells 12 15 / 8 11 / 4 7;
	// descriptions-item.scss: label margin 16 (small 12), vertical label 8/6/4.
	if e.TitleMarginBottom != 20 || e.TitleMarginBottomMD != 16 || e.TitleMarginBottomSM != 12 ||
		e.CellPaddingBlockMD != 8 || e.CellPaddingInlineMD != 11 || e.LabelMarginInlineEnd != 16 || e.LabelMarginInlineEndSM != 12 ||
		e.VerticalLabelPaddingBottom != 8 || e.VerticalLabelPaddingBottomMD != 6 || e.VerticalLabelPaddingBottomSM != 4 || e.TitleFontSizeSM != 14 {
		t.Fatalf("Element descriptions spacing %+v", e)
	}
	// Labels primary / content regular; bordered label regular / content primary.
	if e.LabelColor != el.Text || e.ContentColor != el.TextSecondary || e.BorderedLabelColor != el.TextSecondary || e.BorderedContentColor != el.Text ||
		e.ShowColon || !e.DefaultMiddle {
		t.Fatalf("Element descriptions colors / colon / size %+v", e)
	}
}

func TestAlignB2FListTokens(t *testing.T) {
	ant := AntDesign()
	k := ant.Components.List
	// itemPadding "12 0", itemPaddingSM "8 16", itemPaddingLG "16 24",
	// header / footer paddingBlock paddingSM; actions marginXXL, li 0 paddingXS.
	if k.ItemPaddingBlock != 12 || k.ItemPaddingBlockSM != 8 || k.ItemPaddingInlineSM != 16 || k.ItemPaddingBlockLG != 16 ||
		k.ItemPaddingInlineLG != 24 || k.HeaderPaddingBlock != 12 || k.ActionsMarginInlineStart != 48 || k.ActionsGap != 16 {
		t.Fatalf("Ant list paddings %+v", k)
	}
	// Vertical: metaMarginBottom padding, title fontSizeLG + titleMarginBottom
	// paddingSM, actions marginBlockStart padding, li 0 padding, extra marginLG.
	if k.VerticalMetaMarginBottom != 16 || k.VerticalTitleFontSize != ant.FontSizeLG || k.VerticalTitleMarginBottom != 12 ||
		k.VerticalActionsMarginTop != 16 || k.VerticalActionsGap != 32 || k.VerticalExtraMarginInlineStart != 24 {
		t.Fatalf("Ant vertical list %+v", k)
	}
	// Pagination marginBlockStart marginLG; bordered "margin marginLG"; grid item marginBlockEnd margin.
	if k.PaginationMarginTop != 24 || k.PaginationMarginBlockBordered != 16 || k.PaginationMarginInlineBordered != 24 || k.GridItemMarginBottom != 16 {
		t.Fatalf("Ant list pagination / grid %+v", k)
	}
	// Listy: itemPaddingBlock paddingSM, itemPaddingInline padding; group
	// header paddingXS, colorTextDescription, fontWeightStrong on colorFillAlter.
	if k.ListyItemPaddingBlock != 12 || k.ListyItemPaddingInline != 16 || k.ListyGroupHeaderPaddingBlock != 8 ||
		k.ListyGroupHeaderColor != ant.TextTertiary || k.ListyGroupHeaderFontWeight != 600 || k.ListyGroupHeaderBg != ant.FillAlter ||
		k.ListyItemHoverBg != ant.ItemHoverBg || k.ListySplitColor != ant.Split {
		t.Fatalf("Ant listy %+v", k)
	}
}

func TestAlignB2FTreeTokens(t *testing.T) {
	k := AntDesign().Components.Tree
	// titleHeight = controlHeightSM; switcherSize = indentSize = titleHeight.
	if k.TitleHeight != 24 || k.SwitcherSize != 24 || k.IndentSize != 24 || k.VirtualIndentSize != 24 || k.ShowEmptyText || k.SwitcherIconSize != 10 { // .switcher-icon fontSize 10
		t.Fatalf("Ant tree %+v", k)
	}
	el := ElementPlus()
	e := el.Components.Tree
	// el-tree indent 18, el-tree-v2 indent 16 / item-size 26; empty text
	// in text-color-secondary.
	if e.IndentSize != 18 || e.VirtualIndentSize != 16 || e.TitleHeight != 26 || !e.ShowEmptyText || e.EmptyTextColor != el.TextSecondary || e.SwitcherIconSize != 12 {
		t.Fatalf("Element tree %+v", e)
	}
}
