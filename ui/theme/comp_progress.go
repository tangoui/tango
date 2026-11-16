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
	// ShowInfo makes a line bar show its percentage unless HideInfo is
	// called; false shows only the bar unless ShowInfo is (shadcn).
	ShowInfo bool
}

func deriveProgress(t *Theme) ProgressToken {
	// antd 6.7 es/progress/utils.js getSize: line 8 / small 6; steps 14 / small
	// 2 wide, 8 high; circle 120 / small 60.
	k := ProgressToken{
		DefaultColor: t.Primary.Base, RemainingColor: t.FillSecondary,
		SuccessColor: t.Success.Base, ExceptionColor: t.Error.Base, WarningColor: t.Warning.Base,
		TextColor: t.Text, InnerTextColor: Hex("#ffffff"),
		LineHeight: 8, LineHeightSM: 6, InnerLineHeight: 20, StepsHeight: 8,
		StepsWidth: 14, StepsWidthSM: 2, StepsGap: 2, CircleSize: 120, CircleSizeSM: 60,
		InnerFontSize: t.FontSizeSM, ShowInfo: true,
	}
	switch t.Flavor {
	case Element:
		k.RemainingColor, k.TextColor = t.BorderSecondary, t.TextSecondary
		k.LineHeight, k.CircleSize, k.CircleStrokeWidth = 6, 126, 6
	case StyleShadcn:
		k.RemainingColor = Color{} // follows the bar color
		k.ShowInfo = false         // progress.tsx has no label
	}
	return k
}
