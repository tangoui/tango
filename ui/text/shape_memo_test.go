package text

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-text/typesetting/di"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// shapeMemoCorpus covers the shaping features a run memo could get wrong:
// kerning, ligatures, script and face fallback, emoji sequences, both
// directions, combining marks, spaces (including the missing-space
// EmptyGlyph rule) and tabs. The long entries exceed shapeMemoRunes.
var shapeMemoCorpus = []string{
	"OK", "Cancel", "Submit form", "Hello World", "Settings",
	"AV", "To", "Wa", "Ty", "LT", "AVATAR Today Water Type LTD",
	"fi", "fl", "ffi", "office file flow affix",
	"确定 OK", "设置Settings中文", "日本語テキストabc", "한국어 text",
	"👍 Like", "🎉🎉", "👨‍👩‍👧 family", "1️⃣ 🇨🇳",
	"مرحبا بالعالم", "שלום עולם", "abc مرحبا def", "בדיקה 123", "بب ب",
	"é", "café", "ạ̈", "ñandú", "x̶y",
	"a\tb", "\tlead", "  lead  trail  ", "　全角", "x y", "a b c d", "a b c",
	"12.34", "-32.05 %", "1,234,567.89",
	"The quick brown fox jumps over the lazy dog, then the dog sleeps all afternoon long under the tree.",
	"مرحبا بالعالم مرحبا بالعالم مرحبا بالعالم مرحبا بالعالم مرحبا بالعالم مرحبا بالعالم مرحبا",
}

// noShapeMemo makes every compactShaper.append reach HarfBuzz for the rest
// of the test (tests of the shaping font lifecycle repeat one run).
func noShapeMemo(t *testing.T) {
	shapeMemoOff = true
	t.Cleanup(func() { shapeMemoOff = false })
}

type shapeMemoConfig struct {
	engine bool
	st     Style
	opt    Options
}

func (c shapeMemoConfig) String() string {
	return fmt.Sprintf("engine=%v size=%v weight=%v italic=%v noLiga=%v tab=%v dir=%v", c.engine, c.st.Size, c.st.Weight, c.st.Italic, c.opt.noLiga, c.opt.tabStop, c.opt.Direction)
}

func shapeMemoConfigs() []shapeMemoConfig {
	var out []shapeMemoConfig
	for _, engine := range []bool{false, true} {
		for _, size := range []float32{11, 14, 20} {
			for _, w := range []Weight{Regular, Bold} {
				for _, noLiga := range []bool{false, true} {
					out = append(out, shapeMemoConfig{engine: engine, st: Style{Size: size, Weight: w}, opt: Options{noLiga: noLiga}})
				}
			}
		}
		out = append(out,
			shapeMemoConfig{engine: engine, st: Style{Size: 14, Weight: Light}},
			shapeMemoConfig{engine: engine, st: Style{Size: 14, Italic: true}},
			shapeMemoConfig{engine: engine, st: Style{Size: 14, Mono: true}, opt: Options{tabStop: 56, noLiga: true}},
			shapeMemoConfig{engine: engine, st: Style{Size: 14}, opt: Options{Direction: DirRTL}},
		)
	}
	return out
}

// shapeGlyphs runs the general shaping path (prepareText → shapeItems →
// compactShaper.append) without the layout and paragraph caches.
func shapeGlyphs(s string, c shapeMemoConfig) ([]sg, []float32) {
	st := c.st.norm()
	opt := normOptions(c.opt)
	mu.Lock()
	defer mu.Unlock()
	preparedCache = map[cacheKey]*preparedText{}
	preparedBytes = 0
	p := prepareText(s, []runStyle{{st: st, chain: reg.chain(st.Mono), lang: resolveLang(st)}}, nil, nil, opt)
	return append([]sg(nil), p.glyphs...), append([]float32(nil), p.adv...)
}

func sameMemoGlyphs(t *testing.T, label string, want, got []sg, wantAdv, gotAdv []float32) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: %d glyphs, uncached %d", label, len(got), len(want))
	}
	for i := range want {
		// sg is comparable: face pointer, glyph id, advances, offsets,
		// cluster and style run must all be identical.
		if want[i] != got[i] {
			t.Fatalf("%s glyph %d: %+v, uncached %+v", label, i, got[i], want[i])
		}
	}
	for i := range wantAdv {
		if wantAdv[i] != gotAdv[i] {
			t.Fatalf("%s rune %d advance %v, uncached %v", label, i, gotAdv[i], wantAdv[i])
		}
	}
}

func TestShapeMemoMatchesUncached(t *testing.T) {
	if len(Shape("OK", Style{}, Options{}).Glyphs) == 0 {
		t.Skip("no fonts")
	}
	prev := UseSystemEngine(false)
	defer UseSystemEngine(prev)
	defer func() { shapeMemoOff = false }()
	glyphs := 0
	for _, c := range shapeMemoConfigs() {
		UseSystemEngine(c.engine)
		type result struct {
			gl  []sg
			adv []float32
		}
		want := map[string]result{}
		shapeMemoOff = true
		clearCache()
		for _, s := range shapeMemoCorpus {
			gl, adv := shapeGlyphs(s, c)
			want[s] = result{gl, adv}
			glyphs += len(gl)
		}
		if len(advanceShaper.memo.m) != 0 {
			t.Fatalf("%v: memo filled while disabled", c)
		}
		shapeMemoOff = false
		clearCache()
		for pass := range 2 {
			misses := advanceShaper.memo.misses
			for _, s := range shapeMemoCorpus {
				gl, adv := shapeGlyphs(s, c)
				sameMemoGlyphs(t, fmt.Sprintf("%v pass %d %q", c, pass, s), want[s].gl, gl, want[s].adv, adv)
			}
			if pass == 1 && advanceShaper.memo.misses != misses {
				t.Fatalf("%v: %d memo misses on the repeated pass", c, advanceShaper.memo.misses-misses)
			}
		}
		if advanceShaper.memo.hits == 0 {
			t.Fatalf("%v: memo never hit", c)
		}
	}
	if glyphs == 0 {
		t.Skip("no glyphs shaped")
	}
}

// Full layouts through Shape (several widths, so the layout cache misses
// while the memo hits) equal the uncached layouts.
func TestShapeMemoLayoutsMatchUncached(t *testing.T) {
	prev := UseSystemEngine(false)
	defer UseSystemEngine(prev)
	defer func() { shapeMemoOff = false }()
	opts := []Options{{NoWrap: true}, {MaxWidth: 60}, {MaxWidth: 90, MaxLines: 1, Ellipsis: true}}
	hits := 0
	for _, s := range shapeMemoCorpus {
		for _, opt := range opts {
			shapeMemoOff = true
			clearCache()
			want := Shape(s, Style{Size: 13}, opt)
			shapeMemoOff = false
			clearCache()
			Shape(s, Style{Size: 13}, Options{MaxWidth: 1})
			before := advanceShaper.memo.hits
			got := Shape(s, Style{Size: 13}, opt)
			hits += advanceShaper.memo.hits - before
			if len(want.Glyphs) != len(got.Glyphs) || want.Width != got.Width || len(want.Lines) != len(got.Lines) {
				t.Fatalf("%q %+v: layout %v/%d glyphs/%d lines, uncached %v/%d/%d", s, opt, got.Width, len(got.Glyphs), len(got.Lines), want.Width, len(want.Glyphs), len(want.Lines))
			}
			for i := range want.Glyphs {
				if want.Glyphs[i] != got.Glyphs[i] || want.GlyphCluster[i] != got.GlyphCluster[i] || want.GlyphRun[i] != got.GlyphRun[i] {
					t.Fatalf("%q %+v glyph %d: %+v, uncached %+v", s, opt, i, got.Glyphs[i], want.Glyphs[i])
				}
			}
		}
	}
	if hits == 0 {
		t.Fatal("layouts never used the memo")
	}
}

func memoFace(t *testing.T, s string) *Face {
	t.Helper()
	clearCache()
	l := Shape(s, Style{Size: 14}, Options{NoWrap: true})
	if len(l.Glyphs) == 0 {
		t.Skip("no fonts")
	}
	return l.Glyphs[0].Src.(*Face)
}

// The same run text with different neighbours (HarfBuzz reads five runes of
// context on each side: Arabic joining) must not share an entry.
func TestShapeMemoKeysContext(t *testing.T) {
	face := memoFace(t, "ب")
	defer func() { shapeMemoOff = false }()
	inputs := []shaping.Input{}
	for _, s := range []string{"بب", " ب", "بب", "ب", "ببب", " بب"} {
		rs := []rune(s)
		for _, r := range [][2]int{{1, 2}, {0, 1}, {0, len(rs)}} {
			if r[1] > len(rs) {
				continue
			}
			inputs = append(inputs, shaping.Input{Text: rs, RunStart: r[0], RunEnd: r[1], Face: face.gt(), Size: fixed.I(14), Direction: di.DirectionRTL, Script: language.Arabic, Language: language.NewLanguage("ar")})
		}
	}
	var memo compactShaper
	for round := range 2 {
		for i, in := range inputs {
			shapeMemoOff = true
			var plain compactShaper
			want := plain.append(nil, in, face, 3)
			shapeMemoOff = false
			got := memo.append(nil, in, face, 3)
			sameMemoGlyphs(t, fmt.Sprintf("round %d input %d %q[%d:%d]", round, i, string(in.Text), in.RunStart, in.RunEnd), want, got, nil, nil)
		}
	}
	if memo.memo.hits == 0 {
		t.Fatal("memo never hit")
	}
}

// Every input field that reaches HarfBuzz is part of the key: size,
// direction, script, language, features, ppem and variation coordinates of
// the same *font.Face (go-text lets callers change them in place).
func TestShapeMemoKeysShapingState(t *testing.T) {
	face := memoFace(t, "AVATAR office")
	defer func() { shapeMemoOff = false }()
	rs := []rune("AVATAR office ffi")
	gt := face.gt()
	coords := append([]tables.Coord(nil), gt.Coords()...)
	x, y := gt.Ppem()
	defer func() { gt.SetCoords(coords); gt.SetPpem(x, y) }()
	base := shaping.Input{Text: rs, RunEnd: len(rs), Face: gt, Size: fixed.I(14), Direction: di.DirectionLTR, Script: language.Latin, Language: language.NewLanguage("en")}
	type variant struct {
		name  string
		edit  func(*shaping.Input)
		setup func()
	}
	variants := []variant{
		{"base", func(*shaping.Input) {}, nil},
		{"size", func(in *shaping.Input) { in.Size = fixed.I(20) }, nil},
		{"fractional size", func(in *shaping.Input) { in.Size = fixed.Int26_6(14*64 + 13) }, nil},
		{"rtl", func(in *shaping.Input) { in.Direction = di.DirectionRTL }, nil},
		{"script", func(in *shaping.Input) { in.Script = language.Cyrillic }, nil},
		{"language", func(in *shaping.Input) { in.Language = language.NewLanguage("tr") }, nil},
		{"features", func(in *shaping.Input) { in.FontFeatures = noLigaFeatures }, nil},
		{"kern off", func(in *shaping.Input) {
			in.FontFeatures = []shaping.FontFeature{{Tag: noLigaFeatures[0].Tag, Value: 1}, {Tag: 0x6b65726e, Value: 0}}
		}, nil},
		// Same tags, other value (kern=1 vs kern=0 above): pins value encoding.
		{"kern on", func(in *shaping.Input) {
			in.FontFeatures = []shaping.FontFeature{{Tag: noLigaFeatures[0].Tag, Value: 1}, {Tag: ot.NewTag('k', 'e', 'r', 'n'), Value: 1}}
		}, nil},
		// Same count and values as "kern off", other tag: pins tag encoding.
		{"mark off", func(in *shaping.Input) {
			in.FontFeatures = []shaping.FontFeature{{Tag: noLigaFeatures[0].Tag, Value: 1}, {Tag: ot.NewTag('m', 'a', 'r', 'k'), Value: 0}}
		}, nil},
		{"ppem", func(*shaping.Input) {}, func() { gt.SetPpem(30, 30) }},
	}
	if len(coords) > 0 {
		heavy := append([]tables.Coord(nil), coords...)
		heavy[0] = 0x3000
		variants = append(variants, variant{"coords", func(*shaping.Input) {}, func() { gt.SetCoords(heavy) }})
	}
	var memo compactShaper
	for round := range 2 {
		for _, v := range variants {
			gt.SetCoords(coords)
			gt.SetPpem(x, y)
			if v.setup != nil {
				v.setup()
			}
			in := base
			v.edit(&in)
			shapeMemoOff = true
			var plain compactShaper
			want := plain.append(nil, in, face, 0)
			shapeMemoOff = false
			got := memo.append(nil, in, face, 0)
			sameMemoGlyphs(t, fmt.Sprintf("round %d %s", round, v.name), want, got, nil, nil)
		}
	}
	if memo.memo.hits != len(variants) {
		t.Fatalf("memo hits %d, want %d (one per variant on the second round)", memo.memo.hits, len(variants))
	}
}

func TestShapeMemoInvalidation(t *testing.T) {
	memoFace(t, "OK")
	filled := func(step string) {
		t.Helper()
		Shape("Invalidate me", Style{Size: 15}, Options{NoWrap: true})
		mu.Lock()
		n := len(advanceShaper.memo.m)
		mu.Unlock()
		if n == 0 {
			t.Fatalf("%s: memo not filled", step)
		}
	}
	emptied := func(step string) {
		t.Helper()
		// Shape clears every cache when the registry version moved.
		Shape("x", Style{Size: 9}, Options{})
		mu.Lock()
		defer mu.Unlock()
		for k := range advanceShaper.memo.m {
			if k.size == fixed.Int26_6(15*64) {
				t.Fatalf("%s: stale memo entry survived", step)
			}
		}
	}
	prev := UseSystemEngine(false)
	defer UseSystemEngine(prev)

	filled("start")
	UseSystemEngine(true)
	emptied("UseSystemEngine(true)")
	filled("engine on")
	UseSystemEngine(false)
	emptied("UseSystemEngine(false)")

	filled("registry")
	reg.mu.Lock()
	reg.version++
	reg.mu.Unlock()
	emptied("registry version")

	filled("trim")
	TrimCaches()
	emptied("TrimCaches")

	if data, err := os.ReadFile("/System/Library/Fonts/Supplemental/Arial.ttf"); err == nil {
		reg.mu.Lock()
		saved := append([]*family(nil), reg.user...)
		reg.mu.Unlock()
		defer func() {
			reg.mu.Lock()
			reg.user = saved
			reg.version++
			reg.mu.Unlock()
			clearCache()
		}()
		filled("register")
		if err := RegisterFont("ShapeMemoTest", data); err != nil {
			t.Fatal(err)
		}
		emptied("RegisterFont")
	}
}

// The memo stays within its entry and byte budgets under a stream of
// unique labels, and its accounting matches its contents.
func TestShapeMemoBounded(t *testing.T) {
	prev := UseSystemEngine(false)
	defer UseSystemEngine(prev)
	memoFace(t, "OK")
	for i := range shapeMemoEntries + 500 {
		Shape(fmt.Sprintf("Label %d 标签", i), Style{Size: 12}, Options{NoWrap: true})
	}
	// Long unique runs make the byte budget, not the entry count, evict.
	for i := range 2000 {
		Shape(fmt.Sprintf("%060d", i), Style{Size: 12, Mono: true}, Options{MaxWidth: 1e6})
	}
	Shape("Long run beyond the memo limit: "+string(make([]rune, shapeMemoRunes+1)), Style{Size: 12}, Options{})
	mu.Lock()
	defer mu.Unlock()
	m := &advanceShaper.memo
	if len(m.m) > shapeMemoEntries || m.bytes > shapeMemoBudget {
		t.Fatalf("memo holds %d entries / %d bytes, limits %d / %d", len(m.m), m.bytes, shapeMemoEntries, shapeMemoBudget)
	}
	if len(m.order)-m.head != len(m.m) {
		t.Fatalf("FIFO holds %d keys for %d entries", len(m.order)-m.head, len(m.m))
	}
	total := 0
	for _, k := range m.order[m.head:] {
		gl, ok := m.m[k]
		if !ok {
			t.Fatalf("FIFO key missing from map")
		}
		total += shapeMemoCost(k, gl)
		for _, g := range gl {
			if g.face != nil {
				t.Fatal("memo entries retain glyph faces beyond their key")
			}
		}
	}
	if total != m.bytes {
		t.Fatalf("accounted %d bytes, entries cost %d", m.bytes, total)
	}
	t.Logf("memo: %d entries, %d bytes", len(m.m), m.bytes)
	if len(m.m) < 100 {
		t.Fatalf("only %d entries retained", len(m.m))
	}
}

// A memo hit copies glyphs into dst without allocating.
func TestShapeMemoHitDoesNotAllocate(t *testing.T) {
	face := memoFace(t, "Submit")
	rs := []rune("Submit form")
	in := shaping.Input{Text: rs, RunEnd: len(rs), Face: face.gt(), Size: fixed.I(14), Direction: di.DirectionLTR, Script: language.Latin, Language: language.NewLanguage("en"), FontFeatures: noLigaFeatures}
	var s compactShaper
	out := s.append(nil, in, face, 0)
	allocs := testing.AllocsPerRun(100, func() { out = s.append(out[:0], in, face, 0) })
	if allocs != 0 || s.memo.hits < 100 {
		t.Fatalf("hit path: %v allocs, %d hits", allocs, s.memo.hits)
	}
}

// HarfBuzz reads exactly five runes of context before a run (Arabic joining
// skips transparent marks within it). A joining letter followed by four
// harakat still joins the run's letter; one more haraka puts it out of
// reach. If go-text widens its window, the uncached checks below fail
// (shapeMemoContext must then follow); if shapeMemoContext is narrower
// than the window, the memo replays the wrong joining form.
func TestShapeMemoContextMatchesHarfBuzz(t *testing.T) {
	face := memoFace(t, "بب")
	defer func() { shapeMemoOff = false }()
	if shapeMemoContext != 5 {
		t.Fatalf("shapeMemoContext = %d, harfbuzz.contextLength is 5", shapeMemoContext)
	}
	input := func(lead string, marks int) shaping.Input {
		rs := []rune(lead)
		for range marks {
			rs = append(rs, '\u064e') // fatha: transparent for joining
		}
		rs = append(rs, 'ب')
		return shaping.Input{Text: rs, RunStart: len(rs) - 1, RunEnd: len(rs), Face: face.gt(), Size: fixed.I(14), Direction: di.DirectionRTL, Script: language.Arabic, Language: language.NewLanguage("ar")}
	}
	plain := func(in shaping.Input) uint32 {
		shapeMemoOff = true
		var s compactShaper
		gl := s.append(nil, in, face, 0)
		shapeMemoOff = false
		if len(gl) != 1 {
			t.Fatalf("%q: %d glyphs", string(in.Text), len(gl))
		}
		return gl[0].gid
	}
	isolated := plain(input(" ", 0))
	if plain(input("ب", 0)) == isolated {
		t.Skip("font has no Arabic joining forms")
	}
	if plain(input("ب", shapeMemoContext-1)) == isolated {
		t.Fatalf("a joining letter %d runes back did not join: HarfBuzz reads less context than shapeMemoContext", shapeMemoContext)
	}
	if plain(input("ب", shapeMemoContext)) != isolated {
		t.Fatalf("a joining letter %d runes back joined: HarfBuzz reads more context than shapeMemoContext=%d", shapeMemoContext+1, shapeMemoContext)
	}
	// The memo must tell the joined and the unjoined run apart.
	var memo compactShaper
	for round := range 2 {
		for _, in := range []shaping.Input{input("ب", 4), input(" ", 4), input("ب", 5), input(" ", 5)} {
			shapeMemoOff = true
			var p compactShaper
			want := p.append(nil, in, face, 0)
			shapeMemoOff = false
			got := memo.append(nil, in, face, 0)
			sameMemoGlyphs(t, fmt.Sprintf("round %d %q", round, string(in.Text)), want, got, nil, nil)
		}
	}
}
