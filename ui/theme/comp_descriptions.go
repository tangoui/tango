package theme

// DescriptionsToken styles Descriptions.
//
// Size buckets follow Tango's ControlSize: Large is antd "default" and
// Element "large", Middle is antd "middle" and Element "default", Small
// is "small" in both.
type DescriptionsToken struct {
	Alias
	LabelBg              Color // label cells of a bordered list
	LabelColor           Color `contrast:"text:@Page"`   // borderless label
	ContentColor         Color `contrast:"text:@Page"`   // borderless content
	BorderedLabelColor   Color `contrast:"text:LabelBg"` // bordered label cell
	BorderedContentColor Color `contrast:"text:@Page"`   // bordered content cell
	TitleColor           Color `contrast:"text:@Page"`
	TitleFontSize        float32
	TitleFontSizeSM      float32
	TitleMarginBottom    float32 // Large
	TitleMarginBottomMD  float32
	TitleMarginBottomSM  float32
	BorderColor          Color   // bordered outline and cell rules
	Radius               float32 // bordered outline (zero: square)
	ItemPaddingBottom    float32 // borderless rows, Large (the last row has none)
	ItemPaddingBottomMD  float32
	ItemPaddingBottomSM  float32
	ItemPaddingEnd       float32 // borderless cells except the last of a row
	// VerticalLabelPaddingBottom* pad the label row of a borderless
	// vertical list (antd: the item padding; Element 8 / 6 / 4).
	VerticalLabelPaddingBottom   float32
	VerticalLabelPaddingBottomMD float32
	VerticalLabelPaddingBottomSM float32
	ShowColon                    bool    // borderless labels end with the locale colon (antd colon; Element has none)
	ColonMarginInlineStart       float32 // before the colon (antd colonMarginLeft)
	LabelMarginInlineEnd         float32 // borderless label (and colon) to content
	LabelMarginInlineEndSM       float32
	CellPaddingBlock             float32 // bordered cells, Large (Ant "default")
	CellPaddingInline            float32
	CellPaddingBlockMD           float32 // Middle
	CellPaddingInlineMD          float32
	CellPaddingBlockSM           float32 // Small
	CellPaddingInlineSM          float32
	// DefaultMiddle makes an unsized list use the Middle bucket (Element's
	// default size); otherwise it uses Large (antd's "default").
	DefaultMiddle bool
}

func deriveDescriptions(t *Theme) DescriptionsToken {
	// antd es/descriptions/style/index.js: prepareComponentToken and the
	// bordered / size styles.
	k := DescriptionsToken{
		LabelBg: t.FillAlter, LabelColor: t.TextTertiary, ContentColor: t.Text,
		BorderedLabelColor: t.TextSecondary, BorderedContentColor: t.Text, TitleColor: t.Text,
		TitleFontSize: t.FontSizeLG, TitleFontSizeSM: t.FontSizeLG,
		TitleMarginBottom: 20, TitleMarginBottomMD: 20, TitleMarginBottomSM: 20, // fontSizeSM * lineHeightSM
		BorderColor: t.Split, Radius: t.RadiusLG,
		ItemPaddingBottom: 16, ItemPaddingBottomMD: 12, ItemPaddingBottomSM: 8, ItemPaddingEnd: 16,
		VerticalLabelPaddingBottom: 16, VerticalLabelPaddingBottomMD: 12, VerticalLabelPaddingBottomSM: 8,
		ShowColon: true, ColonMarginInlineStart: 2, LabelMarginInlineEnd: 8, LabelMarginInlineEndSM: 8,
		CellPaddingBlock: 16, CellPaddingInline: 24, CellPaddingBlockMD: 12, CellPaddingInlineMD: 24,
		CellPaddingBlockSM: 8, CellPaddingInlineSM: 16,
	}
	if t.Flavor == Element {
		// theme-chalk descriptions.scss / descriptions-item.scss.
		k.LabelBg, k.Radius, k.BorderColor = t.FillTertiary, 0, t.BorderSecondary
		k.LabelColor, k.ContentColor = t.Text, t.TextSecondary                 // primary label, regular content
		k.BorderedLabelColor, k.BorderedContentColor = t.TextSecondary, t.Text // is-bordered-label regular, content primary
		k.TitleFontSizeSM = 14
		k.TitleMarginBottom, k.TitleMarginBottomMD, k.TitleMarginBottomSM = 20, 16, 12
		k.ItemPaddingEnd = 0
		k.VerticalLabelPaddingBottom, k.VerticalLabelPaddingBottomMD, k.VerticalLabelPaddingBottomSM = 8, 6, 4
		k.ShowColon, k.ColonMarginInlineStart = false, 0
		k.LabelMarginInlineEnd, k.LabelMarginInlineEndSM = 16, 12
		k.CellPaddingBlock, k.CellPaddingInline = 12, 15
		k.CellPaddingBlockMD, k.CellPaddingInlineMD = 8, 11
		k.CellPaddingBlockSM, k.CellPaddingInlineSM = 4, 7
		k.DefaultMiddle = true
	}
	return k
}
