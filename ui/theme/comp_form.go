package theme

// FormToken styles Form items: labels, required marks and the messages
// under the controls.
type FormToken struct {
	Alias
	LabelColor                Color `contrast:"text:@Page"`
	LabelErrorColor           Color // label of an item with an error (zero: unchanged)
	LabelFontSize             float32
	LabelFontWeight           int     // zero: regular
	LabelHeight               float32 // horizontal / inline label row
	LabelColonMarginInlineEnd float32 // between an end-aligned label and the control
	LabelRequiredMarkColor    Color   `contrast:"text:@Page"`
	LabelTooltipColor         Color   // the label's help icon
	LabelOptionalColor        Color   `contrast:"text:@Page"` // Form.RequiredMarkOptional suffix
	VerticalLabelHeight       float32 // label line of vertical forms
	VerticalLabelPadding      float32 // under a vertical label
	ItemMarginBottom          float32 // between items without a message
	InlineItemMarginBottom    float32
	ErrorColor                Color `contrast:"text:@Page"`
	WarningColor              Color
	HelpColor                 Color   `contrast:"text:@Page"` // help text and Extra
	MessageFontSize           float32 // help, error and Extra lines (zero: the text default)
	MessageLineHeight         float32 // zero: the text default
	ExtraMarginTop            float32
	// Pro v3 form/layouts/LoginForm/style.js (2026-11-15).
	LoginLogoGap                 float32
	LoginTitleFontWeight         int
	LoginDescriptionMarginBottom float32
}

func deriveForm(t *Theme) FormToken {
	k := FormToken{
		LabelColor: t.Text, LabelFontSize: t.FontSize, LabelHeight: t.ControlHeight,
		LabelColonMarginInlineEnd: 8,
		LabelRequiredMarkColor:    t.Error.Base, LabelTooltipColor: t.TextTertiary,
		// antd .ant-form-item-optional: colorTextDescription.
		LabelOptionalColor:  t.TextTertiary,
		VerticalLabelHeight: 22, VerticalLabelPadding: 8,
		ItemMarginBottom: 24, InlineItemMarginBottom: 24,
		ErrorColor: t.Error.Base, WarningColor: t.Warning.Base, HelpColor: t.TextTertiary,
		ExtraMarginTop: 2,
		LoginLogoGap:   16, LoginTitleFontWeight: 600, LoginDescriptionMarginBottom: 40,
	}
	switch t.Flavor {
	case Element:
		k.LabelColor = t.TextSecondary
		k.LabelColonMarginInlineEnd = 12
		k.LabelOptionalColor = t.TextSecondary
		k.ItemMarginBottom = 18
		k.MessageFontSize, k.MessageLineHeight = 12, 18
	case StyleShadcn:
		// <Label>: text-sm font-medium leading-none, gap-2 under it; the
		// label turns destructive on error.
		k.LabelFontWeight = 500
		k.LabelErrorColor = t.Error.Base
		k.VerticalLabelHeight, k.VerticalLabelPadding = 14, 8
		k.ExtraMarginTop = 8
	}
	return k
}
