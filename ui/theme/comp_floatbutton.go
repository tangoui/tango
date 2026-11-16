package theme

import "github.com/tangoui/tango/ui/paint"

// FloatButtonToken styles FloatButton, FloatButtonGroup and BackTop.
type FloatButtonToken struct {
	Alias
	Size                float32 // button width and height
	DescriptionHeight   float32 // height of a button with a description
	IconSize            float32
	DescriptionIconSize float32 // icon of a button with a description
	DescriptionFontSize float32
	Bg                  Color
	Color               Color `contrast:"text:Bg"` // icon and description
	HoverBg             Color // tint over the button under the pointer
	PrimaryBg           Color
	PrimaryHoverBg      Color
	PrimaryColor        Color   `contrast:"text:PrimaryBg"`
	Radius              float32 // square shape
	InnerRadius         float32 // hover tint of a member of a square group
	Shadow              []paint.Shadow
	GroupShadow         []paint.Shadow // square group card
	GroupSplitColor     Color          // lines between the members of a square group
	GroupGap            float32        // between the members of a round group
	GroupPadding        float32        // inside a square group card

	// BackTop defaults: antd FloatButton.BackTop visibilityHeight 400 at
	// insetInlineEnd 24 / insetBlockEnd 48 with VerticalAlignTop; el-backtop
	// visibility-height 200, right 40, bottom 40 with a caret.
	BackTopVisibilityHeight float32
	BackTopRight            float32
	BackTopBottom           float32
	BackTopCaret            bool // caret-up icon instead of the bar-and-arrow
}

func deriveFloatButton(t *Theme) FloatButtonToken {
	k := FloatButtonToken{
		Size: 40, DescriptionHeight: 52, IconSize: 18, DescriptionIconSize: 18, DescriptionFontSize: t.FontSizeSM,
		Bg: t.BgElevated, Color: t.Text, HoverBg: t.FillTertiary,
		PrimaryBg: t.Primary.Base, PrimaryHoverBg: t.Primary.Hover, PrimaryColor: t.TextOnPrimary,
		Radius: t.RadiusLG, InnerRadius: t.RadiusSM,
		Shadow: t.ShadowSecondary, GroupShadow: t.ShadowSecondary, GroupSplitColor: t.Split,
		GroupGap: 16, GroupPadding: 4,
		BackTopVisibilityHeight: 400, BackTopRight: 24, BackTopBottom: 48,
	}
	if t.Flavor == Element {
		// el-backtop: brand-colored icon, 20px, a light 6px shadow.
		k.Color, k.IconSize = t.Primary.Base, 20
		k.Shadow = []paint.Shadow{{Blur: 6, Color: RGBA(0, 0, 0, .12)}}
		k.BackTopVisibilityHeight, k.BackTopRight, k.BackTopBottom, k.BackTopCaret = 200, 40, 40, true
	}
	return k
}
