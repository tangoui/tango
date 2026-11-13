package theme

// UploadToken styles Upload: the file list, picture cards and the dragger.
type UploadToken struct {
	Alias
	TipColor Color `contrast:"text:@Page"`

	// Text list.
	ItemHeight      float32 // row height (the progress bar adds below it)
	ItemRadius      float32
	ItemColor       Color `contrast:"text:@Page"`
	ItemIconColor   Color
	ItemHoverBg     Color
	ItemHoverColor  Color // zero: unchanged
	ItemBorderColor Color // zero: none
	ActionsColor    Color `contrast:"nontext:@Page"` // remove button
	ErrorColor      Color `contrast:"text:@Page"`    // failed file, its retry button
	ErrorHoverColor Color
	SuccessColor    Color // done mark / ribbon (zero: none)

	ProgressColor      Color // filled part of the upload progress bar
	ProgressTrailColor Color

	// Picture cards.
	PictureCardSize    float32
	CardRadius         float32 // picture cards, card trigger and dragger
	PictureCardBg      Color
	PictureCardBorder  Color
	PictureCardInnerBg Color // behind a missing thumbnail
	MaskBg             Color // hover mask over a card
	MaskColor          Color `contrast:"nontext:MaskBg"` // its action icons

	// Card trigger and dragger (dashed outlines).
	TriggerBg               Color // zero: none
	TriggerHoverBg          Color // zero: unchanged
	TriggerBorderColor      Color
	TriggerHoverBorderColor Color
	TriggerColor            Color `contrast:"text:@Page"`
	DraggerHeight           float32
	DraggerBg               Color // zero: none
	DraggerHoverBg          Color // zero: unchanged
	DraggerBorderColor      Color
	DraggerHoverBorderColor Color
	DraggerDragBorderColor  Color // while files are dragged over
	DraggerIconColor        Color
	DraggerTitleColor       Color `contrast:"text:@Page"`
	DraggerHintColor        Color
	DisabledBg              Color // zero: none
}

func deriveUpload(t *Theme) UploadToken {
	k := UploadToken{
		TipColor:   t.TextTertiary,
		ItemHeight: 22, ItemRadius: t.RadiusSM,
		ItemColor: t.Text, ItemIconColor: t.TextTertiary, ItemHoverBg: t.ItemHoverBg,
		ActionsColor: t.TextTertiary, ErrorColor: t.Error.Base, ErrorHoverColor: t.Error.Hover,
		ProgressColor: t.Primary.Base, ProgressTrailColor: t.FillSecondary,
		PictureCardSize: 102, CardRadius: t.RadiusLG,
		PictureCardBg: t.BgContainer, PictureCardBorder: t.Border, PictureCardInnerBg: t.FillQuaternary,
		MaskBg: RGBA(0, 0, 0, .45), MaskColor: RGBA(255, 255, 255, .85),
		TriggerBg: t.FillQuaternary, TriggerBorderColor: t.Border, TriggerHoverBorderColor: t.Primary.Base,
		TriggerColor:  t.Text,
		DraggerHeight: 152,
		DraggerBg:     t.FillQuaternary, DraggerBorderColor: t.Border,
		DraggerHoverBorderColor: t.Primary.Hover, DraggerDragBorderColor: t.Primary.Hover,
		DraggerIconColor: t.Primary.Base, DraggerTitleColor: t.Text, DraggerHintColor: t.TextTertiary,
		DisabledBg: t.BgContainerDisabled,
	}
	switch t.Flavor {
	case Element:
		// el-upload: secondary text, brand hover, a success mark / corner
		// ribbon, 148px cards with 6px corners.
		k.TipColor = t.TextSecondary
		k.ItemHeight = 26
		k.ItemColor, k.ItemIconColor, k.ItemHoverColor = t.TextSecondary, t.TextSecondary, t.Primary.Base
		k.ActionsColor = t.TextSecondary
		k.SuccessColor = t.Success.Base
		k.PictureCardSize, k.CardRadius = 148, 6
		k.MaskBg, k.MaskColor = RGBA(0, 0, 0, .5), Hex("#ffffff")
		k.TriggerBorderColor = t.BorderHover
		k.TriggerColor = t.TextTertiary
		k.DraggerHeight = 180
		k.DraggerBg = t.BgContainer
		k.DraggerHoverBorderColor, k.DraggerDragBorderColor = t.Primary.Base, t.Primary.Base
		k.DraggerIconColor, k.DraggerTitleColor = t.TextQuaternary, t.TextSecondary
	case StyleShadcn:
		// No shadcn Upload: bordered rounded-md rows, ghost actions,
		// dashed drop areas that turn bg-muted/50 under the pointer.
		k.ItemHeight, k.ItemRadius = 40, t.Radius
		k.ItemHoverBg, k.ItemBorderColor = t.FillAlter, t.BorderSecondary
		k.TriggerBg, k.TriggerHoverBg = Color{}, t.FillAlter
		k.TriggerHoverBorderColor, k.TriggerColor = t.BorderHover, t.TextTertiary
		k.DraggerBg, k.DraggerHoverBg = Color{}, t.FillAlter
		k.DraggerHoverBorderColor, k.DraggerDragBorderColor = t.BorderHover, t.FocusOutline
		k.DraggerIconColor = t.TextTertiary
		k.DisabledBg = Color{}
	}
	return k
}
