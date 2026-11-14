package theme

import "testing"

// Batch 2, group H token values against their sources (2026-11-14,
// /tmp/tango-ref; docs/specs/align/2-media.md).

func b2hEq(t *testing.T, name string, got, want float32) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func b2hAlpha(t *testing.T, name string, c Color, a float64) {
	t.Helper()
	if d := float64(c.A)/255 - a; d > .005 || d < -.005 {
		t.Errorf("%s alpha = %v, want %v", name, float64(c.A)/255, a)
	}
}

// TestAlignB2HTabsTokens: antd 6.7 es/tabs/style/index.js
// prepareComponentToken (cardHeight controlHeightLG, cardHeightSM
// controlHeight, cardHeightLG controlHeightLG+8, cardPadding
// "(h-fontHeight)/2-1 padding" / SM paddingXS, horizontalItemPadding
// paddingSM / SM paddingXS / LG padding, horizontalItemGutter 32,
// cardGutter marginXXS/2); Element el-tabs 40px items.
func TestAlignB2HTabsTokens(t *testing.T) {
	k := AntDesign().Components.Tabs
	b2hEq(t, "CardHeight", k.CardHeight, 40)
	b2hEq(t, "CardHeightSM", k.CardHeightSM, 32)
	b2hEq(t, "CardHeightLG", k.CardHeightLG, 48)
	b2hEq(t, "CardPaddingH", k.CardPaddingH, 16)
	b2hEq(t, "CardPaddingHSM", k.CardPaddingHSM, 8)
	b2hEq(t, "HorizontalItemPadding", k.HorizontalItemPadding, 12)
	b2hEq(t, "HorizontalItemPaddingSM", k.HorizontalItemPaddingSM, 8)
	b2hEq(t, "HorizontalItemPaddingLG", k.HorizontalItemPaddingLG, 16)
	b2hEq(t, "HorizontalItemGutter", k.HorizontalItemGutter, 32)
	b2hEq(t, "TitleFontSizeLG", k.TitleFontSizeLG, 16)
	e := ElementPlus().Components.Tabs
	b2hEq(t, "Element CardHeightSM", e.CardHeightSM, 40)
	b2hEq(t, "Element CardHeightLG", e.CardHeightLG, 40)
	// shadcn tabs.tsx: dark active trigger bg-input/30 over the muted
	// list, border-input; light bg-background, no border.
	sd := Shadcn(Dark())
	if sd.Components.Tabs.ItemSelectedBorder != sd.Shadcn.Input ||
		sd.Components.Tabs.ItemSelectedBg != Over(Fade(sd.Shadcn.Input, .3), sd.Shadcn.Muted) {
		t.Errorf("shadcn dark active trigger %v %v", sd.Components.Tabs.ItemSelectedBg, sd.Components.Tabs.ItemSelectedBorder)
	}
	sl := Shadcn()
	if sl.Components.Tabs.ItemSelectedBg != sl.Shadcn.Background || sl.Components.Tabs.ItemSelectedBorder.A != 0 {
		t.Errorf("shadcn light active trigger")
	}
}

// TestAlignB2HTimelineTokens: antd 6.7 es/timeline/style/index.js
// (tailWidth / dotBorderWidth lineWidthBold 2, itemPaddingBottom
// padding*1.25 = 20) and es/steps/style/index.js (dotCurrentSize
// controlHeightLG/4 = 10).
func TestAlignB2HTimelineTokens(t *testing.T) {
	k := AntDesign().Components.Timeline
	b2hEq(t, "TailWidth", k.TailWidth, 2)
	b2hEq(t, "DotBorderWidth", k.DotBorderWidth, 2)
	b2hEq(t, "ItemPaddingBottom", k.ItemPaddingBottom, 20)
	b2hEq(t, "DotSize", k.DotSize, 10)
}

// TestAlignB2HCarouselTokens: antd 6.7 es/carousel/style/index.js
// prepareComponentToken (arrowSize 16, arrowOffset marginXS 8, dotWidth
// 16, dotHeight 3, dotActiveWidth 24, dotGap marginXXS 4 per side; dot
// opacity 0.2, arrow opacity 0.4, disabled arrow opacity 0); Element
// carousel.scss (36px arrows 16px in); shadcn carousel.tsx (size-8,
// -left-12).
func TestAlignB2HCarouselTokens(t *testing.T) {
	k := AntDesign().Components.Carousel
	b2hEq(t, "ArrowSize", k.ArrowSize, 16)
	b2hEq(t, "ArrowOffset", k.ArrowOffset, 8)
	b2hEq(t, "DotWidth", k.DotWidth, 16)
	b2hEq(t, "DotHeight", k.DotHeight, 3)
	b2hEq(t, "DotActiveWidth", k.DotActiveWidth, 24)
	b2hEq(t, "DotGap", k.DotGap, 2*4)
	b2hEq(t, "ArrowDisabledOpacity", k.ArrowDisabledOpacity, 0)
	b2hAlpha(t, "DotColor", k.DotColor, .2)
	b2hAlpha(t, "ArrowColor", k.ArrowColor, .4)
	e := ElementPlus().Components.Carousel
	b2hEq(t, "Element ArrowSize", e.ArrowSize, 36)
	b2hEq(t, "Element ArrowOffset", e.ArrowOffset, 16)
	if !e.DotHoverTrigger || k.DotHoverTrigger {
		t.Error("indicator trigger: Element hover, Ant click")
	}
	s := Shadcn().Components.Carousel
	b2hEq(t, "shadcn ArrowSize", s.ArrowSize, 32)
	b2hEq(t, "shadcn ArrowOffset (deviation: inside)", s.ArrowOffset, 16)
}

// TestAlignB2HImageTokens: antd 6.7 es/image/style/index.js
// (previewOperationColor / Hover / Disabled = white at 0.65 / 0.85 / 0.25,
// previewOperationSize fontSizeIcon*1.5 = 18, buttons 18 + 2*paddingSM,
// close marginSM from the corner) and rc-image 1.10 Preview (scaleStep
// 0.5, minScale 1, maxScale 50); Element image-viewer (zoom-rate 1.2,
// min-scale 0.2, max-scale 7, close 40px in, progress 90px up).
func TestAlignB2HImageTokens(t *testing.T) {
	k := AntDesign().Components.Image
	b2hAlpha(t, "PreviewOperationColor", k.PreviewOperationColor, .65)
	b2hAlpha(t, "PreviewOperationHoverColor", k.PreviewOperationHoverColor, .85)
	b2hAlpha(t, "PreviewOperationDisabledColor", k.PreviewOperationDisabledColor, .25)
	b2hEq(t, "PreviewOperationIconSize", k.PreviewOperationIconSize, 18)
	b2hEq(t, "PreviewOperationSize", k.PreviewOperationSize, 42)
	b2hEq(t, "PreviewActionSize", k.PreviewActionSize, 42)
	b2hEq(t, "PreviewActionGap", k.PreviewActionGap, 12)
	b2hEq(t, "PreviewActionsPadding", k.PreviewActionsPadding, 24)
	b2hEq(t, "PreviewCloseOffset", k.PreviewCloseOffset, 12)
	b2hEq(t, "PreviewProgressBottom", k.PreviewProgressBottom, 90)
	b2hEq(t, "PreviewMinScale", k.PreviewMinScale, 1)
	b2hEq(t, "PreviewMaxScale", k.PreviewMaxScale, 50)
	b2hEq(t, "PreviewScaleStep", k.PreviewScaleStep, .5)
	if !k.PreviewMaskClosable || k.PreviewInfinite || !k.PreviewShowProgress {
		t.Error("ant preview behaviour")
	}
	e := ElementPlus().Components.Image
	b2hEq(t, "Element PreviewMinScale", e.PreviewMinScale, .2)
	b2hEq(t, "Element PreviewMaxScale", e.PreviewMaxScale, 7)
	b2hEq(t, "Element PreviewScaleStep", e.PreviewScaleStep, .2)
	b2hEq(t, "Element PreviewCloseOffset", e.PreviewCloseOffset, 40)
	b2hEq(t, "Element PreviewProgressBottom", e.PreviewProgressBottom, 90)
	if e.PreviewMaskClosable || !e.PreviewInfinite || e.PreviewShowProgress {
		t.Error("element preview behaviour")
	}
}

// TestAlignB2HCalendarTokens: the week column takes the picker cell width
// (antd date-picker/style token.js cellWidth = controlHeightSM*1.5).
func TestAlignB2HCalendarTokens(t *testing.T) {
	b2hEq(t, "WeekColumnWidth", AntDesign().Components.Calendar.WeekColumnWidth, 36)
}
