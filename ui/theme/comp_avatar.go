package theme

// AvatarToken styles Avatar.
type AvatarToken struct {
	Alias
	ContainerSize   float32
	ContainerSizeSM float32
	ContainerSizeLG float32
	TextFontSize    float32
	TextFontSizeSM  float32
	TextFontSizeLG  float32
	IconFontSize    float32 // icon avatar glyph size
	IconFontSizeSM  float32
	IconFontSizeLG  float32
	Radius          float32 // square avatar corner radius
	RadiusSM        float32
	RadiusLG        float32
	// TextAutoScale shrinks text wider than the avatar minus the gap
	// (antd gap); el-avatar does not scale its text.
	TextAutoScale    bool
	GroupSpace       float32
	GroupOverlapping float32
	GroupBorderColor Color
	Bg               Color
	Color            Color `contrast:"text:Bg"`
}

func deriveAvatar(t *Theme) AvatarToken {
	// antd 6.7 es/avatar/style/index.js prepareComponentToken:
	// containerSize controlHeight 32 / LG 40 / SM 24; textFontSize
	// fontSize at every size (the text is scaled down to fit, gap 4);
	// iconFontSize round((fontSizeLG + fontSizeXL) / 2) = 18, LG
	// fontSizeHeading3 = 24, SM fontSize = 14; square radius borderRadius /
	// borderRadiusLG / borderRadiusSM.
	k := AvatarToken{
		ContainerSize: 32, ContainerSizeSM: 24, ContainerSizeLG: 40,
		TextFontSize: t.FontSize, TextFontSizeSM: t.FontSize, TextFontSizeLG: t.FontSize,
		IconFontSize: 18, IconFontSizeSM: t.FontSize, IconFontSizeLG: 24,
		Radius: t.Radius, RadiusSM: t.RadiusSM, RadiusLG: t.RadiusLG, TextAutoScale: true,
		GroupSpace: 4, GroupOverlapping: -8, GroupBorderColor: t.BgContainer,
		Bg: t.AvatarBg, Color: t.AvatarText,
	}
	switch t.Flavor {
	case Element:
		// el-avatar (theme-chalk var.scss $avatar-size, $avatar): 24 / 40 /
		// 56 px, text-size 14, icon-size 18 and border-radius-base at every
		// size.
		k.ContainerSize, k.ContainerSizeLG = 40, 56
		k.TextFontSize, k.TextFontSizeSM, k.TextFontSizeLG = 14, 14, 14
		k.IconFontSize, k.IconFontSizeSM, k.IconFontSizeLG = 18, 18, 18
		k.RadiusSM, k.RadiusLG = t.Radius, t.Radius
		k.TextAutoScale = false
	case StyleShadcn:
		// new-york-v4 avatar.tsx: size-8 (sm size-6, lg size-10);
		// AvatarFallback bg-muted text-sm text-muted-foreground (sm
		// text-xs).
		k.TextFontSizeSM = t.FontSizeSM
		k.RadiusSM, k.RadiusLG = t.Radius, t.Radius
		if t.Shadcn != nil {
			k.Color = t.Shadcn.MutedForeground
		}
	}
	return k
}
