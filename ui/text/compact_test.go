//go:build !darwin || ios || !tango_coretext

package text

import (
	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
	"reflect"
	"testing"
)

func TestCompactShaperMatchesHarfBuzz(t *testing.T) {
	clearCache()
	base := Shape("office 12.34", Style{Size: 11}, Options{NoWrap: true})
	if len(base.Glyphs) == 0 {
		t.Skip("no fonts")
	}
	face := base.Glyphs[0].Src.(*Face)
	var compact compactShaper
	var reference shaping.HarfbuzzShaper
	for _, s := range []string{"12.34", "87.01", "AVATAR", "office", "ffi", "abc123", "-32.05", "foo.bar", "x + y", "éa", "مرحبا"} {
		rs := []rune(s)
		for _, size := range []float32{11, 12.5, 17} {
			for _, dir := range []di.Direction{di.DirectionLTR, di.DirectionRTL} {
				in := shaping.Input{Text: rs, RunEnd: len(rs), Face: face.gt(), Size: fixed.Int26_6(size * 64), Script: language.Latin, Language: language.NewLanguage("en"), Direction: dir}
				old := reference.Shape(in)
				got := compact.append(nil, in, face, 0)
				if len(got) != len(old.Glyphs) {
					t.Fatalf("%q glyph count %d != %d", s, len(got), len(old.Glyphs))
				}
				for i, g := range old.Glyphs {
					want := sg{face: face, gid: uint32(g.GlyphID), adv: fx(g.XAdvance), xoff: fx(g.XOffset), yoff: fx(g.YOffset), cluster: g.ClusterIndex}
					if !reflect.DeepEqual(got[i], want) {
						t.Fatalf("%q size%v direction%v glyph%d: %+v != %+v", s, size, dir, i, got[i], want)
					}
				}
			}
		}
	}
}

func BenchmarkCompactShaper(b *testing.B) {
	base := Shape("12.34", Style{Size: 11}, Options{NoWrap: true})
	if len(base.Glyphs) == 0 {
		b.Skip("no font")
	}
	face := base.Glyphs[0].Src.(*Face)
	rs := []rune("12.34")
	in := shaping.Input{Text: rs, RunEnd: len(rs), Face: face.gt(), Size: fixed.I(11), Direction: di.DirectionLTR, Script: language.Latin, Language: language.NewLanguage("en")}
	b.Run("reference", func(b *testing.B) {
		var s shaping.HarfbuzzShaper
		b.ReportAllocs()
		for b.Loop() {
			s.Shape(in)
		}
	})
	b.Run("compact", func(b *testing.B) {
		var s compactShaper
		var out []sg
		b.ReportAllocs()
		for b.Loop() {
			out = s.append(out[:0], in, face, 0)
		}
	})
}

func TestTrimCachesDropsCompactFontReferences(t *testing.T) {
	clearCache()
	Shape("12.34", Style{Size: 11}, Options{NoWrap: true})
	if len(advanceShaper.fonts) == 0 {
		t.Skip("no fonts")
	}
	TrimCaches()
	if len(advanceShaper.fonts) != 0 {
		t.Fatal("TrimCaches retains compact shaping font references")
	}
}
