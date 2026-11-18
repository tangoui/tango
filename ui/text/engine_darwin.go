//go:build darwin

package text

import (
	"image"
	"math"
	"runtime"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/tangoui/tango/ui/paint"
)

// CoreText engine (macOS), written against Apple's CoreText,
// CoreGraphics and CoreFoundation reference documentation and called
// through purego (no cgo).
//
// Fonts are matched by family name with CTFontDescriptor
// (kCTFontFamilyNameAttribute → CTFontDescriptorCreateMatchingFontDescriptors),
// so CoreText parses the font files: PingFang's .ttc, whose glyph data
// go-text cannot read, works. The system UI font (SF Pro) comes from
// CTFontCreateUIFontForLanguage with kCTFontWeightTrait, and per-rune
// fallback from CTFontCreateForString (".SF Arabic" and other private
// families are kept as the CTFont CoreText returned).
//
// Shaping stays on go-text rather than CTLine: each face's tables
// (CTFontCopyTable: cmap, hmtx, GSUB, GPOS, morx, fvar...; not the glyph
// data) are exposed through a small sfnt directory, so go-text picks the same glyph
// ids with the font's own advances, and CTFontDrawGlyphs draws exactly
// those glyphs. Line breaking, kinsoku, carets and the per-rune fallback
// chain stay one code path on every platform. Variable faces get the axis
// values CoreText reports (CTFontCopyVariation) and a pinned optical size
// (kCTFontOpticalSizeAttribute), so every point size draws the instance
// go-text measured; CoreText's own advances then differ only by the font's
// uniform tracking (trak), which our layout does not apply.
//
// Glyphs are drawn into an 8-bit DeviceGray CGBitmapContext with
// antialiasing, sub-pixel positioning (subx is part of the glyph position)
// and font smoothing (CoreText's stem darkening for dark text on a light
// background: black on white, then inverted); skew (synthetic italic) is
// the font matrix. Colour emoji (Apple Color Emoji, sbix) are drawn white
// on black, so the mask is their luminance: a gray-level emoji in the text
// colour, not colour.

type ctEngine struct {
	sysUI uintptr // system UI font (CTFontCreateUIFontForLanguage), fallback base

	mu     sync.Mutex
	hidden map[string]uintptr // "."-named fallback fonts (CTFontCreateForString), not matchable by name
}

func init() { openEngine = openCoreText }

func openCoreText() sysEngine {
	ctOnce.Do(func() { ctOK = loadCoreText() })
	if !ctOK {
		return nil
	}
	ui := ctFontCreateUIFontForLanguage(ctFontUIFontSystem, 16, 0)
	if ui == 0 {
		return nil
	}
	return &ctEngine{sysUI: ui, hidden: map[string]uintptr{}}
}

func (e *ctEngine) name() string { return "coretext" }

// The system UI font (SF Pro on current macOS) is listed under this name.
const ctSystemUI = "SF Pro"

func (e *ctEngine) defaults() (sans, mono []famSpec) {
	sans = []famSpec{
		{name: ctSystemUI},
		{name: "PingFang SC", class: classCJK},
		{name: "Hiragino Sans GB", class: classCJK},
		{name: "Apple SD Gothic Neo", class: classCJK},
		{name: "Apple Color Emoji", class: classEmoji},
		{name: "Apple Symbols"},
	}
	mono = []famSpec{{name: "SF Mono"}, {name: "Menlo"}}
	return
}

func (e *ctEngine) fallback(r rune) string {
	u := utf16.Encode([]rune{r})
	s := cfStr(string(r))
	defer cfRelease(s)
	f := ctFontCreateForString(e.sysUI, s, cfRange{0, int64(len(u))})
	if f == 0 {
		return ""
	}
	defer cfRelease(f)
	n := ctFontCopyFamilyName(f)
	defer cfRelease(n)
	name := goStr(n)
	if name == "" || strings.Contains(name, "LastResort") || name == ".AppleSystemUIFont" {
		return ""
	}
	if strings.HasPrefix(name, ".") {
		// System-private families (.SF Arabic, .ThonburiUI) cannot be
		// matched by name: keep the font CoreText chose.
		e.mu.Lock()
		if _, ok := e.hidden[name]; !ok {
			e.hidden[name] = ctFontCreateCopyWithAttributes(f, 16, nil, 0)
		}
		e.mu.Unlock()
	}
	return name
}

// fonts returns retained CTFonts for every installed face of family name.
func (e *ctEngine) fonts(name string) []uintptr {
	if name == "SF Mono" {
		return fontsFromFiles("/System/Library/Fonts/SFNSMono.ttf", "/System/Library/Fonts/SFNSMonoItalic.ttf")
	}
	e.mu.Lock()
	h, ok := e.hidden[name]
	e.mu.Unlock()
	if ok {
		if h == 0 {
			return nil
		}
		return []uintptr{ctFontCreateCopyWithAttributes(h, 16, nil, 0)}
	}
	k := []uintptr{kCTFontFamilyNameAttribute}
	v := []uintptr{cfStr(name)}
	attrs := cfDict(k, v)
	cfRelease(v[0])
	desc := ctFontDescriptorCreateWithAttributes(attrs)
	cfRelease(attrs)
	if desc == 0 {
		return nil
	}
	defer cfRelease(desc)
	arr := ctFontDescriptorCreateMatchingFontDescriptors(desc, 0)
	if arr == 0 {
		return nil
	}
	defer cfRelease(arr)
	var out []uintptr
	for i := int64(0); i < cfArrayGetCount(arr); i++ {
		f := ctFontCreateWithFontDescriptor(cfArrayGetValueAtIndex(arr, i), 16, nil)
		if f == 0 {
			continue
		}
		fn := ctFontCopyFamilyName(f)
		got := goStr(fn)
		cfRelease(fn)
		if !strings.EqualFold(got, name) {
			cfRelease(f)
			continue
		}
		out = append(out, f)
	}
	return out
}

// uiFontWeighted returns the system UI font at CSS weight w, italic or
// not, through the documented traits route (kCTFontWeightTrait), with the
// optical size pinned to opsz.
func uiFontWeighted(w Weight, italic bool, opsz float64) uintptr {
	up := ctFontCreateUIFontForLanguage(ctFontUIFontSystem, 16, 0)
	if up == 0 {
		return 0
	}
	defer cfRelease(up)
	tw := ctWeightTrait(w)
	num := cfNumberCreate(0, cfNumberFloat64Type, unsafe.Pointer(&tw))
	traits := cfDict([]uintptr{kCTFontWeightTrait}, []uintptr{num})
	cfRelease(num)
	ak, av := []uintptr{kCTFontTraitsAttribute}, []uintptr{traits}
	var on uintptr
	if kCTFontOpticalSizeAttribute != 0 {
		on = cfNumberCreate(0, cfNumberFloat64Type, unsafe.Pointer(&opsz))
		ak, av = append(ak, kCTFontOpticalSizeAttribute), append(av, on)
	}
	attrs := cfDict(ak, av)
	cfRelease(traits)
	if on != 0 {
		cfRelease(on)
	}
	desc := ctFontDescriptorCreateWithAttributes(attrs)
	cfRelease(attrs)
	defer cfRelease(desc)
	f := ctFontCreateCopyWithAttributes(up, 0, nil, desc)
	if f != 0 && italic {
		it := ctFontCreateCopyWithSymbolicTraits(f, 0, nil, ctFontItalicTrait, ctFontItalicTrait)
		cfRelease(f)
		f = it
	}
	return f
}

// fontsFromFiles returns the faces of font files (CoreText parses them).
func fontsFromFiles(paths ...string) []uintptr {
	var out []uintptr
	for _, p := range paths {
		b := []byte(p)
		url := cfURLCreateFromFileSystemRepresentation(0, &b[0], int64(len(b)), false)
		if url == 0 {
			continue
		}
		arr := ctFontManagerCreateFontDescriptorsFromURL(url)
		cfRelease(url)
		if arr == 0 {
			continue
		}
		for i := int64(0); i < cfArrayGetCount(arr); i++ {
			if f := ctFontCreateWithFontDescriptor(cfArrayGetValueAtIndex(arr, i), 16, nil); f != 0 {
				out = append(out, f)
			}
		}
		cfRelease(arr)
	}
	return out
}

func ctWeight(f uintptr) Weight {
	tr := ctFontCopyTraits(f)
	if tr == 0 {
		return Regular
	}
	defer cfRelease(tr)
	w, _ := cfFloat(cfDictionaryGetValue(tr, kCTFontWeightTrait))
	return cssWeightFromCT(w)
}

// ctVariation returns the axis values of a variable CTFont (tag → value).
func ctVariation(f uintptr) map[uint32]float32 {
	d := ctFontCopyVariation(f)
	if d == 0 {
		return nil
	}
	defer cfRelease(d)
	n := cfDictionaryGetCount(d)
	if n == 0 {
		return nil
	}
	keys := make([]uintptr, n)
	vals := make([]uintptr, n)
	cfDictionaryGetKeysValues(d, &keys[0], &vals[0])
	out := map[uint32]float32{}
	for i := range keys {
		var tag int64
		if !cfNumberGetValue(keys[i], 4 /* SInt64 */, unsafe.Pointer(&tag)) {
			continue
		}
		if v, ok := cfFloat(vals[i]); ok {
			out[uint32(tag)] = float32(v)
		}
	}
	return out
}

// withVariation returns a copy of f set to vals (tag → value); 0 on
// failure.
func withVariation(f uintptr, vals map[uint32]float32) uintptr {
	desc := variationDesc(vals)
	defer cfRelease(desc)
	return ctFontCreateCopyWithAttributes(f, 0, nil, desc)
}

// variationDesc is a font descriptor fixing the variation to vals, with
// automatic optical sizing off (CoreText would otherwise move opsz with the
// point size of every sized copy).
func variationDesc(vals map[uint32]float32) uintptr {
	var ks, vs []uintptr
	for tag, v := range vals {
		t := int64(tag)
		fv := float64(v)
		ks = append(ks, cfNumberCreate(0, 4, unsafe.Pointer(&t)))
		vs = append(vs, cfNumberCreate(0, cfNumberFloat64Type, unsafe.Pointer(&fv)))
	}
	vd := cfDict(ks, vs)
	for i := range ks {
		cfRelease(ks[i])
		cfRelease(vs[i])
	}
	ak, av := []uintptr{kCTFontVariationAttribute}, []uintptr{vd}
	var none uintptr
	if kCTFontOpticalSizeAttribute != 0 {
		// A number pins the optical size; "none" would reset opsz to
		// the axis default.
		if v, ok := vals[opszTag]; ok {
			fv := float64(v)
			none = cfNumberCreate(0, cfNumberFloat64Type, unsafe.Pointer(&fv))
		} else {
			none = cfStr("none")
		}
		ak, av = append(ak, kCTFontOpticalSizeAttribute), append(av, none)
	}
	attrs := cfDict(ak, av)
	cfRelease(vd)
	if none != 0 {
		cfRelease(none)
	}
	desc := ctFontDescriptorCreateWithAttributes(attrs)
	cfRelease(attrs)
	return desc
}

var opszTag = tag4("opsz")
var wghtTag = tag4("wght")

func (e *ctEngine) load(name string) []*Face {
	seen := map[faceKey]bool{}
	// Named instances of one variable font share its tables (see
	// ctSharedTables).
	shared := map[string]*ctSharedTables{}
	var out []*Face
	// add registers the style; its tables are read when a layout first
	// uses it (newLazyEngineFace).
	add := func(f uintptr, w Weight, italic bool) {
		if seen[faceKey{w, italic}] {
			cfRelease(f)
			return
		}
		seen[faceKey{w, italic}] = true
		owner := newCTFontOwner(f, cfRelease)
		key := tableKey(f)
		sh := shared[key]
		if sh == nil {
			sh = &ctSharedTables{}
			shared[key] = sh
		}
		out = append(out, ctLazyFace(name, w, italic, owner, sh))
	}
	if name == ctSystemUI {
		// The system UI font is reachable only through the UI font API
		// (CoreText maps its private names to Times otherwise). Weights
		// go through kCTFontWeightTrait; the optical size is pinned (17,
		// the text design) so every point size draws the instance
		// go-text measures.
		for _, w := range []Weight{Light, Regular, Medium, Semibold, Bold} {
			for _, it := range []bool{false, true} {
				if f := uiFontWeighted(w, it, 17); f != 0 {
					add(f, w, it)
				}
			}
		}
		return out
	}
	fonts := e.fonts(name)
	perSlant := map[bool]int{}
	for _, f := range fonts {
		perSlant[ctFontGetSymbolicTraits(f)&ctFontItalicTrait != 0]++
	}
	for _, f := range fonts {
		italic := ctFontGetSymbolicTraits(f)&ctFontItalicTrait != 0
		vals := ctVariation(f)
		hasWght := false
		for _, a := range fvarAxesOfCT(f) {
			if a == wghtTag {
				hasWght = true
			}
		}
		if hasWght && perSlant[italic] == 1 {
			// A lone variable face (the system font, SF Mono):
			// instantiate the weights the UI uses. Families listing
			// named instances (PingFang) use those as they are.
			for _, w := range []Weight{Light, Regular, Medium, Semibold, Bold} {
				nv := map[uint32]float32{}
				for k, v := range vals {
					nv[k] = v
				}
				nv[wghtTag] = float32(w)
				if inst := withVariation(f, nv); inst != 0 {
					add(inst, w, italic)
				}
			}
			cfRelease(f)
			continue
		}
		add(f, ctWeight(f), italic)
	}
	return out
}

// tableKey identifies the font file of f: its head table (checksum
// adjustment, dates) and glyph count.
func tableKey(f uintptr) string {
	var k []byte
	for _, t := range []string{"head", "maxp"} {
		d := ctFontCopyTable(f, tag4(t), 0)
		if d == 0 {
			continue
		}
		k = append(k, unsafe.Slice((*byte)(cfDataGetBytePtr(d)), cfDataGetLength(d))...)
		cfRelease(d)
	}
	return string(k)
}

// fvarAxesOfCT lists the variation axis tags of f.
func fvarAxesOfCT(f uintptr) []uint32 {
	d := ctFontCopyTable(f, tag4("fvar"), 0)
	if d == 0 {
		return nil
	}
	defer cfRelease(d)
	n := cfDataGetLength(d)
	if n < 16 {
		return nil
	}
	b := unsafe.Slice((*byte)(cfDataGetBytePtr(d)), n)
	off := int(b[4])<<8 | int(b[5])
	cnt := int(b[8])<<8 | int(b[9])
	size := int(b[10])<<8 | int(b[11])
	var out []uint32
	for i := 0; i < cnt; i++ {
		p := off + i*size
		if p+4 > len(b) {
			break
		}
		out = append(out, uint32(b[p])<<24|uint32(b[p+1])<<16|uint32(b[p+2])<<8|uint32(b[p+3]))
	}
	return out
}

// ctRaster draws the glyphs of one CTFont.
type ctRaster struct {
	nativeMetrics   map[nativeGlyphMetricKey]cgRect
	nativePositions []cgPoint
	*ctFontOwner
	ctRasterImpl // the text engine's part (engine_<engine>_darwin.go)
	upem         float32
	color        bool // colour glyphs (Apple Color Emoji): mask = luminance
}

type ctSizeKey struct{ size, skew float32 }

// newCTRaster takes ownership of f, including when no Face wraps the raster.
func newCTRaster(f uintptr, vals map[uint32]float32) *ctRaster {
	return newCTRasterOwned(newCTFontOwner(f, cfRelease), vals)
}

func newCTRasterOwned(owner *ctFontOwner, vals map[uint32]float32) *ctRaster {
	f := owner.base
	r := &ctRaster{ctFontOwner: owner, upem: float32(ctFontGetUnitsPerEm(f))}
	for _, t := range []string{"sbix", "COLR", "CBDT", "SVG "} {
		if d := ctFontCopyTable(f, tag4(t), 0); d != 0 {
			cfRelease(d)
			r.color = true
		}
	}
	if len(vals) > 0 {
		r.desc = variationDesc(vals)
	}
	return r
}

func (r *ctRaster) font(size, skew float32) uintptr {
	defer runtime.KeepAlive(r)
	k := ctSizeKey{size, skew}
	if f, ok := r.sized[k]; ok {
		return f
	}
	if len(r.sized) > 64 {
		for k, f := range r.sized {
			cfRelease(f)
			delete(r.sized, k)
		}
	}
	// The text matrix slants x by skew per unit of height above the
	// baseline (CoreText y grows upwards).
	var m *cgAffine
	if skew != 0 {
		m = &cgAffine{A: 1, C: float64(skew), D: 1}
	}
	f := ctFontCreateCopyWithAttributes(r.base, float64(size), m, r.desc)
	if f != 0 {
		r.sized[k] = f
	}
	return f
}

func (r *ctRaster) raster(id uint32, size float32, subx uint8, skew float32) (*image.Alpha, image.Point, bool) {
	defer runtime.KeepAlive(r)
	if id > 0xFFFF || size <= 0 {
		return nil, image.Point{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.font(size, skew)
	if f == 0 {
		return nil, image.Point{}, false
	}
	g := uint16(id)
	var rect cgRect
	ctFontGetBoundingRectsForGlyphs(f, ctFontOrientationHorz, &g, &rect, 1)
	if rect.Size.W <= 0 || rect.Size.H <= 0 {
		return nil, image.Point{}, true // empty glyph (space)
	}
	// A generous box (antialiasing, smoothing dilation, the skew, the
	// sub-pixel shift), trimmed after drawing.
	sk := math.Abs(float64(skew)) * math.Max(math.Abs(rect.Origin.Y), math.Abs(rect.Origin.Y+rect.Size.H))
	ox := int(math.Floor(rect.Origin.X-sk)) - 3
	right := int(math.Ceil(rect.Origin.X+rect.Size.W+sk)) + 4
	oy := int(math.Floor(-(rect.Origin.Y + rect.Size.H))) - 3 // top, device y down
	bottom := int(math.Ceil(-rect.Origin.Y)) + 3
	w, h := right-ox, bottom-oy
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return nil, image.Point{}, false
	}
	cs := cgColorSpaceCreateDeviceGray()
	ctx := cgBitmapContextCreate(0, int64(w), int64(h), 8, 0, cs, cgImageAlphaNone)
	cgColorSpaceRelease(cs)
	if ctx == 0 {
		return nil, image.Point{}, false
	}
	defer cgContextRelease(ctx)
	cgContextSetShouldAntialias(ctx, true)
	// Font smoothing is CoreText's stem darkening, as macOS draws dark
	// text on a light background (no LCD filtering in a gray context).
	cgContextSetAllowsFontSmoothing(ctx, true)
	cgContextSetShouldSmoothFonts(ctx, true)
	cgContextSetAllowsFontSubpixelPositioning(ctx, true)
	cgContextSetShouldSubpixelPositionFonts(ctx, true)
	cgContextSetAllowsFontSubpixelQuantization(ctx, false)
	cgContextSetShouldSubpixelQuantizeFonts(ctx, false)
	data := cgBitmapContextGetData(ctx)
	bpr := int(cgBitmapContextGetBytesPerRow(ctx))
	if data == nil {
		return nil, image.Point{}, false
	}
	src := unsafe.Slice((*byte)(data), bpr*h)
	// Outline glyphs are drawn black on white and inverted: that is the
	// smoothing macOS applies to ordinary (dark on light) text; white on
	// black would get the much heavier light-on-dark dilation. Colour
	// (sbix, COLR) glyphs are drawn white on black, so their mask is the
	// image's luminance (a light emoji face with dark features), not its
	// inverse.
	inv := !r.color
	if inv {
		for i := range src {
			src[i] = 0xFF
		}
		cgContextSetGrayFillColor(ctx, 0, 1)
	} else {
		cgContextSetGrayFillColor(ctx, 1, 1)
	}
	// The pen sits at (-ox + subx/4, -oy) in the y-down mask; CG's origin
	// is the bottom-left corner.
	pos := cgPoint{X: float64(-ox) + float64(subx)/4, Y: float64(h + oy)}
	ctFontDrawGlyphs(f, &g, &pos, 1, ctx)
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := m.Pix[y*w : (y+1)*w]
		copy(row, src[y*bpr:y*bpr+w])
		if inv {
			for i, v := range row {
				row[i] = 255 - v
			}
		}
	}
	m, at := trimMask(m, 1) // keep the 1px transparent border outline masks have
	if m == nil {
		return nil, image.Point{}, true
	}
	// No extra gamma: CoreText's smoothing already gives the native
	// stroke weight.
	return m, image.Pt(ox+at.X, oy+at.Y), true
}

// advance returns CoreText's advance of glyph id at size (tests compare it
// with go-text's).
func (r *ctRaster) advance(id uint32, size float32) float32 {
	defer runtime.KeepAlive(r)
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.font(size, 0)
	g := uint16(id)
	var a cgSize
	ctFontGetAdvancesForGlyphs(f, ctFontOrientationHorz, &g, &a, 1)
	return float32(a.W)
}

// unitPath returns the outline of glyph id in font units, y down.
func (r *ctRaster) unitPath(id uint32) *paint.Path {
	defer runtime.KeepAlive(r)
	if id > 0xFFFF {
		return nil
	}
	r.mu.Lock()
	if r.unit == 0 {
		r.unit = ctFontCreateCopyWithAttributes(r.base, float64(r.upem), nil, r.desc)
	}
	u := r.unit
	r.mu.Unlock()
	if u == 0 {
		return nil
	}
	cp := ctFontCreatePathForGlyph(u, uint16(id), nil)
	if cp == 0 {
		return nil
	}
	defer cgPathRelease(cp)
	pathMu.Lock()
	defer pathMu.Unlock()
	pathCur, pathOpen = paint.NewPath(), false
	cgPathApply(cp, 0, pathApplyCB)
	if pathOpen {
		pathCur.Close()
	}
	p := pathCur
	pathCur = nil
	if len(p.Segs) == 0 {
		return nil
	}
	return p
}
