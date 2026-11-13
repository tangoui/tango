package mobilecanvas

import (
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// UTF-16 offsets. UIKit (NSString, NSRange) and Android (CharSequence)
// count text in UTF-16 code units; Tango's input events count runes. A
// rune outside the Basic Multilingual Plane (most emoji) is one rune but
// two UTF-16 units (a surrogate pair). Combining marks are runes of their
// own in both counts. Every helper clamps out-of-range offsets, and a
// UTF-16 offset that falls between the halves of a surrogate pair rounds
// down to the start of that rune.

// UTF16Len returns the length of s in UTF-16 code units.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16RuneLen(r)
	}
	return n
}

// RuneToUTF16 converts a rune offset into s to a UTF-16 offset.
func RuneToUTF16(s string, runes int) int {
	n, i := 0, 0
	for _, r := range s {
		if i >= runes {
			break
		}
		n += utf16RuneLen(r)
		i++
	}
	return n
}

// UTF16ToRune converts a UTF-16 offset into s to a rune offset.
func UTF16ToRune(s string, u int) int {
	n, i := 0, 0
	for _, r := range s {
		n += utf16RuneLen(r)
		if n > u {
			return i
		}
		i++
	}
	return i
}

// UTF16ToByte converts a UTF-16 offset into s to a byte offset.
func UTF16ToByte(s string, u int) int {
	n := 0
	for b, r := range s {
		n += utf16RuneLen(r)
		if n > u {
			return b
		}
	}
	return len(s)
}

// RuneToByte converts a rune offset into s to a byte offset.
func RuneToByte(s string, runes int) int {
	i := 0
	for b := range s {
		if i >= runes {
			return b
		}
		i++
	}
	return len(s)
}

// SliceUTF16 returns s[start:end] with the bounds in UTF-16 units. Bounds
// are clamped and ordered.
func SliceUTF16(s string, start, end int) string {
	if end < start {
		start, end = end, start
	}
	return s[UTF16ToByte(s, start):UTF16ToByte(s, end)]
}

// utf16RuneLen is utf16.RuneLen with invalid runes (the encoding of bad
// UTF-8) counted as the one U+FFFD unit NSString and Java make of them.
func utf16RuneLen(r rune) int {
	if n := utf16.RuneLen(r); n > 0 {
		return n
	}
	return 1
}

// PrevBoundary returns the rune offset of the user-perceived character
// that ends at rune offset i of s, so one Backspace removes it whole: a
// base letter with its combining marks, an emoji with its variation
// selector, skin-tone modifier and zero-width-joiner sequence, a flag (a
// regional-indicator pair), a keycap, CR LF. It is a simplification of
// Unicode text segmentation (UAX #29) for those cases; Hangul syllables
// composed of jamo are left to the input method. i is clamped.
func PrevBoundary(s string, i int) int {
	rs := []rune(s)
	if i > len(rs) {
		i = len(rs)
	}
	if i <= 0 {
		return 0
	}
	j := i - 1
	// Step over extending runes to their base.
	for j > 0 && extends(rs[j]) {
		j--
	}
	switch {
	case rs[j] == '\n' && j > 0 && rs[j-1] == '\r':
		return j - 1
	case isRegional(rs[j]):
		// Flags pair regional indicators from the start of the run.
		k := j
		for k > 0 && isRegional(rs[k-1]) {
			k--
		}
		if (j-k)%2 == 1 {
			return j - 1
		}
		return j
	}
	// Zero-width-joiner sequences: base ZWJ base ...
	for j >= 2 && rs[j-1] == zwj {
		k := j - 2
		for k > 0 && extends(rs[k]) && rs[k] != zwj {
			k--
		}
		if !isPictographic(rs[k]) {
			break
		}
		j = k
	}
	return j
}

// NextBoundary returns the rune offset of the end of the user-perceived
// character that starts at rune offset i of s (see PrevBoundary). i is
// clamped.
func NextBoundary(s string, i int) int {
	n := utf8.RuneCountInString(s)
	if i < 0 {
		i = 0
	}
	if i >= n {
		return n
	}
	// Walk boundaries from the end until the one at or before i; text
	// fields are short, so the quadratic worst case does not matter.
	end := n
	for {
		b := PrevBoundary(s, end)
		if b <= i {
			return end
		}
		end = b
	}
}

const zwj = '‍'

// extends reports runes that attach to the rune before them.
func extends(r rune) bool {
	switch {
	case r == zwj:
		return true
	case r >= 0xFE00 && r <= 0xFE0F, r >= 0xE0100 && r <= 0xE01EF: // variation selectors
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF: // emoji skin-tone modifiers
		return true
	case r >= 0xE0020 && r <= 0xE007F: // tag characters (subdivision flags)
		return true
	case r == 0x20E3: // combining enclosing keycap
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc)
}

func isRegional(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

// isPictographic approximates Extended_Pictographic: the emoji blocks and
// the older symbol ranges emoji sequences use.
func isPictographic(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF:
		return true
	case r >= 0x2600 && r <= 0x27BF: // miscellaneous symbols, dingbats
		return true
	case r >= 0x2190 && r <= 0x21FF, r >= 0x2300 && r <= 0x23FF, r >= 0x2B00 && r <= 0x2BFF:
		return true
	case r == 0x00A9, r == 0x00AE, r == 0x203C, r == 0x2049, r == 0x2122, r == 0x2139, r == 0x3030, r == 0x303D:
		return true
	}
	return false
}
