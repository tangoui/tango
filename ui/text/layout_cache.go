package text

import "unsafe"

// Full positioned layouts are expensive: a few thousand long paragraphs at
// different widths used to retain hundreds of MiB. Bound them by bytes as
// well as entries, independently of width-independent paragraph shaping.
const layoutBudget = 8 << 20
const layoutEntries = 1024

type layoutSlot struct {
	key   cacheKey
	bytes int
}

var layoutRing [layoutEntries]layoutSlot
var layoutHead, layoutCount, layoutBytes int

func layoutCost(l *Layout) int {
	n := int(unsafe.Sizeof(*l)) + len(l.Text) + cap(l.Runes)*4 + cap(l.Glyphs)*int(unsafe.Sizeof(l.Glyphs[0])) + cap(l.GlyphCluster)*4 + cap(l.GlyphRun)*4 + cap(l.Lines)*int(unsafe.Sizeof(Line{}))
	for _, ln := range l.Lines {
		n += cap(ln.carets) * 4
		if ln.bidi != nil {
			n += cap(ln.bidi.xl)*4 + cap(ln.bidi.xr)*4 + cap(ln.bidi.rtl)
		}
	}
	return n
}

func putLayout(k cacheKey, l *Layout) {
	bytes := layoutCost(l)
	if bytes > layoutBudget {
		return
	}
	for layoutCount > 0 && (layoutCount == layoutEntries || layoutBytes+bytes > layoutBudget) {
		slot := layoutRing[layoutHead]
		delete(cache, slot.key)
		layoutBytes -= slot.bytes
		layoutRing[layoutHead] = layoutSlot{}
		layoutHead = (layoutHead + 1) % layoutEntries
		layoutCount--
	}
	i := (layoutHead + layoutCount) % layoutEntries
	layoutRing[i] = layoutSlot{k, bytes}
	layoutCount++
	layoutBytes += bytes
	cache[k] = l
}

func resetLayouts() {
	cache = map[cacheKey]*Layout{}
	clear(layoutRing[:])
	layoutHead, layoutCount, layoutBytes = 0, 0, 0
}
