package vt

import (
	"strconv"

	"github.com/tangoui/tango/ui/input"
)

// Legacy (xterm) key encoding.
//
// Keys whose sequence carries a modifier parameter (cursor keys,
// Home/End, Insert/Delete/PageUp/PageDown, F1–F12) send CSI 1;m X or
// CSI n;m ~ when a modifier is held, with m = 1 + Shift 1 + Alt 2 +
// Ctrl 4 + Meta 8, as xterm and xterm.js do. All other keys have no
// modifier parameter: Alt sends ESC before the key's encoding (except
// on Tab, which xterm.js sends unchanged), and Meta
// (Command on macOS, Super/Windows elsewhere) makes the key an
// application shortcut, so EncodeKey reports ok=false.

// csiKey describes a key with a modifier-parameter sequence.
type csiKey struct {
	// plain is the sequence without modifiers; app replaces it under
	// DECCKM ("" when DECCKM does not apply).
	plain, app string
	// n and final build the modified form CSI n;m final.
	n     int
	final byte
}

var csiKeys = map[input.Key]csiKey{
	input.KeyUp:       {"\x1b[A", "\x1bOA", 1, 'A'},
	input.KeyDown:     {"\x1b[B", "\x1bOB", 1, 'B'},
	input.KeyRight:    {"\x1b[C", "\x1bOC", 1, 'C'},
	input.KeyLeft:     {"\x1b[D", "\x1bOD", 1, 'D'},
	input.KeyHome:     {"\x1b[H", "\x1bOH", 1, 'H'},
	input.KeyEnd:      {"\x1b[F", "\x1bOF", 1, 'F'},
	input.KeyInsert:   {"\x1b[2~", "", 2, '~'},
	input.KeyDelete:   {"\x1b[3~", "", 3, '~'},
	input.KeyPageUp:   {"\x1b[5~", "", 5, '~'},
	input.KeyPageDown: {"\x1b[6~", "", 6, '~'},
	input.KeyF1:       {"\x1bOP", "", 1, 'P'},
	input.KeyF2:       {"\x1bOQ", "", 1, 'Q'},
	input.KeyF3:       {"\x1bOR", "", 1, 'R'},
	input.KeyF4:       {"\x1bOS", "", 1, 'S'},
	input.KeyF5:       {"\x1b[15~", "", 15, '~'},
	input.KeyF6:       {"\x1b[17~", "", 17, '~'},
	input.KeyF7:       {"\x1b[18~", "", 18, '~'},
	input.KeyF8:       {"\x1b[19~", "", 19, '~'},
	input.KeyF9:       {"\x1b[20~", "", 20, '~'},
	input.KeyF10:      {"\x1b[21~", "", 21, '~'},
	input.KeyF11:      {"\x1b[23~", "", 23, '~'},
	input.KeyF12:      {"\x1b[24~", "", 24, '~'},
}

// ctrlKeys maps the non-letter keys Ctrl turns into C0 controls, by
// their US-layout key (Ctrl+^ is Ctrl+6, Ctrl+_ is Ctrl+Minus; Shift
// does not matter). The digit row follows xterm.js.
var ctrlKeys = map[input.Key]byte{
	input.KeySpace:        0x00,
	input.Key2:            0x00, // Ctrl+@
	input.KeyBracketLeft:  0x1b,
	input.Key3:            0x1b,
	input.KeyBackslash:    0x1c,
	input.Key4:            0x1c,
	input.KeyBracketRight: 0x1d,
	input.Key5:            0x1d,
	input.Key6:            0x1e, // Ctrl+^
	input.KeyMinus:        0x1f, // Ctrl+_
	input.KeySlash:        0x1f,
	input.Key7:            0x1f,
	input.Key8:            0x7f,
}

// shiftedUS maps the character a digit or punctuation key gives on the
// US layout to the one it gives with Shift.
var shiftedUS = map[rune]byte{
	'1': '!', '2': '@', '3': '#', '4': '$', '5': '%',
	'6': '^', '7': '&', '8': '*', '9': '(', '0': ')',
	',': '<', '.': '>', '/': '?', '-': '_', '=': '+',
	';': ':', '\'': '"', '`': '~', '[': '{', ']': '}', '\\': '|',
}

// EncodeKey returns the bytes a key press sends to the program; ok=false
// means the terminal does not handle the key and the caller may treat it
// as an application shortcut or ignore it.
//
// Only presses (Down, including Repeat) are encoded. Printable characters
// arrive as committed text (see [EncodeText]); EncodeKey handles letters,
// digits, punctuation and Space only when Ctrl or Alt is held.
//
// The encoding is xterm's legacy one: Enter "\r", Backspace "\x7f"
// (Ctrl+Backspace "\x08"), Tab "\t", Shift+Tab "\x1b[Z", Esc "\x1b",
// cursor keys "\x1b[A".."\x1b[D" ("\x1bOA".."\x1bOD" when
// [Modes.AppCursor] is set), Home/End, Insert/Delete/PageUp/PageDown,
// F1–F12, Ctrl+letter 0x01–0x1A, Ctrl+[ \ ] ^ _ and Ctrl+Space. Keys
// with a modifier parameter send "\x1b[1;{m}A" or "\x1b[3;{m}~" when a
// modifier is held, with m = 1 + Shift 1 + Alt 2 + Ctrl 4 + Meta 8, and
// ignore AppCursor then. On other keys except Tab (as in xterm.js) Alt
// sends ESC followed by the key's encoding: Alt+x "\x1bx", Alt+Shift+x
// "\x1bX", and Alt+Shift with a digit or punctuation key ESC followed by
// the symbol it gives on the US layout (Alt+Shift+. "\x1b>", readline's
// M->). Meta (Command, Super) is not encoded: such a key returns
// ok=false. Whether Option on macOS counts as Alt is the caller's
// choice: it clears [input.ModAlt] to let Option compose characters
// instead.
//
// Each call returns a new slice.
func EncodeKey(ev input.KeyEvent, m Modes) (b []byte, ok bool) {
	if !ev.Down {
		return nil, false
	}
	mods := ev.Mods
	if ck, found := csiKeys[ev.Key]; found {
		return ck.encode(mods, m.AppCursor), true
	}
	if mods.Has(input.ModMeta) {
		return nil, false
	}
	shift, ctrl := mods.Has(input.ModShift), mods.Has(input.ModCtrl)
	var c []byte
	switch ev.Key {
	case input.KeyEnter, input.KeyNumpadEnter:
		c = []byte{'\r'}
	case input.KeyBackspace:
		if ctrl {
			c = []byte{0x08}
		} else {
			c = []byte{0x7f}
		}
	case input.KeyTab:
		// No ESC prefix for Alt, as in xterm.js.
		if shift {
			return []byte("\x1b[Z"), true
		}
		return []byte{'\t'}, true
	case input.KeyEscape:
		c = []byte{0x1b}
	default:
		r := ev.Key.Rune()
		isLetter := r >= 'a' && r <= 'z'
		switch {
		case r == 0 || ev.Key >= input.KeyNumpad0 && ev.Key <= input.KeyNumpadDecimal:
			return nil, false
		case ctrl && isLetter:
			c = []byte{byte(r-'a') + 1}
		case ctrl:
			v, found := ctrlKeys[ev.Key]
			if !found {
				return nil, false
			}
			c = []byte{v}
		case !mods.Has(input.ModAlt):
			return nil, false // printable: sent as committed text
		case shift && isLetter:
			c = []byte{byte(r - 'a' + 'A')}
		case shift:
			if v, found := shiftedUS[r]; found {
				c = []byte{v}
			} else {
				c = []byte{byte(r)} // Space
			}
		default:
			c = []byte{byte(r)}
		}
	}
	if mods.Has(input.ModAlt) {
		c = append([]byte{0x1b}, c...)
	}
	return c, true
}

// modParam returns xterm's modifier parameter for mods, or 0 when none
// of Shift, Alt, Ctrl and Meta is held.
func modParam(mods input.Modifiers) int {
	p := 0
	if mods.Has(input.ModShift) {
		p |= 1
	}
	if mods.Has(input.ModAlt) {
		p |= 2
	}
	if mods.Has(input.ModCtrl) {
		p |= 4
	}
	if mods.Has(input.ModMeta) {
		p |= 8
	}
	if p == 0 {
		return 0
	}
	return p + 1
}

func (k csiKey) encode(mods input.Modifiers, appCursor bool) []byte {
	p := modParam(mods)
	switch {
	case p != 0:
		b := make([]byte, 0, 10)
		b = append(b, "\x1b["...)
		b = strconv.AppendInt(b, int64(k.n), 10)
		b = append(b, ';')
		b = strconv.AppendInt(b, int64(p), 10)
		return append(b, k.final)
	case appCursor && k.app != "":
		return []byte(k.app)
	}
	return []byte(k.plain)
}

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// EncodePaste returns the bytes pasting s sends to the program. Any
// bracketed-paste markers ("\x1b[200~", "\x1b[201~") in s are removed,
// including ones that removing another marker would form, so pasted text
// cannot end bracketed paste early; "\r\n" and "\n" become "\r". When
// [Modes.BracketedPaste] is set the result is wrapped in the markers.
// Other control characters are kept.
func EncodePaste(s string, m Modes) []byte {
	b := make([]byte, 0, len(s)+len(pasteStart)+len(pasteEnd))
	if m.BracketedPaste {
		b = append(b, pasteStart...)
	}
	start := len(b)
	// Removing a marker as soon as its last byte is appended keeps the
	// output free of markers in one pass, however deeply they nest.
	for i := 0; i < len(s); i++ {
		b = append(b, s[i])
		if s[i] == '~' && len(b)-start >= len(pasteStart) {
			tail := string(b[len(b)-len(pasteStart):])
			if tail == pasteStart || tail == pasteEnd {
				b = b[:len(b)-len(pasteStart)]
			}
		}
	}
	// Normalize newlines in place; this cannot form a marker, which
	// contains neither '\r' nor '\n'.
	w := start
	var prev byte // the previous input byte, before conversion
	for r := start; r < len(b); r++ {
		c := b[r]
		cr := c == '\n' && prev == '\r'
		prev = c
		if cr {
			continue
		}
		if c == '\n' {
			c = '\r'
		}
		b[w] = c
		w++
	}
	b = b[:w]
	if m.BracketedPaste {
		b = append(b, pasteEnd...)
	}
	return b
}

// EncodeText returns the bytes committed text sends to the program: s as
// UTF-8, unchanged. Alt is handled by [EncodeKey].
func EncodeText(s string) []byte { return []byte(s) }
