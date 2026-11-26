package platform

import (
	"strings"

	"github.com/tangoui/tango/ui/input"
)

// WebKeyRune is the printable character for a WebView chord on the US layout.
func WebKeyRune(key input.Key, mods input.Modifiers) rune {
	r := key.Rune()
	if mods&input.ModShift == 0 || key >= input.KeyNumpad0 && key <= input.KeyNumpadEnter {
		return r
	}
	if r >= 'a' && r <= 'z' {
		return r - 'a' + 'A'
	}
	const plain = "`1234567890-=[]\\;',./"
	const shifted = "~!@#$%^&*()_+{}|:\"<>?"
	if i := strings.IndexRune(plain, r); i >= 0 {
		return rune(shifted[i])
	}
	return r
}
