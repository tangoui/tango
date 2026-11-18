//go:build darwin && (ios || !tango_coretext)

package text

import (
	"reflect"
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

func TestCoreTextVariableFacesShareReadOnlyTables(t *testing.T) {
	coreTextOn(t)
	regular := Shape("AVATAR office 0123", Style{Size: 16, Weight: Regular}, Options{NoWrap: true}).Glyphs[0].Src.(*Face)
	bold := Shape("AVATAR office 0123", Style{Size: 16, Weight: Bold}, Options{NoWrap: true}).Glyphs[0].Src.(*Face)
	a, b := regular.gt(), bold.gt()
	if len(a.Coords()) == 0 || len(b.Coords()) == 0 {
		t.Skip("system UI font is not variable")
	}
	if regular.ras.(*ctRaster).tables != bold.ras.(*ctRaster).tables {
		t.Skip("weights use distinct font files")
	}
	if a == b {
		t.Fatal("variable weights share a mutable face")
	}
	if a.Font != b.Font {
		t.Fatal("weights do not share one immutable parsed font")
	}
	if len(a.Font.GSUB.Lookups) == 0 || len(b.Font.GSUB.Lookups) == 0 {
		t.Fatal("system font lacks expected substitution tables")
	}
	if &a.Font.GSUB.Lookups[0] != &b.Font.GSUB.Lookups[0] {
		t.Fatal("weights reparse and duplicate immutable substitution tables")
	}
	if len(a.Font.GPOS.Lookups) == 0 || len(b.Font.GPOS.Lookups) == 0 || &a.Font.GPOS.Lookups[0] != &b.Font.GPOS.Lookups[0] {
		t.Fatal("weights duplicate immutable positioning tables")
	}
	aCoords := append(a.Coords()[:0:0], a.Coords()...)
	bCoords := append(b.Coords()[:0:0], b.Coords()...)
	ax, ay := a.Ppem()
	bx, by := b.Ppem()
	defer func() { b.SetCoords(bCoords); b.SetPpem(bx, by) }()
	changed := append(bCoords[:0:0], bCoords...)
	changed[0] += 100
	b.SetCoords(changed)
	b.SetPpem(27, 27)
	if !reflect.DeepEqual(a.Coords(), aCoords) {
		t.Fatal("one weight changed another's variation state")
	}
	x, y := a.Ppem()
	if x != ax || y != ay {
		t.Fatal("one face changed another's pixel metrics")
	}
}

func TestCoreTextSharedTablesShapeLikeIndependentFonts(t *testing.T) {
	coreTextOn(t)
	sharedShapers := map[*font.Face]*shaping.HarfbuzzShaper{}
	var referenceShaper shaping.HarfbuzzShaper
	for _, w := range []Weight{Bold, Regular, Light, Semibold, Medium, Regular, Bold} {
		for _, italic := range []bool{false, true} {
			s := "AVATAR office ffi 0123 −1 +1"
			l := Shape(s, Style{Size: 16, Weight: w, Italic: italic}, Options{NoWrap: true})
			f := l.Glyphs[0].Src.(*Face)
			shared := f.gt()
			fn, err := font.NewFont(f.ras.(*ctRaster).tables)
			if err != nil {
				t.Fatal(err)
			}
			reference := font.NewFace(fn)
			reference.SetCoords(append(shared.Coords()[:0:0], shared.Coords()...))
			x, y := shared.Ppem()
			reference.SetPpem(x, y)
			for _, size := range []int{11, 16, 56} {
				in := shaping.Input{Text: []rune(s), RunEnd: len([]rune(s)), Face: shared, Size: fixed.I(size), Direction: di.DirectionLTR, Script: language.Latin, Language: language.NewLanguage("en")}
				sharedShaper := sharedShapers[shared]
				if sharedShaper == nil {
					sharedShaper = &shaping.HarfbuzzShaper{}
					sharedShapers[shared] = sharedShaper
				}
				got := sharedShaper.Shape(in)
				in.Face = reference
				want := referenceShaper.Shape(in)
				if !reflect.DeepEqual(got.Glyphs, want.Glyphs) || got.Advance != want.Advance {
					t.Fatalf("w%d italic=%v size=%d: shared tables changed glyphs, clusters, offsets or advances", w, italic, size)
				}
			}
		}
	}
}
