//go:build windows && (amd64 || arm64)

package windows

import "testing"

// TestWebMessageText: messages come through get_WebMessageAsJson whole,
// including the U+0000 that starts every core internal message (it used
// to be cut there by TryGetWebMessageAsString); non-strings are refused.
func TestWebMessageText(t *testing.T) {
	cases := []struct {
		json string
		text string
		ok   bool
	}{
		{`"hello"`, "hello", true},
		{`""`, "", true},
		{`"\u0000tango-int:hotkey:1"`, "\x00tango-int:hotkey:1", true},
		{`"a\"b\\c\né😀"`, "a\"b\\c\né😀", true},
		{`null`, "", false},
		{`{"a":1}`, "", false},
		{`42`, "", false},
		{``, "", false},
		{`"unterminated`, "", false},
	}
	for _, c := range cases {
		text, ok := webMessageText(c.json)
		if text != c.text || ok != c.ok {
			t.Errorf("webMessageText(%q) = %q, %v; want %q, %v", c.json, text, ok, c.text, c.ok)
		}
	}
}
