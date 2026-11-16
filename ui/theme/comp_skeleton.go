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
	TitleMarginTopAvatar     float32 // title offset next to an avatar
	AvatarSize               float32 // the avatar of Skeleton.Avatar (antd size "large")
	// Pulse animates Active blocks by fading them to half opacity and
	// back (shadcn animate-pulse) instead of the sweeping highlight band.
	Pulse bool
}

func deriveSkeleton(t *Theme) SkeletonToken {
	// antd 6.7 es/skeleton/style/index.js: titleHeight / paragraphLiHeight
	// controlHeight / 2 = 16; title + paragraph marginBlockStart
	// controlHeightSM 24, with an avatar paragraphMarginTop marginLG +
	// marginXXS = 28 and the title marginBlockStart marginSM 12; li + li
	// controlHeightXS 16; Skeleton.js avatar size "large" (controlHeightLG).
	k := SkeletonToken{
		BlockRadius: t.RadiusSM, GradientFromColor: t.SkeletonBg, GradientToColor: t.SkeletonHighlight, IconColor: t.SkeletonIcon,
		TitleHeight: 16, ParagraphLiHeight: 16, ParagraphMarginTop: 24, ParagraphMarginTopAvatar: 28, ParagraphGap: 16,
		TitleMarginTopAvatar: 12, AvatarSize: t.ControlHeightLG,
	}
	switch t.Flavor {
	case Element:
		k.ParagraphMarginTop, k.ParagraphMarginTopAvatar = 16, 16
	case StyleShadcn:
		k.BlockRadius = t.Radius // rounded-md
		k.Pulse = true           // skeleton.tsx: bg-accent animate-pulse
	}
	return k
}
