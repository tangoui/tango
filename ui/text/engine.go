package text

import (
	"bytes"
	"image"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unsafe"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/tangoui/tango/ui/paint"
)

// The system text engine.
//
// With the engine on (the default where one is available), system fonts are
// found by family name through the platform text stack and their glyphs are
// rasterized by it, with its hinting and filtering:
//
//   - macOS: CoreText (purego). Fonts are matched with CTFontDescriptor, so
//     formats the system can read but go-text cannot (PingFang's .ttc) work;
//     glyphs are drawn with CTFontDrawGlyphs into a CGBitmapContext.
//   - Windows: DirectWrite (COM over syscall). Fonts come from the system
//     font collection by family name; glyphs from IDWriteGlyphRunAnalysis.
//   - Linux: fontconfig + FreeType (purego dlopen), FT_LOAD_TARGET_LIGHT.
//
// Shaping and line breaking stay on go-text (HarfBuzz port) on every
// platform, fed with the very same font tables the engine renders from
// (CTFontCopyTable on macOS, the font file DirectWrite / fontconfig name
// elsewhere), so glyph ids and advances agree by construction. Fonts
// registered with RegisterFont always use the pure-Go path.
//
// Every engine face is a distinct *Face (a distinct paint.GlyphSource) that
// implements paint.GlyphRasterizer; pure-Go faces never rasterize through
// it. Switching the engine rebuilds the system chain with new Face values,
// so a glyph source never changes rasterization path (the GPU atlas keys
// masks by source).

// engineMode is the requested engine: TANGO_TEXT_ENGINE=go|system sets the
// initial value, UseSystemEngine changes it.
var engineMode = engineFromEnv(os.Getenv("TANGO_TEXT_ENGINE"))

// engineFromEnv parses TANGO_TEXT_ENGINE: "go" (or off/0/false) selects the
// pure-Go path; anything else, including unset and "system", the engine.
func engineFromEnv(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "go", "off", "0", "false":
		return false
	}
	return true
}

// UseSystemEngine enables (the default) or disables the system text engine
// for system fonts. Disabled, system font files are parsed and rasterized
// in pure Go, which renders identically on every machine with the same
// fonts (golden images, CPU/GPU comparison tests). The environment variable
// TANGO_TEXT_ENGINE=go or =system sets the initial state.
//
// It is a process-wide switch (all windows, renderers and goroutines) and
// takes effect through new glyph sources: a switch drops the layout cache
// and the next Shape builds the system font chain again with new *Face
// values (each a distinct paint.GlyphSource, so a distinct Src in the GPU
// atlas key). A Face never changes path: engine faces always rasterize
// through the engine (paint.GlyphRasterizer), pure-Go faces always through
// their outlines. Layouts shaped before the switch keep their old faces and
// draw exactly as before; reshape (or let the UI re-run its frame) to pick
// up the new engine. Fonts added with RegisterFont always use the pure-Go
// path. Call it before rendering starts when possible (tests: at the top of
// the test or in TestMain). It returns the previous setting, for restoring.
func UseSystemEngine(on bool) (prev bool) {
	reg.mu.Lock()
	prev = reg.engineOn
	changed := prev != on
	reg.engineOn = on
	if changed {
		reg.version++
	}
	reg.mu.Unlock()
	if changed {
		clearCache()
	}
	return prev
}

// Engine names the text engine rendering system fonts: "coretext",
// "directwrite", "freetype", or "go" when the system engine is disabled or
// unavailable.
func Engine() string {
	reg.chain(false)
	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.eng != nil {
		return reg.eng.eng.name()
	}
	return "go"
}

// sysEngine is a platform text engine (engine_<os>.go).
type sysEngine interface {
	name() string
	// defaults lists the default sans and monospace families, in priority
	// order.
	defaults() (sans, mono []famSpec)
	// load returns new faces for every installed style of family name (nil
	// when it is not installed).
	load(name string) []*Face
	// fallback names the installed family the platform would use for r
	// ("" when unknown).
	fallback(r rune) string
}

// famSpec is a default family of an engine.
type famSpec struct {
	name  string
	class runeClass // script the family is preferred for (0: none)
}

// glyphEngine rasterizes the glyphs of one engine face (see
// paint.GlyphRasterizer for the contract). It is safe for concurrent use.
type glyphEngine interface {
	raster(id uint32, size float32, subx uint8, skew float32) (*image.Alpha, image.Point, bool)
}

// openEngine opens the platform engine; nil when there is none or its
// libraries are missing. Set by engine_<os>.go.
var openEngine = func() sysEngine { return nil }

// engineState is one instance of the engine chain. A new one is built on
// every engine switch so faces are never shared between paths.
type engineState struct {
	eng    sysEngine
	mu     sync.Mutex
	byName map[string]*family
	byRune map[rune]*family
}

func newEngineState(e sysEngine) *engineState {
	return &engineState{eng: e, byName: map[string]*family{}, byRune: map[rune]*family{}}
}

// family returns the (lazily loaded) family name of this engine instance.
func (s *engineState) family(name string, class runeClass) *family {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(name)
	if fm, ok := s.byName[key]; ok {
		return fm
	}
	fm := &family{name: name, class: class, load: func() []*Face { return s.eng.load(name) }}
	s.byName[key] = fm
	return fm
}

func (s *engineState) chains() (sans, mono []*family) {
	sp, mp := s.eng.defaults()
	for _, f := range sp {
		sans = append(sans, s.family(f.name, f.class))
	}
	for _, f := range mp {
		mono = append(mono, s.family(f.name, f.class))
	}
	return
}

// fallbackFamily returns the platform's fallback family for r (cached per
// rune), or nil.
func (s *engineState) fallbackFamily(r rune) *family {
	s.mu.Lock()
	fm, ok := s.byRune[r]
	s.mu.Unlock()
	if ok {
		return fm
	}
	if name := s.eng.fallback(r); name != "" {
		fm = s.family(name, 0)
	}
	s.mu.Lock()
	if len(s.byRune) > 4096 {
		s.byRune = map[rune]*family{}
	}
	s.byRune[r] = fm
	s.mu.Unlock()
	return fm
}

// RasterGlyph implements paint.GlyphRasterizer. Pure-Go faces always
// return ok=false (their outlines are rasterized by paint); engine faces
// always rasterize through the engine, except keys the engine failed on,
// which keep falling back for the life of the face.
func (f *Face) RasterGlyph(id uint32, size float32, subx uint8, skew float32) (*image.Alpha, image.Point, bool) {
	if f.ras == nil {
		return nil, image.Point{}, false
	}
	k := rasterKey{id, size, subx, skew}
	f.mu.Lock()
	failed := f.noRaster[k]
	f.mu.Unlock()
	if failed {
		return nil, image.Point{}, false
	}
	m, off, ok := f.ras.raster(id, size, subx, skew)
	if !ok {
		f.mu.Lock()
		if f.noRaster == nil {
			f.noRaster = map[rasterKey]bool{}
		}
		f.noRaster[k] = true
		f.mu.Unlock()
	}
	return m, off, ok
}

type rasterKey struct {
	id   uint32
	size float32
	subx uint8
	skew float32
}

// SystemRasterized reports whether the face's glyphs are rasterized by the
// system text engine.
func (f *Face) SystemRasterized() bool { return f.ras != nil }

// newEngineFace wraps an engine face: gt shapes (same tables the engine
// renders), ras rasterizes, unitPath (optional) supplies outlines in font
// units when go-text cannot read them.
func newEngineFace(family string, w Weight, italic bool, gt *font.Face, ras glyphEngine, unitPath func(uint32) *paint.Path) *Face {
	f := newFace(family, w, gt)
	f.Italic = italic
	f.ras = ras
	f.unitPath = unitPath
	return f
}

// runeClass is the script group a rune prefers a font for.
type runeClass uint8

const (
	classNone  runeClass = iota
	classCJK             // Han, kana, Hangul, CJK punctuation and full-width forms
	classEmoji           // pictographic emoji
)

func classOf(r rune) runeClass {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF, r >= 0x2600 && r <= 0x27BF && r != 0x2713 && r != 0x2714,
		r == 0xFE0F, r == 0x200D, r >= 0x1F1E6 && r <= 0x1F1FF:
		return classEmoji
	case r >= 0x2E80 && r <= 0x2FDF, // radicals
		r >= 0x3000 && r <= 0x303F, // CJK symbols and punctuation
		r >= 0x3040 && r <= 0x30FF, // kana
		r >= 0x3100 && r <= 0x31FF,
		r >= 0x3200 && r <= 0x33FF,
		r >= 0xAC00 && r <= 0xD7AF, // Hangul
		r >= 0xFE30 && r <= 0xFE4F,
		r >= 0xFF00 && r <= 0xFFEF, // full-width forms
		unicode.Is(unicode.Han, r):
		return classCJK
	}
	return classNone
}

// cssWeightFromCT maps a CoreText weight trait (-1..1, NSFontWeight
// values) to the nearest CSS weight.
func cssWeightFromCT(t float64) Weight {
	steps := []struct {
		t float64
		w Weight
	}{{-0.8, 100}, {-0.6, 200}, {-0.4, 300}, {0, 400}, {0.23, 500}, {0.3, 600}, {0.4, 700}, {0.56, 800}, {0.62, 900}}
	best, bd := Weight(400), 10.0
	for _, s := range steps {
		d := s.t - t
		if d < 0 {
			d = -d
		}
		if d < bd {
			best, bd = s.w, d
		}
	}
	return best
}

// ctWeightTrait is the CoreText weight trait of a CSS weight.
func ctWeightTrait(w Weight) float64 {
	switch {
	case w <= 100:
		return -0.8
	case w <= 200:
		return -0.6
	case w <= 300:
		return -0.4
	case w <= 400:
		return 0
	case w <= 500:
		return 0.23
	case w <= 600:
		return 0.3
	case w <= 700:
		return 0.4
	case w <= 800:
		return 0.56
	}
	return 0.62
}

// cssWeightFromFC maps a fontconfig FC_WEIGHT to a CSS weight.
func cssWeightFromFC(w int) Weight {
	steps := []struct {
		fc int
		w  Weight
	}{{0, 100}, {40, 200}, {50, 300}, {55, 350}, {75, 400}, {80, 400}, {100, 500}, {180, 600}, {200, 700}, {205, 800}, {210, 900}, {215, 950}}
	best, bd := Weight(400), 1<<30
	for _, s := range steps {
		d := s.fc - w
		if d < 0 {
			d = -d
		}
		if d < bd {
			best, bd = s.w, d
		}
	}
	if best == 350 {
		best = 300
	}
	return best
}

// roundWeight rounds a CSS weight to a multiple of 100 in 100..900.
func roundWeight(w int) Weight {
	w = (w + 50) / 100 * 100
	return Weight(max(100, min(900, w)))
}

// sfntFromTables builds an in-memory OpenType file from tables (tag →
// data), skipping glyph data the shaper does not need (outlines and
// bitmaps: the engine renders those).
func sfntFromTables(tabs map[uint32][]byte) []byte {
	var list []ot.Table
	for tag, data := range tabs {
		if skipTable(tag) {
			continue
		}
		list = append(list, ot.Table{Tag: ot.Tag(tag), Content: data})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Tag < list[j].Tag })
	return ot.WriteTTF(list)
}

func tag4(s string) uint32 {
	return uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
}

// skipTable reports glyph-image tables the shaper never reads.
func skipTable(tag uint32) bool {
	switch tag {
	case tag4("glyf"), tag4("loca"), tag4("CFF "), tag4("CFF2"), tag4("gvar"), tag4("sbix"),
		tag4("CBDT"), tag4("CBLC"), tag4("EBDT"), tag4("EBLC"), tag4("EBSC"), tag4("bdat"), tag4("bloc"),
		tag4("SVG "), tag4("hvgl"), tag4("hvpm"), tag4("fpgm"), tag4("prep"), tag4("cvt "), tag4("hdmx"),
		tag4("LTSH"), tag4("VDMX"), tag4("DSIG"):
		return true
	}
	return false
}

// goTextFace parses an in-memory font (one face) for shaping.
func goTextFace(data []byte) (*font.Face, *ot.Loader, error) {
	ld, err := ot.NewLoader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	fn, err := font.NewFont(ld)
	if err != nil {
		return nil, nil, err
	}
	return font.NewFace(fn), ld, nil
}

// fvarAxes returns the variation axes of a font (nil when not variable).
func fvarAxes(ld *ot.Loader) []tables.VariationAxisRecord {
	raw, err := ld.RawTable(ot.MustNewTag("fvar"))
	if err != nil {
		return nil
	}
	fv, _, err := tables.ParseFvar(raw)
	if err != nil {
		return nil
	}
	return fv.Axis
}

// setDesignCoords sets the variation of face to the engine's axis values
// (tag → design value; missing axes keep their default), so shaping uses
// the instance the engine draws.
func setDesignCoords(face *font.Face, axes []tables.VariationAxisRecord, vals map[uint32]float32) {
	if len(axes) == 0 {
		return
	}
	coords := make([]float32, len(axes))
	for i, a := range axes {
		coords[i] = float32(a.Default)
		if v, ok := vals[uint32(a.Tag)]; ok {
			coords[i] = v
		}
	}
	face.SetCoords(face.Font.NormalizeVariations(coords))
}

// applyGammaLUT maps every coverage value of m through lut.
func applyGammaLUT(m *image.Alpha, lut *[256]uint8) {
	for i, v := range m.Pix {
		m.Pix[i] = lut[v]
	}
}

// trimMask crops m to its non-zero pixels plus pad, returning the cropped
// mask (origin 0,0) and the offset of its top-left inside m; nil when
// empty.
func trimMask(m *image.Alpha, pad int) (*image.Alpha, image.Point) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	x0, y0, x1, y1 := w, h, -1, -1
	for y := 0; y < h; y++ {
		row := m.Pix[y*m.Stride : y*m.Stride+w]
		for x, v := range row {
			if v != 0 {
				x0, x1 = min(x0, x), max(x1, x)
				y0, y1 = min(y0, y), max(y1, y)
			}
		}
	}
	if x1 < 0 {
		return nil, image.Point{}
	}
	x0, y0 = max(0, x0-pad), max(0, y0-pad)
	x1, y1 = min(w-1, x1+pad), min(h-1, y1+pad)
	out := image.NewAlpha(image.Rect(0, 0, x1-x0+1, y1-y0+1))
	for y := y0; y <= y1; y++ {
		copy(out.Pix[(y-y0)*out.Stride:], m.Pix[y*m.Stride+x0:y*m.Stride+x1+1])
	}
	return out, image.Pt(x0, y0)
}

// faceKey is a face's style within a family.
type faceKey struct {
	w      Weight
	italic bool
}

// engineLUT is the coverage curve applied to DirectWrite and FreeType
// masks: the same mild darkening (gamma 0.85) the pure-Go path uses as
// paint.CPU.TextGamma, since both engines output linear coverage and leave
// gamma / contrast to the blender.
var engineLUT = func() (l [256]uint8) {
	for i := range l {
		l[i] = uint8(math.Round(math.Pow(float64(i)/255, 0.85) * 255))
	}
	return
}()

// dwMatrix is DWRITE_MATRIX: x' = x*M11 + y*M21 + Dx, y' = x*M12 + y*M22 + Dy.
type dwMatrix struct{ M11, M12, M21, M22, Dx, Dy float32 }

// dwSkewMatrix slants x by skew per unit of height above the baseline.
// DirectWrite's y grows downwards, so the height above the baseline is -y.
func dwSkewMatrix(skew float32) dwMatrix { return dwMatrix{M11: 1, M21: -skew, M22: 1} }

// clearTypeToGray averages a CLEARTYPE_3x1 texture (RGB per pixel) to
// grayscale coverage.
func clearTypeToGray(buf []byte, w, h int) []byte {
	out := make([]byte, w*h)
	for i := range out {
		s := int(buf[3*i]) + int(buf[3*i+1]) + int(buf[3*i+2])
		out[i] = uint8((s + 1) / 3)
	}
	return out
}

// FreeType structures (LP64 layout, stable across FreeType 2.x; from
// freetype.h / ftimage.h).
type ftMatrix struct{ XX, XY, YX, YY int64 } // FT_Fixed 16.16
type ftVector struct{ X, Y int64 }           // FT_Pos 26.6

const (
	ftFaceGlyphOffset   = 152 // FT_FaceRec.glyph
	ftSlotBitmapOffset  = 152 // FT_GlyphSlotRec.bitmap (FT_Bitmap, 40 bytes)
	ftSlotBitmapLeftOff = 192 // FT_GlyphSlotRec.bitmap_left, bitmap_top
	ftPixelModeMono     = 1
	ftPixelModeGray     = 2
)

// ftTransform is the FT_Set_Transform matrix and delta for a quarter-pixel
// x offset and a synthetic italic skew (FreeType's y grows upwards: x' =
// x + skew*y).
func ftTransform(subx uint8, skew float32) (ftMatrix, ftVector) {
	return ftMatrix{XX: 1 << 16, XY: int64(math.Round(float64(skew) * 65536)), YY: 1 << 16},
		ftVector{X: int64(subx) * 16} // subx/4 px in 26.6
}

// ftBitmap is the rendered bitmap of a glyph slot.
type ftBitmap struct {
	rows, width, pitch int
	buf                unsafe.Pointer
	mode               uint8
	left, top          int // bitmap_left, bitmap_top (y up from the baseline)
}

// readFTBitmap reads the bitmap fields of an FT_GlyphSlotRec.
func readFTBitmap(slot unsafe.Pointer) ftBitmap {
	b := unsafe.Add(slot, ftSlotBitmapOffset)
	return ftBitmap{
		rows:  int(*(*uint32)(b)),
		width: int(*(*uint32)(unsafe.Add(b, 4))),
		pitch: int(*(*int32)(unsafe.Add(b, 8))),
		buf:   *(*unsafe.Pointer)(unsafe.Add(b, 16)),
		mode:  *(*uint8)(unsafe.Add(b, 26)),
		left:  int(*(*int32)(unsafe.Add(slot, ftSlotBitmapLeftOff))),
		top:   int(*(*int32)(unsafe.Add(slot, ftSlotBitmapLeftOff+4))),
	}
}

// alpha copies the bitmap into a mask: nil, true for an empty bitmap;
// false for a pixel mode other than 8-bit gray or 1-bit mono.
func (b ftBitmap) alpha() (*image.Alpha, bool) {
	if b.rows == 0 || b.width == 0 || b.buf == nil {
		return nil, true
	}
	if b.mode != ftPixelModeGray && b.mode != ftPixelModeMono {
		return nil, false
	}
	m := image.NewAlpha(image.Rect(0, 0, b.width, b.rows))
	ap := b.pitch
	if ap < 0 {
		ap = -ap
	}
	for y := 0; y < b.rows; y++ {
		// The pitch is the offset to go down one row; with an up flow
		// (negative pitch) the top row is the last in memory.
		ry := y
		if b.pitch < 0 {
			ry = b.rows - 1 - y
		}
		src := unsafe.Slice((*byte)(unsafe.Add(b.buf, ry*ap)), ap)
		row := m.Pix[y*m.Stride : y*m.Stride+b.width]
		if b.mode == ftPixelModeGray {
			copy(row, src[:b.width])
			continue
		}
		for x := range row {
			if src[x>>3]&(0x80>>(x&7)) != 0 {
				row[x] = 255
			}
		}
	}
	return m, true
}

// fcFace is one face fontconfig lists for a family.
type fcFace struct {
	file   string
	index  int // face index | named instance << 16
	weight int // FC_WEIGHT
	slant  int // FC_SLANT: 0 roman, 100 italic, 110 oblique
	width  int // FC_WIDTH: 100 normal
	vari   bool
}

// pickFCFaces keeps, per (weight, italic), one face fontconfig listed:
// normal width first, named instances over the bare variable font.
func pickFCFaces(in []fcFace) []fcFace {
	best := map[faceKey]fcFace{}
	var order []faceKey
	score := func(f fcFace) int {
		s := 0
		if f.width == 100 {
			s += 2
		}
		if !f.vari {
			s++
		}
		return s
	}
	for _, f := range in {
		if strings.HasSuffix(strings.ToLower(f.file), ".pcf.gz") || strings.HasSuffix(strings.ToLower(f.file), ".pcf") {
			continue // bitmap fonts
		}
		k := faceKey{cssWeightFromFC(f.weight), f.slant != 0}
		cur, ok := best[k]
		if !ok {
			order = append(order, k)
		}
		if !ok || score(f) > score(cur) {
			best[k] = f
		}
	}
	out := make([]fcFace, 0, len(order))
	for _, k := range order {
		out = append(out, best[k])
	}
	return out
}
