package text

import "unicode/utf8"

// simpleNumericText reports whether s is a short number label (digits, '.',
// '-', '+'), the text simpleNumericLayout handles.
func simpleNumericText(s string) bool {
	if s == "" || len(s) > 31 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r == '.' || r == '-' || r == '+') {
			return false
		}
	}
	return true
}

// numericFaceKey identifies the numeric face of a style (numericFaces).
type numericFaceKey struct {
	st      Style
	lang    string
	version int
}

// numericChars are the runes simpleNumericText accepts; numericIndex maps a
// rune to its slot in numericGlyphTable.
const numericChars = "0123456789.-+"

func numericIndex(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r == '.':
		return 10
	case r == '-':
		return 11
	case r == '+':
		return 12
	}
	return -1
}
