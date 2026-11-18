//go:build darwin && !ios && tango_coretext

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
// gives it before its successor. Faces with morx tables, and faces whose
// numeric shaping the pair model does not reproduce, get no table; for
// them Shape takes the general path and ShapeInto shapes with CoreText
// into the destination.

type numericFaceValue struct {
	face  *Face
	fm    *fontmap // the style's font map, for ShapeInto's shaped fallback
	table numericGlyphTable
}

// numericGlyphTable holds, per numeric rune, the face's glyph and its
// advance in px at the size of the key's style (CoreText's advance of the
// sized font), and per ordered pair of numeric runes the advance of the
// first when the second follows it (kerned by the font's GPOS or kern
// table).
type numericGlyphTable struct {
	ok   bool
	gid  [len(numericChars)]uint32
	adv  [len(numericChars)]float32                    // px at the style's size
	kadv [len(numericChars)][len(numericChars)]float32 // px: adv of the first rune of a pair
}

// advance returns the advance of numeric rune index a followed by rune
// index next (-1: the last rune).
func (t *numericGlyphTable) advance(a, next int) float32 {
	if next < 0 {
		return t.adv[a]
	}
	return t.kadv[a][next]
}

// buildNumericTable reads the glyphs and advances of numericChars in face
// at st.Size and verifies them, with the pair advances, against CoreText's
// shaping in lang (probeNumeric); not ok when the face lacks a numeric
// rune, has a morx table or shapes them otherwise.
func buildNumericTable(face *Face, fm *fontmap, st Style, lang string) numericGlyphTable {
	if !face.noMorxTable() {
		return numericGlyphTable{}
	}
	t := numericTableAt(face, st.Size)
	if !t.ok || !probeNumeric(&t, face, fm, st, lang) {
		return numericGlyphTable{}
	}
	return t
}

// numericTableAt reads the character map's glyphs of numericChars in face
// and their advances at size (CTFontGetGlyphsForCharacters,
// CTFontGetAdvancesForGlyphs); not ok when a rune is unmapped.
func numericTableAt(face *Face, size float32) numericGlyphTable {
	var t numericGlyphTable
	c := face.ctFont(size)
	if c == 0 {
		return t
	}
	defer cfRelease(c)
	const n = len(numericChars)
	var chars, glyphs [n]uint16
	for i, r := range numericChars {
		chars[i] = uint16(r)
	}
	if !ctFontGetGlyphsForCharacters(c, &chars[0], &glyphs[0], int64(n)) {
		return t
	}
	for _, g := range glyphs {
		if g == 0 {
			return t
		}
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
// at st's size in lang, every numeric rune alone, every ordered pair and
// numericTriples triples (fixed sequence). It fills t.kadv and reports
// whether CoreText sets them all as t's glyphs, unshifted, each at the
// advance the pair model gives (t.adv, or t.kadv before a successor). Run
// once per table: 13 + 169 + 50 one-to-three-rune CTLines.
func probeNumeric(t *numericGlyphTable, face *Face, fm *fontmap, st Style, lang string) bool {
	if numericProbeHook != nil {
		numericProbeHook(face, st, lang)
	}
	var adv [4]float32
	near := func(a, b float32) bool { return a-b <= numericProbeEps && b-a <= numericProbeEps }
	shape := func(rs []rune) []sg {
		clear(adv[:])
		gl := shapeItems(rs, []item{{start: 0, end: len(rs), st: st, lang: lang}}, []*fontmap{fm}, adv[:len(rs)+1])
		if len(gl) != len(rs) {
			return nil
		}
		for k, g := range gl {
			if g.face != face || g.cluster != k || g.gid != t.gid[numericIndex(rs[k])] || !near(g.xoff, 0) || !near(g.yoff, 0) {
				return nil
			}
		}
		return gl
	}
	var buf [3]rune
	for i, a := range numericChars {
		buf[0] = a
		gl := shape(buf[:1])
		if gl == nil || !near(gl[0].adv, t.adv[i]) {
			return false
		}
		t.adv[i] = gl[0].adv // CoreText's own value, as the general path sees it
	}
	for i, a := range numericChars {
		for j, b := range numericChars {
			buf[0], buf[1] = a, b
			gl := shape(buf[:2])
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
		return int(x>>33) % len(numericChars)
	}
	for range numericTriples {
		a, b, c := next(), next(), next()
		buf[0], buf[1], buf[2] = rune(numericChars[a]), rune(numericChars[b]), rune(numericChars[c])
		gl := shape(buf[:3])
		if gl == nil || !near(gl[0].adv, t.kadv[a][b]) || !near(gl[1].adv, t.kadv[b][c]) || !near(gl[2].adv, t.adv[c]) {
			return false
		}
	}
	return true
}

// numericFaces is guarded by mu; dropped with the shaping caches
// (resetShaper) and when it grows past 1024 styles.
var numericFaces = map[numericFaceKey]numericFaceValue{}

func clearNumericFaces() { numericFaces = map[numericFaceKey]numericFaceValue{} }

// numericFaceFor returns the face every numeric rune of st resolves to;
// false when they resolve to different faces, to none, or to a face the
// general path would replace by language (langFace).
func numericFaceFor(st Style, lang string, version int) (numericFaceValue, bool) {
	k := numericFaceKey{st, lang, version}
	if v, ok := numericFaces[k]; ok {
		return v, v.face != nil
	}
	chain := reg.chain(st.Mono)
	var buf [1]*fontmap
	fm := newFontmaps([]runStyle{{st: st, chain: chain, lang: lang}}, Options{NoWrap: true}, &buf)[0]
	var face *Face
	for _, r := range numericChars {
		f, fam := fm.resolve(r)
		if f == nil || fam != nil && !fam.user && fam.class == classCJK || face != nil && face != f {
			face = nil
			break
		}
		face = f
	}
	v := numericFaceValue{face: face, fm: fm}
	if face != nil {
		v.table = buildNumericTable(face, fm, st, lang)
	}
	if len(numericFaces) >= 1024 {
		clearNumericFaces()
	}
	numericFaces[k] = v
	return v, face != nil
}

// numericOptions reports options the numeric path lays out (one unwrapped
// left-to-right line).
func numericOptions(s string, opt Options) bool {
	return simpleNumericText(s) && opt.NoWrap && !opt.Ellipsis && opt.MaxLines == 0 && opt.Direction != DirRTL
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
	if !ok || !nf.table.ok {
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
		i, next := numericIndex(r), -1
		if k+1 < len(l.Runes) {
			next = numericIndex(l.Runes[k+1])
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
	if !numericOptions(s, opt) {
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
