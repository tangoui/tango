package darwin

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestIMEKey(t *testing.T) {
	for _, tc := range []struct {
		code              uint16
		chars, unmodified string
		flags             uint
		want              platform.IMEKey
	}{
		{0x00, "a", "a", 0, platform.IMEKey{Code: "KeyA", Text: "a"}},
		{0x31, " ", " ", 0, platform.IMEKey{Code: "Space", Text: " "}},
		{0x24, "\r", "\r", 0, platform.IMEKey{Code: "Enter"}},
		{0x33, "\x7f", "\x7f", 0, platform.IMEKey{Code: "Backspace"}},
		{0x35, "\x1b", "\x1b", 0, platform.IMEKey{Code: "Escape"}},
		{0x30, "\t", "\t", 0, platform.IMEKey{Code: "Tab"}},
		{0x7e, "\uf700", "\uf700", 0, platform.IMEKey{Code: "ArrowUp"}},
		{0x00, "A", "a", 1 << 17, platform.IMEKey{Code: "KeyA", Text: "A", Shift: true}},
		{0x12, "1", "1", 0, platform.IMEKey{Code: "Digit1", Text: "1"}},
		{0x4c, "\x03", "\x03", 0, platform.IMEKey{Code: "NumpadEnter"}},
		{0xffff, "ab", "", 0, platform.IMEKey{}},
	} {
		if got := imeKey(tc.code, tc.chars, tc.unmodified, tc.flags, false); got != tc.want {
			t.Errorf("imeKey(%#x, %q) = %+v, want %+v", tc.code, tc.chars, got, tc.want)
		}
	}
}

func TestIMEKeyModifiers(t *testing.T) {
	for _, tc := range []struct {
		flags uint
		want  platform.IMEKey
	}{
		{1 << 20, platform.IMEKey{Code: "KeyA", Meta: true}},
		{1 << 18, platform.IMEKey{Code: "KeyA", Ctrl: true}},
		{1 << 19, platform.IMEKey{Code: "KeyA", Alt: true}},
	} {
		if got := imeKey(0, "", "a", tc.flags, false); got != tc.want {
			t.Errorf("imeKey flags %#x = %+v, want %+v", tc.flags, got, tc.want)
		}
	}
}

func TestUTF16Cursor(t *testing.T) {
	for _, tc := range []struct {
		text         string
		cursor, want int
	}{
		{"你好", 2, 2},
		{"a😀b", 2, 3},
		{"a😀b", 8, 4},
		{"a😀b", -1, 0},
		{"a😀b", 0, 0},
	} {
		if got := utf16Offset(tc.text, tc.cursor); got != tc.want {
			t.Errorf("utf16Offset(%q, %d) = %d, want %d", tc.text, tc.cursor, got, tc.want)
		}
	}
}

func TestIMEModifierFromFlags(t *testing.T) {
	for _, tc := range []struct {
		code  uint16
		flags uint
		want  platform.IMEModifierKey
		ok    bool
	}{
		{0x38, 1 << 17, platform.IMEModifierKey{Code: "ShiftLeft", Down: true}, true},
		{0x38, 0, platform.IMEModifierKey{Code: "ShiftLeft"}, true},
		{0x3c, 1 << 17, platform.IMEModifierKey{Code: "ShiftRight", Down: true}, true},
		{0x38, 1<<17 | nxDeviceRShiftKeyMask, platform.IMEModifierKey{Code: "ShiftLeft"}, true},
		{0x39, 1 << 16, platform.IMEModifierKey{Code: "CapsLock", Down: true, CapsLock: true}, true},
		{0x39, 0, platform.IMEModifierKey{Code: "CapsLock"}, true},
		{0x37, 1 << 20, platform.IMEModifierKey{Code: "MetaLeft", Down: true}, true},
		{0x3b, 1 << 18, platform.IMEModifierKey{Code: "ControlLeft", Down: true}, true},
		{0x3f, 0, platform.IMEModifierKey{}, false},
	} {
		got, ok := imeModifierFromFlags(tc.code, tc.flags)
		if got != tc.want || ok != tc.ok {
			t.Errorf("modifier %#x flags %#x = %+v, %t, want %+v, %t", tc.code, tc.flags, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRecognizedEventsMask(t *testing.T) {
	if got := imeRecognizedEventsMask; got != 1<<10|1<<12 {
		t.Fatalf("recognized events mask = %#x", got)
	}
}

func TestModeIDMapping(t *testing.T) {
	const bundle = "com.x.inputmethod.p"
	if got := imeModeSourceID(bundle, "en"); got != bundle+".en" {
		t.Fatalf("source ID = %q", got)
	}
	for source, want := range map[string]string{
		bundle + ".en": "en",
		bundle + ".":   "",
		bundle:         "",
		"com.y.en":     "",
	} {
		if got := imeModeID(bundle, source); got != want {
			t.Errorf("mode ID for %q = %q, want %q", source, got, want)
		}
	}
}

func TestKeyCapsRepeat(t *testing.T) {
	got := imeKey(0, "A", "a", 1<<16, true)
	if got != (platform.IMEKey{Code: "KeyA", Text: "A", CapsLock: true, Repeat: true}) {
		t.Fatalf("key = %+v", got)
	}
}
