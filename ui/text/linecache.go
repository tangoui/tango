package text

import (
	"hash/maphash"
	"math"
	"sync"
	"unicode"
)

// LineOptions are the per-line settings of LineCache.Shape.
type LineOptions struct {
	// TabSize is the tab stop interval in columns (<= 0 means 4). A column
	// is the advance of a space in the monospace face at the size of the
	// first run; a tab advances the pen to the next multiple of TabSize
	// columns, measured in pixels from the line start (so after wide
	// characters the stop is where their actual glyph widths put it).
	TabSize int
	// Ligatures keeps the OpenType ligature features (liga, clig, calt)
	// HarfBuzz applies by default; false turns them off. Code editors
	// usually leave it false; true always takes the general path.
	Ligatures bool
	// Lang is the language of runs whose Style.Lang is empty (empty:
	// DefaultLang).
	Lang string
	// Direction is the paragraph direction (Options.Direction).
	Direction Direction
}

// LineCache shapes single lines of a code editor (or any list of
// independent single-line paragraphs) and keeps the most recently used
// layouts. Shape(line, runs, opt) returns what
//
//	ShapeRuns(line, runs', Options{NoWrap: true, Direction: opt.Direction})
//
// returns, where runs' are runs with opt.Lang filled into empty Style.Lang,
// plus LineOptions' tab stops and ligature setting: same glyphs, carets,
// GlyphRun and metrics. The line must not hold '\n' (it would be laid out
// as several lines, through the general path).
//
// Entries are keyed by the line text, the runs (ranges and styles), and
// every LineOptions field, and evicted least recently used first; a change
// of the font registry or of DefaultLang empties the cache. The layouts
// bypass the shared Shape/ShapeRuns cache, so scrolling a long file does
// not evict other text's layouts.
//
// Monospace fast path: when every run is Mono, ligatures are off and the
// line is left-to-right, the line is not run through HarfBuzz as a whole.
// It is cut the way the general path cuts it (style runs, script, font
// face) and further into grapheme clusters, and each cluster's shaping is
// memoized per face, size, script and language, then the clusters are
// placed side by side (tabs expanded, combining marks and zero-width
// characters taking the advance HarfBuzz gives them, wide CJK and emoji
// glyphs their own advance). This equals shaping the whole run only when
// nothing reaches across clusters, so the fast path is used only with
// faces that have no kerning, cursive attachment, required ligatures or
// AAT tables (emoji fonts excepted: their sequences are grapheme
// clusters), for scripts that need no contextual shaping (Latin, Greek,
// Cyrillic, Armenian, Georgian, CJK and symbols), and when every rune
// resolves to a face; anything else, including any right-to-left or bidi
// control character, goes through the general path. Lines of printable
// ASCII skip the segmentation step entirely.
//
// A LineCache is safe for concurrent use; the returned Layout is shared
// and must not be mutated.
type LineCache struct {
	mu         sync.Mutex
	capacity   int
	seed       maphash.Seed
	m          map[uint64]*lineEntry
	head, tail *lineEntry // most recently used first
	ver        int        // reg.version the entries were made with
	dlang      string     // DefaultLang the entries were made with
	fp         *fastPath  // fast path memo; nil until needed
	stats      lineStats
}

type lineStats struct {
	hits, misses int
	fast, slow   int // misses shaped by the fast and the general path
}

type lineEntry struct {
	key        uint64
	line       string
	runs       []Run
	opt        LineOptions
	l          *Layout
	prev, next *lineEntry
}

// NewLineCache returns a cache holding up to capacity lines (<= 0: 2048).
func NewLineCache(capacity int) *LineCache {
	if capacity <= 0 {
		capacity = 2048
	}
	return &LineCache{capacity: capacity, seed: maphash.MakeSeed(), m: map[uint64]*lineEntry{}, ver: -1}
}

// Len returns the number of cached lines.
func (c *LineCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}

// Reset empties the cache.
func (c *LineCache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flush()
}

func (c *LineCache) flush() {
	clear(c.m)
	c.head, c.tail = nil, nil
	c.fp = nil
}

// Shape lays out line with the style runs (see ShapeRuns) as one
// unwrapped line, from the cache when it holds the same line, runs and
// options.
func (c *LineCache) Shape(line string, runs []Run, opt LineOptions) *Layout {
	reg.mu.Lock()
	v := reg.version
	reg.mu.Unlock()
	dl := DefaultLang()
	h := c.hash(line, runs, opt)
	c.mu.Lock()
	defer c.mu.Unlock()
	if v != c.ver || dl != c.dlang {
		c.flush()
		c.ver, c.dlang = v, dl
	}
	e := c.m[h]
	if e != nil && e.line == line && e.opt == opt && sameRuns(e.runs, runs) {
		c.stats.hits++
		c.unlink(e)
		c.pushFront(e)
		return e.l
	}
	c.stats.misses++
	rs, sopt := lineShapeArgs(runs, opt)
	mu.Lock()
	l := c.fastShape(line, rs, sopt)
	if l != nil {
		c.stats.fast++
	} else {
		c.stats.slow++
		l = shapeRunsUncached(line, rs, sopt)
	}
	mu.Unlock()
	if e != nil { // hash collision: replace
		c.unlink(e)
		delete(c.m, h)
	}
	e = &lineEntry{key: h, line: line, runs: append([]Run(nil), runs...), opt: opt, l: l}
	c.m[h] = e
	c.pushFront(e)
	for len(c.m) > c.capacity {
		t := c.tail
		c.unlink(t)
		delete(c.m, t.key)
	}
	return l
}

func sameRuns(a, b []Run) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (c *LineCache) unlink(e *lineEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
	e.prev, e.next = nil, nil
}

func (c *LineCache) pushFront(e *lineEntry) {
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *LineCache) hash(line string, runs []Run, opt LineOptions) uint64 {
	h := maphash.String(c.seed, line)
	mix := func(x uint64) {
		h ^= x
		h *= 0x9e3779b97f4a7c15
		h ^= h >> 29
	}
	b := func(v bool) uint64 {
		if v {
			return 1
		}
		return 0
	}
	mix(uint64(len(runs)))
	for _, r := range runs {
		st := r.Style
		mix(uint64(uint32(r.Start))<<32 | uint64(uint32(r.End)))
		mix(uint64(math.Float32bits(st.Size))<<32 | uint64(math.Float32bits(st.LineHeight)))
		mix(uint64(uint32(st.Weight))<<2 | b(st.Mono)<<1 | b(st.Italic))
		if st.Lang != "" {
			mix(maphash.String(c.seed, st.Lang))
		}
		if st.FontFamily != "" {
			mix(maphash.String(c.seed, st.FontFamily))
		}
	}
	mix(uint64(uint32(opt.TabSize))<<16 | uint64(opt.Direction)<<1 | b(opt.Ligatures))
	if opt.Lang != "" {
		mix(maphash.String(c.seed, opt.Lang))
	}
	return h
}

// lineShapeArgs returns the ShapeRuns arguments LineCache.Shape lays a
// line out with: runs with opt.Lang filled in, and Options for one
// unwrapped line with opt's direction, tab stops and ligature setting.
func lineShapeArgs(runs []Run, opt LineOptions) ([]Run, Options) {
	if opt.Lang != "" {
		for i, r := range runs {
			if r.Style.Lang == "" {
				cp := append([]Run(nil), runs...)
				for j := i; j < len(cp); j++ {
					if cp[j].Style.Lang == "" {
						cp[j].Style.Lang = opt.Lang
					}
				}
				runs = cp
				break
			}
		}
	}
	ts := opt.TabSize
	if ts <= 0 {
		ts = 4
	}
	st := Style{Mono: true}
	if len(runs) > 0 {
		st.Size = runs[0].Style.Size
	}
	col, _, _ := Shape(" ", st, Options{NoWrap: true}).Caret(1)
	return runs, Options{NoWrap: true, Direction: opt.Direction, tabStop: float32(ts) * col, noLiga: !opt.Ligatures}
}

// isContinuation reports runes HarfBuzz merges into the previous cluster:
// marks, ZWJ/ZWNJ, variation selectors, emoji modifiers and tags.
func isContinuation(r rune) bool {
	return unicode.Is(unicode.M, r) || r == 0x200c || r == 0x200d || r >= 0xfe00 && r <= 0xfe0f ||
		r >= 0x1f3fb && r <= 0x1f3ff || r >= 0xe0020 && r <= 0xe007f || r >= 0xe0100 && r <= 0xe01ef
}

// simpleText reports text where every rune is a grapheme cluster of its
// own (UAX #29): no combining or joining characters, regional indicators,
// Hangul jamo or CR. (Prepend and Indic conjunct rules concern scripts the
// fast path leaves to the general path.)
func simpleText(rs []rune) bool {
	for _, r := range rs {
		if r < 0x300 {
			if r == '\r' {
				return false
			}
			continue
		}
		if isContinuation(r) || isRI(r) || r >= 0x1100 && r <= 0x11ff || r >= 0xa960 && r <= 0xa97f ||
			r >= 0xd7b0 && r <= 0xd7ff || r == 0x200d {
			return false
		}
	}
	return true
}

// maybeRTL reports whether rs may hold a character of bidi class R, AL or
// AN, or an explicit bidi control: false guarantees all-zero levels in a
// left-to-right or auto paragraph.
func maybeRTL(rs []rune) bool {
	for _, r := range rs {
		switch {
		case r < 0x590:
		case r <= 0x8ff, r == 0x200e, r == 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069,
			r >= 0xfb1d && r <= 0xfdff, r >= 0xfe70 && r <= 0xfefe, r >= 0x10800 && r <= 0x10fff,
			r >= 0x1e800 && r <= 0x1efff:
			return true
		}
	}
	return false
}

func isRI(r rune) bool { return r >= 0x1f1e6 && r <= 0x1f1ff }

func hasRI(rs []rune) bool {
	for _, r := range rs {
		if isRI(r) {
			return true
		}
	}
	return false
}

func allRI(rs []rune) bool {
	for _, r := range rs {
		if !isRI(r) {
			return false
		}
	}
	return true
}
