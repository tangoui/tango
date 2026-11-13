package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Text-input context for platform input methods (docs/specs/mobile-canvas.md
// §2.3, M3b). A focused text field that knows its text describes the
// surroundings of its caret with WantTextInputState; the Runner sends the
// state to windows implementing TextInputStateWindow after each frame in
// which it changed, right after SetTextInput. Input methods that edit the
// field directly (autocorrection, deleteSurroundingText, moving the caret)
// answer with input.TextEdit and input.SelectionChange events.
//
// A field that applies those events itself takes them with TextEditsFor
// (the built-in editor of Input, TextArea and friends does). For any other
// focused widget the Runner turns a TextEdit next to the caret into
// Backspace / Delete key presses plus a TextInput, and drops
// SelectionChange, so widgets that only understand keys keep working.

// TextInputStateWindow is implemented by windows whose input method can
// use the text around the caret. SetTextInputState is called after
// SetTextInput(true, …) whenever the focused field's state changes. When
// focus moves to a text widget without a state, the Runner turns text
// input off and on again so the platform forgets the old state.
type TextInputStateWindow interface {
	SetTextInputState(st input.TextInputState)
}

// imeState is the Context side of the text-input state.
type imeState struct {
	want  bool                 // WantTextInputState called this frame
	st    input.TextInputState // its argument
	edits []input.Event        // TextEdit / SelectionChange of this frame
	aware ID                   // focused widget that took edits (TextEditsFor) this frame
}

// WantTextInputState is WantTextInput for a field that describes its text
// to the input method: st carries the text around the selection (rune
// offsets, see input.TextInputState) and the field's hints. Call it every
// frame the field has focus, like WantTextInput.
func (c *Context) WantTextInputState(caret geom.Rect, st input.TextInputState) {
	c.WantTextInput(caret)
	c.ime.want, c.ime.st = true, st
}

// TextEditsFor returns, in arrival order, the input.TextEdit and
// input.SelectionChange events of this frame for widget id while it has
// focus, and records that id applies them itself: from the next frame on
// they reach it unconverted. Like KeysFor it hands them over once.
func (c *Context) TextEditsFor(id ID) []input.Event {
	if id == 0 || c.focus != id {
		return nil
	}
	c.ime.aware = id
	ev := c.ime.edits
	c.ime.edits = nil
	return ev
}

// sendTextInput reports this frame's text-input request and state to the
// window (afterFrame).
func (r *Runner) sendTextInput() {
	c := r.c
	sw, hasState := r.win.(TextInputStateWindow)
	switch {
	case !c.wantText:
		r.stateOn = false
	case !c.ime.want && r.stateOn:
		// The focused text widget has no state: reset the platform to its
		// context-free input method.
		if r.textOn {
			r.win.SetTextInput(false, r.lastCaret)
			r.textOn = false
		}
		r.stateOn = false
	}
	if c.wantText != r.textOn || (c.wantText && c.textCaret != r.lastCaret) {
		r.win.SetTextInput(c.wantText, c.textCaret)
		r.textOn, r.lastCaret = c.wantText, c.textCaret
	}
	if c.wantText && c.ime.want && (!r.stateOn || c.ime.st != r.lastState || c.focus != r.stateFocus) {
		r.lastState, r.stateOn, r.stateFocus = c.ime.st, true, c.focus
		if hasState {
			sw.SetTextInputState(c.ime.st)
		}
	}
}

// imeEdits routes this frame's TextEdit / SelectionChange events: kept for
// a focused widget that took them last frame (TextEditsFor), otherwise
// converted to key presses and text, or dropped (convertTextEdits).
func (r *Runner) imeEdits(q []input.Event) []input.Event {
	has := false
	for _, e := range q {
		switch e.(type) {
		case input.TextEdit, input.SelectionChange:
			has = true
		}
	}
	if !has || (r.editAware != 0 && r.editAware == r.c.focus) {
		return q
	}
	return convertTextEdits(q, r.lastState, r.stateOn)
}

// convertTextEdits rewrites TextEdit events for a widget that only knows
// keys and text: the runes between the edit's start and the selection go
// with Backspace, those between the selection and the edit's end with
// Delete, then the replacement is typed. st is the state last sent to the
// window (ok false: none, the edit is dropped). Edits that do not touch the
// selection, and every SelectionChange, are dropped.
func convertTextEdits(q []input.Event, st input.TextInputState, ok bool) []input.Event {
	out := make([]input.Event, 0, len(q))
	a, b := st.Offset+st.SelStart, st.Offset+st.SelEnd
	if a > b {
		a, b = b, a
	}
	press := func(k input.Key, n int) {
		for range n {
			out = append(out, input.KeyEvent{Key: k, Down: true}, input.KeyEvent{Key: k})
		}
	}
	for _, e := range q {
		switch ev := e.(type) {
		case input.SelectionChange:
			continue
		case input.TextEdit:
			s, t := min(ev.Start, ev.End), max(ev.Start, ev.End)
			if !ok || s > a || t < b {
				continue
			}
			if a < b {
				press(input.KeyBackspace, 1) // the selection
			}
			press(input.KeyBackspace, a-s)
			press(input.KeyDelete, t-b)
			if ev.Text != "" {
				out = append(out, input.TextInput{Text: ev.Text})
			}
			a = s + len([]rune(ev.Text))
			b = a
			continue
		}
		out = append(out, e)
	}
	return out
}

// editTextState builds the TextInputState of a text editor: rs is the
// field's text (masked for passwords), [selA, selB) the selection, comp
// the composition shown at the caret cur (compCur runes into it).
func editTextState(rs []rune, selA, selB, cur int, comp string, compCur int, h input.TextHints) input.TextInputState {
	n := len(rs)
	selA, selB, cur = clampIdx(selA, n), clampIdx(selB, n), clampIdx(cur, n)
	if selA > selB {
		selA, selB = selB, selA
	}
	cs, ce := -1, -1
	doc := rs
	if comp != "" {
		cr := []rune(comp)
		doc = make([]rune, 0, n+len(cr))
		doc = append(doc, rs[:cur]...)
		doc = append(doc, cr...)
		doc = append(doc, rs[cur:]...)
		cs, ce = cur, cur+len(cr)
		selA = cur + clampIdx(compCur, len(cr))
		selB = selA
	}
	lo := max(0, selA-input.TextStateWindow)
	hi := min(len(doc), selB+input.TextStateWindow)
	if cs >= 0 {
		lo, hi = min(lo, cs), max(hi, ce)
	}
	st := input.TextInputState{
		Hints:     h,
		Text:      string(doc[lo:hi]),
		Offset:    lo,
		SelStart:  selA - lo,
		SelEnd:    selB - lo,
		CompStart: -1,
		CompEnd:   -1,
	}
	if cs >= 0 {
		st.CompStart, st.CompEnd = cs-lo, ce-lo
	}
	return st
}

// TextInputState returns the text-input state the Runner last reported
// and whether one is in effect (tests).
func (d *Driver) TextInputState() (input.TextInputState, bool) {
	return d.r.lastState, d.r.stateOn && d.r.textOn
}
