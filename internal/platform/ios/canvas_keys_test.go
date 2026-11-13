package ios

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestModsFromUIKit(t *testing.T) {
	cases := []struct {
		flags int64
		want  input.Modifiers
	}{
		{0, 0},
		{uiKeyModShift, input.ModShift},
		{uiKeyModControl | uiKeyModAlternate, input.ModCtrl | input.ModAlt},
		{uiKeyModCommand | uiKeyModShift, input.ModMeta | input.ModShift},
		{uiKeyModAlphaShift | uiKeyModNumericPad, 0}, // Caps Lock and keypad are not modifiers
	}
	for _, c := range cases {
		if got := modsFromUIKit(c.flags); got != c.want {
			t.Errorf("modsFromUIKit(%#x) = %v, want %v", c.flags, got, c.want)
		}
	}
}

func TestKeyFromHID(t *testing.T) {
	cases := []struct {
		usage     uint16
		chars     string
		key, code input.Key
	}{
		{0x04, "a", input.KeyA, input.KeyA},
		{0x1D, "z", input.KeyZ, input.KeyZ},
		{0x1E, "1", input.Key1, input.Key1},
		{0x26, "9", input.Key9, input.Key9},
		{0x27, "0", input.Key0, input.Key0},
		// Dvorak: the key at US "c" types "j": shortcuts follow the label.
		{0x06, "j", input.KeyJ, input.KeyC},
		// AZERTY: the US "1" key types "&": named by position.
		{0x1E, "&", input.Key1, input.Key1},
		// German: the US "y" key is labelled Z.
		{0x1C, "z", input.KeyZ, input.KeyY},
		{0x2D, "-", input.KeyMinus, input.KeyMinus},
		{0x35, "`", input.KeyBackquote, input.KeyBackquote},
		{0x38, "/", input.KeySlash, input.KeySlash},
		{0x28, "\r", input.KeyEnter, input.KeyEnter},
		{0x2A, "\b", input.KeyBackspace, input.KeyBackspace},
		{0x4C, "\x7f", input.KeyDelete, input.KeyDelete},
		{0x50, "UIKeyInputLeftArrow", input.KeyLeft, input.KeyLeft},
		{0x52, "UIKeyInputUpArrow", input.KeyUp, input.KeyUp},
		{0x3A, "UIKeyInputF1", input.KeyF1, input.KeyF1},
		{0x45, "", input.KeyF12, input.KeyF12},
		{0x58, "\r", input.KeyNumpadEnter, input.KeyNumpadEnter},
		{0x62, "0", input.KeyNumpad0, input.KeyNumpad0},
		{0x59, "1", input.KeyNumpad1, input.KeyNumpad1},
		{0xE1, "", input.KeyShiftLeft, input.KeyShiftLeft},
		{0xE7, "", input.KeyMetaRight, input.KeyMetaRight},
		{0x64, "§", input.KeyUnknown, input.KeyUnknown}, // Non-US backslash
		{0x7F, "", input.KeyUnknown, input.KeyUnknown},  // Mute
	}
	for _, c := range cases {
		k, code := keyFromHID(c.usage, c.chars)
		if k != c.key || code != c.code {
			t.Errorf("keyFromHID(%#x, %q) = %v, %v; want %v, %v", c.usage, c.chars, k, code, c.key, c.code)
		}
	}
}

func TestHIDTablesComplete(t *testing.T) {
	// Every letter and digit of the main block is named.
	for u := uint16(0x04); u <= 0x27; u++ {
		if hidPhysical[u] == input.KeyUnknown {
			t.Errorf("usage %#x has no physical key", u)
		}
	}
	// The layout-independent table never names a typing key twice.
	seen := map[input.Key]uint16{}
	for u, k := range hidKeys {
		if prev, ok := seen[k]; ok {
			t.Errorf("%v named by %#x and %#x", k, prev, u)
		}
		seen[k] = u
		if _, ok := hidPhysical[u]; ok {
			t.Errorf("usage %#x in both tables", u)
		}
	}
}

func TestPressRoute(t *testing.T) {
	cases := []struct {
		name   string
		key    input.Key
		mods   input.Modifiers
		text   string
		textOn bool
		want   keyRoute
		comp   bool
	}{
		{"letter, no text input", input.KeyA, 0, "a", false, routeKey, false},
		{"letter, text input", input.KeyA, 0, "a", true, routeText, false},
		{"shifted letter", input.KeyA, input.ModShift, "A", true, routeText, false},
		{"option letter types", input.KeyE, input.ModAlt, "´", true, routeText, false},
		{"space types", input.KeySpace, 0, " ", true, routeText, false},
		{"keypad digit types", input.KeyNumpad5, 0, "5", true, routeText, false},
		{"cmd shortcut", input.KeyC, input.ModMeta, "c", true, routeKey, false},
		{"ctrl shortcut", input.KeyA, input.ModCtrl, "\x01", true, routeKey, false},
		{"backspace never types", input.KeyBackspace, 0, "", true, routeKey, false},
		{"return never types", input.KeyEnter, 0, "", true, routeKey, false},
		{"keypad enter", input.KeyNumpadEnter, 0, "", true, routeKey, false},
		{"tab", input.KeyTab, 0, "", true, routeKey, false},
		{"escape", input.KeyEscape, 0, "", true, routeKey, false},
		{"arrow", input.KeyLeft, input.ModShift, "", true, routeKey, false},
		{"forward delete", input.KeyDelete, 0, "", true, routeKey, false},
		{"function key", input.KeyF5, 0, "", true, routeKey, false},
		{"modifier", input.KeyShiftLeft, input.ModShift, "", true, routeKey, false},
		{"caps lock", input.KeyCapsLock, 0, "", true, routeKey, false},
		{"unnamed key that types", input.KeyUnknown, 0, "§", true, routeText, false},
		{"unnamed key, no text input", input.KeyUnknown, 0, "§", false, routeSystem, false},
		{"unnamed key with cmd", input.KeyUnknown, input.ModMeta, "§", true, routeSystem, false},
		{"media key", input.KeyUnknown, 0, "", true, routeSystem, false},
		// Composing (an input method on a hardware keyboard): the
		// editing keys edit the marked text; shortcuts stay ours.
		{"backspace composing", input.KeyBackspace, 0, "", true, routeText, true},
		{"return composing", input.KeyEnter, 0, "", true, routeText, true},
		{"arrow composing", input.KeyLeft, 0, "", true, routeText, true},
		{"escape composing", input.KeyEscape, 0, "", true, routeText, true},
		{"letter composing", input.KeyA, 0, "a", true, routeText, true},
		{"cmd composing", input.KeyC, input.ModMeta, "c", true, routeKey, true},
		{"composing, no text input", input.KeyBackspace, 0, "", false, routeKey, true},
	}
	for _, c := range cases {
		if got := pressRoute(c.key, c.mods, c.text, c.textOn, c.comp); got != c.want {
			t.Errorf("%s: pressRoute = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTypedText(t *testing.T) {
	cases := map[string]string{
		"a":                   "a",
		"中":                   "中",
		"\r":                  "",
		"\b":                  "",
		"\x7f":                "",
		"\x1b":                "",
		"":                   "",
		"UIKeyInputLeftArrow": "",
		"é":                   "é",
	}
	for in, want := range cases {
		if got := typedText(in); got != want {
			t.Errorf("typedText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecodePress(t *testing.T) {
	// Shift+A while typing goes to UIKit, which types "A" (insertText:).
	p := decodePress(0x04, uiKeyModShift, "A", "a", true, false)
	if p.route != routeText {
		t.Fatalf("Shift+A: %+v", p)
	}
	// Without text input it is a key with Shift.
	p = decodePress(0x04, uiKeyModShift, "A", "a", false, false)
	if p.route != routeKey || p.ev != (input.KeyEvent{Key: input.KeyA, Down: true, Mods: input.ModShift}) {
		t.Fatalf("Shift+A, no text input: %+v", p)
	}
	// Cmd+V: a shortcut, no text.
	p = decodePress(0x19, uiKeyModCommand, "v", "v", true, false)
	if p.route != routeKey || p.ev.Key != input.KeyV || p.ev.Mods != input.ModMeta {
		t.Fatalf("Cmd+V: %+v", p)
	}
	// Dvorak: Code carries the physical key.
	p = decodePress(0x06, 0, "j", "j", false, false)
	if p.ev.Key != input.KeyJ || p.ev.Code != input.KeyC {
		t.Fatalf("Dvorak j: %+v", p)
	}
	// A modifier's own press includes its bit even when the flags lag.
	p = decodePress(0xE0, 0, "", "", true, false)
	if p.route != routeKey || p.ev.Key != input.KeyCtrlLeft || p.ev.Mods != input.ModCtrl {
		t.Fatalf("Ctrl down: %+v", p)
	}
	// Return while typing: only the key (UIKit never sees it).
	p = decodePress(0x28, 0, "\r", "\r", true, false)
	if p.route != routeKey || p.ev.Key != input.KeyEnter {
		t.Fatalf("Return: %+v", p)
	}
	// The Globe key goes to UIKit.
	if p = decodePress(0x29e, 0, "", "", true, false); p.route != routeSystem {
		t.Fatalf("Globe: %+v", p)
	}
}

func TestReleaseEvent(t *testing.T) {
	down := input.KeyEvent{Key: input.KeyJ, Code: input.KeyC, Down: true, Mods: input.ModShift}
	if got := releaseEvent(down, uiKeyModShift); got != (input.KeyEvent{Key: input.KeyJ, Code: input.KeyC, Mods: input.ModShift}) {
		t.Fatalf("release J: %+v", got)
	}
	// A modifier's own release excludes its bit even when the flags lag.
	shift := input.KeyEvent{Key: input.KeyShiftLeft, Down: true, Mods: input.ModShift}
	if got := releaseEvent(shift, uiKeyModShift|uiKeyModCommand); got.Mods != input.ModMeta || got.Down {
		t.Fatalf("release Shift: %+v", got)
	}
}
