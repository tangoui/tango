//go:build darwin

package macoslite

import (
	"testing"

	"github.com/ebitengine/purego"
)

// The view and app delegate must implement these themselves, not inherit
// NSView/NSObject defaults.
func TestClassesOverride(t *testing.T) {
	if err := loadNative(); err != nil {
		t.Skip(err)
	}
	if err := registerClasses(); err != nil {
		t.Fatal(err)
	}
	var getImp func(cls id, op uintptr) uintptr
	var respondsTo func(cls id, op uintptr) bool
	purego.RegisterLibFunc(&getImp, libObjC, "class_getMethodImplementation")
	purego.RegisterLibFunc(&respondsTo, libObjC, "class_respondsToSelector")
	for _, s := range []string{"viewDidChangeBackingProperties", "updateLayer", "keyDown:", "mouseDown:"} {
		if getImp(class("MacosLiteView"), uintptr(sel(s))) == getImp(class("NSView"), uintptr(sel(s))) {
			t.Errorf("MacosLiteView does not override %s", s)
		}
	}
	if !respondsTo(class("MacosLiteAppDelegate"), uintptr(sel("applicationShouldTerminate:"))) {
		t.Error("app delegate does not implement applicationShouldTerminate:")
	}
}

func TestKeyName(t *testing.T) {
	for _, c := range []struct {
		code  uint16
		chars string
		want  string
	}{{53, "\x1b", "Escape"}, {36, "\r", "Enter"}, {76, "\x03", "Enter"}, {48, "\t", "Tab"}, {51, "\x7f", "Backspace"}, {0, "a", "a"}, {24, "=", "="}} {
		if got := keyName(c.code, c.chars); got != c.want {
			t.Errorf("keyName(%d, %q) = %q, want %q", c.code, c.chars, got, c.want)
		}
	}
}
