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
//     although no right-to-left run follows it logically.

// bidiParaSep reports a bidi paragraph separator other than '\n' (class
// B): the text after it is a new bidi paragraph.
func bidiParaSep(r rune) bool {
	return r == '\r' || r == 0x2029 || r == 0x85 || (r >= 0x1C && r <= 0x1E)
}

// strongMark reports the explicit strong marks that are not letters (LRM,
// RLM, ALM).
func strongMark(r rune) bool { return r == 0x200E || r == 0x200F || r == 0x061C }

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
	wd := int8(ctWritingDirectionNatural)
	switch dir {
	case DirLTR:
		wd = ctWritingDirectionLeftToRight
	case DirRTL:
		wd = ctWritingDirectionRightToLeft
		base = 1
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
	if dir == DirAuto {
		// Rules P2/P3: the first strong character sets the level. Letters
		// are strong (L, R or AL) and lie in a run of their direction; digits
		// and neutrals are weak.
		for i := a; i < b; i++ {
			if r := runes[i]; unicode.IsLetter(r) || strongMark(r) {
				for _, ru := range runs {
					if i >= ru.start && i < ru.end {
						if ru.rtl {
							base = 1
						}
						break
					}
				}
				break
			}
		}
	}
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
