package theme

// SliderToken styles Slider.
type SliderToken struct {
	Alias
	RailSize             float32
	RailBg               Color
	RailHoverBg          Color
	TrackBg              Color `contrast:"nontext:@Page"`
	TrackHoverBg         Color
	TrackBgDisabled      Color
	HandleSize           float32
	HandleSizeHover      float32
	HandleLineWidth      float32
	HandleLineWidthHover float32
	HandleColor          Color `contrast:"nontext:@Page"` // handle ring
	HandleActiveColor    Color
	HandleColorDisabled  Color
	HandleBg             Color
	// HandleActiveOutlineColor is the halo around a focused (Ant) or
	// hovered / focused (shadcn) handle; zero: none.
	HandleActiveOutlineColor Color `zero:"none"`
	DotSize                  float32
	DotBorderColor           Color // step / mark dot ring off the filled part
}

func deriveSlider(t *Theme) SliderToken {
	k := SliderToken{
		RailSize: t.SliderRailHeight, RailBg: t.FillTertiary, RailHoverBg: t.FillSecondary,
		TrackBg: t.Primary.Border, TrackHoverBg: t.Primary.BorderHover, TrackBgDisabled: t.TextDisabled,
		HandleSize: 10, HandleSizeHover: 12, HandleLineWidth: 2, HandleLineWidthHover: 4,
		HandleColor: t.Primary.Border, HandleActiveColor: t.Primary.Base, HandleColorDisabled: t.TextDisabled,
		HandleBg: t.BgContainer, HandleActiveOutlineColor: Fade(t.Primary.Base, .2),
		DotSize: 8, DotBorderColor: t.BorderSecondary,
	}
	switch t.Flavor {
	case Element:
		k.RailBg, k.RailHoverBg = t.Split, t.Split
		k.TrackBg, k.TrackHoverBg, k.HandleColor = t.Primary.Base, t.Primary.Base, t.Primary.Base
		k.HandleActiveColor = t.Primary.Base
		k.HandleSize, k.HandleSizeHover = 20, 24
		k.HandleLineWidthHover = k.HandleLineWidth
		k.HandleActiveOutlineColor = Color{}
		k.DotSize = 6
	case StyleShadcn:
		k.RailBg, k.RailHoverBg = t.FillSecondary, t.FillSecondary
		k.TrackBg, k.TrackHoverBg, k.HandleColor = t.Primary.Base, t.Primary.Base, t.Primary.Base
		k.HandleActiveColor = t.Primary.Base
		k.HandleSize, k.HandleSizeHover, k.HandleLineWidth, k.HandleLineWidthHover = 16, 16, 1, 1
		// bg-white thumb in light and dark mode.
		k.HandleBg = Hex("#ffffff")
		// ring-4 ring-ring/50: the input halo carries ring/50.
		k.HandleActiveOutlineColor = t.ControlOutline
		if k.HandleActiveOutlineColor.A == 0 {
			k.HandleActiveOutlineColor = Fade(t.FocusOutline, .5)
		}
	}
	return k
}
