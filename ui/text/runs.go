package text

import (
	"encoding/binary"
	"math"
	"unicode/utf8"

	"github.com/go-text/typesetting/language"
	"github.com/tangoui/tango/ui/geom"
)

// Run is a style run of ShapeRuns: runes [Start,End) of the text use Style.
type Run struct {
	Start, End int // rune range
	Style      Style
}

// ShapeRuns lays out s like Shape, with a Style per rune range: each
// paragraph is cut at bidi level changes and where the shaping style
// changes, and every piece is shaped on its own with its run's font,
// weight, slant, size and language (the paragraph direction stays
// opt.Direction). Adjacent runs whose styles shape alike (equal Style
// fields once normalized, with the same resolved language) are shaped as
// one piece, so kerning and ligatures cross token boundaries that only
// differ in what Style does not hold (color, decoration); GlyphRun still
// names each glyph's own run. Shaping keeps the text around a piece as
// context, so Arabic letters still join across a style boundary; kerning
// and ligatures do not cross one.
//
// Coverage: runs are clamped to the text; a rune covered by several runs
// takes the last of them, a rune covered by none takes run 0. With no runs
// ShapeRuns is Shape(s, Style{}, opt); with one run covering the text the
// result is identical to Shape(s, runs[0].Style, opt).
//
// Line metrics: every line has the largest LineHeight among the runs that
// cover some rune (Layout.LineHeight, uniform so IndexAt still works by
// line), and one baseline shared by all runs, placed so that the union of
// the runs' font ascents and descents is centred in the line (for one run:
// Shape's rule). Layout.Ascent is the largest ascent, Layout.Style is
// runs[0].Style (normalized). Each glyph keeps its run's Size and Skew, and
// Layout.GlyphRun maps it to its run. Results are cached; the returned
// Layout must not be mutated.
func ShapeRuns(s string, runs []Run, opt Options) *Layout {
	if len(runs) == 0 {
		return Shape(s, Style{}, opt)
	}
	opt = normOptions(opt)
	p := prepRuns(s, runs)
	reg.mu.Lock()
	v := reg.version
	reg.mu.Unlock()
	k := runsKey{s: s, opt: opt, runs: encodeRuns(runs, p.rs, p.n)}
	mu.Lock()
	defer mu.Unlock()
	if v != cacheV {
		cache = map[cacheKey]*Layout{}
		runsCache = map[runsKey]*Layout{}
		cacheV = v
	}
	if l, ok := runsCache[k]; ok {
		hits++
		return l
	}
	l := layoutRuns(s, p.rs, p.runOf, p.shapeOf, opt)
	if len(runsCache) > 8192 {
		runsCache = map[runsKey]*Layout{}
	}
	runsCache[k] = l
	return l
}

// shapeRunsUncached is ShapeRuns without the shared cache (LineCache keeps
// its own); the caller must hold mu.
func shapeRunsUncached(s string, runs []Run, opt Options) *Layout {
	if len(runs) == 0 {
		runs = []Run{{0, len(s), Style{}}} // one run covering s is Shape(s, Style{}, opt)
	}
	opt = normOptions(opt)
	p := prepRuns(s, runs)
	return layoutRuns(s, p.rs, p.runOf, p.shapeOf, opt)
}

func normOptions(opt Options) Options {
	if geom.IsInf(opt.MaxWidth) || opt.MaxWidth < 0 {
		opt.MaxWidth = 0
	}
	return opt
}

// runPrep is ShapeRuns' view of its runs: the normalized styles, the run
// of every rune (runOf, nil for a single run) and the run every rune is
// shaped with (shapeOf: the first run shaping like it; nil when all runs
// shape alike).
type runPrep struct {
	rs             []runStyle
	runOf, shapeOf []int32
	n              int // runes in s
}

func prepRuns(s string, runs []Run) runPrep {
	n := utf8.RuneCountInString(s)
	rs := make([]runStyle, len(runs))
	var chains [2][]*family
	for k, r := range runs {
		st := r.Style.norm()
		m := 0
		if st.Mono {
			m = 1
		}
		if chains[m] == nil {
			chains[m] = reg.chain(st.Mono)
		}
		rs[k] = runStyle{st: st, chain: chains[m], lang: resolveLang(st)}
	}
	p := runPrep{rs: rs, n: n}
	if len(runs) == 1 {
		return p
	}
	p.runOf = make([]int32, n)
	for k, r := range runs {
		a, b := max(r.Start, 0), min(r.End, n)
		for i := a; i < b; i++ {
			p.runOf[i] = int32(k)
		}
	}
	// group[k]: the first run shaping like run k (a line has few
	// distinct styles, so a scan of the firsts is cheap).
	group := make([]int32, len(rs))
	var firsts []int32
	for k := range rs {
		group[k] = int32(k)
		for _, f := range firsts {
			if rs[f].lang == rs[k].lang && sameShaping(rs[f].st, rs[k].st) {
				group[k] = f
				break
			}
		}
		if group[k] == int32(k) {
			firsts = append(firsts, int32(k))
		}
	}
	alike := true
	p.shapeOf = make([]int32, n)
	for i, r := range p.runOf {
		g := group[r]
		p.shapeOf[i] = g
		alike = alike && g == 0
	}
	if alike {
		p.shapeOf = nil
	}
	return p
}

// sameShaping compares normalized styles ignoring Lang (compared resolved).
func sameShaping(a, b Style) bool {
	a.Lang, b.Lang = "", ""
	return a == b
}

type runsKey struct {
	s    string
	opt  Options
	runs string // encodeRuns
}

// runsCache holds ShapeRuns layouts; guarded by mu and flushed with cache.
var runsCache = map[runsKey]*Layout{}

// encodeRuns serializes the clamped ranges and normalized styles (with
// the resolved language) of runs as a cache key.
func encodeRuns(runs []Run, rs []runStyle, n int) string {
	b := make([]byte, 0, len(runs)*24)
	for k, r := range runs {
		st := rs[k].st
		var flags byte
		if st.Mono {
			flags |= 1
		}
		if st.Italic {
			flags |= 2
		}
		b = binary.AppendUvarint(b, uint64(min(max(r.Start, 0), n)))
		b = binary.AppendUvarint(b, uint64(max(min(r.End, n), 0)))
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(st.Size))
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(st.LineHeight))
		b = binary.AppendVarint(b, int64(st.Weight))
		b = append(b, flags)
		b = binary.AppendUvarint(b, uint64(len(rs[k].lang)))
		b = append(b, rs[k].lang...)
	}
	return string(b)
}

// splitRuns cuts items where the shaping run (shapeOf) changes and gives
// each piece that run's style, language and index.
func splitRuns(items []item, shapeOf []int32, rs []runStyle) []item {
	out := make([]item, 0, len(items))
	langs := make(map[int32]language.Language, len(rs))
	for _, it := range items {
		if it.start >= it.end {
			out = append(out, it)
			continue
		}
		for a := it.start; a < it.end; {
			r := shapeOf[a]
			b := a + 1
			for b < it.end && shapeOf[b] == r {
				b++
			}
			lg, ok := langs[r]
			if !ok {
				lg = language.NewLanguage(rs[r].lang)
				langs[r] = lg
			}
			out = append(out, item{start: a, end: b, st: rs[r].st, lang: lg, level: it.level, run: r})
			a = b
		}
	}
	return out
}
