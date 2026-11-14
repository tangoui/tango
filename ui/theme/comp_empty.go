package theme

// EmptyPicture is the default picture of Empty (EmptyToken.Picture).
// Append-only: never reorder or insert.
type EmptyPicture int

const (
	// EmptyPictureAnt is Ant's open box (Simple: the tray).
	EmptyPictureAnt EmptyPicture = iota
	// EmptyPictureElement is Element Plus's illustration (Simple still
	// draws the Ant tray).
	EmptyPictureElement
)

// EmptyToken styles Empty. The default pictures take their colors from
// Theme.Illustration.
type EmptyToken struct {
	Alias
	ImageHeight          float32      // default picture (Element: its illustration)
	ImageHeightSM        float32      // Simple picture
	DescriptionColor     Color        `contrast:"text:@Page"`
	DescriptionColorSM   Color        `contrast:"text:@Page"` // Simple
	SimpleFill           Color        // Simple tray
	SimpleLine           Color        // Simple tray outline
	SimpleShadow         Color        // Simple ground ellipse
	Picture              EmptyPicture // default illustration when MediaBg is zero
	MediaBg              Color        // shadcn icon tile (zero: the picture is an illustration)
	MediaColor           Color        `contrast:"nontext:MediaBg"` // its icon
	MediaRadius          float32
	TitleFontSize        float32 // Title (shadcn EmptyTitle)
	TitleFontWeight      int     // zero: regular
	TitleColor           Color   `contrast:"text:@Page"`
	PaddingBlock         float32 // around the default (not Simple) empty state
	PaddingInline        float32
	DescriptionMarginTop float32 // picture to the title / description
	FooterMarginTop      float32 // description to the Body actions
}

func deriveEmpty(t *Theme) EmptyToken {
	// antd 6.7 es/empty/style/index.js: marginInline marginXS 8, image
	// marginBottom marginXS 8, footer marginTop margin 16, image height
	// controlHeightLG × 2.5 = 100 (simple: controlHeightLG = 40).
	k := EmptyToken{
		ImageHeight: 100, ImageHeightSM: 40, DescriptionColor: t.TextTertiary, DescriptionColorSM: t.TextQuaternary,
		SimpleFill: t.FillQuaternary, SimpleLine: t.Illustration.Line, SimpleShadow: t.FillTertiary,
		TitleFontSize: t.FontSizeLG, TitleFontWeight: t.FontWeightStrong, TitleColor: t.TextHeading,
		PaddingBlock: 0, PaddingInline: 8, DescriptionMarginTop: 8, FooterMarginTop: 16,
	}
	switch t.Flavor {
	case Element:
		// theme-chalk var.scss $empty: padding 40px 0, image-width 160,
		// description-margin-top 20, bottom-margin-top 20.
		k.ImageHeight = 160
		k.Picture = EmptyPictureElement
		k.PaddingBlock, k.PaddingInline, k.DescriptionMarginTop, k.FooterMarginTop = 40, 0, 20, 20
	case StyleShadcn:
		// new-york-v4 empty.tsx: Empty p-6 gap-6; EmptyMedia variant="icon"
		// is a size-10 rounded-lg bg-muted tile with a foreground icon
		// (mb-2) and the header gap-2; EmptyTitle text-lg font-medium.
		k.ImageHeight = 40
		k.MediaBg, k.MediaColor, k.MediaRadius = t.FillSecondary, t.Text, t.RadiusLG
		k.TitleFontSize, k.TitleFontWeight, k.TitleColor = 18, 500, t.Text
		k.PaddingBlock, k.PaddingInline, k.DescriptionMarginTop, k.FooterMarginTop = 24, 24, 16, 24
	}
	return k
}
