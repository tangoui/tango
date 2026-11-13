package input

// Text-input context for platform input methods (docs/specs/mobile-canvas.md
// §2.3, M3b). Soft keyboards with autocorrection and prediction need the
// text around the caret, the selection and the composing region, and hints
// about the field (keyboard layout, return key, secure entry). A focused
// text widget describes itself with a TextInputState; the host forwards it
// to the platform. Input methods that edit the field directly report the
// change back as TextEdit or SelectionChange.

// TextKind selects the soft keyboard layout of a text field.
type TextKind uint8

const (
	TextPlain    TextKind = iota // general text
	TextNumber                   // digits only
	TextDecimal                  // digits and a decimal separator
	TextPhone                    // telephone keypad
	TextEmail                    // e-mail address
	TextURL                      // web address
	TextPassword                 // secure entry: no echo, no suggestions
	TextSearch                   // search query
)

func (k TextKind) String() string {
	switch k {
	case TextPlain:
		return "plain"
	case TextNumber:
		return "number"
	case TextDecimal:
		return "decimal"
	case TextPhone:
		return "phone"
	case TextEmail:
		return "email"
	case TextURL:
		return "url"
	case TextPassword:
		return "password"
	case TextSearch:
		return "search"
	}
	return "TextKind(" + itoa(int(k)) + ")"
}

// EnterKey is the label of the soft keyboard's return key. Pressing it
// still arrives as a KeyEnter key event.
type EnterKey uint8

const (
	EnterDefault EnterKey = iota // platform default for the field
	EnterDone
	EnterGo
	EnterNext
	EnterSearch
	EnterSend
	EnterNewline // inserts a line break (multi-line fields)
)

func (k EnterKey) String() string {
	switch k {
	case EnterDefault:
		return "default"
	case EnterDone:
		return "done"
	case EnterGo:
		return "go"
	case EnterNext:
		return "next"
	case EnterSearch:
		return "search"
	case EnterSend:
		return "send"
	case EnterNewline:
		return "newline"
	}
	return "EnterKey(" + itoa(int(k)) + ")"
}

// Capitalize is the automatic capitalization a soft keyboard applies.
type Capitalize uint8

const (
	CapitalizeNone Capitalize = iota
	CapitalizeWords
	CapitalizeSentences
)

// TextHints describe a text field to the input method.
type TextHints struct {
	Kind      TextKind
	Enter     EnterKey
	Multiline bool
	// NoAutocorrect turns autocorrection and suggestions off; they are on
	// by default. TextPassword fields never get them (see Autocorrect).
	NoAutocorrect bool
	Capitalize    Capitalize
}

// Autocorrect reports whether the input method may correct and suggest
// words: not for passwords, nor when NoAutocorrect is set.
func (h TextHints) Autocorrect() bool { return !h.NoAutocorrect && h.Kind != TextPassword }

// TextStateWindow is the number of runes TextInputState.Text carries at
// most on each side of the selection.
const TextStateWindow = 256

// TextInputState describes the focused text field near the caret. Offsets
// are rune positions relative to the start of Text; Offset places Text
// within the whole field, so field position = Offset + a Text offset.
type TextInputState struct {
	Hints TextHints
	// Text is the field's text around the selection: at most
	// TextStateWindow runes before SelStart and after SelEnd.
	Text string
	// Offset is the field position of Text's first rune.
	Offset int
	// SelStart..SelEnd is the selection (SelStart == SelEnd: a caret).
	SelStart, SelEnd int
	// CompStart..CompEnd is the composing (pre-edit) region; both are -1
	// when there is none.
	CompStart, CompEnd int
}

// HasComposition reports whether the state has a composing region.
func (s TextInputState) HasComposition() bool { return s.CompStart >= 0 && s.CompEnd >= s.CompStart }

// TextEdit asks the focused field to replace the runes [Start, End) (field
// positions) with Text: an input method's autocorrection, a
// deleteSurroundingText, the commit after setComposingRegion. The caret
// ends after the inserted text unless a SelectionChange follows.
type TextEdit struct {
	Start, End int
	Text       string
}

// SelectionChange asks the focused field to select the runes [Start, End)
// (field positions; Start == End: move the caret).
type SelectionChange struct{ Start, End int }

func (TextEdit) isEvent()        {}
func (SelectionChange) isEvent() {}
