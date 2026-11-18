//go:build darwin && !ios && tango_coretext

package text

import "github.com/tangoui/tango/ui/paint"

// The numeric label fast path (Shape, ShapeInto) on CoreText. As with
// go-text, the face of a style's numeric runes is resolved once per (style,
// language, registry version) and its glyph ids and advances are kept in a
// numericGlyphTable, read with CTFontGetGlyphsForCharacters and
// CTFontGetAdvancesForGlyphs at the style's size; a label is then placed
// glyph by glyph without building a CTLine. Faces with plain advances (no
// kerning tables, no morx) qualify, and so do faces whose only kerning
// table is GPOS when a one-time probe (numericUnkerned) finds their numeric
// runes unkerned; for the others Shape takes the general path and
// ShapeInto shapes with CoreText into the destination.

type numericFaceValue struct {
	face  *Face
	fm    *fontmap // the style's font map, for ShapeInto's shaped fallback
	table numericGlyphTable
}

// numericGlyphTable holds, per numeric rune, the face's glyph and its
// advance in px at the size of the key's style (CoreText's advance of the
// sized font, which the general path's CTLine reports too).
type numericGlyphTable struct {
	ok  bool
	gid [len(numericChars)]uint32
	adv [len(numericChars)]float32 // px at the style's size
}

// buildNumericTable reads the glyphs and advances of numericChars in face
// at size; not ok when the face lacks a numeric rune or may kern them.
func buildNumericTable(face *Face, fm *fontmap, size float32) numericGlyphTable {
	if !face.plainAdvances() && !(face.gposKernOnly() && numericUnkerned(face, fm)) {
		return numericGlyphTable{}
	}
	return numericTableAt(face, size)
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

// numericProbes caches numericUnkerned per face; guarded by mu, cleared
// with numericFaces.
var numericProbes = map[*Face]bool{}

// numericProbeHook, when set (tests), runs on every probe.
var numericProbeHook func(*Face)

// numericProbeSize is the size the probe shapes at: large enough that a
// kerning of one font unit (1/2048 em) shows well above the tolerance.
const numericProbeSize = 64

// numericUnkerned reports whether CoreText sets every numeric rune of face
// alone and every ordered pair of them as their character map's glyphs at
// their own advances, with no offsets: the face's GPOS does not kern its
// numeric set. fm resolves every numeric rune to face. The answer is
// probed once per face (182 short CTLines).
func numericUnkerned(face *Face, fm *fontmap) bool {
	if ok, done := numericProbes[face]; done {
		return ok
	}
	ok := probeNumeric(face, fm)
	numericProbes[face] = ok
	return ok
}

func probeNumeric(face *Face, fm *fontmap) bool {
	if numericProbeHook != nil {
		numericProbeHook(face)
	}
	t := numericTableAt(face, numericProbeSize)
	if !t.ok {
		return false
	}
	st := Style{Size: numericProbeSize}.norm()
	var buf [2]rune
	var adv [3]float32
	near := func(a, b float32) bool { return a-b <= 1e-3 && b-a <= 1e-3 }
	plain := func(rs []rune) bool {
		clear(adv[:])
		gl := shapeItems(rs, []item{{start: 0, end: len(rs), st: st}}, []*fontmap{fm}, adv[:len(rs)+1])
		if len(gl) != len(rs) {
			return false
		}
		for k, g := range gl {
			i := numericIndex(rs[k])
			if g.face != face || g.gid != t.gid[i] || g.cluster != k || !near(g.adv, t.adv[i]) || !near(g.xoff, 0) || !near(g.yoff, 0) {
				return false
			}
		}
		return true
	}
	for _, a := range numericChars {
		buf[0] = a
		if !plain(buf[:1]) {
			return false
		}
		for _, b := range numericChars {
			buf[1] = b
			if !plain(buf[:2]) {
				return false
			}
		}
	}
	return true
}

// numericFaces is guarded by mu; dropped with the shaping caches
// (resetShaper) and when it grows past 1024 styles.
var numericFaces = map[numericFaceKey]numericFaceValue{}

func clearNumericFaces() {
	numericFaces = map[numericFaceKey]numericFaceValue{}
	numericProbes = map[*Face]bool{}
}

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
		v.table = buildNumericTable(face, fm, st.Size)
	}
	if len(numericFaces) >= 1024 {
		numericFaces = map[numericFaceKey]numericFaceValue{} // the probes stay: per face, few
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
	pen := float32(0)
	for _, r := range s {
		i := numericIndex(r)
		carets = append(carets, pen)
		l.Runes = append(l.Runes, r)
		l.Glyphs = append(l.Glyphs, paint.Glyph{Src: face, ID: nf.table.gid[i], Size: st.Size, X: pen, Y: base, Skew: skew})
		l.GlyphCluster = append(l.GlyphCluster, int32(len(l.Glyphs)-1))
		l.GlyphRun = append(l.GlyphRun, 0)
		pen += nf.table.adv[i]
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
