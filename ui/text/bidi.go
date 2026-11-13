package text

import (
	"math"
	"reflect"
	"slices"
	"sort"
	"sync/atomic"
	"unicode"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/language"
	"github.com/tangoui/tango/ui/geom"
)

// Bidirectional text (UAX #9) and language tags.
//
// Pipeline (per paragraph, i.e. per '\n'-separated block):
//
//  1. paraLevels resolves the embedding level of every rune with the
//     UAX #9 implementation of go-text (bidi.Paragraph); the paragraph
//     level comes from Options.Direction (Auto: rules P2/P3).
//  2. itemize cuts the paragraph into shaping items: maximal ranges with
//     one embedding level (and, later, one style). Each item is shaped on
//     its own with the matching Direction; shaping.Segmenter further splits
//     it by script (detected per rune with go-text's language tables) and
//     font face, and resolves Language per script.
//  3. Lines are broken in logical order on the logical advances (UAX #14,
//     breakLines), then every line is reordered for display: rule L1
//     (trailing whitespace and tabs back to the paragraph level) and rule
//     L2 (reverse every run at level >= k, from the highest level down to
//     the lowest odd one), see lineLevels / visualOrder.
//
// A line holding only left-to-right runs in a left-to-right paragraph takes
// the original (pre-bidi) code path, so pure LTR layouts are unchanged.

// Direction is the base direction of a paragraph (UAX #9 paragraph
// embedding level). It lives in Options rather than Style because it is a
// property of the paragraph, not of a run: a future ShapeRuns(text, runs,
// opt) gives every run its own Style but one direction for the paragraph.
type Direction uint8

const (
	// DirAuto takes the direction of the first strong character of each
	// paragraph (rules P2/P3); paragraphs without one are left-to-right.
	DirAuto Direction = iota
	// DirLTR forces a left-to-right paragraph level.
	DirLTR
	// DirRTL forces a right-to-left paragraph level.
	DirRTL
)

// defaultLangTag is the language used when Style.Lang is empty.
var defaultLangTag atomic.Value // string

func init() { defaultLangTag.Store("zh-Hans") }

// SetDefaultLang sets the BCP 47 language tag used for text whose
// Style.Lang is empty (the application language; ui.SetDefaultLocale calls
// it). The language picks script-specific shaping (OpenType locl, e.g.
// Japanese vs Simplified Chinese ideograph forms). An empty tag restores
// the initial "zh-Hans".
func SetDefaultLang(tag string) {
	if tag == "" {
		tag = "zh-Hans"
	}
	defaultLangTag.Store(tag)
}

// DefaultLang returns the tag set by SetDefaultLang ("zh-Hans" initially).
func DefaultLang() string { return defaultLangTag.Load().(string) }

// resolveLang returns the language tag text of style st is shaped with.
func resolveLang(st Style) string {
	if st.Lang != "" {
		return st.Lang
	}
	return DefaultLang()
}

// item is one shaping segment: runes [start,end) of the layout's text shaped
// with one style at one bidi embedding level. Layout builds the item list
// from the bidi levels (cut where the level changes); ShapeRuns also cuts
// it at style-run boundaries (splitRuns), so shaping code only ever sees
// items.
type item struct {
	start, end int
	st         Style
	lang       language.Language
	level      bidi.Level // odd: right-to-left
	run        int32      // style run (ShapeRuns); 0 for Shape
}

func (it item) rtl() bool { return it.level&1 == 1 }

// bpara is reused by paraLevels; it is only used under mu (from layout).
var bpara bidi.Paragraph

// runLevels copies the per-rune embedding levels out of runs into dst.
//
// go-text's bidi package (the UAX #9 implementation shaping.Segmenter
// itself uses) resolves exact levels but only exposes runs cut where the
// direction flips, labelled with the level of their first rune: in
// "abc שלום 123 def" the run "123 def" is reported at level 2 although
// " def" is at 0, and L2 needs the exact levels. Runs keeps them in its
// unexported levels field, read here with reflect (read-only, no unsafe).
// TestBidiExactLevels fails if a go-text update moves it; the fallback
// (run levels) then still gets every rune's direction right.
func runLevels(runs bidi.Runs, dst []bidi.Level) {
	if f := reflect.ValueOf(runs).FieldByName("levels"); f.IsValid() && f.Kind() == reflect.Slice && f.Len() >= len(dst) {
		for i := range dst {
			dst[i] = bidi.Level(f.Index(i).Int())
		}
		return
	}
	for k := 0; k < runs.NumRuns(); k++ {
		r := runs.Run(k)
		for i := r.Start; i < r.End && i < len(dst); i++ {
			dst[i] = r.Level
		}
	}
}

// paraLevels resolves the embedding levels of runes[a:b] (one paragraph,
// no '\n') into lv[a:b] and returns the paragraph level. mixed reports
// whether the paragraph needs bidi treatment at all (a rune at a non-zero
// level or a right-to-left paragraph).
func paraLevels(runes []rune, a, b int, dir Direction, lv []bidi.Level) (base bidi.Level, mixed bool) {
	def := bidi.Neutral
	switch dir {
	case DirLTR:
		def = bidi.LeftToRight
	case DirRTL:
		def = bidi.RightToLeft
		base = 1
	}
	first := true
	for a < b {
		// go-text stops at bidi paragraph separators other than '\n'
		// (U+2029, '\r', U+001C..1E, U+0085); resolve each piece and give
		// the separator the paragraph level.
		runs := bpara.Segment(runes[a:b], def)
		n := 0
		if k := runs.NumRuns(); k > 0 {
			n = runs.Run(k - 1).End
		}
		runLevels(runs, lv[a:a+n])
		lo := bidi.Level(127)
		for _, v := range lv[a : a+n] {
			lo = min(lo, v)
			mixed = mixed || v != 0
		}
		if first && dir == DirAuto && n > 0 {
			// Rules P2/P3 make the first strong character's level the
			// paragraph level, and no rune resolves below it.
			base = lo
		}
		first = false
		a += n
		if a < b { // a separator
			lv[a] = base
			a++
		}
	}
	return base, mixed || base != 0
}

// itemize cuts runes[a:b] into items of constant embedding level.
func itemize(a, b int, lv []bidi.Level, st Style, lang language.Language, out []item) []item {
	for i := a; i < b; {
		j := i + 1
		for j < b && lv[j] == lv[i] {
			j++
		}
		out = append(out, item{start: i, end: j, st: st, lang: lang, level: lv[i]})
		i = j
	}
	return out
}

// isBidiSpace reports runes reset to the paragraph level at a line end by
// rule L1 (whitespace and isolate / embedding format characters).
func isBidiSpace(r rune) bool {
	return unicode.Is(unicode.Zs, r) || r == '\t' || (r >= 0x2066 && r <= 0x2069) || (r >= 0x202a && r <= 0x202e) || r == 0x200b
}

// isSegSep reports bidi class S (segment separators).
func isSegSep(r rune) bool { return r == '\t' || r == 0x0b || r == 0x1f }

// lineLevels returns the levels of runes[a:e] for display, after rule L1:
// segment separators, whitespace before them and trailing whitespace go
// back to the paragraph level.
func lineLevels(runes []rune, a, e int, lv []bidi.Level, base bidi.Level) []bidi.Level {
	out := append([]bidi.Level(nil), lv[a:e]...)
	reset := true
	for i := e - 1; i >= a; i-- {
		r := runes[i]
		switch {
		case isSegSep(r):
			out[i-a] = base
			reset = true
		case reset && isBidiSpace(r):
			out[i-a] = base
		default:
			reset = false
		}
	}
	return out
}

// visualOrder applies rule L2: it returns the logical offsets (into lv) in
// left-to-right display order.
func visualOrder(lv []bidi.Level) []int {
	n := len(lv)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	hi, lo := bidi.Level(0), bidi.Level(127)
	for _, l := range lv {
		hi = max(hi, l)
		if l&1 == 1 {
			lo = min(lo, l)
		}
	}
	if lo == 127 {
		return idx
	}
	cur := append([]bidi.Level(nil), lv...)
	for lvl := hi; lvl >= lo; lvl-- {
		for i := 0; i < n; {
			if cur[i] < lvl {
				i++
				continue
			}
			j := i
			for j < n && cur[j] >= lvl {
				j++
			}
			for p, q := i, j-1; p < q; p, q = p+1, q-1 {
				idx[p], idx[q] = idx[q], idx[p]
				cur[p], cur[q] = cur[q], cur[p]
			}
			i = j
		}
	}
	return idx
}

// bidiLine is the per-rune geometry of a line that needs bidi handling
// (any right-to-left run, or a right-to-left paragraph). Index k is rune
// Line.Start+k; boxes are [xl,xr] in layout x, in visual position.
type bidiLine struct {
	xl, xr []float32
	rtl    []bool // resolved direction of each rune (odd level after L1)
}

// carets returns the caret x of every logical position Start..Start+len
// (see Layout.Caret): position 0 is the leading edge of the first rune,
// position k>0 the trailing edge of rune k-1.
func (b *bidiLine) carets() []float32 {
	n := len(b.xl)
	out := make([]float32, n+1)
	if n == 0 {
		return out
	}
	if b.rtl[0] {
		out[0] = b.xr[0]
	} else {
		out[0] = b.xl[0]
	}
	for k := 1; k <= n; k++ {
		if b.rtl[k-1] {
			out[k] = b.xl[k-1]
		} else {
			out[k] = b.xr[k-1]
		}
	}
	return out
}

func (b *bidiLine) shift(dx float32) {
	for k := range b.xl {
		b.xl[k] += dx
		b.xr[k] += dx
	}
}

// indexAt implements Layout.IndexAt on a bidi line: the logical position
// whose caret is nearest x. Several positions can share one x (the end of a
// run and the start of the next); the tie goes to the rune edge under x,
// see hitIndex.
func (b *bidiLine) indexAt(start int, x float32, carets []float32) int {
	best, bd := -1, float32(math.MaxFloat32)
	var ties []int
	for k, cx := range carets {
		d := float32(math.Abs(float64(cx - x)))
		switch {
		case d < bd-0.001:
			best, bd = k, d
			ties = append(ties[:0], k)
		case d <= bd+0.001:
			ties = append(ties, k)
		}
	}
	if len(ties) > 1 {
		if h := b.hitIndex(start, x) - start; slices.Contains(ties, h) {
			return start + h
		}
	}
	return start + max(best, 0)
}

// hitIndex maps x to the rune under it: its left half to its visual-left
// edge and its right half to its visual-right edge, as logical positions
// (for a left-to-right rune k: k and k+1; right-to-left: k+1 and k).
// Points left / right of the line go to the outer edge of the visually
// first / last rune.
func (b *bidiLine) hitIndex(start int, x float32) int {
	edge := func(k int, right bool) int {
		if right != b.rtl[k] {
			return start + k + 1
		}
		return start + k
	}
	lk, rk := -1, -1
	for k := range b.xl {
		l, r := b.xl[k], b.xr[k]
		if r <= l {
			continue
		}
		if x >= l && x < r {
			return edge(k, x > (l+r)/2)
		}
		if lk < 0 || l < b.xl[lk] {
			lk = k
		}
		if rk < 0 || r > b.xr[rk] {
			rk = k
		}
	}
	switch {
	case lk < 0:
		return start
	case x < b.xl[lk]:
		return edge(lk, false)
	default:
		return edge(rk, true)
	}
}

// SelectionRects returns the boxes covering the runes [start,end) (in any
// order), one or more per line: on a line mixing directions a logical range
// is visually discontiguous, so it yields one rect per visual stretch, left
// to right. Rects are in layout coordinates with the full line height.
// Lines contribute nothing when none of their visible runes is selected
// (an empty line in the middle of a selection included); callers that
// mark selected line breaks add that themselves.
func (l *Layout) SelectionRects(start, end int) []geom.Rect {
	if start > end {
		start, end = end, start
	}
	var out []geom.Rect
	for _, ln := range l.Lines {
		s0, s1 := max(start, ln.Start), min(end, ln.End)
		if s0 >= s1 {
			continue
		}
		if ln.bidi == nil {
			x0, x1 := ln.carets[min(s0-ln.Start, len(ln.carets)-1)], ln.carets[min(s1-ln.Start, len(ln.carets)-1)]
			if x1 > x0 {
				out = append(out, geom.Rect{X: x0, Y: ln.Top, W: x1 - x0, H: l.LineHeight})
			}
			continue
		}
		type iv struct{ a, b float32 }
		var ivs []iv
		for i := s0; i < s1; i++ {
			k := i - ln.Start
			if k >= len(ln.bidi.xl) {
				break
			}
			if a, b := ln.bidi.xl[k], ln.bidi.xr[k]; b > a {
				ivs = append(ivs, iv{a, b})
			}
		}
		sort.Slice(ivs, func(i, j int) bool { return ivs[i].a < ivs[j].a })
		for i := 0; i < len(ivs); {
			a, b := ivs[i].a, ivs[i].b
			j := i + 1
			for j < len(ivs) && ivs[j].a <= b+0.01 {
				b = max(b, ivs[j].b)
				j++
			}
			out = append(out, geom.Rect{X: a, Y: ln.Top, W: b - a, H: l.LineHeight})
			i = j
		}
	}
	return out
}
