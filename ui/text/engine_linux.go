//go:build linux && !android

package text

import (
	"image"
	"math"
	"os"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// FreeType + fontconfig engine (Linux), written against the FreeType 2 API
// reference and the fontconfig developer reference, loaded with purego
// dlopen (no cgo). When either library is missing the pure-Go path is
// used.
//
// fontconfig lists the faces of a family (FcFontList: file, index, weight,
// slant) and finds per-rune fallbacks (FcFontMatch with a charset). go-text
// shapes from the same file and face index. Named instances of variable
// fonts are opened in FreeType by their instance index, and go-text is set
// to the design coordinates FreeType reports
// (FT_Get_Var_Design_Coordinates), so both use the same instance.
//
// Glyphs are loaded with FT_LOAD_TARGET_LIGHT (light, vertical-only
// hinting) | FT_LOAD_NO_BITMAP and rendered with FT_RENDER_MODE_LIGHT to an
// 8-bit gray bitmap; subx and skew are FT_Set_Transform's delta and
// matrix. Colour emoji fonts with bitmaps only (Noto Color Emoji, CBDT)
// have no outline to load: those glyphs are left blank this round.

var (
	ftOnce sync.Once
	ftLib  unsafe.Pointer
	ftMu   sync.Mutex // FT_New_Face / FT_Done_Face use the library

	ftInitFreeType       func(alib *unsafe.Pointer) int32
	ftNewFace            func(lib unsafe.Pointer, path string, index int64, aface *unsafe.Pointer) int32
	ftSetCharSize        func(face unsafe.Pointer, w, h int64, hres, vres uint32) int32
	ftSetTransform       func(face unsafe.Pointer, m *ftMatrix, d *ftVector)
	ftLoadGlyph          func(face unsafe.Pointer, glyph uint32, flags int32) int32
	ftRenderGlyph        func(slot unsafe.Pointer, mode int32) int32
	ftGetVarDesignCoords func(face unsafe.Pointer, n uint32, coords *int64) int32
	fcInit               func() int32
	fcPatternCreate      func() unsafe.Pointer
	fcPatternDestroy     func(unsafe.Pointer)
	fcPatternAddString   func(p unsafe.Pointer, obj, s string) int32
	fcPatternAddCharSet  func(p unsafe.Pointer, obj string, cs unsafe.Pointer) int32
	fcPatternGetString   func(p unsafe.Pointer, obj string, n int32, s **byte) int32
	fcPatternGetInteger  func(p unsafe.Pointer, obj string, n int32, v *int32) int32
	fcPatternGetDouble   func(p unsafe.Pointer, obj string, n int32, v *float64) int32
	fcPatternGetCharSet  func(p unsafe.Pointer, obj string, n int32, cs *unsafe.Pointer) int32
	fcCharSetCreate      func() unsafe.Pointer
	fcCharSetAddChar     func(cs unsafe.Pointer, r uint32) int32
	fcCharSetHasChar     func(cs unsafe.Pointer, r uint32) int32
	fcCharSetDestroy     func(unsafe.Pointer)
	fcConfigSubstitute   func(cfg, p unsafe.Pointer, kind int32) int32
	fcDefaultSubstitute  func(p unsafe.Pointer)
	fcFontMatch          func(cfg, p unsafe.Pointer, res *int32) unsafe.Pointer
	fcObjectSetCreate    func() unsafe.Pointer
	fcObjectSetAdd       func(os unsafe.Pointer, obj string) int32
	fcObjectSetDestroy   func(unsafe.Pointer)
	fcFontList           func(cfg, p, os unsafe.Pointer) unsafe.Pointer
	fcFontSetDestroy     func(unsafe.Pointer)
)

const (
	ftLoadNoBitmap    = 1 << 3
	ftLoadTargetLight = 1 << 16 // FT_LOAD_TARGET_(FT_RENDER_MODE_LIGHT)
	ftRenderModeLight = 1
	fcMatchPattern    = 0
	fcResultMatch     = 0
)

func init() { openEngine = openFreeType }

func openFreeType() sysEngine {
	ftOnce.Do(func() {
		// The FT_FaceRec / FT_GlyphSlotRec offsets read here are the
		// LP64 layout.
		if unsafe.Sizeof(uintptr(0)) != 8 || !loadFreeType() {
			return
		}
		var lib unsafe.Pointer
		if ftInitFreeType(&lib) != 0 || lib == nil || fcInit() == 0 {
			return
		}
		ftLib = lib
	})
	if ftLib == nil {
		return nil
	}
	return &ftEngine{files: map[string]*os.File{}}
}

func loadFreeType() (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	open := func(names ...string) uintptr {
		for _, n := range names {
			if h, err := purego.Dlopen(n, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
				return h
			}
		}
		panic("not found")
	}
	ft := open("libfreetype.so.6", "libfreetype.so")
	fc := open("libfontconfig.so.1", "libfontconfig.so")
	purego.RegisterLibFunc(&ftInitFreeType, ft, "FT_Init_FreeType")
	purego.RegisterLibFunc(&ftNewFace, ft, "FT_New_Face")
	purego.RegisterLibFunc(&ftSetCharSize, ft, "FT_Set_Char_Size")
	purego.RegisterLibFunc(&ftSetTransform, ft, "FT_Set_Transform")
	purego.RegisterLibFunc(&ftLoadGlyph, ft, "FT_Load_Glyph")
	purego.RegisterLibFunc(&ftRenderGlyph, ft, "FT_Render_Glyph")
	purego.RegisterLibFunc(&ftGetVarDesignCoords, ft, "FT_Get_Var_Design_Coordinates")
	purego.RegisterLibFunc(&fcInit, fc, "FcInit")
	purego.RegisterLibFunc(&fcPatternCreate, fc, "FcPatternCreate")
	purego.RegisterLibFunc(&fcPatternDestroy, fc, "FcPatternDestroy")
	purego.RegisterLibFunc(&fcPatternAddString, fc, "FcPatternAddString")
	purego.RegisterLibFunc(&fcPatternAddCharSet, fc, "FcPatternAddCharSet")
	purego.RegisterLibFunc(&fcPatternGetString, fc, "FcPatternGetString")
	purego.RegisterLibFunc(&fcPatternGetInteger, fc, "FcPatternGetInteger")
	purego.RegisterLibFunc(&fcPatternGetDouble, fc, "FcPatternGetDouble")
	purego.RegisterLibFunc(&fcPatternGetCharSet, fc, "FcPatternGetCharSet")
	purego.RegisterLibFunc(&fcCharSetCreate, fc, "FcCharSetCreate")
	purego.RegisterLibFunc(&fcCharSetAddChar, fc, "FcCharSetAddChar")
	purego.RegisterLibFunc(&fcCharSetHasChar, fc, "FcCharSetHasChar")
	purego.RegisterLibFunc(&fcCharSetDestroy, fc, "FcCharSetDestroy")
	purego.RegisterLibFunc(&fcConfigSubstitute, fc, "FcConfigSubstitute")
	purego.RegisterLibFunc(&fcDefaultSubstitute, fc, "FcDefaultSubstitute")
	purego.RegisterLibFunc(&fcFontMatch, fc, "FcFontMatch")
	purego.RegisterLibFunc(&fcObjectSetCreate, fc, "FcObjectSetCreate")
	purego.RegisterLibFunc(&fcObjectSetAdd, fc, "FcObjectSetAdd")
	purego.RegisterLibFunc(&fcObjectSetDestroy, fc, "FcObjectSetDestroy")
	purego.RegisterLibFunc(&fcFontList, fc, "FcFontList")
	purego.RegisterLibFunc(&fcFontSetDestroy, fc, "FcFontSetDestroy")
	return true
}

type ftEngine struct {
	mu    sync.Mutex
	files map[string]*os.File // font files kept open for go-text's lazy reads
}

func (e *ftEngine) name() string { return "freetype" }

func (e *ftEngine) defaults() (sans, mono []famSpec) {
	sans = []famSpec{
		{name: "Noto Sans"},
		{name: "DejaVu Sans"},
		{name: "Cantarell"},
		{name: "Ubuntu"},
		{name: "Noto Sans CJK SC", class: classCJK},
		{name: "Source Han Sans SC", class: classCJK},
		{name: "WenQuanYi Micro Hei", class: classCJK},
		{name: "WenQuanYi Zen Hei", class: classCJK},
		{name: "Droid Sans Fallback", class: classCJK},
	}
	mono = []famSpec{{name: "DejaVu Sans Mono"}, {name: "Noto Sans Mono"}, {name: "Liberation Mono"}}
	return
}

func cString(p *byte) string {
	if p == nil {
		return ""
	}
	var b []byte
	for q := unsafe.Pointer(p); *(*byte)(q) != 0; q = unsafe.Add(q, 1) {
		b = append(b, *(*byte)(q))
	}
	return string(b)
}

func fcString(p unsafe.Pointer, obj string) string {
	var s *byte
	if fcPatternGetString(p, obj, 0, &s) != fcResultMatch {
		return ""
	}
	return cString(s)
}

func fcInt(p unsafe.Pointer, obj string, def int) int {
	var v int32
	if fcPatternGetInteger(p, obj, 0, &v) == fcResultMatch {
		return int(v)
	}
	var d float64
	if fcPatternGetDouble(p, obj, 0, &d) == fcResultMatch {
		return int(math.Round(d))
	}
	return def
}

// fallback asks fontconfig for the best sans-serif face covering r.
func (e *ftEngine) fallback(r rune) string {
	ftMu.Lock()
	defer ftMu.Unlock()
	p := fcPatternCreate()
	defer fcPatternDestroy(p)
	cs := fcCharSetCreate()
	fcCharSetAddChar(cs, uint32(r))
	fcPatternAddString(p, "family", "sans-serif")
	fcPatternAddCharSet(p, "charset", cs)
	fcCharSetDestroy(cs) // the pattern holds its own reference
	fcConfigSubstitute(nil, p, fcMatchPattern)
	fcDefaultSubstitute(p)
	var res int32
	m := fcFontMatch(nil, p, &res)
	if m == nil {
		return ""
	}
	defer fcPatternDestroy(m)
	var mcs unsafe.Pointer
	if fcPatternGetCharSet(m, "charset", 0, &mcs) != fcResultMatch || fcCharSetHasChar(mcs, uint32(r)) == 0 {
		return ""
	}
	return fcString(m, "family")
}

func (e *ftEngine) list(name string) []fcFace {
	ftMu.Lock()
	defer ftMu.Unlock()
	p := fcPatternCreate()
	defer fcPatternDestroy(p)
	fcPatternAddString(p, "family", name)
	osz := fcObjectSetCreate()
	defer fcObjectSetDestroy(osz)
	for _, o := range []string{"file", "index", "weight", "slant", "width", "variable", "family"} {
		fcObjectSetAdd(osz, o)
	}
	fs := fcFontList(nil, p, osz)
	if fs == nil {
		return nil
	}
	defer fcFontSetDestroy(fs)
	// FcFontSet: int nfont; int sfont; FcPattern **fonts.
	n := int(*(*int32)(fs))
	arr := *(*unsafe.Pointer)(unsafe.Add(fs, 8))
	var out []fcFace
	for i := 0; i < n; i++ {
		fp := *(*unsafe.Pointer)(unsafe.Add(arr, i*8))
		f := fcFace{file: fcString(fp, "file"), index: fcInt(fp, "index", 0), weight: fcInt(fp, "weight", 80),
			slant: fcInt(fp, "slant", 0), width: fcInt(fp, "width", 100), vari: fcInt(fp, "variable", 0) != 0}
		if f.file != "" {
			out = append(out, f)
		}
	}
	return out
}

func (e *ftEngine) load(name string) []*Face {
	faces := pickFCFaces(e.list(name))
	var out []*Face
	for _, f := range faces {
		if face := e.face(name, f); face != nil {
			out = append(out, face)
		}
	}
	return out
}

func (e *ftEngine) face(name string, f fcFace) *Face {
	ftMu.Lock()
	var ftf unsafe.Pointer
	errc := ftNewFace(ftLib, f.file, int64(f.index), &ftf)
	ftMu.Unlock()
	if errc != 0 || ftf == nil {
		return nil
	}
	e.mu.Lock()
	fh, ok := e.files[f.file]
	if !ok {
		var err error
		if fh, err = os.Open(f.file); err != nil {
			e.mu.Unlock()
			return nil
		}
		e.files[f.file] = fh
	}
	lds, err := ot.NewLoaders(fh)
	e.mu.Unlock()
	if err != nil || f.index&0xFFFF >= len(lds) {
		return nil
	}
	ld := lds[f.index&0xFFFF]
	fn, err := font.NewFont(ld)
	if err != nil {
		return nil
	}
	gt := font.NewFace(fn)
	if axes := fvarAxes(ld); len(axes) > 0 {
		coords := make([]int64, len(axes))
		if ftGetVarDesignCoords(ftf, uint32(len(axes)), &coords[0]) == 0 {
			vals := map[uint32]float32{}
			for i, a := range axes {
				vals[uint32(a.Tag)] = float32(coords[i]) / 65536
			}
			setDesignCoords(gt, axes, vals)
		}
	}
	return newEngineFace(name, cssWeightFromFC(f.weight), f.slant != 0, gt, &ftRaster{face: ftf}, nil)
}

// ftRaster rasterizes the glyphs of one FT_Face.
type ftRaster struct {
	mu   sync.Mutex
	face unsafe.Pointer
}

func (r *ftRaster) raster(id uint32, size float32, subx uint8, skew float32) (*image.Alpha, image.Point, bool) {
	if size <= 0 {
		return nil, image.Point{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if ftSetCharSize(r.face, 0, int64(math.Round(float64(size)*64)), 72, 72) != 0 {
		return nil, image.Point{}, false
	}
	m, d := ftTransform(subx, skew)
	ftSetTransform(r.face, &m, &d)
	if ftLoadGlyph(r.face, id, ftLoadTargetLight|ftLoadNoBitmap) != 0 {
		return nil, image.Point{}, false // no outline (bitmap-only colour emoji)
	}
	slot := *(*unsafe.Pointer)(unsafe.Add(r.face, ftFaceGlyphOffset))
	if ftRenderGlyph(slot, ftRenderModeLight) != 0 {
		return nil, image.Point{}, false
	}
	bm := readFTBitmap(slot)
	mask, ok := bm.alpha()
	if !ok {
		return nil, image.Point{}, false
	}
	if mask == nil {
		return nil, image.Point{}, true
	}
	if mask.Rect.Dx() > 4096 || mask.Rect.Dy() > 4096 {
		return nil, image.Point{}, false
	}
	applyGammaLUT(mask, &engineLUT)
	return mask, image.Pt(bm.left, -bm.top), true
}
