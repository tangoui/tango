package ios

// The document behind the canvas view's UITextInput (docs/specs/mobile-canvas.md
// §2.3). UIKit asks a text view many questions per keystroke (the text in
// a range, the selection, the marked text); canvas_text.m answers them
// from a mirror of this document and calls Go only for edits. Every edit
// goes through textDoc, which returns the input events for the sink and
// leaves the new mirror in mirror().
//
// Without a TextInputState from the focused widget (M3a) the document is
// mobilecanvas.IMEDoc's: only the marked text. With one (M3b) it is the
// widget's text window with the composition, which package ui keeps out
// of the widget's text until it is committed, spliced in place of the
// selection. Edits UIKit makes to committed text become input.TextEdit
// and input.SelectionChange; typing at the caret stays input.TextInput so
// widgets that only know the M3a events still work. Each edit is also
// applied to the local copy of the widget's text, so the mirror is right
// before the widget's next state arrives (one frame later).
//
// Kept free of build tags so it is tested on any host.

import (
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/internal/platform/mobilecanvas"
	"github.com/tangoui/tango/ui/input"
)

// textMirror is what canvas_text.m holds: the document text and its
// selection and marked ranges in UTF-16 units.
type textMirror struct {
	Text string
	Sel  mobilecanvas.Range
	Mark mobilecanvas.Range // Start < 0: no marked text
}

func (m textMirror) marked() bool { return m.Mark.Start >= 0 }

// textDoc is the input-method document of one canvas. The zero value is
// an empty M3a document. Main thread.
type textDoc struct {
	ime   mobilecanvas.IMEDoc
	state bool // the focused widget describes its text (M3b)
	hints input.TextHints
	off   int    // field position of base's first rune
	base  string // the widget's text window, without the composition
	s, e  int    // selection in base (runes); the composition replaces it in the mirror
}

// reset empties the document: text input turned on or off.
func (d *textDoc) reset() { *d = textDoc{} }

// mirror returns the document as UIKit sees it.
func (d *textDoc) mirror() textMirror {
	if !d.ime.Marked() {
		pre := d.base[:mobilecanvas.RuneToByte(d.base, d.s)]
		sel := d.base[len(pre):mobilecanvas.RuneToByte(d.base, d.e)]
		return textMirror{Text: d.base, Sel: mobilecanvas.Range{Start: mobilecanvas.UTF16Len(pre), Len: mobilecanvas.UTF16Len(sel)}, Mark: mobilecanvas.Range{Start: -1}}
	}
	snap := d.ime.Snapshot()
	pre := d.base[:mobilecanvas.RuneToByte(d.base, d.s)]
	post := d.base[mobilecanvas.RuneToByte(d.base, d.e):]
	p := mobilecanvas.UTF16Len(pre)
	return textMirror{
		Text: pre + snap.Marked + post,
		Sel:  mobilecanvas.Range{Start: p + snap.Sel.Start, Len: snap.Sel.Len},
		Mark: mobilecanvas.Range{Start: p, Len: snap.Len},
	}
}

// field converts a UTF-16 offset into the mirror to a field position.
// Offsets inside the composition map to where it sits.
func (d *textDoc) field(u int) int {
	m := d.mirror()
	r := mobilecanvas.UTF16ToRune(m.Text, u)
	if m.marked() {
		ml := utf8.RuneCountInString(d.ime.Snapshot().Marked)
		switch {
		case r >= d.s+ml:
			r = r - ml + (d.e - d.s)
		case r > d.s:
			r = d.s
		}
	}
	return d.off + r
}

// setState takes the focused widget's state. It reports whether the
// mirror's text and its selection changed, so the view can tell UIKit
// (the input delegate) about changes it did not make itself.
func (d *textDoc) setState(st input.TextInputState) (textChanged, selChanged bool) {
	old := d.mirror()
	d.state, d.hints, d.off = true, st.Hints, max(st.Offset, 0)
	text := strings.ToValidUTF8(st.Text, "�")
	n := utf8.RuneCountInString(text)
	clamp := func(i int) int { return min(max(i, 0), n) }
	s, e := clamp(st.SelStart), clamp(st.SelEnd)
	if e < s {
		s, e = e, s
	}
	if st.CompStart >= 0 && st.CompEnd > st.CompStart {
		// The widget shows the composition inside its text: take it out
		// of base and make it the input method's marked text. (An empty
		// region, as in a zero state, is no composition.)
		cs, ce := clamp(st.CompStart), clamp(st.CompEnd)
		bs, be := mobilecanvas.RuneToByte(text, cs), mobilecanvas.RuneToByte(text, ce)
		comp := text[bs:be]
		d.base = text[:bs] + text[be:]
		in := func(i int) int { return min(max(i, cs), ce) - cs }
		sel := mobilecanvas.Range{Start: mobilecanvas.RuneToUTF16(comp, in(s))}
		sel.Len = mobilecanvas.RuneToUTF16(comp, in(e)) - sel.Start
		d.s, d.e = cs, cs
		if comp != d.ime.Snapshot().Marked || sel != d.ime.Snapshot().Sel {
			d.ime.SetMarked(comp, sel) // the widget already has it: no events
		}
	} else {
		// A composition in progress stays: package ui draws it over the
		// widget, which does not report it.
		d.base, d.s, d.e = text, s, e
	}
	now := d.mirror()
	return now.Text != old.Text || now.Mark != old.Mark, now.Sel != old.Sel
}

// clearState returns to the M3a document: text input moved to a widget
// that does not describe its text.
func (d *textDoc) clearState() {
	marked := d.ime
	*d = textDoc{}
	d.ime = marked
}

// apply replaces base runes [s, e) with text in the local copy and puts
// the caret after it (what the widget does with the events sent).
func (d *textDoc) apply(s, e int, text string) {
	if !d.state {
		return
	}
	text = d.fieldText(text)
	n := utf8.RuneCountInString(d.base)
	s, e = min(max(s, 0), n), min(max(e, 0), n)
	if e < s {
		s, e = e, s
	}
	d.base = d.base[:mobilecanvas.RuneToByte(d.base, s)] + text + d.base[mobilecanvas.RuneToByte(d.base, e):]
	d.s = s + utf8.RuneCountInString(text)
	d.e = d.s
}

// fieldText is text as the widget stores it: line breaks only in
// multi-line fields (a single-line field takes them as Enter).
func (d *textDoc) fieldText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if !d.hints.Multiline {
		text = strings.ReplaceAll(text, "\n", "")
	}
	return strings.Map(func(r rune) rune {
		if r != '\n' && (r < 0x20 || r == 0x7f) {
			return -1
		}
		return r
	}, text)
}

// setMarked is setMarkedText:selectedRange:.
func (d *textDoc) setMarked(text string, sel mobilecanvas.Range) []input.Event {
	evs, _ := d.ime.SetMarked(text, sel)
	return evs
}

// insert is insertText: (and the commit part of other edits): text
// replaces the composition, or else the selection.
func (d *textDoc) insert(text string) []input.Event {
	evs, _ := d.ime.Insert(text)
	d.apply(d.s, d.e, text)
	return evs
}

// unmark is unmarkText: the composition is committed as it stands.
func (d *textDoc) unmark() []input.Event {
	marked := d.ime.Snapshot().Marked
	evs, _ := d.ime.Unmark()
	if marked != "" {
		d.apply(d.s, d.e, marked)
	}
	return evs
}

// deleteBackward is deleteBackward: one character of the composition, or
// a Backspace press for the widget.
func (d *textDoc) deleteBackward() []input.Event {
	if d.ime.Marked() {
		evs, _ := d.ime.DeleteBackward(1)
		return evs
	}
	evs, _ := d.ime.DeleteBackward(1)
	if d.s < d.e {
		d.apply(d.s, d.e, "")
	} else if d.s > 0 {
		d.apply(mobilecanvas.PrevBoundary(d.base, d.s), d.s, "")
	}
	return evs
}

// replace is replaceRange:withText: on the mirror range r (UTF-16): an
// autocorrection or a suggestion taken. Replacing the selection is
// typing; any other range of committed text becomes a TextEdit.
func (d *textDoc) replace(r mobilecanvas.Range, text string) []input.Event {
	m := d.mirror()
	if m.marked() {
		if r == m.Mark {
			return d.insert(text)
		}
		if d.inMark(m, r) {
			cur := d.ime.Snapshot().Marked
			rel := mobilecanvas.Range{Start: r.Start - m.Mark.Start, Len: r.Len}
			next := mobilecanvas.SliceUTF16(cur, 0, rel.Start) + text + mobilecanvas.SliceUTF16(cur, rel.End(), m.Mark.Len)
			caret := rel.Start + mobilecanvas.UTF16Len(text)
			return d.setMarked(next, mobilecanvas.Range{Start: caret})
		}
	}
	var evs []input.Event
	if m.marked() {
		evs = d.unmark()
		m = d.mirror()
	}
	if !d.state || r == m.Sel {
		return append(evs, d.insert(text)...)
	}
	a, b := d.field(r.Start), d.field(r.End())
	if b < a {
		a, b = b, a
	}
	text = d.fieldText(text)
	evs = append(evs, input.TextEdit{Start: a, End: b, Text: text})
	d.apply(a-d.off, b-d.off, text)
	return evs
}

func (d *textDoc) inMark(m textMirror, r mobilecanvas.Range) bool {
	return r.Start >= m.Mark.Start && r.End() <= m.Mark.End() && r.Len >= 0
}

// selectRange is setSelectedTextRange: with the mirror range r (UTF-16).
// Inside the composition it moves the composition's caret; on committed
// text of a widget with state it becomes a SelectionChange. The M3a
// document has nothing else to select.
func (d *textDoc) selectRange(r mobilecanvas.Range) []input.Event {
	m := d.mirror()
	if r.Len < 0 {
		r = mobilecanvas.Range{Start: r.End(), Len: -r.Len}
	}
	if r == m.Sel {
		return nil
	}
	if m.marked() {
		if d.inMark(m, r) {
			cur := d.ime.Snapshot().Marked
			return d.setMarked(cur, mobilecanvas.Range{Start: r.Start - m.Mark.Start, Len: r.Len})
		}
		if !d.state {
			return nil
		}
		// Leaving the composition commits it first.
		evs := d.unmark()
		return append(evs, d.selectRange(r)...)
	}
	if !d.state {
		return nil
	}
	a, b := d.field(r.Start), d.field(r.End())
	d.s, d.e = a-d.off, b-d.off
	return []input.Event{input.SelectionChange{Start: a, End: b}}
}
