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
	// The same sizes in every design system: 24 / 32 / 40 px with 12 / 14 /
	// 18 px initials.
	return AvatarToken{
		ContainerSize: 32, ContainerSizeSM: 24, ContainerSizeLG: 40,
		TextFontSize: 14, TextFontSizeSM: 12, TextFontSizeLG: 18,
		GroupSpace: 4, GroupOverlapping: -8, GroupBorderColor: t.BgContainer,
		Bg: t.AvatarBg, Color: t.AvatarText,
	}
}
