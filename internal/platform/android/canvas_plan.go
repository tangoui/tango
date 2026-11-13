package android

// Text field hints to the EditorInfo of the canvas' input connection
// (docs/specs/mobile-canvas.md §2.3): input.TextHints decide
// EditorInfo.inputType (keyboard layout, secure entry, suggestions,
// capitalization, multi-line) and EditorInfo.imeOptions (the label of the
// return key). Kept free of build tags so it is unit tested on every OS.
//
// Numbers are the public constants of android.text.InputType and
// android.view.inputmethod.EditorInfo (API 24+).

import "github.com/tangoui/tango/ui/input"

// InputType classes, variations and flags.
const (
	itClassText   = 0x1
	itClassNumber = 0x2
	itClassPhone  = 0x3

	itTextCapWords       = 0x2000
	itTextCapSentences   = 0x4000
	itTextAutoCorrect    = 0x8000
	itTextMultiLine      = 0x20000
	itTextNoSuggestions  = 0x80000
	itTextVariationURI   = 0x10
	itTextVariationEmail = 0x20
	itTextVariationPass  = 0x80

	itNumberDecimal = 0x2000
)

// EditorInfo.imeOptions actions and flags.
const (
	imeActionUnspecified = 0
	imeActionNone        = 1
	imeActionGo          = 2
	imeActionSearch      = 3
	imeActionSend        = 4
	imeActionNext        = 5
	imeActionDone        = 6

	imeFlagNoPersonalizedLearning = 0x1000000
	imeFlagNoFullscreen           = 0x2000000
	imeFlagNoExtractUI            = 0x10000000
	imeFlagNoEnterAction          = 0x40000000
)

// editorInfo returns EditorInfo.inputType and imeOptions for a field.
// The canvas never uses the full-screen extract editor: it has no text of
// its own to show there.
func editorInfo(h input.TextHints) (inputType, imeOptions int) {
	text := false
	multi := false
	switch h.Kind {
	case input.TextNumber:
		inputType = itClassNumber
	case input.TextDecimal:
		inputType = itClassNumber | itNumberDecimal
	case input.TextPhone:
		inputType = itClassPhone
	case input.TextEmail:
		inputType = itClassText | itTextVariationEmail
	case input.TextURL:
		inputType = itClassText | itTextVariationURI
	case input.TextPassword:
		inputType = itClassText | itTextVariationPass
	default: // plain, search
		inputType = itClassText
		text = true
	}
	if inputType&0xf == itClassText {
		if h.Autocorrect() {
			inputType |= itTextAutoCorrect
		} else {
			inputType |= itTextNoSuggestions
		}
		if text {
			switch h.Capitalize {
			case input.CapitalizeWords:
				inputType |= itTextCapWords
			case input.CapitalizeSentences:
				inputType |= itTextCapSentences
			}
		}
		if h.Multiline && h.Kind != input.TextPassword {
			inputType |= itTextMultiLine
			multi = true
		}
	}

	imeOptions = imeFlagNoExtractUI | imeFlagNoFullscreen
	if h.Kind == input.TextPassword {
		imeOptions |= imeFlagNoPersonalizedLearning
	}
	switch h.Enter {
	case input.EnterDone:
		imeOptions |= imeActionDone
	case input.EnterGo:
		imeOptions |= imeActionGo
	case input.EnterNext:
		imeOptions |= imeActionNext
	case input.EnterSearch:
		imeOptions |= imeActionSearch
	case input.EnterSend:
		imeOptions |= imeActionSend
	case input.EnterNewline:
		imeOptions |= imeActionNone | imeFlagNoEnterAction
	default:
		switch {
		case multi:
			// The return key breaks lines.
			imeOptions |= imeActionUnspecified | imeFlagNoEnterAction
		case h.Kind == input.TextSearch:
			imeOptions |= imeActionSearch
		case h.Kind == input.TextURL:
			imeOptions |= imeActionGo
		default:
			imeOptions |= imeActionDone
		}
	}
	return inputType, imeOptions
}
