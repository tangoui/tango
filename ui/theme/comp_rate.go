package theme

// RateToken styles Rate.
type RateToken struct {
	Alias
	StarColor         Color `contrast:"nontext:@Page"` // filled stars
	StarBg            Color `contrast:"nontext:@Page"` // empty stars
	StarBgDisabled    Color // empty stars of a disabled rate
	StarOutlineColor  Color // draws empty stars as outlines in this color (zero: filled StarBg)
	StarSize          float32
	StarSizeSM        float32 // Rate.Small
	StarSizeLG        float32 // Rate.Large
	StarGap           float32
	StarHoverScale    float32 // scale of the hovered star (1: none)
	Height            float32 // zero: the star size + 2, at least 22
	HeightSM          float32 // Rate.Small (zero: from the star size)
	HeightLG          float32 // Rate.Large (zero: from the star size)
	TextColor         Color   `contrast:"text:@Page"` // texts after the stars
	ScoreColor        Color   // score after the stars (zero: the star color)
	TextColorDisabled Color   // texts and score of a disabled rate (zero: unchanged)
}

func deriveRate(t *Theme) RateToken {
	k := RateToken{
		StarColor: t.RateStar, StarBg: t.RateStarEmpty, StarBgDisabled: t.RateStarEmpty,
		// antd rate/style: starSize controlHeight*0.625, SM / LG from
		// controlHeightSM / LG; stars marginXS apart.
		StarSize: t.ControlHeight * 0.625, StarSizeSM: t.ControlHeightSM * 0.625, StarSizeLG: t.ControlHeightLG * 0.625,
		StarGap: 8, StarHoverScale: 1.1,
		TextColor: t.Text, ScoreColor: t.Text, TextColorDisabled: t.TextTertiary,
	}
	switch t.Flavor {
	case Element:
		// el-rate: 18px icons, 6px apart in a 32px row, the score in the
		// star color; the void of a disabled rate uses the fill color, no
		// hover zoom.
		// rate.scss: $rate-height large 40 / default 32 / small 24,
		// $rate-size small 14 (large keeps the 18px icon).
		k.StarSize, k.StarGap, k.StarHoverScale, k.Height = 18, 6, 1, 32
		k.StarSizeSM, k.StarSizeLG, k.HeightSM, k.HeightLG = 14, 18, 24, 40
		k.StarBgDisabled = t.Fill
		k.ScoreColor, k.TextColorDisabled = Color{}, Color{}
	case StyleShadcn:
		// No shadcn counterpart: lucide outline stars for the voids, the
		// Ant default star sizes (not derived from the taller controls).
		k.StarSize, k.StarSizeSM, k.StarSizeLG = 20, 15, 25
		k.StarOutlineColor = t.TextTertiary
		k.StarHoverScale = 1
		k.TextColorDisabled = Color{}
	}
	return k
}
