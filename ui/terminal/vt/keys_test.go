package vt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/input"
)

const (
	shift = input.ModShift
	ctrl  = input.ModCtrl
	alt   = input.ModAlt
	meta  = input.ModMeta
)

type keyCase struct {
	key  input.Key
	mods input.Modifiers
	want string // "" with ok=false expected
}

func (c keyCase) name() string {
	return fmt.Sprintf("%v+%d", c.key, c.mods)
}

func runKeyCases(t *testing.T, m Modes, cases []keyCase) {
	t.Helper()
	for _, c := range cases {
		ev := input.KeyEvent{Key: c.key, Down: true, Mods: c.mods}
		b, ok := EncodeKey(ev, m)
		wantOK := c.want != ""
		if ok != wantOK || string(b) != c.want {
			t.Errorf("%s: EncodeKey = %q, %v; want %q, %v", c.name(), b, ok, c.want, wantOK)
		}
	}
}

func TestEncodeKeyLegacy(t *testing.T) {
	runKeyCases(t, Modes{}, []keyCase{
		// Enter, Backspace, Tab, Escape.
		{input.KeyEnter, 0, "\r"},
		{input.KeyNumpadEnter, 0, "\r"},
		{input.KeyEnter, alt, "\x1b\r"},
		{input.KeyBackspace, 0, "\x7f"},
		{input.KeyBackspace, ctrl, "\x08"},
		{input.KeyBackspace, alt, "\x1b\x7f"},
		{input.KeyBackspace, ctrl | alt, "\x1b\x08"},
		{input.KeyTab, 0, "\t"},
		{input.KeyTab, shift, "\x1b[Z"},
		{input.KeyEscape, 0, "\x1b"},
		{input.KeyEscape, alt, "\x1b\x1b"},

		// Cursor keys, plain and with modifiers (m = 1 + Shift1 + Alt2 + Ctrl4 + Meta8).
		{input.KeyUp, 0, "\x1b[A"},
		{input.KeyDown, 0, "\x1b[B"},
		{input.KeyRight, 0, "\x1b[C"},
		{input.KeyLeft, 0, "\x1b[D"},
		{input.KeyUp, shift, "\x1b[1;2A"},
		{input.KeyDown, alt, "\x1b[1;3B"},
		{input.KeyRight, ctrl, "\x1b[1;5C"},
		{input.KeyLeft, ctrl | shift, "\x1b[1;6D"},
		{input.KeyLeft, meta, "\x1b[1;9D"},
		{input.KeyUp, shift | alt | ctrl | meta, "\x1b[1;16A"},

		// Home/End.
		{input.KeyHome, 0, "\x1b[H"},
		{input.KeyEnd, 0, "\x1b[F"},
		{input.KeyHome, ctrl, "\x1b[1;5H"},
		{input.KeyEnd, shift, "\x1b[1;2F"},

		// Insert/Delete/PageUp/PageDown.
		{input.KeyInsert, 0, "\x1b[2~"},
		{input.KeyDelete, 0, "\x1b[3~"},
		{input.KeyPageUp, 0, "\x1b[5~"},
		{input.KeyPageDown, 0, "\x1b[6~"},
		{input.KeyDelete, ctrl, "\x1b[3;5~"},
		{input.KeyPageDown, alt, "\x1b[6;3~"},

		// Function keys.
		{input.KeyF1, 0, "\x1bOP"},
		{input.KeyF2, 0, "\x1bOQ"},
		{input.KeyF3, 0, "\x1bOR"},
		{input.KeyF4, 0, "\x1bOS"},
		{input.KeyF1, shift, "\x1b[1;2P"},
		{input.KeyF4, ctrl, "\x1b[1;5S"},
		{input.KeyF5, 0, "\x1b[15~"},
		{input.KeyF6, 0, "\x1b[17~"},
		{input.KeyF7, 0, "\x1b[18~"},
		{input.KeyF8, 0, "\x1b[19~"},
		{input.KeyF9, 0, "\x1b[20~"},
		{input.KeyF10, 0, "\x1b[21~"},
		{input.KeyF11, 0, "\x1b[23~"},
		{input.KeyF12, 0, "\x1b[24~"},
		{input.KeyF5, shift, "\x1b[15;2~"},
		{input.KeyF12, ctrl | alt, "\x1b[24;7~"},

		// Ctrl+letter, with or without Shift (like xterm).
		{input.KeyA, ctrl, "\x01"},
		{input.KeyC, ctrl, "\x03"},
		{input.KeyZ, ctrl, "\x1a"},
		{input.KeyA, ctrl | shift, "\x01"},
		{input.KeyD, ctrl | alt, "\x1b\x04"},

		// Ctrl+punctuation and Ctrl+Space.
		{input.KeyBracketLeft, ctrl, "\x1b"},
		{input.KeyBackslash, ctrl, "\x1c"},
		{input.KeyBracketRight, ctrl, "\x1d"},
		{input.Key6, ctrl | shift, "\x1e"}, // Ctrl+^
		{input.Key6, ctrl, "\x1e"},
		{input.KeyMinus, ctrl | shift, "\x1f"}, // Ctrl+_
		{input.KeyMinus, ctrl, "\x1f"},
		{input.KeySlash, ctrl, "\x1f"},
		{input.KeySpace, ctrl, "\x00"},
		{input.Key2, ctrl, "\x00"}, // Ctrl+@
		{input.Key3, ctrl, "\x1b"},
		{input.Key4, ctrl, "\x1c"},
		{input.Key5, ctrl, "\x1d"},
		{input.Key7, ctrl, "\x1f"},
		{input.Key8, ctrl, "\x7f"},

		// Alt+x: ESC + the key's encoding.
		{input.KeyX, alt, "\x1bx"},
		{input.KeyX, alt | shift, "\x1bX"},
		{input.Key1, alt, "\x1b1"},
		{input.KeySpace, alt, "\x1b "},
		{input.KeyPeriod, alt, "\x1b."},
		{input.KeySpace, ctrl | alt, "\x1b\x00"},
		// Alt+Shift+digit/punctuation: ESC + the US-layout shifted symbol
		// (readline M-< M-> M-? M-! M-% M-{ ...).
		{input.KeyComma, alt | shift, "\x1b<"},
		{input.KeyPeriod, alt | shift, "\x1b>"},
		{input.KeySlash, alt | shift, "\x1b?"},
		{input.Key1, alt | shift, "\x1b!"},
		{input.Key5, alt | shift, "\x1b%"},
		{input.KeyBracketLeft, alt | shift, "\x1b{"},
		{input.KeyBracketRight, alt | shift, "\x1b}"},
		{input.KeyBackquote, alt | shift, "\x1b~"},
		{input.Key0, alt | shift, "\x1b)"},
		{input.Key2, alt | shift, "\x1b@"},
		{input.Key9, alt | shift, "\x1b("},
		{input.KeyMinus, alt | shift, "\x1b_"},
		{input.KeyEqual, alt | shift, "\x1b+"},
		{input.KeySemicolon, alt | shift, "\x1b:"},
		{input.KeyQuote, alt | shift, "\x1b\""},
		{input.KeyBackslash, alt | shift, "\x1b|"},
		{input.KeySpace, alt | shift, "\x1b "},
		// Tab takes no ESC prefix (xterm.js).
		{input.KeyTab, alt, "\t"},
		{input.KeyTab, alt | shift, "\x1b[Z"},

		// Not handled by EncodeKey.
		{input.KeyA, 0, ""},     // printable: sent by TextInput
		{input.KeyA, shift, ""}, // printable
		{input.Key1, 0, ""},
		{input.KeySpace, 0, ""},
		{input.KeySpace, shift, ""},
		{input.KeyPeriod, 0, ""},
		{input.Key1, ctrl, ""},
		{input.KeyQuote, ctrl, ""},
		{input.KeyA, meta, ""}, // Cmd/Super shortcut
		{input.KeyA, meta | ctrl, ""},
		{input.KeyEnter, meta, ""},
		{input.KeyBackspace, meta, ""},
		{input.KeyTab, meta, ""},
		{input.KeyShiftLeft, shift, ""},
		{input.KeyCtrlLeft, ctrl, ""},
		{input.KeyCapsLock, 0, ""},
		{input.KeyUnknown, 0, ""},
		{input.KeyNumpad5, 0, ""},
	})
}

func TestEncodeKeyAppCursor(t *testing.T) {
	runKeyCases(t, Modes{AppCursor: true}, []keyCase{
		{input.KeyUp, 0, "\x1bOA"},
		{input.KeyDown, 0, "\x1bOB"},
		{input.KeyRight, 0, "\x1bOC"},
		{input.KeyLeft, 0, "\x1bOD"},
		{input.KeyHome, 0, "\x1bOH"},
		{input.KeyEnd, 0, "\x1bOF"},
		// Modifiers use the CSI form regardless of DECCKM.
		{input.KeyUp, ctrl, "\x1b[1;5A"},
		{input.KeyHome, shift, "\x1b[1;2H"},
		// Keys DECCKM does not affect.
		{input.KeyPageUp, 0, "\x1b[5~"},
		{input.KeyF1, 0, "\x1bOP"},
		{input.KeyEnter, 0, "\r"},
	})
}

func TestEncodeKeyUpAndRepeat(t *testing.T) {
	ev := input.KeyEvent{Key: input.KeyUp, Down: true, Repeat: true}
	if b, ok := EncodeKey(ev, Modes{}); !ok || string(b) != "\x1b[A" {
		t.Errorf("repeat: got %q, %v; want \"\\x1b[A\", true", b, ok)
	}
	ev = input.KeyEvent{Key: input.KeyUp}
	if b, ok := EncodeKey(ev, Modes{}); ok || b != nil {
		t.Errorf("key up: got %q, %v; want nil, false", b, ok)
	}
	ev = input.KeyEvent{Key: input.KeyC, Mods: ctrl}
	if b, ok := EncodeKey(ev, Modes{}); ok || b != nil {
		t.Errorf("Ctrl+C up: got %q, %v; want nil, false", b, ok)
	}
}

// Each call returns a fresh slice: changing one result must not affect
// the next.
func TestEncodeKeyFreshSlice(t *testing.T) {
	ev := input.KeyEvent{Key: input.KeyUp, Down: true}
	b, _ := EncodeKey(ev, Modes{})
	b[0] = 'x'
	if b2, _ := EncodeKey(ev, Modes{}); string(b2) != "\x1b[A" {
		t.Fatalf("second call = %q after mutating the first", b2)
	}
}

func TestPasteStripsBracketEnd(t *testing.T) {
	got := EncodePaste("a\x1b[201~b\nc", Modes{BracketedPaste: true})
	if want := "\x1b[200~ab\rc\x1b[201~"; string(got) != want {
		t.Fatalf("EncodePaste = %q, want %q", got, want)
	}
}

func TestPasteNoBracket(t *testing.T) {
	got := EncodePaste("x\x1b[200~y\x1b[201~z", Modes{})
	if want := "xyz"; string(got) != want {
		t.Fatalf("EncodePaste = %q, want %q", got, want)
	}
}

func TestPasteCases(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"plain", "plain"},
		{"a\r\nb\nc\rd", "a\rb\rc\rd"},
		{"\n\n", "\r\r"},
		{"\r\n\r\n", "\r\r"},
		{"\r\r\n", "\r\r"},
		{"tab\there\x07\x1b[31mred", "tab\there\x07\x1b[31mred"}, // other controls kept
		{"\x1b[200", "\x1b[200"},                                 // incomplete marker kept
		{"\x1b[202~", "\x1b[202~"},
		// Removing one marker must not leave another behind.
		{"\x1b[20\x1b[201~1~", ""},
		{"\x1b[20\x1b[20\x1b[200~1~0~x", "x"},
		{"\x1b[201\x1b[200~~", ""},
		// A marker split by a newline is not a marker, before or after
		// normalization.
		{"\x1b[20\n1~", "\x1b[20\r1~"},
		{"\r\x1b[201~\n", "\r"},
	} {
		if got := EncodePaste(c.in, Modes{}); string(got) != c.want {
			t.Errorf("EncodePaste(%q) = %q, want %q", c.in, got, c.want)
		}
		got := string(EncodePaste(c.in, Modes{BracketedPaste: true}))
		inner, ok := strings.CutPrefix(got, "\x1b[200~")
		if ok {
			inner, ok = strings.CutSuffix(inner, "\x1b[201~")
		}
		if !ok || inner != c.want {
			t.Errorf("bracketed EncodePaste(%q) = %q, want markers around %q", c.in, got, c.want)
		}
	}
}

func TestPasteLargeNested(t *testing.T) {
	// Deeply nested fake markers are removed in linear time.
	const n = 200000
	s := strings.Repeat("\x1b[20", n) + strings.Repeat("1~", n)
	if got := EncodePaste(s, Modes{}); len(got) != 0 {
		t.Fatalf("EncodePaste of nested markers left %d bytes", len(got))
	}
}

func TestEncodeText(t *testing.T) {
	for _, s := range []string{"", "a", "héllo", "中文", "\x1b"} {
		if got := EncodeText(s); string(got) != s {
			t.Errorf("EncodeText(%q) = %q", s, got)
		}
	}
}
