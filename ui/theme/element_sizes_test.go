package theme

import "testing"

// TestElementPlusComponentSizes: Element's component token sizes follow
// Element Plus theme-chalk (packages/theme-chalk/src/common/var.scss and
// input-number.scss, checked 2026-11-13): $input-font-size 12/14/14,
// $input-padding-horizontal 8/12/16 (with the 1px border), $avatar-size
// 24/40/56 with 14px text, $pagination button height 32 and 24 when small,
// $tour width 520 and padding 12, $input-number-width 150 with step
// buttons as wide as the control is high. Ant and shadcn keep their own.
func TestElementPlusComponentSizes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		var opts []Option
		if dark {
			opts = append(opts, Dark())
		}
		k := ElementPlus(opts...).Components
		in := k.Input
		if in.InputFontSizeSM != 12 || in.InputFontSize != 14 || in.InputFontSizeLG != 14 ||
			in.PaddingInlineSM != 7 || in.PaddingInline != 11 || in.PaddingInlineLG != 15 {
			t.Errorf("dark=%v input: font %v/%v/%v padding %v/%v/%v", dark,
				in.InputFontSizeSM, in.InputFontSize, in.InputFontSizeLG, in.PaddingInlineSM, in.PaddingInline, in.PaddingInlineLG)
		}
		av := k.Avatar
		if av.ContainerSizeSM != 24 || av.ContainerSize != 40 || av.ContainerSizeLG != 56 ||
			av.TextFontSizeSM != 14 || av.TextFontSize != 14 || av.TextFontSizeLG != 14 {
			t.Errorf("dark=%v avatar: %+v", dark, av)
		}
		if pg := k.Pagination; pg.ItemSize != 32 || pg.ItemSizeSM != 24 {
			t.Errorf("dark=%v pagination: %v / %v", dark, pg.ItemSize, pg.ItemSizeSM)
		}
		if tr := k.Tour; tr.Width != 520 || tr.PanelPadding != 12 {
			t.Errorf("dark=%v tour: %v / %v", dark, tr.Width, tr.PanelPadding)
		}
		if n := k.InputNumber; n.ControlWidth != 150 || n.HandleWidth != 32 {
			t.Errorf("dark=%v input number: %v / %v", dark, n.ControlWidth, n.HandleWidth)
		}
	}
	// The others are unchanged: Ant sizes (shadcn size-8 avatars match).
	for _, th := range []*Theme{AntDesign(), Shadcn()} {
		k := th.Components
		if k.Avatar.ContainerSize != 32 || k.Avatar.ContainerSizeLG != 40 || k.InputNumber.ControlWidth != 90 || k.Tour.Width != 320 {
			t.Errorf("%v: avatar %v/%v input number %v tour %v", th.Flavor,
				k.Avatar.ContainerSize, k.Avatar.ContainerSizeLG, k.InputNumber.ControlWidth, k.Tour.Width)
		}
	}
}

// TestElementPlusDivider: el-divider draws its rule in --el-border-color
// with the title 20px from it and 24px margins with or without a title
// (theme-chalk divider.scss); Ant keeps Split / 16 / 16.
func TestElementPlusDivider(t *testing.T) {
	el := ElementPlus()
	if d := el.Components.Divider; d.Color != el.Border || d.TextPaddingInline != 20 || d.TextMarginBlock != 24 || d.MarginBlock != 24 {
		t.Errorf("element divider: %+v", d)
	}
	ant := AntDesign()
	if d := ant.Components.Divider; d.Color != ant.Split || d.TextPaddingInline != 16 || d.TextMarginBlock != 16 {
		t.Errorf("ant divider: %+v", d)
	}
}
