//go:build darwin && !ios && tango_coretext

package text

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/tangoui/tango/ui/paint"
)

// Faces backed by CoreText: every Face wraps a CTFont (its ctFontOwner) and
// a ctRaster, which draws its glyphs and supplies their outlines. Fonts
// registered with RegisterFont are read from memory
// (CTFontManagerCreateFontDescriptorsFromData) without registering them
// with the process font manager; system faces come from ctEngine.load and
// from font files (sysFont.family); fonts CoreText picks on its own map to
// a Face through faceForCTFont.

// ctRasterImpl is the CoreText part of ctRaster: CoreText shapes from the
// CTFont itself, so the raster keeps nothing more.
type ctRasterImpl struct{}

// faceImpl is the engine part of Face.
type faceImpl struct {
	own    *ctFontOwner // the CTFont; nil until built (or when the build failed)
	upem   float32
	asc    float32 // font units, positive
	desc   float32 // font units, positive
	italic bool    // the CTFont has the italic trait (a designed italic or oblique)
	user   bool    // registered with RegisterFont: kept in the Face table across TrimFontCaches

	// Lazily built faces (newCTFace): returns the raster, nil when the
	// face cannot be built.
	build func() *ctRaster
}

// ready builds a lazy face on first use; a no-op for faces built eagerly.
func (f *Face) ready() {
	f.once.Do(func() {
		build := f.build
		if build == nil {
			return
		}
		f.build = nil
		r := build()
		if r == nil || !f.setRaster(r) {
			f.broken = true
			return
		}
		registerCTFace(ctFaceKey(f.own.base), f)
	})
}

// setRaster makes r the face's font, reading the metrics and traits from
// its CTFont; false when the font is unusable.
func (f *Face) setRaster(r *ctRaster) bool {
	defer runtime.KeepAlive(r)
	base := r.base
	if base == 0 || r.upem <= 0 {
		return false
	}
	f.ras, f.own, f.upem = r, r.ctFontOwner, r.upem
	size := ctFontGetSize(base)
	if size <= 0 {
		size = 16
	}
	f.asc = float32(ctFontGetAscent(base)/size) * f.upem
	f.desc = float32(ctFontGetDescent(base)/size) * f.upem
	if f.asc <= 0 && f.desc <= 0 {
		f.asc, f.desc = f.upem*0.8, f.upem*0.2
	}
	f.italic = ctFontGetSymbolicTraits(base)&ctFontItalicTrait != 0
	return true
}

// newCTFace is a face of family built on first use from owner's CTFont;
// vals (optional) pins the variation every sized copy uses.
func newCTFace(family string, w Weight, italic bool, owner *ctFontOwner, vals map[uint32]float32) *Face {
	return &Face{Family: family, Weight: w, Italic: italic, paths: map[uint32]*paint.Path{}, faceImpl: faceImpl{
		build: func() *ctRaster { return newCTRasterOwned(owner, vals) },
	}}
}

// ctRaster returns the face's raster (nil when the face cannot be built).
func (f *Face) ctRaster() *ctRaster {
	f.ready()
	r, _ := f.ras.(*ctRaster)
	return r
}

// ctFont returns the face's CTFont at size (pt = px), for shaping: the same
// copy ctRaster draws with (ctRaster.font). The reference is retained: the
// caller releases it with cfRelease. 0 when the face cannot be built.
func (f *Face) ctFont(size float32) uintptr {
	r := f.ctRaster()
	if r == nil {
		return 0
	}
	defer runtime.KeepAlive(r)
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.font(size, 0)
	if c != 0 {
		cfRetain(c)
	}
	return c
}

// metricsUnits returns the units per em and the ascent and descent (font
// units, positive), building a lazy face first.
func (f *Face) metricsUnits() (upem, asc, desc float32) {
	f.ready()
	if f.own == nil {
		return 1000, 800, 200
	}
	return f.upem, f.asc, f.desc
}

// hasRune reports whether the face maps r.
func (f *Face) hasRune(r rune) bool {
	_, ok := f.glyphFor(r)
	return ok
}

// glyphFor returns the glyph the face's character map gives r (false when
// unmapped: glyph 0).
func (f *Face) glyphFor(r rune) (uint32, bool) {
	f.ready()
	if f.own == nil {
		return 0, false
	}
	defer runtime.KeepAlive(f)
	var u, g [2]uint16
	n := int64(1)
	switch {
	case r < 0 || r > 0x10FFFF || r >= 0xD800 && r <= 0xDFFF:
		return 0, false
	case r >= 0x10000:
		hi, lo := utf16.EncodeRune(r)
		u[0], u[1], n = uint16(hi), uint16(lo), 2
	default:
		u[0] = uint16(r)
	}
	if !ctFontGetGlyphsForCharacters(f.own.base, &u[0], &g[0], n) || g[0] == 0 {
		return 0, false
	}
	return uint32(g[0]), true
}

// unitOutline returns the outline of glyph id in font units, y down (nil
// when the glyph has none).
func (f *Face) unitOutline(id uint32) *paint.Path {
	r := f.ctRaster()
	if r == nil {
		return nil
	}
	return r.unitPath(id)
}

// ctLoaded loads the CoreText bindings; false when they are unavailable.
func ctLoaded() bool {
	ctOnce.Do(func() { ctOK = loadCoreText() })
	return ctOK
}

// parseFaces reads a font file (TTF, OTF, TTC) held in memory
// (RegisterFont). Data CoreText cannot read is an error.
func parseFaces(name string, data []byte) ([]*Face, error) {
	if !ctLoaded() {
		return nil, errors.New("text: CoreText unavailable")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("text: parse font %s: no data", name)
	}
	d := cfDataCreate(0, &data[0], int64(len(data)))
	if d == 0 {
		return nil, fmt.Errorf("text: parse font %s: out of memory", name)
	}
	arr := ctFontManagerCreateFontDescriptorsFromData(d)
	cfRelease(d)
	if arr == 0 {
		return nil, fmt.Errorf("text: parse font %s: CoreText cannot read the font data", name)
	}
	fonts := fontsFromDescriptors(arr)
	cfRelease(arr)
	if len(fonts) == 0 {
		return nil, fmt.Errorf("text: parse font %s: CoreText cannot read the font data", name)
	}
	faces := ctFacesFromFonts(name, fonts, nil)
	for _, f := range faces {
		f.user = true
	}
	return faces, nil
}

// fontsFromDescriptors creates a 16pt CTFont for every descriptor of arr.
func fontsFromDescriptors(arr uintptr) []uintptr {
	var out []uintptr
	for i := int64(0); i < cfArrayGetCount(arr); i++ {
		if f := ctFontCreateWithFontDescriptor(cfArrayGetValueAtIndex(arr, i), 16, nil); f != 0 {
			out = append(out, f)
		}
	}
	return out
}

// ctFacesFromFonts makes the faces of family name from fonts, taking
// ownership of every font. keep, when not nil, lists the CoreText families
// to keep. A variable font with a wght axis is instantiated at the weights
// 300-700 it supports, its other axes at their defaults; the first face of
// each (weight, slant) wins.
func ctFacesFromFonts(name string, fonts []uintptr, keep map[string]bool) []*Face {
	seen := map[faceKey]bool{}
	var out []*Face
	add := func(f uintptr, w Weight, italic bool, vals map[uint32]float32) {
		if seen[faceKey{w, italic}] {
			cfRelease(f)
			return
		}
		seen[faceKey{w, italic}] = true
		out = append(out, newCTFace(name, w, italic, newCTFontOwner(f, cfRelease), vals))
	}
	for _, f := range fonts {
		if keep != nil {
			fn := ctFontCopyFamilyName(f)
			fam := goStr(fn)
			if fn != 0 {
				cfRelease(fn)
			}
			if !keep[fam] {
				cfRelease(f)
				continue
			}
		}
		italic := ctFontGetSymbolicTraits(f)&ctFontItalicTrait != 0
		if lo, hi, ok := ctAxisRange(f, wghtTag); ok {
			n := 0
			for _, w := range []Weight{Light, Regular, Medium, Semibold, Bold} {
				if float32(w) < lo || float32(w) > hi || seen[faceKey{w, italic}] {
					continue
				}
				vals := map[uint32]float32{wghtTag: float32(w)}
				if inst := withVariation(f, vals); inst != 0 {
					add(inst, w, italic, vals)
					n++
				}
			}
			if n > 0 {
				cfRelease(f)
				continue
			}
		}
		add(f, ctWeight(f), italic, nil)
	}
	return out
}

// ctAxisRange returns the minimum and maximum of variation axis tag of f
// (its fvar table); false when f has no such axis.
func ctAxisRange(f uintptr, tag uint32) (lo, hi float32, ok bool) {
	d := ctFontCopyTable(f, tag4("fvar"), 0)
	if d == 0 {
		return 0, 0, false
	}
	defer cfRelease(d)
	n := cfDataGetLength(d)
	if n < 16 {
		return 0, 0, false
	}
	b := unsafe.Slice((*byte)(cfDataGetBytePtr(d)), n)
	u16 := func(p int) int { return int(b[p])<<8 | int(b[p+1]) }
	fixed := func(p int) float32 {
		return float32(int32(uint32(b[p])<<24|uint32(b[p+1])<<16|uint32(b[p+2])<<8|uint32(b[p+3]))) / 65536
	}
	off, cnt, size := u16(4), u16(8), u16(10)
	if size < 16 {
		return 0, 0, false
	}
	for i := 0; i < cnt; i++ {
		p := off + i*size
		if p+16 > len(b) {
			break
		}
		if uint32(b[p])<<24|uint32(b[p+1])<<16|uint32(b[p+2])<<8|uint32(b[p+3]) == tag {
			return fixed(p + 4), fixed(p + 12), true
		}
	}
	return 0, 0, false
}

func (s sysFont) family() *family {
	return &family{name: s.name, load: func() []*Face {
		if !ctLoaded() {
			return nil
		}
		var keep map[string]bool
		if len(s.families) > 0 {
			keep = map[string]bool{}
			for _, k := range s.families {
				keep[k] = true
			}
		}
		for _, p := range s.paths {
			if p == "" {
				continue
			}
			faces := ctFacesFromFonts(s.name, fontsFromFiles(p), keep)
			if len(faces) == 0 {
				continue
			}
			for _, x := range s.extra {
				faces = append(faces, ctFacesFromFonts(s.name, fontsFromFiles(x), keep)...)
			}
			return faces
		}
		return nil
	}}
}

// newCTLazyFaces returns the constructor of the lazy faces of family name
// (ctEngine.load). A variable face keeps the axis values CoreText reports
// with the optical size pinned (16 unless set), so every point size draws
// one instance.
func newCTLazyFaces(name string) func(w Weight, italic bool, owner *ctFontOwner) *Face {
	return func(w Weight, italic bool, owner *ctFontOwner) *Face {
		return &Face{Family: name, Weight: w, Italic: italic, paths: map[uint32]*paint.Path{}, faceImpl: faceImpl{
			build: func() *ctRaster { return ctEngineRaster(owner) },
		}}
	}
}

func ctEngineRaster(owner *ctFontOwner) *ctRaster {
	defer runtime.KeepAlive(owner)
	f := owner.base
	if f == 0 {
		return nil
	}
	var vals map[uint32]float32
	if axes := fvarAxesOfCT(f); len(axes) > 0 {
		vals = ctVariation(f)
		if vals == nil {
			vals = map[uint32]float32{}
		}
		for _, a := range axes {
			if a == opszTag {
				if _, ok := vals[opszTag]; !ok {
					vals[opszTag] = 16
				}
			}
		}
		if len(vals) > 0 {
			if nf := withVariation(f, vals); nf != 0 {
				owner.base = nf
				cfRelease(f)
			}
		}
	}
	return newCTRasterOwned(owner, vals)
}

// The Face table maps CoreText fonts to Faces: faces register when built,
// and faceForCTFont maps the fonts CoreText picks itself (a shaped run's
// font) to them, or to new faces. TrimFontCaches drops every entry but
// the faces of RegisterFont; faces already handed out stay usable.
var ctFaces struct {
	mu    sync.Mutex
	byKey map[string]*Face
}

// registerCTFace records f under key unless another face holds it; it
// returns the face the table holds.
func registerCTFace(key string, f *Face) *Face {
	ctFaces.mu.Lock()
	defer ctFaces.mu.Unlock()
	if old, ok := ctFaces.byKey[key]; ok {
		return old
	}
	if ctFaces.byKey == nil {
		ctFaces.byKey = map[string]*Face{}
	}
	ctFaces.byKey[key] = f
	return f
}

// trimFaceTable drops the Face table but for registered fonts
// (TrimFontCaches).
func trimFaceTable() {
	ctFaces.mu.Lock()
	defer ctFaces.mu.Unlock()
	for k, f := range ctFaces.byKey {
		if !f.user {
			delete(ctFaces.byKey, k)
		}
	}
}

// ctFaceKey identifies a CoreText font independently of its size: its
// PostScript name, weight, slant and variation coordinates.
func ctFaceKey(f uintptr) string {
	n := ctFontCopyPostScriptName(f)
	var b strings.Builder
	b.WriteString(goStr(n))
	if n != 0 {
		cfRelease(n)
	}
	b.WriteByte(0)
	b.WriteString(strconv.Itoa(int(ctWeight(f))))
	if ctFontGetSymbolicTraits(f)&ctFontItalicTrait != 0 {
		b.WriteString("i")
	}
	vals := ctVariation(f)
	tags := make([]uint32, 0, len(vals))
	for t := range vals {
		tags = append(tags, t)
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i] < tags[j] })
	for _, t := range tags {
		b.WriteByte(0)
		b.WriteString(strconv.FormatUint(uint64(t), 16))
		b.WriteByte('=')
		b.WriteString(strconv.FormatFloat(float64(vals[t]), 'g', 6, 32))
	}
	return b.String()
}

// faceForCTFont returns the Face of CoreText font f (any size): the same
// *Face for the same font until TrimFontCaches. f stays owned by the
// caller. nil when f is 0 or unusable.
func faceForCTFont(f uintptr) *Face {
	if f == 0 {
		return nil
	}
	key := ctFaceKey(f)
	ctFaces.mu.Lock()
	fc, ok := ctFaces.byKey[key]
	ctFaces.mu.Unlock()
	if ok {
		return fc
	}
	c := ctFontCreateCopyWithAttributes(f, 16, nil, 0)
	if c == 0 {
		return nil
	}
	fn := ctFontCopyFamilyName(c)
	fc = &Face{Family: goStr(fn), Weight: ctWeight(c), paths: map[uint32]*paint.Path{}}
	if fn != 0 {
		cfRelease(fn)
	}
	fc.Italic = ctFontGetSymbolicTraits(c)&ctFontItalicTrait != 0
	if !fc.setRaster(newCTRasterOwned(newCTFontOwner(c, cfRelease), ctVariation(f))) {
		return nil
	}
	return registerCTFace(key, fc)
}
