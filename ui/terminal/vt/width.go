package vt

//go:generate go run ./internal/widthgen -data $UNICODE_DATA

// runeRange is an inclusive range of code points.
type runeRange struct{ lo, hi rune }

// Width reports how many cells r occupies: 0 for combining marks, format
// and other zero-width characters, 2 for East Asian wide and full-width
// characters and for emoji with emoji presentation, and 1 otherwise. The
// rules match xterm.js's unicode11 addon; the tables come from the
// Unicode version in width_table.go.
//
// C0 and C1 controls report 0; the screen never stores them. Values
// outside the Unicode range report 1.
func Width(r rune) int {
	switch {
	case r < 0x20:
		if r < 0 {
			return 1
		}
		return 0
	case r < 0x7F:
		return 1
	case r < 0xA0:
		return 0
	case r > 0x10FFFF:
		return 1
	case inTable(zeroWidth, r):
		return 0
	case inTable(wideWidth, r):
		return 2
	}
	return 1
}

// inTable reports whether r falls in one of the sorted ranges t.
func inTable(t []runeRange, r rune) bool {
	if len(t) == 0 || r < t[0].lo || r > t[len(t)-1].hi {
		return false
	}
	lo, hi := 0, len(t)
	for lo < hi {
		m := int(uint(lo+hi) >> 1)
		switch {
		case r < t[m].lo:
			hi = m
		case r > t[m].hi:
			lo = m + 1
		default:
			return true
		}
	}
	return false
}
