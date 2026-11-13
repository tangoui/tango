package ui

import "github.com/tangoui/tango/ui/theme"

// shadSliderRing is the hover / focus ring of a shadcn Slider thumb
// (ring-4 ring-ring/50). The value lives in the Slider token
// (theme.SliderToken.HandleActiveOutlineColor, derived from the input
// halo); the thumb's bg-white fill is SliderToken.HandleBg.
func shadSliderRing(th *theme.Theme) theme.Color {
	return th.For(theme.CompSlider).Components.Slider.HandleActiveOutlineColor
}
