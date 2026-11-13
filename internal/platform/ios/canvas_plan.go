package ios

// Text-input traits of the canvas view (docs/specs/mobile-canvas.md §2.3,
// M3b): input.TextHints to the UITextInputTraits values canvas_text.m
// applies. The UIKit enums are plain integers here so the tables are
// tested on any host.

import "github.com/tangoui/tango/ui/input"

// UIKeyboardType values.
const (
	uiKeyboardDefault      = 0
	uiKeyboardURL          = 3
	uiKeyboardNumberPad    = 4
	uiKeyboardPhonePad     = 5
	uiKeyboardEmailAddress = 7
	uiKeyboardDecimalPad   = 8
	uiKeyboardWebSearch    = 10
)

// UIReturnKeyType values.
const (
	uiReturnDefault = 0
	uiReturnGo      = 1
	uiReturnNext    = 4
	uiReturnSearch  = 6
	uiReturnSend    = 7
	uiReturnDone    = 9
)

// UITextAutocorrectionType and UITextSpellCheckingType share their values.
const (
	uiCorrectDefault = 0
	uiCorrectNo      = 1
)

// UITextAutocapitalizationType values.
const (
	uiCapNone      = 0
	uiCapWords     = 1
	uiCapSentences = 2
)

// textTraits are the UITextInputTraits of the focused field.
type textTraits struct {
	Keyboard   int // UIKeyboardType
	Return     int // UIReturnKeyType
	Secure     bool
	Correct    int // UITextAutocorrectionType
	SpellCheck int // UITextSpellCheckingType
	Capitalize int // UITextAutocapitalizationType
}

var kindKeyboard = map[input.TextKind]int{
	input.TextPlain:    uiKeyboardDefault,
	input.TextNumber:   uiKeyboardNumberPad,
	input.TextDecimal:  uiKeyboardDecimalPad,
	input.TextPhone:    uiKeyboardPhonePad,
	input.TextEmail:    uiKeyboardEmailAddress,
	input.TextURL:      uiKeyboardURL,
	input.TextPassword: uiKeyboardDefault,
	input.TextSearch:   uiKeyboardWebSearch,
}

var enterReturn = map[input.EnterKey]int{
	input.EnterDefault: uiReturnDefault,
	input.EnterDone:    uiReturnDone,
	input.EnterGo:      uiReturnGo,
	input.EnterNext:    uiReturnNext,
	input.EnterSearch:  uiReturnSearch,
	input.EnterSend:    uiReturnSend,
	input.EnterNewline: uiReturnDefault,
}

var capitalizeCap = map[input.Capitalize]int{
	input.CapitalizeNone:      uiCapNone,
	input.CapitalizeWords:     uiCapWords,
	input.CapitalizeSentences: uiCapSentences,
}

// traitsFor maps the hints of a field. Unknown kinds and keys fall back
// to the defaults. Fields whose text is not prose (passwords, numbers,
// addresses) get neither corrections nor capitals; a search field
// without an explicit return key gets the Search key.
func traitsFor(h input.TextHints) textTraits {
	t := textTraits{
		Keyboard:   kindKeyboard[h.Kind],
		Return:     enterReturn[h.Enter],
		Secure:     h.Kind == input.TextPassword,
		Correct:    uiCorrectDefault,
		SpellCheck: uiCorrectDefault,
		Capitalize: capitalizeCap[h.Capitalize],
	}
	if h.Kind == input.TextSearch && h.Enter == input.EnterDefault && !h.Multiline {
		t.Return = uiReturnSearch
	}
	prose := h.Kind == input.TextPlain || h.Kind == input.TextSearch
	if !h.Autocorrect() || !prose {
		t.Correct, t.SpellCheck = uiCorrectNo, uiCorrectNo
	}
	if !prose {
		t.Capitalize = uiCapNone
	}
	return t
}
