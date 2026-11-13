package text

import (
	"math"
	"strings"
	"sync"
	"unicode"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/segmenter"
	"github.com/go-text/typesetting/shaping"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"golang.org/x/image/math/fixed"
)

// Style selects font and size.
type Style struct {
	Size       float32 // px; 0 means 14
	Weight     Weight  // 0 means Regular
	LineHeight float32 // px; 0 means round(Size*22/14)
	Mono       bool    // prefer the monospace chain
	// Italic selects each family's italic (or oblique) face; families
	// without one draw their upright face slanted by ItalicSkew (advances
	// unchanged). Widths always come from the face actually used.
	Italic bool
	// Lang is the BCP 47 language tag of the text ("ja", "zh-Hant", "ar");
	// empty means the application language (DefaultLang). It selects
	// language-specific shaping (OpenType locl): the same ideograph takes
	// its Japanese or Chinese form. Runs of another script get that
	// script's language (e.g. Arabic runs inside "en" text).
	Lang string
}

// ItalicSkew is the slant of synthetic italics (tan of ~11°).
const ItalicSkew = 0.2

func (s Style) norm() Style {
	if s.Size <= 0 {
		s.Size = 14
	}
	if s.Weight == 0 {
		s.Weight = Regular
	}
	if s.LineHeight <= 0 {
		s.LineHeight = float32(math.Round(float64(s.Size) * 22 / 14))
		if s.Size == 12 {
			s.LineHeight = 20
		}
	}
	return s
}

// Options control wrapping and truncation.
type Options struct {
	MaxWidth float32 // <=0 or +Inf: no width limit
	MaxLines int     // 0: unlimited
	NoWrap   bool    // keep a single line per paragraph
	Ellipsis bool    // cut overflow with "…" (needs MaxWidth)
	// Direction is the base direction of every paragraph ('\n'-separated
	// block); the zero value DirAuto takes it from the paragraph's first
	// strong character (UAX #9 P2/P3).
	Direction Direction

	// Set by LineCache (linecache.go); the zero values keep Shape's
	// behaviour.
	tabStop float32 // >0: a tab advances to the next multiple of tabStop px
	noLiga  bool    // turn off liga, clig and calt
}

// Line is one laid-out line.
type Line struct {
	Start, End int     // rune range [Start,End) of the source text shown on this line
	Width      float32 // advance width without trailing spaces
	Top        float32
	Baseline   float32
	// RTL reports a right-to-left paragraph level. Glyphs of every line
	// still start at x=0 (visible content spans [0,Width]; trailing spaces
	// of an RTL line hang left of 0), so callers right-align RTL lines
	// themselves when they want natural alignment.
	RTL    bool
	carets []float32 // caret x for rune indices Start..End (see Caret)
	bidi   *bidiLine // per-rune boxes; nil on a pure left-to-right line
}

// Layout is shaped, wrapped text ready for painting and hit testing.
type Layout struct {
	Text   string
	Runes  []rune
	Style  Style
	Width  float32 // widest line
	Height float32
	Lines  []Line
	Glyphs []paint.Glyph // visual order, line by line; origin: layout top-left; Y is the baseline
	// GlyphCluster[g] is the rune index (into Runes) Glyphs[g] was shaped
	// from: the first rune of its cluster (ligatures and combining
	// sequences map to their first rune). -1 marks the synthesized
	// ellipsis. Same length as Glyphs.
	GlyphCluster []int32
	// GlyphRun[g] is the index (into the runs given to ShapeRuns) of the
	// style run Glyphs[g] belongs to; the ellipsis takes the run of the
	// last rune kept on its line. Same length as Glyphs; all 0 in layouts
	// made by Shape (one implicit run).
	GlyphRun   []int32
	Truncated  bool
	Ascent     float32
	LineHeight float32
}

// Size returns the layout box.
func (l *Layout) Size() geom.Size { return geom.Size{W: l.Width, H: l.Height} }

// LineOf returns the index of the line containing rune index i (carets at a
// wrap boundary belong to the next line).
func (l *Layout) LineOf(i int) int {
	for n, ln := range l.Lines {
		if i < ln.End || (i == ln.End && (n == len(l.Lines)-1 || l.Lines[n+1].Start > i)) {
			return n
		}
	}
	return max(0, len(l.Lines)-1)
}

// Caret returns the caret x and the line box for logical rune index i
// (the caret sits before rune i).
//
// With mixed directions a logical position can sit at two visual places
// (the end of the previous run and the start of the next); Caret follows
// the character before it: x is the trailing edge of rune i-1 (its right
// edge if it is left-to-right, its left edge if right-to-left). At the start
// of a line, where there is no previous rune on the line, x is the leading
// edge of rune i (left edge for LTR, right edge for RTL). In pure
// left-to-right text this is simply the pen position before rune i.
func (l *Layout) Caret(i int) (x, top, height float32) {
	if len(l.Lines) == 0 {
		return 0, 0, l.LineHeight
	}
	n := l.LineOf(i)
	ln := l.Lines[n]
	j := i - ln.Start
	if j < 0 {
		j = 0
	}
	if j >= len(ln.carets) {
		j = len(ln.carets) - 1
	}
	if j < 0 {
		return 0, ln.Top, l.LineHeight
	}
	return ln.carets[j], ln.Top, l.LineHeight
}

// IndexAt returns the logical rune index for the point (layout coords): the
// position whose caret (see Caret) is nearest x on the line at y, so
// Caret(IndexAt(x, y)) is the caret drawn closest to x and IndexAt(Caret(i))
// is i or, at a direction boundary, another position drawn at the same x.
// When two positions share the nearest x, the rune under the point decides:
// the left half of a left-to-right rune k means k, its right half k+1; for
// a right-to-left rune the left half means k+1 and the right half k.
func (l *Layout) IndexAt(x, y float32) int {
	if len(l.Lines) == 0 {
		return 0
	}
	n := int(y / l.LineHeight)
	n = max(0, min(n, len(l.Lines)-1))
	ln := l.Lines[n]
	if ln.bidi != nil {
		return ln.bidi.indexAt(ln.Start, x, ln.carets)
	}
	best, bd := ln.Start, float32(math.MaxFloat32)
	for k, cx := range ln.carets {
		d := float32(math.Abs(float64(cx - x)))
		if d < bd {
			best, bd = ln.Start+k, d
		}
	}
	return best
}

type cacheKey struct {
	s    string
	st   Style
	opt  Options
	lang string // resolved Style.Lang (DefaultLang when empty)
}

var (
	mu      sync.Mutex
	cache   = map[cacheKey]*Layout{}
	cacheV  = -1
	shaper  shaping.HarfbuzzShaper
	segm    shaping.Segmenter
	breaker segmenter.Segmenter
	hits    int
)

func clearCache() {
	mu.Lock()
	cache = map[cacheKey]*Layout{}
	runsCache = map[runsKey]*Layout{}
	mu.Unlock()
}

// Shape lays out s. Results are cached; the returned Layout must not be
// mutated.
func Shape(s string, st Style, opt Options) *Layout {
	st = st.norm()
	if geom.IsInf(opt.MaxWidth) || opt.MaxWidth < 0 {
		opt.MaxWidth = 0
	}
	chain := reg.chain(st.Mono)
	reg.mu.Lock()
	v := reg.version
	reg.mu.Unlock()
	lang := resolveLang(st)
	k := cacheKey{s, st, opt, lang}
	mu.Lock()
	defer mu.Unlock()
	if v != cacheV {
		cache = map[cacheKey]*Layout{}
		runsCache = map[runsKey]*Layout{}
		cacheV = v
	}
	if l, ok := cache[k]; ok {
		hits++
		return l
	}
	l := layout(s, st, opt, chain, lang)
	if len(cache) > 8192 {
		cache = map[cacheKey]*Layout{}
	}
	cache[k] = l
	return l
}

// Measure returns the size of s laid out with opt.
func Measure(s string, st Style, opt Options) geom.Size { return Shape(s, st, opt).Size() }

// Width returns the single-line advance width of s.
func Width(s string, st Style) float32 { return Shape(s, st, Options{NoWrap: true}).Width }

// LineHeightOf returns the line height used for st.
func LineHeightOf(st Style) float32 { return st.norm().LineHeight }

type fontmap struct {
	chain  []*family
	eng    *engineState // system engine: per-rune platform fallback
	w      Weight
	italic bool
	byFF   map[*font.Face]*Face
	first  *Face
	feats  []shaping.FontFeature // Input.FontFeatures of every shaped run
}

// noLigaFeatures turns off the ligature features HarfBuzz enables by
// default (Options.noLiga).
var noLigaFeatures = []shaping.FontFeature{
	{Tag: ot.NewTag('l', 'i', 'g', 'a'), Value: 0},
	{Tag: ot.NewTag('c', 'l', 'i', 'g'), Value: 0},
	{Tag: ot.NewTag('c', 'a', 'l', 't'), Value: 0},
}

// ResolveFace picks the font for r: app-registered fonts first; then, for
// CJK and emoji, the system families meant for that script (so 「，」 comes
// from PingFang / YaHei / Noto CJK rather than a Latin font that happens to
// map it); then the chain in order; then the platform's own fallback for
// the rune (CTFontCreateForString, fontconfig charset match).
func (m *fontmap) ResolveFace(r rune) *font.Face {
	if f, _ := m.resolve(r); f != nil {
		return f.face
	}
	if m.first != nil {
		m.byFF[m.first.face] = m.first
		return m.first.face
	}
	return nil
}

// resolve is ResolveFace without the last resort (the first face tried,
// which does not map r): it returns the face mapping r and its family, or
// nil.
func (m *fontmap) resolve(r rune) (*Face, *family) {
	try := func(fam *family) *Face {
		f, _ := fam.pick(m.w, m.italic)
		if f == nil {
			return nil
		}
		if m.first == nil {
			m.first = f
		}
		if f.HasRune(r) {
			m.byFF[f.face] = f
			return f
		}
		return nil
	}
	if cl := classOf(r); cl != classNone {
		for _, fam := range m.chain {
			if fam.user || fam.class == cl {
				if f := try(fam); f != nil {
					return f, fam
				}
			}
		}
	}
	for _, fam := range m.chain {
		if f := try(fam); f != nil {
			return f, fam
		}
	}
	if m.eng != nil && !unicode.IsControl(r) {
		if fam := m.eng.fallbackFamily(r); fam != nil {
			if f := try(fam); f != nil {
				return f, fam
			}
		}
	}
	return nil, nil
}

func (m *fontmap) primary() *Face {
	for _, fam := range m.chain {
		if f, _ := fam.pick(m.w, m.italic); f != nil {
			return f
		}
	}
	return nil
}

type sg struct {
	face       *Face
	gid        uint32
	adv        float32
	xoff, yoff float32
	cluster    int
	run        int32 // style run (index into layoutRuns' runs)
}

func fx(v fixed.Int26_6) float32 { return float32(v) / 64 }

// shapeRange shapes runes[a:b] left-to-right and returns glyphs plus
// per-rune advances.
func shapeRange(runes []rune, a, b int, st Style, fm *fontmap, adv []float32) []sg {
	items := [1]item{{start: a, end: b, st: st, lang: language.NewLanguage(resolveLang(st))}}
	fms := [1]*fontmap{fm}
	return shapeItems(runes, items[:], fms[:], adv)
}

// shapeHook, when set (tests), sees every run handed to the shaper.
var shapeHook func(shaping.Input)

// shapeItems shapes each item (one embedding level, one style) with the
// font map of its style run (fms[it.run]) and returns the glyphs plus
// per-rune advances (added into adv). Glyphs of a right-to-left item come
// in the shaper's visual order; the clusters are placed by the caller in
// line visual order. Without any font (every chain is empty, so fms[0]
// tells) advances are approximated from each item's size.
func shapeItems(runes []rune, items []item, fms []*fontmap, adv []float32) []sg {
	if fms[0].primary() == nil {
		for _, it := range items {
			for i := it.start; i < it.end; i++ {
				adv[i] = approxAdvance(runes[i], it.st.Size)
			}
		}
		return nil
	}
	var out []sg
	for _, it := range items {
		a, b := it.start, it.end
		if a >= b {
			continue
		}
		fm := fms[it.run]
		dir := di.DirectionLTR
		if it.rtl() {
			dir = di.DirectionRTL
		}
		// Split re-runs bidi on the item alone (a single level, so it stays
		// one direction), then cuts it by script (detected per rune) and
		// face, and resolves Language per script.
		in := shaping.Input{Text: runes, RunStart: a, RunEnd: b, Direction: dir, Size: fixed.Int26_6(it.st.Size * 64), Language: it.lang, FontFeatures: fm.feats}
		for _, run := range segm.Split(in, fm) {
			if run.Face == nil {
				continue
			}
			if shapeHook != nil {
				shapeHook(run)
			}
			face := fm.byFF[run.Face]
			o := shaper.Shape(run)
			for _, g := range o.Glyphs {
				gl := sg{face: face, gid: uint32(g.GlyphID), adv: fx(g.XAdvance), xoff: fx(g.XOffset), yoff: fx(g.YOffset), cluster: g.ClusterIndex, run: it.run}
				if g.ClusterIndex >= a && g.ClusterIndex < b {
					adv[g.ClusterIndex] += gl.adv
				}
				out = append(out, gl)
			}
		}
	}
	return out
}

func approxAdvance(r rune, size float32) float32 {
	switch {
	case r == '\t':
		return size * 2
	case r < 0x2e80:
		return size * 0.55
	default:
		return size
	}
}

func layout(s string, st Style, opt Options, chain []*family, langTag string) *Layout {
	return layoutRuns(s, []runStyle{{st: st, chain: chain, lang: langTag}}, nil, nil, opt)
}

// runStyle is one style run of layoutRuns: its normalized Style, the font
// chain for st.Mono and the resolved language tag.
type runStyle struct {
	st    Style
	chain []*family
	lang  string
}

// newFontmaps returns the font map of every run: one per distinct (mono,
// weight, italic), shared by the runs using it (and so its face cache).
// With one run the slice is buf's.
func newFontmaps(rs []runStyle, opt Options, buf *[1]*fontmap) []*fontmap {
	fms := buf[:]
	if len(rs) > 1 {
		fms = make([]*fontmap, len(rs))
	}
	var feats []shaping.FontFeature
	if opt.noLiga {
		feats = noLigaFeatures
	}
	var distinct []int // first run of each font map
	for k, r := range rs {
		for _, p := range distinct {
			if q := rs[p].st; q.Mono == r.st.Mono && q.Weight == r.st.Weight && q.Italic == r.st.Italic {
				fms[k] = fms[p]
				break
			}
		}
		if fms[k] == nil {
			fms[k] = &fontmap{chain: r.chain, w: r.st.Weight, italic: r.st.Italic, byFF: map[*font.Face]*Face{}, feats: feats}
			if len(r.chain) > 0 {
				fms[k].eng = reg.engineFallback()
			}
			if len(rs) > 1 {
				distinct = append(distinct, k)
			}
		}
	}
	return fms
}

// lineMetrics returns the line height (the tallest of the runs in use),
// ascent and descent (the largest of their fonts'); a run is in use when it
// covers some rune of the n (run 0 when runOf is nil or n is 0).
func lineMetrics(rs []runStyle, fms []*fontmap, runOf []int32, n int) (lh, asc, desc float32) {
	var buf [64]bool
	used := buf[:0]
	if len(rs) <= len(buf) {
		used = buf[:len(rs)]
	} else {
		used = make([]bool, len(rs))
	}
	used[0] = runOf == nil || n == 0
	for _, r := range runOf {
		used[r] = true
	}
	for k, r := range rs {
		if !used[k] {
			continue
		}
		a, d := r.st.Size*0.8, r.st.Size*0.2
		if p := fms[k].primary(); p != nil {
			a, d = p.Metrics(r.st.Size)
		}
		lh, asc, desc = max(lh, r.st.LineHeight), max(asc, a), max(desc, d)
	}
	return lh, asc, desc
}

// expandTabs gives every tab of runes[a:b] (one paragraph, logical order)
// the advance that takes the pen to the next multiple of stop, in adv and
// in the first glyph of its cluster (gl may be nil: approximate metrics).
func expandTabs(runes []rune, a, b int, adv []float32, gl []sg, stop float32) {
	var first []int32 // first glyph of each cluster, built on the first tab
	pen := float64(0)
	for i := a; i < b; i++ {
		if runes[i] == '\t' {
			st := float64(stop)
			w := (math.Floor(pen/st)+1)*st - pen
			if w < st*1e-4 { // pen/st rounded below a stop the pen is at
				w += st
			}
			d := float32(w) - adv[i]
			adv[i] = float32(w)
			if gl != nil {
				if first == nil {
					first = make([]int32, b-a)
					for k := range first {
						first[k] = -1
					}
					for g := len(gl) - 1; g >= 0; g-- {
						if c := gl[g].cluster; c >= a && c < b {
							first[c-a] = int32(g)
						}
					}
				}
				if g := first[i-a]; g >= 0 {
					gl[g].adv += d
				}
			}
		}
		pen += float64(adv[i])
	}
}

// addGlyph appends one placed glyph (with its cluster and run) to l; the
// three slices are allocated together on the first glyph, with room for n.
func (l *Layout) addGlyph(g paint.Glyph, cluster, run int32, n int) {
	if l.Glyphs == nil {
		l.Glyphs = make([]paint.Glyph, 0, n)
		l.GlyphCluster = make([]int32, 0, n)
		l.GlyphRun = make([]int32, 0, n)
	}
	l.Glyphs = append(l.Glyphs, g)
	l.GlyphCluster = append(l.GlyphCluster, cluster)
	l.GlyphRun = append(l.GlyphRun, run)
}

// layoutRuns lays out s with the style runs rs; runOf[i] is the run of rune
// i (nil: every rune in run 0, the Shape case) and shapeOf[i] the run whose
// style it is shaped with: the first of the runs that shape alike (see
// prepRuns; nil: runs are not cut for shaping). With one run the result is
// exactly the single-style layout.
func layoutRuns(s string, rs []runStyle, runOf, shapeOf []int32, opt Options) *Layout {
	runes := []rune(s)
	st := rs[0].st
	lang := language.NewLanguage(rs[0].lang)
	var fmBuf [1]*fontmap
	fms := newFontmaps(rs, opt, &fmBuf)
	skew := func(run int32, f *Face) float32 {
		if rs[run].st.Italic && !f.Italic {
			return ItalicSkew
		}
		return 0
	}
	runAt := func(i int) int32 {
		if runOf == nil {
			return 0
		}
		return runOf[i]
	}
	// Line metrics: the tallest line height of the runs in use, and one
	// baseline per line centring the union of their ascents and descents
	// (for a single run: the single-style rule).
	lh, asc, desc := lineMetrics(rs, fms, runOf, len(runes))
	l := &Layout{Text: s, Runes: runes, Style: st, LineHeight: lh}
	l.Ascent = asc
	baseOff := (lh-(asc+desc))/2 + asc
	adv := make([]float32, len(runes)+1)
	maxW := opt.MaxWidth
	wrap := maxW > 0 && !opt.NoWrap

	// Bidi state of the paragraph each span belongs to.
	type paraInfo struct {
		base  bidi.Level
		mixed bool // needs bidi handling (some RTL rune or an RTL paragraph)
	}
	var spans []span
	var paras []paraInfo
	var glyphs []sg
	var items []item
	lv := make([]bidi.Level, len(runes))
	// Paragraphs.
	pa := 0
	for pa <= len(runes) {
		pb := pa
		for pb < len(runes) && runes[pb] != '\n' {
			pb++
		}
		var pi paraInfo
		if opt.Direction == DirRTL {
			pi = paraInfo{base: 1, mixed: true}
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
		n0 := len(spans)
		if !wrap || pb == pa {
			spans = append(spans, span{pa, pb})
		} else {
			spans = append(spans, breakLines(runes, pa, pb, adv, maxW)...)
		}
		for range spans[n0:] {
			paras = append(paras, pi)
		}
		pa = pb + 1
	}
	// Max lines / ellipsis.
	truncLast := false
	if opt.MaxLines > 0 && len(spans) > opt.MaxLines {
		spans = spans[:opt.MaxLines]
		truncLast = true
		l.Truncated = true
	}
	// Ellipsis glyphs per style run (the run of the line's last rune).
	type ellipsis struct {
		gl []sg
		w  float32
	}
	ells := map[int32]ellipsis{}
	needEll := func(sp span) bool {
		return opt.Ellipsis && maxW > 0 && (truncLast || spanWidth(adv, sp.a, sp.b, runes) > maxW+0.01)
	}
	// Build glyph index by cluster for quick lookup.
	byCluster := make([][]int, len(runes)+1)
	for i, g := range glyphs {
		if g.cluster >= 0 && g.cluster < len(byCluster) {
			byCluster[g.cluster] = append(byCluster[g.cluster], i)
		}
	}
	for n, sp := range spans {
		top := float32(n) * lh
		ln := Line{Start: sp.a, End: sp.b, Top: top, Baseline: top + baseOff}
		end := sp.b
		ellipsize := false
		var ell []sg
		var ellW float32
		if (n == len(spans)-1 && needEll(sp)) || (opt.Ellipsis && maxW > 0 && spanWidth(adv, sp.a, sp.b, runes) > maxW+0.01) {
			ellipsize = true
			var er int32
			if runOf != nil && len(runes) > 0 {
				er = runOf[min(max(sp.b-1, 0), len(runes)-1)]
			}
			e, ok := ells[er]
			if !ok {
				eadv := make([]float32, 2)
				e.gl = shapeRange([]rune{'…', ' '}, 0, 1, rs[er].st, fms[er], eadv)
				e.w = eadv[0]
				for i := range e.gl {
					e.gl[i].run = er
				}
				ells[er] = e
			}
			ell, ellW = e.gl, e.w
			for end > sp.a && spanWidth(adv, sp.a, end, runes)+ellW > maxW {
				end--
			}
			for end > sp.a && unicode.IsSpace(runes[end-1]) {
				end--
			}
			l.Truncated = true
		}
		pi := paras[n]
		ln.RTL = pi.base&1 == 1
		var llv []bidi.Level
		if pi.mixed {
			llv = lineLevels(runes, sp.a, end, lv, pi.base)
			mixed := ln.RTL
			for _, v := range llv {
				mixed = mixed || v != 0
			}
			if !mixed {
				llv = nil
			}
		}
		pen := float32(0)
		g0 := len(l.Glyphs)
		ln.carets = make([]float32, 0, end-sp.a+1)
		if llv == nil {
			// Left-to-right only: the original placement.
			for i := sp.a; i < end; i++ {
				ln.carets = append(ln.carets, pen)
				for _, gi := range byCluster[i] {
					g := glyphs[gi]
					if g.face != nil && runes[i] != '\t' {
						l.addGlyph(paint.Glyph{Src: g.face, ID: g.gid, Size: rs[g.run].st.Size, X: pen + g.xoff, Y: ln.Baseline - g.yoff, Skew: skew(g.run, g.face)}, int32(i), runAt(i), len(glyphs)+2)
					}
					pen += g.adv
				}
				if glyphs == nil { // approximate metrics, no fonts
					pen += adv[i]
				}
			}
			ln.carets = append(ln.carets, pen)
		} else {
			// Bidi: walk the runes in visual order (rule L2), placing each
			// cluster's glyphs (kept in shaper order) at the pen.
			k := end - sp.a
			bl := &bidiLine{xl: make([]float32, k), xr: make([]float32, k), rtl: make([]bool, k)}
			if ln.RTL {
				// Trailing spaces of an RTL line sit at its visual left:
				// let them hang left of 0 so the visible text spans
				// [0,Width] as in LTR lines.
				for i := sp.a; i < end; i++ {
					pen -= adv[i]
				}
				pen += spanWidth(adv, sp.a, end, runes)
			}
			for _, j := range visualOrder(llv) {
				i := sp.a + j
				bl.xl[j] = pen
				bl.rtl[j] = llv[j]&1 == 1
				for _, gi := range byCluster[i] {
					g := glyphs[gi]
					if g.face != nil && runes[i] != '\t' {
						l.addGlyph(paint.Glyph{Src: g.face, ID: g.gid, Size: rs[g.run].st.Size, X: pen + g.xoff, Y: ln.Baseline - g.yoff, Skew: skew(g.run, g.face)}, int32(i), runAt(i), len(glyphs)+2)
					}
					pen += g.adv
				}
				if glyphs == nil {
					pen += adv[i]
				}
				bl.xr[j] = pen
			}
			ln.carets = bl.carets()
			ln.bidi = bl
		}
		ln.Width = spanWidth(adv, sp.a, end, runes)
		if ellipsize {
			ex := pen
			if ln.bidi != nil && ln.RTL {
				// The ellipsis goes at the visual end of an RTL line: its
				// left. Shift the line right to make room.
				ex = 0
				for g := g0; g < len(l.Glyphs); g++ {
					l.Glyphs[g].X += ellW
				}
				ln.bidi.shift(ellW)
				for c := range ln.carets {
					ln.carets[c] += ellW
				}
				pen += ellW
			}
			for _, g := range ell {
				if g.face != nil {
					l.addGlyph(paint.Glyph{Src: g.face, ID: g.gid, Size: rs[g.run].st.Size, X: ex + g.xoff, Y: ln.Baseline - g.yoff, Skew: skew(g.run, g.face)}, -1, g.run, len(glyphs)+2)
				}
				ex += g.adv
			}
			ln.Width = max(pen, ex)
			ln.End = end
		}
		l.Width = max(l.Width, ln.Width)
		l.Lines = append(l.Lines, ln)
	}
	l.Height = float32(len(l.Lines)) * lh
	return l
}

// spanWidth sums advances of [a,b) excluding trailing whitespace.
func spanWidth(adv []float32, a, b int, runes []rune) float32 {
	for b > a && (runes[b-1] == ' ' || runes[b-1] == '　' || runes[b-1] == '\t') {
		b--
	}
	var w float32
	for i := a; i < b; i++ {
		w += adv[i]
	}
	return w
}

type span struct{ a, b int }

// breakLines greedily fills lines using UAX #14 break opportunities, falling
// back to per-character breaks for segments wider than a line.
func breakLines(runes []rune, a, b int, adv []float32, maxW float32) []span {
	breaker.Init(runes[a:b])
	it := breaker.LineIterator()
	var out []span
	start, cur := a, a
	for it.Next() {
		ln := it.Line()
		segB := a + ln.Offset + len(ln.Text)
		if spanWidth(adv, start, segB, runes) <= maxW {
			cur = segB
			continue
		}
		if cur > start {
			out = append(out, span{start, cur})
			start = cur
		}
		for spanWidth(adv, start, segB, runes) > maxW {
			j := start + 1
			for j < segB && spanWidth(adv, start, j+1, runes) <= maxW {
				j++
			}
			out = append(out, span{start, j})
			start = j
		}
		cur = segB
	}
	if cur > start || len(out) == 0 {
		out = append(out, span{start, cur})
	}
	return out
}

// TrimSpaceRight is a helper for callers building labels.
func TrimSpaceRight(s string) string { return strings.TrimRight(s, " \t") }
