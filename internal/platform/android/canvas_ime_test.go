package android

import (
	"reflect"
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestEditorInfo(t *testing.T) {
	const noX = imeFlagNoExtractUI | imeFlagNoFullscreen
	for _, tc := range []struct {
		name      string
		h         input.TextHints
		typ, opts int
	}{
		{"plain default", input.TextHints{}, itClassText | itTextAutoCorrect, noX | imeActionDone},
		{"no autocorrect", input.TextHints{NoAutocorrect: true}, itClassText | itTextNoSuggestions, noX | imeActionDone},
		{"sentences", input.TextHints{Capitalize: input.CapitalizeSentences}, itClassText | itTextAutoCorrect | itTextCapSentences, noX | imeActionDone},
		{"words", input.TextHints{Capitalize: input.CapitalizeWords}, itClassText | itTextAutoCorrect | itTextCapWords, noX | imeActionDone},
		{"multiline breaks lines", input.TextHints{Multiline: true}, itClassText | itTextAutoCorrect | itTextMultiLine, noX | imeFlagNoEnterAction},
		{"multiline send", input.TextHints{Multiline: true, Enter: input.EnterSend}, itClassText | itTextAutoCorrect | itTextMultiLine, noX | imeActionSend},
		{"number", input.TextHints{Kind: input.TextNumber}, itClassNumber, noX | imeActionDone},
		{"decimal next", input.TextHints{Kind: input.TextDecimal, Enter: input.EnterNext}, itClassNumber | itNumberDecimal, noX | imeActionNext},
		{"phone", input.TextHints{Kind: input.TextPhone}, itClassPhone, noX | imeActionDone},
		{"email", input.TextHints{Kind: input.TextEmail, Capitalize: input.CapitalizeWords}, itClassText | itTextVariationEmail | itTextAutoCorrect, noX | imeActionDone},
		{"url goes", input.TextHints{Kind: input.TextURL}, itClassText | itTextVariationURI | itTextAutoCorrect, noX | imeActionGo},
		{"password", input.TextHints{Kind: input.TextPassword, Multiline: true}, itClassText | itTextVariationPass | itTextNoSuggestions, noX | imeFlagNoPersonalizedLearning | imeActionDone},
		{"search", input.TextHints{Kind: input.TextSearch}, itClassText | itTextAutoCorrect, noX | imeActionSearch},
		{"go", input.TextHints{Enter: input.EnterGo}, itClassText | itTextAutoCorrect, noX | imeActionGo},
		{"newline", input.TextHints{Enter: input.EnterNewline}, itClassText | itTextAutoCorrect, noX | imeActionNone | imeFlagNoEnterAction},
	} {
		typ, opts := editorInfo(tc.h)
		if typ != tc.typ || opts != tc.opts {
			t.Errorf("%s: editorInfo(%+v) = %#x, %#x; want %#x, %#x", tc.name, tc.h, typ, opts, tc.typ, tc.opts)
		}
	}
}

func key(k input.Key) []input.Event {
	return []input.Event{input.KeyEvent{Key: k, Down: true}, input.KeyEvent{Key: k}}
}

func evs(e ...any) []input.Event {
	var out []input.Event
	for _, x := range e {
		switch v := x.(type) {
		case []input.Event:
			out = append(out, v...)
		case input.Event:
			out = append(out, v)
		}
	}
	return out
}

func checkEvs(t *testing.T, what string, got, want []input.Event) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: events\n got %#v\nwant %#v", what, got, want)
	}
}

func checkMirror(t *testing.T, what string, s *imeSession, text string, base, ss, se, cs, ce int) {
	t.Helper()
	gt, gb, gss, gse, gcs, gce := s.m.java()
	if gt != text || gb != base || gss != ss || gse != se || gcs != cs || gce != ce {
		t.Errorf("%s: mirror %q base %d sel %d-%d comp %d-%d; want %q base %d sel %d-%d comp %d-%d",
			what, gt, gb, gss, gse, gcs, gce, text, base, ss, se, cs, ce)
	}
}

// Without a widget state (M3a) the mirror holds only the pre-edit.
func TestIMEShadow(t *testing.T) {
	var s imeSession
	s.reset(false)
	checkMirror(t, "empty", &s, "", 0, 0, 0, -1, -1)

	checkEvs(t, "compose", s.composing("ni", 1), evs(input.Composition{Text: "ni", Cursor: 2}))
	checkMirror(t, "compose", &s, "ni", 0, 2, 2, 0, 2)

	checkEvs(t, "compose more", s.composing("nih😀", 1), evs(input.Composition{Text: "nih😀", Cursor: 4}))
	checkMirror(t, "compose more", &s, "nih😀", 0, 5, 5, 0, 5)

	checkEvs(t, "backspace in pre-edit", s.backspace(), evs(input.Composition{Text: "nih", Cursor: 3}))
	checkMirror(t, "backspace in pre-edit", &s, "nih", 0, 3, 3, 0, 3)

	checkEvs(t, "commit", s.commit("你好", 1), evs(input.TextInput{Text: "你好"}, input.Composition{}))
	checkMirror(t, "commit", &s, "", 0, 0, 0, -1, -1)

	checkEvs(t, "backspace", s.backspace(), key(input.KeyBackspace))
	checkEvs(t, "delete around", s.deleteAround(2, 1, false), evs(key(input.KeyBackspace), key(input.KeyBackspace), key(input.KeyDelete)))

	s.composing("abc", 1)
	checkEvs(t, "finish", s.finish(), evs(input.TextInput{Text: "abc"}, input.Composition{}))
	checkEvs(t, "finish again", s.finish(), nil)

	s.composing("x", 1)
	checkEvs(t, "enter", s.enter(), evs(input.TextInput{Text: "x"}, input.Composition{}, key(input.KeyEnter)))
	checkEvs(t, "commit with line break", s.commit("a\r\nb", 1), evs(input.TextInput{Text: "a"}, key(input.KeyEnter), input.TextInput{Text: "b"}))

	// Context-only calls are ignored without context.
	checkEvs(t, "region", s.composeRegion(0, 1), nil)
	checkEvs(t, "selection", s.selection(0, 1), nil)

	s.composing("q", 1)
	checkEvs(t, "reset cancels", s.reset(true), evs(input.Composition{}))
	checkEvs(t, "typed", s.typed("z"), evs(input.TextInput{Text: "z"}))
}

func withState(text string, offset, sel int, h input.TextHints) *imeSession {
	s := &imeSession{}
	s.reset(false)
	s.setState(input.TextInputState{Hints: h, Text: text, Offset: offset, SelStart: sel, SelEnd: sel, CompStart: -1, CompEnd: -1})
	return s
}

func TestIMEStateRestart(t *testing.T) {
	var s imeSession
	s.reset(false)
	st := input.TextInputState{Text: "hello", SelStart: 5, SelEnd: 5, CompStart: -1, CompEnd: -1}
	if _, r := s.setState(st); !r {
		t.Error("first state does not restart")
	}
	checkMirror(t, "state", &s, "hello", 0, 5, 5, -1, -1)
	if _, r := s.setState(st); !r {
		t.Error("the same state again (focus moved to a twin field) does not restart")
	}
	st.SelStart, st.SelEnd = 1, 3
	if _, r := s.setState(st); r {
		t.Error("selection change restarts")
	}
	checkMirror(t, "selection", &s, "hello", 0, 1, 3, -1, -1)
	st.Hints.Kind = input.TextEmail
	if _, r := s.setState(st); !r {
		t.Error("new hints do not restart")
	}
	// Predicted commit, confirmed by the widget: no restart.
	st.SelStart, st.SelEnd = 5, 5
	s.setState(st)
	checkEvs(t, "commit", s.commit("!", 1), evs(input.TextInput{Text: "!"}))
	checkMirror(t, "commit", &s, "hello!", 0, 6, 6, -1, -1)
	if _, r := s.setState(input.TextInputState{Hints: st.Hints, Text: "hello!", SelStart: 6, SelEnd: 6, CompStart: -1, CompEnd: -1}); r {
		t.Error("confirmed prediction restarts")
	}
	// The widget refused the character: restart.
	s.commit("?", 1)
	if _, r := s.setState(input.TextInputState{Hints: st.Hints, Text: "hello!", SelStart: 6, SelEnd: 6, CompStart: -1, CompEnd: -1}); !r {
		t.Error("rejected edit does not restart")
	}
}

func TestIMEStatePassword(t *testing.T) {
	h := input.TextHints{Kind: input.TextPassword}
	s := withState("••", 0, 2, h)
	s.commit("x", 1)
	if _, r := s.setState(input.TextInputState{Hints: h, Text: "•••", SelStart: 3, SelEnd: 3, CompStart: -1, CompEnd: -1}); r {
		t.Error("masked text restarts")
	}
	s.commit("y", 1)
	if _, r := s.setState(input.TextInputState{Hints: h, Text: "•••", SelStart: 3, SelEnd: 3, CompStart: -1, CompEnd: -1}); !r {
		t.Error("rejected character in a password field does not restart")
	}
}

func TestIMEStateWindowSlides(t *testing.T) {
	// A long field: the window starts at field position 100 and slides by
	// one when a character is typed at its end.
	s := withState("abcdef", 100, 6, input.TextHints{})
	s.commit("g", 1)
	checkMirror(t, "predicted", s, "abcdefg", 100, 7, 7, -1, -1)
	if _, r := s.setState(input.TextInputState{Text: "bcdefg", Offset: 101, SelStart: 6, SelEnd: 6, CompStart: -1, CompEnd: -1}); r {
		t.Error("slid window restarts")
	}
	checkMirror(t, "slid", s, "bcdefg", 101, 6, 6, -1, -1)
	// Document positions from the input method are base + UTF-16 offset.
	checkEvs(t, "selection", s.selection(102, 104), evs(input.SelectionChange{Start: 102, End: 104}))
}

func TestIMEStatePreedit(t *testing.T) {
	s := withState("ab", 0, 2, input.TextHints{})
	checkEvs(t, "compose", s.composing("ni", 1), evs(input.Composition{Text: "ni", Cursor: 2}))
	checkMirror(t, "compose", s, "abni", 0, 4, 4, 2, 4)
	// A widget that keeps the pre-edit out of Text: overlaid, no restart.
	if ev, r := s.setState(input.TextInputState{Text: "ab", SelStart: 2, SelEnd: 2, CompStart: -1, CompEnd: -1}); r || ev != nil {
		t.Errorf("state without the pre-edit: restart %v, events %v", r, ev)
	}
	checkMirror(t, "overlay", s, "abni", 0, 4, 4, 2, 4)
	// A widget that includes it.
	s.composing("ni", 1)
	if _, r := s.setState(input.TextInputState{Text: "abni", SelStart: 4, SelEnd: 4, CompStart: 2, CompEnd: 4}); r {
		t.Error("state with the pre-edit restarts")
	}
	checkEvs(t, "backspace", s.backspace(), evs(input.Composition{Text: "n", Cursor: 1}))
	checkMirror(t, "backspace", s, "abn", 0, 3, 3, 2, 3)
	checkEvs(t, "commit", s.commit("你", 1), evs(input.TextInput{Text: "你"}, input.Composition{}))
	checkMirror(t, "commit", s, "ab你", 0, 3, 3, -1, -1)

	// A restart while composing cancels the pre-edit.
	s.composing("x", 1)
	ev, r := s.setState(input.TextInputState{Text: "zzz", SelStart: 0, SelEnd: 0, CompStart: -1, CompEnd: -1})
	if !r {
		t.Error("foreign text does not restart")
	}
	checkEvs(t, "restart", ev, evs(input.Composition{}))
	checkMirror(t, "restart", s, "zzz", 0, 0, 0, -1, -1)
	if s.doc.Marked() {
		t.Error("pre-edit survives the restart")
	}
}

func TestIMEStateRegion(t *testing.T) {
	s := withState("teh cat", 10, 3, input.TextHints{})
	checkEvs(t, "region", s.composeRegion(10, 13), nil)
	checkMirror(t, "region", s, "teh cat", 10, 3, 3, 0, 3)
	// The widget's state does not know the region; it survives.
	s.setState(input.TextInputState{Text: "teh cat", Offset: 10, SelStart: 3, SelEnd: 3, CompStart: -1, CompEnd: -1})
	checkMirror(t, "region kept", s, "teh cat", 10, 3, 3, 0, 3)
	// Autocorrection: one edit.
	checkEvs(t, "commit", s.commit("the", 1), evs(input.TextEdit{Start: 10, End: 13, Text: "the"}))
	checkMirror(t, "commit", s, "the cat", 10, 3, 3, -1, -1)

	// Recomposing a picked word edits it in place.
	s.composeRegion(14, 17)
	checkEvs(t, "recompose", s.composing("cats", 1), evs(input.TextEdit{Start: 14, End: 17, Text: "cats"}))
	checkMirror(t, "recompose", s, "the cats", 10, 8, 8, 4, 8)
	if _, r := s.setState(input.TextInputState{Text: "the cats", Offset: 10, SelStart: 8, SelEnd: 8, CompStart: -1, CompEnd: -1}); r {
		t.Error("recomposed word restarts")
	}
	checkMirror(t, "recompose confirmed", s, "the cats", 10, 8, 8, 4, 8)
	checkEvs(t, "commit recomposed", s.commit("cat's", 1), evs(input.TextEdit{Start: 14, End: 18, Text: "cat's"}))
	checkMirror(t, "commit recomposed", s, "the cat's", 10, 9, 9, -1, -1)
	s.composeRegion(14, 19)
	checkEvs(t, "finish", s.finish(), nil)
	checkMirror(t, "finish", s, "the cat's", 10, 9, 9, -1, -1)

	// finish on a region just drops it.
	s.composeRegion(10, 13)
	checkEvs(t, "finish region", s.finish(), nil)
	checkMirror(t, "finish region", s, "the cat's", 10, 9, 9, -1, -1)
}

func TestIMEStateDeleteAround(t *testing.T) {
	s := withState("a😀bcd", 0, 3, input.TextHints{}) // caret after b (UTF-16 4)
	checkEvs(t, "delete", s.deleteAround(3, 1, false), evs(input.TextEdit{Start: 3, End: 4}, input.TextEdit{Start: 1, End: 3}))
	checkMirror(t, "delete", s, "ad", 0, 1, 1, -1, -1)

	s = withState("abcdef", 0, 3, input.TextHints{})
	checkEvs(t, "code points", s.deleteAround(1, 10, true), evs(input.TextEdit{Start: 3, End: 6}, input.TextEdit{Start: 2, End: 3}))
	checkMirror(t, "code points", s, "ab", 0, 2, 2, -1, -1)
}

func TestIMEStateKeys(t *testing.T) {
	s := withState("ab👍🏽", 0, 4, input.TextHints{})
	checkEvs(t, "backspace", s.backspace(), key(input.KeyBackspace))
	checkMirror(t, "backspace removes the whole emoji", s, "ab", 0, 2, 2, -1, -1)
	checkEvs(t, "enter single line", s.enter(), key(input.KeyEnter))
	checkMirror(t, "enter single line", s, "ab", 0, 2, 2, -1, -1)

	s = withState("ab", 0, 1, input.TextHints{Multiline: true})
	s.enter()
	checkMirror(t, "enter multi-line", s, "a\nb", 0, 2, 2, -1, -1)
	s.predictKey(input.KeyDelete)
	checkMirror(t, "delete", s, "a\n", 0, 2, 2, -1, -1)
	s.composing("k", 1)
	checkEvs(t, "typed commits the pre-edit", s.typed("z"), evs(input.TextInput{Text: "k"}, input.Composition{}, input.TextInput{Text: "z"}))
	checkMirror(t, "typed", s, "a\nkz", 0, 4, 4, -1, -1)
}

func TestPlanKey(t *testing.T) {
	for _, tc := range []struct {
		name              string
		textOn, composing bool
		k                 input.Key
		printable         bool
		want              keyPlan
	}{
		{"text off", false, false, input.KeyA, true, keyOnly},
		{"printable", true, false, input.KeyA, true, keyAndText},
		{"printable while composing", true, true, input.KeyA, true, keyAndText},
		{"arrow", true, false, input.KeyLeft, false, keyOnly},
		{"backspace", true, false, input.KeyBackspace, false, keyOnly},
		{"backspace while composing", true, true, input.KeyBackspace, false, keyToIME},
		{"enter while composing", true, true, input.KeyEnter, false, keyToIME},
		{"shortcut", true, false, input.KeyC, false, keyOnly},
	} {
		if got := planKey(tc.textOn, tc.composing, tc.k, tc.printable); got != tc.want {
			t.Errorf("%s: planKey = %d, want %d", tc.name, got, tc.want)
		}
	}
}
