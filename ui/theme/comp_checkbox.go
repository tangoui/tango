package theme

import "github.com/tangoui/tango/ui/paint"

// CheckboxToken styles Checkbox.
type CheckboxToken struct {
	Alias
	ControlInteractiveSize float32 // box size
	Radius                 float32 // box corner radius
	Bg                     Color   // unchecked box fill
	BorderColor            Color   `contrast:"nontext:@Page"`
	CheckedBg              Color   `contrast:"nontext:@Page"`
	CheckedHoverBg         Color
	CheckColor             Color `contrast:"nontext:CheckedBg"` // the check mark
	LabelFontWeight        int
	Shadow                 []paint.Shadow // unchecked box shadow (shadcn shadow-xs)
	// ControlInteractiveSizeSM is the box of a small checkbox (Element
	// shrinks it; the other systems keep one size).
	ControlInteractiveSizeSM float32

	// Bordered checkbox (Border): inline padding before the box and after
	// the label, and the frame color of a checked one.
	BorderedPaddingStart float32
	BorderedPaddingEnd   float32
	BorderedCheckedColor Color `contrast:"nontext:@Page"`

	// Button-style checkbox (Button / CheckboxGroupW.Buttons).
	ButtonBg              Color
	ButtonCheckedBg       Color
	ButtonColor           Color `contrast:"text:ButtonBg"`
	ButtonCheckedColor    Color `contrast:"text:ButtonCheckedBg"`
	ButtonCheckedBorder   Color
	ButtonPaddingInline   float32
	ButtonPaddingInlineSM float32
}

func deriveCheckbox(t *Theme) CheckboxToken {
	k := CheckboxToken{
		ControlInteractiveSize: t.CheckboxSize, Radius: t.CheckboxRadius,
		Bg: t.BgContainer, BorderColor: t.Border,
		CheckedBg: t.Primary.Base, CheckedHoverBg: t.Primary.Base, CheckColor: t.TextOnPrimary,
		LabelFontWeight: t.LabelFontWeight,
		Shadow:          t.InputShadow,

		ControlInteractiveSizeSM: t.CheckboxSize,
		BorderedPaddingStart:     9, BorderedPaddingEnd: 15,
		BorderedCheckedColor: t.Primary.Base,
		ButtonBg:             t.BgContainer, ButtonCheckedBg: t.BgContainer,
		ButtonColor: t.Text, ButtonCheckedColor: t.Primary.Base, ButtonCheckedBorder: t.Primary.Base,
		ButtonPaddingInline: 15, ButtonPaddingInlineSM: 7,
	}
	switch t.Flavor {
	case Element:
		// el-checkbox--small shrinks the box to 12px; el-checkbox-button
		// fills with the brand color when checked.
		k.ControlInteractiveSizeSM = 12
		k.ButtonColor = t.TextSecondary
		k.ButtonCheckedBg, k.ButtonCheckedColor = t.Primary.Base, t.TextOnPrimary
	case StyleShadcn:
		k.Bg = t.ControlBg
		k.BorderedPaddingStart, k.BorderedPaddingEnd = 12, 12
		k.ButtonCheckedBg, k.ButtonCheckedColor, k.ButtonCheckedBorder = t.ItemSelectedBg, t.ItemSelected, t.Border
	case Ant:
		k.CheckedHoverBg = t.Primary.Hover
	}
	return k
}
