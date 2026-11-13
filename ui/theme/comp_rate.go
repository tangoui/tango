package theme

// RateToken styles Rate.
type RateToken struct {
	Alias
	StarColor         Color `contrast:"nontext:@Page"` // filled stars
	StarBg            Color `contrast:"nontext:@Page"` // empty stars
	StarBgDisabled    Color // empty stars of a disabled rate
	StarOutlineColor  Color // draws empty stars as outlines in this color (zero: filled StarBg)
	StarSize          float32
	StarGap           float32
	StarHoverScale    float32 // scale of the hovered star (1: none)
	Height            float32 // zero: the star size + 2, at least 22
	TextColor         Color   `contrast:"text:@Page"` // texts after the stars
	ScoreColor        Color   // score after the stars (zero: the star color)
	TextColorDisabled Color   // texts and score of a disabled rate (zero: unchanged)
}

func deriveRate(t *Theme) RateToken {
	k := RateToken{
		StarColor: t.RateStar, StarBg: t.RateStarEmpty, StarBgDisabled: t.RateStarEmpty,
		StarSize: 20, StarGap: 8, StarHoverScale: 1.1,
		TextColor: t.Text, ScoreColor: t.Text, TextColorDisabled: t.TextTertiary,
	}
	switch t.Flavor {
	case Element:
		// el-rate: 18px icons, 6px apart in a 32px row, the score in the
		// star color; the void of a disabled rate uses the fill color, no
		// hover zoom.
		k.StarSize, k.StarGap, k.StarHoverScale, k.Height = 18, 6, 1, 32
		k.StarBgDisabled = t.Fill
		k.ScoreColor, k.TextColorDisabled = Color{}, Color{}
	case StyleShadcn:
		// No shadcn counterpart: lucide outline stars for the voids.
		k.StarOutlineColor = t.TextTertiary
		k.StarHoverScale = 1
		k.TextColorDisabled = Color{}
	}
	return k
}
