package darwin

import (
	"testing"

	"github.com/tangoui/tango/ui/input"
)

func TestModsFromMac(t *testing.T) {
	got := modsFromMac(nsShiftKeyMask | nsCommandKeyMask | 0x100) // plus a device-dependent bit
	if got != input.ModShift|input.ModMeta {
		t.Fatalf("mods = %v", got)
	}
	if modsFromMac(nsControlKeyMask|nsAlternateKeyMask) != input.ModCtrl|input.ModAlt {
		t.Fatal("ctrl+alt")
	}
}

func TestKeyFromMac(t *testing.T) {
	for _, tc := range []struct {
		code  uint16
		chars string
		want  input.Key
	}{
		{0x24, "\r", input.KeyEnter},
		{0x4C, "\x03", input.KeyNumpadEnter},
		{0x33, "\x7f", input.KeyBackspace},
		{0x75, "", input.KeyDelete},
		{0x7B, "", input.KeyLeft},
		{0x7E, "", input.KeyUp},
		{0x73, "", input.KeyHome},
		{0x79, "", input.KeyPageDown},
		{0x31, " ", input.KeySpace},
		{0x35, "\x1b", input.KeyEscape},
		{0x08, "c", input.KeyC},
		{0x22, "c", input.KeyC}, // Dvorak: the key labelled C sits elsewhere
		{0x00, "A", input.KeyA},
		{0x10, "y", input.KeyY},
		{0x12, "1", input.Key1},
		{0x0B, "b", input.KeyB},
		{0x22, "i", input.KeyI},
		{0x2A, "\\", input.KeyBackslash},
		{0x0A, "§", input.KeyUnknown}, // ISO section key: no Key
		{0x00, "", input.KeyUnknown},
		{0x00, "ab", input.KeyUnknown},
	} {
		if got := keyFromMac(tc.code, tc.chars); got != tc.want {
			t.Errorf("keyFromMac(%#x, %q) = %v, want %v", tc.code, tc.chars, got, tc.want)
		}
	}
}

func TestKeyFromMacNewKeys(t *testing.T) {
	for _, tc := range []struct {
		code  uint16
		chars string
		want  input.Key
	}{
		// Function keys (charactersIgnoringModifiers is a private-use char).
		{0x7A, "\uF704", input.KeyF1}, {0x78, "\uF705", input.KeyF2}, {0x63, "\uF706", input.KeyF3},
		{0x76, "\uF707", input.KeyF4}, {0x60, "\uF708", input.KeyF5}, {0x61, "\uF709", input.KeyF6},
		{0x62, "\uF70A", input.KeyF7}, {0x64, "\uF70B", input.KeyF8}, {0x65, "\uF70C", input.KeyF9},
		{0x6D, "\uF70D", input.KeyF10}, {0x67, "\uF70E", input.KeyF11}, {0x6F, "\uF70F", input.KeyF12},
		// Punctuation by position, whatever the layout produces.
		{0x1B, "-", input.KeyMinus}, {0x18, "=", input.KeyEqual},
		{0x21, "[", input.KeyBracketLeft}, {0x1E, "]", input.KeyBracketRight},
		{0x2A, "\\", input.KeyBackslash}, {0x29, ";", input.KeySemicolon},
		{0x27, "'", input.KeyQuote}, {0x2B, ",", input.KeyComma},
		{0x2F, ".", input.KeyPeriod}, {0x2C, "/", input.KeySlash},
		{0x32, "`", input.KeyBackquote},
		{0x2C, "?", input.KeySlash},   // Shift held
		{0x2C, "-", input.KeySlash},   // German layout: the US slash key
		{0x1B, "ß", input.KeyMinus},   // German layout: the US minus key
		{0x29, "m", input.KeyM},       // AZERTY: a letter wins (layout-aware shortcuts)
		{0x2E, ",", input.KeyUnknown}, // AZERTY: comma on the US M key is not KeyComma
		// Digits: by character, by position when the character is not one.
		{0x12, "&", input.Key1}, // AZERTY
		{0x1D, "à", input.Key0},
		{0x12, "!", input.Key1}, // Shift held
		// Keypad: always Numpad, never the main-block keys.
		{0x52, "0", input.KeyNumpad0}, {0x53, "1", input.KeyNumpad1}, {0x54, "2", input.KeyNumpad2},
		{0x55, "3", input.KeyNumpad3}, {0x56, "4", input.KeyNumpad4}, {0x57, "5", input.KeyNumpad5},
		{0x58, "6", input.KeyNumpad6}, {0x59, "7", input.KeyNumpad7}, {0x5B, "8", input.KeyNumpad8},
		{0x5C, "9", input.KeyNumpad9},
		{0x45, "+", input.KeyNumpadAdd}, {0x4E, "-", input.KeyNumpadSubtract},
		{0x43, "*", input.KeyNumpadMultiply}, {0x4B, "/", input.KeyNumpadDivide},
		{0x41, ".", input.KeyNumpadDecimal}, {0x4C, "\x03", input.KeyNumpadEnter},
		{0x47, "\uF739", input.KeyUnknown}, // keypad Clear
		{0x51, "=", input.KeyUnknown},      // keypad =
		// Others.
		{0x72, "\uF746", input.KeyInsert},
		{0x6E, "\x10", input.KeyContextMenu},
	} {
		if got := keyFromMac(tc.code, tc.chars); got != tc.want {
			t.Errorf("keyFromMac(%#x, %q) = %v, want %v", tc.code, tc.chars, got, tc.want)
		}
	}
}

func TestModifierKeyFromMac(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  uint16
		flags uint
		key   input.Key
		down  bool
		mods  input.Modifiers
	}{
		{"lshift down", 0x38, nsShiftKeyMask | nxDeviceLShiftKeyMask, input.KeyShiftLeft, true, input.ModShift},
		{"lshift up", 0x38, 0, input.KeyShiftLeft, false, 0},
		{"rshift down", 0x3C, nsShiftKeyMask | nxDeviceRShiftKeyMask, input.KeyShiftRight, true, input.ModShift},
		{"rshift up", 0x3C, 0, input.KeyShiftRight, false, 0},
		// Left released while right still held: up, Shift still in Mods.
		{"lshift up, rshift held", 0x38, nsShiftKeyMask | nxDeviceRShiftKeyMask, input.KeyShiftLeft, false, input.ModShift},
		{"lctrl down", 0x3B, nsControlKeyMask | nxDeviceLCtlKeyMask, input.KeyCtrlLeft, true, input.ModCtrl},
		{"rctrl down", 0x3E, nsControlKeyMask | nxDeviceRCtlKeyMask, input.KeyCtrlRight, true, input.ModCtrl},
		{"rctrl up", 0x3E, 0, input.KeyCtrlRight, false, 0},
		{"lalt down", 0x3A, nsAlternateKeyMask | nxDeviceLAltKeyMask, input.KeyAltLeft, true, input.ModAlt},
		{"ralt down", 0x3D, nsAlternateKeyMask | nxDeviceRAltKeyMask, input.KeyAltRight, true, input.ModAlt},
		{"ralt up", 0x3D, 0, input.KeyAltRight, false, 0},
		{"lcmd down", 0x37, nsCommandKeyMask | nxDeviceLCmdKeyMask, input.KeyMetaLeft, true, input.ModMeta},
		{"rcmd down", 0x36, nsCommandKeyMask | nxDeviceRCmdKeyMask, input.KeyMetaRight, true, input.ModMeta},
		{"rcmd up", 0x36, 0, input.KeyMetaRight, false, 0},
		// Other modifiers held stay in Mods.
		{"lshift down with cmd", 0x38, nsShiftKeyMask | nsCommandKeyMask | nxDeviceLShiftKeyMask | nxDeviceLCmdKeyMask, input.KeyShiftLeft, true, input.ModShift | input.ModMeta},
		{"lshift up with cmd", 0x38, nsCommandKeyMask | nxDeviceLCmdKeyMask, input.KeyShiftLeft, false, input.ModMeta},
		// Synthesized events without device bits: the generic bit decides.
		{"synthetic down", 0x38, nsShiftKeyMask, input.KeyShiftLeft, true, input.ModShift},
	} {
		k, down, mods, ok := modifierKeyFromMac(tc.code, tc.flags)
		if !ok || k != tc.key || down != tc.down || mods != tc.mods {
			t.Errorf("%s: = %v %v %v %v, want %v %v %v", tc.name, k, down, mods, ok, tc.key, tc.down, tc.mods)
		}
	}
	for _, code := range []uint16{macCapsLockCode, 0x3F /* Fn */, 0x00} {
		if _, _, _, ok := modifierKeyFromMac(code, nsShiftKeyMask); ok {
			t.Errorf("modifierKeyFromMac(%#x) ok", code)
		}
	}
}

func TestCommandKey(t *testing.T) {
	// Physical key known and agreeing: real modifiers.
	if k, m, ok := commandKey(input.KeyLeft, input.ModShift, "moveLeftAndModifySelection:"); !ok || k != input.KeyLeft || m != input.ModShift {
		t.Fatalf("shift+left = %v %v %v", k, m, ok)
	}
	// Emacs binding: Ctrl+A → Home.
	if k, m, ok := commandKey(input.KeyA, input.ModCtrl, "moveToBeginningOfLine:"); !ok || k != input.KeyHome || m != 0 {
		t.Fatalf("ctrl+a = %v %v %v", k, m, ok)
	}
	// Unknown selector (noop:): the physical key.
	if k, _, ok := commandKey(input.KeyEscape, 0, "noop:"); !ok || k != input.KeyEscape {
		t.Fatalf("noop escape = %v %v", k, ok)
	}
	if _, _, ok := commandKey(input.KeyUnknown, 0, "noop:"); ok {
		t.Fatal("unknown key and selector reported")
	}
	// Keypad Enter through insertNewline: stays NumpadEnter (keyUp: matches).
	if k, _, ok := commandKey(input.KeyNumpadEnter, 0, "insertNewline:"); !ok || k != input.KeyNumpadEnter {
		t.Fatalf("keypad enter = %v %v", k, ok)
	}
	if k, m, ok := keyFromSelector("insertBacktab:"); !ok || k != input.KeyTab || m != input.ModShift {
		t.Fatal("insertBacktab:")
	}
}

func TestCursorSelector(t *testing.T) {
	want := map[input.Cursor]string{
		input.CursorDefault: "arrowCursor", input.CursorPointer: "pointingHandCursor",
		input.CursorText: "IBeamCursor", input.CursorNotAllowed: "operationNotAllowedCursor",
		input.CursorResizeEW: "resizeLeftRightCursor", input.CursorResizeNS: "resizeUpDownCursor",
		input.CursorGrab: "openHandCursor", input.CursorGrabbing: "closedHandCursor",
		input.CursorMove: "openHandCursor", input.Cursor(200): "arrowCursor",
	}
	for c, s := range want {
		if got := cursorSelector(c); got != s {
			t.Errorf("cursorSelector(%d) = %s, want %s", c, got, s)
		}
	}
}

func TestScrollDelta(t *testing.T) {
	// Wheel / fingers moving content up (AppKit delta < 0) reveal content below.
	if dx, dy := scrollDelta(0, -12.5, true); dx != 0 || dy != 12.5 {
		t.Fatalf("precise = %v %v", dx, dy)
	}
	if dx, dy := scrollDelta(1, -1, false); dx != -scrollLine || dy != scrollLine {
		t.Fatalf("lines = %v %v", dx, dy)
	}
}

func TestUTF16(t *testing.T) {
	s := "a你😀b" // 1 + 1 + 2 + 1 UTF-16 units
	if n := utf16Len(s); n != 5 {
		t.Fatalf("utf16Len = %d", n)
	}
	for u16, want := range map[int]int{0: 0, 1: 1, 2: 2, 4: 3, 5: 4, 99: 4} {
		if got := runeIndex(s, u16); got != want {
			t.Errorf("runeIndex(%d) = %d, want %d", u16, got, want)
		}
	}
	if got := cleanInsert("a\x7fb\tc"); got != "abc" {
		t.Fatalf("cleanInsert = %q", got)
	}
}
