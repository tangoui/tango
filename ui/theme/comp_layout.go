package theme

// LayoutToken styles Layout and ProLayout chrome (header, sider, footer).
// The Header* and Footer* fields are the Layout regions; ProHeader* the
// ProLayout header bar.
type LayoutToken struct {
	Alias
	BodyBg              Color // zero: none
	HeaderBg            Color // zero: none
	HeaderColor         Color `contrast:"text:HeaderBg"`
	HeaderHeight        float32
	HeaderPaddingInline float32
	FooterBg            Color
	FooterPadding       float32 // block (vertical) padding
	FooterPaddingInline float32
	SiderBg             Color // dark sider
	LightSiderBg        Color
	SiderWidth          float32
	SiderCollapsedWidth float32
	TriggerBg           Color
	TriggerColor        Color `contrast:"text:TriggerBg"`
	TriggerHeight       float32
	LightTriggerBg      Color
	LightTriggerColor   Color `contrast:"text:LightTriggerBg"`
	ZeroTriggerWidth    float32
	ZeroTriggerHeight   float32
	ProHeaderBg         Color
	ProHeaderHeight     float32
}

func deriveLayout(t *Theme) LayoutToken {
	k := LayoutToken{
		// Ant Layout.Header: a 64px dark bar padded 50px.
		BodyBg: t.BgLayout, HeaderBg: t.SiderBg, HeaderColor: t.SiderTextActive, HeaderHeight: 64, HeaderPaddingInline: 50,
		FooterBg: t.BgLayout, FooterPadding: t.PaddingLG, FooterPaddingInline: 50,
		SiderBg: t.SiderBg, LightSiderBg: t.SiderLightBg, SiderWidth: t.SiderWidth, SiderCollapsedWidth: t.SiderCollapsed,
		TriggerBg: t.SiderSubmenuBg, TriggerColor: Hex("#ffffff"), TriggerHeight: 48,
		LightTriggerBg: t.SiderLightBg, LightTriggerColor: t.Text,
		ZeroTriggerWidth: 40, ZeroTriggerHeight: 40,
		ProHeaderBg: t.HeaderBg, ProHeaderHeight: t.HeaderHeight,
	}
	switch t.Flavor {
	case Element:
		// el-container regions have no backgrounds; header and footer are
		// 60px high with 20px padding.
		k.BodyBg, k.HeaderBg, k.HeaderColor, k.HeaderHeight, k.HeaderPaddingInline = Color{}, Color{}, t.Text, 60, t.PaddingMD
		k.FooterPadding, k.FooterPaddingInline = 0, t.PaddingMD
	case StyleShadcn:
		// Site header (h-14, bg-background, border-b) and footer.
		k.BodyBg, k.HeaderBg, k.HeaderColor, k.HeaderHeight, k.HeaderPaddingInline = t.BgContainer, t.HeaderBg, t.Text, t.HeaderHeight, t.Padding
		k.FooterPadding, k.FooterPaddingInline = t.Padding, t.PaddingLG
	}
	return k
}
