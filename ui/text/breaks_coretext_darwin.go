//go:build darwin && !ios && tango_coretext

package text

import "unicode/utf8"

// utf16Len is the number of UTF-16 code units r encodes to in a CFString
// built by cfStringRunes (invalid runes become one U+FFFD unit).
func utf16Len(r rune) int {
	if r > 0xFFFF && r <= utf8.MaxRune {
		return 2
	}
	return 1
}

// utf16Index maps every UTF-16 offset into the CFString of runes[a:b]
// (cfStringRunes) to the absolute rune index holding it: toRune[u] is the
// rune whose code units include u (the low surrogate of an astral rune maps
// to that rune), and toRune[len] == b.
func utf16Index(runes []rune, a, b int) (toRune []int32) {
	n := 0
	for _, r := range runes[a:b] {
		n += utf16Len(r)
	}
	toRune = make([]int32, 0, n+1)
	for i := a; i < b; i++ {
		for k := utf16Len(runes[i]); k > 0; k-- {
			toRune = append(toRune, int32(i))
		}
	}
	return append(toRune, int32(b))
}

// cfStringRunes returns a new CFString (the caller releases it) holding
// runes[a:b] as UTF-16, offsets as utf16Index describes.
func cfStringRunes(runes []rune, a, b int) uintptr {
	u := make([]uint16, 0, b-a+1)
	for _, r := range runes[a:b] {
		switch {
		case utf16Len(r) == 2:
			r -= 0x10000
			u = append(u, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
		case r < 0 || (r >= 0xD800 && r <= 0xDFFF):
			u = append(u, 0xFFFD)
		default:
			u = append(u, uint16(r))
		}
	}
	if len(u) == 0 {
		return cfStringCreateWithCharacters(0, nil, 0)
	}
	return cfStringCreateWithCharacters(0, &u[0], int64(len(u)))
}

// lineEnds returns the line break opportunities of runes[a:b] (CoreFoundation's
// UAX #14 line break tokenizer, kCFStringTokenizerUnitLineBreak with no
// locale): rune indices a break may come before, ascending, ending with b.
//
// breakAt wants go-text's segmenter shape, where every line segment keeps
// its trailing spaces ("hello world foo" -> 6, 12, 15). The tokenizer's
// tokens already have that shape (they tile the string, each one ending
// after its trailing spaces: "hello " "world " "foo"), so a token end is the
// next token's start. A break is taken at the start of every token after
// the first, which stays right should a token ever leave a gap (skipped
// whitespace); b closes the list.
func lineEnds(runes []rune, a, b int) []int {
	if b <= a || !ctLoaded() {
		return []int{b}
	}
	toRune := utf16Index(runes, a, b)
	s := cfStringRunes(runes, a, b)
	defer cfRelease(s)
	tk := cfStringTokenizerCreate(0, s, cfRange{0, int64(len(toRune) - 1)}, cfStringTokenizerUnitLineBreak, 0)
	if tk == 0 {
		return []int{b}
	}
	defer cfRelease(tk)
	var ends []int
	for cfStringTokenizerAdvanceToNextToken(tk) != 0 {
		r := cfStringTokenizerGetCurrentTokenRange(tk)
		if r.Loc <= 0 || r.Loc >= int64(len(toRune)-1) {
			continue
		}
		e := int(toRune[r.Loc])
		if len(ends) == 0 || e > ends[len(ends)-1] {
			ends = append(ends, e)
		}
	}
	return append(ends, b)
}
