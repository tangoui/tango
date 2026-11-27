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
		if got := imeKey(tc.code, tc.chars, tc.unmodified, tc.flags); got != tc.want {
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
		if got := imeKey(0, "", "a", tc.flags); got != tc.want {
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
