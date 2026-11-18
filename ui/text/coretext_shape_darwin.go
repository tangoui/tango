//go:build darwin && (ios || !tango_coretext)

package text

import (
	"os"

	"github.com/tangoui/tango/ui/paint"
)

// coreTextShape is an opt-in lightweight path for short, unwrapped text.
// CoreText owns shaping/fallback; ctRaster still owns deterministic glyph
// rasterization for paint CPU and the GPU atlas.
func coreTextShape(s string, st Style, opt Options) *Layout {
	st = st.norm()
	if cfStringCreateWithBytes == nil {
		if openCoreText() == nil {
			return nil
		}
	}
	if os.Getenv("TANGO_TEXT_ENGINE") != "coretext" || opt.Ellipsis || opt.MaxLines != 0 || !opt.NoWrap || opt.Direction == DirRTL || st.Italic {
		return nil
	}
	if s == "" || len([]rune(s)) < 2 {
		return nil
	}
	raw := []byte(s)
	if len(raw) == 0 {
		return nil
	}
	cs := cfStringCreateWithBytes(0, &raw[0], int64(len(raw)), cfStringEncodingUTF8, false)
	if cs == 0 {
		return nil
	}
	defer cfRelease(cs)
	base := uiFontWeighted(st.Weight, false, 17)
	if base == 0 {
		return nil
	}
	defer cfRelease(base)
	sized := ctFontCreateCopyWithAttributes(base, float64(st.Size), nil, 0)
	if sized == 0 {
		return nil
	}
	defer cfRelease(sized)
	keys := []uintptr{kCTFontAttributeName}
	vals := []uintptr{sized}
	attrs := cfDict(keys, vals)
	if attrs == 0 {
		return nil
	}
	defer cfRelease(attrs)
	lineAttrs := cfAttributedStringCreate(0, cs, attrs)
	if lineAttrs == 0 {
		return nil
	}
	defer cfRelease(lineAttrs)
	line := ctLineCreateWithAttributedString(lineAttrs)
	if line == 0 {
		return nil
	}
	defer cfRelease(line)
	runs := ctLineGetGlyphRuns(line)
	if runs == 0 {
		return nil
	}
	runes := []rune(s)
	u16 := make([]int, 0, len(s))
	// Map UTF-16 string indices to rune indices.
	for i, r := range runes {
		n := 1
		if r > 0xffff {
			n = 2
		}
		for j := 0; j < n; j++ {
			u16 = append(u16, i)
		}
	}
	type runData struct {
		face   *Face
		glyphs []uint16
		pos    []cgPoint
		idx    []int64
		adv    []cgSize
	}
	var all []struct {
		f *Face
		g uint16
		x cgPoint
		i int64
		a cgSize
	}
	var asc, desc float32
	for i := int64(0); i < cfArrayGetCount(runs); i++ {
		run := cfArrayGetValueAtIndex(runs, i)
		n := int(ctRunGetGlyphCount(run))
		if n == 0 {
			continue
		}
		gs := make([]uint16, n)
		ps := make([]cgPoint, n)
		ix := make([]int64, n)
		as := make([]cgSize, n)
		ctRunGetGlyphs(run, cfRange{}, &gs[0])
		ctRunGetPositions(run, cfRange{}, &ps[0])
		ctRunGetAdvances(run, cfRange{}, &as[0])
		ctRunGetStringIndices(run, cfRange{}, &ix[0])
		ra := ctRunGetAttributes(run)
		rf := cfDictionaryGetValue(ra, kCTFontAttributeName)
		if rf == 0 {
			return nil
		}
		own := newCTFontOwner(cfRetain(rf), cfRelease)
		r := newCTRasterOwned(own, nil)
		face := &Face{Family: "CoreText", Weight: st.Weight, faceImpl: faceImpl{face: nil, upem: r.upem, asc: float32(ctFontGetAscent(rf)), desc: float32(ctFontGetDescent(rf)), unitPath: r.unitPath}, ras: r, paths: map[uint32]*paint.Path{}}
		if face.asc > asc {
			asc = face.asc
		}
		if face.desc > desc {
			desc = face.desc
		}
		for j := range gs {
			all = append(all, struct {
				f *Face
				g uint16
				x cgPoint
				i int64
				a cgSize
			}{face, gs[j], ps[j], ix[j], as[j]})
		}
	}
	if len(all) == 0 {
		return nil
	}
	l := &Layout{Text: s, Runes: runes, Style: st, LineHeight: st.LineHeight, Ascent: asc}
	carets := make([]float32, len(runes)+1)
	for i := range carets {
		carets[i] = 0
	}
	for _, g := range all {
		ri := 0
		if g.i >= 0 && int(g.i) < len(u16) {
			ri = u16[g.i]
		}
		l.Glyphs = append(l.Glyphs, paint.Glyph{Src: g.f, ID: uint32(g.g), Size: st.Size, X: float32(g.x.X), Y: asc})
		l.GlyphCluster = append(l.GlyphCluster, int32(ri))
		l.GlyphRun = append(l.GlyphRun, 0)
		if ri >= 0 && ri < len(carets) {
			carets[ri] = float32(g.x.X)
		}
	}
	// CoreText positions already include advances; use the last run position + advance.
	width := float32(0)
	for _, g := range all {
		if w := float32(g.x.X + g.a.W); w > width {
			width = w
		}
	}
	if len(carets) > 0 {
		carets[len(carets)-1] = width
	}
	for i := len(carets) - 2; i >= 0; i-- {
		if carets[i] == 0 {
			carets[i] = carets[i+1]
		}
	}
	l.Width = width
	l.Height = st.LineHeight
	l.Lines = []Line{{Start: 0, End: len(runes), Width: width, Baseline: asc, carets: carets}}
	return l
}
