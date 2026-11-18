//go:build !darwin || ios || !tango_coretext

package text

import (
	"encoding/binary"
	"math"
	"reflect"
	"runtime"
	"testing"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

func TestMetricFaceAvoidsWholeFontCopies(t *testing.T) {
	clearCache()
	l := Shape("AVATAR office 0123", Style{Size: 16}, Options{NoWrap: true})
	if len(l.Glyphs) == 0 {
		t.Skip("no font")
	}
	f := l.Glyphs[0].Src.(*Face)
	source, ok := f.ras.(interface{ shapingTables() *ot.Loader })
	if !ok || source.shapingTables() == nil || len(f.gt().Coords()) == 0 {
		t.Skip("no variable system font tables")
	}
	ld := source.shapingTables()
	raw, err := ld.RawTable(ot.NewTag('H', 'V', 'A', 'R'))
	if err != nil {
		t.Skip("no HVAR table")
	}
	if _, changed := specializeHVAR(raw, f.gt().Coords()); !changed {
		t.Skip("font has no zero-contribution HVAR regions")
	}
	var tableBytes int64
	for _, tag := range ld.Tables() {
		raw, err := ld.RawTable(tag)
		if err != nil {
			t.Fatal(err)
		}
		tableBytes += int64(len(raw))
	}
	parse := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			fn, err := font.NewFont(ld)
			if err != nil {
				b.Fatal(err)
			}
			runtime.KeepAlive(fn)
		}
	})
	specialize := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			runtime.KeepAlive(specializeMetricFace(f.gt(), ld))
		}
	})
	// Allow table reads plus 1 MiB for HVAR pruning/encoding, on top of
	// the parser itself. Concatenating/checksumming a second complete font
	// needlessly adds several more MiB on the system variable font.
	limit := parse.AllocedBytesPerOp() + tableBytes + 1<<20
	if got := specialize.AllocedBytesPerOp(); got > limit {
		t.Fatalf("specializing metrics allocates %d bytes, want <= %d (parser %d + tables %d + HVAR budget)", got, limit, parse.AllocedBytesPerOp(), tableBytes)
	}
}

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
		// Cross the specialization boundary after every coordinate change,
		// checking both the original and specialized metric paths.
		for range 80 {
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
}
