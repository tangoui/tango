package theme

// StepsToken styles Steps. Per status (Process, Finish, Wait, Error and
// Success, the "success" finish status) it holds the icon fill, the icon
// glyph / number color, the icon outline (zero: none), the title and the
// description colors.
type StepsToken struct {
	Alias
	IconSize            float32 // icon circle, default size
	IconSizeSM          float32
	NavIconSize         float32 // icon circle of the navigation type
	NavIconSizeSM       float32
	IconFontSize        float32 // step number
	IconFontSizeSM      float32
	NavIconFontSize     float32
	NavIconFontSizeSM   float32
	DotSize             float32
	DotCurrentSize      float32
	TitleFontSize       float32
	TitleFontSizeSM     float32
	NavTitleFontSize    float32
	NavTitleFontSizeSM  float32
	DescriptionFontSize float32
	DescriptionMaxWidth float32 // horizontal steps with labels beside the icon
	TailWidth           float32 // connector thickness
	DotTailWidth        float32 // connector of horizontal dot steps

	ProcessIconBg           Color
	ProcessIconColor        Color `contrast:"text:ProcessIconBg"`
	ProcessIconBorderColor  Color // zero: none
	ProcessTitleColor       Color `contrast:"text:@Page"`
	ProcessDescriptionColor Color `contrast:"text:@Page"`

	FinishIconBg           Color
	FinishIconColor        Color `contrast:"nontext:FinishIconBg"`
	FinishIconBorderColor  Color // zero: none
	FinishTitleColor       Color `contrast:"text:@Page"`
	FinishDescriptionColor Color `contrast:"text:@Page"`

	WaitIconBg           Color
	WaitIconColor        Color `contrast:"text:WaitIconBg"`
	WaitIconBorderColor  Color // zero: none
	WaitTitleColor       Color `contrast:"text:@Page"`
	WaitDescriptionColor Color `contrast:"text:@Page"`

	ErrorIconBg           Color
	ErrorIconColor        Color `contrast:"nontext:ErrorIconBg"`
	ErrorIconBorderColor  Color // zero: none
	ErrorTitleColor       Color `contrast:"text:@Page"`
	ErrorDescriptionColor Color `contrast:"text:@Page"`

	SuccessIconBg           Color
	SuccessIconColor        Color `contrast:"nontext:SuccessIconBg"`
	SuccessIconBorderColor  Color // zero: none
	SuccessTitleColor       Color `contrast:"text:@Page"`
	SuccessDescriptionColor Color `contrast:"text:@Page"`

	TailColor        Color // connector after a waiting / current / failed step
	FinishTailColor  Color
	SuccessTailColor Color

	// Dots and custom icons (Ant and shadcn; Element uses the icon colors).
	DotActiveColor Color `contrast:"nontext:@Page"` // current and finished steps
	DotErrorColor  Color `contrast:"nontext:@Page"`
	DotWaitColor   Color

	HoverColor           Color `contrast:"text:@Page"` // clickable step under the pointer
	HoverIconBorderColor Color // outline of a waiting step's icon under the pointer
	SubTitleColor        Color `contrast:"text:@Page"`
	ProgressColor        Color // percent ring of the current step
	ProgressTrailColor   Color

	NavArrowColor  Color // chevron between navigation steps
	NavInkBarColor Color // underline of the current navigation step (zero: none)
	NavBg          Color // navigation strip (zero: none)
	NavRadius      float32

	// Inline type: a compact row of small dots with the titles under them.
	InlineDotSize        float32
	InlineDotCurrentSize float32
	InlineTitleFontSize  float32
	InlinePaddingX       float32 // inside each step, beside the title
	InlinePaddingY       float32
	InlineHoverBg        Color   // clickable step under the pointer
	InlineRadius         float32 // hover background

	// Panel type: arrow-shaped panels in a row.
	PanelBg         Color   `contrast:"nontext:@Page"` // waiting and finished steps
	PanelActiveBg   Color   // the current step
	PanelErrorBg    Color   // a failed step
	PanelHoverBg    Color   // clickable step under the pointer
	PanelArrowWidth float32 // depth of the arrow point and notch
	PanelGap        float32 // gap between two panels
	PanelPaddingX   float32
	PanelPaddingY   float32
	PanelRadius     float32 // outer corners of the row
	PanelIconSize   float32
}

func deriveSteps(t *Theme) StepsToken {
	k := StepsToken{
		IconSize: 32, IconSizeSM: 24, NavIconSize: 32, NavIconSizeSM: 24,
		IconFontSize: t.FontSizeLG, IconFontSizeSM: t.FontSizeSM, NavIconFontSize: t.FontSizeLG, NavIconFontSizeSM: t.FontSizeSM,
		DotSize: 8, DotCurrentSize: 10,
		TitleFontSize: t.FontSizeLG, TitleFontSizeSM: t.FontSize, NavTitleFontSize: t.FontSizeLG, NavTitleFontSizeSM: t.FontSize,
		DescriptionFontSize: t.FontSize, DescriptionMaxWidth: 140,
		TailWidth: 1, DotTailWidth: 3,

		ProcessIconBg: t.Primary.Base, ProcessIconColor: t.TextOnPrimary, ProcessTitleColor: t.Text, ProcessDescriptionColor: t.Text,
		FinishIconBg: t.Primary.Bg, FinishIconColor: t.Primary.Base, FinishTitleColor: t.Text, FinishDescriptionColor: t.TextTertiary,
		WaitIconBg: t.FillSecondary, WaitIconColor: t.TextTertiary, WaitTitleColor: t.TextTertiary, WaitDescriptionColor: t.TextTertiary,
		ErrorIconBg: t.Error.Bg, ErrorIconColor: t.Error.Base, ErrorTitleColor: t.Error.Base, ErrorDescriptionColor: t.Error.Base,
		SuccessIconBg: t.Success.Bg, SuccessIconColor: t.Success.Base, SuccessTitleColor: t.Text, SuccessDescriptionColor: t.TextTertiary,
		TailColor: t.Split, FinishTailColor: t.Primary.Base, SuccessTailColor: t.Success.Base,

		DotActiveColor: t.Primary.Base, DotErrorColor: t.Error.Base, DotWaitColor: t.TextQuaternary,
		HoverColor: t.Primary.Base, HoverIconBorderColor: t.Primary.Base, SubTitleColor: t.TextTertiary,
		ProgressColor: t.Primary.Base, ProgressTrailColor: t.Split,
		NavArrowColor: t.TextQuaternary, NavInkBarColor: t.Primary.Base,

		InlineDotSize: 6, InlineDotCurrentSize: 8, InlineTitleFontSize: t.FontSizeSM,
		InlinePaddingX: 8, InlinePaddingY: 4, InlineHoverBg: t.FillTertiary, InlineRadius: t.RadiusSM,

		PanelBg: t.FillTertiary, PanelActiveBg: t.Primary.Bg, PanelErrorBg: t.Error.Bg, PanelHoverBg: t.FillSecondary,
		PanelArrowWidth: 12, PanelGap: 4, PanelPaddingX: 16, PanelPaddingY: 10, PanelRadius: t.Radius, PanelIconSize: 24,
	}
	switch t.Flavor {
	case Element:
		// el-steps: outlined icons, everything of a step in its status color.
		k.IconSize, k.NavIconSize, k.NavIconSizeSM = 24, 16, 16
		k.IconFontSize, k.IconFontSizeSM = t.FontSize, t.FontSize
		k.NavIconFontSize, k.NavIconFontSizeSM = t.FontSizeSM, t.FontSizeSM
		k.TitleFontSizeSM = t.FontSizeLG
		k.NavTitleFontSize, k.NavTitleFontSizeSM = 13, 13
		k.DescriptionFontSize = t.FontSizeSM
		k.TailWidth = 2
		status := func(col Color) (bg, fg, border, title, desc Color) {
			return t.BgContainer, col, col, col, col
		}
		k.ProcessIconBg, k.ProcessIconColor, k.ProcessIconBorderColor, k.ProcessTitleColor, k.ProcessDescriptionColor = status(t.Text)
		k.FinishIconBg, k.FinishIconColor, k.FinishIconBorderColor, k.FinishTitleColor, k.FinishDescriptionColor = status(t.Primary.Base)
		k.WaitIconBg, k.WaitIconColor, k.WaitIconBorderColor, k.WaitTitleColor, k.WaitDescriptionColor = status(t.TextQuaternary)
		k.ErrorIconBg, k.ErrorIconColor, k.ErrorIconBorderColor, k.ErrorTitleColor, k.ErrorDescriptionColor = status(t.Error.Base)
		k.SuccessIconBg, k.SuccessIconColor, k.SuccessIconBorderColor, k.SuccessTitleColor, k.SuccessDescriptionColor = status(t.Success.Base)
		k.TailColor = t.BorderHover
		k.NavArrowColor, k.NavInkBarColor = t.TextTertiary, Color{}
		k.NavBg, k.NavRadius = t.FillTertiary, 4
		k.InlineDotSize, k.InlineDotCurrentSize = 8, 8
		k.PanelIconSize = 20
	case StyleShadcn:
		// No shadcn Steps; its stepper patterns: finished and current
		// steps solid primary, waiting ones outlined, errors outlined in
		// destructive, descriptions muted.
		k.ProcessDescriptionColor = t.TextTertiary
		k.FinishIconBg, k.FinishIconColor = t.Primary.Base, t.TextOnPrimary
		k.SuccessIconBg, k.SuccessIconColor = t.Success.Base, t.TextOnSolid
		k.ErrorIconBg, k.ErrorIconBorderColor = t.BgContainer, t.Error.Base
		k.WaitIconBg, k.WaitIconBorderColor = t.BgContainer, t.Border
		k.HoverColor, k.HoverIconBorderColor = t.Text, t.BorderHover
		k.InlineHoverBg, k.PanelHoverBg = t.ItemHoverBg, t.ItemHoverBg
		k.PanelActiveBg = t.Primary.BgHover
	}
	return k
}
