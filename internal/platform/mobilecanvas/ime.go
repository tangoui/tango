package mobilecanvas

import (
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/input"
)

// Input-method shadow document without context (docs/specs/mobile-canvas.md
// §2.3, M3a). UITextInput and Android's InputConnection both expect a
// document to edit. Until the focused widget describes its text
// (input.TextInputState, M3b) the native side edits a shadow document that
// holds only the current composition (marked text) with the caret at its
// end. Every native edit calls the matching IMEDoc method once; it returns
// the events for the canvas sink and the new snapshot, from which the
// native mirror (NSString, SpannableStringBuilder) answers the input
// method's frequent queries locally.
//
//	native call                                   events
//	setMarkedText / setComposingText              Composition{Text, Cursor}
//	insertText / commitText                       TextInput{Text} + Composition{""}
//	unmarkText / finishComposingText              TextInput{marked} + Composition{""}
//	deleteBackward / deleteSurroundingText(n, 0)  n × Backspace down + up
//	insertText "\n" / performEditorAction         Enter down + up

// Range is a span of UTF-16 code units: the unit of NSRange and of
// Android's CharSequence offsets.
type Range struct{ Start, Len int }

// End returns Start+Len.
func (r Range) End() int { return r.Start + r.Len }

// IMESnapshot is the shadow document after an IMEDoc call, in UTF-16 units
// (what the native mirror needs).
type IMESnapshot struct {
	// Marked is the composition; "" when there is none. The document holds
	// nothing else.
	Marked string
	// Len is Marked's length in UTF-16 units.
	Len int
	// Sel is the selection inside Marked.
	Sel Range
	// Caret is where the caret sits in the document: the end of Marked.
	Caret int
}

// IMEDoc is the shadow document of one canvas' input method. The zero
// value is an empty document. Main thread.
type IMEDoc struct {
	marked string
	sel    Range // UTF-16, within marked
}

// Snapshot returns the current document.
func (d *IMEDoc) Snapshot() IMESnapshot {
	n := UTF16Len(d.marked)
	return IMESnapshot{Marked: d.marked, Len: n, Sel: d.sel, Caret: n}
}

// Marked reports whether a composition is in progress.
func (d *IMEDoc) Marked() bool { return d.marked != "" }

// SetMarked replaces the composition with text and selects sel inside it
// (UTF-16 units, clamped). The caret the widget shows is sel's start. An
// empty text cancels the composition without committing it.
func (d *IMEDoc) SetMarked(text string, sel Range) ([]input.Event, IMESnapshot) {
	text = cleanText(text)
	if text == "" {
		evs := d.clear(nil)
		return evs, d.Snapshot()
	}
	d.marked = text
	d.sel = clampRange(sel, UTF16Len(text))
	ev := input.Composition{Text: text, Cursor: UTF16ToRune(text, d.sel.Start)}
	return []input.Event{ev}, d.Snapshot()
}

// Insert commits text in place of the composition (if any). Line breaks in
// text become Enter key presses between the committed pieces, so a
// single-line field can treat them as "submit".
func (d *IMEDoc) Insert(text string) ([]input.Event, IMESnapshot) {
	var evs []input.Event
	had := d.marked != ""
	d.marked, d.sel = "", Range{}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	for i, part := range strings.Split(text, "\n") {
		if i > 0 {
			evs = appendKey(evs, input.KeyEnter)
		}
		if part = cleanText(part); part != "" {
			evs = append(evs, input.TextInput{Text: part})
		}
	}
	if had {
		evs = append(evs, input.Composition{})
	}
	return evs, d.Snapshot()
}

// Unmark commits the composition as it stands; nothing happens without
// one.
func (d *IMEDoc) Unmark() ([]input.Event, IMESnapshot) {
	if d.marked == "" {
		return nil, d.Snapshot()
	}
	text := d.marked
	d.marked, d.sel = "", Range{}
	return []input.Event{input.TextInput{Text: text}, input.Composition{}}, d.Snapshot()
}

// DeleteBackward removes n characters before the caret. Inside a
// composition it shortens the marked text by n user-perceived characters
// (an emoji sequence or a letter with its accents counts as one) and
// reports the new composition; otherwise it presses Backspace n times so
// the widget edits its own text.
func (d *IMEDoc) DeleteBackward(n int) ([]input.Event, IMESnapshot) {
	if n <= 0 {
		return nil, d.Snapshot()
	}
	if d.marked == "" {
		evs := make([]input.Event, 0, 2*n)
		for range n {
			evs = appendKey(evs, input.KeyBackspace)
		}
		return evs, d.Snapshot()
	}
	end := utf8.RuneCountInString(d.marked)
	for range n {
		if end == 0 {
			break
		}
		end = PrevBoundary(d.marked, end)
	}
	text := d.marked[:RuneToByte(d.marked, end)]
	if text == "" {
		return d.clear(nil), d.Snapshot()
	}
	d.marked = text
	u := UTF16Len(text)
	d.sel = Range{Start: u}
	return []input.Event{input.Composition{Text: text, Cursor: end}}, d.Snapshot()
}

// Enter presses the Enter key (the return key, performEditorAction). A
// composition in progress is committed first.
func (d *IMEDoc) Enter() ([]input.Event, IMESnapshot) {
	evs, _ := d.Unmark()
	return appendKey(evs, input.KeyEnter), d.Snapshot()
}

// Reset drops the composition without events: the text input was turned
// off or moved to another field, whose widget already ended it.
func (d *IMEDoc) Reset() { d.marked, d.sel = "", Range{} }

// clear cancels the composition, appending Composition{""} if there was one.
func (d *IMEDoc) clear(evs []input.Event) []input.Event {
	if d.marked == "" {
		return evs
	}
	d.marked, d.sel = "", Range{}
	return append(evs, input.Composition{})
}

func appendKey(evs []input.Event, k input.Key) []input.Event {
	return append(evs, input.KeyEvent{Key: k, Down: true}, input.KeyEvent{Key: k})
}

// clampRange limits r to [0, n] with a non-negative length.
func clampRange(r Range, n int) Range {
	s := min(max(r.Start, 0), n)
	e := min(max(r.End(), s), n)
	return Range{Start: s, Len: e - s}
}

// cleanText drops control characters (input methods sometimes pass them
// through); text input carries text only. Line breaks are handled by the
// callers.
func cleanText(s string) string {
	clean := true
	for _, r := range s {
		if isControl(r) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) }
