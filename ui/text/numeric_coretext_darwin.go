//go:build darwin && !ios && !tango_gotext

package text

import "github.com/tangoui/tango/ui/paint"

// The numeric label fast path (Shape, ShapeInto) on CoreText. As with
// go-text, the face of a style's numeric runes is resolved once per (style,
// language, registry version) and its glyph ids and advances are kept in a
// numericGlyphTable, read with CTFontGetGlyphsForCharacters and
// CTFontGetAdvancesForGlyphs at the style's size; a label is then placed
// glyph by glyph without building a CTLine. Every table is verified against
// CoreText's own shaping at the style's size and language when it is built
// (probeNumeric), which also measures the kerning of every ordered pair of
// numeric runes: the table places each glyph with the advance CoreText
// gives it before its successor. Faces with unsupported substitutions or
// shaping the pair model cannot reproduce get no table. For them Shape
// takes the general path and ShapeInto shapes with CoreText into dst.

// The CoreText table covers ctNumericChars: simpleNumericText's runes plus
// '%' and ',' ("+0.12%", "1,234,567"), which a table places only when the
// face maps them and the probe verified them with the others.
const ctNumericChars = "0123456789.-+%,"

// ctNumericBase is how many leading runes of ctNumericChars (those of
// simpleNumericText) every table must place.
const ctNumericBase = len(numericChars)

// ctNumericIndex maps a rune to its slot in numericGlyphTable (-1: none).
func ctNumericIndex(r rune) int {
	switch r {
	case '%':
		return 13
	case ',':
		return 14
	}
	return numericIndex(r)
}

// ctNumericText reports a short label of ctNumericChars runes (at most 31
// bytes, as simpleNumericText).
func ctNumericText(s string) bool {
	if s == "" || len(s) > 31 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if ctNumericIndex(rune(s[i])) < 0 { // every such rune is ASCII
			return false
		}
	}
	return true
}

// numericLabel reports text ShapeInto lays out with the numeric path.
func numericLabel(s string) bool { return ctNumericText(s) }

type numericFaceValue struct {
	face  *Face
	fm    *fontmap // the style's font map, for ShapeInto's shaped fallback
	table *numericGlyphTable
}

// numericGlyphTable holds, per numeric rune, the face's glyph and its
// advance in px at the size of the key's style (CoreText's advance of the
// sized font), and per ordered pair of numeric runes the advance of the
// first when the second follows it (kerned by the font's GPOS or kern
// table). has marks the runes the table places: always the
// simpleNumericText ones, '%' and ',' when the face maps them and they
// passed the probe.
type numericGlyphTable struct {
	ok   bool
	has  [len(ctNumericChars)]bool
	gid  [len(ctNumericChars)]uint32
	adv  [len(ctNumericChars)]float32                      // px at the style's size
	kadv [len(ctNumericChars)][len(ctNumericChars)]float32 // px: adv of the first rune of a pair
}

// advance returns the advance of numeric rune index a followed by rune
// index next (-1: the last rune).
func (t *numericGlyphTable) advance(a, next int) float32 {
	if next < 0 {
		return t.adv[a]
	}
	return t.kadv[a][next]
}

// covers reports whether the table places every rune of s (a
// ctNumericText label).
func (t *numericGlyphTable) covers(s string) bool {
	if !t.ok {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !t.has[ctNumericIndex(rune(s[i]))] {
			return false
		}
	}
	return true
}

// buildNumericTable reads the glyphs and advances of ctNumericChars in face
// at st.Size and verifies them, with the pair advances, against CoreText's
// shaping in lang (probeNumeric); not ok when the face lacks a
// simpleNumericText rune, or if the layout guard or probe rejects it. extra
// allows '%' and ',' (which fm resolves to face); they are dropped again
// when the face lacks them or the probe with them fails.
func buildNumericTable(face *Face, fm *fontmap, st Style, lang string, extra bool) numericGlyphTable {
	o := readOTL(face, lang, true)
	if !o.ok {
		return numericGlyphTable{}
	}
	t := numericTableAt(face, st.Size, extra)
	alphabet := make(map[uint16]bool, len(t.gid))
	for i, g := range t.gid {
		if t.has[i] {
			alphabet[uint16(g)] = true
		}
	}
	o.restrictLigatures(alphabet)
	for i, r := range ctNumericChars {
		if t.has[i] && !o.allows(t.gid[i], otlScriptOf(r)) {
			if i < ctNumericBase {
				return numericGlyphTable{}
			}
			t.has[i] = false
		}
	}
	if t.ok && probeNumeric(&t, face, fm, st, lang) {
		return t
	}
	if !t.ok || !t.has[13] && !t.has[14] {
		return numericGlyphTable{}
	}
	t = numericTableAt(face, st.Size, false)
	if !t.ok || !probeNumeric(&t, face, fm, st, lang) {
		return numericGlyphTable{}
	}
	return t
}

// numericTableAt reads the character map's glyphs of ctNumericChars (the
// simpleNumericText ones only, unless extra) in face and their advances at
// size (CTFontGetGlyphsForCharacters, CTFontGetAdvancesForGlyphs); not ok
// when a simpleNumericText rune is unmapped.
func numericTableAt(face *Face, size float32, extra bool) numericGlyphTable {
	var t numericGlyphTable
	c := face.ctFont(size)
	if c == 0 {
		return t
	}
	defer cfRelease(c)
	const n = len(ctNumericChars)
	var chars, glyphs [n]uint16
	for i, r := range ctNumericChars {
		chars[i] = uint16(r)
	}
	// False when some rune is unmapped (its glyph 0), checked per rune.
	ctFontGetGlyphsForCharacters(c, &chars[0], &glyphs[0], int64(n))
	for i, g := range glyphs {
		if i < ctNumericBase && g == 0 {
			return t
		}
		t.has[i] = g != 0 && (i < ctNumericBase || extra)
	}
	var advs [n]cgSize
	ctFontGetAdvancesForGlyphs(c, ctFontOrientationHorz, &glyphs[0], &advs[0], int64(n))
	for i := range glyphs {
		t.gid[i] = uint32(glyphs[i])
		t.adv[i] = float32(advs[i].W)
	}
	t.ok = true
	return t
}

// numericProbeHook, when set (tests), runs on every probe.
var numericProbeHook func(f *Face, st Style, lang string)

// numericTriples is how many random triples probeNumeric checks against
// the pair model (contextual kerning or substitution beyond pairs).
const numericTriples = 50

// numericProbeEps bounds the difference the probe accepts between
// CoreText's values and the model's (px): well below the 1e-3 the placed
// glyphs of a 31-rune label may drift from the general path.
const numericProbeEps = 1e-5

// probeNumeric shapes, with the general path's CoreText shaping (shapeItems)
// at st's size in lang, every numeric rune of t (t.has) alone, every
// ordered pair and numericTriples triples (fixed sequence). It fills t.kadv
// and reports whether CoreText sets them all as t's glyphs, unshifted, each
// at the advance the pair model gives (t.adv, or t.kadv before a
// successor). Run once per table: 15 + 225 + 50 one-to-three-rune CTLines
// (13 + 169 + 50 without '%' and ',').
func probeNumeric(t *numericGlyphTable, face *Face, fm *fontmap, st Style, lang string) bool {
	if numericProbeHook != nil {
		numericProbeHook(face, st, lang)
	}
	var idx []int // the slots of t's runes
	for i := range ctNumericChars {
		if t.has[i] {
			idx = append(idx, i)
		}
	}
	var adv [4]float32
	near := func(a, b float32) bool { return a-b <= numericProbeEps && b-a <= numericProbeEps }
	var buf [3]rune
	shape := func(slots ...int) []sg {
		rs := buf[:len(slots)]
		for k, i := range slots {
			rs[k] = rune(ctNumericChars[i])
		}
		clear(adv[:])
		gl := shapeItems(rs, []item{{start: 0, end: len(rs), st: st, lang: lang}}, []*fontmap{fm}, adv[:len(rs)+1])
		if len(gl) != len(rs) {
			return nil
		}
		for k, g := range gl {
			if g.face != face || g.cluster != k || g.gid != t.gid[slots[k]] || !near(g.xoff, 0) || !near(g.yoff, 0) {
				return nil
			}
		}
		return gl
	}
	for _, i := range idx {
		gl := shape(i)
		if gl == nil || !near(gl[0].adv, t.adv[i]) {
			return false
		}
		t.adv[i] = gl[0].adv // CoreText's own value, as the general path sees it
	}
	for _, i := range idx {
		for _, j := range idx {
			gl := shape(i, j)
			if gl == nil || !near(gl[1].adv, t.adv[j]) {
				return false
			}
			t.kadv[i][j] = gl[0].adv
		}
	}
	// A fixed pseudo-random sequence (64-bit LCG) of triples.
	x := uint64(0x9E3779B97F4A7C15)
	next := func() int {
		x = x*6364136223846793005 + 1442695040888963407
		return idx[int(x>>33)%len(idx)]
	}
	for range numericTriples {
		a, b, c := next(), next(), next()
		gl := shape(a, b, c)
		if gl == nil || !near(gl[0].adv, t.kadv[a][b]) || !near(gl[1].adv, t.kadv[b][c]) || !near(gl[2].adv, t.adv[c]) {
			return false
		}
	}
	return true
}

// numericFaces is guarded by mu; dropped with the shaping caches
// (resetShaper) and when it grows past 1024 styles. numericTables holds
// the glyph tables per face, size and language, shared by the styles that
// differ only in what the table does not depend on (line height); dropped
// with numericFaces.
var (
	numericFaces  = map[numericFaceKey]numericFaceValue{}
	numericTables = map[numericTableKey]*numericGlyphTable{}
)

type numericTableKey struct {
	face  *Face
	size  float32
	lang  string
	extra bool
}

func clearNumericFaces() {
	numericFaces = map[numericFaceKey]numericFaceValue{}
	numericTables = map[numericTableKey]*numericGlyphTable{}
}

// numericFaceFor returns the face every simpleNumericText rune of st
// resolves to; false when they resolve to different faces, to none, or to
// a face the general path would replace by language (langFace). '%' and
// ',' join the table when they resolve to that face too.
func numericFaceFor(st Style, lang string, version int) (numericFaceValue, bool) {
	k := numericFaceKey{st, lang, version}
	if v, ok := numericFaces[k]; ok {
		return v, v.face != nil
	}
	chain := reg.chainFor(st)
	var buf [1]*fontmap
	fm := newFontmaps([]runStyle{{st: st, chain: chain, lang: lang}}, Options{NoWrap: true}, &buf)[0]
	var face *Face
	extra := true
	for i, r := range ctNumericChars {
		f, fam := fm.resolve(r)
		bad := f == nil || fam != nil && !fam.user && fam.class == classCJK || face != nil && face != f
		if i >= ctNumericBase {
			extra = extra && !bad
			continue
		}
		if bad {
			face = nil
			break
		}
		face = f
	}
	v := numericFaceValue{face: face, fm: fm, table: &numericGlyphTable{}}
	if face != nil {
		tk := numericTableKey{face, st.Size, lang, extra}
		if t := numericTables[tk]; t != nil {
			v.table = t
		} else {
			t := buildNumericTable(face, fm, st, lang, extra)
			v.table = &t
			numericTables[tk] = v.table
		}
	}
	if len(numericFaces) >= 1024 {
		clearNumericFaces()
	}
	numericFaces[k] = v
	return v, face != nil
}

// numericWrapMargin is how far inside the width a wrapping numeric label
// must end for the table to lay it out: the general path, whose advances
// may differ from the table's by numericProbeEps per glyph, then keeps it
// on one line as well.
const numericWrapMargin = 1e-3

// numericOptions reports options the numeric path lays out: one
// left-to-right line, unwrapped, or wrapping at opt.MaxWidth (the caller
// checks that the label fits; see numericFits).
func numericOptions(s string, opt Options) bool {
	return ctNumericText(s) && !opt.Ellipsis && opt.MaxLines == 0 && opt.Direction != DirRTL
}

// numericFits reports whether s, laid out from t, stays on one line with
// opt: always unwrapped; wrapping (normOptions: MaxWidth > 0) only when it
// ends numericWrapMargin inside the width, where the general path, which
// wraps a paragraph only when it does not fit (fitsLine), keeps it whole.
func numericFits(t *numericGlyphTable, s string, opt Options) bool {
	if opt.NoWrap || opt.MaxWidth <= 0 {
		return true
	}
	pen := float32(0)
	for k := 0; k < len(s); k++ {
		next := -1
		if k+1 < len(s) {
			next = ctNumericIndex(rune(s[k+1]))
		}
		pen += t.advance(ctNumericIndex(rune(s[k])), next)
	}
	return pen <= opt.MaxWidth-numericWrapMargin
}

func directNumericLayout(s string, st Style, opt Options, version int) *Layout {
	return directNumericLayoutInto(nil, s, st, opt, version)
}

// directNumericLayoutInto places s from the numeric glyph table into dst
// (reusing its slices; a new Layout when dst is nil); nil when the table
// does not apply.
func directNumericLayoutInto(dst *Layout, s string, st Style, opt Options, version int) *Layout {
	st = st.norm()
	if !numericOptions(s, opt) {
		return nil
	}
	nf, ok := numericFaceFor(st, resolveLang(st), version)
	if !ok || !nf.table.covers(s) || !numericFits(nf.table, s, opt) {
		return nil
	}
	face := nf.face
	if dst == nil {
		dst = &Layout{}
	}
	l := dst
	oldRunes, oldGlyphs, oldClusters, oldRuns, oldLines := l.Runes, l.Glyphs, l.GlyphCluster, l.GlyphRun, l.Lines
	*l = Layout{Text: s, Style: st, LineHeight: st.LineHeight}
	l.Runes = oldRunes[:0]
	l.Glyphs = oldGlyphs[:0]
	l.GlyphCluster = oldClusters[:0]
	l.GlyphRun = oldRuns[:0]
	asc, desc := nf.fm.primary().Metrics(st.Size) // the line metrics of the general path (lineMetrics)
	l.Ascent, l.Height = asc, st.LineHeight
	base := (st.LineHeight-(asc+desc))/2 + asc
	var skew float32
	if st.Italic && !face.Italic {
		skew = ItalicSkew
	}
	var carets []float32
	if len(oldLines) > 0 {
		carets = oldLines[0].carets[:0]
	}
	for _, r := range s {
		l.Runes = append(l.Runes, r)
	}
	pen := float32(0)
	for k, r := range l.Runes {
		i, next := ctNumericIndex(r), -1
		if k+1 < len(l.Runes) {
			next = ctNumericIndex(l.Runes[k+1])
		}
		carets = append(carets, pen)
		l.Glyphs = append(l.Glyphs, paint.Glyph{Src: face, ID: nf.table.gid[i], Size: st.Size, X: pen, Y: base, Skew: skew})
		l.GlyphCluster = append(l.GlyphCluster, int32(k))
		l.GlyphRun = append(l.GlyphRun, 0)
		pen += nf.table.advance(i, next)
	}
	carets = append(carets, pen)
	l.Lines = append(oldLines[:0], Line{Start: 0, End: len(l.Runes), Width: pen, Baseline: base, carets: carets})
	l.Width = pen
	return l
}

func simpleNumericLayout(s string, st Style, opt Options, version int) *Layout {
	return directNumericLayout(s, st, opt, version)
}

// simpleNumericLayoutInto is directNumericLayoutInto, or when the glyph
// table does not apply (kerning, morx), s shaped by CoreText into dst's
// slices; nil when s or opt is not a numeric label, or no face maps it.
func simpleNumericLayoutInto(dst *Layout, s string, st Style, opt Options, version int) *Layout {
	if fast := directNumericLayoutInto(dst, s, st, opt, version); fast != nil {
		return fast
	}
	st = st.norm()
	if !numericOptions(s, opt) || !opt.NoWrap || !simpleNumericText(s) {
		return nil
	}
	lang := resolveLang(st)
	nf, ok := numericFaceFor(st, lang, version)
	if !ok {
		return nil
	}
	if dst == nil {
		dst = &Layout{}
	}
	oldRunes, oldGlyphs, oldClusters, oldRuns := dst.Runes, dst.Glyphs, dst.GlyphCluster, dst.GlyphRun
	var oldCarets []float32
	if len(dst.Lines) > 0 {
		oldCarets = dst.Lines[0].carets
	}
	runes := oldRunes[:0]
	for _, r := range s {
		runes = append(runes, r)
	}
	n := len(runes)
	adv := make([]float32, n+1)
	gl := shapeItems(runes, []item{{start: 0, end: n, st: st, lang: lang}}, []*fontmap{nf.fm}, adv)
	if len(gl) == 0 {
		return nil
	}
	asc, desc := nf.fm.primary().Metrics(st.Size)
	base := (st.LineHeight-(asc+desc))/2 + asc
	l := dst
	*l = Layout{Text: s, Style: st, LineHeight: st.LineHeight, Ascent: asc}
	l.Runes = runes
	l.Glyphs = oldGlyphs[:0]
	l.GlyphCluster = oldClusters[:0]
	l.GlyphRun = oldRuns[:0]
	carets := oldCarets[:0]
	pen := float32(0)
	next := 0
	for _, g := range gl {
		for next <= g.cluster && next < n {
			carets = append(carets, pen)
			next++
		}
		var skew float32
		if st.Italic && !g.face.Italic {
			skew = ItalicSkew
		}
		l.addGlyph(paint.Glyph{Src: g.face, ID: g.gid, Size: st.Size, X: pen + g.xoff, Y: base - g.yoff, Skew: skew}, int32(g.cluster), 0, len(gl))
		pen += g.adv
	}
	for next <= n {
		carets = append(carets, pen)
		next++
	}
	l.Lines = []Line{{Start: 0, End: n, Width: pen, Baseline: base, carets: carets}}
	l.Width, l.Height = pen, st.LineHeight
	return l
}
