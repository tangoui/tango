package text

import (
	"hash/maphash"
	"math"
	"sync"
	"unicode"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/segmenter"
	"github.com/go-text/typesetting/shaping"
	"github.com/tangoui/tango/ui/paint"
	"golang.org/x/image/math/fixed"
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

// fastPath memoizes the monospace fast path's font resolution and cluster
// shaping; it lives as long as the font registry version.
type fastPath struct {
	fonts  map[fastFontKey]*fastFont
	tables map[asciiKey]*unitTable
	nunits int // memoized units, all tables
	langs  map[langKey]language.Language
	safe   map[*Face]bool
	shaper shaping.HarfbuzzShaper
	split  shaping.Segmenter
	graph  segmenter.Segmenter
	lv     []bidi.Level
	sgs    []sg
}

type fastFontKey struct {
	w      Weight
	italic bool
}

type asciiKey struct {
	face   *Face
	size   float32
	script language.Script
	lang   language.Language
}

type langKey struct {
	tag    string
	script language.Script
}

// unitTable memoizes the shaping of units (grapheme clusters) with one
// face, size, script and language.
type unitTable struct {
	ascii [128]unitShape
	one   map[rune][]sg   // other one-rune units
	multi map[string][]sg // longer units
}

type unitShape struct {
	gl   []sg // cluster relative to the unit start
	done bool
}

// fastFont is a monospace font map of the fast path: font resolution
// memoized per rune, plus the per-line state that reproduces fontmap's
// last resort (the first face it tried in the layout).
type fastFont struct {
	fm      *fontmap
	primary *Face
	low     [128]resolved
	high    map[rune]resolved
	byFont  map[*font.Face]*Face
	fams    map[*Face]*family
	// asciiFace is the one face of every printable ASCII rune; nil
	// disables the ASCII lane. tabFace is the face of a run of spaces and
	// tabs ending with a tab (the face mapping tab, else the primary).
	asciiFace, tabFace *Face
	// Per line.
	firstSet bool
	first    *Face
}

type resolved struct {
	f    *Face
	fam  *family
	done bool
}

func (fp *fastPath) font(w Weight, italic bool) *fastFont {
	k := fastFontKey{w, italic}
	if ff := fp.fonts[k]; ff != nil {
		return ff
	}
	chain := reg.chain(true)
	fm := &fontmap{chain: chain, w: w, italic: italic, byFF: map[*font.Face]*Face{}, feats: noLigaFeatures}
	if len(chain) > 0 {
		fm.eng = reg.engineFallback()
	}
	ff := &fastFont{fm: fm, primary: fm.primary(), high: map[rune]resolved{}, byFont: map[*font.Face]*Face{}, fams: map[*Face]*family{}}
	if ff.primary != nil {
		ff.asciiFace = ff.primary
		for r := rune(' '); r <= '~'; r++ {
			if f := ff.lookup(r); f != ff.primary {
				ff.asciiFace = nil
				break
			}
		}
		ff.tabFace = ff.primary
		if f := ff.lookup('\t'); f != nil {
			ff.tabFace = f
		}
		if ff.asciiFace != nil && !fp.isSafe(ff.asciiFace, ff.fams[ff.asciiFace]) {
			ff.asciiFace = nil
		}
	}
	fp.fonts[k] = ff
	return ff
}

// lookup returns the face fontmap.ResolveFace picks for r when some face
// maps it, or nil.
func (ff *fastFont) lookup(r rune) *Face {
	var res resolved
	if r >= 0 && r < 128 {
		res = ff.low[r]
	} else {
		res = ff.high[r]
	}
	if !res.done {
		res.f, res.fam = ff.fm.resolve(r)
		res.done = true
		if r >= 0 && r < 128 {
			ff.low[r] = res
		} else {
			ff.high[r] = res
		}
		if res.f != nil {
			ff.byFont[res.f.face] = res.f
			ff.fams[res.f] = res.fam
		}
	}
	return res.f
}

// firstTried returns the face fontmap.ResolveFace tries first for r,
// which becomes its last resort for the rest of the layout.
func (ff *fastFont) firstTried(r rune) (*Face, *family) {
	m := ff.fm
	if cl := classOf(r); cl != classNone {
		for _, fam := range m.chain {
			if fam.user || fam.class == cl {
				if f, _ := fam.pick(m.w, m.italic); f != nil {
					return f, fam
				}
			}
		}
	}
	for _, fam := range m.chain {
		if f, _ := fam.pick(m.w, m.italic); f != nil {
			return f, fam
		}
	}
	return nil, nil // engine fallback families: let the general path decide
}

// ResolveFace implements shaping.Fontmap like fontmap.ResolveFace in a
// layout of its own (see fastFont).
func (ff *fastFont) ResolveFace(r rune) *font.Face {
	if !ff.firstSet {
		ff.firstSet = true
		f, fam := ff.firstTried(r)
		ff.first = f
		if f != nil {
			ff.byFont[f.face] = f
			ff.fams[f] = fam
		}
	}
	if f := ff.lookup(r); f != nil {
		return f.face
	}
	if ff.first != nil {
		return ff.first.face
	}
	return nil
}

var (
	tagKern = ot.NewTag('k', 'e', 'r', 'n')
	tagDist = ot.NewTag('d', 'i', 's', 't')
	tagCurs = ot.NewTag('c', 'u', 'r', 's')
	tagRlig = ot.NewTag('r', 'l', 'i', 'g')
	tagRclt = ot.NewTag('r', 'c', 'l', 't')
)

// isSafe reports whether shaping with f never reaches across grapheme
// clusters (with ligatures off): no kerning, distances, cursive
// attachment, required ligatures or contextual alternates, no AAT
// morx/kerx or legacy kern. Emoji fonts are trusted: their multi-rune
// sequences are single grapheme clusters.
func (fp *fastPath) isSafe(f *Face, fam *family) bool {
	if ok, done := fp.safe[f]; done {
		return ok
	}
	ok := true
	if fam == nil || fam.class != classEmoji {
		ft := f.face.Font
		ok = len(ft.Morx) == 0 && len(ft.Kern) == 0 && len(ft.Kerx) == 0
		for _, x := range ft.GPOS.Features {
			ok = ok && x.Tag != tagKern && x.Tag != tagDist && x.Tag != tagCurs
		}
		for _, x := range ft.GSUB.Features {
			ok = ok && x.Tag != tagRlig && x.Tag != tagRclt
		}
	}
	fp.safe[f] = ok
	return ok
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

// fastScript reports scripts shaped without context in HarfBuzz' default,
// Hangul and CJK shapers.
func fastScript(s language.Script) bool {
	switch s {
	case language.Common, language.Inherited, language.Latin, language.Greek, language.Cyrillic,
		language.Armenian, language.Georgian, language.Han, language.Hiragana, language.Katakana,
		language.Bopomofo, language.Hangul:
		return true
	}
	return false
}

// enforced returns the Language shaping.Segmenter gives a run of script
// in text of language tag (enforceLanguages), found by splitting a probe.
func (fp *fastPath) enforced(tag string, lang language.Language, script language.Script, fm *fontmap) language.Language {
	k := langKey{tag, script}
	if l, ok := fp.langs[k]; ok {
		return l
	}
	probe := []rune{'1'}
	if script == language.Latin {
		probe[0] = 'a'
	}
	out := fp.split.Split(shaping.Input{Text: probe, RunEnd: 1, Direction: di.DirectionLTR, Size: fixed.I(14), Language: lang}, fm)
	l := out[0].Language
	fp.langs[k] = l
	return l
}

// table returns the unit memo of k.
func (fp *fastPath) table(k asciiKey) *unitTable {
	t := fp.tables[k]
	if t == nil {
		t = &unitTable{}
		fp.tables[k] = t
	}
	return t
}

// unit shapes runes[a:b] (one grapheme cluster, or the part of one in a
// single face) alone with k, memoized in t.
func (fp *fastPath) unit(t *unitTable, runes []rune, a, b int, k asciiKey) []sg {
	if b == a+1 {
		r := runes[a]
		if r >= 0 && r < 128 {
			u := &t.ascii[r]
			if !u.done {
				u.gl, u.done = fp.shapeUnit(runes, a, b, k), true
			}
			return u.gl
		}
		if gl, ok := t.one[r]; ok {
			return gl
		}
		gl := fp.shapeUnit(runes, a, b, k)
		if t.one == nil {
			t.one = map[rune][]sg{}
		}
		t.one[r] = gl
		fp.counted()
		return gl
	}
	if gl, ok := t.multi[string(runes[a:b])]; ok {
		return gl
	}
	gl := fp.shapeUnit(runes, a, b, k)
	if t.multi == nil {
		t.multi = map[string][]sg{}
	}
	t.multi[string(runes[a:b])] = gl
	fp.counted()
	return gl
}

// counted bounds the memo: past 1<<15 units every table is dropped.
func (fp *fastPath) counted() {
	fp.nunits++
	if fp.nunits > 1<<15 {
		clear(fp.tables)
		fp.nunits = 0
	}
}

func (fp *fastPath) shapeUnit(runes []rune, a, b int, k asciiKey) []sg {
	in := shaping.Input{Text: runes, RunStart: a, RunEnd: b, Direction: di.DirectionLTR, Face: k.face.face,
		Size: fixed.Int26_6(k.size * 64), Script: k.script, Language: k.lang, FontFeatures: noLigaFeatures}
	if shapeHook != nil {
		shapeHook(in)
	}
	o := fp.shaper.Shape(in)
	gl := make([]sg, len(o.Glyphs))
	for i, g := range o.Glyphs {
		gl[i] = sg{face: k.face, gid: uint32(g.GlyphID), adv: fx(g.XAdvance), xoff: fx(g.XOffset), yoff: fx(g.YOffset), cluster: g.ClusterIndex - a}
	}
	return gl
}

// fastShape is the monospace fast path (see LineCache); it returns nil
// when the line needs the general path. The caller holds mu.
func (c *LineCache) fastShape(line string, runs []Run, opt Options) *Layout {
	if !opt.noLiga || opt.Direction == DirRTL || len(runs) == 0 || line == "" {
		return nil
	}
	p := prepRuns(line, runs)
	for _, r := range p.rs {
		if !r.st.Mono {
			return nil
		}
	}
	runes := []rune(line)
	n := len(runes)
	ascii := true
	for _, r := range runes {
		if r == '\n' {
			return nil
		}
		ascii = ascii && (r >= ' ' && r <= '~' || r == '\t')
	}
	if c.fp == nil {
		c.fp = &fastPath{fonts: map[fastFontKey]*fastFont{}, tables: map[asciiKey]*unitTable{},
			langs: map[langKey]language.Language{}, safe: map[*Face]bool{}}
	}
	fp := c.fp
	ffs := make([]*fastFont, len(p.rs))
	for k, r := range p.rs {
		ffs[k] = fp.font(r.st.Weight, r.st.Italic)
		ffs[k].firstSet = false
		if ffs[k].primary == nil {
			return nil // no fonts: approximate metrics
		}
		ascii = ascii && ffs[k].asciiFace != nil
	}
	if !ascii {
		if cap(fp.lv) < n {
			fp.lv = make([]bidi.Level, n)
		}
		if !maybeRTL(runes) {
			// No right-to-left or explicit bidi character: every level is 0.
		} else if _, mixed := paraLevels(runes, 0, n, opt.Direction, fp.lv[:n]); mixed {
			return nil
		}
	}
	// Items: maximal ranges shaped with one run's style.
	items := []item{{start: 0, end: n, st: p.rs[0].st, lang: language.NewLanguage(p.rs[0].lang)}}
	if p.shapeOf != nil {
		items = splitRuns(items, p.shapeOf, p.rs)
	}
	gl := fp.sgs[:0]
	adv := make([]float32, n+1)
	add := func(us []sg, a int, run int32) {
		for _, g := range us {
			g.cluster += a
			g.run = run
			adv[g.cluster] += g.adv
			gl = append(gl, g)
		}
	}
	for _, it := range items {
		ff := ffs[it.run]
		size := it.st.Size
		if ascii {
			script := language.Common
			for i := it.start; i < it.end; i++ {
				if r := runes[i] | 0x20; r >= 'a' && r <= 'z' {
					script = language.Latin
					break
				}
			}
			// The face of the item (see shaping.Segmenter): spaces and tabs
			// take the face of the first other rune, or, when there is
			// none, that of the last one.
			face := ff.asciiFace
			if script == language.Common && runes[it.end-1] == '\t' {
				blank := true
				for i := it.start; i < it.end && blank; i++ {
					blank = runes[i] == ' ' || runes[i] == '\t'
				}
				if blank {
					face = ff.tabFace
					if !fp.isSafe(face, ff.fams[face]) {
						return nil
					}
				}
			}
			k := asciiKey{face, size, script, fp.enforced(p.rs[it.run].lang, it.lang, script, ff.fm)}
			t := fp.table(k)
			for i := it.start; i < it.end; i++ {
				add(fp.unit(t, runes, i, i+1, k), i, it.run)
			}
			continue
		}
		in := shaping.Input{Text: runes, RunStart: it.start, RunEnd: it.end, Direction: di.DirectionLTR,
			Size: fixed.Int26_6(size * 64), Language: it.lang, FontFeatures: noLigaFeatures}
		for _, run := range fp.split.Split(in, ff) {
			if run.Face == nil {
				return nil
			}
			face := ff.byFont[run.Face]
			if face == nil || !fastScript(run.Script) || !fp.isSafe(face, ff.fams[face]) {
				return nil
			}
			k := asciiKey{face, size, run.Script, run.Language}
			// Units: the grapheme clusters of the run's own text (what
			// HarfBuzz sees), so cuts at style, script or face changes
			// split clusters exactly as in the general path.
			seg := runes[run.RunStart:run.RunEnd]
			t := fp.table(k)
			if simpleText(seg) {
				// Every rune is a cluster of its own.
				for i := run.RunStart; i < run.RunEnd; i++ {
					add(fp.unit(t, runes, i, i+1, k), i, it.run)
				}
				continue
			}
			fp.graph.Init(seg)
			a := 0
			for gi := fp.graph.GraphemeIterator(); gi.Next(); {
				gr := gi.Grapheme()
				b := gr.Offset + len(gr.Text)
				// A cluster starting with a combining character (after a
				// control, UAX #29 GB4: tab, ZWSP, ...) is still merged
				// into the previous one by HarfBuzz, and so is a
				// pictograph after a ZWJ (letters, digits, ASCII never are).
				if r := seg[a]; a > 0 && (isContinuation(r) ||
					seg[a-1] == 0x200d && r >= 0x80 && !unicode.IsLetter(r) && !unicode.IsDigit(r)) {
					return nil
				}
				// Emoji fonts pair regional indicators (flags) across
				// anything ignorable; keep to clusters of indicators only.
				if hasRI(gr.Text) && (len(gr.Text) > 2 || !allRI(gr.Text)) {
					return nil
				}
				add(fp.unit(t, runes, run.RunStart+a, run.RunStart+b, k), run.RunStart+a, it.run)
				a = b
			}
		}
	}
	fp.sgs = gl[:0]
	expandTabs(runes, 0, n, adv, gl, opt.tabStop)

	fms := make([]*fontmap, len(ffs))
	for k, ff := range ffs {
		fms[k] = ff.fm
	}
	lh, asc, desc := lineMetrics(p.rs, fms, p.runOf, n)
	l := &Layout{Text: line, Runes: runes, Style: p.rs[0].st, LineHeight: lh, Ascent: asc}
	ln := Line{Start: 0, End: n, Baseline: (lh-(asc+desc))/2 + asc}
	ln.carets = make([]float32, 0, n+1)
	pen := float32(0)
	g := 0
	for i := range n {
		ln.carets = append(ln.carets, pen)
		for ; g < len(gl) && gl[g].cluster == i; g++ {
			sg := gl[g]
			if runes[i] != '\t' {
				run := int32(0)
				if p.runOf != nil {
					run = p.runOf[i]
				}
				var skew float32
				if p.rs[sg.run].st.Italic && !sg.face.Italic {
					skew = ItalicSkew
				}
				l.addGlyph(paint.Glyph{Src: sg.face, ID: sg.gid, Size: p.rs[sg.run].st.Size, X: pen + sg.xoff, Y: ln.Baseline - sg.yoff, Skew: skew}, int32(i), run, len(gl))
			}
			pen += sg.adv
		}
	}
	if g != len(gl) {
		return nil // clusters out of logical order: not a simple line
	}
	ln.carets = append(ln.carets, pen)
	ln.Width = spanWidth(adv, 0, n, runes)
	l.Width = ln.Width
	l.Lines = []Line{ln}
	l.Height = lh
	return l
}
