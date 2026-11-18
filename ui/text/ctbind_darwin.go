//go:build darwin

package text

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/tangoui/tango/ui/paint"
)

// purego bindings to CoreFoundation, CoreText and CoreGraphics, and small
// CoreFoundation helpers, shared by the text engines on Apple platforms.

const (
	cfStringEncodingUTF8  = 0x08000100
	cfNumberSInt32Type    = 3
	cfNumberFloat64Type   = 6
	ctFontUIFontSystem    = 2
	ctFontItalicTrait     = 1 << 0
	ctFontMonoSpaceTrait  = 1 << 10
	cgImageAlphaNone      = 0
	ctFontOrientationHorz = 1

	cfStringTokenizerUnitLineBreak         = 3
	ctParagraphStyleSpecifierBaseDirection = 13 // kCTParagraphStyleSpecifierBaseWritingDirection
	ctWritingDirectionNatural              = -1
	ctWritingDirectionLeftToRight          = 0
	ctWritingDirectionRightToLeft          = 1
	ctRunStatusRightToLeft                 = 1 << 0
)

type cgPoint struct{ X, Y float64 }
type cgSize struct{ W, H float64 }
type cgRect struct {
	Origin cgPoint
	Size   cgSize
}
type cgAffine struct{ A, B, C, D, Tx, Ty float64 }
type cfRange struct{ Loc, Len int64 }

// ctParagraphStyleSetting is CTParagraphStyleSetting.
type ctParagraphStyleSetting struct {
	Spec  uint32
	_     uint32
	Size  uint64
	Value unsafe.Pointer
}

var (
	ctOnce sync.Once
	ctOK   bool

	cfRelease                                  func(uintptr)
	cfRetain                                   func(uintptr) uintptr
	cfAttributedStringCreate                   func(uintptr, uintptr, uintptr) uintptr
	cfStringCreateWithBytes                    func(alloc uintptr, b *byte, n int64, enc uint32, ext bool) uintptr
	cfStringGetLength                          func(uintptr) int64
	cfStringGetCString                         func(s uintptr, buf *byte, n int64, enc uint32) bool
	cfArrayGetCount                            func(uintptr) int64
	cfArrayGetValueAtIndex                     func(uintptr, int64) uintptr
	cfDataGetLength                            func(uintptr) int64
	cfDataGetBytePtr                           func(uintptr) unsafe.Pointer
	cfDictionaryCreate                         func(alloc uintptr, keys, vals *uintptr, n int64, kcb, vcb uintptr) uintptr
	cfDictionaryGetValue                       func(d, k uintptr) uintptr
	cfDictionaryGetCount                       func(uintptr) int64
	cfDictionaryGetKeysValues                  func(d uintptr, keys, vals *uintptr)
	cfNumberCreate                             func(alloc uintptr, typ int64, v unsafe.Pointer) uintptr
	cfNumberGetValue                           func(n uintptr, typ int64, v unsafe.Pointer) bool
	cfURLCreateFromFileSystemRepresentation    func(alloc uintptr, b *byte, n int64, dir bool) uintptr
	ctFontManagerCreateFontDescriptorsFromURL  func(url uintptr) uintptr
	cfDataCreate                               func(alloc uintptr, b *byte, n int64) uintptr
	ctFontManagerCreateFontDescriptorsFromData func(data uintptr) uintptr

	ctFontDescriptorCreateWithAttributes          func(uintptr) uintptr
	ctFontDescriptorCreateMatchingFontDescriptors func(desc, mandatory uintptr) uintptr
	ctFontCreateWithFontDescriptor                func(desc uintptr, size float64, m *cgAffine) uintptr
	ctFontCreateCopyWithAttributes                func(f uintptr, size float64, m *cgAffine, desc uintptr) uintptr
	ctFontCreateCopyWithSymbolicTraits            func(f uintptr, size float64, m *cgAffine, val, mask uint32) uintptr
	ctFontCreateWithName                          func(name uintptr, size float64, m *cgAffine) uintptr
	ctFontCreateUIFontForLanguage                 func(typ uint32, size float64, lang uintptr) uintptr
	ctFontCopyFamilyName                          func(uintptr) uintptr
	ctFontCopyPostScriptName                      func(uintptr) uintptr
	ctFontGetGlyphsForCharacters                  func(f uintptr, chars *uint16, glyphs *uint16, n int64) bool
	ctFontGetSize                                 func(uintptr) float64
	ctFontCopyTraits                              func(uintptr) uintptr
	ctFontGetSymbolicTraits                       func(uintptr) uint32
	ctFontCopyVariation                           func(uintptr) uintptr
	ctFontCopyAvailableTables                     func(f uintptr, opt uint32) uintptr
	ctFontCopyTable                               func(f uintptr, tag uint32, opt uint32) uintptr
	ctFontGetUnitsPerEm                           func(uintptr) uint32
	ctFontGetBoundingRectsForGlyphs               func(f uintptr, o uint32, g *uint16, r *cgRect, n int64) cgRect
	ctFontGetAdvancesForGlyphs                    func(f uintptr, o uint32, g *uint16, a *cgSize, n int64) float64
	ctFontCreatePathForGlyph                      func(f uintptr, g uint16, m *cgAffine) uintptr
	ctFontDrawGlyphs                              func(f uintptr, g *uint16, p *cgPoint, n int64, ctx uintptr)
	ctFontCreateForString                         func(f, s uintptr, r cfRange) uintptr
	ctLineCreateWithAttributedString              func(uintptr) uintptr
	ctLineGetGlyphRuns                            func(uintptr) uintptr
	ctRunGetGlyphCount                            func(uintptr) int64
	ctRunGetGlyphs                                func(uintptr, cfRange, *uint16)
	ctRunGetPositions                             func(uintptr, cfRange, *cgPoint)
	ctRunGetAdvances                              func(uintptr, cfRange, *cgSize)
	ctRunGetStringIndices                         func(uintptr, cfRange, *int64)
	ctRunGetAttributes                            func(uintptr) uintptr
	ctFontGetAscent                               func(uintptr) float64
	ctFontGetDescent                              func(uintptr) float64

	cfStringCreateWithCharacters           func(alloc uintptr, chars *uint16, n int64) uintptr
	cfStringTokenizerCreate                func(alloc, s uintptr, r cfRange, opts uint64, locale uintptr) uintptr
	cfStringTokenizerAdvanceToNextToken    func(uintptr) uint64
	cfStringTokenizerGetCurrentTokenRange  func(uintptr) cfRange
	ctParagraphStyleCreate                 func(settings *ctParagraphStyleSetting, n uint64) uintptr
	ctTypesetterCreateWithAttributedString func(uintptr) uintptr
	ctTypesetterCreateLine                 func(ts uintptr, r cfRange) uintptr
	ctRunGetStatus                         func(uintptr) uint32
	ctRunGetStringRange                    func(uintptr) cfRange

	cgColorSpaceCreateDeviceGray               func() uintptr
	cgColorSpaceRelease                        func(uintptr)
	cgBitmapContextCreate                      func(data uintptr, w, h, bpc, bpr int64, cs uintptr, info uint32) uintptr
	cgBitmapContextGetData                     func(uintptr) unsafe.Pointer
	cgBitmapContextGetBytesPerRow              func(uintptr) int64
	cgContextRelease                           func(uintptr)
	cgContextSetGrayFillColor                  func(ctx uintptr, g, a float64)
	cgContextSetShouldAntialias                func(uintptr, bool)
	cgContextSetAllowsFontSmoothing            func(uintptr, bool)
	cgContextSetShouldSmoothFonts              func(uintptr, bool)
	cgContextSetAllowsFontSubpixelPositioning  func(uintptr, bool)
	cgContextSetShouldSubpixelPositionFonts    func(uintptr, bool)
	cgContextSetAllowsFontSubpixelQuantization func(uintptr, bool)
	cgContextSetShouldSubpixelQuantizeFonts    func(uintptr, bool)
	cgPathApply                                func(path, info, fn uintptr)
	cgPathRelease                              func(uintptr)

	kCFTypeDictKeyCB, kCFTypeDictValueCB uintptr
	kCTFontFamilyNameAttribute           uintptr
	kCTFontAttributeName                 uintptr
	kCTFontVariationAttribute            uintptr
	kCTFontWeightTrait                   uintptr
	kCTFontTraitsAttribute               uintptr
	kCTFontOpticalSizeAttribute          uintptr // 0 when the OS lacks it
	kCTParagraphStyleAttributeName       uintptr

	pathApplyCB uintptr
	pathMu      sync.Mutex
	pathCur     *paint.Path
	pathOpen    bool
)

func loadCoreText() (ok bool) {
	defer func() {
		if recover() != nil { // a missing symbol panics in RegisterLibFunc
			ok = false
		}
	}()
	open := func(p string) uintptr {
		h, err := purego.Dlopen(p, purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			panic(err)
		}
		return h
	}
	cf := open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	ct := open("/System/Library/Frameworks/CoreText.framework/CoreText")
	cg := open("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	reg := func(fn any, lib uintptr, name string) { purego.RegisterLibFunc(fn, lib, name) }
	reg(&cfRelease, cf, "CFRelease")
	reg(&cfRetain, cf, "CFRetain")
	reg(&cfAttributedStringCreate, cf, "CFAttributedStringCreate")
	reg(&cfStringCreateWithBytes, cf, "CFStringCreateWithBytes")
	reg(&cfStringGetLength, cf, "CFStringGetLength")
	reg(&cfStringGetCString, cf, "CFStringGetCString")
	reg(&cfArrayGetCount, cf, "CFArrayGetCount")
	reg(&cfArrayGetValueAtIndex, cf, "CFArrayGetValueAtIndex")
	reg(&cfDataGetLength, cf, "CFDataGetLength")
	reg(&cfDataGetBytePtr, cf, "CFDataGetBytePtr")
	reg(&cfDictionaryCreate, cf, "CFDictionaryCreate")
	reg(&cfDictionaryGetValue, cf, "CFDictionaryGetValue")
	reg(&cfDictionaryGetCount, cf, "CFDictionaryGetCount")
	reg(&cfDictionaryGetKeysValues, cf, "CFDictionaryGetKeysAndValues")
	reg(&cfNumberCreate, cf, "CFNumberCreate")
	reg(&cfNumberGetValue, cf, "CFNumberGetValue")
	reg(&cfURLCreateFromFileSystemRepresentation, cf, "CFURLCreateFromFileSystemRepresentation")
	reg(&ctFontManagerCreateFontDescriptorsFromURL, ct, "CTFontManagerCreateFontDescriptorsFromURL")
	reg(&cfDataCreate, cf, "CFDataCreate")
	reg(&ctFontManagerCreateFontDescriptorsFromData, ct, "CTFontManagerCreateFontDescriptorsFromData") // macOS 10.13

	reg(&ctFontDescriptorCreateWithAttributes, ct, "CTFontDescriptorCreateWithAttributes")
	reg(&ctFontDescriptorCreateMatchingFontDescriptors, ct, "CTFontDescriptorCreateMatchingFontDescriptors")
	reg(&ctFontCreateWithFontDescriptor, ct, "CTFontCreateWithFontDescriptor")
	reg(&ctFontCreateCopyWithAttributes, ct, "CTFontCreateCopyWithAttributes")
	reg(&ctFontCreateCopyWithSymbolicTraits, ct, "CTFontCreateCopyWithSymbolicTraits")
	reg(&ctFontCreateUIFontForLanguage, ct, "CTFontCreateUIFontForLanguage")
	reg(&ctFontCreateWithName, ct, "CTFontCreateWithName")
	reg(&ctFontCopyFamilyName, ct, "CTFontCopyFamilyName")
	reg(&ctFontCopyPostScriptName, ct, "CTFontCopyPostScriptName")
	reg(&ctFontGetGlyphsForCharacters, ct, "CTFontGetGlyphsForCharacters")
	reg(&ctFontGetSize, ct, "CTFontGetSize")
	reg(&ctFontCopyTraits, ct, "CTFontCopyTraits")
	reg(&ctFontGetSymbolicTraits, ct, "CTFontGetSymbolicTraits")
	reg(&ctFontCopyVariation, ct, "CTFontCopyVariation")
	reg(&ctFontCopyAvailableTables, ct, "CTFontCopyAvailableTables")
	reg(&ctFontCopyTable, ct, "CTFontCopyTable")
	reg(&ctFontGetUnitsPerEm, ct, "CTFontGetUnitsPerEm")
	reg(&ctFontGetBoundingRectsForGlyphs, ct, "CTFontGetBoundingRectsForGlyphs")
	reg(&ctFontCreatePathForGlyph, ct, "CTFontCreatePathForGlyph")
	reg(&ctFontGetAdvancesForGlyphs, ct, "CTFontGetAdvancesForGlyphs")
	reg(&ctFontDrawGlyphs, ct, "CTFontDrawGlyphs")
	reg(&ctFontCreateForString, ct, "CTFontCreateForString")
	reg(&ctLineCreateWithAttributedString, ct, "CTLineCreateWithAttributedString")
	reg(&ctLineGetGlyphRuns, ct, "CTLineGetGlyphRuns")
	reg(&ctRunGetGlyphCount, ct, "CTRunGetGlyphCount")
	reg(&ctRunGetGlyphs, ct, "CTRunGetGlyphs")
	reg(&ctRunGetPositions, ct, "CTRunGetPositions")
	reg(&ctRunGetAdvances, ct, "CTRunGetAdvances")
	reg(&ctRunGetStringIndices, ct, "CTRunGetStringIndices")
	reg(&ctRunGetAttributes, ct, "CTRunGetAttributes")
	reg(&ctFontGetAscent, ct, "CTFontGetAscent")
	reg(&ctFontGetDescent, ct, "CTFontGetDescent")
	reg(&cfStringCreateWithCharacters, cf, "CFStringCreateWithCharacters")
	reg(&cfStringTokenizerCreate, cf, "CFStringTokenizerCreate")
	reg(&cfStringTokenizerAdvanceToNextToken, cf, "CFStringTokenizerAdvanceToNextToken")
	reg(&cfStringTokenizerGetCurrentTokenRange, cf, "CFStringTokenizerGetCurrentTokenRange")
	reg(&ctParagraphStyleCreate, ct, "CTParagraphStyleCreate")
	reg(&ctTypesetterCreateWithAttributedString, ct, "CTTypesetterCreateWithAttributedString")
	reg(&ctTypesetterCreateLine, ct, "CTTypesetterCreateLine")
	reg(&ctRunGetStatus, ct, "CTRunGetStatus")
	reg(&ctRunGetStringRange, ct, "CTRunGetStringRange")

	reg(&cgColorSpaceCreateDeviceGray, cg, "CGColorSpaceCreateDeviceGray")
	reg(&cgColorSpaceRelease, cg, "CGColorSpaceRelease")
	reg(&cgBitmapContextCreate, cg, "CGBitmapContextCreate")
	reg(&cgBitmapContextGetData, cg, "CGBitmapContextGetData")
	reg(&cgBitmapContextGetBytesPerRow, cg, "CGBitmapContextGetBytesPerRow")
	reg(&cgContextRelease, cg, "CGContextRelease")
	reg(&cgContextSetGrayFillColor, cg, "CGContextSetGrayFillColor")
	reg(&cgContextSetShouldAntialias, cg, "CGContextSetShouldAntialias")
	reg(&cgContextSetAllowsFontSmoothing, cg, "CGContextSetAllowsFontSmoothing")
	reg(&cgContextSetShouldSmoothFonts, cg, "CGContextSetShouldSmoothFonts")
	reg(&cgContextSetAllowsFontSubpixelPositioning, cg, "CGContextSetAllowsFontSubpixelPositioning")
	reg(&cgContextSetShouldSubpixelPositionFonts, cg, "CGContextSetShouldSubpixelPositionFonts")
	reg(&cgContextSetAllowsFontSubpixelQuantization, cg, "CGContextSetAllowsFontSubpixelQuantization")
	reg(&cgContextSetShouldSubpixelQuantizeFonts, cg, "CGContextSetShouldSubpixelQuantizeFonts")
	reg(&cgPathApply, cg, "CGPathApply")
	reg(&cgPathRelease, cg, "CGPathRelease")

	sym := func(lib uintptr, name string) uintptr {
		p, err := purego.Dlsym(lib, name)
		if err != nil {
			panic(err)
		}
		return p
	}
	val := func(lib uintptr, name string) uintptr {
		p := sym(lib, name)
		return **(**uintptr)(unsafe.Pointer(&p)) // the variable's value, not its address
	}
	kCFTypeDictKeyCB = sym(cf, "kCFTypeDictionaryKeyCallBacks")
	kCFTypeDictValueCB = sym(cf, "kCFTypeDictionaryValueCallBacks")
	kCTFontFamilyNameAttribute = val(ct, "kCTFontFamilyNameAttribute")
	kCTFontAttributeName = val(ct, "kCTFontAttributeName")
	kCTFontVariationAttribute = val(ct, "kCTFontVariationAttribute")
	kCTFontWeightTrait = val(ct, "kCTFontWeightTrait")
	kCTFontTraitsAttribute = val(ct, "kCTFontTraitsAttribute")
	kCTParagraphStyleAttributeName = val(ct, "kCTParagraphStyleAttributeName")
	if p, err := purego.Dlsym(ct, "kCTFontOpticalSizeAttribute"); err == nil {
		kCTFontOpticalSizeAttribute = **(**uintptr)(unsafe.Pointer(&p))
	}
	pathApplyCB = purego.NewCallback(pathApplier)
	return true
}

// cgPathElement is CGPathElement.
type cgPathElement struct {
	Type   int32
	_      int32
	Points *cgPoint
}

// pathApplier is the CGPathApplierFunction collecting a glyph outline into
// pathCur (pathMu held by the caller of CGPathApply).
func pathApplier(info uintptr, e *cgPathElement) {
	p := pathCur
	pts := unsafe.Slice(e.Points, 3)
	switch e.Type {
	case 0: // moveTo
		if pathOpen {
			p.Close()
		}
		p.MoveTo(float32(pts[0].X), float32(-pts[0].Y))
		pathOpen = true
	case 1:
		p.LineTo(float32(pts[0].X), float32(-pts[0].Y))
	case 2:
		p.QuadTo(float32(pts[0].X), float32(-pts[0].Y), float32(pts[1].X), float32(-pts[1].Y))
	case 3:
		p.CubeTo(float32(pts[0].X), float32(-pts[0].Y), float32(pts[1].X), float32(-pts[1].Y), float32(pts[2].X), float32(-pts[2].Y))
	case 4:
		if pathOpen {
			p.Close()
			pathOpen = false
		}
	}
}

func cfStr(s string) uintptr {
	b := []byte(s)
	if len(b) == 0 {
		b = []byte{0}
		return cfStringCreateWithBytes(0, &b[0], 0, cfStringEncodingUTF8, false)
	}
	return cfStringCreateWithBytes(0, &b[0], int64(len(b)), cfStringEncodingUTF8, false)
}

func goStr(s uintptr) string {
	if s == 0 {
		return ""
	}
	n := cfStringGetLength(s)*4 + 1
	buf := make([]byte, n)
	if !cfStringGetCString(s, &buf[0], n, cfStringEncodingUTF8) {
		return ""
	}
	for i, c := range buf {
		if c == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

func cfFloat(n uintptr) (float64, bool) {
	var v float64
	if n == 0 || !cfNumberGetValue(n, cfNumberFloat64Type, unsafe.Pointer(&v)) {
		return 0, false
	}
	return v, true
}

func cfDict(keys, vals []uintptr) uintptr {
	return cfDictionaryCreate(0, &keys[0], &vals[0], int64(len(keys)), kCFTypeDictKeyCB, kCFTypeDictValueCB)
}
