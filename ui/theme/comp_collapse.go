package theme

// CollapseToken styles Collapse.
type CollapseToken struct {
	Alias
	HeaderBg                       Color // header band (Ant: the whole group; zero: none)
	HeaderColor                    Color `contrast:"text:HeaderBg"`
	ArrowColor                     Color `contrast:"nontext:HeaderBg"`
	ArrowSize                      float32
	HeaderFontSize                 float32
	HeaderFontSizeLG               float32
	HeaderFontWeight               int     // zero: regular
	HeaderHeight                   float32 // minimum header height and label line height (zero: none)
	HeaderPaddingBlock             float32
	HeaderPaddingInline            float32
	HeaderPaddingBlockSM           float32
	HeaderPaddingInlineSM          float32
	HeaderPaddingBlockLG           float32
	HeaderPaddingInlineLG          float32
	HeaderGap                      float32 // between arrow, label and extra
	ContentBg                      Color   // zero: none
	ContentPaddingBlock            float32 // Element and shadcn: below the content only
	ContentPaddingInline           float32
	ContentPaddingBlockSM          float32
	ContentPaddingInlineSM         float32
	ContentPaddingBlockLG          float32
	ContentPaddingInlineLG         float32
	BorderlessContentPadding       float32 // above the content of a borderless / ghost group
	BorderlessContentPaddingBottom float32
	BorderColor                    Color // frame and dividers
	Radius                         float32
}

func deriveCollapse(t *Theme) CollapseToken {
	k := CollapseToken{
		HeaderBg: t.FillAlter, HeaderColor: t.Text, ArrowColor: t.Text, ArrowSize: 12,
		HeaderFontSize: t.FontSize, HeaderFontSizeLG: t.FontSizeLG,
		HeaderPaddingBlock: 12, HeaderPaddingInline: 16, HeaderPaddingBlockSM: 8, HeaderPaddingInlineSM: 12,
		HeaderPaddingBlockLG: 16, HeaderPaddingInlineLG: 24, HeaderGap: 12,
		ContentBg: t.BgContainer, ContentPaddingBlock: 16, ContentPaddingInline: 16,
		ContentPaddingBlockSM: 8, ContentPaddingInlineSM: 8, ContentPaddingBlockLG: 16, ContentPaddingInlineLG: 24,
		BorderlessContentPadding: 4, BorderlessContentPaddingBottom: 16,
		BorderColor: t.Border, Radius: t.RadiusLG,
	}
	switch t.Flavor {
	case Element:
		// el-collapse: 48px white headers, 13px medium titles, a light
		// divider, 25px below the content; one size.
		k.HeaderBg, k.ArrowColor = t.BgContainer, t.TextSecondary
		k.HeaderFontSize, k.HeaderFontSizeLG, k.HeaderFontWeight, k.HeaderHeight = 13, 13, 500, 48
		k.HeaderPaddingBlock, k.HeaderPaddingInline = 0, 0
		k.HeaderPaddingBlockSM, k.HeaderPaddingInlineSM, k.HeaderPaddingBlockLG, k.HeaderPaddingInlineLG = 0, 0, 0, 0
		k.HeaderGap, k.ContentPaddingBlock, k.ContentPaddingInline = 8, 25, 0
		k.BorderColor = t.BorderSecondary
	case StyleShadcn:
		// Accordion: no frame, items split by lines, py-4 medium triggers
		// with a muted 16px chevron, pb-4 content; one size.
		k.HeaderBg, k.ContentBg, k.ArrowColor, k.ArrowSize = Color{}, Color{}, t.TextTertiary, 16
		k.HeaderFontSizeLG, k.HeaderFontWeight = t.FontSize, 500
		k.HeaderPaddingBlock, k.HeaderPaddingInline = 16, 0
		k.HeaderPaddingBlockSM, k.HeaderPaddingInlineSM, k.HeaderPaddingBlockLG, k.HeaderPaddingInlineLG = 16, 0, 16, 0
		k.HeaderGap, k.ContentPaddingBlock, k.ContentPaddingInline = 16, 16, 0
		k.BorderColor = t.Split
	}
	return k
}
