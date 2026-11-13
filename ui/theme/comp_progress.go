package theme

// ProgressToken styles Progress.
type ProgressToken struct {
	Alias
	DefaultColor      Color `contrast:"nontext:@Page"` // bar without status or Color
	RemainingColor    Color // rail (zero: the bar color at 20% over the page, shadcn bg-primary/20)
	SuccessColor      Color
	ExceptionColor    Color
	WarningColor      Color
	TextColor         Color `contrast:"text:@Page"` // percentage of lines and circles
	InnerTextColor    Color // TextInside label on the bar
	LineHeight        float32
	LineHeightSM      float32
	InnerLineHeight   float32 // TextInside bar
	StepsHeight       float32
	StepsWidth        float32 // one step segment
	StepsWidthSM      float32
	StepsGap          float32
	CircleSize        float32 // circle / dashboard diameter
	CircleSizeSM      float32
	CircleStrokeWidth float32 // zero: 6% of the diameter
	InnerFontSize     float32
}

func deriveProgress(t *Theme) ProgressToken {
	k := ProgressToken{
		DefaultColor: t.Primary.Base, RemainingColor: t.FillSecondary,
		SuccessColor: t.Success.Base, ExceptionColor: t.Error.Base, WarningColor: t.Warning.Base,
		TextColor: t.Text, InnerTextColor: Hex("#ffffff"),
		LineHeight: 8, LineHeightSM: 6, InnerLineHeight: 20, StepsHeight: 8,
		StepsWidth: 14, StepsWidthSM: 2, StepsGap: 2, CircleSize: 120, CircleSizeSM: 80,
		InnerFontSize: t.FontSizeSM,
	}
	switch t.Flavor {
	case Element:
		k.RemainingColor, k.TextColor = t.BorderSecondary, t.TextSecondary
		k.LineHeight, k.CircleSize, k.CircleStrokeWidth = 6, 126, 6
	case StyleShadcn:
		k.RemainingColor = Color{} // follows the bar color
	}
	return k
}
