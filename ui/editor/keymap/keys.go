// Package keymap parses keybinding text and resolves key events to commands,
// with VS Code's rule semantics.
//
// Key text is MinCode's format (vue-app/src/domain/keybindings/keys.ts),
// which is VS Code's with a few aliases: chords "mod+shift+p" joined by
// "+", case-insensitive; "mod" (also "cmdorctrl") is ⌘ on macOS and Ctrl
// elsewhere; "cmd"/"command"/"meta"/"win"/"super" is ⌘ / the Windows key;
// "ctrl"/"control", "shift", "alt"/"option"/"opt". Key names are VS Code's:
// a-z, 0-9, f1-f12, up down left right, enter (return), escape (esc), tab,
// space, backspace, delete (del), insert, home, end, pageup, pagedown,
// / \ . , ; ' ` [ ] - =, numpad0-numpad9, numpad_add, numpad_subtract,
// numpad_multiply, numpad_divide, numpad_decimal, capslock, contextmenu. A
// sequence is one chord or two separated by blanks ("mod+k mod+c";
// MaxChords), or a double tap of one modifier ("shift shift").
//
// Not supported (ParseKey reports an error): f13-f24 (input.Key has no
// such keys), numpad_separator, VS Code's scan-code form ("[KeyA]") and
// the oem_* / abnt_c1 / abnt_c2 names, the media and browser keys
// (audiovolumemute, mediaplaypause, browserback...), pausebreak,
// printscreen, numlock, scrolllock, and a "+" key (use "=" with shift or
// numpad_add).
//
// Keys match by physical position: input.KeyEvent.Code, which the Runner
// fills from Key when the platform reports none. Where the platform
// reports the physical key (Windows), ⌥/Alt+letter and non-US layouts
// match by position. Keypad keys match both their own name and the main
// key they duplicate (Numpad7 matches "numpad7" and "7", NumpadEnter
// matches "enter"), as MinCode's normalizeKeyCode folds them.
//
// Platform limitation (macOS and Linux, as of this milestone): a key that
// produces text while the text input client is on arrives as TextInput
// only, with no KeyEvent (macOS passes it through interpretKeyEvents, GTK
// through its input-method filter), and macOS never fills Code, so the
// Runner's Code is the layout-dependent Key. Consequently ⌥-letter
// bindings never fire on macOS, plain-character chord second keys
// ("mod+k c") arrive as text (the editor resolves them from the text, see
// EventForText), and physical-key matching on macOS holds only for keys
// that do not depend on the layout. Fixing it needs the platform layer to
// fill Code from the hardware keyCode and to offer key events to the
// keymap before interpretKeyEvents / the IM filter.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package keymap

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/tangoui/tango/ui/input"
)

// Platform selects the meaning of "mod", platform-specific default rules
// and key labels.
type Platform int

const (
	Mac Platform = iota
	Windows
	Linux
)

func (p Platform) String() string {
	switch p {
	case Mac:
		return "mac"
	case Windows:
		return "windows"
	}
	return "linux"
}

// Current returns the Platform of runtime.GOOS (iOS counts as Mac, every
// other non-Windows system as Linux).
func Current() Platform { return PlatformFrom(runtime.GOOS) }

// PlatformFrom maps a platform name (runtime.GOOS, navigator.platform,
// "macOS", "Win32"...) to a Platform, as MinCode's keyboardPlatformFrom:
// anything mentioning mac, darwin, iphone, ipad or ipod is Mac, then
// anything mentioning win is Windows, the rest Linux.
func PlatformFrom(name string) Platform {
	s := strings.ToLower(name)
	for _, m := range []string{"mac", "iphone", "ipad", "ipod", "darwin", "ios"} {
		if strings.Contains(s, m) {
			return Mac
		}
	}
	if strings.Contains(s, "win") {
		return Windows
	}
	return Linux
}

// MaxChords is the longest chord sequence (VS Code's two).
const MaxChords = 2

// MaxKeyTextLength is the longest key text ParseKey accepts.
const MaxKeyTextLength = 64

// modMask is the modifiers a chord compares.
const modMask = input.ModShift | input.ModCtrl | input.ModAlt | input.ModMeta

// Chord is one key press: modifiers and a physical key.
type Chord struct {
	Mods input.Modifiers
	Key  input.Key
}

// String returns the canonical, platform-independent text of the chord,
// modifiers in the order ctrl, shift, alt, meta ("ctrl+shift+p",
// "alt+meta+up").
func (c Chord) String() string {
	var b strings.Builder
	for _, m := range [...]struct {
		bit  input.Modifiers
		name string
	}{{input.ModCtrl, "ctrl"}, {input.ModShift, "shift"}, {input.ModAlt, "alt"}, {input.ModMeta, "meta"}} {
		if c.Mods&m.bit != 0 {
			b.WriteString(m.name)
			b.WriteByte('+')
		}
	}
	b.WriteString(keyToken(c.Key))
	return b.String()
}

// Sequence is parsed key text: one or two chords, or (Double non-zero) a
// double tap of one modifier.
type Sequence struct {
	Chords []Chord
	Double input.Modifiers
}

// String returns the canonical, platform-independent text ("ctrl+shift+p",
// "meta+k meta+s", "shift shift"), MinCode's sequenceId.
func (s Sequence) String() string {
	if s.Double != 0 {
		n := modName(s.Double)
		return n + " " + n
	}
	parts := make([]string, len(s.Chords))
	for i, c := range s.Chords {
		parts[i] = c.String()
	}
	return strings.Join(parts, " ")
}

// Equal reports whether s and o are the same sequence.
func (s Sequence) Equal(o Sequence) bool {
	if s.Double != o.Double || len(s.Chords) != len(o.Chords) {
		return false
	}
	for i := range s.Chords {
		if s.Chords[i] != o.Chords[i] {
			return false
		}
	}
	return true
}

func modName(m input.Modifiers) string {
	switch m {
	case input.ModCtrl:
		return "ctrl"
	case input.ModShift:
		return "shift"
	case input.ModAlt:
		return "alt"
	}
	return "meta"
}

// modAliases maps modifier names; 0xff stands for "mod".
var modAliases = map[string]input.Modifiers{
	"mod":       0xff,
	"cmdorctrl": 0xff,
	"ctrl":      input.ModCtrl,
	"control":   input.ModCtrl,
	"shift":     input.ModShift,
	"alt":       input.ModAlt,
	"option":    input.ModAlt,
	"opt":       input.ModAlt,
	"meta":      input.ModMeta,
	"cmd":       input.ModMeta,
	"command":   input.ModMeta,
	"win":       input.ModMeta,
	"super":     input.ModMeta,
}

func modifier(token string, p Platform) (input.Modifiers, bool) {
	m, ok := modAliases[token]
	if m == 0xff {
		if p == Mac {
			m = input.ModMeta
		} else {
			m = input.ModCtrl
		}
	}
	return m, ok
}

// namedKeys maps VS Code key names (and MinCode's aliases) to keys. The
// first name listed for a key in keyNameOrder is its canonical token.
var namedKeys = map[string]input.Key{
	"up": input.KeyUp, "down": input.KeyDown, "left": input.KeyLeft, "right": input.KeyRight,
	"enter": input.KeyEnter, "return": input.KeyEnter,
	"escape": input.KeyEscape, "esc": input.KeyEscape,
	"tab": input.KeyTab, "space": input.KeySpace, "backspace": input.KeyBackspace,
	"delete": input.KeyDelete, "del": input.KeyDelete, "insert": input.KeyInsert,
	"home": input.KeyHome, "end": input.KeyEnd, "pageup": input.KeyPageUp, "pagedown": input.KeyPageDown,
	"/": input.KeySlash, "\\": input.KeyBackslash, ".": input.KeyPeriod, ",": input.KeyComma,
	";": input.KeySemicolon, "'": input.KeyQuote, "`": input.KeyBackquote,
	"[": input.KeyBracketLeft, "]": input.KeyBracketRight, "-": input.KeyMinus, "=": input.KeyEqual,
	"numpad_add": input.KeyNumpadAdd, "numpad_subtract": input.KeyNumpadSubtract,
	"numpad_multiply": input.KeyNumpadMultiply, "numpad_divide": input.KeyNumpadDivide,
	"numpad_decimal": input.KeyNumpadDecimal,
	"capslock":       input.KeyCapsLock, "contextmenu": input.KeyContextMenu,
}

// canonical tokens of keys with several names.
var keyTokens = map[input.Key]string{
	input.KeyEnter: "enter", input.KeyEscape: "escape", input.KeyDelete: "delete",
	input.KeyNumpadEnter: "numpad_enter",
}

func init() {
	for name, k := range namedKeys {
		if _, ok := keyTokens[k]; !ok {
			keyTokens[k] = name
		}
	}
}

// keyNamed returns the key of a (lower-case) key name.
func keyNamed(tok string) (input.Key, bool) {
	if len(tok) == 1 {
		if k := input.KeyForRune(rune(tok[0])); k != input.KeyUnknown {
			return k, true
		}
	}
	if k, ok := namedKeys[tok]; ok {
		return k, true
	}
	if n, ok := strings.CutPrefix(tok, "f"); ok && len(n) <= 2 && n != "" && n[0] != '0' {
		v := 0
		for _, c := range n {
			if c < '0' || c > '9' {
				return 0, false
			}
			v = v*10 + int(c-'0')
		}
		if v >= 1 && v <= 12 {
			return input.KeyF1 + input.Key(v-1), true
		}
		return 0, false
	}
	if n, ok := strings.CutPrefix(tok, "numpad"); ok && len(n) == 1 && n[0] >= '0' && n[0] <= '9' {
		return input.KeyNumpad0 + input.Key(n[0]-'0'), true
	}
	return 0, false
}

// keyToken returns the canonical lower-case token of k ("k", "1", "f12",
// "up", "numpad7").
func keyToken(k input.Key) string {
	if r := k.Rune(); r != 0 && k != input.KeySpace && !(k >= input.KeyNumpad0 && k <= input.KeyNumpadEnter) {
		if t, ok := keyTokens[k]; ok {
			return t
		}
		return string(r)
	}
	switch {
	case k >= input.KeyF1 && k <= input.KeyF12:
		return fmt.Sprintf("f%d", int(k-input.KeyF1)+1)
	case k >= input.KeyNumpad0 && k <= input.KeyNumpad9:
		return fmt.Sprintf("numpad%d", int(k-input.KeyNumpad0))
	}
	if t, ok := keyTokens[k]; ok {
		return t
	}
	return strings.ToLower(k.String())
}

// ParseKey parses key text for platform p.
func ParseKey(text string, p Platform) (Sequence, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		return Sequence{}, errors.New("keymap: empty key")
	}
	if len(t) > MaxKeyTextLength {
		return Sequence{}, fmt.Errorf("keymap: key text exceeds %d characters", MaxKeyTextLength)
	}
	parts := strings.Fields(t)
	if len(parts) == 2 {
		a, okA := modifier(strings.ToLower(parts[0]), p)
		b, okB := modifier(strings.ToLower(parts[1]), p)
		if okA && okB {
			if a != b {
				return Sequence{}, fmt.Errorf("keymap: %q: a double tap needs the same modifier twice", text)
			}
			return Sequence{Double: a}, nil
		}
	}
	if len(parts) > MaxChords {
		return Sequence{}, fmt.Errorf("keymap: %q: more than %d chords", text, MaxChords)
	}
	var seq Sequence
	for _, part := range parts {
		c, err := parseChord(part, p)
		if err != nil {
			return Sequence{}, fmt.Errorf("keymap: %q: %w", text, err)
		}
		seq.Chords = append(seq.Chords, c)
	}
	return seq, nil
}

func parseChord(text string, p Platform) (Chord, error) {
	toks := strings.Split(text, "+")
	var c Chord
	for i, raw := range toks {
		if raw == "" {
			return Chord{}, errors.New("empty part (a trailing or doubled +)")
		}
		tok := strings.ToLower(raw)
		if i < len(toks)-1 {
			m, ok := modifier(tok, p)
			if !ok {
				return Chord{}, fmt.Errorf("unknown modifier %q", raw)
			}
			if c.Mods&m != 0 {
				return Chord{}, fmt.Errorf("modifier %q twice", raw)
			}
			c.Mods |= m
			continue
		}
		k, ok := keyNamed(tok)
		if !ok {
			return Chord{}, fmt.Errorf("unknown key %q", raw)
		}
		c.Key = k
	}
	return c, nil
}

// MustParseKey is ParseKey that panics on error (tables and tests).
func MustParseKey(text string, p Platform) Sequence {
	s, err := ParseKey(text, p)
	if err != nil {
		panic(err)
	}
	return s
}

// isModifierKey reports whether k is a modifier key or another key that
// never forms a chord on its own (Caps Lock).
func isModifierKey(k input.Key) bool {
	return k.Modifier() != 0 || k == input.KeyCapsLock
}

// navKey reports whether k is a key the keypad stands for with NumLock off.
func navKey(k input.Key) bool {
	switch k {
	case input.KeyUp, input.KeyDown, input.KeyLeft, input.KeyRight, input.KeyHome, input.KeyEnd,
		input.KeyPageUp, input.KeyPageDown, input.KeyInsert, input.KeyDelete:
		return true
	}
	return false
}

// physical returns the event's physical key: Code (Key when Code is 0),
// except that a keypad key reported as a navigation key (NumLock off)
// keeps the navigation key.
func physical(ev input.KeyEvent) input.Key {
	code := ev.Code
	if code == 0 {
		code = ev.Key
	}
	if navKey(ev.Key) && code >= input.KeyNumpad0 && code <= input.KeyNumpadDecimal {
		return ev.Key
	}
	return code
}

// ChordOf returns the chord of a key event with keypad keys folded onto
// the main keys (Key.Base), as MinCode's keyChordFromEvent; ok is false for
// modifier keys pressed alone and unknown keys.
func ChordOf(ev input.KeyEvent) (Chord, bool) {
	k := physical(ev)
	if k == input.KeyUnknown || isModifierKey(k) {
		return Chord{}, false
	}
	return Chord{Mods: ev.Mods & modMask, Key: k.Base()}, true
}

// chordsOf returns the chords an event matches: the exact physical key
// first, then its main-key duplicate for keypad keys.
func chordsOf(ev input.KeyEvent) []Chord {
	k := physical(ev)
	if k == input.KeyUnknown || isModifierKey(k) {
		return nil
	}
	m := ev.Mods & modMask
	if b := k.Base(); b != k {
		return []Chord{{m, k}, {m, b}}
	}
	return []Chord{{m, k}}
}

// --- labels ---

var macKeyLabels = map[input.Key]string{
	input.KeyUp: "↑", input.KeyDown: "↓", input.KeyLeft: "←", input.KeyRight: "→",
	input.KeyEnter: "↩", input.KeyBackspace: "⌫", input.KeyDelete: "⌦", input.KeyEscape: "⎋",
	input.KeyTab: "⇥", input.KeySpace: "␣", input.KeyPageUp: "⇞", input.KeyPageDown: "⇟",
	input.KeyHome: "↖", input.KeyEnd: "↘",
}

var textKeyLabels = map[input.Key]string{input.KeyEscape: "Esc"}

func keyLabel(k input.Key, p Platform) string {
	byName := k == input.KeySpace || (k >= input.KeyNumpad0 && k <= input.KeyNumpadEnter)
	if r := k.Rune(); r != 0 && !byName {
		return strings.ToUpper(string(r))
	}
	if p == Mac {
		if l, ok := macKeyLabels[k]; ok {
			return l
		}
	}
	if l, ok := textKeyLabels[k]; ok {
		return l
	}
	return k.String()
}

var macModGlyphs = map[input.Modifiers]string{
	input.ModCtrl: "⌃", input.ModAlt: "⌥", input.ModShift: "⇧", input.ModMeta: "⌘",
}

func textModLabel(m input.Modifiers, p Platform) string {
	switch m {
	case input.ModCtrl:
		return "Ctrl"
	case input.ModShift:
		return "Shift"
	case input.ModAlt:
		return "Alt"
	}
	if p == Windows {
		return "Win"
	}
	return "Super"
}

// Label returns the chord as shown to the user: "⇧⌘P" on macOS (Apple's
// modifier order ⌃⌥⇧⌘), "Ctrl+Shift+P" elsewhere (VS Code's order Ctrl,
// Shift, Alt, Win/Super).
func (c Chord) Label(p Platform) string {
	key := keyLabel(c.Key, p)
	if p == Mac {
		var b strings.Builder
		for _, m := range []input.Modifiers{input.ModCtrl, input.ModAlt, input.ModShift, input.ModMeta} {
			if c.Mods&m != 0 {
				b.WriteString(macModGlyphs[m])
			}
		}
		return b.String() + key
	}
	var parts []string
	for _, m := range []input.Modifiers{input.ModCtrl, input.ModShift, input.ModAlt, input.ModMeta} {
		if c.Mods&m != 0 {
			parts = append(parts, textModLabel(m, p))
		}
	}
	return strings.Join(append(parts, key), "+")
}

// Label returns the sequence as shown to the user, MinCode's
// formatKeySequence: "⇧⌘P", "⌘K ⌘S", "⇧⇧" on macOS; "Ctrl+Shift+P",
// "Ctrl+K Ctrl+S", "Shift Shift" elsewhere.
func (s Sequence) Label(p Platform) string {
	if s.Double != 0 {
		if p == Mac {
			return strings.Repeat(macModGlyphs[s.Double], 2)
		}
		l := textModLabel(s.Double, p)
		return l + " " + l
	}
	parts := make([]string, len(s.Chords))
	for i, c := range s.Chords {
		parts[i] = c.Label(p)
	}
	return strings.Join(parts, " ")
}

// Label formats key text directly; text that does not parse is returned
// as is (MinCode's formatKeyText).
func Label(text string, p Platform) string {
	s, err := ParseKey(text, p)
	if err != nil {
		return text
	}
	return s.Label(p)
}

// shiftedUS maps the characters Shift produces on a US layout to their
// key.
var shiftedUS = map[rune]input.Key{
	'!': input.Key1, '@': input.Key2, '#': input.Key3, '$': input.Key4, '%': input.Key5,
	'^': input.Key6, '&': input.Key7, '*': input.Key8, '(': input.Key9, ')': input.Key0,
	'_': input.KeyMinus, '+': input.KeyEqual, '{': input.KeyBracketLeft, '}': input.KeyBracketRight,
	'|': input.KeyBackslash, ':': input.KeySemicolon, '"': input.KeyQuote, '<': input.KeyComma,
	'>': input.KeyPeriod, '?': input.KeySlash, '~': input.KeyBackquote,
}

// EventForText returns the key press that types text on a US layout: one
// ASCII letter, digit, space or punctuation character (an upper-case
// letter or a shifted symbol with Shift); ok is false for anything else.
// It stands in for the KeyEvent that macOS and Linux do not send for keys
// that produce text (see the package doc), for example to resolve the
// second chord of "mod+k c" from the committed "c". Modifiers other than
// Shift are unknown and not set.
//
// The mapping is a guess from the character alone, so it is wrong when the
// character was not typed the US way:
//
//   - CapsLock: with CapsLock on, "c" arrives as "C" and is reported as
//     Shift+C, so "mod+k c" does not complete (and "mod+k shift+c" does).
//   - Non-US layouts and ⌥ on macOS: a character the layout produces with
//     ⌥ (or AltGr) that happens to be ASCII, such as "@" from ⌥L on a
//     German layout, is reported as the US key for it (Shift+2), without
//     Alt; a chord on that US key fires, one on the physical key does not.
func EventForText(text string) (input.KeyEvent, bool) {
	if len(text) != 1 {
		return input.KeyEvent{}, false
	}
	r := rune(text[0])
	var k input.Key
	var m input.Modifiers
	switch {
	case r >= 'A' && r <= 'Z':
		k, m = input.KeyForRune(r), input.ModShift
	case r == ' ':
		k = input.KeySpace
	default:
		if sk, ok := shiftedUS[r]; ok {
			k, m = sk, input.ModShift
			break
		}
		if kk := input.KeyForRune(r); kk != input.KeyUnknown {
			k = kk
			break
		}
		for name, nk := range namedKeys {
			if len(name) == 1 && rune(name[0]) == r {
				k = nk
			}
		}
	}
	if k == input.KeyUnknown {
		return input.KeyEvent{}, false
	}
	return input.KeyEvent{Key: k, Code: k, Down: true, Mods: m}, true
}
