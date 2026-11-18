//go:build darwin && !ios && tango_coretext

package text

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
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
// on 1 200 random lines per language (zh-Hans, ja, zh-Hant, en, ko) (single style and token runs, two
// sizes) and checks that most of them take the fast path.
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
		if lang != "ja" && c.stats.fast < c.stats.slow {
			t.Errorf("%s: fast path took %d of %d lines", lang, c.stats.fast, c.stats.fast+c.stats.slow)
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
