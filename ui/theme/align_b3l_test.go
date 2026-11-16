package theme

// Spec alignment batch 3, group L: token values compared with the source
// (antd@6.7.0, element-plus@2.14.8 theme-chalk, shadcn new-york-v4,
// sonner@2.0.7) as recorded in docs/specs/align/3-feedback.md. The
// expected values are the source values.

import "testing"

func TestAlignB3LAlertTokens(t *testing.T) {
	ant := AntDesign()
	tk := ant.Components.Alert
	// antd es/alert/style/index.js prepareComponentToken: defaultPadding
	// paddingContentVerticalSM 8 / 12, withDescriptionPadding paddingMD 20
	// / paddingContentHorizontalLG 24, withDescriptionIconSize
	// fontSizeHeading3 24, containerBorderRadius borderRadiusLG; title
	// colorTextHeading, close colorIcon / colorIconHover (colorText),
	// fontSizeIcon 12.
	if tk.DefaultPaddingBlock != 8 || tk.DefaultPaddingInline != 12 || tk.WithDescriptionPaddingBlock != 20 ||
		tk.WithDescriptionPaddingInline != 24 || tk.WithDescriptionIconSize != 24 || tk.Radius != ant.RadiusLG {
		t.Errorf("Ant alert sizes: %+v", tk)
	}
	// &-with-description title: marginBottom marginXS 8, fontSizeLG, no
	// fontWeight anywhere in the alert style.
	if tk.TitleMarginBottom != 8 || tk.TitleFontWeight != 0 || tk.TitleFontSize != 16 {
		t.Errorf("Ant alert title: margin %v weight %v size %v", tk.TitleMarginBottom, tk.TitleFontWeight, tk.TitleFontSize)
	}
	if tk.TitleColor != ant.TextHeading || tk.CloseColor != ant.TextTertiary || tk.CloseHoverColor != ant.Text || tk.CloseIconSize != 12 {
		t.Errorf("Ant alert title %v close %v/%v %v", tk.TitleColor, tk.CloseColor, tk.CloseHoverColor, tk.CloseIconSize)
	}
	// EP theme-chalk/src/common/var.scss $alert: padding 8px 16px, title
	// 14px / 16px with a description (regular), description 14px, icon
	// 16px / 28px, close 16px (custom text 14px); alert.scss: title and
	// description in the type color, content gap 4px, align-items center;
	// close --el-text-color-placeholder.
	el := ElementPlus()
	ek := el.Components.Alert
	if ek.TitleColor.A != 0 || ek.DescriptionColor.A != 0 {
		t.Error("Element alert text must take the status color")
	}
	if ek.IconSize != 16 || ek.TitleFontSize != 16 || ek.TitleFontWeight != 0 || ek.DescriptionFontSize != 14 || ek.TitleMarginBottom != 4 ||
		ek.WithDescriptionIconSize != 28 || !ek.WithDescriptionIconCenter || ek.CloseIconSize != 16 || ek.CloseFontSize != 14 ||
		ek.CloseColor != Hex("#a8abb2") {
		t.Errorf("Element alert: %+v", ek)
	}
	// shadcn alert.tsx v4: bg-card, border (destructive keeps the
	// neutral border), gap-y-0.5, icon size-4, gap-x-3.
	sh := Shadcn()
	sk := sh.Components.Alert
	if sk.ErrorBorderColor != sh.BorderSecondary || sk.InfoBg != sh.Shadcn.Card || sk.TitleMarginBottom != 2 || sk.IconSize != 16 || sk.IconMarginInlineEnd != 12 {
		t.Errorf("shadcn alert: border %v bg %v gap %v", sk.ErrorBorderColor, sk.InfoBg, sk.TitleMarginBottom)
	}
	dk := Shadcn(Dark()).Components.Alert
	if dk.InfoBg == Shadcn(Dark()).BgContainer {
		t.Error("shadcn dark alert should use --card, not --background")
	}
}

func TestAlignB3LMessageTokens(t *testing.T) {
	ant := AntDesign()
	tk := ant.Components.Message
	// antd es/message/style/index.js: contentPadding (controlHeightLG −
	// fontSize × lineHeight) / 2 = 9, paddingSM 12; useMessage.js
	// DEFAULT_OFFSET 8; notification list gap token.margin 16.
	if tk.PaddingBlock != 9 || tk.PaddingInline != 12 || tk.Top != 8 || tk.Gap != 16 || tk.ContentBg != ant.BgElevated {
		t.Errorf("Ant message: %+v", tk)
	}
	// EP message.mjs offset 16, instance.mjs space 16, $message padding
	// 11px 15px.
	ek := ElementPlus().Components.Message
	if ek.Top != 16 || ek.Gap != 16 || ek.PaddingBlock != 11 || ek.PaddingInline != 15 {
		t.Errorf("Element message: %+v", ek)
	}
	// sonner 2.0.7: TOAST_WIDTH 356, padding 16px, font-size 13px,
	// box-shadow 0 4px 12px rgba(0,0,0,.1); shadcn sonner.tsx
	// --border-radius var(--radius).
	sh := Shadcn()
	sk := sh.Components.Message
	if sk.Width != 356 || sk.PaddingBlock != 16 || sk.ContentFontSize != 13 || sk.Radius != sh.RadiusLG ||
		len(sk.Shadow) != 1 || sk.Shadow[0].Y != 4 || sk.Shadow[0].Blur != 12 || sk.Shadow[0].Color != RGBA(0, 0, 0, .1) {
		t.Errorf("shadcn message (sonner): %+v", sk)
	}
}

func TestAlignB3LNotificationTokens(t *testing.T) {
	ant := AntDesign()
	tk := ant.Components.Notification
	// antd es/notification/style: width 384, padding paddingMD 20 /
	// paddingContentHorizontalLG 24, section gap marginXS 8, wrapper gap
	// marginSM 12, actions marginTop marginSM 12, marginEdge marginLG 24,
	// list gap margin 16, progress 2px gradient colorPrimaryBorderHover →
	// colorPrimary on rgba(0,0,0,.04) inset borderRadiusLG.
	if tk.Width != 384 || tk.PaddingBlock != 20 || tk.Padding != 24 || tk.TitleMarginBottom != 8 || tk.IconGap != 12 ||
		tk.ActionsMarginTop != 12 || tk.MarginEdge != 24 || tk.Gap != 16 || tk.CloseColor != ant.TextTertiary {
		t.Errorf("Ant notification: %+v", tk)
	}
	if tk.ProgressColorStart != ant.Primary.BorderHover || tk.ProgressColor != ant.Primary.Base || tk.ProgressRail != ant.FillTertiary ||
		tk.ProgressInset != ant.RadiusLG || tk.ProgressHeight != 2 {
		t.Errorf("Ant notification progress: %+v", tk)
	}
	// EP $notification: width 330, padding 14px 26px 14px 13px, group
	// margin-left 13px, content margin-top 6px; .right 16px; notify.mjs
	// GAP_SIZE 16; ElProgress strokeWidth 3 with the type status.
	ek := ElementPlus().Components.Notification
	if ek.Width != 330 || ek.TitleMarginBottom != 6 || ek.IconGap != 13 || ek.MarginEdge != 16 || ek.Gap != 16 ||
		ek.ProgressHeight != 3 || !ek.ProgressStatusColor {
		t.Errorf("Element notification: %+v", ek)
	}
	// sonner 2.0.7: 13px, title weight 500 line-height 1.5, description
	// line-height 1.4, content gap 2px.
	sk := Shadcn().Components.Notification
	if sk.Width != 356 || sk.TitleFontSize != 13 || sk.TitleFontWeight != 500 || sk.TitleLineHeight != 19.5 ||
		sk.DescriptionFontSize != 13 || sk.DescriptionLineHeight != 18.2 || sk.TitleMarginBottom != 2 {
		t.Errorf("shadcn notification (sonner): %+v", sk)
	}
}

func TestAlignB3LResultTokens(t *testing.T) {
	// antd es/result/style/index.js: padding paddingLG×2 48 / paddingXL
	// 32, iconFontSize fontSizeHeading3×3 72, icon marginBottom 24,
	// titleFontSize 24, title marginBlock marginXS 8, extraMargin 24, body
	// colorFillAlter without radius.
	ant := AntDesign()
	tk := ant.Components.Result
	if tk.PaddingBlock != 48 || tk.PaddingInline != 32 || tk.IconFontSize != 72 || tk.IconMarginBottom != 24 ||
		tk.TitleFontSize != 24 || tk.SubtitleMarginTop != 8 || tk.ExtraMargin != 24 || tk.ContentBg != ant.FillAlter || tk.ContentRadius != 0 {
		t.Errorf("Ant result: %+v", tk)
	}
	// EP $result: padding 40px 30px, icon 64px, title 20px margin-top
	// 20px, subtitle margin-top 10px, extra margin-top 30px; line-height
	// 1.3.
	ek := ElementPlus().Components.Result
	if ek.PaddingBlock != 40 || ek.PaddingInline != 30 || ek.IconFontSize != 64 || ek.TitleFontSize != 20 || ek.IconMarginBottom != 20 ||
		ek.SubtitleMarginTop != 10 || ek.ExtraMargin != 30 || ek.TitleLineHeight != 26 || ek.SubtitleLineHeight != 18.2 {
		t.Errorf("Element result: %+v", ek)
	}
}

func TestAlignB3LSpinTokens(t *testing.T) {
	// antd es/spin/style/index.js prepareComponentToken: dotSize
	// controlHeightLG/2 20, dotSizeSM controlHeightLG×0.35 14, dotSizeLG
	// controlHeight 32; section gap paddingSM 12, description fontSize;
	// nested container opacity .5 under colorBgContainer at .4;
	// fullscreen colorBgMask with colorWhite; Progress stroke viewSize/5.
	ant := AntDesign()
	tk := ant.Components.Spin
	if tk.Indicator != SpinDots || tk.DotSize != 20 || tk.DotSizeSM != 14 || tk.DotSizeLG != 32 || tk.DescriptionGap != 12 ||
		tk.DescriptionFontSize != 14 || tk.ContainerOpacity != 0.5 || tk.MaskBg != Fade(ant.BgContainer, 0.4) ||
		tk.FullscreenMaskBg != ant.BgMask || tk.FullscreenColor != Hex("#ffffff") || tk.ProgressStrokeRatio != 0.2 || tk.Color != ant.Primary.Base {
		t.Errorf("Ant spin: %+v", tk)
	}
	// EP loading.scss: --el-loading-spinner-size 42, fullscreen 50, text
	// margin 3px, mask --el-mask-color rgba(255,255,255,.9) (dark
	// rgba(0,0,0,.8)); content not dimmed.
	ek := ElementPlus().Components.Spin
	if ek.Indicator != SpinCircular || ek.DotSize != 42 || ek.FullscreenDotSize != 50 || ek.DescriptionGap != 3 ||
		ek.MaskBg != RGBA(255, 255, 255, 0.9) || ek.ContainerOpacity != 1 {
		t.Errorf("Element spin: %+v", ek)
	}
	if dk := ElementPlus(Dark()).Components.Spin; dk.MaskBg != RGBA(0, 0, 0, 0.8) {
		t.Errorf("Element dark mask %v", dk.MaskBg)
	}
	// shadcn spinner.tsx: Loader2Icon size-4 in the current color (small
	// / large: size-3 / size-6).
	sh := Shadcn()
	if sk := sh.Components.Spin; sk.Indicator != SpinIcon || sk.DotSize != 16 || sk.DotSizeSM != 12 || sk.DotSizeLG != 24 || sk.Color != sh.Text {
		t.Errorf("shadcn spin: %+v", sk)
	}
}

// TestAlignB3LSkeletonEmptyTokens: the shadcn pulse and Element's empty
// illustration are tokens, not Flavor branches in the widgets.
func TestAlignB3LSkeletonEmptyTokens(t *testing.T) {
	// shadcn skeleton.tsx: animate-pulse; antd / EP sweep a gradient.
	if AntDesign().Components.Skeleton.Pulse || ElementPlus().Components.Skeleton.Pulse || !Shadcn().Components.Skeleton.Pulse {
		t.Error("Skeleton.Pulse must be true only for shadcn")
	}
	if AntDesign().Components.Empty.Picture != EmptyPictureAnt || ElementPlus().Components.Empty.Picture != EmptyPictureElement ||
		Shadcn().Components.Empty.Picture != EmptyPictureAnt {
		t.Error("Empty.Picture: Element draws its illustration, the others Ant's box (shadcn uses the media tile)")
	}
}
