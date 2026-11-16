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
	// ArrowOffset is the distance from the frame edge to an arrow
	// (negative: outside the frame, as shadcn's -left-12).
	ArrowOffset float32
	// ArrowIconInset is the padding between an arrow's box and its icon.
	ArrowIconInset float32
	// DotHoverTrigger makes hovering an indicator select its slide by
	// default (Element trigger="hover"); Carousel.Trigger overrides it.
	DotHoverTrigger bool
}

func deriveCarousel(t *Theme) CarouselToken {
	white := Hex("#ffffff")
	// antd 6.7 es/carousel/style/index.js: prepareComponentToken
	// (arrowSize 16, arrowOffset marginXS 8, dotWidth 16, dotHeight 3,
	// dotActiveWidth 24, dotGap marginXXS 4 on each side); dot buttons are
	// colorBgContainer at opacity 0.2 (hover 0.75, active 1), arrows #fff at
	// opacity 0.4 (hover 1) and hidden when disabled (slick-disabled
	// opacity 0).
	k := CarouselToken{
		DotWidth: 16, DotActiveWidth: 24, DotHeight: 3, DotRadius: 1.5, DotGap: 8,
		DotColor: RGBA(255, 255, 255, .2), DotHoverColor: RGBA(255, 255, 255, .75), DotActiveColor: white,
		DotOutsideColor: t.Fill, DotOutsideActiveColor: t.Primary.Base, DotRingColor: RGBA(0, 0, 0, .3),
		ArrowSize: 16, ArrowColor: RGBA(255, 255, 255, .4), ArrowHoverColor: white, CardMaskColor: t.BgContainer,
		ArrowDisabledOpacity: 0, ArrowOffset: 8,
	}
	switch t.Flavor {
	case Element:
		// theme-chalk carousel.scss: 36px round arrows 16px from the edge.
		k.DotWidth, k.DotActiveWidth, k.DotHeight, k.DotRadius, k.DotGap = 30, 30, 2, 0, 0
		k.DotColor, k.DotHoverColor, k.DotOutsideColor = RGBA(255, 255, 255, .48), white, t.Border
		k.ArrowSize, k.ArrowColor, k.ArrowOffset, k.ArrowIconInset = 36, white, 16, 10
		k.ArrowBg, k.ArrowHoverBg = RGBA(31, 45, 61, .11), RGBA(31, 45, 61, .23)
		k.ArrowDisabledOpacity, k.DotHoverTrigger = .35, true
	case StyleShadcn:
		// carousel.tsx: size-8 outline buttons; items in rounded cards.
		// Deviation: shadcn puts the buttons outside the frame (-left-12 /
		// -right-12), which needs 48px of free room beside every carousel
		// (in a Card or Scroll they get clipped or cover the neighbour), so
		// the default keeps them 16px inside; set ArrowOffset to -48 through
		// ConfigProvider.Components for the outside look.
		k.ArrowSize, k.Radius, k.ArrowOffset, k.ArrowIconInset = 32, t.RadiusLG, 16, 8
		k.ArrowDisabledOpacity = .5 // Button disabled:opacity-50
		k.DotColor = RGBA(255, 255, 255, .3)
	}
	return k
}
