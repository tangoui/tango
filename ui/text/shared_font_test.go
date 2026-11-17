package text

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

// Weights of one variable system font share one parsed *font.Font (each
// weight has its own *font.Face with its own coordinates). Every shaping
// path must still measure each weight with its own coordinates.
func TestSharedVariableFontWeightsMeasureDifferently(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the default sans is a variable font on macOS")
	}
	withEngine(t, true)
	clearCache()
	widths := map[Weight]float32{}
	for _, w := range []Weight{Light, Regular, Bold} {
		widths[w] = Shape("Hello World", Style{Size: 16, Weight: w}, Options{}).Width
	}
	if !(widths[Light] < widths[Regular] && widths[Regular] < widths[Bold]) {
		t.Fatalf("Shape widths %v, want strictly increasing", widths)
	}
	reg := Shape("Hi", Style{Size: 16, Weight: Regular}, Options{}).Glyphs[0].Src.(*Face)
	bold := Shape("Hi", Style{Size: 16, Weight: Bold}, Options{}).Glyphs[0].Src.(*Face)
	if reg.gt().Font != bold.gt().Font {
		t.Fatal("weights of one variable font must share one parsed *font.Font")
	}
}

// fastPath.shapeUnit shapes through a shaper whose cache must not be keyed
// by *font.Font alone: weights of one variable font share that Font. Shaping
// Regular then Bold (and the reverse) must give each weight its own
// advances, as independent shaping does.
func TestFastPathShapeUnitSharedFontWeights(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the default sans is a variable font on macOS")
	}
	withEngine(t, true)
	clearCache()
	face := func(w Weight) *Face {
		return Shape("Hello", Style{Size: 16, Weight: w}, Options{NoWrap: true}).Glyphs[0].Src.(*Face)
	}
	reg, bold := face(Regular), face(Bold)
	runes := []rune("Hello World")
	key := func(f *Face) asciiKey {
		return asciiKey{face: f, size: 16, script: language.Latin, lang: language.NewLanguage("en")}
	}
	alone := func(f *Face) []sg { var fp fastPath; return fp.shapeUnit(runes, 0, len(runes), key(f)) }
	wantReg, wantBold := alone(reg), alone(bold)
	if reflect.DeepEqual(wantReg, wantBold) {
		t.Fatal("Regular and Bold shaped alike even independently")
	}
	for _, order := range [][2]*Face{{reg, bold}, {bold, reg}} {
		var fp fastPath
		for _, f := range order {
			want := wantReg
			if f == bold {
				want = wantBold
			}
			if got := fp.shapeUnit(runes, 0, len(runes), key(f)); !reflect.DeepEqual(got, want) {
				t.Fatalf("weight %v after another weight on one shaper: advances differ from independent shaping", f.Weight)
			}
		}
	}
}

// HVAR specialization re-parses the whole font, so a face is specialized
// only once it is shaped often; before and after, every glyph is identical.
func TestCompactShaperSpecializesHotFacesOnly(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs the variable system font")
	}
	withEngine(t, true)
	clearCache()
	noShapeMemo(t) // count HarfBuzz shapes of one run
	face := Shape("Hello", Style{Size: 16, Weight: Bold}, Options{NoWrap: true}).Glyphs[0].Src.(*Face)
	runes := []rune("AVATAR office 12.34 ffi")
	in := shaping.Input{Text: runes, RunEnd: len(runes), Face: face.gt(), Size: fixed.I(16), Direction: di.DirectionLTR, Script: language.Latin, Language: language.NewLanguage("en")}
	var s compactShaper
	first := s.append(nil, in, face, 0)
	if s.fonts[in.Face].specialized {
		t.Fatal("a cold face must not be HVAR-specialized")
	}
	for range hotShapingUses {
		s.append(nil, in, face, 0)
	}
	if !s.fonts[in.Face].specialized {
		t.Fatalf("a face shaped %d times must be specialized", hotShapingUses+1)
	}
	if got := s.append(nil, in, face, 0); !reflect.DeepEqual(got, first) {
		t.Fatal("specialization changed the shaped glyphs")
	}
}
