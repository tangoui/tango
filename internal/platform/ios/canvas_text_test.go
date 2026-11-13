package ios

import (
	"reflect"
	"testing"

	"github.com/tangoui/tango/internal/platform/mobilecanvas"
	"github.com/tangoui/tango/ui/input"
)

func TestTraitsFor(t *testing.T) {
	for _, tc := range []struct {
		name string
		h    input.TextHints
		want textTraits
	}{
		{"zero", input.TextHints{}, textTraits{Keyboard: uiKeyboardDefault, Return: uiReturnDefault}},
		{"sentences", input.TextHints{Capitalize: input.CapitalizeSentences, Enter: input.EnterDone},
			textTraits{Return: uiReturnDone, Capitalize: uiCapSentences}},
		{"no autocorrect", input.TextHints{NoAutocorrect: true, Capitalize: input.CapitalizeWords},
			textTraits{Correct: uiCorrectNo, SpellCheck: uiCorrectNo, Capitalize: uiCapWords}},
		{"password", input.TextHints{Kind: input.TextPassword, Capitalize: input.CapitalizeWords, Enter: input.EnterGo},
			textTraits{Return: uiReturnGo, Secure: true, Correct: uiCorrectNo, SpellCheck: uiCorrectNo}},
		{"email", input.TextHints{Kind: input.TextEmail, Enter: input.EnterNext, Capitalize: input.CapitalizeSentences},
			textTraits{Keyboard: uiKeyboardEmailAddress, Return: uiReturnNext, Correct: uiCorrectNo, SpellCheck: uiCorrectNo}},
		{"url", input.TextHints{Kind: input.TextURL, Enter: input.EnterGo},
			textTraits{Keyboard: uiKeyboardURL, Return: uiReturnGo, Correct: uiCorrectNo, SpellCheck: uiCorrectNo}},
		{"number", input.TextHints{Kind: input.TextNumber},
			textTraits{Keyboard: uiKeyboardNumberPad, Correct: uiCorrectNo, SpellCheck: uiCorrectNo}},
		{"decimal", input.TextHints{Kind: input.TextDecimal},
			textTraits{Keyboard: uiKeyboardDecimalPad, Correct: uiCorrectNo, SpellCheck: uiCorrectNo}},
		{"phone", input.TextHints{Kind: input.TextPhone, Enter: input.EnterSend},
			textTraits{Keyboard: uiKeyboardPhonePad, Return: uiReturnSend, Correct: uiCorrectNo, SpellCheck: uiCorrectNo}},
		{"search", input.TextHints{Kind: input.TextSearch},
			textTraits{Keyboard: uiKeyboardWebSearch, Return: uiReturnSearch}},
		{"search done", input.TextHints{Kind: input.TextSearch, Enter: input.EnterDone},
			textTraits{Keyboard: uiKeyboardWebSearch, Return: uiReturnDone}},
		{"multiline newline", input.TextHints{Multiline: true, Enter: input.EnterNewline},
			textTraits{Return: uiReturnDefault}},
		{"unknown kind", input.TextHints{Kind: 99, Enter: 99}, textTraits{Correct: uiCorrectNo, SpellCheck: uiCorrectNo}},
	} {
		if got := traitsFor(tc.h); got != tc.want {
			t.Errorf("%s: traitsFor(%+v) = %+v, want %+v", tc.name, tc.h, got, tc.want)
		}
	}
}

func TestTraitsTablesComplete(t *testing.T) {
	for k := input.TextPlain; k <= input.TextSearch; k++ {
		if _, ok := kindKeyboard[k]; !ok {
			t.Errorf("kindKeyboard misses %v", k)
		}
	}
	for k := input.EnterDefault; k <= input.EnterNewline; k++ {
		if _, ok := enterReturn[k]; !ok {
			t.Errorf("enterReturn misses %v", k)
		}
	}
	for k := input.CapitalizeNone; k <= input.CapitalizeSentences; k++ {
		if _, ok := capitalizeCap[k]; !ok {
			t.Errorf("capitalizeCap misses %d", k)
		}
	}
}

func rng(s, n int) mobilecanvas.Range { return mobilecanvas.Range{Start: s, Len: n} }

func noMark() mobilecanvas.Range { return mobilecanvas.Range{Start: -1} }

func checkMirror(t *testing.T, d *textDoc, want textMirror) {
	t.Helper()
	if got := d.mirror(); got != want {
		t.Errorf("mirror = %+v, want %+v", got, want)
	}
}

func checkEvents(t *testing.T, got []input.Event, want ...input.Event) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %#v, want %#v", got, want)
	}
}

func keyPair(k input.Key) []input.Event {
	return []input.Event{input.KeyEvent{Key: k, Down: true}, input.KeyEvent{Key: k}}
}

// M3a: no state, the document is only the composition.
func TestTextDocNoState(t *testing.T) {
	var d textDoc
	checkMirror(t, &d, textMirror{Mark: noMark()})
	checkEvents(t, d.setMarked("zhong", rng(5, 0)), input.Composition{Text: "zhong", Cursor: 5})
	checkMirror(t, &d, textMirror{Text: "zhong", Sel: rng(5, 0), Mark: rng(0, 5)})
	checkEvents(t, d.deleteBackward(), input.Composition{Text: "zhon", Cursor: 4})
	checkEvents(t, d.insert("中"), input.TextInput{Text: "中"}, input.Composition{})
	checkMirror(t, &d, textMirror{Mark: noMark()})
	checkEvents(t, d.insert("abc"), input.TextInput{Text: "abc"})
	checkEvents(t, d.deleteBackward(), keyPair(input.KeyBackspace)...)
	checkEvents(t, d.insert("\n"), keyPair(input.KeyEnter)...)
	checkEvents(t, d.selectRange(rng(0, 0)))
	// Without state a replacement can only type at the caret.
	checkEvents(t, d.replace(rng(0, 3), "abd"), input.TextInput{Text: "abd"})
	d.setMarked("ni", rng(2, 0))
	checkEvents(t, d.unmark(), input.TextInput{Text: "ni"}, input.Composition{})
	checkMirror(t, &d, textMirror{Mark: noMark()})
}

func TestTextDocState(t *testing.T) {
	var d textDoc
	tc, sc := d.setState(input.TextInputState{Text: "hello wrld", Offset: 100, SelStart: 10, SelEnd: 10, CompStart: -1, CompEnd: -1})
	if !tc || !sc {
		t.Errorf("setState changed = %v, %v, want true, true", tc, sc)
	}
	checkMirror(t, &d, textMirror{Text: "hello wrld", Sel: rng(10, 0), Mark: noMark()})
	if tc, sc := d.setState(input.TextInputState{Text: "hello wrld", Offset: 100, SelStart: 10, SelEnd: 10, CompStart: -1, CompEnd: -1}); tc || sc {
		t.Errorf("same state changed = %v, %v", tc, sc)
	}

	// Autocorrection replaces a word before the caret.
	checkEvents(t, d.replace(rng(6, 4), "world"), input.TextEdit{Start: 106, End: 110, Text: "world"})
	checkMirror(t, &d, textMirror{Text: "hello world", Sel: rng(11, 0), Mark: noMark()})

	// Typing at the caret stays TextInput.
	checkEvents(t, d.insert("!"), input.TextInput{Text: "!"})
	checkMirror(t, &d, textMirror{Text: "hello world!", Sel: rng(12, 0), Mark: noMark()})
	checkEvents(t, d.replace(rng(12, 0), "?"), input.TextInput{Text: "?"})

	// Backspace edits the copy too.
	checkEvents(t, d.deleteBackward(), keyPair(input.KeyBackspace)...)
	checkMirror(t, &d, textMirror{Text: "hello world!", Sel: rng(12, 0), Mark: noMark()})

	// UIKit moves the caret: SelectionChange, then typing there.
	checkEvents(t, d.selectRange(rng(5, 0)), input.SelectionChange{Start: 105, End: 105})
	checkEvents(t, d.insert(","), input.TextInput{Text: ","})
	checkMirror(t, &d, textMirror{Text: "hello, world!", Sel: rng(6, 0), Mark: noMark()})

	// A selection, deleted with Backspace.
	checkEvents(t, d.selectRange(rng(0, 5)), input.SelectionChange{Start: 100, End: 105})
	checkMirror(t, &d, textMirror{Text: "hello, world!", Sel: rng(0, 5), Mark: noMark()})
	d.deleteBackward()
	checkMirror(t, &d, textMirror{Text: ", world!", Sel: rng(0, 0), Mark: noMark()})
}

func TestTextDocStateComposition(t *testing.T) {
	var d textDoc
	d.setState(input.TextInputState{Text: "ab😀cd", SelStart: 3, SelEnd: 4, CompStart: -1, CompEnd: -1})
	checkMirror(t, &d, textMirror{Text: "ab😀cd", Sel: rng(4, 1), Mark: noMark()})

	// The composition replaces the selection in the mirror.
	checkEvents(t, d.setMarked("ni", rng(2, 0)), input.Composition{Text: "ni", Cursor: 2})
	checkMirror(t, &d, textMirror{Text: "ab😀nid", Sel: rng(6, 0), Mark: rng(4, 2)})
	if got := d.field(6); got != 4 {
		t.Errorf("field(after composition) = %d, want 4", got)
	}
	if got := d.field(5); got != 3 {
		t.Errorf("field(inside composition) = %d, want 3", got)
	}

	// The widget's next state does not report the composition: it stays.
	if tc, sc := d.setState(input.TextInputState{Text: "ab😀cd", SelStart: 3, SelEnd: 4, CompStart: -1, CompEnd: -1}); tc || sc {
		t.Errorf("state during composition changed = %v, %v", tc, sc)
	}

	// Moving the caret inside the composition.
	checkEvents(t, d.selectRange(rng(5, 0)), input.Composition{Text: "ni", Cursor: 1})
	checkMirror(t, &d, textMirror{Text: "ab😀nid", Sel: rng(5, 0), Mark: rng(4, 2)})

	// Replacing part of the composition keeps composing.
	checkEvents(t, d.replace(rng(4, 1), "N"), input.Composition{Text: "Ni", Cursor: 1})

	// Commit replaces the selection.
	checkEvents(t, d.insert("你"), input.TextInput{Text: "你"}, input.Composition{})
	checkMirror(t, &d, textMirror{Text: "ab😀你d", Sel: rng(5, 0), Mark: noMark()})

	// Selecting outside the composition commits it first.
	d.setMarked("x", rng(1, 0))
	checkEvents(t, d.selectRange(rng(0, 0)), input.TextInput{Text: "x"}, input.Composition{}, input.SelectionChange{Start: 0, End: 0})
	checkMirror(t, &d, textMirror{Text: "ab😀你xd", Sel: rng(0, 0), Mark: noMark()})
}

func TestTextDocStateWithWidgetComposition(t *testing.T) {
	var d textDoc
	// A widget that reports the composition inside its text.
	d.setState(input.TextInputState{Text: "a中bc", SelStart: 3, SelEnd: 3, CompStart: 2, CompEnd: 4})
	checkMirror(t, &d, textMirror{Text: "a中bc", Sel: rng(3, 0), Mark: rng(2, 2)})
	checkEvents(t, d.unmark(), input.TextInput{Text: "bc"}, input.Composition{})
	checkMirror(t, &d, textMirror{Text: "a中bc", Sel: rng(4, 0), Mark: noMark()})
}

func TestTextDocMultiline(t *testing.T) {
	var d textDoc
	d.setState(input.TextInputState{Text: "ab", SelStart: 2, SelEnd: 2, CompStart: -1, CompEnd: -1})
	checkEvents(t, d.insert("\n"), keyPair(input.KeyEnter)...)
	checkMirror(t, &d, textMirror{Text: "ab", Sel: rng(2, 0), Mark: noMark()})
	d.setState(input.TextInputState{Hints: input.TextHints{Multiline: true}, Text: "ab", SelStart: 2, SelEnd: 2, CompStart: -1, CompEnd: -1})
	d.insert("c\nd")
	checkMirror(t, &d, textMirror{Text: "abc\nd", Sel: rng(5, 0), Mark: noMark()})
}

func TestTextDocClearState(t *testing.T) {
	var d textDoc
	d.setState(input.TextInputState{Text: "abc", SelStart: 3, SelEnd: 3, CompStart: -1, CompEnd: -1})
	d.setMarked("x", rng(1, 0))
	d.clearState()
	checkMirror(t, &d, textMirror{Text: "x", Sel: rng(1, 0), Mark: rng(0, 1)})
	d.reset()
	checkMirror(t, &d, textMirror{Mark: noMark()})
}

func TestTextDocZeroStateKeepsComposition(t *testing.T) {
	var d textDoc
	d.setMarked("ni", rng(2, 0))
	d.setState(input.TextInputState{}) // CompStart == CompEnd == 0
	checkMirror(t, &d, textMirror{Text: "ni", Sel: rng(2, 0), Mark: rng(0, 2)})
}
