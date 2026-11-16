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
	if face == nil {
		return nil
	}
	gt := face.gt()
	if gt == nil {
		return nil
	}
	if dst == nil {
		dst = &Layout{}
	}
	l := dst
	oldRunes, oldGlyphs, oldClusters, oldRuns := l.Runes, l.Glyphs, l.GlyphCluster, l.GlyphRun
	*l = Layout{Text: s, Style: st, LineHeight: st.LineHeight}
	l.Runes = oldRunes[:0]
	l.Glyphs = oldGlyphs[:0]
	l.GlyphCluster = oldClusters[:0]
	l.GlyphRun = oldRuns[:0]
	asc, desc := face.Metrics(st.Size)
	l.Ascent, l.Height = asc, st.LineHeight
	pen := float32(0)
	for _, r := range s {
		gid, exists := gt.NominalGlyph(r)
		if !exists {
			return nil
		}
		adv := gt.HorizontalAdvance(gid) * st.Size / face.upem
		l.Runes = append(l.Runes, r)
		l.Glyphs = append(l.Glyphs, paint.Glyph{Src: face, ID: uint32(gid), Size: st.Size, X: pen, Y: (st.LineHeight-(asc+desc))/2 + asc})
		pen += adv
	}
	l.GlyphCluster = make([]int32, len(l.Glyphs))
	l.GlyphRun = make([]int32, len(l.Glyphs))
	for i := range l.GlyphCluster {
		l.GlyphCluster[i] = int32(i)
	}
	l.Lines = []Line{{Start: 0, End: len(l.Runes), Width: pen, Baseline: (st.LineHeight-(asc+desc))/2 + asc}}
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
