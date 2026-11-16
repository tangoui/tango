package theme

import "testing"

// Token fixes from the batch-2 review.

func TestAlignB2FixTokens(t *testing.T) {
	ant, el, sh := AntDesign(), ElementPlus(), Shadcn()
	// Timeline: theme defaults as tokens, not Flavor branches.
	if tk := ant.Components.Timeline; !tk.DotOutlined || !tk.BusyTailDashed {
		t.Errorf("Ant timeline %+v", tk)
	}
	if tk := el.Components.Timeline; tk.DotOutlined || tk.BusyTailDashed {
		t.Error("Element timeline: filled dots, no dotted axis")
	}
	if tk := sh.Components.Timeline; tk.DotOutlined || !tk.BusyTailDashed {
		t.Error("shadcn timeline: filled dots, dotted axis")
	}
	// Tag plain label: Ant Text, Element primary, shadcn foreground.
	if ant.Components.Tag.PlainColor != ant.Text || el.Components.Tag.PlainColor != el.Primary.Base ||
		sh.Components.Tag.PlainColor != sh.Text {
		t.Error("TagToken.PlainColor")
	}
	if el.Components.Tag.PlainBorderColor != el.Primary.Border || ant.Components.Tag.PlainBorderColor != ant.Border {
		t.Error("TagToken.PlainBorderColor")
	}
	// Progress line info.
	if !ant.Components.Progress.ShowInfo || !el.Components.Progress.ShowInfo || sh.Components.Progress.ShowInfo {
		t.Error("ProgressToken.ShowInfo")
	}
	// Table.
	if ant.Components.Table.SelectionBorderedExtra != 16 || el.Components.Table.SelectionBorderedExtra != 0 ||
		sh.Components.Table.SelectionBorderedExtra != 0 {
		t.Error("TableToken.SelectionBorderedExtra")
	}
	if !ant.Components.Table.SorterTooltip || el.Components.Table.SorterTooltip || !sh.Components.Table.SorterTooltip {
		t.Error("TableToken.SorterTooltip")
	}
	// Tooltip: antd halfArrowWidth + marginXXS = 12.
	if ant.Components.Tooltip.Offset != 12 || el.Components.Tooltip.Offset != 12 || sh.Components.Tooltip.Offset != 8 {
		t.Error("TooltipToken.Offset")
	}
	// VirtualTree follows IndentSize except under Element.
	for _, th := range []*Theme{ant, sh} {
		if tk := th.Components.Tree; tk.VirtualIndentSize != tk.IndentSize {
			t.Errorf("%v VirtualIndentSize %v, IndentSize %v", th.Flavor, tk.VirtualIndentSize, tk.IndentSize)
		}
	}
	if el.Components.Tree.VirtualIndentSize != 16 {
		t.Error("Element VirtualIndentSize")
	}
}
