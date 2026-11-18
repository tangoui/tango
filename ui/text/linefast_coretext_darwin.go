//go:build darwin && !ios && tango_coretext

package text

import "github.com/tangoui/tango/ui/paint"

// The monospace fast path of LineCache (see LineCache) on CoreText. A line
// qualifies when every run is Mono, ligatures are off, every rune is a
// grapheme cluster of its own (simpleText) with no right-to-left or bidi
// control character (maybeRTL), and every rune but tabs resolves to its
// run's primary face, which maps it (glyph id not 0) and has plain
// advances (Face.plainAdvances: no kerning tables, no morx). CoreText then
// sets each rune as its character map's glyph at that glyph's advance
// (CTFontGetGlyphsForCharacters, CTFontGetAdvancesForGlyphs), which is what
// the general path's CTLine gives; tabs take expandTabs' advances. Default
// ignorable and control characters (CoreText may hide them) take the
// general path. A face with a GSUB or trak table (Face.needsProbe), whose
// default features (ccmp, locl) CoreText applies, takes only lines of
// ASCII, and only in the (size, language) where a probe found CoreText
// setting the whole printable ASCII range as the character map's glyphs
// at their own advances (asciiProbe).

// fastPath memoizes the fast path's font resolution and glyph metrics; it
// lives as long as the font registry version (LineCache.flush).
type fastPath struct {
	fonts   map[fastFontKey]*fastFont
	tables  map[ctSizedFace]*monoTable
	nglyphs int  // memoized non-ASCII glyphs, all tables
	sgs     []sg // scratch
}

type fastFontKey struct {
	w      Weight
	italic bool
}

// fastFont is the monospace font map of one (weight, slant) with its
// primary face and whether each rune resolves to it.
type fastFont struct {
	fm      *fontmap
	primary *Face // nil: no fonts (approximate metrics) or no plain advances
	low     [128]primaryRune
	high    map[rune]bool
}

type primaryRune struct{ ok, done bool }

// monoTable memoizes a face's glyphs and advances at one size.
type monoTable struct {
	ascii [128]monoGlyph
	other map[rune]monoGlyph
	plain map[string]bool // asciiProbe per resolved language
}

type monoGlyph struct {
	gid  uint32
	adv  float32 // px
	done bool
}

func (fp *fastPath) font(w Weight, italic bool) *fastFont {
	k := fastFontKey{w, italic}
	if ff := fp.fonts[k]; ff != nil {
		return ff
	}
	chain := reg.chain(true)
	fm := &fontmap{chain: chain, w: w, italic: italic, noLiga: true}
	if len(chain) > 0 {
		fm.eng = reg.engineFallback()
	}
	ff := &fastFont{fm: fm, high: map[rune]bool{}}
	if p := fm.primary(); p != nil && p.plainAdvances() {
		ff.primary = p
	}
	fp.fonts[k] = ff
	return ff
}

// onPrimary reports whether the general path sets r in the primary face:
// fontmap.resolve picks it and langFace keeps it.
func (ff *fastFont) onPrimary(r rune) bool {
	if r >= 0 && r < 128 {
		if p := &ff.low[r]; p.done {
			return p.ok
		}
	} else if ok, done := ff.high[r]; done {
		return ok
	}
	f, fam := ff.fm.resolve(r)
	ok := f != nil && f == ff.primary && (fam == nil || fam.user || fam.class != classCJK)
	if r >= 0 && r < 128 {
		ff.low[r] = primaryRune{ok, true}
	} else {
		if len(ff.high) >= 4096 {
			clear(ff.high)
		}
		ff.high[r] = ok
	}
	return ok
}

// glyph returns face f's glyph for r and its advance at size (memoized);
// false when the face does not map r.
func (fp *fastPath) glyph(f *Face, size float32, r rune) (monoGlyph, bool) {
	k := ctSizedFace{f, size}
	t := fp.tables[k]
	if t == nil {
		t = &monoTable{}
		fp.tables[k] = t
	}
	if r >= 0 && r < 128 {
		if g := t.ascii[r]; g.done {
			return g, g.gid != 0
		}
	} else if g, ok := t.other[r]; ok {
		return g, g.gid != 0
	}
	g := monoGlyph{done: true}
	if id, ok := f.glyphFor(r); ok {
		if c := f.ctFont(size); c != 0 {
			gid := uint16(id)
			var sz cgSize
			ctFontGetAdvancesForGlyphs(c, ctFontOrientationHorz, &gid, &sz, 1)
			cfRelease(c)
			g.gid, g.adv = id, float32(sz.W)
		}
	}
	if r >= 0 && r < 128 {
		t.ascii[r] = g
	} else {
		if t.other == nil {
			t.other = map[rune]monoGlyph{}
		}
		t.other[r] = g
		if fp.nglyphs++; fp.nglyphs > 1<<15 {
			clear(fp.tables)
			fp.nglyphs = 0
		}
	}
	return g, g.gid != 0
}

// asciiProbe reports whether CoreText, shaping the printable ASCII range
// as one item of face f at size in lang with ligatures off (the general
// path), sets every rune as the glyph and advance fp.glyph gives it, with
// no offset; memoized per (face, size, lang).
func (fp *fastPath) asciiProbe(ff *fastFont, st Style, lang string) bool {
	f := ff.primary
	k := ctSizedFace{f, st.Size}
	t := fp.tables[k]
	if t != nil {
		if ok, done := t.plain[lang]; done {
			return ok
		}
	}
	runes := make([]rune, 0, '~'-' '+1)
	for r := rune(' '); r <= '~'; r++ {
		runes = append(runes, r)
	}
	n := len(runes)
	adv := make([]float32, n+1)
	gl := shapeItems(runes, []item{{start: 0, end: n, st: st, lang: lang}}, []*fontmap{ff.fm}, adv)
	ok := len(gl) == n
	for i := 0; ok && i < n; i++ {
		g := gl[i]
		mg, mapped := fp.glyph(f, st.Size, runes[i])
		ok = mapped && g.face == f && g.cluster == i && g.gid == mg.gid &&
			near5(g.adv, mg.adv) && near5(g.xoff, 0) && near5(g.yoff, 0)
	}
	t = fp.tables[k] // fp.glyph made it (or cleared the tables)
	if t == nil {
		t = &monoTable{}
		fp.tables[k] = t
	}
	if t.plain == nil {
		t.plain = map[string]bool{}
	}
	t.plain[lang] = ok
	return ok
}

func near5(a, b float32) bool { return a-b <= 1e-5 && b-a <= 1e-5 }

// ctIgnorable reports control and default ignorable characters, which
// CoreText may set without the font's glyph or advance.
func ctIgnorable(r rune) bool {
	switch {
	case r < 0x20 || r >= 0x7f && r < 0xa0, r == 0xad, r == 0x34f, r == 0x61c,
		r >= 0x115f && r <= 0x1160, r >= 0x17b4 && r <= 0x17b5, r >= 0x180b && r <= 0x180f,
		r >= 0x200b && r <= 0x200f, r >= 0x2028 && r <= 0x202e, r >= 0x2060 && r <= 0x206f,
		r == 0x3164, r >= 0xfe00 && r <= 0xfe0f, r == 0xfeff, r == 0xffa0, r >= 0xfff0 && r <= 0xfffb,
		r >= 0x1bca0 && r <= 0x1bca3, r >= 0x1d173 && r <= 0x1d17a, r >= 0xe0000 && r <= 0xe0fff:
		return true
	}
	return false
}

// fastShape is the monospace fast path (see LineCache); it returns nil
// when the line needs the general path. The caller holds mu.
func (c *LineCache) fastShape(line string, runs []Run, opt Options) *Layout {
	if !opt.noLiga || opt.Direction == DirRTL || len(runs) == 0 || line == "" {
		return nil
	}
	p := prepRuns(line, runs)
	for _, r := range p.rs {
		if !r.st.Mono {
			return nil
		}
	}
	runes := []rune(line)
	n := len(runes)
	if !simpleText(runes) || maybeRTL(runes) {
		return nil
	}
	tabs := false
	for _, r := range runes {
		if r == '\t' {
			tabs = true
		} else if ctIgnorable(r) {
			return nil // also '\n', '\r'
		}
	}
	if tabs && opt.tabStop <= 0 {
		return nil // CoreText's own tab stops
	}
	if c.fp == nil {
		c.fp = &fastPath{fonts: map[fastFontKey]*fastFont{}, tables: map[ctSizedFace]*monoTable{}}
	}
	fp := c.fp
	ascii := true
	for _, r := range runes {
		ascii = ascii && r < 0x80
	}
	ffs := make([]*fastFont, len(p.rs))
	for k, r := range p.rs {
		ffs[k] = fp.font(r.st.Weight, r.st.Italic)
		if ffs[k].primary == nil {
			return nil
		}
	}
	// Faces CoreText may substitute or track in: ASCII lines in a probed
	// (size, language) only, for every run the line is shaped with (run 0
	// when all runs shape alike).
	for k, r := range p.rs {
		if p.shapeOf == nil && k > 0 || p.shapeOf != nil && !usedShaping(p.shapeOf, int32(k)) {
			continue
		}
		if ffs[k].primary.needsProbe() && (!ascii || !fp.asciiProbe(ffs[k], r.st, r.lang)) {
			return nil
		}
	}
	// One glyph per rune, in its shaping run (the first run shaping like
	// it, as the general path's items).
	gl := fp.sgs[:0]
	adv := make([]float32, n+1)
	for i, r := range runes {
		sr := int32(0)
		if p.shapeOf != nil {
			sr = p.shapeOf[i]
		}
		ff := ffs[sr]
		g := sg{face: ff.primary, cluster: i, run: sr}
		if r != '\t' {
			if !ff.onPrimary(r) {
				return nil
			}
			mg, ok := fp.glyph(ff.primary, p.rs[sr].st.Size, r)
			if !ok {
				return nil
			}
			g.gid, g.adv = mg.gid, mg.adv
		}
		adv[i] = g.adv
		gl = append(gl, g)
	}
	fp.sgs = gl[:0]
	if tabs {
		expandTabs(runes, 0, n, adv, gl, opt.tabStop)
	}

	fms := make([]*fontmap, len(ffs))
	for k, ff := range ffs {
		fms[k] = ff.fm
	}
	lh, asc, desc := lineMetrics(p.rs, fms, p.runOf, n)
	l := &Layout{Text: line, Runes: runes, Style: p.rs[0].st, LineHeight: lh, Ascent: asc}
	ln := Line{Start: 0, End: n, Baseline: (lh-(asc+desc))/2 + asc}
	ln.carets = make([]float32, 0, n+1)
	pen := float32(0)
	for i, g := range gl {
		ln.carets = append(ln.carets, pen)
		if runes[i] != '\t' {
			run := int32(0)
			if p.runOf != nil {
				run = p.runOf[i]
			}
			var skew float32
			if p.rs[g.run].st.Italic && !g.face.Italic {
				skew = ItalicSkew
			}
			l.addGlyph(paint.Glyph{Src: g.face, ID: g.gid, Size: p.rs[g.run].st.Size, X: pen, Y: ln.Baseline, Skew: skew}, int32(i), run, n)
		}
		pen += g.adv
	}
	ln.carets = append(ln.carets, pen)
	ln.Width = spanWidth(adv, 0, n, runes)
	l.Width = ln.Width
	l.Lines = []Line{ln}
	l.Height = lh
	return l
}

// usedShaping reports whether some rune is shaped with run k.
func usedShaping(shapeOf []int32, k int32) bool {
	for _, s := range shapeOf {
		if s == k {
			return true
		}
	}
	return false
}
