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
	// Tour: antd 6.7 tour/style width 520 with max-width fit-content
	// (batch 3 group K: 320 -> 520).
	for _, th := range []*Theme{AntDesign(), Shadcn()} {
		k := th.Components
		if k.Avatar.ContainerSize != 32 || k.Avatar.ContainerSizeLG != 40 || k.InputNumber.ControlWidth != 90 || k.Tour.Width != 520 {
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

func TestElementPlusControlRefinements(t *testing.T) {
	for _, opt := range [][]Option{{ElementPlusControls()}, {ElementPlusControls(), Dark()}} {
		el := ElementPlus(opt...)
		k := el.Components
		if k.Input.ControlRadius != 4 || k.Input.ActiveBorderColor != el.Primary.Base || k.Input.ContentColor != el.TextSecondary ||
			k.Select.ControlRadius != 4 || k.Select.ActiveBorderColor != el.Primary.Base || !k.Select.InputSizing {
			t.Fatal("Element input or select sizing/color mismatch")
		}
		if k.Checkbox.ControlInteractiveSize != 14 || k.Checkbox.Radius != 2 || k.Radio.RadioSize != 14 ||
			k.Switch.TrackHeight != 20 || k.Switch.TrackMinWidth != 40 || len(k.Switch.HandleShadow) != 0 {
			t.Fatal("Element checkbox, radio or switch sizing mismatch")
		}
		if k.Button.ContentFontSizeSM != 12 || k.Button.ContentFontSizeLG != 14 || k.Button.RadiusSM != 3 || k.Button.RadiusLG != 4 ||
			k.Tabs.HorizontalItemGutter != 40 || k.Tabs.CardHeight != 40 || k.Table.CellLineHeight != 23 || k.Table.HeaderColor != el.TextSecondary {
			t.Fatal("Element button, tabs or table sizing/color mismatch")
		}
		if k.Dropdown.ItemHeight != 32 || k.Dropdown.PopupBorderColor != el.Split || k.Modal.MaskBg != RGBA(0, 0, 0, .5) ||
			k.Drawer.MaskBg != k.Modal.MaskBg || k.Tooltip.ContentFontSize != 12 || k.Tooltip.MinHeight != 30 {
			t.Fatal("Element dropdown, dialog, drawer or tooltip mismatch")
		}
		s := k.Scrollbar
		if s.TrackInset != 2 || s.Thickness != 6 || s.MinThumbLength != 20 || s.ThumbRadius != 4 ||
			s.Thumb != Fade(el.TextSecondary, .3) || s.ThumbHover != Fade(el.TextSecondary, .5) {
			t.Fatalf("Element scrollbar = %+v", s)
		}
	}
	for _, th := range []*Theme{AntDesign(ElementPlusControls()), ElementPlus()} {
		if th.Components.Scrollbar.TrackInset != 0 || th.Components.Input.ControlRadius != 0 {
			t.Fatal("control refinements require an opt-in Element theme")
		}
	}
}
