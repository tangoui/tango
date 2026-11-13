package editor

import (
	"unicode"
	"unicode/utf8"
)

// Grapheme clusters.
//
// The standard library has no grapheme segmentation and the module takes no
// new dependency, so this file implements the subset of Unicode extended
// grapheme clusters (UAX #29) that matters for caret movement in code:
//
//   - GB3–GB5: line breaks and other control characters (Cc, U+2028,
//     U+2029) are clusters of their own.
//   - GB6–GB8: Hangul syllables from conjoining jamo (L, V, T) and
//     precomposed LV / LVT syllables.
//   - GB9 / GB9a: Extend and SpacingMark never start a cluster: nonspacing,
//     enclosing and spacing combining marks (Mn, Me, Mc), ZWNJ, ZWJ,
//     variation selectors (U+FE00–FE0F, U+E0100–E01EF), emoji skin-tone
//     modifiers (U+1F3FB–1F3FF) and tag characters (U+E0020–E007F).
//   - GB11: emoji ZWJ sequences — an Extended_Pictographic character,
//     optional Extend, ZWJ, then another Extended_Pictographic character.
//     Extended_Pictographic is approximated by the emoji and pictograph
//     blocks (see isPict).
//   - GB12 / GB13: regional-indicator pairs (flags); a run of indicators
//     splits into pairs from its start.
//
// Not implemented: Prepend (GB9b), Indic conjunct breaks (GB9c, so a
// virama followed by a consonant starts a new cluster), and the exact
// Extended_Pictographic table. Every other boundary falls between code
// points.

type gbClass uint8

const (
	gbOther gbClass = iota
	gbControl
	gbExtend
	gbZWJ
	gbRI
	gbL
	gbV
	gbT
	gbLV
	gbLVT
)

func classOf(r rune) gbClass {
	switch {
	case r < 0x7f:
		if r < 0x20 {
			return gbControl
		}
		return gbOther
	case r == 0x200d:
		return gbZWJ
	case r == 0x200c,
		r >= 0xfe00 && r <= 0xfe0f,
		r >= 0xe0100 && r <= 0xe01ef,
		r >= 0x1f3fb && r <= 0x1f3ff,
		r >= 0xe0020 && r <= 0xe007f:
		return gbExtend
	case r >= 0x1f1e6 && r <= 0x1f1ff:
		return gbRI
	case r >= 0x1100 && r <= 0x115f, r >= 0xa960 && r <= 0xa97c:
		return gbL
	case r >= 0x1160 && r <= 0x11a7, r >= 0xd7b0 && r <= 0xd7c6:
		return gbV
	case r >= 0x11a8 && r <= 0x11ff, r >= 0xd7cb && r <= 0xd7fb:
		return gbT
	case r >= 0xac00 && r <= 0xd7a3:
		if (r-0xac00)%28 == 0 {
			return gbLV
		}
		return gbLVT
	case r == 0x2028, r == 0x2029, unicode.Is(unicode.Cc, r):
		return gbControl
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc):
		return gbExtend
	}
	return gbOther
}

// isPict approximates Extended_Pictographic.
func isPict(r rune) bool {
	switch {
	case r < 0xa9:
		return false
	case r == 0xa9, r == 0xae, r == 0x203c, r == 0x2049, r == 0x2122, r == 0x2139,
		r >= 0x2194 && r <= 0x21aa,
		r >= 0x231a && r <= 0x23ff,
		r == 0x24c2, r >= 0x25aa && r <= 0x25fe,
		r >= 0x2600 && r <= 0x27bf,
		r >= 0x2934 && r <= 0x2935, r >= 0x2b05 && r <= 0x2b55,
		r == 0x3030, r == 0x303d, r == 0x3297, r == 0x3299:
		return true
	case r >= 0x1f000 && r <= 0x1faff:
		return !(r >= 0x1f1e6 && r <= 0x1f1ff) && !(r >= 0x1f3fb && r <= 0x1f3ff)
	case r >= 0x1fc00 && r <= 0x1fffd:
		return true
	}
	return false
}

// nextGrapheme returns the end of the grapheme cluster starting at byte i of
// s (len(s) if i is at or past the end). i is treated as a cluster start.
func nextGrapheme(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	r, size := utf8.DecodeRuneInString(s[i:])
	prev := classOf(r)
	pict := isPict(r) // the cluster so far is Pict Extend*
	zwjPict := false  // the cluster so far is Pict Extend* ZWJ
	ri := 0           // regional indicators in the cluster
	if prev == gbRI {
		ri = 1
	}
	pos := i + size
	for pos < len(s) {
		r, size := utf8.DecodeRuneInString(s[pos:])
		c := classOf(r)
		if !joins(prev, c, r, zwjPict, ri) {
			break
		}
		switch {
		case c == gbZWJ:
			zwjPict, pict = pict, false
		case c == gbExtend:
			zwjPict = false
		case prev == gbZWJ && isPict(r):
			pict, zwjPict = true, false
		default:
			pict, zwjPict = false, false
		}
		if c == gbRI {
			ri++
		}
		prev = c
		pos += size
	}
	return pos
}

// joins reports whether there is no boundary between a character of class
// prev and the next character r of class c.
func joins(prev, c gbClass, r rune, zwjPict bool, ri int) bool {
	switch {
	case prev == gbControl || c == gbControl:
		return false // GB4, GB5
	case prev == gbL && (c == gbL || c == gbV || c == gbLV || c == gbLVT): // GB6
		return true
	case (prev == gbLV || prev == gbV) && (c == gbV || c == gbT): // GB7
		return true
	case (prev == gbLVT || prev == gbT) && c == gbT: // GB8
		return true
	case c == gbExtend || c == gbZWJ: // GB9, GB9a
		return true
	case prev == gbZWJ && zwjPict && isPict(r): // GB11
		return true
	case prev == gbRI && c == gbRI: // GB12, GB13
		return ri%2 == 1
	}
	return false
}

// safeStart reports whether a character always starts a cluster: no rule
// joins it to what precedes it.
func safeStart(r rune) bool {
	c := classOf(r)
	return (c == gbOther || c == gbControl) && !isPict(r)
}

// prevGrapheme returns the start of the grapheme cluster that ends at or
// contains byte i-1 of s (0 if i <= 0).
func prevGrapheme(s string, i int) int {
	if i <= 0 {
		return 0
	}
	i = min(i, len(s))
	// Back up to a character that always starts a cluster, then segment
	// forward from there.
	start := 0
	for j := i; j > 0; {
		r, size := utf8.DecodeLastRuneInString(s[:j])
		j -= size
		if safeStart(r) {
			start = j
			break
		}
	}
	b := start
	for {
		nb := nextGrapheme(s, b)
		if nb >= i {
			return b
		}
		b = nb
	}
}

// graphemeWidth is the number of columns a cluster takes in a monospace
// layout: 2 for East Asian wide and fullwidth characters and emoji
// presentation, otherwise 1. Tabs are handled by the caller.
func graphemeWidth(g string) int {
	r, _ := utf8.DecodeRuneInString(g)
	if isWide(r) {
		return 2
	}
	for _, r := range g {
		if r == 0xfe0f {
			return 2
		}
	}
	return 1
}

func isWide(r rune) bool {
	switch {
	case r < 0x1100:
		return false
	case r <= 0x115f,
		r >= 0x2e80 && r <= 0x303e,
		r >= 0x3041 && r <= 0x33ff,
		r >= 0x3400 && r <= 0x4dbf,
		r >= 0x4e00 && r <= 0x9fff,
		r >= 0xa000 && r <= 0xa4cf,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe4f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f,
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd:
		return true
	}
	return false
}
