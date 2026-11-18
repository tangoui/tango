//go:build !darwin || ios || !tango_coretext

package text

import (
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// Shaping with go-text (HarfBuzz port): shaping.Segmenter cuts each item by
// script and face, compactShaper shapes the runs.

var (
	advanceShaper compactShaper
	segm          shaping.Segmenter
)

// resetShaper drops the shaping caches (clearPrepared).
func resetShaper() {
	clearNumericFaces()
	advanceShaper = compactShaper{}
}

// fontmapImpl is the engine part of fontmap: the reverse mapping from the
// go-text faces shaping.Segmenter hands out to their Face.
type fontmapImpl = gtFaceMap

// gtFaceMap maps go-text faces back to the Face they came from.
type gtFaceMap struct {
	byFF       map[*font.Face]*Face
	inlineFont *font.Face
	inlineFace *Face
}

// Most labels use one typeface. Keep its reverse mapping inline and
// allocate a map only when fallback introduces a second font.
func (m *gtFaceMap) remember(gt *font.Face, f *Face) {
	if m.inlineFont == nil {
		m.inlineFont, m.inlineFace = gt, f
	}
	if gt == m.inlineFont {
		m.inlineFace = f
		if m.byFF != nil {
			m.byFF[gt] = f
		}
		return
	}
	if m.byFF == nil {
		m.byFF = map[*font.Face]*Face{m.inlineFont: m.inlineFace}
	}
	m.byFF[gt] = f
}
func (m *gtFaceMap) fromFont(gt *font.Face) *Face {
	if gt == m.inlineFont {
		return m.inlineFace
	}
	return m.byFF[gt]
}

// rememberFace records f as the face resolve picked (see ResolveFace).
func (m *fontmap) rememberFace(f *Face) { m.remember(f.gt(), f) }

// features returns the Input.FontFeatures of every run m shapes.
func (m *fontmap) features() []shaping.FontFeature {
	if m.noLiga {
		return noLigaFeatures
	}
	return nil
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
		return f.gt()
	}
	if m.first != nil {
		m.remember(m.first.gt(), m.first)
		return m.first.gt()
	}
	return nil
}

func fx(v fixed.Int26_6) float32 { return float32(v) / 64 }

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
		in := shaping.Input{Text: runes, RunStart: a, RunEnd: b, Direction: dir, Size: fixed.Int26_6(it.st.Size * 64), Language: language.NewLanguage(it.lang), FontFeatures: fm.features()}
		for _, run := range segm.Split(in, fm) {
			if run.Face == nil {
				continue
			}
			if shapeHook != nil {
				shapeHook(run)
			}
			face := fm.fromFont(run.Face)
			first := len(out)
			out = advanceShaper.append(out, run, face, it.run)
			for _, gl := range out[first:] {
				if gl.cluster >= a && gl.cluster < b {
					adv[gl.cluster] += gl.adv
				}
			}
		}
	}
	return out
}
