package text

// level is a bidi embedding level (UAX #9): even left-to-right, odd
// right-to-left. Every text engine resolves levels into it (paraLevels).
type level uint8

// rtl reports a right-to-left level.
func (l level) rtl() bool { return l&1 == 1 }
