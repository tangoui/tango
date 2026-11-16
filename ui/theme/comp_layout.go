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
	// SiderCollapsedWidth is the collapsed Sider width. Difference from
	// antd 6.7: Layout.Sider collapsedWidth defaults to 80
	// (antd/es/layout/Sider.js); Tango's Ant theme keeps 64 because
	// ProLayout collapses the same Sider to 64 (Pro v6). Recorded in
	// docs/design-baseline.md.
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
	// Pro v3 layout/ProLayout.js, TopNavHeader/style.js and provider
	// typing/layoutToken.js. Separate from the basic Layout regions.
	ProSiderWidth, ProMixSiderWidth                      float32
	ProHeaderTitleFontSize, ProLogoSize, ProLogoTitleGap float32
	ProTopNavPaddingInline, ProMenuPadding               float32
	ProFixedWidth                                        float32
	PagePaddingInline, PagePaddingBlock                  float32
	PageHeaderPaddingTop, PageHeaderPaddingBottom        float32
	PageBg, PageFixedBg                                  Color
	DefaultFooterLinkGap, DefaultFooterPaddingInline     float32
	DefaultFooterMarginTop, DefaultFooterMarginBottom    float32
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
		ProHeaderBg: Fade(t.BgElevated, .6), ProHeaderHeight: 56,
		ProSiderWidth: 256, ProMixSiderWidth: 215,
		ProHeaderTitleFontSize: 16, ProLogoSize: 32, ProLogoTitleGap: 6,
		ProTopNavPaddingInline: 16, ProMenuPadding: 6, ProFixedWidth: 1152,
		PagePaddingInline: 40, PagePaddingBlock: 32,
		PageHeaderPaddingTop: 8, PageHeaderPaddingBottom: 16,
		PageFixedBg:          t.BgElevated,
		DefaultFooterLinkGap: 8, DefaultFooterPaddingInline: 16,
		DefaultFooterMarginTop: 48, DefaultFooterMarginBottom: 24,
	}
	switch t.Flavor {
	case Element:
		// el-container regions have no backgrounds; header and footer are
		// 60px high with 20px padding.
		k.BodyBg, k.HeaderBg, k.HeaderColor, k.HeaderHeight, k.HeaderPaddingInline = Color{}, Color{}, t.Text, 60, t.PaddingMD
		k.FooterPadding, k.FooterPaddingInline = 0, t.PaddingMD
		k.ProSiderWidth, k.ProMixSiderWidth = t.SiderWidth, t.SiderWidth
		k.ProHeaderBg, k.ProHeaderHeight = t.HeaderBg, t.HeaderHeight
		k.PagePaddingInline, k.PagePaddingBlock = 20, 20
	case StyleShadcn:
		// Site header (h-14, bg-background, border-b) and footer.
		k.BodyBg, k.HeaderBg, k.HeaderColor, k.HeaderHeight, k.HeaderPaddingInline = t.BgContainer, t.HeaderBg, t.Text, t.HeaderHeight, t.Padding
		k.FooterPadding, k.FooterPaddingInline = t.Padding, t.PaddingLG
		k.ProSiderWidth, k.ProMixSiderWidth = t.SiderWidth, t.SiderWidth
		k.ProHeaderBg, k.ProHeaderHeight = t.HeaderBg, t.HeaderHeight
		k.PagePaddingInline, k.PagePaddingBlock = 24, 24
	default:
		// antd layout/style/index.js: triggerBg is the literal '#002140'
		// beside siderBg '#001529' in both algorithms. Tango's dark Ant
		// sider is not #001529 (SiderBg follows the dark palette), so the
		// dark trigger keeps deriving from SiderSubmenuBg to sit on it;
		// the light theme's dark sider is antd's and takes antd's value.
		if !t.Dark {
			k.TriggerBg = Hex("#002140")
		}
	}
	return k
}
