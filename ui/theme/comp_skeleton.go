package theme

// SkeletonToken styles Skeleton.
type SkeletonToken struct {
	Alias
	BlockRadius              float32
	GradientFromColor        Color
	GradientToColor          Color
	IconColor                Color // picture placeholder icon
	TitleHeight              float32
	ParagraphLiHeight        float32
	ParagraphMarginTop       float32 // title to the first paragraph line
	ParagraphMarginTopAvatar float32 // the same next to an avatar
	ParagraphGap             float32 // between paragraph lines
}

func deriveSkeleton(t *Theme) SkeletonToken {
	k := SkeletonToken{
		BlockRadius: t.RadiusSM, GradientFromColor: t.SkeletonBg, GradientToColor: t.SkeletonHighlight, IconColor: t.SkeletonIcon,
		TitleHeight: 16, ParagraphLiHeight: 16, ParagraphMarginTop: 28, ParagraphMarginTopAvatar: 24, ParagraphGap: 16,
	}
	switch t.Flavor {
	case Element:
		k.ParagraphMarginTop, k.ParagraphMarginTopAvatar = 16, 16
	case StyleShadcn:
		k.BlockRadius = t.Radius // rounded-md
	}
	return k
}
