package text

import (
	"bytes"
	"image"
	"testing"
	"unsafe"

	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/tangoui/tango/ui/paint"
)

// Pure-logic tests of the system text engine (no platform tags): they run
// on every OS. Tests that need a working engine skip when Engine() is "go".

func TestEngineFromEnv(t *testing.T) {
	for v, want := range map[string]bool{"": true, "system": true, "SYSTEM": true, "go": false, " Go ": false, "off": false, "0": false} {
		if engineFromEnv(v) != want {
			t.Errorf("%q: want %v", v, want)
		}
	}
}

func TestClassOf(t *testing.T) {
	for r, want := range map[rune]runeClass{
		'中': classCJK, '，': classCJK, '。': classCJK, 'あ': classCJK, '한': classCJK, '（': classCJK,
		'a': classNone, ',': classNone, '“': classNone, '✓': classNone, 'é': classNone,
		'😀': classEmoji, '🎉': classEmoji, '☀': classEmoji, 0xFE0F: classEmoji,
	} {
		if got := classOf(r); got != want {
			t.Errorf("%q: class %d want %d", r, got, want)
		}
	}
}

func TestWeightMaps(t *testing.T) {
	for _, w := range []Weight{100, 200, 300, 400, 500, 600, 700, 800, 900} {
		if got := cssWeightFromCT(ctWeightTrait(w)); got != w {
			t.Errorf("CoreText round trip %d -> %d", w, got)
		}
	}
	if cssWeightFromCT(0.24) != 500 || cssWeightFromCT(-0.1) != 400 {
		t.Error("nearest CoreText weight")
	}
	for fc, want := range map[int]Weight{0: 100, 40: 200, 50: 300, 75: 400, 80: 400, 100: 500, 180: 600, 200: 700, 210: 900} {
		if got := cssWeightFromFC(fc); got != want {
			t.Errorf("FC_WEIGHT %d -> %d want %d", fc, got, want)
		}
	}
	for in, want := range map[int]Weight{350: 400, 449: 400, 450: 500, 950: 900, 1: 100} {
		if got := roundWeight(in); got != want {
			t.Errorf("roundWeight(%d) = %d want %d", in, got, want)
		}
	}
}

func TestSfntFromTablesSkipsGlyphData(t *testing.T) {
	data := sfntFromTables(map[uint32][]byte{
		tag4("cmap"): {0, 0, 0, 0}, tag4("glyf"): {1, 2, 3}, tag4("hmtx"): {0, 1}, tag4("hvgl"): {9}, tag4("GSUB"): {7, 7},
	})
	ld, err := ot.NewLoader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, tg := range ld.Tables() {
		have[tg.String()] = true
	}
	if !have["cmap"] || !have["hmtx"] || !have["GSUB"] || have["glyf"] || have["hvgl"] {
		t.Fatalf("tables %v", have)
	}
	if raw, _ := ld.RawTable(ot.MustNewTag("GSUB")); !bytes.Equal(raw, []byte{7, 7}) {
		t.Fatalf("GSUB content %v", raw)
	}
}

func TestDirectWriteSkewMatrix(t *testing.T) {
	m := dwSkewMatrix(0.2)
	// A point 10px above the baseline (y = -10, y down) moves right by 2.
	x := 0*m.M11 + (-10)*m.M21 + m.Dx
	if x != 2 || m.M11 != 1 || m.M22 != 1 || m.M12 != 0 {
		t.Fatalf("matrix %+v x=%v", m, x)
	}
}

func TestFreeTypeTransform(t *testing.T) {
	m, d := ftTransform(3, 0.25)
	if m.XX != 65536 || m.YY != 65536 || m.YX != 0 || m.XY != 16384 {
		t.Fatalf("matrix %+v", m)
	}
	if d.X != 48 || d.Y != 0 { // 3/4 px in 26.6
		t.Fatalf("delta %+v", d)
	}
	if m, d := ftTransform(0, 0); m.XY != 0 || d.X != 0 {
		t.Fatal("identity")
	}
}

func TestClearTypeToGray(t *testing.T) {
	got := clearTypeToGray([]byte{0, 0, 0, 255, 255, 255, 30, 60, 90, 255, 0, 0}, 2, 2)
	if !bytes.Equal(got, []byte{0, 255, 60, 85}) {
		t.Fatalf("%v", got)
	}
}

// fakeSlot lays out an FT_GlyphSlotRec (LP64) in Go memory.
func fakeSlot(rows, width, pitch int, buf []byte, mode uint8, left, top int32) []uint64 {
	slot := make([]uint64, 32) // 256 bytes, pointer-aligned
	p := unsafe.Pointer(&slot[0])
	b := unsafe.Add(p, ftSlotBitmapOffset)
	*(*uint32)(b) = uint32(rows)
	*(*uint32)(unsafe.Add(b, 4)) = uint32(width)
	*(*int32)(unsafe.Add(b, 8)) = int32(pitch)
	*(*unsafe.Pointer)(unsafe.Add(b, 16)) = unsafe.Pointer(&buf[0])
	*(*uint8)(unsafe.Add(b, 26)) = mode
	*(*int32)(unsafe.Add(p, ftSlotBitmapLeftOff)) = left
	*(*int32)(unsafe.Add(p, ftSlotBitmapLeftOff+4)) = top
	return slot
}

func TestReadFTBitmap(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("LP64 layout")
	}
	// 3×2 gray, pitch 4 (row padding).
	buf := []byte{1, 2, 3, 0, 4, 5, 6, 0}
	slot := fakeSlot(2, 3, 4, buf, ftPixelModeGray, -1, 9)
	bm := readFTBitmap(unsafe.Pointer(&slot[0]))
	if bm.rows != 2 || bm.width != 3 || bm.pitch != 4 || bm.left != -1 || bm.top != 9 {
		t.Fatalf("%+v", bm)
	}
	m, ok := bm.alpha()
	if !ok || !bytes.Equal(m.Pix, []byte{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("gray %v", m.Pix)
	}
	// Up flow: negative pitch, first row in memory is the bottom one.
	slot = fakeSlot(2, 3, -4, buf, ftPixelModeGray, 0, 0)
	m, _ = readFTBitmap(unsafe.Pointer(&slot[0])).alpha()
	if !bytes.Equal(m.Pix, []byte{4, 5, 6, 1, 2, 3}) {
		t.Fatalf("up flow %v", m.Pix)
	}
	// 1-bit mono, MSB first.
	mono := []byte{0b1010_0000, 0b0100_0000}
	slot = fakeSlot(2, 3, 1, mono, ftPixelModeMono, 0, 0)
	m, _ = readFTBitmap(unsafe.Pointer(&slot[0])).alpha()
	if !bytes.Equal(m.Pix, []byte{255, 0, 255, 0, 255, 0}) {
		t.Fatalf("mono %v", m.Pix)
	}
	// Empty and unsupported (BGRA colour) bitmaps.
	slot = fakeSlot(0, 0, 0, buf, ftPixelModeGray, 0, 0)
	if m, ok := readFTBitmap(unsafe.Pointer(&slot[0])).alpha(); m != nil || !ok {
		t.Fatal("empty bitmap")
	}
	slot = fakeSlot(1, 1, 4, buf, 7, 0, 0)
	if _, ok := readFTBitmap(unsafe.Pointer(&slot[0])).alpha(); ok {
		t.Fatal("BGRA must fall back")
	}
}

func TestTrimMask(t *testing.T) {
	m := image.NewAlpha(image.Rect(0, 0, 8, 6))
	m.Pix[2*8+3], m.Pix[3*8+5] = 10, 20
	got, at := trimMask(m, 1)
	if at != image.Pt(2, 1) || got.Rect != image.Rect(0, 0, 5, 4) || got.Pix[1*5+1] != 10 || got.Pix[2*5+3] != 20 {
		t.Fatalf("at %v rect %v", at, got.Rect)
	}
	if e, _ := trimMask(image.NewAlpha(image.Rect(0, 0, 3, 3)), 1); e != nil {
		t.Fatal("empty mask")
	}
}

func TestPickFCFaces(t *testing.T) {
	got := pickFCFaces([]fcFace{
		{file: "a-VF.ttf", weight: 80, vari: true},
		{file: "a-VF.ttf", index: 3 << 16, weight: 80}, // named instance wins over the bare variable font
		{file: "a-Cond.ttf", weight: 200, width: 75},
		{file: "a-Bold.ttf", weight: 200, width: 100}, // normal width wins
		{file: "a-It.ttf", weight: 80, slant: 100, width: 100},
		{file: "fixed.pcf.gz", weight: 50},
	})
	if len(got) != 3 || got[0].index != 3<<16 || got[1].file != "a-Bold.ttf" || got[2].file != "a-It.ttf" {
		t.Fatalf("%+v", got)
	}
}

// withEngine runs f with the system engine on or off, restoring it after.
func withEngine(t *testing.T, on bool) {
	t.Helper()
	prev := UseSystemEngine(on)
	t.Cleanup(func() { UseSystemEngine(prev) })
}

func TestEngineUnavailableFallsBack(t *testing.T) {
	withEngine(t, true)
	goChain := func() []string {
		withEngine(t, false)
		defer UseSystemEngine(true)
		return Families()
	}()
	reg.mu.Lock()
	impl, opened := reg.engImpl, reg.engOpened
	reg.engImpl, reg.engOpened, reg.sysBuilt = nil, true, false // as if dlopen failed
	reg.version++
	reg.mu.Unlock()
	clearCache()
	t.Cleanup(func() {
		reg.mu.Lock()
		reg.engImpl, reg.engOpened, reg.sysBuilt = impl, opened, false
		reg.version++
		reg.mu.Unlock()
		clearCache()
	})
	if Engine() != "go" {
		t.Fatalf("engine %s", Engine())
	}
	if got := Families(); len(got) != len(goChain) {
		t.Fatalf("fallback chain %v, pure-Go chain %v", got, goChain)
	}
	for _, g := range Shape("你好 Tango", Style{Size: 14}, Options{}).Glyphs {
		if g.Src.(*Face).SystemRasterized() {
			t.Fatal("engine face without an engine")
		}
	}
}

// TestEngineSwitchUsesNewSources: one GlyphSource never changes path; a
// switch hands out new sources (the GPU atlas keys masks by source).
func TestEngineSwitchUsesNewSources(t *testing.T) {
	withEngine(t, false)
	if len(Families()) == 0 {
		t.Skip("no system fonts")
	}
	s := "中文 Tango"
	goL := Shape(s, Style{Size: 14}, Options{})
	for _, g := range goL.Glyphs {
		if _, _, ok := g.Src.(*Face).RasterGlyph(g.ID, 14, 0, 0); ok {
			t.Fatal("pure-Go face rasterized through the engine")
		}
	}
	UseSystemEngine(true)
	if Engine() == "go" {
		t.Skip("no system text engine here")
	}
	sysL := Shape(s, Style{Size: 14}, Options{})
	UseSystemEngine(false)
	goL2 := Shape(s, Style{Size: 14}, Options{})
	UseSystemEngine(true)
	sysL2 := Shape(s, Style{Size: 14}, Options{})
	srcs := func(l *Layout) map[paint.GlyphSource]bool {
		m := map[paint.GlyphSource]bool{}
		for _, g := range l.Glyphs {
			m[g.Src] = true
		}
		return m
	}
	a, b, c, d := srcs(goL), srcs(sysL), srcs(goL2), srcs(sysL2)
	for src := range b {
		if a[src] || c[src] || d[src] {
			t.Fatal("a system-engine source was reused across a switch")
		}
		if !src.(*Face).SystemRasterized() {
			t.Fatal("system layout glyph from a pure-Go face")
		}
	}
	for src := range a {
		if c[src] {
			t.Fatal("a pure-Go source was reused across a switch")
		}
	}
	// The old layouts keep their faces, which keep their path.
	for _, g := range sysL.Glyphs {
		if !g.Src.(*Face).SystemRasterized() {
			t.Fatal("old layout changed path")
		}
	}
}

// engineFaceGlyph shapes s with the engine on and returns its first glyph.
func engineFaceGlyph(t *testing.T, s string, st Style) paint.Glyph {
	t.Helper()
	withEngine(t, true)
	if Engine() == "go" {
		t.Skip("no system text engine here")
	}
	l := Shape(s, st, Options{})
	if len(l.Glyphs) == 0 || !l.Glyphs[0].Src.(*Face).SystemRasterized() {
		t.Skipf("%q has no engine face", s)
	}
	return l.Glyphs[0]
}

func inkBounds(m *image.Alpha) image.Rectangle {
	var r image.Rectangle
	for y := 0; y < m.Rect.Dy(); y++ {
		for x := 0; x < m.Rect.Dx(); x++ {
			if m.Pix[y*m.Stride+x] > 40 {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}

func TestEngineMaskGeometry(t *testing.T) {
	g := engineFaceGlyph(t, "H", Style{Size: 20})
	f := g.Src.(*Face)
	m, off, ok := f.RasterGlyph(g.ID, 20, 0, 0)
	if !ok || m == nil {
		t.Fatal("no mask")
	}
	ink := inkBounds(m).Add(off) // relative to the pen, y down
	// Cap height of a UI sans at 20px is ~14px: the H sits on the
	// baseline, starts near the pen and is roughly as wide as tall.
	if ink.Max.Y < -1 || ink.Max.Y > 1 || ink.Min.Y > -12 || ink.Min.Y < -17 {
		t.Fatalf("H ink %v (off %v, mask %v)", ink, off, m.Rect)
	}
	if ink.Min.X < 0 || ink.Min.X > 4 || ink.Dx() < 9 || ink.Dx() > 17 {
		t.Fatalf("H ink %v", ink)
	}
	// Same path every time, same result.
	m2, off2, ok2 := f.RasterGlyph(g.ID, 20, 0, 0)
	if !ok2 || off2 != off || !bytes.Equal(m.Pix, m2.Pix) {
		t.Fatal("RasterGlyph not deterministic")
	}
	// It agrees with the outline rasterized by paint (±1px).
	cpu := paint.NewCPU()
	om, ooff := cpu.GlyphMask(paint.Glyph{Src: outlineOnly{f}, ID: g.ID, Size: 20}, 20, 0)
	oink := inkBounds(om).Add(ooff)
	if d := ink.Min.Sub(oink.Min); d.X < -1 || d.X > 1 || d.Y < -1 || d.Y > 1 {
		t.Fatalf("engine ink %v vs outline %v", ink, oink)
	}
}

// outlineOnly hides RasterGlyph so paint fills the outline.
type outlineOnly struct{ f *Face }

func (o outlineOnly) GlyphPath(id uint32, size float32) *paint.Path { return o.f.GlyphPath(id, size) }

func TestEngineSubpixelAndSkew(t *testing.T) {
	g := engineFaceGlyph(t, "l", Style{Size: 14})
	f := g.Src.(*Face)
	m0, off0, _ := f.RasterGlyph(g.ID, 14, 0, 0)
	m2, off2, _ := f.RasterGlyph(g.ID, 14, 2, 0)
	if off0 == off2 && bytes.Equal(m0.Pix, m2.Pix) {
		t.Fatal("subx does not move the glyph")
	}
	// Half a pixel to the right: the ink's centre of mass moves ~0.5px.
	if d := centroidX(m2, off2) - centroidX(m0, off0); d < 0.3 || d > 0.7 {
		t.Fatalf("subx 2 moved the ink by %.2fpx", d)
	}
	ms, offs, _ := f.RasterGlyph(g.ID, 14, 0, ItalicSkew)
	up, sk := inkBounds(m0).Add(off0), inkBounds(ms).Add(offs)
	// The slanted stem's top moves right by ~skew × height.
	topShift := topInkX(ms, offs) - topInkX(m0, off0)
	if sk.Dx() <= up.Dx() || topShift < 1 {
		t.Fatalf("skew: upright %v slanted %v top shift %.1f", up, sk, topShift)
	}
}

func centroidX(m *image.Alpha, off image.Point) float64 {
	var s, sx float64
	for y := 0; y < m.Rect.Dy(); y++ {
		for x := 0; x < m.Rect.Dx(); x++ {
			v := float64(m.Pix[y*m.Stride+x])
			s += v
			sx += v * (float64(x+off.X) + 0.5)
		}
	}
	return sx / s
}

func topInkX(m *image.Alpha, off image.Point) float64 {
	b := inkBounds(m)
	var s, sx float64
	for x := 0; x < m.Rect.Dx(); x++ {
		v := float64(m.Pix[(b.Min.Y+1)*m.Stride+x])
		s += v
		sx += v * float64(x+off.X)
	}
	return sx / s
}

// TestEngineMetrics: the engine reads the same font tables, so the layout
// metrics of the primary face (ascent, descent; line height is the style's)
// stay those of the pure-Go path when both find the same primary family.
func TestEngineMetrics(t *testing.T) {
	withEngine(t, false)
	if len(Families()) == 0 {
		t.Skip("no system fonts")
	}
	goFirst := Families()[0]
	gl := Shape("Tango 中文", Style{Size: 14}, Options{})
	withEngine(t, true)
	if Engine() == "go" {
		t.Skip("no system text engine here")
	}
	sl := Shape("Tango 中文", Style{Size: 14}, Options{})
	if sl.LineHeight != gl.LineHeight || sl.Height != gl.Height {
		t.Fatalf("line height %v vs %v", sl.LineHeight, gl.LineHeight)
	}
	if sl.Ascent < 14*0.7 || sl.Ascent > 14*1.1 {
		t.Fatalf("engine ascent %v at 14px", sl.Ascent)
	}
	if Families()[0] == goFirst && sl.Ascent != gl.Ascent {
		t.Fatalf("same primary %s, ascent %v vs %v", goFirst, sl.Ascent, gl.Ascent)
	}
	t.Logf("engine %s: ascent %.2f (go %.2f), width %.2f (go %.2f)", Engine(), sl.Ascent, gl.Ascent, sl.Width, gl.Width)
}
