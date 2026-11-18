//go:build !darwin || ios || !tango_coretext

package text

import (
	"reflect"

	"github.com/go-text/typesetting/bidi"
)

// UAX #9 embedding levels from go-text's bidi package (the implementation
// shaping.Segmenter itself uses).

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
func runLevels(runs bidi.Runs, dst []level) {
	if f := reflect.ValueOf(runs).FieldByName("levels"); f.IsValid() && f.Kind() == reflect.Slice && f.Len() >= len(dst) {
		for i := range dst {
			dst[i] = level(f.Index(i).Int())
		}
		return
	}
	for k := 0; k < runs.NumRuns(); k++ {
		r := runs.Run(k)
		for i := r.Start; i < r.End && i < len(dst); i++ {
			dst[i] = level(r.Level)
		}
	}
}

// paraLevels resolves the embedding levels of runes[a:b] (one paragraph,
// no '\n') into lv[a:b] and returns the paragraph level. mixed reports
// whether the paragraph needs bidi treatment at all (a rune at a non-zero
// level or a right-to-left paragraph).
func paraLevels(runes []rune, a, b int, dir Direction, lv []level) (base level, mixed bool) {
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
		lo := level(127)
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
