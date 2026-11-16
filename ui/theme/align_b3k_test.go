package theme

// Spec alignment batch 3, group K (overlays and dialogs): token values
// compared with the source (antd@6.7.0, @rc-component/tour@2.4.0,
// element-plus@2.14.8 theme-chalk, shadcn new-york-v4,
// @radix-ui/react-hover-card@1.1.15) as recorded in
// docs/specs/align/3-overlay.md. The expected values are the source
// values.

import (
	"testing"
	"time"
)

func TestAlignB3KModalTokens(t *testing.T) {
	ant := AntDesign()
	m := ant.Components.Modal
	// antd es/modal/Modal.js width = 520; style/index.js top 100,
	// contentPadding paddingMD 20 / paddingContentHorizontalLG 24,
	// headerMarginBottom marginXS 8, footerMarginTop marginSM 12,
	// titleFontSize fontSizeHeading5 16; prepareToken modalCloseBtnSize
	// controlHeight 32 at (modalHeaderHeight 56 − 32) / 2 = 12.
	if m.Width != 520 || m.Top != 100 || m.WidthRatio != 0 || m.TopRatio != 0 || m.Centered {
		t.Errorf("Ant modal size: %v / %v / %v / %v / %v", m.Width, m.Top, m.WidthRatio, m.TopRatio, m.Centered)
	}
	if m.PaddingBlock != 20 || m.Padding != 24 || m.HeaderMarginBottom != 8 || m.FooterMarginTop != 12 || m.TitleFontSize != 16 {
		t.Errorf("Ant modal padding: %+v", m)
	}
	if m.CloseBtnSize != 32 || m.CloseIconSize != 16 || m.CloseInsetTop != 12 || m.CloseInsetEnd != 12 {
		t.Errorf("Ant modal close: %v %v %v %v", m.CloseBtnSize, m.CloseIconSize, m.CloseInsetTop, m.CloseInsetEnd)
	}
	// ConfirmDialog.js width 416; style/index.js confirmBtnsMarginTop
	// marginSM 12.
	if m.ConfirmWidth != 416 || m.ConfirmBtnsMarginTop != 12 {
		t.Errorf("Ant confirm: width %v btns %v", m.ConfirmWidth, m.ConfirmBtnsMarginTop)
	}
	// el-dialog: width 50%, margin-top 15vh, padding-primary 16; headerbtn
	// 48×48 at 0 / 0 with a 16px icon; messagebox width 420.
	e := ElementPlus().Components.Modal
	if e.WidthRatio != .5 || e.TopRatio != .15 || e.Padding != 16 || e.CloseBtnSize != 48 || e.CloseIconSize != 16 ||
		e.CloseInsetTop != 0 || e.CloseInsetEnd != 0 || e.ConfirmWidth != 420 {
		t.Errorf("Element dialog: %+v", e)
	}
	// dialog.tsx: sm:max-w-lg 512, centered, p-6, title text-lg
	// leading-none, close top-4 right-4 size-4.
	s := Shadcn().Components.Modal
	if s.Width != 512 || !s.Centered || s.Padding != 24 || s.TitleFontSize != 18 || s.TitleLineHeight != 18 ||
		s.CloseBtnSize != 16 || s.CloseInsetTop != 16 || s.CloseInsetEnd != 16 || s.ConfirmWidth != 512 || s.MaskBg != RGBA(0, 0, 0, .5) {
		t.Errorf("shadcn dialog: %+v", s)
	}
}

func TestAlignB3KDrawerTokens(t *testing.T) {
	ant := AntDesign()
	d := ant.Components.Drawer
	// antd es/drawer/Drawer.js DEFAULT_SIZE 378; style: close fontSizeLG +
	// paddingXS = 24, marginInlineEnd marginXS 8 (placement start),
	// footer paddingXS / padding, draggerSize 4 with colorPrimary at .2
	// (hover) / .3 (dragging).
	if d.Size != 378 || d.SizeRatio != 0 || d.CloseAtEnd || d.CloseBtnSize != 24 || d.CloseIconSize != 16 || d.HeaderGap != 8 {
		t.Errorf("Ant drawer header: %+v", d)
	}
	if d.FooterPaddingBlock != 8 || d.FooterPaddingInline != 16 || d.Padding != 24 {
		t.Errorf("Ant drawer padding: %+v", d)
	}
	if d.ResizeHandleSize != 4 || d.ResizeLineSize != 4 || d.ResizeLineColor != Fade(ant.Primary.Base, .2) || d.ResizeLineActiveColor != Fade(ant.Primary.Base, .3) {
		t.Errorf("Ant drawer dragger: %v %v %v %v", d.ResizeHandleSize, d.ResizeLineSize, d.ResizeLineColor, d.ResizeLineActiveColor)
	}
	// el-drawer: size 30%, header color text-color-primary, padding 20,
	// close-btn font-size-extra-large 20 at the end, dragger 8.
	el := ElementPlus()
	e := el.Components.Drawer
	if e.Size != 0 || e.SizeRatio != .3 || !e.CloseAtEnd || e.TitleColor != el.Text || e.Padding != 20 || e.CloseIconSize != 20 || e.ResizeHandleSize != 8 {
		t.Errorf("Element drawer: %+v", e)
	}
	// sheet.tsx: w-3/4 sm:max-w-sm (384), SheetHeader / SheetFooter p-4,
	// title font-semibold (1rem), close at the end.
	s := Shadcn().Components.Drawer
	if s.Size != 384 || s.SizeRatio != .75 || !s.CloseAtEnd || s.HeaderPaddingTop != 16 || s.HeaderPaddingInline != 16 ||
		s.FooterPaddingBlock != 16 || s.FooterPaddingInline != 16 || s.TitleFontSize != 16 || s.TitleFontWeight != 600 || s.MaskBg != RGBA(0, 0, 0, .5) {
		t.Errorf("shadcn sheet: %+v", s)
	}
}

func TestAlignB3KTourTokens(t *testing.T) {
	ant := AntDesign()
	tk := ant.Components.Tour
	// antd es/tour/style/index.js: width 520 with max-width fit-content,
	// boxShadowTertiary, header padding-bottom / footer padding-top
	// paddingXS 8, indicator 6×6 on colorFill; @rc-component/tour
	// hooks/useTarget.js gap radius 2.
	if tk.Width != 520 || !tk.FitContent || tk.HeaderMarginBottom != 8 || tk.FooterMarginTop != 8 || tk.IndicatorSize != 6 ||
		tk.IndicatorColor != ant.Fill || tk.HoleRadius != 2 || len(tk.Shadow) != len(ant.ShadowTertiary) || tk.Shadow[0] != ant.ShadowTertiary[0] {
		t.Errorf("Ant tour: %+v", tk)
	}
	// el-tour: width 520 (fixed), padding-primary 12 above the body and
	// the footer.
	e := ElementPlus().Components.Tour
	if e.Width != 520 || e.FitContent || e.PanelPadding != 12 || e.HeaderMarginBottom != 12 || e.FooterMarginTop != 12 || e.HoleRadius != 2 {
		t.Errorf("Element tour: %+v", e)
	}
}

func TestAlignB3KPopoverTokens(t *testing.T) {
	// @radix-ui/react-hover-card 1.1.15 dist/index.mjs: openDelay 700,
	// closeDelay 300 (shadcn HoverCard = a hover Popover).
	s := Shadcn().Components.Popover
	if s.MouseEnterDelay != 700*time.Millisecond || s.MouseLeaveDelay != 300*time.Millisecond {
		t.Errorf("shadcn hover card delays %v / %v", s.MouseEnterDelay, s.MouseLeaveDelay)
	}
	if s.Arrow || s.Offset != 4 || s.OffsetNoArrow != 4 {
		t.Errorf("shadcn popover: arrow %v offset %v / %v, want none, sideOffset 4", s.Arrow, s.Offset, s.OffsetNoArrow)
	}
	// antd es/popconfirm/index.js placement 'top', arrow on; antd popover
	// delays unchanged (0.1 s).
	a := AntDesign().Components.Popover
	if a.PopconfirmBottom || a.PopconfirmWidth != 0 || a.PopconfirmCancelText || !a.Arrow || a.Offset != 12 || a.OffsetNoArrow != 4 || a.MouseEnterDelay != 100*time.Millisecond {
		t.Errorf("Ant popconfirm: %+v", a)
	}
	// el-popconfirm: width 150, cancel-button-type text, popper placement
	// bottom.
	e := ElementPlus().Components.Popover
	if !e.PopconfirmBottom || e.PopconfirmWidth != 150 || !e.PopconfirmCancelText || !e.Arrow || e.Offset != 12 || e.OffsetNoArrow != 12 {
		t.Errorf("Element popconfirm: %+v", e)
	}
}
