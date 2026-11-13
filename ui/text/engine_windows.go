//go:build windows

package text

import (
	"image"
	"math"
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// DirectWrite engine (Windows), written against the Microsoft DirectWrite
// reference (dwrite.h, dwrite_2.h) and called as COM through syscall (no
// cgo).
//
// Fonts are matched by family name in the system font collection
// (IDWriteFontCollection::FindFamilyName); each non-simulated IDWriteFont
// gives an IDWriteFontFace, whose file (IDWriteLocalFontFileLoader) and
// face index feed go-text for shaping, so glyph ids and advances come from
// the same font file DirectWrite renders. Glyphs are rasterized with
// IDWriteGlyphRunAnalysis: on Windows 8.1+ (IDWriteFactory2) in
// NATURAL_SYMMETRIC mode with grid fitting on and grayscale antialiasing,
// read as an ALIASED_1x1 (8-bit) texture; on older systems (IDWriteFactory)
// the CLEARTYPE_3x1 texture is averaged to gray. subx is the run's
// baseline origin x, skew a DWRITE_MATRIX. Colour emoji (Segoe UI Emoji
// COLR) render as their monochrome base glyphs.
//
// Floating-point arguments travel in the x64 registers syscall loads, so
// the engine runs on windows/amd64 only; other architectures use the
// pure-Go path.

type comObj = unsafe.Pointer

var (
	iidIDWriteFactory             = winGUID{0xb859ee5a, 0xd838, 0x4b5b, [8]byte{0xa2, 0xe8, 0x1a, 0xdc, 0x7d, 0x93, 0xdb, 0x48}}
	iidIDWriteFactory2            = winGUID{0x0439fc60, 0xca44, 0x4994, [8]byte{0x8d, 0xee, 0x3a, 0x9a, 0xf7, 0xb7, 0x32, 0xec}}
	iidIDWriteLocalFontFileLoader = winGUID{0xb2d9f3ec, 0xc9fe, 0x4a11, [8]byte{0xa2, 0xec, 0xd8, 0x62, 0x08, 0xf7, 0xc0, 0xa2}}
	procDWriteCreateFactory       = syscall.NewLazyDLL("dwrite.dll").NewProc("DWriteCreateFactory")
	dwOnce                        sync.Once
	dwFactory, dwFactory2, dwColl comObj
)

type winGUID struct {
	Data1        uint32
	Data2, Data3 uint16
	Data4        [8]byte
}

// vtable indices (dwrite.h order, IUnknown first).
const (
	vtQueryInterface = 0
	vtRelease        = 2

	vtFactoryGetSystemFontCollection = 3
	vtFactoryCreateGlyphRunAnalysis  = 23
	vtFactory2CreateGlyphRunAnalysis = 30

	vtCollFindFamilyName = 5
	vtCollGetFontFamily  = 4

	vtListGetFontCount = 4
	vtListGetFont      = 5

	vtFontGetWeight      = 4
	vtFontGetStretch     = 5
	vtFontGetStyle       = 6
	vtFontGetSimulations = 10
	vtFontCreateFontFace = 13

	vtFaceGetFiles = 4
	vtFaceGetIndex = 5

	vtFileGetReferenceKey = 3
	vtFileGetLoader       = 4

	vtLocalGetFilePathLength = 4
	vtLocalGetFilePath       = 5

	vtAnalysisGetBounds  = 3
	vtAnalysisCreateTex  = 4
	dwTextureAliased1x1  = 0
	dwTextureClearType   = 1
	dwRenderNaturalSym   = 5
	dwMeasureNatural     = 0
	dwGridFitEnabled     = 2
	dwAntialiasGrayscale = 1
	dwStretchNormal      = 5
	dwStyleNormal        = 0
)

func comCall(obj comObj, idx int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, idx*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func comRelease(obj comObj) {
	if obj != nil {
		comCall(obj, vtRelease)
	}
}

func hrOK(hr uintptr) bool { return int32(hr) >= 0 }

func init() {
	if runtime.GOARCH == "amd64" {
		openEngine = openDirectWrite
	}
}

func openDirectWrite() sysEngine {
	dwOnce.Do(func() {
		if procDWriteCreateFactory.Find() != nil {
			return
		}
		var f comObj
		hr, _, _ := procDWriteCreateFactory.Call(0 /* SHARED */, uintptr(unsafe.Pointer(&iidIDWriteFactory)), uintptr(unsafe.Pointer(&f)))
		if !hrOK(hr) || f == nil {
			return
		}
		var coll comObj
		if !hrOK(comCall(f, vtFactoryGetSystemFontCollection, uintptr(unsafe.Pointer(&coll)), 0)) || coll == nil {
			comRelease(f)
			return
		}
		var f2 comObj
		if hrOK(comCall(f, vtQueryInterface, uintptr(unsafe.Pointer(&iidIDWriteFactory2)), uintptr(unsafe.Pointer(&f2)))) {
			dwFactory2 = f2
		}
		dwFactory, dwColl = f, coll
	})
	if dwFactory == nil {
		return nil
	}
	return &dwEngine{files: map[string]*os.File{}}
}

type dwEngine struct {
	mu    sync.Mutex
	files map[string]*os.File // font files kept open for go-text's lazy reads
}

func (e *dwEngine) name() string { return "directwrite" }

func (e *dwEngine) defaults() (sans, mono []famSpec) {
	sans = []famSpec{
		{name: "Segoe UI"},
		{name: "Microsoft YaHei", class: classCJK},
		{name: "Yu Gothic UI", class: classCJK},
		{name: "Malgun Gothic", class: classCJK},
		{name: "SimSun", class: classCJK},
		{name: "Segoe UI Emoji", class: classEmoji},
		{name: "Segoe UI Symbol"},
		{name: "Nirmala UI"},
		{name: "Leelawadee UI"},
		{name: "Ebrima"},
		{name: "Segoe UI Historic"},
		{name: "Arial"},
	}
	mono = []famSpec{{name: "Consolas"}, {name: "Cascadia Mono"}}
	return
}

// fallback: DirectWrite's per-character fallback (IDWriteFontFallback)
// needs a COM text-analysis source implemented in Go; the default chain
// lists the system's script fonts instead.
func (e *dwEngine) fallback(r rune) string { return "" }

func (e *dwEngine) load(name string) []*Face {
	wname, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil
	}
	var idx uint32
	var exists int32
	if !hrOK(comCall(dwColl, vtCollFindFamilyName, uintptr(unsafe.Pointer(wname)), uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&exists)))) || exists == 0 {
		return nil
	}
	var fam comObj
	if !hrOK(comCall(dwColl, vtCollGetFontFamily, uintptr(idx), uintptr(unsafe.Pointer(&fam)))) || fam == nil {
		return nil
	}
	defer comRelease(fam)
	seen := map[faceKey]bool{}
	var out []*Face
	n := int(uint32(comCall(fam, vtListGetFontCount)))
	for pass := 0; pass < 2; pass++ { // normal-width faces first, then any
		for i := 0; i < n; i++ {
			var f comObj
			if !hrOK(comCall(fam, vtListGetFont, uintptr(i), uintptr(unsafe.Pointer(&f)))) || f == nil {
				continue
			}
			face := e.face(name, f, pass == 0, seen)
			comRelease(f)
			if face != nil {
				out = append(out, face)
			}
		}
		if len(out) > 0 {
			break
		}
	}
	return out
}

// face builds the engine face of IDWriteFont f; nil when skipped
// (simulated bold/oblique, other widths when normalOnly, duplicates).
func (e *dwEngine) face(name string, f comObj, normalOnly bool, seen map[faceKey]bool) *Face {
	if uint32(comCall(f, vtFontGetSimulations)) != 0 {
		return nil // go-text cannot reproduce simulated styles
	}
	if normalOnly && uint32(comCall(f, vtFontGetStretch)) != dwStretchNormal {
		return nil
	}
	w := roundWeight(int(uint32(comCall(f, vtFontGetWeight))))
	italic := uint32(comCall(f, vtFontGetStyle)) != dwStyleNormal
	key := faceKey{w, italic}
	if seen[key] {
		return nil
	}
	var ff comObj
	if !hrOK(comCall(f, vtFontCreateFontFace, uintptr(unsafe.Pointer(&ff)))) || ff == nil {
		return nil
	}
	path, ok := dwFacePath(ff)
	if !ok {
		comRelease(ff)
		return nil
	}
	index := int(uint32(comCall(ff, vtFaceGetIndex)))
	gt, err := e.goText(path, index)
	if err != nil {
		comRelease(ff)
		return nil
	}
	seen[key] = true
	return newEngineFace(name, w, italic, gt, &dwRaster{face: ff}, nil)
}

// goText opens face index of the font file at path for shaping. Variable
// faces are set to their weight (DirectWrite lists named instances).
func (e *dwEngine) goText(path string, index int) (*font.Face, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fh, ok := e.files[path]
	if !ok {
		var err error
		if fh, err = os.Open(path); err != nil {
			return nil, err
		}
		e.files[path] = fh
	}
	lds, err := ot.NewLoaders(fh)
	if err != nil {
		return nil, err
	}
	if index >= len(lds) {
		return nil, os.ErrNotExist
	}
	fn, err := font.NewFont(lds[index])
	if err != nil {
		return nil, err
	}
	return font.NewFace(fn), nil
}

// dwFacePath returns the local file of a font face.
func dwFacePath(ff comObj) (string, bool) {
	var nfiles uint32
	if !hrOK(comCall(ff, vtFaceGetFiles, uintptr(unsafe.Pointer(&nfiles)), 0)) || nfiles == 0 {
		return "", false
	}
	files := make([]comObj, nfiles)
	if !hrOK(comCall(ff, vtFaceGetFiles, uintptr(unsafe.Pointer(&nfiles)), uintptr(unsafe.Pointer(&files[0])))) {
		return "", false
	}
	defer func() {
		for _, f := range files {
			comRelease(f)
		}
	}()
	file := files[0]
	var key unsafe.Pointer
	var keySize uint32
	if !hrOK(comCall(file, vtFileGetReferenceKey, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&keySize)))) {
		return "", false
	}
	var loader comObj
	if !hrOK(comCall(file, vtFileGetLoader, uintptr(unsafe.Pointer(&loader)))) || loader == nil {
		return "", false
	}
	defer comRelease(loader)
	var local comObj
	if !hrOK(comCall(loader, vtQueryInterface, uintptr(unsafe.Pointer(&iidIDWriteLocalFontFileLoader)), uintptr(unsafe.Pointer(&local)))) || local == nil {
		return "", false // not a local file (memory / downloadable font)
	}
	defer comRelease(local)
	var n uint32
	if !hrOK(comCall(local, vtLocalGetFilePathLength, uintptr(key), uintptr(keySize), uintptr(unsafe.Pointer(&n)))) {
		return "", false
	}
	buf := make([]uint16, n+1)
	if !hrOK(comCall(local, vtLocalGetFilePath, uintptr(key), uintptr(keySize), uintptr(unsafe.Pointer(&buf[0])), uintptr(n+1))) {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

// dwGlyphRun is DWRITE_GLYPH_RUN (x64 layout).
type dwGlyphRun struct {
	fontFace      comObj
	fontEmSize    float32
	glyphCount    uint32
	glyphIndices  *uint16
	glyphAdvances *float32
	glyphOffsets  unsafe.Pointer
	isSideways    int32
	bidiLevel     uint32
}

// dwRect is RECT.
type dwRect struct{ Left, Top, Right, Bottom int32 }

// dwRaster rasterizes the glyphs of one IDWriteFontFace.
type dwRaster struct {
	mu   sync.Mutex
	face comObj
}

func f32(v float32) uintptr { return uintptr(math.Float32bits(v)) }

func (r *dwRaster) raster(id uint32, size float32, subx uint8, skew float32) (*image.Alpha, image.Point, bool) {
	if id > 0xFFFF || size <= 0 {
		return nil, image.Point{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	g := uint16(id)
	adv := float32(0)
	run := dwGlyphRun{fontFace: r.face, fontEmSize: size, glyphCount: 1, glyphIndices: &g, glyphAdvances: &adv}
	var mp *dwMatrix
	if skew != 0 {
		m := dwSkewMatrix(skew)
		mp = &m
	}
	ox := float32(subx) / 4
	var an comObj
	texType := uintptr(dwTextureAliased1x1)
	var hr uintptr
	if dwFactory2 != nil {
		hr = comCall(dwFactory2, vtFactory2CreateGlyphRunAnalysis, uintptr(unsafe.Pointer(&run)), uintptr(unsafe.Pointer(mp)),
			dwRenderNaturalSym, dwMeasureNatural, dwGridFitEnabled, dwAntialiasGrayscale, f32(ox), f32(0), uintptr(unsafe.Pointer(&an)))
	} else {
		texType = dwTextureClearType
		hr = comCall(dwFactory, vtFactoryCreateGlyphRunAnalysis, uintptr(unsafe.Pointer(&run)), f32(1), uintptr(unsafe.Pointer(mp)),
			dwRenderNaturalSym, dwMeasureNatural, f32(ox), f32(0), uintptr(unsafe.Pointer(&an)))
	}
	runtime.KeepAlive(&g)
	if !hrOK(hr) || an == nil {
		return nil, image.Point{}, false
	}
	defer comRelease(an)
	var rc dwRect
	if !hrOK(comCall(an, vtAnalysisGetBounds, texType, uintptr(unsafe.Pointer(&rc)))) {
		return nil, image.Point{}, false
	}
	if rc.Right <= rc.Left || rc.Bottom <= rc.Top {
		if texType != dwTextureAliased1x1 {
			return nil, image.Point{}, true // empty glyph
		}
		// The analysis may only offer the 3x1 texture: retry with it.
		texType = dwTextureClearType
		if !hrOK(comCall(an, vtAnalysisGetBounds, texType, uintptr(unsafe.Pointer(&rc)))) || rc.Right <= rc.Left || rc.Bottom <= rc.Top {
			return nil, image.Point{}, true
		}
	}
	w, h := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	if w > 4096 || h > 4096 {
		return nil, image.Point{}, false
	}
	bpp := 1
	if texType == dwTextureClearType {
		bpp = 3
	}
	buf := make([]byte, w*h*bpp)
	if !hrOK(comCall(an, vtAnalysisCreateTex, texType, uintptr(unsafe.Pointer(&rc)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))) {
		return nil, image.Point{}, false
	}
	var pix []byte
	if bpp == 3 {
		pix = clearTypeToGray(buf, w, h)
	} else {
		pix = buf
	}
	m := &image.Alpha{Pix: pix, Stride: w, Rect: image.Rect(0, 0, w, h)}
	applyGammaLUT(m, &engineLUT)
	return m, image.Pt(int(rc.Left), int(rc.Top)), true
}
