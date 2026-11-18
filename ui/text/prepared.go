package text

import (
	"unsafe"
)

// Width changes wrapping and placement, not paragraph shaping. Keep a small,
// byte-bounded set of shaped paragraphs separately from width-keyed Layouts.
// Access is protected by mu, as are the shared shaper and font maps.
const preparedBudget = 4 << 20

type paragraphInfo struct {
	a, b      int
	base      level
	mixed     bool
	ends      []int
	endsReady bool
}
type preparedText struct {
	memo                         bool
	wrapped, unwrapped           *Layout
	wrappedBytes, unwrappedBytes int
	runes                        []rune
	adv                          []float32
	glyphs                       []sg
	lv                           []level
	paras                        []paragraphInfo
	byCluster                    glyphClusters
	fms                          []*fontmap
	lh, asc, desc                float32
}

var preparedCache = map[cacheKey]*preparedText{}
var preparedBytes int

func prepareText(s string, rs []runStyle, runOf, shapeOf []int32, opt Options) *preparedText {
	var key cacheKey
	// Multi-style runs have additional identity. Leave those on the general
	// path; single-style paragraphs cover labels and ordinary wrapped text.
	// With CoreText, where shaping a paragraph costs a CTLine, short labels
	// are kept too: a label is measured and painted at slightly different
	// widths, and each would otherwise be shaped again.
	minLen := 32
	if coreTextOnly {
		minLen = 1
	}
	memo := len(rs) == 1 && len(s) >= minLen
	if memo {
		shapeOpt := opt
		shapeOpt.MaxWidth, shapeOpt.MaxLines, shapeOpt.NoWrap, shapeOpt.Ellipsis = 0, 0, false, false
		key = cacheKey{s, rs[0].st, shapeOpt, rs[0].lang}
		if p := preparedCache[key]; p != nil {
			return p
		}
	}
	runes := []rune(s)
	st := rs[0].st
	lang := rs[0].lang
	var fmBuf [1]*fontmap
	fms := newFontmaps(rs, opt, &fmBuf)
	lh, asc, desc := lineMetrics(rs, fms, runOf, len(runes))
	adv := make([]float32, len(runes)+1)
	var paras []paragraphInfo
	var glyphs []sg
	var items []item
	lv := make([]level, len(runes))
	pa := 0
	for pa <= len(runes) {
		pb := pa
		for pb < len(runes) && runes[pb] != '\n' {
			pb++
		}
		pi := paragraphInfo{a: pa, b: pb}
		if opt.Direction == DirRTL {
			pi.base, pi.mixed = 1, true
		}
		if pb > pa {
			pi.base, pi.mixed = paraLevels(runes, pa, pb, opt.Direction, lv)
		}
		items = items[:0]
		if pi.mixed {
			items = itemize(pa, pb, lv, st, lang, items)
		} else {
			items = append(items, item{start: pa, end: pb, st: st, lang: lang})
		}
		if shapeOf != nil {
			items = splitRuns(items, shapeOf, rs)
		}
		gl := shapeItems(runes, items, fms, adv)
		if opt.tabStop > 0 {
			expandTabs(runes, pa, pb, adv, gl, opt.tabStop)
		}
		glyphs = append(glyphs, gl...)
		paras = append(paras, pi)
		pa = pb + 1
	}
	byCluster := indexGlyphClusters(len(runes), glyphs)
	p := &preparedText{runes: runes, adv: adv, glyphs: glyphs, lv: lv, paras: paras, byCluster: byCluster, fms: fms, lh: lh, asc: asc, desc: desc}
	if memo {
		// Account capacities, not lengths: long runs of empty paragraphs
		// allocate paragraph records even though they contain no glyphs.
		bytes := len(s) + int(unsafe.Sizeof(*p)) + cap(runes)*4 + cap(adv)*4 + cap(lv)*int(unsafe.Sizeof(level(0))) + cap(glyphs)*int(unsafe.Sizeof(sg{})) + cap(paras)*int(unsafe.Sizeof(paragraphInfo{})) + (cap(byCluster.first)+cap(byCluster.next))*4 + cap(fms)*int(unsafe.Sizeof((*fontmap)(nil)))
		// Font maps are tiny compared with glyphs, but include a conservative
		// allowance for their map buckets and resolved-face references.
		bytes += len(fms) * 512
		if bytes <= preparedBudget {
			if preparedBytes+bytes > preparedBudget || len(preparedCache) >= 256 {
				preparedCache = map[cacheKey]*preparedText{}
				preparedBytes = 0
			}
			p.memo = true
			preparedCache[key] = p
			preparedBytes += bytes
		}
	}
	return p
}

func clearPrepared() {
	preparedCache = map[cacheKey]*preparedText{}
	preparedBytes = 0
	resetShaper()
}

// Equal line breaks yield identical placement even at different widths.
// Remember the latest wrapped and unwrapped layouts separately: flex layout
// measures both, and an unconstrained probe must not displace the wrapped result.
func (p *preparedText) placement(spans []span, truncated, wrap bool) *Layout {
	l := p.unwrapped
	if wrap {
		l = p.wrapped
	}
	if l == nil || l.Truncated != truncated || len(l.Lines) != len(spans) {
		return nil
	}
	for i, sp := range spans {
		if l.Lines[i].Start != sp.a || l.Lines[i].End != sp.b {
			return nil
		}
	}
	return l
}

func (p *preparedText) rememberPlacement(l *Layout, wrap bool) {
	if !p.memo {
		return
	}
	old := p.unwrappedBytes
	if wrap {
		old = p.wrappedBytes
	}
	n := layoutCost(l)
	if preparedBytes+n-old > preparedBudget {
		return
	}
	preparedBytes += n - old
	if wrap {
		p.wrapped, p.wrappedBytes = l, n
	} else {
		p.unwrapped, p.unwrappedBytes = l, n
	}
}

// Unicode break opportunities depend on the paragraph's text, not its width.
// Compute once; continue to use the original float32 span accumulation so
// boundary decisions remain bit-for-bit identical to the uncached algorithm.
func (p *preparedText) lineEnds(i int) []int {
	para := &p.paras[i]
	if para.endsReady {
		return para.ends
	}
	ends := lineEnds(p.runes, para.a, para.b)
	cost := cap(ends) * int(unsafe.Sizeof(int(0)))
	if !p.memo || preparedBytes+cost <= preparedBudget {
		para.ends, para.endsReady = ends, true
		if p.memo {
			preparedBytes += cost
		}
	}
	return ends
}
