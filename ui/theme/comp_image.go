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
	PreviewOperationColor         Color // toolbar actions
	PreviewOperationHoverColor    Color
	PreviewOperationDisabledColor Color
	PreviewToolbarBg              Color
	PreviewSplitColor             Color // Element toolbar separators

	// Round close / prev / next buttons: icon colors and distance from the
	// window edges.
	PreviewSwitchColor      Color
	PreviewSwitchHoverColor Color
	PreviewCloseOffset      float32 // close button from the top and inline-end edges
	PreviewSwitchOffset     float32 // prev / next from the inline edges

	// Toolbar (Ant layout): action box size, gap between actions, inline
	// padding of the bar.
	PreviewActionSize     float32
	PreviewActionGap      float32
	PreviewActionsPadding float32

	// Group counter ("2 / 5"): distance from the bottom edge and color.
	PreviewProgressBottom float32
	PreviewProgressColor  Color

	// Behaviour defaults of the preview (ImageW methods override them):
	// zoom bounds and step, backdrop click closes, group wraps around,
	// group counter shown.
	PreviewMinScale     float32
	PreviewMaxScale     float32
	PreviewScaleStep    float32
	PreviewMaskClosable bool
	PreviewInfinite     bool
	PreviewShowProgress bool
	// PreviewDblClickZoom makes a double click zoom one step or reset
	// (rc-image); Element's viewer has no double-click action.
	PreviewDblClickZoom bool
}

func deriveImage(t *Theme) ImageToken {
	white := Hex("#ffffff")
	// antd 6.7 es/image/style/index.js: prepareComponentToken
	// (previewOperationColor / HoverColor / ColorDisabled =
	// colorTextLightSolid at 0.65 / 0.85 / 0.25, previewOperationSize =
	// fontSizeIcon*1.5 = 18); close and switch buttons are #fff on
	// colorBgMask at 0.1 (hover 0.2), padding paddingSM 12 (18 + 24 = 42),
	// marginSM 12 from the edges; the footer sits marginXL 32 from the
	// bottom, its actions are 18 + 2*paddingSM with gap paddingSM and
	// padding 0 paddingLG; progress over the actions with gap margin 16.
	// rc-image 1.10 Preview: scaleStep 0.5, minScale 1, maxScale 50,
	// maskClosable true; PrevNext stops at the ends.
	k := ImageToken{
		PlaceholderBg: t.FillTertiary, PlaceholderColor: t.TextQuaternary,
		PreviewMaskBg: RGBA(0, 0, 0, .5), PreviewMaskColor: white, PreviewBg: t.BgMask,
		PreviewOperationSize: 42, PreviewOperationIconSize: 18,
		PreviewOperationBg: RGBA(0, 0, 0, .1), PreviewOperationHoverBg: RGBA(0, 0, 0, .2),
		PreviewOperationColor: Fade(white, .65), PreviewOperationHoverColor: Fade(white, .85), PreviewOperationDisabledColor: Fade(white, .25),
		PreviewToolbarBg: RGBA(0, 0, 0, .1), PreviewSplitColor: RGBA(255, 255, 255, .35),
		PreviewSwitchColor: white, PreviewSwitchHoverColor: white, PreviewCloseOffset: 12, PreviewSwitchOffset: 12,
		PreviewActionSize: 42, PreviewActionGap: 12, PreviewActionsPadding: 24,
		PreviewProgressBottom: 32 + 42 + 16, PreviewProgressColor: Fade(white, .65),
		PreviewMinScale: 1, PreviewMaxScale: 50, PreviewScaleStep: .5, PreviewMaskClosable: true, PreviewShowProgress: true, PreviewDblClickZoom: true,
	}
	// shadcn/ui v4 has no image viewer: shadcn follows the Ant preview.
	if t.Flavor == Element {
		// theme-chalk image-viewer.scss (close 40px from the corner, arrows
		// 40px from the sides, progress 90px from the bottom, #fff) and
		// image-viewer props (zoom-rate 1.2, min-scale 0.2, max-scale 7,
		// infinite, hide-on-click-modal false, show-progress false).
		k.PreviewOperationSize, k.PreviewOperationIconSize = 44, 24
		k.PreviewOperationBg, k.PreviewOperationHoverBg, k.PreviewToolbarBg = Hex("#606266"), Color{}, Hex("#606266")
		k.PreviewOperationColor, k.PreviewOperationHoverColor = Fade(white, .85), white
		k.PreviewSwitchColor, k.PreviewSwitchHoverColor = Fade(white, .85), white
		k.PreviewCloseOffset, k.PreviewSwitchOffset = 40, 40
		k.PreviewProgressBottom, k.PreviewProgressColor = 90, white
		k.PreviewMinScale, k.PreviewMaxScale, k.PreviewScaleStep = .2, 7, .2
		k.PreviewMaskClosable, k.PreviewInfinite, k.PreviewShowProgress, k.PreviewDblClickZoom = false, true, false, false
	}
	return k
}
