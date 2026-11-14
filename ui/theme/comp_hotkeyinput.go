package theme

// HotkeyInputToken styles HotkeyInput, the shortcut recorder: an input
// frame (Input token colors) holding key caps.
type HotkeyInputToken struct {
	Alias
	MinWidth         float32
	PaddingInline    float32
	KeyGap           float32 // between key caps
	PlaceholderColor Color   `contrast:"text:@Page"`
	RecordingColor   Color   `contrast:"text:@Page"` // the prompt while recording
	ClearIconSize    float32
	ClearColor       Color   `contrast:"nontext:@Page"`
	MessageFontSize  float32 // conflict message under the field
	MessageColor     Color   `contrast:"text:@Page"`
}

func deriveHotkeyInput(t *Theme) HotkeyInputToken {
	k := HotkeyInputToken{
		MinWidth: 180, PaddingInline: t.ControlPaddingH - 4, KeyGap: 4,
		PlaceholderColor: t.TextQuaternary, RecordingColor: t.Primary.Base,
		ClearIconSize: 12, ClearColor: t.TextTertiary,
		MessageFontSize: t.FontSizeSM, MessageColor: t.Warning.Text,
	}
	if t.Flavor == StyleShadcn {
		k.PlaceholderColor, k.RecordingColor = t.TextTertiary, t.Text
	}
	return k
}
