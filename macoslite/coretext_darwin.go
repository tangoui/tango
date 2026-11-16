//go:build darwin

package macoslite

import (
	"strings"
	"unicode/utf8"
	"unsafe"
)

// fontCacheSize bounds the CTFonts kept between frames.
const fontCacheSize = 8

// kCTFontUIFontSystem selects the system UI font (SF Pro).
const kCTFontUIFontSystem = 2

// fontCache keeps owned CTFonts by point size, least recently used evicted.
// Main thread only.
type fontCache struct {
	entries []fontEntry   // most recently used last
	release func(uintptr) // nil means cfRelease; tests count releases
}

type fontEntry struct {
	size float64
	font uintptr // owned
}

func (f *fontCache) len() int { return len(f.entries) }

func (f *fontCache) drop(h uintptr) {
	if f.release != nil {
		f.release(h)
	} else {
		cfRelease(h)
	}
}

// get returns a cached font of the given point size, creating it if needed.
// The cache keeps ownership; the font stays valid until evicted or cleared.
func (f *fontCache) get(size float64) uintptr {
	for i, e := range f.entries {
		if e.size == size {
			copy(f.entries[i:], f.entries[i+1:])
			f.entries[len(f.entries)-1] = e
			return e.font
		}
	}
	h := ctFontCreateUIFontForLanguage(kCTFontUIFontSystem, size, 0)
	if h == 0 {
		return 0
	}
	if len(f.entries) == fontCacheSize {
		f.drop(f.entries[0].font)
		f.entries = append(f.entries[:0], f.entries[1:]...)
	}
	f.entries = append(f.entries, fontEntry{size, h})
	return h
}

// clear releases every cached font.
func (f *fontCache) clear() {
	for _, e := range f.entries {
		f.drop(e.font)
	}
	f.entries = nil
}

// Text draws s with its baseline-left at origin (points), in the system UI
// font at size points. CoreText picks fallback fonts (CJK, symbols) itself.
// Invalid UTF-8 is drawn with replacement characters.
func (c *Canvas) Text(s string, origin Point, size float64, col Color) {
	if s == "" || col.A == 0 || !(size > 0) || c.fonts == nil {
		return
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	font := c.fonts.get(size)
	if font == 0 {
		return
	}
	str := cfString(s)
	if str == 0 {
		return
	}
	defer cfRelease(str)
	a := float64(col.A) / 255
	color := cgColorCreateSRGB(float64(col.R)/255, float64(col.G)/255, float64(col.B)/255, a)
	if color == 0 {
		return
	}
	defer cgColorRelease(color)
	keys := [2]uintptr{kCTFontAttributeName, kCTForegroundColorAttributeName}
	vals := [2]uintptr{font, color}
	attrs := cfDictionaryCreate(0, &keys[0], &vals[0], 2, kCFTypeDictionaryKeyCallBacks, kCFTypeDictionaryValueCallBacks)
	if attrs == 0 {
		return
	}
	defer cfRelease(attrs)
	as := cfAttributedStringCreate(0, str, attrs)
	if as == 0 {
		return
	}
	defer cfRelease(as)
	line := ctLineCreateWithAttributedString(as)
	if line == 0 {
		return
	}
	defer cfRelease(line)

	// Draw straight into the frame's pixels. The bitmap is premultiplied
	// RGBA like the image, and CoreGraphics composites source-over onto
	// what Fill and RoundRect painted.
	img := c.img
	w, h := img.Rect.Dx(), img.Rect.Dy()
	space := cgColorSpaceCreateWithName(kCGColorSpaceSRGB)
	if space == 0 {
		return
	}
	defer cgColorSpaceRelease(space)
	ctx := cgBitmapContextCreate(unsafe.Pointer(&img.Pix[0]), w, h, 8, img.Stride, space,
		kCGImageAlphaPremultipliedLast|kCGBitmapByteOrder32Big)
	if ctx == 0 {
		return
	}
	defer cgContextRelease(ctx)
	// CoreGraphics has a bottom-left origin in pixels; text keeps its own
	// upright orientation, so place the baseline by flipping y only.
	cgContextScaleCTM(ctx, c.scale, c.scale)
	cgContextSetTextPosition(ctx, origin.X, float64(h)/c.scale-origin.Y)
	ctLineDraw(line, ctx)
}
