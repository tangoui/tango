package text

import (
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"github.com/tangoui/tango/ui/paint"
	"golang.org/x/image/math/fixed"
	"unicode/utf8"
)

type numericFaceKey struct {
	st      Style
	lang    string
	version int
}
type numericFaceValue struct {
	face  *Face
	feats []shaping.FontFeature
	table numericGlyphTable
}

// numericChars are the runes simpleNumericText accepts; numericIndex maps a
// rune to its slot in numericGlyphTable.
const numericChars = "0123456789.-+"

func numericIndex(r rune) int {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0')
	case r == '.':
		return 10
	case r == '-':
		return 11
	case r == '+':
		return 12
	}
	return -1
}

// numericGlyphTable lays out numeric labels without shaping each one, yet
// exactly as HarfBuzz would: it is built by shaping, once per style, every
// numeric rune alone (its glyph and 26.6-rounded advance) and every ordered
// pair (the pair kerning added to the first glyph's advance), and is only
// used when a probe set laid out from it matches full shaping glyph for
// glyph (numericProbes). A numeric label thus measures and places the same
// on the fast path, through HarfBuzz, and in either base direction.
type numericGlyphTable struct {
	ok   bool
	gid  [len(numericChars)]uint32
	adv  [len(numericChars)]float32                    // px, as shaped alone
	kern [len(numericChars)][len(numericChars)]float32 // px added to the first glyph's advance
}

// numericProbes check a table against full shaping: kerning around the dot
// and signs, repeated digits, and longer runs.
var numericProbes = []string{"12.34", "1.1", "-0.5", "+7", "100.00", "-123.45", "99.99", "7.7.7", "1-1", "+-0", "0123456789", "452.79"}

// shapeNumeric shapes runes as the general path does for a feature-free
// numeric face.
func shapeNumeric(face *Face, runes []rune, st Style, lang string) []sg {
	in := shaping.Input{Text: runes, RunEnd: len(runes), Direction: di.DirectionLTR, Script: language.Latin, Language: language.NewLanguage(lang), Face: face.gt(), Size: fixed.Int26_6(st.Size * 64)}
	return advanceShaper.append(nil, in, face, 0)
}

func buildNumericTable(face *Face, st Style, lang string) numericGlyphTable {
	var t numericGlyphTable
	if face.gt() == nil {
		return t
	}
	chars := []rune(numericChars)
	for i, r := range chars {
		gl := shapeNumeric(face, []rune{r}, st, lang)
		if len(gl) != 1 || gl[0].face != face || gl[0].xoff != 0 || gl[0].yoff != 0 {
			return numericGlyphTable{}
		}
		t.gid[i], t.adv[i] = gl[0].gid, gl[0].adv
	}
	for i, a := range chars {
		for j, b := range chars {
			gl := shapeNumeric(face, []rune{a, b}, st, lang)
			if len(gl) != 2 || gl[0].gid != t.gid[i] || gl[1].gid != t.gid[j] || gl[1].xoff != 0 || gl[1].yoff != 0 {
				return numericGlyphTable{}
			}
			t.kern[i][j] = gl[0].adv - t.adv[i]
		}
	}
	for _, p := range numericProbes {
		runes := []rune(p)
		gl := shapeNumeric(face, runes, st, lang)
		if len(gl) != len(runes) {
			return numericGlyphTable{}
		}
		x, want := float32(0), float32(0)
		for k, r := range runes {
			i := numericIndex(r)
			if gl[k].gid != t.gid[i] || gl[k].xoff != 0 || x != want {
				return numericGlyphTable{}
			}
			x += t.adv[i]
			if k+1 < len(runes) {
				x += t.kern[i][numericIndex(runes[k+1])]
			}
			want += gl[k].adv
		}
		if x != want {
			return numericGlyphTable{}
		}
	}
	t.ok = true
	return t
}

var numericFaces = map[numericFaceKey]numericFaceValue{}

func clearNumericFaces() { numericFaces = map[numericFaceKey]numericFaceValue{} }
func simpleNumericText(s string) bool {
	if s == "" || len(s) > 31 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r == '.' || r == '-' || r == '+') {
			return false
		}
	}
	return true
}
func numericFaceFor(st Style, lang string, version int) (numericFaceValue, bool) {
	k := numericFaceKey{st, lang, version}
	if v, ok := numericFaces[k]; ok {
		return v, true
	}
	chain := reg.chain(st.Mono)
	var buf [1]*fontmap
	fm := newFontmaps([]runStyle{{st: st, chain: chain, lang: lang}}, Options{NoWrap: true}, &buf)[0]
	var face *Face
	for _, r := range "0123456789.-+" {
		f, _ := fm.resolve(r)
		if f == nil {
			return numericFaceValue{}, false
		}
		if face == nil {
			face = f
		} else if face != f {
			return numericFaceValue{}, false
		}
	}
	v := numericFaceValue{face: face, feats: append([]shaping.FontFeature(nil), fm.feats...)}
	if len(v.feats) == 0 {
		v.table = buildNumericTable(face, st, lang)
	}
	numericFaces[k] = v
	return v, true
}
func directNumericLayout(s string, st Style, opt Options, version int) *Layout {
	return directNumericLayoutInto(nil, s, st, opt, version)
}

func directNumericLayoutInto(dst *Layout, s string, st Style, opt Options, version int) *Layout {
	st = st.norm()
	if !simpleNumericText(s) || !opt.NoWrap || opt.Ellipsis || opt.MaxLines != 0 || opt.Direction == DirRTL {
		return nil
	}
	nf, ok := numericFaceFor(st, resolveLang(st), version)
	if !ok || len(nf.feats) != 0 {
		return nil // preserve feature-bearing shaping semantics
	}
	face := nf.face
	if face == nil || !nf.table.ok {
		return nil
	}
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
	asc, desc := face.Metrics(st.Size)
	l.Ascent, l.Height = asc, st.LineHeight
	base := (st.LineHeight-(asc+desc))/2 + asc
	var carets []float32
	if len(oldLines) > 0 {
		carets = oldLines[0].carets[:0]
	}
	pen, prev := float32(0), -1
	for _, r := range s {
		i := numericIndex(r)
		if prev >= 0 {
			pen += nf.table.kern[prev][i]
		}
		carets = append(carets, pen)
		l.Runes = append(l.Runes, r)
		l.Glyphs = append(l.Glyphs, paint.Glyph{Src: face, ID: nf.table.gid[i], Size: st.Size, X: pen, Y: base})
		l.GlyphCluster = append(l.GlyphCluster, int32(len(l.Glyphs)-1))
		l.GlyphRun = append(l.GlyphRun, 0)
		pen += nf.table.adv[i]
		prev = i
	}
	carets = append(carets, pen)
	l.Lines = append(oldLines[:0], Line{Start: 0, End: len(l.Runes), Width: pen, Baseline: base, carets: carets})
	l.Width = pen
	return l
}

func simpleNumericLayout(s string, st Style, opt Options, version int) *Layout {
	return directNumericLayout(s, st, opt, version)
}

func simpleNumericLayoutInto(dst *Layout, s string, st Style, opt Options, version int) *Layout {
	if fast := directNumericLayoutInto(dst, s, st, opt, version); fast != nil {
		return fast
	}
	st = st.norm()
	if !simpleNumericText(s) || !opt.NoWrap || opt.Ellipsis || opt.MaxLines != 0 || opt.Direction == DirRTL {
		return nil
	}
	nf, ok := numericFaceFor(st, resolveLang(st), version)
	if !ok {
		return nil
	}
	face := nf.face
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
	in := shaping.Input{Text: runes, RunEnd: len(runes), Direction: di.DirectionLTR, Script: language.Latin, Language: language.NewLanguage(resolveLang(st)), Face: face.gt(), Size: fixed.Int26_6(st.Size * 64), FontFeatures: nf.feats}
	gl := advanceShaper.append(dst.scratch[:0], in, face, 0)
	asc, desc := face.Metrics(st.Size)
	l := dst
	*l = Layout{Text: s, Style: st, LineHeight: st.LineHeight, Ascent: asc, scratch: gl}
	l.Runes = oldRunes[:0]
	l.Runes = append(l.Runes, runes...)
	l.Glyphs = oldGlyphs[:0]
	l.GlyphCluster = oldClusters[:0]
	l.GlyphRun = oldRuns[:0]
	carets := oldCarets[:0]
	if cap(carets) < len(runes)+1 {
		carets = make([]float32, len(runes)+1)
	} else {
		carets = carets[:len(runes)+1]
	}
	pen := float32(0)
	next := 0
	for _, g := range gl {
		for next <= g.cluster && next < len(carets) {
			carets[next] = pen
			next++
		}
		l.addGlyph(paint.Glyph{Src: g.face, ID: g.gid, Size: st.Size, X: pen + g.xoff, Y: 0}, int32(g.cluster), 0, len(gl))
		pen += g.adv
	}
	for next < len(carets) {
		carets[next] = pen
		next++
	}
	base := (st.LineHeight-(asc+desc))/2 + asc
	for i := range l.Glyphs {
		l.Glyphs[i].Y = base
	}
	l.Lines = []Line{{Start: 0, End: len(runes), Width: pen, Baseline: base, carets: carets}}
	l.Width, l.Height = pen, st.LineHeight
	return l
}
