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
