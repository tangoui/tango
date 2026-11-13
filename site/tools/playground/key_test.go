package main

import (
	"github.com/tangoui/tango/ui/input"
	"testing"
)

func TestBrowserKeyMapping(t *testing.T) {
	for code, want := range map[string]input.Key{"ArrowLeft": input.KeyLeft, "KeyA": input.KeyA, "Digit3": input.Key3, "Space": input.KeySpace, "Backspace": input.KeyBackspace, "ControlLeft": input.KeyCtrlLeft, "NumpadEnter": input.KeyNumpadEnter} {
		if got := browserKey(code); got != want {
			t.Errorf("%s=%v want %v", code, got, want)
		}
	}
	if browserKey("Unidentified") != input.KeyUnknown {
		t.Fatal("unknown key mapped")
	}
}
