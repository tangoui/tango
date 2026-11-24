//go:build darwin

package darwin

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

// AppKit's character-coordinate notification is a synchronous round trip
// (4 ms on a background window's first text input). It only matters while
// the canvas is the active input client: an unfocused canvas defers it to
// the moment it gains focus.
func TestIMECoordinatesNotify(t *testing.T) {
	a, b := geom.Rect{X: 1, Y: 2, W: 1, H: 16}, geom.Rect{X: 9, Y: 2, W: 1, H: 16}
	for _, tc := range []struct {
		name                 string
		focused, wasOn, isOn bool
		old, caret           geom.Rect
		want                 bool
	}{
		{"focused text input starts", true, false, true, a, a, true},
		{"focused caret moves", true, true, true, a, b, true},
		{"focused caret unchanged", true, true, true, a, a, false},
		{"text input off", true, true, false, a, b, false},
		{"unfocused text input starts", false, false, true, a, a, false},
		{"unfocused caret moves", false, true, true, a, b, false},
	} {
		if got := imeCoordinatesChanged(tc.focused, tc.wasOn, tc.isOn, tc.old, tc.caret); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	for _, tc := range []struct {
		focused, textOn, want bool
	}{{true, true, true}, {true, false, false}, {false, true, false}} {
		if got := imeCoordinatesOnFocus(tc.focused, tc.textOn); got != tc.want {
			t.Errorf("focus %v with text input %v: got %v, want %v", tc.focused, tc.textOn, got, tc.want)
		}
	}
}
