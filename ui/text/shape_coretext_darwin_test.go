//go:build darwin && !ios && tango_coretext

package text

import (
	"math"
	"os"
	"testing"

	"github.com/ebitengine/purego"
)

// ctAdvances returns the per-rune advances the shared pipeline computes
// for s (one paragraph) with st.
func ctAdvances(t *testing.T, s string, st Style) []float32 {
	t.Helper()
	st = st.norm()
	mu.Lock()
	defer mu.Unlock()
	p := prepareText(s, []runStyle{{st: st, chain: reg.chain(st.Mono), lang: resolveLang(st)}}, nil, nil, Options{NoWrap: true})
	return p.adv[:len(p.runes)]
}

func needCT(t *testing.T) {
	t.Helper()
	if !ctLoaded() {
		t.Skip("CoreText unavailable")
	}
	if fm := (&fontmap{chain: reg.chain(false)}); fm.primary() == nil {
		t.Skip("no system font")
	}
}

func glyphFace(t *testing.T, l *Layout, g int) *Face {
	t.Helper()
	f, ok := l.Glyphs[g].Src.(*Face)
	if !ok || f == nil {
		t.Fatalf("glyph %d: Src %T is not a *Face", g, l.Glyphs[g].Src)
	}
	return f
}

func ctNear(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

func TestCTShapeBasicLatin(t *testing.T) {
	needCT(t)
	st := Style{Size: 16}
	l := Shape("Hello", st, Options{})
	if len(l.Glyphs) != 5 {
		t.Fatalf("len(Glyphs) = %d, want 5", len(l.Glyphs))
	}
	for i, c := range l.GlyphCluster {
		if c != int32(i) {
			t.Fatalf("GlyphCluster = %v, want [0 1 2 3 4]", l.GlyphCluster)
		}
	}
	var sum float32
	for _, a := range ctAdvances(t, "Hello", st) {
		sum += a
	}
	if l.Width <= 0 || !ctNear(l.Width, sum) {
		t.Errorf("Width = %v, sum of rune advances %v", l.Width, sum)
	}
}

func TestCTShapeKerningAffectsWidth(t *testing.T) {
	needCT(t)
	st := Style{Size: 32}
	av, a, v := Width("AV", st), Width("A", st), Width("V", st)
	if !(av < a+v) {
		t.Errorf("Width(AV) = %v, Width(A)+Width(V) = %v: no kerning", av, a+v)
	}
}

func TestCTShapeNoLigatures(t *testing.T) {
	needCT(t)
	c := NewLineCache(8)
	runs := []Run{{0, 3, Style{Size: 16}}}
	off := c.Shape("ffi", runs, LineOptions{Ligatures: false})
	if len(off.Glyphs) != 3 {
		t.Errorf("Ligatures:false: %d glyphs for ffi, want 3", len(off.Glyphs))
	}
	on := c.Shape("ffi", runs, LineOptions{Ligatures: true})
	if n := len(on.Glyphs); n < 1 || n > 3 {
		t.Errorf("Ligatures:true: %d glyphs for ffi, want 1..3", n)
	}
}

// noLiga shapes with a font copy whose feature settings turn liga, clig
// and calt off (kCTLigatureAttributeName alone leaves calt on).
func TestCTNoLigaFontFeatureSettings(t *testing.T) {
	needCT(t)
	if !loadCTShape() {
		t.Skip("CoreText shaping symbols unavailable")
	}
	// CTFontCopyFeatureSettings lists only features the font has, so use
	// the system sans (SF Pro has liga, clig and calt), not SF Mono.
	f := (&fontmap{chain: reg.chain(false), w: Regular}).primary()
	if f == nil {
		t.Fatal("no sans face")
	}
	fc := noLigaFont(f, 13)
	if fc == 0 {
		t.Fatal("noLigaFont = 0")
	}
	defer cfRelease(fc)
	if again := noLigaFont(f, 13); again != fc {
		t.Errorf("noLigaFont not cached: %#x, %#x", again, fc)
		cfRelease(again)
	} else {
		cfRelease(again)
	}
	settings := func(arr uintptr) map[string]float64 {
		got := map[string]float64{}
		for i := int64(0); i < cfArrayGetCount(arr); i++ {
			d := cfArrayGetValueAtIndex(arr, i)
			tag := goStr(cfDictionaryGetValue(d, kCTFontOpenTypeFeatureTag))
			if v, ok := cfFloat(cfDictionaryGetValue(d, kCTFontOpenTypeFeatureValue)); ok && tag != "" {
				got[tag] = v
			}
		}
		return got
	}
	// The descriptor asks for all three off.
	ct, err := purego.Dlopen("/System/Library/Frameworks/CoreText.framework/CoreText", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		t.Fatal(err)
	}
	var copyAttr func(desc, name uintptr) uintptr
	purego.RegisterLibFunc(&copyAttr, ct, "CTFontDescriptorCopyAttribute")
	desc := noLigaDesc()
	if desc == 0 {
		t.Fatal("noLigaDesc = 0")
	}
	defer cfRelease(desc)
	da := copyAttr(desc, kCTFontFeatureSettingsAttribute)
	if da == 0 {
		t.Fatal("descriptor has no feature settings")
	}
	defer cfRelease(da)
	want := settings(da)
	for _, tag := range []string{"liga", "clig", "calt"} {
		if v, ok := want[tag]; !ok || v != 0 {
			t.Errorf("descriptor feature settings %v: want %s = 0", want, tag)
		}
	}
	// The copy keeps those the font has (CoreText normalizes the list to
	// the font's features: SF Pro has liga and calt, no clig).
	arr := ctFontCopyFeatureSettings(fc)
	if arr == 0 {
		t.Fatal("CTFontCopyFeatureSettings = NULL")
	}
	defer cfRelease(arr)
	got := settings(arr)
	if v, ok := got["calt"]; !ok || v != 0 {
		t.Errorf("font feature settings %v: want calt = 0", got)
	}
	for _, tag := range []string{"liga", "clig", "calt"} {
		if v, ok := got[tag]; ok && v != 0 {
			t.Errorf("font feature settings %v: %s = %v, want 0", got, tag, v)
		}
	}
	// The shaper uses the copy, and maps runs set in it back to the face.
	var sh ctShaper
	defer sh.release()
	if c := sh.font(f, 13, true); c != fc {
		t.Errorf("ctShaper.font(noLiga) = %#x, want the copy %#x", c, fc)
	}
	if got := sh.faceOf(fc); got != f {
		t.Errorf("faceOf(copy) = %p, want %p", got, f)
	}
}

// End to end with a font whose ligatures are contextual alternates (calt):
// Ligatures:false draws every rune with its own cmap glyph.
func TestCTNoLigaCaltFont(t *testing.T) {
	needCT(t)
	home, _ := os.UserHomeDir()
	var data []byte
	for _, dir := range []string{home + "/Library/Fonts", "/Library/Fonts"} {
		for _, n := range []string{"FiraCode-Regular.ttf", "FiraCode-VF.ttf", "JetBrainsMono-Regular.ttf", "JetBrainsMono[wght].ttf", "CascadiaCode.ttf", "CascadiaCode-Regular.ttf"} {
			if b, err := os.ReadFile(dir + "/" + n); err == nil {
				data = b
				break
			}
		}
		if data != nil {
			break
		}
	}
	if data == nil {
		t.Skip("no calt ligature font (Fira Code, JetBrains Mono, Cascadia Code) installed")
	}
	saveUserFonts(t)
	if err := RegisterFont("CaltCode", data); err != nil {
		t.Fatal(err)
	}
	line := "a -> b != c"
	runs := []Run{{0, len([]rune(line)), Style{Size: 14}}}
	c := NewLineCache(8)
	off := c.Shape(line, runs, LineOptions{Ligatures: false})
	on := c.Shape(line, runs, LineOptions{Ligatures: true})
	differ := false
	for g, k := range off.GlyphCluster {
		f := glyphFace(t, off, g)
		want, _ := f.glyphFor(off.Runes[k])
		if off.Glyphs[g].ID != want {
			t.Errorf("Ligatures:false: glyph %d of %q is %d, want the cmap glyph %d", g, off.Runes[k], off.Glyphs[g].ID, want)
		}
	}
	for g := range on.Glyphs {
		if g >= len(off.Glyphs) || on.Glyphs[g].ID != off.Glyphs[g].ID {
			differ = true
		}
	}
	if !differ {
		t.Log("Ligatures:true drew the same glyphs (font without calt arrows?)")
	}
}

// Style.Lang is caller input: an invalid UTF-8 tag is skipped, not passed
// to CoreText as a NULL string.
func TestCTInvalidLangTag(t *testing.T) {
	needCT(t)
	for _, s := range []string{"Hello", "直"} {
		l := Shape(s, Style{Size: 16, Lang: "\xff\xfe"}, Options{})
		if len(l.Glyphs) == 0 {
			t.Errorf("%q with an invalid Lang: no glyphs", s)
		}
	}
	c := NewLineCache(4)
	if l := c.Shape("ffi", []Run{{0, 3, Style{Size: 16}}}, LineOptions{Lang: "\xff\xfe"}); len(l.Glyphs) == 0 {
		t.Error("LineCache with an invalid Lang: no glyphs")
	}
}

func TestCTShapeClusters(t *testing.T) {
	needCT(t)
	for _, s := range []string{"é", "é", "👨‍👩‍👧", "🇨🇳"} {
		l := Shape(s, Style{Size: 16}, Options{})
		if len(l.Glyphs) == 0 {
			t.Errorf("%q: no glyphs", s)
			continue
		}
		for g, c := range l.GlyphCluster {
			if c != 0 {
				t.Errorf("%q: glyph %d has cluster %d, want 0 (the cluster start)", s, g, c)
			}
		}
	}
	s := "👨‍👩‍👧"
	l := Shape(s, Style{Size: 16}, Options{})
	x0, _, _ := l.Caret(0)
	x1, _, _ := l.Caret(1)
	xe, _, _ := l.Caret(len(l.Runes))
	if x1 != x0 && x1 != xe {
		t.Errorf("Caret(1) = %v inside the ZWJ cluster [%v, %v]", x1, x0, xe)
	}
}

func TestCTShapeAstral(t *testing.T) {
	needCT(t)
	l := Shape("a😀b", Style{Size: 16}, Options{})
	if len(l.Runes) != 3 {
		t.Fatalf("len(Runes) = %d, want 3", len(l.Runes))
	}
	found := false
	for g, c := range l.GlyphCluster {
		if glyphFace(t, l, g).HasRune('b') && c == 2 {
			found = true
		}
		if c < 0 || c > 2 {
			t.Errorf("glyph %d: cluster %d out of range", g, c)
		}
	}
	if !found || l.GlyphCluster[len(l.GlyphCluster)-1] != 2 {
		t.Errorf("GlyphCluster = %v, want b's glyph at cluster 2", l.GlyphCluster)
	}
}

func TestCTShapeEmptyParagraphs(t *testing.T) {
	needCT(t)
	st := Style{Size: 16}
	for _, s := range []string{"", "\n", "\n\n", "   ", "\t"} {
		l := Shape(s, st, Options{})
		if l.LineHeight != st.norm().LineHeight {
			t.Errorf("%q: LineHeight = %v, want %v", s, l.LineHeight, st.norm().LineHeight)
		}
	}
	if n := len(Shape("\n\n", st, Options{}).Lines); n != 3 {
		t.Errorf(`"\n\n": %d lines, want 3`, n)
	}
}

func TestCTShapeMixedCJK(t *testing.T) {
	needCT(t)
	l := Shape("Tango 中文", Style{Size: 16}, Options{})
	var sawCJK, sawLatin bool
	for g, c := range l.GlyphCluster {
		f := glyphFace(t, l, g)
		switch r := l.Runes[c]; {
		case r == '中' || r == '文':
			sawCJK = true
			if !f.HasRune('中') || !f.HasRune('文') {
				t.Errorf("glyph of %q from %q, which lacks CJK", r, f.Family)
			}
		case r >= 'A' && r <= 'z':
			sawLatin = true
			if f.HasRune('中') {
				t.Errorf("Latin glyph of %q from CJK family %q", r, f.Family)
			}
		}
	}
	if !sawCJK || !sawLatin {
		t.Fatalf("glyphs missing: CJK %v Latin %v", sawCJK, sawLatin)
	}
}

func TestCTFallbackFaceStable(t *testing.T) {
	needCT(t)
	a := Shape("中", Style{Size: 16}, Options{})
	b := Shape("中", Style{Size: 16}, Options{NoWrap: true})
	c := Shape("中", Style{Size: 23}, Options{})
	if len(a.Glyphs) == 0 || len(b.Glyphs) == 0 || len(c.Glyphs) == 0 {
		t.Fatal("no glyphs")
	}
	if a.Glyphs[0].Src != b.Glyphs[0].Src {
		t.Errorf("two Shape calls: Src %p, %p", a.Glyphs[0].Src, b.Glyphs[0].Src)
	}
	if a.Glyphs[0].Src != c.Glyphs[0].Src {
		t.Errorf("sizes 16 and 23: Src %p, %p", a.Glyphs[0].Src, c.Glyphs[0].Src)
	}
	// A font CoreText picks itself (the system UI font, optical size
	// following the point size) maps to one face at every size.
	f12 := ctFontCreateUIFontForLanguage(ctFontUIFontSystem, 12, 0)
	f30 := ctFontCreateUIFontForLanguage(ctFontUIFontSystem, 30, 0)
	if f12 == 0 || f30 == 0 {
		t.Skip("no system UI font")
	}
	defer cfRelease(f12)
	defer cfRelease(f30)
	if x, y := faceForCTFont(f12), faceForCTFont(f30); x == nil || x != y {
		t.Errorf("system UI font at 12 and 30: faces %p, %p", x, y)
	}
}

func TestCTWrapAndEllipsis(t *testing.T) {
	needCT(t)
	s := "hello world tango text"
	st := Style{Size: 16}
	w := Width(s, st)
	if l := Shape(s, st, Options{MaxWidth: w / 2}); len(l.Lines) < 2 {
		t.Errorf("MaxWidth %v < width %v: %d lines", w/2, w, len(l.Lines))
	}
	l := Shape(s, st, Options{MaxWidth: w / 2, MaxLines: 1, Ellipsis: true})
	if !l.Truncated {
		t.Fatal("Truncated = false")
	}
	last := len(l.Glyphs) - 1
	if last < 0 {
		t.Fatal("no glyphs")
	}
	id, ok := glyphFace(t, l, last).glyphFor('…')
	if !ok || l.Glyphs[last].ID != id {
		t.Errorf("last glyph %d, want … (%d, %v)", l.Glyphs[last].ID, id, ok)
	}
}

func TestCTItalicNoDoubleSkew(t *testing.T) {
	needCT(t)
	l := Shape("Hello", Style{Size: 16, Italic: true}, Options{})
	real := false
	for g := range l.Glyphs {
		if f := glyphFace(t, l, g); f.Italic {
			real = true
			if l.Glyphs[g].Skew != 0 {
				t.Errorf("glyph %d of italic face %q: Skew %v, want 0", g, f.Family, l.Glyphs[g].Skew)
			}
		}
	}
	if !real {
		t.Log("system sans has no italic face; skipped the designed-italic half")
	}
	data, err := os.ReadFile("/System/Library/Fonts/Geneva.ttf")
	if err != nil {
		t.Skip("no Geneva.ttf")
	}
	saveUserFonts(t)
	if err := RegisterFont("UprightOnly", data); err != nil {
		t.Fatal(err)
	}
	l = Shape("Hello", Style{Size: 16, Italic: true}, Options{})
	if len(l.Glyphs) == 0 {
		t.Fatal("no glyphs")
	}
	for g := range l.Glyphs {
		if f := glyphFace(t, l, g); f.Italic || l.Glyphs[g].Skew != ItalicSkew {
			t.Errorf("glyph %d (%q italic=%v): Skew %v, want %v", g, f.Family, f.Italic, l.Glyphs[g].Skew, ItalicSkew)
		}
	}
}

func TestCTRegisteredFontMissingRune(t *testing.T) {
	needCT(t)
	data, err := os.ReadFile("/System/Library/Fonts/Geneva.ttf")
	if err != nil {
		t.Skip("no Geneva.ttf")
	}
	saveUserFonts(t)
	UseSystemFonts(false)
	t.Cleanup(func() { UseSystemFonts(true) })
	if err := RegisterFont("LatinOnly", data); err != nil {
		t.Fatal(err)
	}
	l := Shape("A中", Style{Size: 16}, Options{})
	for g, c := range l.GlyphCluster {
		if f := glyphFace(t, l, g); f.Family != "LatinOnly" {
			t.Errorf("glyph %d (rune %d) from %q, want the registered face only", g, c, f.Family)
		}
	}
}

func TestCTLangAttributeCJK(t *testing.T) {
	needCT(t)
	ja := Shape("直", Style{Size: 16, Lang: "ja"}, Options{})
	zh := Shape("直", Style{Size: 16, Lang: "zh-Hans"}, Options{})
	if len(ja.Glyphs) != 1 || len(zh.Glyphs) != 1 {
		t.Fatalf("glyphs: ja %d zh %d", len(ja.Glyphs), len(zh.Glyphs))
	}
	if ja.Glyphs[0].ID == zh.Glyphs[0].ID && ja.Glyphs[0].Src == zh.Glyphs[0].Src {
		t.Errorf("直: same glyph %d of the same face for ja and zh-Hans", ja.Glyphs[0].ID)
	}
}

func TestCTBidiCaretAndSelection(t *testing.T) {
	needCT(t)
	s := "abc שלום def"
	st := Style{Size: 16}
	l := Shape(s, st, Options{})
	if len(l.Lines) != 1 {
		t.Fatalf("%d lines", len(l.Lines))
	}
	// Inside the Hebrew word (4 and 8 are direction boundaries).
	prev, _, _ := l.Caret(5)
	for i := 6; i <= 7; i++ {
		x, _, _ := l.Caret(i)
		if !(x < prev) {
			t.Errorf("Caret(%d) = %v, not left of Caret(%d) = %v", i, x, i-1, prev)
		}
		prev = x
	}
	adv := ctAdvances(t, s, st)
	var hw float32
	for _, a := range adv[4:8] {
		hw += a
	}
	rs := l.SelectionRects(4, 8)
	if len(rs) != 1 {
		t.Fatalf("SelectionRects(4, 8) = %v, want 1 rect", rs)
	}
	if !ctNear(rs[0].W, hw) {
		t.Errorf("selection width %v, want %v", rs[0].W, hw)
	}
	if i := l.IndexAt(-5, l.LineHeight/2); i != 0 {
		t.Errorf("IndexAt left of the line = %d, want 0", i)
	}
	if i := l.IndexAt(l.Width+50, l.LineHeight/2); i != len(l.Runes) {
		t.Errorf("IndexAt right of the line = %d, want %d", i, len(l.Runes))
	}
}

func TestCTTabsExpand(t *testing.T) {
	needCT(t)
	st := Style{Size: 14, Mono: true}
	c := NewLineCache(8)
	l := c.Shape("a\tb", []Run{{0, 3, st}}, LineOptions{TabSize: 4})
	col := ctAdvances(t, " ", st)[0] // Width trims trailing spaces
	var bx float32 = -1
	for g, k := range l.GlyphCluster {
		if k == 2 {
			bx = l.Glyphs[g].X
		}
	}
	if !ctNear(bx, 4*col) {
		t.Errorf("b at x = %v, want 4 columns = %v", bx, 4*col)
	}
}
