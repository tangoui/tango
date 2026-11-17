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

// numericGlyphTable holds, per numeric rune, the face's glyph and its
// advance in font units at the face's variation instance. Reading it avoids
// re-evaluating variable-font advance deltas for every label every frame.
type numericGlyphTable struct {
	ok  bool
	gid [len(numericChars)]uint32
	adv [len(numericChars)]float32 // font units
}

func buildNumericTable(face *Face) numericGlyphTable {
	var t numericGlyphTable
	gt := face.gt()
	if gt == nil {
		return t
	}
	for i, r := range numericChars {
		gid, ok := gt.NominalGlyph(r)
		if !ok {
			return numericGlyphTable{}
		}
		t.gid[i] = uint32(gid)
		t.adv[i] = gt.HorizontalAdvance(gid)
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
		v.table = buildNumericTable(face)
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
	unit := st.Size / face.upem
	pen := float32(0)
	for _, r := range s {
		i := numericIndex(r)
		l.Runes = append(l.Runes, r)
		l.Glyphs = append(l.Glyphs, paint.Glyph{Src: face, ID: nf.table.gid[i], Size: st.Size, X: pen, Y: base})
		l.GlyphCluster = append(l.GlyphCluster, int32(len(l.Glyphs)-1))
		l.GlyphRun = append(l.GlyphRun, 0)
		pen += nf.table.adv[i] * unit
	}
	var carets []float32
	if len(oldLines) > 0 {
		carets = oldLines[0].carets[:0]
	}
	cx := float32(0)
	for _, r := range l.Runes {
		carets = append(carets, cx)
		cx += nf.table.adv[numericIndex(r)] * unit
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
