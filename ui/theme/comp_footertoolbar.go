package theme

// FooterToolbarToken styles the Pro FooterToolbar (ui.FooterToolbar): the
// bar's surface. Pinned to the bottom (Sticky) over content that scrolls
// under it, the bar is frosted — a backdrop blur (paint.BackdropBlur) of
// BlurRadius under the translucent FrostedBg, like Ant Design Pro's
// FooterToolbar and shadcn/ui's sticky bars (bg-background/60 with
// backdrop-blur). In flow, in high-contrast themes and on renderers
// without backdrop blur it draws Bg (opaque in high contrast).
type FooterToolbarToken struct {
	Alias
	Bg         Color   // background in flow (slightly translucent)
	FrostedBg  Color   // background over the blurred page while pinned
	BlurRadius float32 // backdrop blur under FrostedBg (logical px; ZeroPx: never frosted)
}

func deriveFooterToolbar(t *Theme) FooterToolbarToken {
	// Ant Design Pro: rgba(255,255,255,0.6) with backdrop-filter
	// blur(8px); in flow the 96% container color.
	k := FooterToolbarToken{
		Bg:         Fade(t.BgContainer, 0.96),
		FrostedBg:  Fade(t.BgContainer, 0.6),
		BlurRadius: 8,
	}
	switch t.Flavor {
	case Element:
		// Element has no FooterToolbar; its flat surfaces keep more of
		// the container color over a slightly stronger blur.
		k.FrostedBg, k.BlurRadius = Fade(t.BgContainer, 0.72), 10
	case StyleShadcn:
		// bg-background/95, and supports-[backdrop-filter]:bg-background/60
		// backdrop-blur (blur(8px)) where the browser can blur.
		k.Bg = Fade(t.BgContainer, 0.95)
	}
	return k
}
