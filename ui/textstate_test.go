package ui

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

func TestEditTextStateWindow(t *testing.T) {
	h := input.TextHints{Kind: input.TextEmail}
	st := editTextState([]rune("héllo"), 2, 2, 2, "", 0, h)
	want := input.TextInputState{Hints: h, Text: "héllo", SelStart: 2, SelEnd: 2, CompStart: -1, CompEnd: -1}
	if st != want {
		t.Fatalf("short text: %+v", st)
	}
	// Long text: at most TextStateWindow runes on each side.
	long := []rune(strings.Repeat("a", 1000) + strings.Repeat("b", 1000))
	st = editTextState(long, 1000, 1010, 1010, "", 0, h)
	n := input.TextStateWindow
	if st.Offset != 1000-n || len([]rune(st.Text)) != 10+2*n || st.SelStart != n || st.SelEnd != n+10 {
		t.Fatalf("window: offset %d len %d sel %d..%d", st.Offset, len([]rune(st.Text)), st.SelStart, st.SelEnd)
	}
	// Reversed selection and out-of-range indices are normalized.
	st = editTextState([]rune("abc"), 9, 1, 1, "", 0, h)
	if st.SelStart != 1 || st.SelEnd != 3 {
		t.Fatalf("normalized selection %d..%d", st.SelStart, st.SelEnd)
	}
	// A composition is part of the text, with the caret inside it.
	st = editTextState([]rune("ab"), 1, 1, 1, "ni", 1, h)
	if st.Text != "anib" || st.CompStart != 1 || st.CompEnd != 3 || st.SelStart != 2 || st.SelEnd != 2 || !st.HasComposition() {
		t.Fatalf("composition: %+v", st)
	}
}

func TestConvertTextEdits(t *testing.T) {
	bs := func(n int) []input.Event {
		var out []input.Event
		for range n {
			out = append(out, input.KeyEvent{Key: input.KeyBackspace, Down: true}, input.KeyEvent{Key: input.KeyBackspace})
		}
		return out
	}
	del := func(n int) []input.Event {
		var out []input.Event
		for range n {
			out = append(out, input.KeyEvent{Key: input.KeyDelete, Down: true}, input.KeyEvent{Key: input.KeyDelete})
		}
		return out
	}
	cat := func(parts ...[]input.Event) []input.Event {
		var out []input.Event
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	text := func(s string) []input.Event { return []input.Event{input.TextInput{Text: s}} }
	// Caret at field position 13 (Offset 10 + 3).
	st := input.TextInputState{Offset: 10, SelStart: 3, SelEnd: 3}
	tests := []struct {
		name string
		in   []input.Event
		want []input.Event
	}{
		{"autocorrect before the caret", []input.Event{input.TextEdit{Start: 10, End: 13, Text: "the"}}, cat(bs(3), text("the"))},
		{"delete surrounding", []input.Event{input.TextEdit{Start: 12, End: 15}}, cat(bs(1), del(2))},
		{"after the caret", []input.Event{input.TextEdit{Start: 13, End: 14, Text: "X"}}, cat(del(1), text("X"))},
		{"away from the caret", []input.Event{input.TextEdit{Start: 2, End: 4, Text: "x"}}, nil},
		{"selection dropped", []input.Event{input.SelectionChange{Start: 1, End: 1}, input.TextInput{Text: "a"}}, text("a")},
		{"two edits in a row", []input.Event{input.TextEdit{Start: 12, End: 13}, input.TextEdit{Start: 11, End: 12, Text: "yz"}},
			cat(bs(1), bs(1), text("yz"))},
	}
	for _, tt := range tests {
		got := convertTextEdits(tt.in, st, true)
		if len(got) != len(tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("%s: event %d %v, want %v", tt.name, i, got[i], tt.want[i])
			}
		}
	}
	// A selection that the edit covers is removed first.
	sel := input.TextInputState{SelStart: 2, SelEnd: 4}
	got := convertTextEdits([]input.Event{input.TextEdit{Start: 1, End: 5, Text: "Q"}}, sel, true)
	want := cat(bs(1), bs(1), del(1), text("Q"))
	if len(got) != len(want) {
		t.Fatalf("selection: %v", got)
	}
	// Without a state the edit cannot be placed.
	if got := convertTextEdits([]input.Event{input.TextEdit{Start: 0, End: 1}}, input.TextInputState{}, false); len(got) != 0 {
		t.Fatalf("no state: %v", got)
	}
}

// stateWin records SetTextInput and SetTextInputState calls.
type stateWin struct {
	damageWin
	calls  []string
	textOn bool
	st     input.TextInputState
	states int
}

func (w *stateWin) SetTextInput(on bool, caret geom.Rect) {
	w.textOn = on
	if on {
		w.calls = append(w.calls, "on")
	} else {
		w.calls = append(w.calls, "off")
	}
}

func (w *stateWin) SetTextInputState(st input.TextInputState) {
	w.calls = append(w.calls, "state")
	w.st = st
	w.states++
}

// The Runner sends the focused field's state after SetTextInput, only when
// it changed, and resets the platform when focus moves to a text widget
// without a state.
func TestTextInputStateSent(t *testing.T) {
	var a, b string = "abc", ""
	var ida, idb ID
	plain := false // second widget: a key-only text widget
	page := func(c *Context) {
		Column(c, func() {
			ida = Input(c, &a).Kind(input.TextEmail).EnterKey(input.EnterNext).ID()
			if plain {
				box := Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 100, H: 30} }, func(l *paint.List, r geom.Rect) {})
				box.Focusable(input.CursorText)
				idb = box.n.id
				if c.Focused() == idb {
					c.WantTextInput(geom.Rect{X: 1, Y: 40, W: 1, H: 10})
				}
			} else {
				idb = Input(c, &b).ID()
			}
		})
	}
	w := &stateWin{damageWin: *newWin()}
	g := newDamageRig(t, page, w, RunnerOptions{NoAnimations: true})
	g.frame(t)
	g.r.c.SetFocus(ida)
	g.frame(t)
	g.frame(t)
	if len(w.calls) < 2 || w.calls[0] != "on" || w.calls[1] != "state" {
		t.Fatalf("calls %v, want on then state", w.calls)
	}
	h := w.st.Hints
	if w.st.Text != "abc" || h.Kind != input.TextEmail || h.Enter != input.EnterNext || h.Multiline {
		t.Fatalf("state %+v", w.st)
	}
	n := w.states
	g.frame(t)
	g.frame(t)
	if w.states != n {
		t.Fatalf("unchanged state sent again (%d calls)", w.states-n)
	}
	g.r.HandleEvent(input.KeyEvent{Key: input.KeyEnd, Down: true}) // focus selected all
	g.frame(t)
	g.r.HandleEvent(input.TextInput{Text: "d"})
	g.frame(t)
	if w.st.Text != "abcd" || w.st.SelStart != 4 || w.states != n+2 {
		t.Fatalf("after typing: %+v (%d sends)", w.st, w.states-n)
	}

	// Focus moves to the other Input: its state replaces the old one.
	g.r.c.SetFocus(idb)
	g.frame(t)
	g.frame(t)
	if w.st.Text != "" || w.st.Hints.Kind != input.TextPlain {
		t.Fatalf("second field state %+v", w.st)
	}

	// A key-only text widget: text input is turned off and on again.
	plain = true
	g.r.c.SetFocus(ida)
	g.frame(t)
	g.frame(t)
	g.r.c.SetFocus(idb)
	w.calls = nil
	g.frame(t)
	g.frame(t)
	if strings.Join(w.calls, " ") != "off on" {
		t.Fatalf("calls %v, want off on", w.calls)
	}
	if _, ok := (&Driver{r: g.r}).TextInputState(); ok {
		t.Fatal("state still reported for a widget without one")
	}

	// Focus leaves: text input off, no state.
	g.r.c.SetFocus(0)
	w.calls = nil
	g.frame(t)
	if strings.Join(w.calls, " ") != "off" {
		t.Fatalf("calls %v, want off", w.calls)
	}
}

// focusInput builds a Driver page with one widget and focuses it.
func focusedDriver(t *testing.T, build func(c *Context) ID) (*Driver, *ID) {
	t.Helper()
	var id ID
	d := NewDriver(func(c *Context) {
		Column(c, func() { id = build(c) }, layout.Padding(geom.All(20)))
	}, DriverOptions{Width: 400, Height: 300})
	d.Frame()
	d.r.c.SetFocus(id)
	d.Frame()
	d.Frame()
	return d, &id
}

func TestEditorHints(t *testing.T) {
	var s string
	var n float64
	tests := []struct {
		name  string
		build func(c *Context) ID
		want  input.TextHints
		text  string
	}{
		{"input", func(c *Context) ID { return Input(c, &s).ID() }, input.TextHints{}, "pw12"},
		{"email", func(c *Context) ID { return Input(c, &s).Kind(input.TextEmail).ID() }, input.TextHints{Kind: input.TextEmail}, "pw12"},
		{"url go", func(c *Context) ID { return Input(c, &s).Kind(input.TextURL).EnterKey(input.EnterGo).ID() },
			input.TextHints{Kind: input.TextURL, Enter: input.EnterGo}, "pw12"},
		{"password", func(c *Context) ID { return Input(c, &s).Password().ID() },
			input.TextHints{Kind: input.TextPassword, NoAutocorrect: true}, "••••"},
		{"textarea", func(c *Context) ID { return TextArea(c, &s).ID() },
			input.TextHints{Multiline: true, Enter: input.EnterNewline}, "pw12"},
		{"number", func(c *Context) ID { return InputNumber(c, &n).ed.id }, input.TextHints{Kind: input.TextDecimal}, ""},
	}
	for _, tt := range tests {
		s, n = "pw12", 0
		d, _ := focusedDriver(t, tt.build)
		st, ok := d.TextInputState()
		if !ok {
			t.Errorf("%s: no state", tt.name)
			continue
		}
		if st.Hints != tt.want {
			t.Errorf("%s: hints %+v, want %+v", tt.name, st.Hints, tt.want)
		}
		if tt.text != "" && st.Text != tt.text {
			t.Errorf("%s: text %q, want %q", tt.name, st.Text, tt.text)
		}
	}
}

// The built-in editor applies TextEdit and SelectionChange itself, in
// order with keys and text.
func TestEditorTextEdit(t *testing.T) {
	s := "teh"
	d, _ := focusedDriver(t, func(c *Context) ID { return Input(c, &s).ID() })
	d.Key(input.KeyEnd, 0)
	d.Frame()
	// Autocorrection: replace the word, then the space typed after it.
	d.Inject(input.TextEdit{Start: 0, End: 3, Text: "the"})
	d.Inject(input.TextInput{Text: " "})
	d.Frame()
	if s != "the " {
		t.Fatalf("after autocorrect %q", s)
	}
	st, _ := d.TextInputState()
	if st.SelStart != 4 || st.SelEnd != 4 {
		t.Fatalf("caret %d..%d, want 4", st.SelStart, st.SelEnd)
	}
	// deleteSurroundingText(1, 0) then commit: order matters.
	d.Inject(input.TextEdit{Start: 3, End: 4})
	d.Inject(input.TextInput{Text: "!"})
	d.Frame()
	if s != "the!" {
		t.Fatalf("delete then commit %q", s)
	}
	// Move the caret, then type.
	d.Inject(input.SelectionChange{Start: 1, End: 1})
	d.Inject(input.TextInput{Text: "X"})
	d.Frame()
	if s != "tXhe!" {
		t.Fatalf("selection change then type %q", s)
	}
	// Select a range: the state reports it.
	d.Inject(input.SelectionChange{Start: 0, End: 2})
	d.Frame()
	d.Frame()
	if st, _ := d.TextInputState(); st.SelStart != 0 || st.SelEnd != 2 {
		t.Fatalf("selection %d..%d", st.SelStart, st.SelEnd)
	}
	// Out-of-range edits are clamped; single-line fields drop newlines.
	d.Inject(input.TextEdit{Start: 3, End: 99, Text: "a\nb"})
	d.Frame()
	if s != "tXhab" {
		t.Fatalf("clamped edit %q", s)
	}
}

// A focused widget that does not take TextEdit events gets Backspace /
// Delete presses and text instead; SelectionChange is dropped.
func TestTextEditFallback(t *testing.T) {
	var got []input.Event
	var id ID
	val := []rune("hello")
	page := func(c *Context) {
		box := Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 100, H: 30} }, func(l *paint.List, r geom.Rect) {
			if c.Focused() == id {
				c.WantTextInputState(geom.Rect{X: r.X, Y: r.Y, W: 1, H: r.H},
					input.TextInputState{Text: string(val), SelStart: len(val), SelEnd: len(val), CompStart: -1, CompEnd: -1})
			}
		})
		box.Focusable(input.CursorText)
		id = box.n.id
		for _, e := range c.EventsFor(id) {
			got = append(got, e)
		}
	}
	d := NewDriver(page, DriverOptions{Width: 300, Height: 200})
	d.Frame()
	d.r.c.SetFocus(id)
	d.Frame()
	d.Frame()
	got = nil
	d.Inject(input.SelectionChange{Start: 1, End: 1})
	d.Inject(input.TextEdit{Start: 2, End: 5, Text: "LP"})
	d.Frame()
	var desc []string
	for _, e := range got {
		switch e := e.(type) {
		case input.KeyEvent:
			if e.Down {
				desc = append(desc, e.Key.String())
			}
		case input.TextInput:
			desc = append(desc, "text:"+e.Text)
		default:
			desc = append(desc, "other")
		}
	}
	want := []string{input.KeyBackspace.String(), input.KeyBackspace.String(), input.KeyBackspace.String(), "text:LP"}
	if strings.Join(desc, ",") != strings.Join(want, ",") {
		t.Fatalf("events %v, want %v", desc, want)
	}
}
