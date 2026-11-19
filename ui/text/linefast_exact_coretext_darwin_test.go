//go:build darwin && !ios && !tango_gotext

package text

import (
	"encoding/binary"
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/image/font/gofont/gomono"
)

// The exact route of the monospace fast path (linefast_coretext_darwin.go)
// must lay out mixed CJK, Latin and punctuation lines exactly as the
// general path, in every CJK language, and must actually fire.

// exactPieces are the material of the random lines: Han (Simplified,
// Traditional, characters with Japanese and Traditional Chinese local
// forms), kana, CJK and ASCII punctuation, fullwidth forms, Latin-1 and
// ASCII words, digits and spaces.
var exactPieces = [][]rune{
	[]rune("的一是不了人我在有他这中大来上国个到说们为子和你地出道也时年得就那要下以生会自着去之过家学对可她里后小么心多天而能好都然没日于起还发成事只作当想看文无开手十用主行方又如前所本见经头面公同三已老从动两长知民样现分将外但身些与高意进把法此实回二理美点月明其种声全工己话儿者向情部正名定女问力机给等几很业最间新什打便位因重被走电四第门相次东政海口使教西再平真听世气信北少关并内加化由却代军产入先山五太水万市眼体别处总才场师书比住员九笑性通目华报立马命张活难神数件安表原车白应路期叫死常提感金何更反合放做系计或司利受光王果亲界及今京务制解各任至清物台象记边共风战干接它许八特觉望直服毛林题建南度统色字请交爱让认算论百吃义科怎元社术结六功指思非流每青管夫连远资队跟带花快条院变联言权往展该领传近留红治决周保达办运武半候七必城父强步完革深区即求品士转量空甚众技轻程告江语英基派满式李息写呢识极令黄德收脸钱党倒未持取设始版双历越史商千片容研像找友孩站广改议形委早房音火际则首单据导影失拿网香似斯专石若兵弟谁校读志飞观争究包组造落视济喜离虽坏兴切团音樂國語學說話門開關體這來時對會經過還東車書長電點邊見頁發後為與從們個氣實現灣產業"),
	[]rune("あいうえおかきくけこさしすせそたちつてとなにぬねのはひふへほまみむめもやゆよらりるれろわをんがぎぐげござじずぜぞだぢづでどばびぶべぼぱぴぷぺぽアイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホマミムメモヤユヨラリルレロワヲンーッャュョ"),
	[]rune("，。、；：？！「」『』（）《》〈〉【】“”‘’…—·～〜・％＃＆＊＋－＝／"),
	[]rune("ＡＢＣＤＥＦａｂｃ０１２３４５６７８９"),
	[]rune("éüñçàößøåæ€£©®°±×÷"),
}

var exactWords = []string{"func", "return", "the", "Tango", "CoreText", "x", "i", "j", "fi", "ff", "ll", "go-text", "LineCache", "API", "URL", "v0.3.1", "a+b", "x := 1", "// note", "[link](url)", "**bold**", "`code`", "#", "-", "1.", "2026-11-18"}

// randomExactLine returns a line of 1 to 12 random pieces.
func randomExactLine(rng *rand.Rand) string {
	var sb strings.Builder
	for n := 1 + rng.IntN(12); n > 0; n-- {
		switch k := rng.IntN(10); {
		case k < 4: // CJK text
			src := exactPieces[0]
			if rng.IntN(4) == 0 {
				src = exactPieces[1]
			}
			for m := 1 + rng.IntN(8); m > 0; m-- {
				sb.WriteRune(src[rng.IntN(len(src))])
			}
		case k < 6:
			sb.WriteRune(exactPieces[2][rng.IntN(len(exactPieces[2]))])
		case k < 7:
			src := exactPieces[3+rng.IntN(2)]
			sb.WriteRune(src[rng.IntN(len(src))])
		case k < 9:
			sb.WriteString(exactWords[rng.IntN(len(exactWords))])
		default:
			sb.WriteString([]string{" ", "  ", "\t", "　"}[rng.IntN(4)])
		}
	}
	return sb.String()
}

// TestCTMonoExactRandomLines compares the fast path with the general path
// on 1 200 random lines per language, with single-style and token runs
// at two sizes. The fast path should handle most lines in zh-Hans, en and
// ko, and still fire on safe zh-Hant lines.
func TestCTMonoExactRandomLines(t *testing.T) {
	needCT(t)
	rng := rand.New(rand.NewPCG(20261118, 2))
	opt := LineOptions{TabSize: 4}
	for _, lang := range []string{"zh-Hans", "ja", "zh-Hant", "en", "ko"} {
		c := NewLineCache(64)
		for i := range 1200 {
			line := randomExactLine(rng)
			n := utf8.RuneCountInString(line)
			size := float32(13)
			if i%3 == 0 {
				size = 14.5
			}
			st := Style{Size: size, Mono: true, Lang: lang}
			runs := []Run{{0, n, st}}
			if i%4 == 1 && n > 2 { // syntax-highlighted: bold, italic and plain tokens
				a, b := n/3, 2*n/3
				bold, it := st, st
				bold.Weight, it.Italic = Bold, true
				runs = []Run{{0, a, bold}, {a, b, st}, {b, n, it}}
			}
			l := c.Shape(line, runs, opt)
			sameLayout(t, lang+" "+line, l, monoGeneral(line, runs, opt))
			if t.Failed() {
				t.Fatalf("%s line %d %q differs", lang, i, line)
			}
		}
		t.Logf("%s: fast %d, slow %d, probe CTLines %d", lang, c.stats.fast, c.stats.slow, c.fp.probes)
		switch lang {
		case "ja": // CJK lines use the general path (exactLang).
		case "zh-Hant":
			// PingFang's hani/latn locl lookups can differ from DFLT. The
			// guard must not count such substitutions as script-independent.
			if c.stats.fast == 0 {
				t.Error("zh-Hant: no lines used the fast path")
			}
		default:
			if c.stats.fast < c.stats.slow {
				t.Errorf("%s: fast path took %d of %d lines", lang, c.stats.fast, c.stats.fast+c.stats.slow)
			}
		}
	}
}

// TestCTMonoExactProbesOncePerLine checks that a line's new runes share
// one probe CTLine and that runes already probed need none.
func TestCTMonoExactProbesOncePerLine(t *testing.T) {
	needCT(t)
	c := NewLineCache(8)
	st := Style{Size: 13, Mono: true, Lang: "zh-Hans"}
	shape := func(line string) {
		c.Shape(line, []Run{{0, utf8.RuneCountInString(line), st}}, LineOptions{})
	}
	shape("文字排版：中文和English混排。")
	if c.stats.fast != 1 {
		t.Fatalf("fast %d slow %d", c.stats.fast, c.stats.slow)
	}
	p := c.fp.probes
	if p < 1 || p > 2 { // one per shaping run; ASCII probe aside
		t.Fatalf("probes %d after one line", p)
	}
	shape("排版中文：混排English。")
	if c.fp.probes != p || c.stats.fast != 2 {
		t.Fatalf("probes %d -> %d, fast %d: known runes probed again", p, c.fp.probes, c.stats.fast)
	}
}

// TestCTOTLRejectsContextualLookups checks the table analysis on the
// default monospace face: Catalan's contextual locl ("l·l") starts at the
// middle dot, which is set by itself in Chinese but not in Catalan.
func TestCTOTLRejectsContextualLookups(t *testing.T) {
	needCT(t)
	mono := (&fontmap{chain: reg.chain(true), w: Regular}).primary()
	if mono == nil {
		t.Skip("no monospace face")
	}
	dot, ok := mono.glyphFor('\u00b7')
	if !ok {
		t.Skip("no middle dot")
	}
	if !otlFor(mono, "zh-Hans").allows(dot, "") {
		t.Error("middle dot not allowed in zh-Hans")
	}
	if o := otlFor(mono, "ca"); hasContextual(o) && o.allows(dot, "") {
		t.Error("middle dot allowed in Catalan although a contextual locl starts at it")
	}
}

// Japanese lines never take the exact route (exactLang) and still lay out
// as the general path.
func TestCTMonoExactNotJapanese(t *testing.T) {
	needCT(t)
	line := "return有。"
	runs := []Run{{0, utf8.RuneCountInString(line), Style{Size: 13, Mono: true, Lang: "ja"}}}
	c := NewLineCache(8)
	l := c.Shape(line, runs, LineOptions{})
	if c.stats.fast != 0 {
		t.Fatal("Japanese CJK line took the fast path")
	}
	sameLayout(t, line, l, monoGeneral(line, runs, LineOptions{}))
}

func hasContextual(o *otlInfo) bool {
	for _, l := range o.lookups {
		if !l.gpos && (l.kind == 5 || l.kind == 6) {
			return true
		}
	}
	return false
}

func TestCTOTLIncludesDefaultScript(t *testing.T) {
	// latn is advertised but has no feature. DFLT has a contextual ccmp.
	o := fixtureOTL(t, fixtureGSUB("DFLT", true, 1,
		fixtureLookup(6, fixtureChain(60, 10, 60, 1)),
		fixtureLookup(1, fixtureSingle(10, 20))))
	if o.allows(10, "latn") {
		t.Error("latn discarded contextual DFLT coverage of glyph 10")
	}
	if !o.allows(80, "latn") {
		t.Error("uncovered glyph sent to the general path")
	}
}

func TestCTOTLRejectsIntermediateContext(t *testing.T) {
	// 10 → 20; 20 → 30 between two 60s; otherwise the final lookup
	// maps 20 → 40 (or 30 → 50 in context). Neither 10 nor the probe's
	// final 40 is in the contextual lookup's coverage.
	o := fixtureOTL(t, fixtureGSUB("latn", false, 3,
		fixtureLookup(1, fixtureSingle(10, 20)),
		fixtureLookup(6, fixtureChain(60, 20, 60, 3)),
		fixtureLookup(1, fixtureSingle(20, 40), fixtureSingle(30, 50)),
		fixtureLookup(1, fixtureSingle(20, 30))))
	if !o.allows(40, "latn") {
		t.Fatal("fixture's probe output is not uncovered")
	}
	if o.allows(10, "latn") {
		t.Error("cmap glyph 10 allowed despite contextual coverage of intermediate glyph 20")
	}
}

func TestCTOTLAllowsIsolatedSingleSubstitution(t *testing.T) {
	o := fixtureOTL(t, fixtureGSUB("latn", false, 2,
		fixtureLookup(1, fixtureSingle(10, 20)),
		fixtureLookup(6, fixtureChain(60, 30, 60, 2)),
		fixtureLookup(1, fixtureSingle(30, 40))))
	if !o.allows(10, "latn") {
		t.Error("isolated single substitution sent to the general path")
	}
}

func TestCTOTLRejectsUnknownContextFormat(t *testing.T) {
	// A future contextual format need not have coverage at offset +2.
	// Accepting it as format 1/2 could silently miss affected glyphs.
	gsub := fixtureGSUB("latn", false, 1,
		fixtureLookup(6, fixtureU16(4, 4, 1, 1, 20)))
	if _, ok := parseOTL(gsub, false, true, nil); ok {
		t.Error("unknown contextual subtable format allowed")
	}
}

func TestCTOTLNumericLigaturesRespectAlphabet(t *testing.T) {
	// 10 → 20 can feed a ligature with 60. None of the substitutes
	// are cmap glyphs, so checking only cmap glyphs would miss it.
	gsub := fixtureGSUB("latn", false, 2,
		fixtureLookup(1, fixtureSingle(10, 20)),
		fixtureLookup(4, fixtureU16(1, 18, 1, 8, 1, 4, 30, 2, 60, 1, 1, 20)))
	for _, tc := range []struct {
		other uint16
		want  bool
	}{{60, false}, {70, true}} {
		ls, ok := parseOTL(gsub, false, false, nil)
		if !ok {
			t.Fatal("numeric ligature fixture failed to parse")
		}
		o := &otlInfo{ok: true, lookups: ls.lookups}
		o.restrictLigatures(map[uint16]bool{10: true, tc.other: true})
		if got := o.allows(10, ""); got != tc.want {
			t.Errorf("alphabet {10, %d}: allows = %v, want %v", tc.other, got, tc.want)
		}
	}
}

func TestCTOTLBoundsExpandedLookupWork(t *testing.T) {
	// Each reference is small enough to parse on its own. Traversing two
	// lookups must share a budget, including when records alias each other.
	const refs, components = 32, 96
	lig := fixtureU16(1, 8, 1, 14, 1, 1, 10, refs)
	for range refs {
		lig = binary.BigEndian.AppendUint16(lig, 2+2*refs)
	}
	lig = append(lig, fixtureU16(20, components)...)
	for range components - 1 {
		lig = binary.BigEndian.AppendUint16(lig, 30)
	}
	// Coverage format 2 encodes a long contiguous range in one record.
	single := fixtureU16(1, 6, 1, 2, 1, 10, 3009, 0)
	for _, tc := range []struct {
		name   string
		lookup []byte
	}{
		{"repeated-ligature-record", fixtureLookup(4, lig)},
		{"single-substitution-range", fixtureLookup(1, single)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := parseOTL(fixtureGSUB("latn", false, 1, tc.lookup), false, false, nil); !ok {
				t.Fatal("one lookup should fit the parse budget")
			}
			if _, ok := parseOTL(fixtureGSUB("latn", false, 2, tc.lookup, tc.lookup), false, false, nil); ok {
				t.Error("two lookups exceeded the cumulative parse budget but were accepted")
			}
		})
	}
}

func TestCTMonoASCIIFallsBackOnContextualGSUB(t *testing.T) {
	gid := fixtureMono(t)
	a, x, A := gid('a'), gid('x'), gid('A')
	gsub := fixtureGSUB("latn", false, 1,
		fixtureLookup(6, fixtureChain(x, a, x, 1)),
		fixtureLookup(1, fixtureSingle(a, A)))
	if err := RegisterFont("ASCIIContextFixture", fixtureFont(gsub)); err != nil {
		t.Fatal(err)
	}
	// The ascending ASCII probe contains no xax; contextual ccmp does
	// apply to this line despite the probe reporting cmap equality.
	line := "xax"
	runs := []Run{{0, len(line), Style{Size: 13, Mono: true, Lang: "en"}}}
	opt := LineOptions{}
	general := monoGeneral(line, runs, opt)
	if len(general.Glyphs) != 3 || general.Glyphs[1].ID != uint32(A) {
		t.Fatalf("CoreText did not apply the fixture's contextual substitution: %+v", general.Glyphs)
	}
	c := NewLineCache(8)
	got := c.Shape(line, runs, opt)
	if c.stats.fast != 0 {
		t.Errorf("ASCII cmap route bypassed contextual GSUB (glyph %d instead of %d)", got.Glyphs[1].ID, A)
	}
	sameLayout(t, line, got, general)
}

func TestCTNumericFallsBackOnContextualGSUB(t *testing.T) {
	gid := fixtureMono(t)
	one, two, three := gid('1'), gid('2'), gid('3')
	// Change 1 to 3 when preceded and followed by two 2s. This context
	// is longer than any of the numeric table's probe strings.
	chain := fixtureU16(3, 2, 24, 24, 1, 30, 2, 24, 24, 1, 0, 1, 1, 1, two, 1, 1, one)
	gsub := fixtureGSUB("latn", false, 1,
		fixtureLookup(6, chain), fixtureLookup(1, fixtureSingle(one, three)))
	if err := RegisterFont("NumericContextFixture", fixtureFont(gsub)); err != nil {
		t.Fatal(err)
	}
	line := "22122"
	st := Style{Size: 13, Mono: true, Lang: "en"}
	runs := []Run{{0, len(line), st}}
	opt := Options{NoWrap: true}
	mu.Lock()
	general := shapeRunsUncached(line, runs, opt)
	fast := directNumericLayout(line, st, opt, regVersion())
	mu.Unlock()
	if len(general.Glyphs) != 5 || general.Glyphs[2].ID != uint32(three) {
		t.Fatalf("CoreText did not apply the fixture's contextual substitution: %+v", general.Glyphs)
	}
	if fast != nil {
		t.Errorf("numeric table bypassed contextual GSUB (glyph %d instead of %d)", fast.Glyphs[2].ID, three)
	}
	var dst Layout
	sameLayout(t, line, ShapeInto(&dst, line, st, opt), general)
}

func TestCTMonoExactTraditionalChineseScriptDependentLocl(t *testing.T) {
	needCT(t)
	st := Style{Size: 13, Mono: true, Lang: "zh-Hant"}
	for _, tc := range []struct {
		line string
		fast bool
	}{
		{"中文和English一", true},
		{"中文和English排", false}, // PingFang's 排 has hani but not DFLT locl.
	} {
		runs := []Run{{0, utf8.RuneCountInString(tc.line), st}}
		opt := LineOptions{}
		c := NewLineCache(8)
		got := c.Shape(tc.line, runs, opt)
		sameLayout(t, tc.line, got, monoGeneral(tc.line, runs, opt))
		if (c.stats.fast == 1) != tc.fast {
			t.Errorf("%q: fast/slow %d/%d, want fast %v", tc.line, c.stats.fast, c.stats.slow, tc.fast)
		}
	}
}

func fixtureMono(t *testing.T) func(rune) uint16 {
	t.Helper()
	needCT(t)
	saveUserFonts(t)
	faces, err := parseFaces("UnmodifiedGoMono", gomono.TTF)
	if err != nil || len(faces) != 1 {
		t.Fatalf("opening Go Mono fixture: %d faces, %v", len(faces), err)
	}
	reg.mu.Lock()
	disabled := reg.disabled
	reg.mu.Unlock()
	t.Cleanup(func() { UseSystemFonts(!disabled) })
	UseSystemFonts(false)
	return func(r rune) uint16 {
		g, ok := faces[0].glyphFor(r)
		if !ok {
			t.Fatalf("fixture missing glyph %q", r)
		}
		return uint16(g)
	}
}

func fixtureOTL(t *testing.T, gsub []byte) *otlInfo {
	t.Helper()
	ls, ok := parseOTL(gsub, false, true, nil)
	if !ok {
		t.Fatal("fixture GSUB failed to parse")
	}
	return &otlInfo{ok: true, lookups: ls.lookups}
}

// A single default ccmp feature references the first n lookups. Remaining
// lookups can be invoked from contextual rules but aren't top-level.
func fixtureGSUB(script string, emptyLatn bool, n int, lookups ...[]byte) []byte {
	var sl []byte
	if emptyLatn { // DFLT plus a latn default LangSys without features
		sl = append(fixtureU16(2), "DFLT"...)
		sl = append(sl, fixtureU16(14)...)
		sl = append(sl, "latn"...)
		sl = append(sl, fixtureU16(26, 4, 0, 0, 0xffff, 1, 0, 4, 0, 0, 0xffff, 0)...)
	} else {
		sl = append(fixtureU16(1), script...)
		sl = append(sl, fixtureU16(8, 4, 0, 0, 0xffff, 1, 0)...)
	}
	fl := append(fixtureU16(1), "ccmp"...)
	fl = append(fl, fixtureU16(8, 0, uint16(n))...)
	for i := range n {
		fl = binary.BigEndian.AppendUint16(fl, uint16(i))
	}
	b := fixtureU16(1, 0, 10, uint16(10+len(sl)), uint16(10+len(sl)+len(fl)))
	b = append(append(b, sl...), fl...)
	return append(b, fixtureOffsetList(fixtureU16(uint16(len(lookups))), lookups...)...)
}

func fixtureLookup(kind uint16, subtables ...[]byte) []byte {
	return fixtureOffsetList(fixtureU16(kind, 0, uint16(len(subtables))), subtables...)
}

func fixtureOffsetList(header []byte, entries ...[]byte) []byte {
	off := len(header) + 2*len(entries)
	for _, entry := range entries {
		header = binary.BigEndian.AppendUint16(header, uint16(off))
		off += len(entry)
	}
	for _, entry := range entries {
		header = append(header, entry...)
	}
	return header
}

func fixtureSingle(in, out uint16) []byte {
	return fixtureU16(2, 8, 1, out, 1, 1, in) // format 2; one glyph in coverage
}

func fixtureChain(before, in, after, lookup uint16) []byte {
	return fixtureU16(3, 1, 20, 1, 26, 1, 32, 1, 0, lookup,
		1, 1, before, 1, 1, in, 1, 1, after)
}

func fixtureU16(vals ...uint16) []byte {
	var b []byte
	for _, x := range vals {
		b = binary.BigEndian.AppendUint16(b, x)
	}
	return b
}

// Append GSUB to the bundled Go Mono font: reuse its glyphs and cmap,
// omitting layout tables that would otherwise disable the ASCII route.
func fixtureFont(gsub []byte) []byte {
	src := gomono.TTF
	dir := src[12 : 12+16*int(binary.BigEndian.Uint16(src[4:]))]
	type entry struct {
		tag string
		b   []byte
	}
	tables := []entry{{"GSUB", gsub}}
	for i := 0; i < len(dir); i += 16 {
		tag := string(dir[i : i+4])
		switch tag {
		case "GSUB", "GPOS", "kern", "DSIG":
			continue
		}
		off := binary.BigEndian.Uint32(dir[i+8:])
		n := binary.BigEndian.Uint32(dir[i+12:])
		tables = append(tables, entry{tag, src[off : off+n]})
	}
	// CoreText needs the directory and aligned tables, not checksums.
	b := make([]byte, 12+16*len(tables))
	copy(b, src[:4])
	binary.BigEndian.PutUint16(b[4:], uint16(len(tables)))
	for k, table := range tables {
		i := 12 + 16*k
		copy(b[i:], table.tag)
		binary.BigEndian.PutUint32(b[i+8:], uint32(len(b)))
		binary.BigEndian.PutUint32(b[i+12:], uint32(len(table.b)))
		b = append(b, table.b...)
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
	}
	return b
}
