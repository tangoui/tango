package text

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

func TestFixedFontPreservesAllMetricsAndDeltas(t *testing.T) {
	clearCache()
	l := Shape("12.34 office", Style{Size: 11}, Options{NoWrap: true})
	if len(l.Glyphs) == 0 {
		t.Skip("no font")
	}
	f := l.Glyphs[0].Src.(*Face)
	source, ok := f.ras.(interface{ shapingTables() *ot.Loader })
	if !ok || source.shapingTables() == nil {
		t.Skip("no raw system tables")
	}
	original := f.gt()
	optimized := fixedShapingFace(original, source.shapingTables())
	maxp, err := source.shapingTables().RawTable(ot.NewTag('m', 'a', 'x', 'p'))
	if err != nil || len(maxp) < 6 {
		t.Fatal("maxp unavailable")
	}
	for gid := range int(binary.BigEndian.Uint16(maxp[4:6])) {
		a, b := original.HorizontalAdvance(font.GID(gid)), optimized.HorizontalAdvance(font.GID(gid))
		if math.Float32bits(a) != math.Float32bits(b) {
			t.Fatalf("glyph%d metrics changed: %v -> %v", gid, a, b)
		}
	}
	store := original.Font.GDEF.ItemVarStore
	for outer, d := range store.ItemVariationDatas {
		for inner := range d.DeltaSets {
			ix := tables.VariationStoreIndex{DeltaSetOuter: uint16(outer), DeltaSetInner: uint16(inner)}
			a, b := store.GetDelta(ix, original.Coords()), optimized.Font.GDEF.ItemVarStore.GetDelta(ix, optimized.Coords())
			if math.Float32bits(a) != math.Float32bits(b) {
				t.Fatalf("GDEF item%d/%d changed", outer, inner)
			}
		}
	}
}

func TestCompactRebuildsFixedFontAfterInstanceChanges(t *testing.T) {
	l := Shape("12.34", Style{Size: 11}, Options{NoWrap: true})
	if len(l.Glyphs) == 0 {
		t.Skip("no font")
	}
	f := l.Glyphs[0].Src.(*Face)
	original := font.NewFace(f.gt().Font)
	coords := append([]tables.Coord(nil), f.gt().Coords()...)
	if len(coords) == 0 {
		t.Skip("not a variable font")
	}
	var compact compactShaper
	var reference shaping.HarfbuzzShaper
	for _, change := range []bool{false, true, false} {
		next := append([]tables.Coord(nil), coords...)
		if change {
			next[0] = 4096
		}
		original.SetCoords(next)
		original.SetPpem(11, 11)
		text := []rune("12.34 AVATAR office")
		in := shaping.Input{Text: text, RunEnd: len(text), Face: original, Size: fixed.I(11), Direction: di.DirectionLTR, Script: language.Latin, Language: language.NewLanguage("en")}
		old := reference.Shape(in)
		got := compact.append(nil, in, f, 0)
		if len(got) != len(old.Glyphs) {
			t.Fatal("glyph count changed")
		}
		for i, g := range old.Glyphs {
			want := sg{face: f, gid: uint32(g.GlyphID), adv: fx(g.XAdvance), xoff: fx(g.XOffset), yoff: fx(g.YOffset), cluster: g.ClusterIndex}
			if !reflect.DeepEqual(got[i], want) {
				t.Fatalf("instance change %v glyph%d: %+v != %+v", change, i, got[i], want)
			}
		}
	}
}
