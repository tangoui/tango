//go:build darwin && !ios && tango_coretext

package text

import (
	"unicode"
	"unsafe"
)

// UAX #9 embedding levels from CoreText: the paragraph is laid out as one
// CTLine (CTTypesetterCreateLine) with the base writing direction in its
// paragraph style, and the levels are derived from the line's glyph runs.
//
// CoreText exposes only the direction of a run (CTRunGetStatus), not its
// level, so the levels are rebuilt from the directions and the visual order
// of the runs (CTLineGetGlyphRuns lists them left to right), assuming no
// explicit embeddings (levels at most base+2):
//
//   - a right-to-left run takes the smallest odd level >= base;
//   - with an odd base, a left-to-right run takes base+1;
//   - with an even base, a left-to-right run nested in a right-to-left
//     stretch takes base+2, any other takes base. A stretch at an odd level
//     is displayed reversed, so its runs follow one another visually with
//     descending logical starts; a left-to-right run joined that way to a
//     right-to-left run (directly or through other such runs) is nested.
//     "abc שלום 123 def" lays out as "abc " "123" "שלום " " def": "123"
//     (9) comes visually before "שלום " (4), so it is nested (level 2)
//     although no right-to-left run follows it logically. Runs cut by
//     font alone are merged first (mergeDirRuns).
//
// The paragraph level comes from Options.Direction or, for DirAuto, from
// firstStrong (rules P2/P3), and the line is always laid out with that
// explicit base direction, so the levels and CoreText's layout agree on it.

// bidiParaSep reports a bidi paragraph separator other than '\n' (class
// B): the text after it is a new bidi paragraph.
func bidiParaSep(r rune) bool {
	return r == '\r' || r == 0x2029 || r == 0x85 || (r >= 0x1C && r <= 0x1E)
}

// rtlBlock reports a rune in a block of right-to-left scripts (Hebrew,
// Arabic, Syriac, Thaana, NKo, Samaritan, Mandaic, their supplements and
// presentation forms, and the astral right-to-left blocks), where every
// character defaults to bidi class R or AL.
func rtlBlock(r rune) bool {
	return (r >= 0x0590 && r <= 0x08FF) || (r >= 0xFB1D && r <= 0xFDFF) || (r >= 0xFE70 && r <= 0xFEFE) ||
		(r >= 0x10800 && r <= 0x10FFF) || (r >= 0x1E800 && r <= 0x1EFFF)
}

// rtlWeak lists the characters of the right-to-left blocks that are not
// strong (UAX #9 classes AN, CS, ET, ON): number signs, separators and
// ornaments. Marks, format characters and digits are caught by category.
func rtlWeak(r rune) bool {
	switch {
	case r >= 0x0600 && r <= 0x0605, // number signs (AN)
		r == 0x0609, r == 0x060A, // per mille / per ten thousand (ET)
		r == 0x060C,              // Arabic comma (CS)
		r == 0x060E, r == 0x060F, // poetic verse / misra signs (ON)
		r >= 0x066A && r <= 0x066C,            // percent, decimal and thousands separators
		r == 0x06DD, r == 0x06DE, r == 0x06E9, // end of ayah (AN), ornaments (ON)
		r == 0x0890, r == 0x0891, r == 0x08E2, // pound / piastre marks, disputed end of ayah (AN)
		r >= 0x07F6 && r <= 0x07F9, // NKo symbols and punctuation (ON)
		r == 0xFD3E, r == 0xFD3F,   // ornate parentheses (ON)
		r == 0xFDCF, r >= 0xFDFD && r <= 0xFDFF: // ornaments (ON)
		return true
	}
	return false
}

// strongDir returns the direction of r if it is a strong character for
// rules P2/P3 (bidi class L, R or AL): 1 right-to-left, 0 left-to-right,
// -1 not strong. It follows the Unicode character database closely enough
// for paragraph direction without carrying its bidi table:
//
//   - LRM is L; RLM and ALM are R/AL;
//   - in the right-to-left blocks everything but marks (NSM), format
//     characters (BN), digits (AN/EN) and the rtlWeak list is R/AL, which
//     takes in their punctuation (؟ ؛ ۔ ־ ׀ ׃ ׆);
//   - elsewhere letters, spacing marks (Mc), letter numbers (Nl: Ⅻ) and
//     circled letters (Ⓐ, So but L) are L.
func strongDir(r rune) int {
	switch r {
	case 0x200E:
		return 0
	case 0x200F, 0x061C:
		return 1
	}
	if rtlBlock(r) {
		if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Nd) || rtlWeak(r) || !unicode.IsGraphic(r) {
			return -1
		}
		return 1
	}
	if unicode.IsLetter(r) || unicode.In(r, unicode.Mc, unicode.Nl) || (r >= 0x24B6 && r <= 0x24E9) {
		return 0
	}
	return -1
}

// firstStrong applies rules P2/P3 to runes[a:b]: the direction of the first
// strong character outside isolates (LRI, RLI, FSI up to the matching PDI),
// 0 when there is none.
func firstStrong(runes []rune, a, b int) level {
	depth := 0
	for _, r := range runes[a:b] {
		switch {
		case r >= 0x2066 && r <= 0x2068: // LRI, RLI, FSI
			depth++
		case r == 0x2069: // PDI
			depth = max(depth-1, 0)
		case depth == 0:
			if d := strongDir(r); d >= 0 {
				return level(d)
			}
		}
	}
	return 0
}

// paraLevels resolves the embedding levels of runes[a:b] (one paragraph,
// no '\n') into lv[a:b] and returns the paragraph level. mixed reports
// whether the paragraph needs bidi treatment at all (a rune at a non-zero
// level or a right-to-left paragraph), as in the go-text engine: prepared.go
// itemizes by level only when mixed, so a paragraph that is right-to-left
// throughout must report mixed.
func paraLevels(runes []rune, a, b int, dir Direction, lv []level) (base level, mixed bool) {
	if dir == DirRTL {
		base = 1
	}
	first := true
	for a < b {
		n := a
		for n < b && !bidiParaSep(runes[n]) {
			n++
		}
		pb := pieceLevels(runes, a, n, dir, lv)
		if first && dir == DirAuto {
			base = pb
		}
		first = false
		for _, v := range lv[a:n] {
			mixed = mixed || v != 0
		}
		a = n
		if a < b { // a separator takes the paragraph level
			lv[a] = base
			a++
		}
	}
	return base, mixed || base != 0
}

// pieceLevels resolves runes[a:b] (no paragraph separator) into lv[a:b]
// and returns its paragraph level.
func pieceLevels(runes []rune, a, b int, dir Direction, lv []level) level {
	var base level
	switch dir {
	case DirRTL:
		base = 1
	case DirAuto:
		base = firstStrong(runes, a, b)
	}
	wd := int8(ctWritingDirectionLeftToRight)
	if base == 1 {
		wd = ctWritingDirectionRightToLeft
	}
	fill := func(v level) {
		for i := a; i < b; i++ {
			lv[i] = v
		}
	}
	if b <= a {
		return base
	}
	if !ctLoaded() {
		fill(base)
		return base
	}
	runs := ctDirRuns(runes, a, b, wd)
	if runs == nil {
		fill(base)
		return base
	}
	runs = mergeDirRuns(runs)
	fill(base)
	odd := base | 1
	// Nested left-to-right runs (even base): mark the runs of every visual
	// chain with descending logical starts that holds a right-to-left run.
	nested := make([]bool, len(runs))
	if base&1 == 0 {
		for i := 0; i < len(runs); {
			j := i + 1
			for j < len(runs) && runs[j].start < runs[j-1].start {
				j++
			}
			hasRTL := false
			for _, ru := range runs[i:j] {
				hasRTL = hasRTL || ru.rtl
			}
			for k := i; k < j; k++ {
				nested[k] = hasRTL
			}
			i = j
		}
	}
	for k, ru := range runs {
		v := base
		switch {
		case ru.rtl:
			v = odd
		case base&1 == 1:
			v = base + 1
		case nested[k]:
			v = base + 2
		}
		for i := ru.start; i < ru.end; i++ {
			lv[i] = v
		}
	}
	return base
}

// mergeDirRuns joins visually adjacent runs of one direction that are also
// logically contiguous (in reading order: left to right for left-to-right
// runs, right to left for right-to-left ones). CoreText cuts runs by font
// as well as direction, and a nested left-to-right stretch split in two
// ("12" and the fallback-font "３" in "abc שלום 12３ def") shows its parts
// in ascending logical order, which would cut the descending chain that
// marks it nested. The merge cannot join a nested run with an unnested
// one: a right-to-left stretch starts with a right-to-left character, so
// its visual ends are never a left-to-right run contiguous with an outside
// one. runs is modified in place.
func mergeDirRuns(runs []ctDirRun) []ctDirRun {
	out := runs[:0]
	for _, ru := range runs {
		if k := len(out) - 1; k >= 0 && out[k].rtl == ru.rtl {
			p := &out[k]
			if !ru.rtl && p.end == ru.start {
				p.end = ru.end
				continue
			}
			if ru.rtl && ru.end == p.start {
				p.start = ru.start
				continue
			}
		}
		out = append(out, ru)
	}
	return out
}

// ctDirRun is a glyph run of a CTLine: absolute rune range and direction.
type ctDirRun struct {
	start, end int
	rtl        bool
}

// ctDirRuns lays out runes[a:b] as one CTLine with base writing direction
// wd and returns its runs in visual order (left to right); nil on failure.
func ctDirRuns(runes []rune, a, b int, wd int8) []ctDirRun {
	toRune := utf16Index(runes, a, b)
	s := cfStringRunes(runes, a, b)
	if s == 0 {
		return nil
	}
	defer cfRelease(s)
	set := ctParagraphStyleSetting{Spec: ctParagraphStyleSpecifierBaseDirection, Size: 1, Value: unsafe.Pointer(&wd)}
	ps := ctParagraphStyleCreate(&set, 1)
	if ps == 0 {
		return nil
	}
	defer cfRelease(ps)
	attrs := cfDict([]uintptr{kCTParagraphStyleAttributeName}, []uintptr{ps})
	if attrs == 0 {
		return nil
	}
	defer cfRelease(attrs)
	as := cfAttributedStringCreate(0, s, attrs)
	if as == 0 {
		return nil
	}
	defer cfRelease(as)
	ts := ctTypesetterCreateWithAttributedString(as)
	if ts == 0 {
		return nil
	}
	defer cfRelease(ts)
	line := ctTypesetterCreateLine(ts, cfRange{0, 0}) // length 0: the whole string
	if line == 0 {
		return nil
	}
	defer cfRelease(line)
	arr := ctLineGetGlyphRuns(line) // owned by line
	n := cfArrayGetCount(arr)
	last := int64(len(toRune) - 1)
	out := make([]ctDirRun, 0, n)
	for k := int64(0); k < n; k++ {
		run := cfArrayGetValueAtIndex(arr, k)
		r := ctRunGetStringRange(run)
		lo, hi := r.Loc, r.Loc+r.Len
		if lo < 0 || hi > last || lo >= hi {
			continue
		}
		out = append(out, ctDirRun{
			start: int(toRune[lo]),
			end:   int(toRune[hi-1]) + 1,
			rtl:   ctRunGetStatus(run)&ctRunStatusRightToLeft != 0,
		})
	}
	return out
}
