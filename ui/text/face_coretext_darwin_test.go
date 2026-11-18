//go:build darwin && !ios && tango_coretext

package text

import (
	"os"
	"testing"
)

// saveUserFonts restores the registered families after the test: they go
// first in every chain.
func saveUserFonts(t *testing.T) {
	t.Helper()
	reg.mu.Lock()
	saved := append([]*family(nil), reg.user...)
	reg.mu.Unlock()
	t.Cleanup(func() {
		reg.mu.Lock()
		reg.user = saved
		reg.version++
		reg.mu.Unlock()
		clearCache()
	})
}

func readTestFont(t *testing.T) []byte {
	t.Helper()
	for _, p := range []string{"/System/Library/Fonts/Geneva.ttf", "/System/Library/Fonts/Monaco.ttf", "/System/Library/Fonts/Supplemental/Arial.ttf"} {
		if b, err := os.ReadFile(p); err == nil {
			return b
		}
	}
	t.Skip("no TTF test font on this machine")
	return nil
}

func userFamily(t *testing.T, name string) *family {
	t.Helper()
	reg.mu.Lock()
	defer reg.mu.Unlock()
	for _, fm := range reg.user {
		if fm.name == name {
			return fm
		}
	}
	t.Fatalf("family %q not registered", name)
	return nil
}

func TestCTRegisterFontFromData(t *testing.T) {
	saveUserFonts(t)
	if err := RegisterFont("T", readTestFont(t)); err != nil {
		t.Fatal(err)
	}
	if fams := Families(); len(fams) == 0 || fams[0] != "T" {
		t.Fatalf("Families() = %v, want T first", fams)
	}
	f, _ := userFamily(t, "T").pick(Regular, false)
	if f == nil {
		t.Fatal("no face")
	}
	if !f.HasRune('A') {
		t.Error("HasRune('A') = false")
	}
	if f.HasRune(0x10FFFD) {
		t.Error("HasRune(U+10FFFD) = true")
	}
	asc, desc := f.Metrics(16)
	if asc <= 0 || desc <= 0 {
		t.Errorf("Metrics(16) = %v, %v; want both > 0", asc, desc)
	}
	if !f.SystemRasterized() {
		t.Error("SystemRasterized() = false for a CoreText face")
	}
}

func TestCTRegisterFontRejectsGarbage(t *testing.T) {
	saveUserFonts(t)
	if err := RegisterFont("bad", []byte("not a font")); err == nil {
		t.Fatal("RegisterFont accepted garbage")
	}
	if err := RegisterFont("empty", nil); err == nil {
		t.Fatal("RegisterFont accepted empty data")
	}
}

func TestCTRegisterTTCAllFaces(t *testing.T) {
	// PingFang.ttc first; current macOS keeps it outside /System/Library/Fonts,
	// so other multi-weight collections stand in.
	var data []byte
	for _, p := range []string{
		"/System/Library/Fonts/PingFang.ttc",
		"/System/Library/Fonts/Hiragino Sans GB.ttc",
		"/System/Library/Fonts/HelveticaNeue.ttc",
		"/System/Library/Fonts/Helvetica.ttc",
	} {
		if b, err := os.ReadFile(p); err == nil {
			data = b
			break
		}
	}
	if data == nil {
		t.Skip("no font collection on this machine")
	}
	saveUserFonts(t)
	if err := RegisterFont("PF", data); err != nil {
		t.Fatal(err)
	}
	fm := userFamily(t, "PF")
	weights := map[Weight]bool{}
	for _, f := range fm.faces {
		weights[f.Weight] = true
	}
	if len(fm.faces) < 2 || len(weights) < 2 {
		t.Fatalf("faces = %d, weights = %v; want several weights", len(fm.faces), weights)
	}
	f, _ := fm.pick(Regular, false)
	if f == nil || !f.HasRune('A') {
		t.Fatal("the Regular face does not map A")
	}
}

func TestCTRegisterVariableWeights(t *testing.T) {
	data, err := os.ReadFile("/System/Library/Fonts/SFNS.ttf")
	if err != nil {
		t.Skip("no SFNS.ttf on this machine")
	}
	saveUserFonts(t)
	if err := RegisterFont("V", data); err != nil {
		t.Fatal(err)
	}
	got := map[Weight]bool{}
	for _, f := range userFamily(t, "V").faces {
		got[f.Weight] = true
	}
	for _, w := range []Weight{Light, Regular, Medium, Semibold, Bold} {
		if !got[w] {
			t.Errorf("no %d instance (have %v)", w, got)
		}
	}
	light, _ := userFamily(t, "V").pick(Light, false)
	bold, _ := userFamily(t, "V").pick(Bold, false)
	id := ctGlyph(t, light, 'H')
	if a, b := light.ctRaster().advance(id, 16), bold.ctRaster().advance(id, 16); a >= b {
		t.Errorf("H advance: light %v, bold %v; want bold wider", a, b)
	}
}

func TestCTHasRuneAstral(t *testing.T) {
	f := faceForCTFont(ctTestFont(t, "Apple Color Emoji"))
	if !f.HasRune('😀') {
		t.Error("Apple Color Emoji does not map U+1F600")
	}
	if f.HasRune('中') {
		t.Error("Apple Color Emoji maps 中")
	}
}

func TestCTGlyphPathMatchesAdvanceBox(t *testing.T) {
	saveUserFonts(t)
	if err := RegisterFont("T", readTestFont(t)); err != nil {
		t.Fatal(err)
	}
	f, _ := userFamily(t, "T").pick(Regular, false)
	r := f.ctRaster()
	if r == nil {
		t.Fatal("no CoreText raster")
	}
	id := ctGlyph(t, f, 'H')
	p := f.GlyphPath(id, 16)
	if p == nil || len(p.Segs) == 0 {
		t.Fatal("empty path for H")
	}
	b := p.Bounds()
	if b.H <= 0 || b.H >= 16 {
		t.Errorf("H height = %v, want in (0, 16)", b.H)
	}
	if b.Y >= 0 {
		t.Errorf("H top = %v, want negative (y down)", b.Y)
	}
	if _, _, ok := f.RasterGlyph(id, 16, 0, 0); !ok {
		t.Error("RasterGlyph failed")
	}
}

func TestCTFaceDedup(t *testing.T) {
	f := ctTestFont(t, "Menlo")
	a, b := faceForCTFont(f), faceForCTFont(f)
	if a == nil || a != b {
		t.Fatalf("faceForCTFont: %p, %p; want the same face", a, b)
	}
	// A different size of the same font is the same face.
	g := ctFontCreateCopyWithAttributes(f, 31, nil, 0)
	defer cfRelease(g)
	if c := faceForCTFont(g); c != a {
		t.Errorf("faceForCTFont at another size: %p, want %p", c, a)
	}
	if a.Family != "Menlo" || a.Weight != Regular || a.Italic {
		t.Errorf("face = %q %d italic=%v", a.Family, a.Weight, a.Italic)
	}
	if !a.HasRune('m') {
		t.Error("Menlo does not map m")
	}
	sz := a.ctFont(13)
	if sz == 0 {
		t.Fatal("ctFont(13) = 0")
	}
	defer cfRelease(sz)
	if got := faceForCTFont(sz); got != a {
		t.Errorf("faceForCTFont(ctFont(13)) = %p, want %p", got, a)
	}
}

func TestCTTrimFontCachesKeepsLiveFaces(t *testing.T) {
	f := ctTestFont(t, "Menlo")
	old := faceForCTFont(f)
	id := ctGlyph(t, old, 'g')
	TrimFontCaches()
	if nf := faceForCTFont(f); nf == old {
		t.Error("TrimFontCaches kept the dedup table")
	}
	if m, _, ok := old.RasterGlyph(id, 15, 0, 0); !ok || m == nil {
		t.Error("old face no longer rasterizes after TrimFontCaches")
	}
	if p := old.GlyphPath(id, 15); p == nil {
		t.Error("old face lost its outlines after TrimFontCaches")
	}
}

// ctTestFont returns a CTFont of family name at 16pt (released after the
// test).
func ctTestFont(t *testing.T, name string) uintptr {
	t.Helper()
	if openCoreText() == nil {
		t.Skip("CoreText unavailable")
	}
	s := cfStr(name)
	defer cfRelease(s)
	f := ctFontCreateWithName(s, 16, nil)
	if f == 0 {
		t.Skipf("no %s", name)
	}
	t.Cleanup(func() { cfRelease(f) })
	fn := ctFontCopyFamilyName(f)
	got := goStr(fn)
	cfRelease(fn)
	if got != name {
		t.Skipf("%s resolved to %s", name, got)
	}
	return f
}

// ctGlyph is the glyph id face f maps r to.
func ctGlyph(t *testing.T, f *Face, r rune) uint32 {
	t.Helper()
	id, ok := f.glyphFor(r)
	if !ok {
		t.Fatalf("no glyph for %q", r)
	}
	return id
}
