package theme

// CarouselToken styles Carousel.
type CarouselToken struct {
	Alias
	DotWidth        float32
	DotActiveWidth  float32
	DotHeight       float32
	DotRadius       float32
	DotGap          float32
	DotColor        Color // on the slides
	DotHoverColor   Color
	DotActiveColor  Color
	DotOutsideColor Color // DotPosition("outside"), on the page
	// DotRingColor outlines the dots drawn on the slides, so the light
	// dots stay visible on light slides (zero: none).
	DotRingColor          Color
	DotOutsideActiveColor Color `contrast:"nontext:@Page"`
	ArrowSize             float32
	ArrowBg               Color // zero: none
	ArrowHoverBg          Color
	ArrowColor            Color
	ArrowHoverColor       Color
	ArrowDisabledOpacity  float32 // Loop(false): arrow at the first / last slide
	Radius                float32 // frame corners (not in card mode)
	CardMaskColor         Color   // dims the side cards of Card mode
}

func deriveCarousel(t *Theme) CarouselToken {
	white := Hex("#ffffff")
	k := CarouselToken{
		DotWidth: 16, DotActiveWidth: 24, DotHeight: 3, DotRadius: 1.5, DotGap: 8,
		DotColor: RGBA(255, 255, 255, .3), DotHoverColor: RGBA(255, 255, 255, .75), DotActiveColor: white,
		DotOutsideColor: t.Fill, DotOutsideActiveColor: t.Primary.Base, DotRingColor: RGBA(0, 0, 0, .3),
		ArrowSize: 28, ArrowColor: RGBA(255, 255, 255, .6), ArrowHoverColor: white, CardMaskColor: t.BgContainer,
		ArrowDisabledOpacity: .35,
	}
	switch t.Flavor {
	case Element:
		k.DotWidth, k.DotActiveWidth, k.DotHeight, k.DotRadius, k.DotGap = 30, 30, 2, 0, 0
		k.DotColor, k.DotHoverColor, k.DotOutsideColor = RGBA(255, 255, 255, .48), white, t.Border
		k.ArrowSize, k.ArrowColor = 36, white
		k.ArrowBg, k.ArrowHoverBg = RGBA(31, 45, 61, .11), RGBA(31, 45, 61, .23)
	case StyleShadcn:
		k.ArrowSize, k.Radius = 32, t.RadiusLG // size-8 outline buttons; items in rounded cards
	}
	return k
}
