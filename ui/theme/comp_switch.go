package theme

import "github.com/tangoui/tango/ui/paint"

// SwitchToken styles Switch.
type SwitchToken struct {
	Alias
	TrackHeight     float32
	TrackHeightSM   float32
	TrackMinWidth   float32
	TrackMinWidthSM float32
	TrackPadding    float32 // gap between track and thumb, middle size
	HandleSize      float32
	HandleSizeSM    float32
	// Large size (Switch.Large / Size(Large)).
	TrackHeightLG   float32
	TrackMinWidthLG float32
	HandleSizeLG    float32
	// InnerFontSize is the inline prompt text (Texts) inside the track;
	// InnerFontSizeLG the same for the large size.
	InnerFontSize   float32
	InnerFontSizeLG float32
	// LabelGap separates the outer labels (OuterTexts) from the track;
	// LabelFontSize is their font size.
	LabelGap      float32
	LabelFontSize float32
	HandleBg      Color // thumb when on
	HandleOffBg   Color // thumb when off
	OnBg          Color `contrast:"nontext:@Page"` // track when on
	OnHoverBg     Color
	OffBg         Color `contrast:"nontext:@Page"` // track when off (its boundary)
	OffHoverBg    Color

	TrackShadow  []paint.Shadow // shadcn shadow-xs
	HandleShadow []paint.Shadow
}

func deriveSwitch(t *Theme) SwitchToken {
	k := SwitchToken{
		TrackHeight: t.SwitchHeight, TrackMinWidth: t.SwitchWidth,
		HandleSize: t.SwitchHeight - 4,
		HandleBg:   t.SwitchHandle, HandleOffBg: t.SwitchHandleOff,
		OnBg: t.Primary.Base, OnHoverBg: t.Primary.Hover,
		OffBg: t.SwitchOffBg, OffHoverBg: t.SwitchOffBgHover,
		TrackShadow:  t.InputShadow,
		HandleShadow: []paint.Shadow{{Y: 2, Blur: 4, Color: RGBA(0, 35, 11, .2)}},
	}
	k.TrackHeightSM, k.TrackMinWidthSM, k.HandleSizeSM = 16, 28, 12
	// Ant has no large switch: one step up from the default.
	k.TrackHeightLG, k.TrackMinWidthLG = k.TrackHeight+4, k.TrackMinWidth+10
	k.InnerFontSize, k.InnerFontSizeLG = 12, 14
	k.LabelGap, k.LabelFontSize = 8, t.FontSize
	switch t.Flavor {
	case Element:
		k.TrackMinWidthSM = 30
		k.OnHoverBg = t.Primary.Base
		// el-switch--large: core 24px high, 50px wide, 20px handle.
		k.TrackHeightLG, k.TrackMinWidthLG = 24, 50
		k.LabelGap = 10
	case StyleShadcn:
		// new-york v4: h-[1.15rem] w-8 with a size-4 thumb; sm h-3.5 w-6
		// with a size-3 thumb; ring-0 (no thumb shadow).
		k.HandleSize = 16
		k.TrackHeightSM, k.TrackMinWidthSM = 14, 24
		// Large: the classic h-6 w-11 track with a size-5 thumb.
		k.TrackHeightLG, k.TrackMinWidthLG = 24, 44
		k.OnHoverBg = t.Primary.Base
		k.HandleShadow = nil
	}
	k.TrackPadding = (k.TrackHeight - k.HandleSize) / 2
	k.HandleSizeLG = k.TrackHeightLG - 2*k.TrackPadding
	if t.Flavor == StyleShadcn {
		k.HandleSizeLG = 20
	}
	return k
}
