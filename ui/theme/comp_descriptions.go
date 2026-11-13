package theme

// DescriptionsToken styles Descriptions.
type DescriptionsToken struct {
	Alias
	LabelBg              Color // label cells of a bordered list
	LabelColor           Color `contrast:"text:LabelBg"`
	ContentColor         Color `contrast:"text:@Page"`
	TitleColor           Color `contrast:"text:@Page"`
	TitleFontSize        float32
	TitleMarginBottom    float32
	BorderColor          Color   // bordered outline and cell rules
	Radius               float32 // bordered outline (zero: square)
	ItemPaddingBottom    float32 // borderless rows
	LabelMarginInlineEnd float32 // borderless label to content (Element)
	CellPaddingBlock     float32 // bordered cells, Large (Ant "default")
	CellPaddingInline    float32
	CellPaddingBlockMD   float32 // Middle
	CellPaddingInlineMD  float32
	CellPaddingBlockSM   float32 // Small
	CellPaddingInlineSM  float32
}

func deriveDescriptions(t *Theme) DescriptionsToken {
	k := DescriptionsToken{
		LabelBg: t.FillAlter, LabelColor: t.TextTertiary, ContentColor: t.Text, TitleColor: t.Text,
		TitleFontSize: t.FontSizeLG, TitleMarginBottom: 20, BorderColor: t.BorderSecondary, Radius: t.RadiusLG,
		ItemPaddingBottom: 16,
		CellPaddingBlock:  16, CellPaddingInline: 24, CellPaddingBlockMD: 12, CellPaddingInlineMD: 24,
		CellPaddingBlockSM: 8, CellPaddingInlineSM: 16,
	}
	if t.Flavor == Element {
		k.LabelBg, k.LabelColor, k.Radius = t.FillTertiary, t.TextSecondary, 0
		k.ItemPaddingBottom, k.LabelMarginInlineEnd = 12, 16
		k.CellPaddingBlock, k.CellPaddingInline = 12, 15
		k.CellPaddingBlockMD, k.CellPaddingInlineMD = 8, 11
		k.CellPaddingBlockSM, k.CellPaddingInlineSM = 4, 7
	}
	return k
}
