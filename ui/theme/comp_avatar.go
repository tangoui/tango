package theme

// AvatarToken styles Avatar.
type AvatarToken struct {
	Alias
	ContainerSize    float32
	ContainerSizeSM  float32
	ContainerSizeLG  float32
	TextFontSize     float32
	TextFontSizeSM   float32
	TextFontSizeLG   float32
	GroupSpace       float32
	GroupOverlapping float32
	GroupBorderColor Color
	Bg               Color
	Color            Color `contrast:"text:Bg"`
}

func deriveAvatar(t *Theme) AvatarToken {
	// Ant and shadcn (size-8): 24 / 32 / 40 px with 12 / 14 / 18 px
	// initials.
	k := AvatarToken{
		ContainerSize: 32, ContainerSizeSM: 24, ContainerSizeLG: 40,
		TextFontSize: 14, TextFontSizeSM: 12, TextFontSizeLG: 18,
		GroupSpace: 4, GroupOverlapping: -8, GroupBorderColor: t.BgContainer,
		Bg: t.AvatarBg, Color: t.AvatarText,
	}
	if t.Flavor == Element {
		// el-avatar ($avatar-size, $avatar in theme-chalk var.scss):
		// 24 / 40 / 56 px, 14px text at every size.
		k.ContainerSize, k.ContainerSizeLG = 40, 56
		k.TextFontSizeSM, k.TextFontSizeLG = 14, 14
	}
	return k
}
