package theme

// Spec alignment batch 2, group G: token values compared with the source
// (antd@6.7.0, element-plus@2.14.8 theme-chalk, shadcn new-york-v4) as
// recorded in docs/specs/align/2-display.md. The expected values are the
// source values.

import (
	"testing"
	"time"
)

func TestAlignB2GTagTokens(t *testing.T) {
	ant := AntDesign()
	tk := ant.Components.Tag
	// antd es/tag/style/index.js prepareComponentToken: defaultBg =
	// colorFillTertiary (rgba(0,0,0,.04)) on colorBgContainer = #f5f5f5;
	// defaultColor colorText; tagPaddingHorizontal 8 − lineWidth 1 = 7;
	// height lineHeightSM × fontSizeSM (20) + 2 × lineWidth = 22.
	if tk.DefaultBg != Hex("#f5f5f5") || tk.DefaultColor != ant.Text || tk.PaddingInline != 7 || tk.Height != 22 {
		t.Errorf("Ant tag: bg %v color %v pad %v h %v", tk.DefaultBg, tk.DefaultColor, tk.PaddingInline, tk.Height)
	}
	// antd useColor.js: defaultVariant "filled" (no border).
	if tk.Outlined {
		t.Error("Ant tags default to filled, not outlined")
	}
	// antd theme/themes/default/colors.js colorBgSolid = colorTextBase
	// (#000) at 1; dark/colors.js at 0.95.
	if tk.SolidBg != Hex("#000000") {
		t.Errorf("Ant SolidBg %v, want #000000", tk.SolidBg)
	}
	// Element el-tag default effect light (tinted with a border).
	if !ElementPlus().Components.Tag.Outlined {
		t.Error("Element tags default to the outlined (light) look")
	}
	// shadcn badge.tsx: rounded-full, px-2 (8 − 1 border), font-medium.
	sh := Shadcn().Components.Tag
	if sh.Radius < 100 || sh.PaddingInline != 7 || sh.FontWeight != 500 {
		t.Errorf("shadcn badge: radius %v pad %v weight %v", sh.Radius, sh.PaddingInline, sh.FontWeight)
	}
	// badge.tsx:15 dark:bg-destructive/60 (the destructive button's value).
	dk := Shadcn(Dark())
	if want := Over(Fade(dk.Shadcn.Destructive, .6), dk.Shadcn.Background); dk.Components.Tag.DestructiveBg != want {
		t.Errorf("shadcn dark destructive %v, want %v", dk.Components.Tag.DestructiveBg, want)
	}
	if r := ContrastRatio(Hex("#ffffff"), dk.Components.Tag.DestructiveBg); r < 4.5 {
		t.Errorf("white on dark destructive %.2f", r)
	}
}

func TestAlignB2GBadgeTokens(t *testing.T) {
	// antd es/badge/style/index.js prepareComponentToken: indicatorHeight
	// round(14 × 1.5714) − 2 = 20, indicatorHeightSM 14, dotSize 6,
	// paddingInline paddingXS 8 (multiple-words, both sizes).
	k := AntDesign().Components.Badge
	if k.IndicatorHeight != 20 || k.IndicatorHeightSM != 14 || k.DotSize != 6 || k.IndicatorPaddingInline != 8 || k.IndicatorPaddingInlineSM != 8 {
		t.Errorf("Ant badge %v/%v dot %v pad %v/%v", k.IndicatorHeight, k.IndicatorHeightSM, k.DotSize, k.IndicatorPaddingInline, k.IndicatorPaddingInlineSM)
	}
	// theme-chalk var.scss $badge: size 18, padding 6.
	e := ElementPlus().Components.Badge
	if e.IndicatorHeight != 18 || e.IndicatorPaddingInline != 6 {
		t.Errorf("Element badge %v pad %v", e.IndicatorHeight, e.IndicatorPaddingInline)
	}
}

func TestAlignB2GAvatarTokens(t *testing.T) {
	// antd es/avatar/style/index.js prepareComponentToken: 32/40/24,
	// textFontSize fontSize (14) at every size, iconFontSize 18 / LG 24 /
	// SM 14, square radius 6 / 8 / 4.
	k := AntDesign().Components.Avatar
	if k.ContainerSize != 32 || k.ContainerSizeLG != 40 || k.ContainerSizeSM != 24 {
		t.Errorf("Ant avatar sizes %v %v %v", k.ContainerSize, k.ContainerSizeLG, k.ContainerSizeSM)
	}
	if k.TextFontSize != 14 || k.TextFontSizeLG != 14 || k.TextFontSizeSM != 14 {
		t.Errorf("Ant avatar text %v %v %v", k.TextFontSize, k.TextFontSizeLG, k.TextFontSizeSM)
	}
	if k.IconFontSize != 18 || k.IconFontSizeLG != 24 || k.IconFontSizeSM != 14 {
		t.Errorf("Ant avatar icons %v %v %v", k.IconFontSize, k.IconFontSizeLG, k.IconFontSizeSM)
	}
	if k.Radius != 6 || k.RadiusLG != 8 || k.RadiusSM != 4 {
		t.Errorf("Ant avatar radius %v %v %v", k.Radius, k.RadiusLG, k.RadiusSM)
	}
	// theme-chalk var.scss $avatar-size 56/40/24, $avatar text-size 14,
	// icon-size 18.
	e := ElementPlus().Components.Avatar
	if e.ContainerSize != 40 || e.ContainerSizeLG != 56 || e.IconFontSizeLG != 18 || e.TextFontSizeLG != 14 {
		t.Errorf("Element avatar %v %v %v %v", e.ContainerSize, e.ContainerSizeLG, e.IconFontSizeLG, e.TextFontSizeLG)
	}
	// shadcn avatar.tsx AvatarFallback text-muted-foreground, sm text-xs.
	s := Shadcn()
	if s.Components.Avatar.Color != s.Shadcn.MutedForeground || s.Components.Avatar.TextFontSizeSM != 12 {
		t.Errorf("shadcn avatar fallback %v / %v", s.Components.Avatar.Color, s.Components.Avatar.TextFontSizeSM)
	}
}

func TestAlignB2GSkeletonProgressTokens(t *testing.T) {
	// antd es/skeleton/style/index.js: title + paragraph controlHeightSM
	// 24, with an avatar paragraphMarginTop marginLG + marginXXS = 28 and
	// the title marginSM 12; li + li controlHeightXS 16; title / li height
	// controlHeight / 2 = 16; Skeleton.js avatar size "large" = 40.
	k := AntDesign().Components.Skeleton
	if k.ParagraphMarginTop != 24 || k.ParagraphMarginTopAvatar != 28 || k.TitleMarginTopAvatar != 12 ||
		k.ParagraphGap != 16 || k.TitleHeight != 16 || k.AvatarSize != 40 {
		t.Errorf("Ant skeleton %+v", k)
	}
	// antd es/progress/utils.js getSize: circle small 60, default 120;
	// line 8 / small 6; steps 14 / small 2.
	p := AntDesign().Components.Progress
	if p.CircleSizeSM != 60 || p.CircleSize != 120 || p.LineHeight != 8 || p.LineHeightSM != 6 || p.StepsWidth != 14 || p.StepsWidthSM != 2 {
		t.Errorf("Ant progress %+v", p)
	}
}

func TestAlignB2GEmptyTokens(t *testing.T) {
	// antd es/empty/style/index.js: image height controlHeightLG × 2.5 =
	// 100, simple controlHeightLG 40, marginInline marginXS 8, image
	// marginBottom marginXS 8, footer marginTop margin 16.
	a := AntDesign().Components.Empty
	if a.ImageHeight != 100 || a.ImageHeightSM != 40 || a.PaddingInline != 8 || a.DescriptionMarginTop != 8 || a.FooterMarginTop != 16 {
		t.Errorf("Ant empty %+v", a)
	}
	// theme-chalk var.scss $empty: padding 40px 0, image-width 160,
	// description-margin-top 20, bottom-margin-top 20.
	e := ElementPlus().Components.Empty
	if e.PaddingBlock != 40 || e.PaddingInline != 0 || e.ImageHeight != 160 || e.DescriptionMarginTop != 20 || e.FooterMarginTop != 20 {
		t.Errorf("Element empty %+v", e)
	}
	// shadcn empty.tsx: p-6, gap-6, EmptyTitle text-lg font-medium.
	s := Shadcn().Components.Empty
	if s.PaddingBlock != 24 || s.FooterMarginTop != 24 || s.TitleFontSize != 18 || s.TitleFontWeight != 500 {
		t.Errorf("shadcn empty %+v", s)
	}
}

func TestAlignB2GTooltipPopoverTokens(t *testing.T) {
	// antd es/tooltip/index.js and es/popover/index.js: mouseEnterDelay /
	// mouseLeaveDelay default 0.1 s.
	ms := time.Millisecond
	a := AntDesign()
	if a.Components.Tooltip.MouseEnterDelay != 100*ms || a.Components.Tooltip.MouseLeaveDelay != 100*ms {
		t.Errorf("Ant tooltip delays %v %v", a.Components.Tooltip.MouseEnterDelay, a.Components.Tooltip.MouseLeaveDelay)
	}
	if a.Components.Popover.MouseEnterDelay != 100*ms || a.Components.Popover.MouseLeaveDelay != 100*ms || a.Components.Popover.Width != 0 {
		t.Errorf("Ant popover %v %v %v", a.Components.Popover.MouseEnterDelay, a.Components.Popover.MouseLeaveDelay, a.Components.Popover.Width)
	}
	// element-plus es/hooks/use-delayed-toggle: showAfter 0, hideAfter 200;
	// el-tooltip offset 12; el-popover width 150.
	e := ElementPlus()
	if e.Components.Tooltip.MouseEnterDelay != 0 || e.Components.Tooltip.MouseLeaveDelay != 200*ms || e.Components.Tooltip.Offset != 12 {
		t.Errorf("Element tooltip %v %v %v", e.Components.Tooltip.MouseEnterDelay, e.Components.Tooltip.MouseLeaveDelay, e.Components.Tooltip.Offset)
	}
	if e.Components.Popover.Width != 150 || e.Components.Popover.MouseLeaveDelay != 200*ms {
		t.Errorf("Element popover %v %v", e.Components.Popover.Width, e.Components.Popover.MouseLeaveDelay)
	}
	// shadcn tooltip.tsx delayDuration 0; popover.tsx w-72.
	s := Shadcn()
	if s.Components.Tooltip.MouseEnterDelay != 0 || s.Components.Popover.Width != 288 {
		t.Errorf("shadcn tooltip delay %v popover width %v", s.Components.Tooltip.MouseEnterDelay, s.Components.Popover.Width)
	}
}
