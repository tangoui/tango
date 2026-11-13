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
}

func deriveCheckbox(t *Theme) CheckboxToken {
	k := CheckboxToken{
		ControlInteractiveSize: t.CheckboxSize, Radius: t.CheckboxRadius,
		Bg: t.BgContainer, BorderColor: t.Border,
		CheckedBg: t.Primary.Base, CheckedHoverBg: t.Primary.Base, CheckColor: t.TextOnPrimary,
		LabelFontWeight: t.LabelFontWeight,
		Shadow:          t.InputShadow,
	}
	switch t.Flavor {
	case StyleShadcn:
		k.Bg = t.ControlBg
	case Ant:
		k.CheckedHoverBg = t.Primary.Hover
	}
	return k
}
