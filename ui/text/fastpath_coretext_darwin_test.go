//go:build darwin && !ios && !tango_gotext

package text

import (
	"math"
	"math/rand/v2"
	"os"
	"testing"
	"unicode/utf8"
)

// The CoreText fast paths (numeric labels, monospace lines) must lay out
// exactly what the general path does when they fire, and leave the line
// to it otherwise.

// regVersion is the font registry version the fast paths are called with.
func regVersion() int {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	return reg.version
}

// sameLayout reports the first difference between the glyphs, carets and
// metrics of fast and general (X within 1e-3).
func sameLayout(t *testing.T, what string, fast, general *Layout) {
	t.Helper()
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) <= 1e-3 }
	if !near(fast.Width, general.Width) || fast.Height != general.Height || fast.Ascent != general.Ascent {
		t.Errorf("%s: Width/Height/Ascent = %v/%v/%v, general %v/%v/%v", what, fast.Width, fast.Height, fast.Ascent, general.Width, general.Height, general.Ascent)
	}
	if len(fast.Glyphs) != len(general.Glyphs) {
		t.Fatalf("%s: %d glyphs, general %d", what, len(fast.Glyphs), len(general.Glyphs))
	}
	for i, g := range fast.Glyphs {
		w := general.Glyphs[i]
		if g.ID != w.ID || g.Src != w.Src || g.Size != w.Size || !near(g.X, w.X) || !near(g.Y, w.Y) || g.Skew != w.Skew {
			t.Errorf("%s: glyph %d = %+v, general %+v", what, i, g, w)
		}
		if fast.GlyphCluster[i] != general.GlyphCluster[i] || fast.GlyphRun[i] != general.GlyphRun[i] {
			t.Errorf("%s: glyph %d cluster/run = %d/%d, general %d/%d", what, i, fast.GlyphCluster[i], fast.GlyphRun[i], general.GlyphCluster[i], general.GlyphRun[i])
		}
	}
	if len(fast.Lines) != 1 || len(general.Lines) != 1 {
		t.Fatalf("%s: %d lines, general %d", what, len(fast.Lines), len(general.Lines))
	}
	fl, gl := fast.Lines[0], general.Lines[0]
	if fl.Start != gl.Start || fl.End != gl.End || !near(fl.Width, gl.Width) || fl.Baseline != gl.Baseline || len(fl.carets) != len(gl.carets) {
		t.Fatalf("%s: line %+v, general %+v", what, fl, gl)
	}
	for i := range fl.carets {
		if !near(fl.carets[i], gl.carets[i]) {
			t.Errorf("%s: caret %d = %v, general %v", what, i, fl.carets[i], gl.carets[i])
		}
	}
}

// numericFastMatches checks that simpleNumericLayout takes the fast path
// for st and lays out what the general path does.
func numericFastMatches(t *testing.T, st Style) {
	t.Helper()
	opt := Options{NoWrap: true}
	v := regVersion()
	for _, size := range []float32{11, 13, 27.5} {
		st.Size = size
		st := st.norm()
		for _, s := range []string{"12345.67", "-0.5", "+42", "100", "7474.1"} {
			mu.Lock()
			fast := simpleNumericLayout(s, st, opt, v)
			general := shapeRunsUncached(s, []Run{{0, utf8.RuneCountInString(s), st}}, opt)
			mu.Unlock()
			if fast == nil {
				t.Fatalf("simpleNumericLayout(%q, %+v) = nil", s, st)
			}
			sameLayout(t, s, fast, general)
		}
	}
}

func TestCTNumericFastMatchesGeneral(t *testing.T) {
	needCT(t)
	// The default monospace face (SF Mono): no kerning tables.
	numericFastMatches(t, Style{Mono: true})
	numericFastMatches(t, Style{Mono: true, Weight: Bold, Italic: true})
	// The default sans face (SF Pro): GPOS kerning of digit pairs ("74",
	// "1."), placed from the probe's pair advances.
	numericFastMatches(t, Style{})
	numericFastMatches(t, Style{Weight: Semibold})
	// "100%" is outside simpleNumericText: the general path lays it out.
	mu.Lock()
	pct := simpleNumericLayout("100%", Style{Size: 13, Mono: true}.norm(), Options{NoWrap: true}, regVersion())
	mu.Unlock()
	if pct != nil {
		t.Errorf("simpleNumericLayout(100%%) took the fast path")
	}
}

// Random numeric labels in the system UI font (SF Pro, which kerns digit
// pairs) at several sizes and languages take the fast path and lay out
// exactly as the general path.
func TestCTNumericFastSFProRandom(t *testing.T) {
	needCT(t)
	rng := rand.New(rand.NewPCG(1, 2))
	opt := Options{NoWrap: true}
	v := regVersion()
	buf := make([]byte, 0, 12)
	for _, size := range []float32{11, 13, 16, 24} {
		for _, lang := range []string{"", "en", "zh-Hans"} {
			st := Style{Size: size, Lang: lang}.norm()
			for range 1000 {
				buf = buf[:0]
				for range 1 + rng.IntN(12) {
					buf = append(buf, numericChars[rng.IntN(len(numericChars))])
				}
				s := string(buf)
				mu.Lock()
				fast := simpleNumericLayout(s, st, opt, v)
				general := shapeRunsUncached(s, []Run{{0, len(s), st}}, opt)
				mu.Unlock()
				if fast == nil {
					t.Fatalf("size %v lang %q: %q took the general path", size, lang, s)
				}
				sameLayout(t, s, fast, general)
				if t.Failed() {
					t.Fatalf("size %v lang %q: %q differs", size, lang, s)
				}
			}
		}
	}
}

// The probe runs once per numeric table: per style (with its size) and
// language, not per label.
func TestCTNumericProbeOncePerStyle(t *testing.T) {
	needCT(t)
	type key struct {
		size float32
		lang string
	}
	probes := map[key]int{}
	numericProbeHook = func(f *Face, st Style, lang string) { probes[key{st.Size, lang}]++ }
	t.Cleanup(func() { numericProbeHook = nil })
	mu.Lock()
	clearNumericFaces()
	mu.Unlock()
	opt := Options{NoWrap: true}
	v := regVersion()
	for _, s := range []string{"1", "12.5", "-7474", "+0.01"} {
		for _, size := range []float32{11, 13} {
			st := Style{Size: size}.norm()
			mu.Lock()
			fast := simpleNumericLayout(s, st, opt, v)
			mu.Unlock()
			if fast == nil {
				t.Fatalf("%q size %v took the general path", s, size)
			}
			var l Layout
			if got := ShapeInto(&l, s, st, opt); got != &l {
				t.Fatal("ShapeInto did not return the destination")
			}
		}
	}
	if len(probes) != 2 {
		t.Fatalf("probes = %v, want one per size", probes)
	}
	for k, n := range probes {
		if n != 1 {
			t.Errorf("%+v probed %d times, want 1", k, n)
		}
	}
}

// The probe rejects a table CoreText does not reproduce: advances read at
// another size than the one shaped.
func TestCTNumericProbeRejectsMismatch(t *testing.T) {
	needCT(t)
	fm := &fontmap{chain: reg.chain(false), w: Regular}
	face := fm.primary()
	mu.Lock()
	defer mu.Unlock()
	tb := numericTableAt(face, 13)
	if !tb.ok {
		t.Fatal("no numeric table")
	}
	if probeNumeric(&tb, face, fm, Style{Size: 26}.norm(), "") {
		t.Error("probe accepted advances of another size")
	}
	tb = numericTableAt(face, 13)
	tb.gid[3]++
	if probeNumeric(&tb, face, fm, Style{Size: 13}.norm(), "") {
		t.Error("probe accepted a wrong glyph")
	}
}

// A face whose only kerning table is GPOS and whose digits do not kern
// (Comic Sans MS) takes the fast path too.
func TestCTNumericFastGPOSUnkerned(t *testing.T) {
	needCT(t)
	data, err := os.ReadFile("/System/Library/Fonts/Supplemental/Comic Sans MS.ttf")
	if err != nil {
		t.Skip("Comic Sans MS not installed")
	}
	saveUserFonts(t)
	if err := RegisterFont("ProbeGPOS", data); err != nil {
		t.Fatal(err)
	}
	numericFastMatches(t, Style{})
}

// A face with a morx table (Geneva) gets no numeric table; ShapeInto still
// fills the destination, shaped by CoreText.
func TestCTNumericFastSkipsMorxFont(t *testing.T) {
	needCT(t)
	data, err := os.ReadFile("/System/Library/Fonts/Geneva.ttf")
	if err != nil {
		t.Skip("Geneva not installed")
	}
	saveUserFonts(t)
	if err := RegisterFont("ProbeMorx", data); err != nil {
		t.Fatal(err)
	}
	f, _ := userFamily(t, "ProbeMorx").pick(Regular, false)
	if f.noMorxTable() {
		t.Skip("Geneva has no morx table here")
	}
	st := Style{Size: 13}.norm()
	opt := Options{NoWrap: true}
	mu.Lock()
	fast := simpleNumericLayout("12345.67", st, opt, regVersion())
	mu.Unlock()
	if fast != nil {
		t.Fatal("simpleNumericLayout took the fast path for a morx font")
	}
	var l Layout
	got := ShapeInto(&l, "12345.67", st, opt)
	if got != &l {
		t.Fatal("ShapeInto did not return the destination")
	}
	mu.Lock()
	general := shapeRunsUncached("12345.67", []Run{{0, 8, st}}, opt)
	mu.Unlock()
	sameLayout(t, "ShapeInto", got, general)
}

func TestCTNoKernTables(t *testing.T) {
	needCT(t)
	sans := (&fontmap{chain: reg.chain(false), w: Regular}).primary()
	mono := (&fontmap{chain: reg.chain(true), w: Regular}).primary()
	if !ctFontHasTable(sans, "GPOS") {
		t.Skip("system UI font has no GPOS")
	}
	if sans.plainAdvances() {
		t.Error("a face with GPOS has plain advances")
	}
	if !ctFontHasTable(mono, "GPOS") && !ctFontHasTable(mono, "kern") && !ctFontHasTable(mono, "kerx") && !ctFontHasTable(mono, "morx") && !mono.plainAdvances() {
		t.Error("a face without kern, kerx, GPOS and morx has no plain advances")
	}
}

// ctFontHasTable reports whether face f's font has table tag.
func ctFontHasTable(f *Face, tag string) bool {
	f.ready()
	arr := ctFontCopyAvailableTables(f.own.base, 0)
	if arr == 0 {
		return false
	}
	defer cfRelease(arr)
	for i := int64(0); i < cfArrayGetCount(arr); i++ {
		if uint32(cfArrayGetValueAtIndex(arr, i)) == tag4(tag) {
			return true
		}
	}
	return false
}

// monoGeneral is the general-path layout of line as LineCache.Shape would
// produce it.
func monoGeneral(line string, runs []Run, opt LineOptions) *Layout {
	rs, sopt := lineShapeArgs(runs, opt)
	mu.Lock()
	defer mu.Unlock()
	return shapeRunsUncached(line, rs, sopt)
}

func TestCTMonoFastMatchesGeneral(t *testing.T) {
	needCT(t)
	mono := (&fontmap{chain: reg.chain(true), w: Regular}).primary()
	if !mono.plainAdvances() {
		t.Skip("default monospace font has kerning tables")
	}
	cases := []struct {
		line string
		runs []Run
	}{
		{"func main() { return 42 }", nil},
		{"\tx := 1\t// tab", nil},
		{"a\t\tb", nil},
		{"bold italic", []Run{{0, 4, Style{Size: 13, Mono: true, Weight: Bold}}, {4, 5, Style{Size: 13, Mono: true}}, {5, 11, Style{Size: 13, Mono: true, Italic: true}}}},
		{"big small", []Run{{0, 3, Style{Size: 20, Mono: true}}, {3, 9, Style{Size: 12, Mono: true}}}},
	}
	for _, tc := range cases {
		runs := tc.runs
		if runs == nil {
			runs = []Run{{0, utf8.RuneCountInString(tc.line), Style{Size: 13, Mono: true}}}
		}
		c := NewLineCache(8)
		before := c.stats.fast
		l := c.Shape(tc.line, runs, LineOptions{Ligatures: false})
		if c.stats.fast != before+1 {
			t.Fatalf("%q: stats.fast = %d, want %d (slow %d)", tc.line, c.stats.fast, before+1, c.stats.slow)
		}
		sameLayout(t, tc.line, l, monoGeneral(tc.line, runs, LineOptions{Ligatures: false}))
	}
}

// Every printable ASCII character, in languages whose locl forms the
// monospace font could carry, lays out as in the general path.
func TestCTMonoFastMatchesGeneralASCIIAndLanguages(t *testing.T) {
	needCT(t)
	mono := (&fontmap{chain: reg.chain(true), w: Regular}).primary()
	if !mono.plainAdvances() {
		t.Skip("default monospace font has kerning tables")
	}
	var b []byte
	for r := byte(' '); r <= '~'; r++ {
		b = append(b, r)
	}
	line := string(b)
	for _, lang := range []string{"", "en", "tr", "az", "nl", "ca", "ro", "pl", "de", "zh-Hans", "ja", "ko", "ru", "sr", "bg"} {
		for _, w := range []Weight{Regular, Bold} {
			for _, italic := range []bool{false, true} {
				runs := []Run{{0, len(line), Style{Size: 13, Mono: true, Weight: w, Italic: italic, Lang: lang}}}
				c := NewLineCache(8)
				l := c.Shape(line, runs, LineOptions{})
				if c.stats.fast != 1 {
					t.Fatalf("lang %q w %d italic %v: fast path did not fire", lang, w, italic)
				}
				sameLayout(t, "lang "+lang, l, monoGeneral(line, runs, LineOptions{}))
			}
		}
	}
}

func TestCTMonoFastFallsBackOnCJK(t *testing.T) {
	needCT(t)
	line := "x := \"中文\""
	runs := []Run{{0, utf8.RuneCountInString(line), Style{Size: 13, Mono: true}}}
	c := NewLineCache(8)
	rs, sopt := lineShapeArgs(runs, LineOptions{})
	mu.Lock()
	fast := c.fastShape(line, rs, sopt)
	mu.Unlock()
	if fast != nil {
		t.Fatal("fastShape laid out a line with CJK")
	}
	l := c.Shape(line, runs, LineOptions{})
	if c.stats.slow != 1 || c.stats.fast != 0 {
		t.Fatalf("stats fast/slow = %d/%d, want 0/1", c.stats.fast, c.stats.slow)
	}
	if len(l.Glyphs) == 0 {
		t.Fatal("general path laid out no glyphs")
	}
}

func TestCTMonoFastFallsBackOnRTL(t *testing.T) {
	needCT(t)
	line := "x := \"שלום\""
	runs := []Run{{0, utf8.RuneCountInString(line), Style{Size: 13, Mono: true}}}
	c := NewLineCache(8)
	rs, sopt := lineShapeArgs(runs, LineOptions{})
	mu.Lock()
	fast := c.fastShape(line, rs, sopt)
	mu.Unlock()
	if fast != nil {
		t.Fatal("fastShape laid out a line with Hebrew")
	}
	c.Shape(line, runs, LineOptions{})
	if c.stats.slow != 1 || c.stats.fast != 0 {
		t.Fatalf("stats fast/slow = %d/%d, want 0/1", c.stats.fast, c.stats.slow)
	}
}

func TestCTMonoFastNeedsMonoAndNoLigatures(t *testing.T) {
	needCT(t)
	line := "return 42"
	for _, tc := range []struct {
		st   Style
		liga bool
	}{
		{Style{Size: 13}, false},            // not Mono
		{Style{Size: 13, Mono: true}, true}, // ligatures on
	} {
		c := NewLineCache(8)
		c.Shape(line, []Run{{0, 9, tc.st}}, LineOptions{Ligatures: tc.liga})
		if c.stats.fast != 0 || c.stats.slow != 1 {
			t.Errorf("%+v liga %v: stats fast/slow = %d/%d, want 0/1", tc.st, tc.liga, c.stats.fast, c.stats.slow)
		}
	}
}

func TestCTMonoFastFallsBackOnIgnorables(t *testing.T) {
	needCT(t)
	for _, line := range []string{"a\u200bb", "a\u00adb", "a\x01b", "a\ufeffb"} {
		c := NewLineCache(8)
		rs, sopt := lineShapeArgs([]Run{{0, 3, Style{Size: 13, Mono: true}}}, LineOptions{})
		mu.Lock()
		fast := c.fastShape(line, rs, sopt)
		mu.Unlock()
		if fast != nil {
			t.Errorf("fastShape(%q) took the fast path", line)
		}
	}
}

// SF Mono's GSUB applies ccmp and locl by default: "ŀ" and "Ŀ" decompose
// (ccmp), and Catalan's "l·l" gets a zero-width middle dot (locl). The fast
// path leaves non-ASCII lines in such faces to the general path, so these
// lay out exactly as there.
func TestCTMonoFastGSUBLines(t *testing.T) {
	needCT(t)
	for _, tc := range []struct{ line, lang string }{
		{"\u0140", ""}, {"\u013f", "en"}, {"l\u00b7l", "ca"}, {"x = \u0140 + l\u00b7l", "ca"},
	} {
		runs := []Run{{0, utf8.RuneCountInString(tc.line), Style{Size: 13, Mono: true, Lang: tc.lang}}}
		c := NewLineCache(8)
		l := c.Shape(tc.line, runs, LineOptions{})
		sameLayout(t, tc.line, l, monoGeneral(tc.line, runs, LineOptions{}))
		mono := (&fontmap{chain: reg.chain(true), w: Regular}).primary()
		if mono.needsProbe() && c.stats.fast != 0 {
			t.Errorf("%q (%s): non-ASCII line in a GSUB face took the fast path", tc.line, tc.lang)
		}
	}
}
