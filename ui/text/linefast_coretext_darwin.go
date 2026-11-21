//go:build darwin && !ios && !tango_gotext

package text

import (
	"slices"
	"strings"

	"github.com/tangoui/tango/ui/paint"
)

// The monospace fast path of LineCache (see LineCache) on CoreText. A line
// qualifies when every run is Mono, ligatures are off, every rune is a
// grapheme cluster of its own (simpleText) with no right-to-left or bidi
// control character (maybeRTL), no rune is a default ignorable or control
// character (CoreText may hide them) and every rune resolves to a face.
// Each rune is then set by itself, by one of two routes; tabs take
// expandTabs' advances.
//
// Character map route: a rune set in its run's primary face, which has
// plain advances (Face.plainAdvances: no kerning tables, no morx), takes
// its character map's glyph at that glyph's advance
// (CTFontGetGlyphsForCharacters, CTFontGetAdvancesForGlyphs), which is what
// the general path's CTLine gives. In a primary face with a GSUB or trak
// table (Face.needsProbe), whose default features (ccmp, locl) CoreText
// applies, only lines of ASCII take this route, and only in the (size,
// language) where a probe found CoreText setting the whole printable ASCII
// range as the character map's glyphs at their own advances (asciiProbe),
// and the layout tables prove the glyphs are independent of context.
//
// Exact route: every other rune (CJK and other fallback faces, non-ASCII
// runes of a GSUB or trak face, faces with GPOS) is set in the face the
// general path's segmentItem picks (fontmap.resolve, then langFace for the
// system CJK family). The face's OpenType tables must show that no lookup
// CoreText applies by default can make the rune depend on its neighbours
// or on its run's script (otlInfo.allows; AAT faces never qualify), and a
// probe measures what CoreText sets for the rune by itself: the rune three
// times over in one item of the run's face, size and language must give
// one glyph per rune, the same glyph and advance all three times, with no
// offset (so tracking is included). The measured glyph and advance are
// memoized per (face, size, language, rune); the probes of a line's new
// runes share one CTLine.

// fastPath memoizes the fast path's font resolution and glyph metrics; it
// lives as long as the font registry version (LineCache.flush).
type fastPath struct {
	fonts   map[fastFontKey]*fastFont
	tables  map[ctSizedFace]*monoTable
	nglyphs int  // memoized non-ASCII glyphs, all tables
	sgs     []sg // scratch

	exact  map[exactKey]map[rune]exactGlyph // the exact path's probed runes
	nexact int                              // memoized runes, all of exact
	otl    map[faceLang]*otlInfo
	probes int // CTLines the exact path's probes built (tests)
}

type exactKey struct {
	f    *Face
	size float32
	lang string
}

type faceLang struct {
	f    *Face
	lang string
}

// exactGlyph is what CoreText sets for a rune by itself; !ok: the rune
// takes the general path.
type exactGlyph struct {
	gid uint32
	adv float32
	ok  bool
}

type fastFontKey struct {
	w      Weight
	italic bool
}

// fastFont is the monospace font map of one (weight, slant) with its
// primary face and whether each rune resolves to it.
type fastFont struct {
	fm      *fontmap
	primary *Face               // nil: no fonts (approximate metrics)
	plain   bool                // primary has plain advances (Face.plainAdvances)
	faces   map[langRune1]*Face // faceFor
}

type langRune1 struct {
	lang string
	r    rune
}

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
	ff := &fastFont{fm: fm, faces: map[langRune1]*Face{}}
	if p := fm.primary(); p != nil {
		ff.primary, ff.plain = p, p.plainAdvances()
	}
	fp.fonts[k] = ff
	return ff
}

// faceFor is the face the general path sets r in for language lang
// (segmentItem): fontmap.resolve, then langFace for the system CJK family;
// nil when no face maps r.
func (ff *fastFont) faceFor(r rune, lang string) *Face {
	k := langRune1{lang, r}
	if f, ok := ff.faces[k]; ok {
		return f
	}
	f, fam := ff.fm.resolve(r)
	if fam != nil && !fam.user && fam.class == classCJK {
		f = langFace(ff.fm, f, r, lang)
	}
	if len(ff.faces) >= 8192 {
		clear(ff.faces)
	}
	ff.faces[k] = f
	return f
}

// otlOf is f's layout table analysis for lang (memoized).
func (fp *fastPath) otlOf(f *Face, lang string) *otlInfo {
	k := faceLang{f, lang}
	if o := fp.otl[k]; o != nil {
		return o
	}
	if fp.otl == nil || len(fp.otl) >= 256 {
		fp.otl = map[faceLang]*otlInfo{}
	}
	o := otlFor(f, lang)
	fp.otl[k] = o
	return o
}

// exactGlyph returns the probed glyph of r in face f at size in lang;
// false when r was not probed yet.
func (fp *fastPath) exactGlyph(f *Face, size float32, lang string, r rune) (exactGlyph, bool) {
	g, ok := fp.exact[exactKey{f, size, lang}][r]
	return g, ok
}

func (fp *fastPath) setExact(k exactKey, r rune, g exactGlyph) {
	if fp.exact == nil || fp.nexact >= 1<<15 {
		fp.exact = map[exactKey]map[rune]exactGlyph{}
		fp.nexact = 0
	}
	t := fp.exact[k]
	if t == nil {
		t = map[rune]exactGlyph{}
		fp.exact[k] = t
	}
	t[r] = g
	fp.nexact++
}

// exactLang reports whether the exact route may set lines of language
// lang. Not Japanese: CoreText's Japanese line layout spaces kana, Han and
// fullwidth forms from Latin letters and digits and compresses or widens
// CJK punctuation by its neighbours, across faces (JIS X 4051-style
// spacing), so no rune there is set by itself.
func exactLang(lang string) bool {
	p := strings.ToLower(lang)
	return p != "ja" && !strings.HasPrefix(p, "ja-") && !strings.HasPrefix(p, "ja_")
}

// exactRune is a rune of a line waiting for its probe, with its face.
type exactRune struct {
	r rune
	f *Face
}

// probe measures the runes of rs (distinct, not probed yet in their
// face at st.Size in lang) for the exact path: see the file comment.
func (fp *fastPath) probe(ff *fastFont, st Style, lang string, rs []exactRune) {
	var todo []exactRune
	for _, e := range rs {
		g0, ok := e.f.glyphFor(e.r)
		if !ok || !fp.otlOf(e.f, lang).allows(g0, otlScriptOf(e.r)) {
			fp.setExact(exactKey{e.f, st.Size, lang}, e.r, exactGlyph{})
			continue
		}
		todo = append(todo, e)
	}
	if len(todo) == 0 {
		return
	}
	if !fp.probeRunes(ff, st, lang, todo) {
		for _, e := range todo { // misaligned: one at a time
			fp.probeRunes(ff, st, lang, []exactRune{e})
		}
	}
}

// probeRunes shapes every rune of rs three times over in one item and
// records the result; false (nothing recorded) when the glyphs do not come
// one per rune.
func (fp *fastPath) probeRunes(ff *fastFont, st Style, lang string, rs []exactRune) bool {
	runes := make([]rune, 0, 3*len(rs))
	for _, e := range rs {
		runes = append(runes, e.r, e.r, e.r)
	}
	n := len(runes)
	adv := make([]float32, n+1)
	fp.probes++
	gl := shapeItems(runes, []item{{start: 0, end: n, st: st, lang: lang}}, []*fontmap{ff.fm}, adv)
	if len(gl) != n {
		if len(rs) == 1 {
			fp.setExact(exactKey{rs[0].f, st.Size, lang}, rs[0].r, exactGlyph{})
			return true
		}
		return false
	}
	for k, e := range rs {
		g := gl[3*k : 3*k+3]
		ok := true
		for j := range g {
			ok = ok && g[j].cluster == 3*k+j && g[j].face == e.f && g[j].gid == g[0].gid &&
				near5(g[j].adv, g[0].adv) && near5(adv[3*k+j], g[0].adv) && near5(g[j].xoff, 0) && near5(g[j].yoff, 0)
		}
		if ok {
			if g0, _ := e.f.glyphFor(e.r); g[0].gid != g0 {
				ok = fp.otlOf(e.f, lang).allows(g[0].gid, otlScriptOf(e.r))
			}
		}
		eg := exactGlyph{}
		if ok {
			eg = exactGlyph{gid: g[0].gid, adv: g[0].adv, ok: true}
		}
		fp.setExact(exactKey{e.f, st.Size, lang}, e.r, eg)
	}
	return true
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
	o := fp.otlOf(f, lang)
	for i := 0; ok && i < n; i++ {
		g := gl[i]
		mg, mapped := fp.glyph(f, st.Size, runes[i])
		ok = mapped && o.allows(mg.gid, otlScriptOf(runes[i])) &&
			g.face == f && g.cluster == i && g.gid == mg.gid &&
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
		if !r.st.Mono || r.st.FontFamily != "" {
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
	// A rune takes the character map route when it is set in its run's
	// primary face with plain advances, except in a face with GSUB or trak
	// (Face.needsProbe), where only ASCII lines in a probed (size,
	// language) do (asciiProbe); every other rune takes the exact route.
	probed := make([]int8, len(p.rs)) // asciiProbe per shaping run: 0 unknown, 1 ok, -1 not
	cmapRoute := func(sr int32, f *Face) bool {
		ff := ffs[sr]
		if f != ff.primary || !ff.plain {
			return false
		}
		if !f.needsProbe() {
			return true
		}
		if !ascii {
			return false
		}
		if probed[sr] == 0 {
			probed[sr] = -1
			if fp.asciiProbe(ff, p.rs[sr].st, p.rs[sr].lang) {
				probed[sr] = 1
			}
		}
		return probed[sr] > 0
	}
	shapeRun := func(i int) int32 {
		if p.shapeOf != nil {
			return p.shapeOf[i]
		}
		return 0
	}
	langOK := make([]int8, len(p.rs)) // exactLang per shaping run: 0 unknown, 1 yes, -1 no
	// One glyph per rune, in its shaping run (the first run shaping like
	// it, as the general path's items). The exact route's runes not probed
	// yet wait (todo), probed together per shaping run.
	gl := fp.sgs[:0]
	adv := make([]float32, n+1)
	var todo []int
	var pending map[int32][]exactRune
	var lastKey exactKey
	var last map[rune]exactGlyph
	for i, r := range runes {
		sr := shapeRun(i)
		ff := ffs[sr]
		g := sg{face: ff.primary, cluster: i, run: sr}
		if r != '\t' {
			st, lang := p.rs[sr].st, p.rs[sr].lang
			f := ff.faceFor(r, lang)
			if f == nil {
				return nil
			}
			if cmapRoute(sr, f) {
				mg, ok := fp.glyph(f, st.Size, r)
				if !ok {
					return nil
				}
				g.gid, g.adv = mg.gid, mg.adv
			} else {
				if langOK[sr] == 0 {
					langOK[sr] = -1
					if exactLang(lang) {
						langOK[sr] = 1
					}
				}
				if langOK[sr] < 0 {
					return nil
				}
				g.face = f
				if k := (exactKey{f, st.Size, lang}); k != lastKey || last == nil {
					lastKey, last = k, fp.exact[k]
				}
				if eg, ok := last[r]; ok {
					if !eg.ok {
						return nil
					}
					g.gid, g.adv = eg.gid, eg.adv
				} else {
					todo = append(todo, i)
					if pending == nil {
						pending = map[int32][]exactRune{}
					}
					if e := (exactRune{r, f}); !slices.Contains(pending[sr], e) {
						pending[sr] = append(pending[sr], e)
					}
				}
			}
		}
		gl = append(gl, g)
	}
	for sr, rs := range pending {
		fp.probe(ffs[sr], p.rs[sr].st, p.rs[sr].lang, rs)
	}
	for _, i := range todo {
		g := &gl[i]
		eg, ok := fp.exactGlyph(g.face, p.rs[g.run].st.Size, p.rs[g.run].lang, runes[i])
		if !ok || !eg.ok {
			fp.sgs = gl[:0]
			return nil
		}
		g.gid, g.adv = eg.gid, eg.adv
	}
	for i := range gl {
		adv[i] = gl[i].adv
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
