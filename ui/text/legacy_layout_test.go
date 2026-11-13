package text

// legacy_layout_test.go keeps a frozen copy of the pre-bidi layout (layout
// and shapeRange as they were before bidi.go) so TestPureLTRUnchanged can
// prove that left-to-right text lays out exactly as before.

import (
	"unicode"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"github.com/tangoui/tango/ui/paint"
	"golang.org/x/image/math/fixed"
)

var legacyZhLang = language.NewLanguage("zh-Hans")

// shapeRange shapes runes[a:b] and returns glyphs plus per-rune advances.
func legacyShapeRange(runes []rune, a, b int, st Style, fm *fontmap, adv []float32) []sg {
	if a >= b {
		return nil
	}
	if fm.primary() == nil {
		for i := a; i < b; i++ {
			adv[i] = approxAdvance(runes[i], st.Size)
		}
		return nil
	}
	in := shaping.Input{Text: runes, RunStart: a, RunEnd: b, Direction: di.DirectionLTR, Size: fixed.Int26_6(st.Size * 64), Language: legacyZhLang}
	var out []sg
	for _, run := range segm.Split(in, fm) {
		if run.Face == nil {
			continue
		}
		face := fm.byFF[run.Face]
		o := shaper.Shape(run)
		for _, g := range o.Glyphs {
			gl := sg{face: face, gid: uint32(g.GlyphID), adv: fx(g.XAdvance), xoff: fx(g.XOffset), yoff: fx(g.YOffset), cluster: g.ClusterIndex}
			if g.ClusterIndex >= a && g.ClusterIndex < b {
				adv[g.ClusterIndex] += gl.adv
			}
			out = append(out, gl)
		}
	}
	return out
}

func legacyLayout(s string, st Style, opt Options, chain []*family) *Layout {
	runes := []rune(s)
	fm := &fontmap{chain: chain, w: st.Weight, italic: st.Italic, byFF: map[*font.Face]*Face{}}
	if len(chain) > 0 {
		fm.eng = reg.engineFallback()
	}
	skew := func(f *Face) float32 {
		if st.Italic && !f.Italic {
			return ItalicSkew
		}
		return 0
	}
	l := &Layout{Text: s, Runes: runes, Style: st, LineHeight: st.LineHeight}
	asc, desc := st.Size*0.8, st.Size*0.2
	if p := fm.primary(); p != nil {
		asc, desc = p.Metrics(st.Size)
	}
	l.Ascent = asc
	baseOff := (st.LineHeight-(asc+desc))/2 + asc
	adv := make([]float32, len(runes)+1)
	maxW := opt.MaxWidth
	wrap := maxW > 0 && !opt.NoWrap

	var spans []span
	var glyphs []sg
	// Paragraphs.
	pa := 0
	for pa <= len(runes) {
		pb := pa
		for pb < len(runes) && runes[pb] != '\n' {
			pb++
		}
		gl := legacyShapeRange(runes, pa, pb, st, fm, adv)
		glyphs = append(glyphs, gl...)
		if !wrap || pb == pa {
			spans = append(spans, span{pa, pb})
		} else {
			spans = append(spans, breakLines(runes, pa, pb, adv, maxW)...)
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
	var ell []sg
	var ellW float32
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
		top := float32(n) * st.LineHeight
		ln := Line{Start: sp.a, End: sp.b, Top: top, Baseline: top + baseOff}
		end := sp.b
		ellipsize := false
		if (n == len(spans)-1 && needEll(sp)) || (opt.Ellipsis && maxW > 0 && spanWidth(adv, sp.a, sp.b, runes) > maxW+0.01) {
			ellipsize = true
			if ell == nil {
				eadv := make([]float32, 2)
				ell = legacyShapeRange([]rune{'…', ' '}, 0, 1, st, fm, eadv)
				ellW = eadv[0]
			}
			for end > sp.a && spanWidth(adv, sp.a, end, runes)+ellW > maxW {
				end--
			}
			for end > sp.a && unicode.IsSpace(runes[end-1]) {
				end--
			}
			l.Truncated = true
		}
		pen := float32(0)
		ln.carets = make([]float32, 0, end-sp.a+1)
		for i := sp.a; i < end; i++ {
			ln.carets = append(ln.carets, pen)
			for _, gi := range byCluster[i] {
				g := glyphs[gi]
				if g.face != nil && runes[i] != '\t' {
					l.Glyphs = append(l.Glyphs, paint.Glyph{Src: g.face, ID: g.gid, Size: st.Size, X: pen + g.xoff, Y: ln.Baseline - g.yoff, Skew: skew(g.face)})
				}
				pen += g.adv
			}
			if glyphs == nil { // approximate metrics, no fonts
				pen += adv[i]
			}
		}
		ln.carets = append(ln.carets, pen)
		ln.Width = spanWidth(adv, sp.a, end, runes)
		if ellipsize {
			for _, g := range ell {
				if g.face != nil {
					l.Glyphs = append(l.Glyphs, paint.Glyph{Src: g.face, ID: g.gid, Size: st.Size, X: pen + g.xoff, Y: ln.Baseline - g.yoff, Skew: skew(g.face)})
				}
				pen += g.adv
			}
			ln.Width = pen
			ln.End = end
		}
		l.Width = max(l.Width, ln.Width)
		l.Lines = append(l.Lines, ln)
	}
	l.Height = float32(len(l.Lines)) * st.LineHeight
	return l
}
