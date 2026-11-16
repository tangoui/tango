package theme

// SpinIndicator is the kind of indicator Spin draws (SpinToken.Indicator).
// Append-only: never reorder or insert.
type SpinIndicator int

const (
	// SpinDots is antd's four rotating dots.
	SpinDots SpinIndicator = iota
	// SpinCircular is Element Plus's rotating circular arc (v-loading).
	SpinCircular
	// SpinIcon spins the theme's Loading icon (shadcn Spinner:
	// lucide Loader2).
	SpinIcon
)

// SpinToken styles Spin and Loading (antd es/spin/style, EP loading, shadcn spinner.tsx).
type SpinToken struct {
	Alias
	// Indicator is SpinDots, SpinCircular or SpinIcon.
	Indicator SpinIndicator
	// DotSize, DotSizeSM and DotSizeLG are the indicator sizes of the
	// middle, small and large Spin (antd dotSize / dotSizeSM /
	// dotSizeLG; EP --el-loading-spinner-size).
	DotSize   float32
	DotSizeSM float32
	DotSizeLG float32
	// FullscreenDotSize is the indicator size of a fullscreen loading
	// (EP --el-loading-fullscreen-spinner-size; zero: the size of the
	// spin).
	FullscreenDotSize float32
	// Color is the indicator (and description) color.
	Color Color
	// DescriptionGap is the space between indicator and description.
	DescriptionGap      float32
	DescriptionFontSize float32
	// ContainerOpacity is the opacity of content wrapped by a spinning
	// Spin / Loading; MaskBg covers it.
	ContainerOpacity float64
	MaskBg           Color
	// FullscreenMaskBg and FullscreenColor are the backdrop and the
	// indicator color of a fullscreen loading.
	FullscreenMaskBg Color
	FullscreenColor  Color
	// ProgressStrokeRatio is the width of the Percent ring as a fraction
	// of the indicator size (antd viewSize / 5).
	ProgressStrokeRatio float64
}

func deriveSpin(t *Theme) SpinToken {
	// antd es/spin/style/index.js prepareComponentToken and genSpinStyle.
	k := SpinToken{
		Indicator: SpinDots,
		DotSize:   t.ControlHeightLG / 2, DotSizeSM: t.ControlHeightLG * 0.35, DotSizeLG: t.ControlHeight,
		Color:          t.Primary.Base,
		DescriptionGap: t.PaddingSM, DescriptionFontSize: t.FontSize,
		ContainerOpacity: 0.5, MaskBg: Fade(t.BgContainer, 0.4),
		FullscreenMaskBg: t.BgMask, FullscreenColor: Hex("#ffffff"),
		ProgressStrokeRatio: 0.2,
	}
	switch t.Flavor {
	case Element:
		// theme-chalk/src/loading.scss: 42px circular spinner (50px
		// fullscreen), text margin 3px 0, mask --el-mask-color over the
		// content (which is not dimmed).
		k.Indicator = SpinCircular
		k.DotSize, k.DotSizeSM, k.DotSizeLG, k.FullscreenDotSize = 42, 28, 50, 50
		k.DescriptionGap = 3
		k.ContainerOpacity = 1
		k.MaskBg = RGBA(255, 255, 255, 0.9)
		if t.Dark {
			k.MaskBg = RGBA(0, 0, 0, 0.8) // dark/var.scss $mask-color
		}
		k.FullscreenMaskBg, k.FullscreenColor = k.MaskBg, t.Primary.Base
	case StyleShadcn:
		// spinner.tsx: Loader2Icon size-4 animate-spin in the current
		// text color.
		k.Indicator = SpinIcon
		// size-4 by default; small / large follow Tailwind's size-3 /
		// size-6 (the component has no size prop).
		k.DotSize, k.DotSizeSM, k.DotSizeLG = 16, 12, 24
		k.Color = t.Text
		k.DescriptionGap = 8
	}
	return k
}
