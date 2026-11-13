package theme

// ImageToken styles Image and its preview.
type ImageToken struct {
	Alias
	PlaceholderBg                 Color // loading / failed box
	PlaceholderColor              Color // its icon (Element: text)
	PreviewMaskBg                 Color // hover mask of a previewable image
	PreviewMaskColor              Color // "preview" label on the mask
	PreviewBg                     Color // preview backdrop
	PreviewOperationSize          float32
	PreviewOperationIconSize      float32
	PreviewOperationBg            Color // round close / prev / next buttons
	PreviewOperationHoverBg       Color // layered on hover (zero: none)
	PreviewOperationColor         Color
	PreviewOperationHoverColor    Color
	PreviewOperationDisabledColor Color
	PreviewToolbarBg              Color
	PreviewSplitColor             Color // Element toolbar separators
}

func deriveImage(t *Theme) ImageToken {
	white := Hex("#ffffff")
	k := ImageToken{
		PlaceholderBg: t.FillTertiary, PlaceholderColor: t.TextQuaternary,
		PreviewMaskBg: RGBA(0, 0, 0, .5), PreviewMaskColor: white, PreviewBg: t.BgMask,
		PreviewOperationSize: 40, PreviewOperationIconSize: 18,
		PreviewOperationBg: RGBA(0, 0, 0, .1), PreviewOperationHoverBg: RGBA(0, 0, 0, .2),
		PreviewOperationColor: Fade(white, .85), PreviewOperationHoverColor: white, PreviewOperationDisabledColor: Fade(white, .25),
		PreviewToolbarBg: RGBA(0, 0, 0, .1), PreviewSplitColor: RGBA(255, 255, 255, .35),
	}
	if t.Flavor == Element {
		k.PreviewOperationSize, k.PreviewOperationIconSize = 44, 24
		k.PreviewOperationBg, k.PreviewOperationHoverBg, k.PreviewToolbarBg = Hex("#606266"), Color{}, Hex("#606266")
	}
	return k
}
