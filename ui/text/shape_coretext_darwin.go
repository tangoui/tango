//go:build darwin && !ios && !tango_gotext

package text

import (
	"slices"
	"strings"
	"sync"
	"unicode"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Shaping with CoreText. Each item (one embedding level, one style) becomes
// one attributed string: the runes are given faces with fontmap.resolve,
// adjacent runes of one face form a segment carrying that face's CTFont
// (kCTFontAttributeName), and the whole string carries the item's language
// (kCTLanguageAttributeName), its direction as an explicit base writing
// direction (never Natural: the item is a single level already) and, with
// Options.noLiga, kCTLigatureAttributeName 0. The CTLine of the string is
// read run by run: the font of each run decides its glyphs' face (CoreText
// may still substitute fonts on its own), and glyphs, positions, advances
// and string indices give the sg values.
//
// Clusters follow the HarfBuzz convention the shared pipeline expects: a
// glyph's cluster is the first rune of its cluster, so combining marks,
// emoji sequences (ZWJ, modifiers, variation selectors, tags, flag pairs)
// and runes a glyph reordered across all take the first rune's index, and
// the advances of a cluster add up in adv at that rune.

// CoreText and CoreFoundation symbols only the shaper uses.
var (
	ctShapeOnce sync.Once
	ctShapeOK   bool

	cfAttributedStringCreateMutable func(alloc uintptr, max int64) uintptr
	cfAttributedStringReplaceString func(as uintptr, r cfRange, s uintptr)
	cfAttributedStringSetAttribute  func(as uintptr, r cfRange, name, value uintptr)
	cfAttributedStringBeginEditing  func(uintptr)
	cfAttributedStringEndEditing    func(uintptr)

	kCTLigatureAttributeName uintptr
	kCTLanguageAttributeName uintptr // 0 when the OS lacks it

	cfArrayCreate             func(alloc uintptr, vals *uintptr, n int64, cb uintptr) uintptr
	ctFontCopyFeatureSettings func(uintptr) uintptr
	kCFTypeArrayCallBacks     uintptr

	kCTFontFeatureSettingsAttribute uintptr
	kCTFontOpenTypeFeatureTag       uintptr
	kCTFontOpenTypeFeatureValue     uintptr

	ctLineGetTrailingWhitespaceWidth func(line uintptr) float64

	// CTFontCreateForStringWithLanguage (macOS 10.15); nil when missing.
	ctFontCreateForStringWithLanguage func(f, s uintptr, r cfRange, lang uintptr) uintptr
)

func loadCTShape() bool {
	ctShapeOnce.Do(func() {
		if !ctLoaded() {
			return
		}
		defer func() {
			if recover() != nil { // a missing symbol panics in RegisterLibFunc
				ctShapeOK = false
			}
		}()
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			return
		}
		ct, err := purego.Dlopen("/System/Library/Frameworks/CoreText.framework/CoreText", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&cfAttributedStringCreateMutable, cf, "CFAttributedStringCreateMutable")
		purego.RegisterLibFunc(&cfAttributedStringReplaceString, cf, "CFAttributedStringReplaceString")
		purego.RegisterLibFunc(&cfAttributedStringSetAttribute, cf, "CFAttributedStringSetAttribute")
		purego.RegisterLibFunc(&cfAttributedStringBeginEditing, cf, "CFAttributedStringBeginEditing")
		purego.RegisterLibFunc(&cfAttributedStringEndEditing, cf, "CFAttributedStringEndEditing")
		purego.RegisterLibFunc(&cfArrayCreate, cf, "CFArrayCreate")
		purego.RegisterLibFunc(&ctFontCopyFeatureSettings, ct, "CTFontCopyFeatureSettings")
		purego.RegisterLibFunc(&ctLineGetTrailingWhitespaceWidth, ct, "CTLineGetTrailingWhitespaceWidth")
		if p, err := purego.Dlsym(cf, "kCFTypeArrayCallBacks"); err == nil {
			kCFTypeArrayCallBacks = p // the structure's address
		}
		val := func(name string) uintptr {
			p, err := purego.Dlsym(ct, name)
			if err != nil {
				return 0
			}
			return **(**uintptr)(unsafe.Pointer(&p)) // the variable's value
		}
		if _, err := purego.Dlsym(ct, "CTFontCreateForStringWithLanguage"); err == nil {
			purego.RegisterLibFunc(&ctFontCreateForStringWithLanguage, ct, "CTFontCreateForStringWithLanguage")
		}
		kCTLigatureAttributeName = val("kCTLigatureAttributeName")
		kCTLanguageAttributeName = val("kCTLanguageAttributeName")
		kCTFontFeatureSettingsAttribute = val("kCTFontFeatureSettingsAttribute")
		kCTFontOpenTypeFeatureTag = val("kCTFontOpenTypeFeatureTag")
		kCTFontOpenTypeFeatureValue = val("kCTFontOpenTypeFeatureValue")
		ctShapeOK = kCTLigatureAttributeName != 0 && kCFTypeArrayCallBacks != 0 &&
			kCTFontFeatureSettingsAttribute != 0 && kCTFontOpenTypeFeatureTag != 0 && kCTFontOpenTypeFeatureValue != 0
	})
	return ctShapeOK
}

// resetShaper drops the shaping caches (clearPrepared).
func resetShaper() {
	clearNumericFaces()
	langFaces.mu.Lock()
	langFaces.m = nil
	langFaces.mu.Unlock()
	clearNoLigaFonts()
}

// fontmapImpl is the engine part of fontmap: CoreText needs no reverse
// mapping (a run's CTFont maps to its Face through faceForCTFont).
type fontmapImpl struct{}

// rememberFace records f as the face resolve picked.
func (m *fontmap) rememberFace(f *Face) {}

// coreTextShape is the opt-in short-text path of the go-text engine; the
// CoreText engine shapes everything through shapeItems.
func coreTextShape(s string, st Style, opt Options) *Layout { return nil }

// shapeItems shapes each item (one embedding level, one style) with the
// font map of its style run (fms[it.run]) and returns the glyphs plus
// per-rune advances (added into adv). Glyphs of a right-to-left item come
// in visual order (CoreText's run order); the clusters are placed by the
// caller in line visual order. Without any font (every chain is empty, so
// fms[0] tells) advances are approximated from each item's size.
func shapeItems(runes []rune, items []item, fms []*fontmap, adv []float32) []sg {
	if fms[0].primary() == nil || !loadCTShape() {
		approxItems(runes, items, adv)
		return nil
	}
	var sh ctShaper
	defer sh.release()
	var out []sg
	for _, it := range items {
		if it.start >= it.end {
			continue
		}
		out = sh.item(out, runes, it, fms[it.run], adv)
	}
	return out
}

func approxItems(runes []rune, items []item, adv []float32) {
	for _, it := range items {
		for i := it.start; i < it.end; i++ {
			adv[i] = approxAdvance(runes[i], it.st.Size)
		}
	}
}

// ctShaper holds what one shapeItems call shares between its items: the
// retained sized CTFonts (released by release) and the faces of the fonts
// seen in runs.
type ctShaper struct {
	fonts  []ctSized
	byFont map[uintptr]*Face // run font (retained) -> face (fonts CoreText picked)
	keys   map[*Face]string  // ctFaceKey of the faces set on segments

	glyphs []uint16
	pos    []cgPoint
	advs   []cgSize
	idx    []int64
	start  []int32 // cluster start of every rune of the item (item-relative)
}

type ctSized struct {
	face   *Face
	size   float32
	noLiga bool
	font   uintptr // retained
}

func (sh *ctShaper) release() {
	for _, s := range sh.fonts {
		cfRelease(s.font)
	}
	for c := range sh.byFont {
		cfRelease(c)
	}
	sh.fonts, sh.byFont = nil, nil
}

// font returns the retained CTFont of face at size, with liga, clig and
// calt off when noLiga (noLigaFont); 0 when the face cannot be built. It
// stays valid until release.
func (sh *ctShaper) font(f *Face, size float32, noLiga bool) uintptr {
	for _, s := range sh.fonts {
		if s.face == f && s.size == size && s.noLiga == noLiga {
			return s.font
		}
	}
	var c uintptr
	if noLiga {
		c = noLigaFont(f, size)
	} else {
		c = f.ctFont(size)
	}
	if c != 0 {
		sh.fonts = append(sh.fonts, ctSized{f, size, noLiga, c})
	}
	return c
}

// Options.noLiga turns off the OpenType features liga, clig and calt, as
// the go-text engine does. kCTLigatureAttributeName alone leaves calt
// (contextual alternates, which code fonts use for their ligatures) on, so
// the font itself is copied with kCTFontFeatureSettingsAttribute setting
// the three to 0. The copies are kept per face and size (at most 64; all
// dropped by clearPrepared).
var noLigaFonts struct {
	mu sync.Mutex
	m  map[ctSizedFace]uintptr // owned
}

type ctSizedFace struct {
	f    *Face
	size float32
}

func clearNoLigaFonts() {
	noLigaFonts.mu.Lock()
	defer noLigaFonts.mu.Unlock()
	for _, c := range noLigaFonts.m {
		cfRelease(c)
	}
	noLigaFonts.m = nil
}

// noLigaFont returns face f's CTFont at size with liga, clig and calt off,
// retained for the caller; 0 when the face cannot be built.
func noLigaFont(f *Face, size float32) uintptr {
	k := ctSizedFace{f, size}
	noLigaFonts.mu.Lock()
	defer noLigaFonts.mu.Unlock()
	if c, ok := noLigaFonts.m[k]; ok {
		cfRetain(c)
		return c
	}
	base := f.ctFont(size)
	if base == 0 {
		return 0
	}
	defer cfRelease(base)
	desc := noLigaDesc()
	if desc == 0 {
		cfRetain(base)
		return base
	}
	defer cfRelease(desc)
	c := ctFontCreateCopyWithAttributes(base, 0, nil, desc)
	if c == 0 {
		cfRetain(base)
		return base
	}
	if noLigaFonts.m == nil || len(noLigaFonts.m) >= 64 {
		for _, o := range noLigaFonts.m {
			cfRelease(o)
		}
		noLigaFonts.m = map[ctSizedFace]uintptr{}
	}
	noLigaFonts.m[k] = c
	cfRetain(c)
	return c
}

// noLigaDesc is a font descriptor setting the OpenType features liga,
// clig and calt to 0; 0 on failure. The caller releases it.
func noLigaDesc() uintptr {
	var zero int32
	off := cfNumberCreate(0, cfNumberSInt32Type, unsafe.Pointer(&zero))
	if off == 0 {
		return 0
	}
	defer cfRelease(off)
	var feats []uintptr
	defer func() {
		for _, d := range feats {
			cfRelease(d)
		}
	}()
	for _, tag := range []string{"liga", "clig", "calt"} {
		t := cfStr(tag)
		if t == 0 {
			return 0
		}
		d := cfDict([]uintptr{kCTFontOpenTypeFeatureTag, kCTFontOpenTypeFeatureValue}, []uintptr{t, off})
		cfRelease(t)
		if d == 0 {
			return 0
		}
		feats = append(feats, d)
	}
	arr := cfArrayCreate(0, &feats[0], int64(len(feats)), kCFTypeArrayCallBacks)
	if arr == 0 {
		return 0
	}
	defer cfRelease(arr)
	attrs := cfDict([]uintptr{kCTFontFeatureSettingsAttribute}, []uintptr{arr})
	if attrs == 0 {
		return 0
	}
	defer cfRelease(attrs)
	return ctFontDescriptorCreateWithAttributes(attrs)
}

// faceOf maps the font c of a run (in an item whose segments use the faces
// of sh.fonts) to its Face: a segment's own face when c is its font or the
// same font at another size or instance (same ctFaceKey), else
// faceForCTFont.
func (sh *ctShaper) faceOf(c uintptr) *Face {
	for _, s := range sh.fonts {
		if s.font == c {
			return s.face
		}
	}
	if f, ok := sh.byFont[c]; ok {
		return f
	}
	key := ctFaceKey(c)
	var face *Face
	for _, s := range sh.fonts {
		if sh.keys == nil {
			sh.keys = map[*Face]string{}
		}
		k, ok := sh.keys[s.face]
		if !ok {
			k = ctFaceKey(s.font)
			sh.keys[s.face] = k
		}
		if k == key {
			face = s.face
			break
		}
	}
	if face == nil {
		face = faceForCTFont(c)
	}
	if sh.byFont == nil {
		sh.byFont = map[uintptr]*Face{}
	}
	// Retained: the run (and its line) may go before the call ends, and
	// another font could then reuse the address.
	cfRetain(c)
	sh.byFont[c] = face
	return face
}

// ctSegment is a stretch of an item set in one face: runes [a,b), UTF-16
// offsets [ua,ub) in the item's string.
type ctSegment struct {
	face   *Face
	a, b   int
	ua, ub int64
	font   uintptr
}

// item shapes one item and appends its glyphs to out.
func (sh *ctShaper) item(out []sg, runes []rune, it item, fm *fontmap, adv []float32) []sg {
	a, b := it.start, it.end
	segs := segmentItem(runes, a, b, fm, it.lang)
	if segs == nil {
		approxItems(runes, []item{it}, adv)
		return out
	}
	for i := range segs {
		segs[i].font = sh.font(segs[i].face, it.st.Size, fm.noLiga)
	}
	toRune := utf16Index(runes, a, b)
	line := ctItemLine(runes, a, b, it, fm.noLiga, segs)
	if line == 0 {
		approxItems(runes, []item{it}, adv)
		return out
	}
	defer cfRelease(line)
	sh.clusterStarts(runes, a, b)
	first := len(out)
	// CoreText's pen: sum of the advances of the line so far. A
	// right-to-left line hangs its trailing whitespace (visually first)
	// left of the origin: every position is moved left by its width, which
	// the pen starts at so that offsets stay relative to the glyph's own
	// place (the item's glyphs are placed from 0 by the caller).
	pen := 0.0
	if it.rtl() {
		pen = -ctLineGetTrailingWhitespaceWidth(line)
	}
	arr := ctLineGetGlyphRuns(line) // owned by line
	last := int64(len(toRune) - 1)
	for k, nr := int64(0), cfArrayGetCount(arr); k < nr; k++ {
		run := cfArrayGetValueAtIndex(arr, k)
		n := ctRunGetGlyphCount(run)
		if n <= 0 {
			continue
		}
		rtl := ctRunGetStatus(run)&ctRunStatusRightToLeft != 0
		runFont := cfDictionaryGetValue(ctRunGetAttributes(run), kCTFontAttributeName)
		face := sh.faceOf(runFont)
		if face == nil || (fm.eng == nil && !segFace(segs, face)) {
			// System fallback is off (UseSystemFonts(false)) or the font
			// is unusable: the segment's own face draws the runes,
			// with .notdef where it lacks them.
			out, pen = sh.ownGlyphs(out, runes, toRune, run, rtl, segs, it, pen)
			continue
		}
		sh.glyphs = grow(sh.glyphs, n)
		sh.pos = grow(sh.pos, n)
		sh.advs = grow(sh.advs, n)
		sh.idx = grow(sh.idx, n)
		r := cfRange{0, n}
		ctRunGetGlyphs(run, r, &sh.glyphs[0])
		ctRunGetPositions(run, r, &sh.pos[0])
		ctRunGetAdvances(run, r, &sh.advs[0])
		ctRunGetStringIndices(run, r, &sh.idx[0])
		sh.mergeReordered(toRune, a, n, rtl, last)
		for g := int64(0); g < n; g++ {
			u := sh.idx[g]
			if u < 0 || u >= last {
				pen += sh.advs[g].W
				continue
			}
			ri := int(toRune[u])
			c := a + int(sh.start[ri-a])
			out = append(out, sg{
				face:    face,
				gid:     uint32(sh.glyphs[g]),
				adv:     float32(sh.advs[g].W),
				xoff:    float32(sh.pos[g].X - pen),
				yoff:    float32(sh.pos[g].Y), // CoreText y grows upwards, as yoff does
				cluster: c,
				run:     it.run,
			})
			pen += sh.advs[g].W
		}
	}
	for _, gl := range out[first:] {
		adv[gl.cluster] += gl.adv
	}
	return out
}

// segFace reports whether f is the face of one of segs.
func segFace(segs []ctSegment, f *Face) bool {
	for _, s := range segs {
		if s.face == f {
			return true
		}
	}
	return false
}

// ownGlyphs appends, for the runes of run (in visual order: reversed when
// rtl), the glyph their segment's face maps them to (0, .notdef, when it
// lacks them) with that face's advance: one glyph per rune, at the rune's
// cluster start. It returns out and pen moved by CoreText's own advances of
// the run, which the positions of later runs include.
func (sh *ctShaper) ownGlyphs(out []sg, runes []rune, toRune []int32, run uintptr, rtl bool, segs []ctSegment, it item, pen float64) ([]sg, float64) {
	if n := ctRunGetGlyphCount(run); n > 0 {
		sh.advs = grow(sh.advs, n)
		ctRunGetAdvances(run, cfRange{0, n}, &sh.advs[0])
		for _, a := range sh.advs[:n] {
			pen += a.W
		}
	}
	sr := ctRunGetStringRange(run)
	last := int64(len(toRune) - 1)
	lo, hi := sr.Loc, sr.Loc+sr.Len
	if lo < 0 || hi > last || lo >= hi {
		return out, pen
	}
	ra, rb := int(toRune[lo]), int(toRune[hi-1])+1
	emit := func(i int) {
		var seg *ctSegment
		for k := range segs {
			if i >= segs[k].a && i < segs[k].b {
				seg = &segs[k]
			}
		}
		if seg == nil || seg.font == 0 {
			return
		}
		id, _ := seg.face.glyphFor(runes[i])
		g := uint16(id)
		var sz cgSize
		ctFontGetAdvancesForGlyphs(seg.font, ctFontOrientationHorz, &g, &sz, 1)
		out = append(out, sg{face: seg.face, gid: id, adv: float32(sz.W), cluster: it.start + int(sh.start[i-it.start]), run: it.run})
	}
	if rtl {
		for i := rb - 1; i >= ra; i-- {
			emit(i)
		}
	} else {
		for i := ra; i < rb; i++ {
			emit(i)
		}
	}
	return out, pen
}

func grow[T any](s []T, n int64) []T {
	if int64(cap(s)) < n {
		return make([]T, n)
	}
	return s[:n]
}

// segmentItem cuts runes[a:b] into segments of one face (fm.resolve per
// rune, then langFace). A rune no face maps joins the segment before it
// (the first such runes the segment after them); nil when no rune resolves
// and fm has no face at all.
func segmentItem(runes []rune, a, b int, fm *fontmap, lang string) []ctSegment {
	var segs []ctSegment
	var u int64
	for i := a; i < b; i++ {
		f := fm.resolveCTFace(runes[i], lang)
		n := int64(utf16Len(runes[i]))
		switch k := len(segs) - 1; {
		case f == nil && k >= 0, k >= 0 && segs[k].face == f:
			segs[k].b, segs[k].ub = i+1, u+n
		case f == nil:
			segs = append(segs, ctSegment{a: i, b: i + 1, ua: u, ub: u + n}) // face filled below
		case k >= 0 && segs[k].face == nil:
			segs[k].face, segs[k].b, segs[k].ub = f, i+1, u+n
		default:
			segs = append(segs, ctSegment{face: f, a: i, b: i + 1, ua: u, ub: u + n})
		}
		u += n
	}
	if len(segs) == 1 && segs[0].face == nil {
		if segs[0].face = fm.primary(); segs[0].face == nil {
			return nil
		}
	}
	return segs
}

// resolveCTFace picks the face mapping r, including the language-specific
// variant of a system CJK fallback. Preferred families keep their face.
func (fm *fontmap) resolveCTFace(r rune, lang string) *Face {
	f, fam := fm.resolve(r)
	if fam != nil && !fam.user && fam.class == classCJK && !slices.Contains(fm.chain[:fm.preferred], fam) {
		return langFace(fm, f, r, lang)
	}
	return f
}

// The CJK family of the system chain (PingFang on macOS) is a Chinese
// font: CoreText applies its locl forms for Traditional Chinese, but not
// Japanese or Korean ones, which the system draws from other fonts
// (Hiragino, Apple SD Gothic Neo). For text in those languages langFace
// asks CoreText for the font it would use after the system UI font of the
// font map's weight (CTFontCreateForStringWithLanguage) and takes its face
// (faceForCTFont); the answers are cached per language, weight and rune
// until clearPrepared.
var langFaces struct {
	mu sync.Mutex
	m  map[langRune]*Face
}

type langRune struct {
	lang string
	w    Weight
	r    rune
}

// langFace returns the face rune r, which the system CJK face f maps, is
// drawn with in language lang: f itself except for Japanese and Korean.
func langFace(fm *fontmap, f *Face, r rune, lang string) *Face {
	if f == nil || fm.eng == nil || ctFontCreateForStringWithLanguage == nil {
		return f
	}
	p := strings.ToLower(lang)
	if i := strings.IndexAny(p, "-_"); i >= 0 {
		p = p[:i]
	}
	if p != "ja" && p != "ko" {
		return f
	}
	k := langRune{p, fm.w, r}
	langFaces.mu.Lock()
	lf, ok := langFaces.m[k]
	langFaces.mu.Unlock()
	if !ok {
		lf = ctLangFace(r, p, fm.w)
		langFaces.mu.Lock()
		if langFaces.m == nil || len(langFaces.m) > 4096 {
			langFaces.m = map[langRune]*Face{}
		}
		langFaces.m[k] = lf
		langFaces.mu.Unlock()
	}
	if lf == nil || !lf.HasRune(r) {
		return f
	}
	return lf
}

// ctLangFace is the face of the font CoreText picks for r in language lang
// after the system UI font at weight w; nil when it cannot tell.
func ctLangFace(r rune, lang string, w Weight) *Face {
	base := uiFontWeighted(w, false, 17)
	if base == 0 {
		return nil
	}
	defer cfRelease(base)
	s := cfStringRunes([]rune{r}, 0, 1)
	if s == 0 {
		return nil
	}
	defer cfRelease(s)
	l := cfStr(lang)
	if l == 0 {
		return nil
	}
	defer cfRelease(l)
	c := ctFontCreateForStringWithLanguage(base, s, cfRange{0, cfStringGetLength(s)}, l)
	if c == 0 {
		return nil
	}
	defer cfRelease(c)
	n := ctFontCopyFamilyName(c)
	name := goStr(n)
	if n != 0 {
		cfRelease(n)
	}
	if strings.Contains(name, "LastResort") {
		return nil
	}
	return faceForCTFont(c)
}

// ctItemLine builds the CTLine of item it (runes[a:b], set in segs); 0 on
// failure. The caller releases the line.
func ctItemLine(runes []rune, a, b int, it item, noLiga bool, segs []ctSegment) uintptr {
	s := cfStringRunes(runes, a, b)
	if s == 0 {
		return 0
	}
	defer cfRelease(s)
	as := cfAttributedStringCreateMutable(0, 0)
	if as == 0 {
		return 0
	}
	defer cfRelease(as)
	cfAttributedStringReplaceString(as, cfRange{0, 0}, s)
	all := cfRange{0, cfStringGetLength(s)}
	cfAttributedStringBeginEditing(as)
	wd := int8(ctWritingDirectionLeftToRight)
	if it.rtl() {
		wd = ctWritingDirectionRightToLeft
	}
	set := ctParagraphStyleSetting{Spec: ctParagraphStyleSpecifierBaseDirection, Size: 1, Value: unsafe.Pointer(&wd)}
	if ps := ctParagraphStyleCreate(&set, 1); ps != 0 {
		cfAttributedStringSetAttribute(as, all, kCTParagraphStyleAttributeName, ps)
		cfRelease(ps)
	}
	if it.lang != "" && kCTLanguageAttributeName != 0 {
		// Style.Lang is caller input: invalid UTF-8 gives no string.
		if l := cfStr(it.lang); l != 0 {
			cfAttributedStringSetAttribute(as, all, kCTLanguageAttributeName, l)
			cfRelease(l)
		}
	}
	if noLiga {
		var zero int32
		if n := cfNumberCreate(0, cfNumberSInt32Type, unsafe.Pointer(&zero)); n != 0 {
			cfAttributedStringSetAttribute(as, all, kCTLigatureAttributeName, n)
			cfRelease(n)
		}
	}
	for _, sg := range segs {
		if sg.font != 0 {
			cfAttributedStringSetAttribute(as, cfRange{sg.ua, sg.ub - sg.ua}, kCTFontAttributeName, sg.font)
		}
	}
	cfAttributedStringEndEditing(as)
	return ctLineCreateWithAttributedString(as)
}

// clusterStarts fills sh.start[i-a] with the first rune of the grapheme
// cluster of rune i, for the runes of [a,b): a combining mark, ZWJ,
// variation selector, emoji modifier or tag joins the rune before it, as
// does the rune after a ZWJ and the second regional indicator of a flag.
func (sh *ctShaper) clusterStarts(runes []rune, a, b int) {
	sh.start = grow(sh.start, int64(b-a))
	ri := 0 // regional indicators in a row
	for i := a; i < b; i++ {
		r := runes[i]
		s := int32(i - a)
		if i > a {
			join := clusterExtend(r) || runes[i-1] == 0x200D
			if r >= 0x1F1E6 && r <= 0x1F1FF {
				join = join || ri%2 == 1
			}
			if join {
				s = sh.start[i-a-1]
			}
		}
		if r >= 0x1F1E6 && r <= 0x1F1FF {
			ri++
		} else {
			ri = 0
		}
		sh.start[i-a] = s
	}
}

// clusterExtend reports a rune that extends the cluster before it.
func clusterExtend(r rune) bool {
	switch {
	case r == 0x200D, r == 0x200C,
		r >= 0xFE00 && r <= 0xFE0F, r >= 0xE0100 && r <= 0xE01EF, // variation selectors
		r >= 0x1F3FB && r <= 0x1F3FF, // emoji modifiers
		r >= 0xE0020 && r <= 0xE007F: // tags
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc)
}

// mergeReordered merges into one cluster the runes a glyph of the current
// run (sh.idx[:n], visual order, reversed in logical order when rtl) was
// moved across: walking the glyphs in logical order, a glyph of a rune
// before the furthest rune seen joins every rune up to it.
func (sh *ctShaper) mergeReordered(toRune []int32, a int, n int64, rtl bool, last int64) {
	far := -1
	for k := int64(0); k < n; k++ {
		g := k
		if rtl {
			g = n - 1 - k
		}
		u := sh.idx[g]
		if u < 0 || u >= last {
			continue
		}
		i := int(toRune[u]) - a
		if i >= far {
			far = i
			continue
		}
		s := sh.start[i]
		top := sh.start[far]
		for j := int(s); j < len(sh.start) && (j <= far || sh.start[j] == top); j++ {
			sh.start[j] = s
		}
	}
}
