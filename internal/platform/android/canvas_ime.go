package android

import (
	"slices"
	"strings"

	"github.com/tangoui/tango/internal/platform/mobilecanvas"
	"github.com/tangoui/tango/ui/input"
)

// The input-method session of the Android canvas
// (docs/specs/mobile-canvas.md §2.3). CanvasInput.java, the canvas'
// InputConnection, calls Go once per edit (setComposingText, commitText,
// deleteSurroundingText, ...); the session turns the edit into input events
// for the canvas sink and updates its mirror: the document the Java side
// keeps in a SpannableStringBuilder to answer the input method's frequent
// queries (getTextBeforeCursor, getSelectedText, ...) without crossing JNI.
//
// Two modes:
//
//   - Without context (M3a): the focused widget does not describe its text.
//     The mirror is mobilecanvas.IMEDoc's shadow document, which holds only
//     the composition; edits become Composition, TextInput and key events.
//   - With context (M3b): the widget sent an input.TextInputState. The
//     mirror is a window of the field around the selection. The pre-edit
//     still travels as Composition events, but edits of committed text
//     (a word picked with setComposingRegion and recomposed or committed
//     in place, deleteSurroundingText, setSelection) become input.TextEdit
//     and input.SelectionChange. Each
//     edit is also applied to the mirror at once, so the input method reads
//     its own change before the widget's next state arrives; that state
//     then replaces the prediction. A state that disagrees with the
//     prediction (the widget filtered the text, or the app changed it)
//     restarts the input method.
//
// Kept free of build tags so it is unit tested on every OS.

// imeMirror is the document of the Java mirror: a window of the focused
// field. Positions are runes within Text; Offset is the field position of
// Text's first rune.
type imeMirror struct {
	Text               []rune
	Offset             int
	SelStart, SelEnd   int
	CompStart, CompEnd int // -1: no composing span
}

func (m *imeMirror) hasComp() bool { return m.CompStart >= 0 && m.CompEnd >= m.CompStart }

func (m *imeMirror) clearComp() { m.CompStart, m.CompEnd = -1, -1 }

// span returns the range the next pre-edit or commit replaces: the
// composing span, else the selection.
func (m *imeMirror) span() (int, int) {
	if m.hasComp() {
		return m.CompStart, m.CompEnd
	}
	return m.SelStart, m.SelEnd
}

// replace puts text in place of Text[s:e].
func (m *imeMirror) replace(s, e int, text []rune) {
	m.Text = slices.Concat(m.Text[:s:s], text, m.Text[e:])
}

// java returns the mirror in the units of the Java side: the text and
// UTF-16 offsets within it, plus base, which the input method adds to them
// to get document positions. Runes before the window count one unit each
// there: the canvas never learns the UTF-16 length of the text it does not
// send, and the input method only needs positions that stay consistent.
func (m *imeMirror) java() (text string, base, selStart, selEnd, compStart, compEnd int) {
	compStart, compEnd = -1, -1
	if m.hasComp() {
		compStart, compEnd = utf16At(m.Text, m.CompStart), utf16At(m.Text, m.CompEnd)
	}
	return string(m.Text), m.Offset, utf16At(m.Text, m.SelStart), utf16At(m.Text, m.SelEnd), compStart, compEnd
}

// rune converts a document position of the input method (base + UTF-16
// offset) to a rune position in Text, clamped to the window.
func (m *imeMirror) rune(pos int) int { return runeAt(m.Text, pos-m.Offset) }

// utf16At returns the UTF-16 length of rs[:i].
func utf16At(rs []rune, i int) int {
	n := 0
	for _, r := range rs[:min(max(i, 0), len(rs))] {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// runeAt converts a UTF-16 offset into rs to a rune index, clamped; an
// offset inside a surrogate pair rounds down.
func runeAt(rs []rune, u int) int {
	n := 0
	for i, r := range rs {
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if n+w > u {
			return i
		}
		n += w
	}
	return len(rs)
}

// consistent reports whether the widget's state n agrees with the
// prediction p: the same text where both windows overlap, ending at the
// same field position. Window starts may differ (the window slides with the
// selection); its end moves with every edit in both.
func consistent(p, n imeMirror) bool {
	if p.Offset+len(p.Text) != n.Offset+len(n.Text) {
		return false
	}
	lo := max(p.Offset, n.Offset)
	hi := p.Offset + len(p.Text)
	return slices.Equal(p.Text[lo-p.Offset:hi-p.Offset], n.Text[lo-n.Offset:hi-n.Offset])
}

// imeSession is the input-method state of one canvas while text input is
// on. Main thread.
type imeSession struct {
	doc mobilecanvas.IMEDoc
	// state: the focused widget describes itself (M3b).
	state bool
	// hints are the hints of the widget's last state (zero: a plain field).
	hints input.TextHints
	m     imeMirror
	// region: m's composing span is committed text the input method picked
	// with setComposingRegion (Gboard does when the caret enters a word),
	// not a pre-edit. A pre-edit is in doc.
	region bool
	// edited: the input method or a key edited the document since the
	// last state, so the next state answers it.
	edited bool
}

// reset drops everything, back to an empty shadow document. With cancel a
// pre-edit in progress is cancelled with Composition{""}; without it the
// widget already ended it (focus moved, text input turned off).
func (s *imeSession) reset(cancel bool) []input.Event {
	var evs []input.Event
	if cancel && s.doc.Marked() {
		evs = append(evs, input.Composition{})
	}
	*s = imeSession{}
	s.m.clearComp()
	return evs
}

// syncDoc rebuilds the mirror from the shadow document (M3a).
func (s *imeSession) syncDoc() {
	snap := s.doc.Snapshot()
	rs := []rune(snap.Marked)
	s.m = imeMirror{Text: rs}
	s.m.SelStart = runeAt(rs, snap.Sel.Start)
	s.m.SelEnd = runeAt(rs, snap.Sel.End())
	s.m.clearComp()
	if len(rs) > 0 {
		s.m.CompStart, s.m.CompEnd = 0, len(rs)
	}
}

// setPreedit shows the shadow document's pre-edit in the mirror in place of
// the span, with the caret at cursor runes into it (M3b).
func (s *imeSession) setPreedit(start, end, cursor int) {
	mk := []rune(s.doc.Snapshot().Marked)
	s.m.replace(start, end, mk)
	if len(mk) == 0 {
		s.m.clearComp()
	} else {
		s.m.CompStart, s.m.CompEnd = start, start+len(mk)
	}
	c := start + min(max(cursor, 0), len(mk))
	s.m.SelStart, s.m.SelEnd = c, c
}

// ncpCaret converts an InputConnection newCursorPosition for text of n
// runes placed at start into a position, clamped to [0, total]: above zero
// it counts from the end of the text (1 = right after it), otherwise from
// its start.
func ncpCaret(start, n, ncp, total int) int {
	c := start + ncp
	if ncp > 0 {
		c = start + n + ncp - 1
	}
	return min(max(c, 0), total)
}

// composing handles setComposingText.
func (s *imeSession) composing(text string, ncp int) []input.Event {
	s.edited = true
	text = imeClean(text, false)
	rs := []rune(text)
	if !s.state {
		c := ncpCaret(0, len(rs), ncp, len(rs))
		evs, _ := s.doc.SetMarked(text, mobilecanvas.Range{Start: utf16At(rs, c)})
		s.syncDoc()
		return evs
	}
	start, end := s.m.span()
	c := ncpCaret(0, len(rs), ncp, len(rs))
	if s.region {
		// Recomposing committed text (the input method picked a word):
		// the text stays in the field, edited in place, so the field's
		// value never loses the word while it is being changed.
		evs := []input.Event{input.TextEdit{Start: s.m.Offset + start, End: s.m.Offset + end, Text: text}}
		s.m.replace(start, end, rs)
		if len(rs) == 0 {
			s.region = false
			s.m.clearComp()
		} else {
			s.m.CompStart, s.m.CompEnd = start, start+len(rs)
		}
		s.m.SelStart = start + c
		s.m.SelEnd = s.m.SelStart
		return evs
	}
	evs, _ := s.doc.SetMarked(text, mobilecanvas.Range{Start: utf16At(rs, c)})
	// A caret outside the pre-edit is not representable in the widget;
	// the mirror keeps it inside too.
	s.setPreedit(start, end, c)
	return evs
}

// commit handles commitText.
func (s *imeSession) commit(text string, ncp int) []input.Event {
	s.edited = true
	text = imeClean(text, true)
	if !s.state {
		evs, _ := s.doc.Insert(text)
		s.syncDoc()
		return evs
	}
	start, end := s.m.span()
	var evs []input.Event
	if s.region {
		s.region = false
		if !strings.Contains(text, "\n") {
			// An autocorrection or a re-picked word: one edit.
			evs = append(evs, input.TextEdit{Start: s.m.Offset + start, End: s.m.Offset + end, Text: text})
			s.place(start, end, text, ncp)
			return evs
		}
		evs = append(evs, input.TextEdit{Start: s.m.Offset + start, End: s.m.Offset + end})
	}
	more, _ := s.doc.Insert(text)
	evs = append(evs, more...)
	if !s.hints.Multiline {
		text = strings.ReplaceAll(text, "\n", "") // line breaks were Enter presses
	}
	s.place(start, end, text, ncp)
	return evs
}

// place predicts committed text in the mirror.
func (s *imeSession) place(start, end int, text string, ncp int) {
	rs := []rune(text)
	s.m.replace(start, end, rs)
	s.m.clearComp()
	c := ncpCaret(start, len(rs), ncp, len(s.m.Text))
	s.m.SelStart, s.m.SelEnd = c, c
}

// finish handles finishComposingText: the pre-edit is committed as it
// stands; a picked region simply stops being one.
func (s *imeSession) finish() []input.Event {
	s.edited = true
	if s.region {
		s.region = false
		s.m.clearComp()
		return nil
	}
	evs, _ := s.doc.Unmark()
	if !s.state {
		s.syncDoc()
	} else {
		s.m.clearComp()
	}
	return evs
}

// deleteAround handles deleteSurroundingText (codePoints false: lengths
// in UTF-16 units) and deleteSurroundingTextInCodePoints: before units
// before the selection and after units after it.
func (s *imeSession) deleteAround(before, after int, codePoints bool) []input.Event {
	s.edited = true
	before, after = max(before, 0), max(after, 0)
	if !s.state || s.doc.Marked() {
		if s.state {
			// Not expected while composing; commit the pre-edit first so
			// the deletion reaches committed text.
			evs := s.finish()
			return append(evs, s.deleteAround(before, after, codePoints)...)
		}
		// Shadow document: Backspace and Delete presses.
		var evs []input.Event
		if before > 0 {
			evs, _ = s.doc.DeleteBackward(before)
		}
		for range after {
			evs = append(evs, input.KeyEvent{Key: input.KeyDelete, Down: true}, input.KeyEvent{Key: input.KeyDelete})
		}
		s.syncDoc()
		return evs
	}
	ss, se := s.m.SelStart, s.m.SelEnd
	var bs, ae int
	if codePoints {
		bs, ae = max(ss-before, 0), min(se+after, len(s.m.Text))
	} else {
		bs = runeAt(s.m.Text, utf16At(s.m.Text, ss)-before)
		ae = runeAt(s.m.Text, utf16At(s.m.Text, se)+after)
	}
	var evs []input.Event
	if ae > se { // after first: the positions before stay valid
		evs = append(evs, input.TextEdit{Start: s.m.Offset + se, End: s.m.Offset + ae})
	}
	if bs < ss {
		evs = append(evs, input.TextEdit{Start: s.m.Offset + bs, End: s.m.Offset + ss})
	}
	if s.region {
		cs, ce := s.m.CompStart, s.m.CompEnd
		switch {
		case ce <= bs:
		case cs >= ae:
			cs, ce = cs-(ss-bs)-(ae-se), ce-(ss-bs)-(ae-se)
		case cs >= ss && ce <= se:
			cs, ce = cs-(ss-bs), ce-(ss-bs)
		default:
			s.region = false
		}
		s.m.CompStart, s.m.CompEnd = cs, ce
	}
	s.m.replace(se, ae, nil)
	s.m.replace(bs, ss, nil)
	s.m.SelStart, s.m.SelEnd = bs, se-(ss-bs)
	if !s.region {
		s.m.clearComp()
	}
	return evs
}

// composeRegion handles setComposingRegion (document positions). Without
// context there is nothing to pick.
func (s *imeSession) composeRegion(start, end int) []input.Event {
	s.edited = true
	if !s.state {
		return nil
	}
	var evs []input.Event
	if s.doc.Marked() {
		evs = s.finish()
	}
	a, b := s.m.rune(start), s.m.rune(end)
	if a > b {
		a, b = b, a
	}
	if a == b {
		s.region = false
		s.m.clearComp()
		return evs
	}
	s.region = true
	s.m.CompStart, s.m.CompEnd = a, b
	return evs
}

// selection handles setSelection (document positions). A pre-edit is
// committed first: the widget shows it at the caret.
func (s *imeSession) selection(start, end int) []input.Event {
	s.edited = true
	if !s.state {
		return nil
	}
	var evs []input.Event
	if s.doc.Marked() {
		evs = s.finish()
	}
	a, b := s.m.rune(start), s.m.rune(end)
	if a > b {
		a, b = b, a
	}
	s.m.SelStart, s.m.SelEnd = a, b
	return append(evs, input.SelectionChange{Start: s.m.Offset + a, End: s.m.Offset + b})
}

// backspace handles the input method's KEYCODE_DEL: inside a pre-edit it
// shortens the pre-edit, otherwise the widget gets a Backspace press.
func (s *imeSession) backspace() []input.Event {
	s.edited = true
	if s.state && s.doc.Marked() {
		start, end := s.m.span()
		evs, _ := s.doc.DeleteBackward(1)
		s.setPreedit(start, end, len([]rune(s.doc.Snapshot().Marked)))
		return evs
	}
	evs, _ := s.doc.DeleteBackward(1)
	if !s.state {
		s.syncDoc()
	} else {
		s.predictKey(input.KeyBackspace)
	}
	return evs
}

// enter handles the return key (KEYCODE_ENTER, performEditorAction): the
// pre-edit is committed, then Enter is pressed.
func (s *imeSession) enter() []input.Event {
	s.edited = true
	had := s.doc.Marked()
	evs, _ := s.doc.Enter()
	if !s.state {
		s.syncDoc()
		return evs
	}
	if had || s.region {
		s.region = false
		s.m.clearComp()
	}
	s.predictKey(input.KeyEnter)
	return evs
}

// typed commits the character of a hardware key press (the KeyEvent itself
// is delivered by the caller): a pre-edit in progress is committed first.
func (s *imeSession) typed(text string) []input.Event {
	evs := s.finish()
	return append(evs, s.commit(text, 1)...)
}

// predictKey applies a key the widget edits its text with to the mirror
// (M3b), so its state matches: Backspace, Delete, and Enter in multi-line
// fields. Other keys leave the text alone.
func (s *imeSession) predictKey(k input.Key) {
	s.edited = true
	if !s.state {
		return
	}
	m := &s.m
	ss, se := m.SelStart, m.SelEnd
	switch k {
	case input.KeyBackspace, input.KeyDelete:
		if ss == se {
			str := string(m.Text)
			if k == input.KeyBackspace {
				ss = mobilecanvas.PrevBoundary(str, ss)
			} else {
				se = mobilecanvas.NextBoundary(str, se)
			}
		}
		if ss == se {
			return
		}
		m.replace(ss, se, nil)
	case input.KeyEnter:
		if !s.hints.Multiline {
			return
		}
		m.replace(ss, se, []rune{'\n'})
		ss++
	default:
		return
	}
	s.region = false
	m.clearComp()
	m.SelStart, m.SelEnd = ss, ss
}

// setState takes the focused widget's state (M3b). It reports whether the
// input method must restart (the first state, other hints, or text that
// does not match the prediction) and returns the events of a pre-edit that
// the restart cancels.
func (s *imeSession) setState(st input.TextInputState) (evs []input.Event, restart bool) {
	rs := []rune(st.Text)
	clamp := func(i int) int { return min(max(i, 0), len(rs)) }
	n := imeMirror{Text: rs, Offset: max(st.Offset, 0), SelStart: clamp(st.SelStart), SelEnd: clamp(st.SelEnd)}
	if n.SelEnd < n.SelStart {
		n.SelStart, n.SelEnd = n.SelEnd, n.SelStart
	}
	n.clearComp()
	base := n
	switch {
	case st.HasComposition():
		n.CompStart, n.CompEnd = clamp(st.CompStart), clamp(st.CompEnd)
	case s.doc.Marked():
		// The widget keeps its pre-edit out of Text: show it at the caret,
		// where the input method put it.
		n.Text = slices.Clone(rs)
		mk := []rune(s.doc.Snapshot().Marked)
		n.replace(n.SelStart, n.SelEnd, mk)
		n.CompStart, n.CompEnd = n.SelStart, n.SelStart+len(mk)
		n.SelStart, n.SelEnd = n.CompEnd, n.CompEnd
	}
	match := consistent(s.m, n)
	if !match && st.Hints.Kind == input.TextPassword && st.Hints == s.hints {
		// Password fields send masked text, which never equals what the
		// input method typed: compare only where the text ends.
		match = s.m.Offset+len(s.m.Text) == n.Offset+len(n.Text)
	}
	// The runner sends a state when it changed or when focus moved to
	// another field. One that equals the mirror while no edit is pending
	// is the second case: a different field that happens to look the same
	// (two empty fields).
	same := match && !s.edited && slices.Equal(s.m.Text, n.Text) && s.m.Offset == n.Offset &&
		s.m.SelStart == n.SelStart && s.m.SelEnd == n.SelEnd
	s.edited = false
	restart = !s.state || st.Hints != s.hints || !match || same
	switch {
	case restart:
		s.region = false
		if s.doc.Marked() {
			// The restarted input method starts without a pre-edit.
			evs = append(evs, input.Composition{})
			s.doc.Reset()
			n = base
		}
	case s.region && !st.HasComposition():
		// Keep the picked region (field positions) while it is in view.
		cs, ce := s.m.Offset+s.m.CompStart-n.Offset, s.m.Offset+s.m.CompEnd-n.Offset
		if cs >= 0 && ce <= len(n.Text) {
			n.CompStart, n.CompEnd = cs, ce
		} else {
			s.region = false
		}
	default:
		s.region = false
	}
	s.state, s.hints, s.m = true, st.Hints, n
	return evs, restart
}

// imeClean normalizes text from the input method: line breaks become "\n"
// (or are dropped when lines is false) and other control characters go.
func imeClean(s string, lines bool) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' && lines {
			return r
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

// keyPlan says how a hardware key press (or release) reaches the canvas.
type keyPlan uint8

const (
	// keyOnly: the KeyEvent alone (text input off, function keys,
	// shortcuts).
	keyOnly keyPlan = iota
	// keyAndText: the KeyEvent, and on the press its character committed
	// through the input method session (which commits a pre-edit first
	// and keeps the mirror up to date). Never also as a separate
	// TextInput: that would type it twice.
	keyAndText
	// keyToIME: a pre-edit is in progress and the key edits it
	// (Backspace, Enter): the session handles the press and the KeyEvent
	// is not delivered, or the widget would edit its text as well.
	keyToIME
)

// planKey routes a hardware key while text input is on (textOn), with or
// without a pre-edit (composing). printable reports whether the press
// produces a character (canvasText). Keys the input method consumed (a
// hardware keyboard in pinyin mode, say) never get here.
func planKey(textOn, composing bool, k input.Key, printable bool) keyPlan {
	if !textOn {
		return keyOnly
	}
	if composing && (k == input.KeyBackspace || k == input.KeyEnter || k == input.KeyNumpadEnter) {
		return keyToIME
	}
	if printable {
		return keyAndText
	}
	return keyOnly
}
