//go:build !darwin || ios || tango_gotext

package text

import (
	"encoding/binary"
	"github.com/go-text/typesetting/font/opentype/tables"
	"math"
	"testing"
)

func TestSpecializedHVARPreservesEveryAdvanceDelta(t *testing.T) {
	region := func(start, peak, end int16) tables.VariationRegion {
		return tables.VariationRegion{RegionAxes: []tables.RegionAxisCoordinates{{StartCoord: tables.Coord(start), PeakCoord: tables.Coord(peak), EndCoord: tables.Coord(end)}}}
	}
	store := tables.ItemVarStore{VariationRegionList: tables.VariationRegionList{VariationRegions: []tables.VariationRegion{region(-16384, -8192, 0), region(0, 8192, 16384)}}, ItemVariationDatas: []tables.ItemVariationData{{RegionIndexes: []uint16{0, 1}, DeltaSets: [][]int16{{100, 20}, {-25, 13}, {0, -7}}}}}
	raw := make([]byte, 20)
	binary.BigEndian.PutUint16(raw, 1)
	binary.BigEndian.PutUint32(raw[4:], 20)
	raw = append(raw, encodeVariationStore(store)...)
	a, _, err := tables.ParseHVAR(raw)
	if err != nil {
		t.Fatal(err)
	}
	coords := []tables.Coord{4096}
	fixed, ok := specializeHVAR(raw, coords)
	if !ok {
		t.Fatal("HVAR specialization was not applied")
	}
	b, _, err := tables.ParseHVAR(fixed)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		before, after := a.AdvanceDelta(tables.GlyphID(i), coords), b.AdvanceDelta(tables.GlyphID(i), coords)
		if math.Float32bits(before) != math.Float32bits(after) {
			t.Fatalf("glyph%d advance delta %v -> %v", i, before, after)
		}
	}
	if len(a.ItemVariationStore.ItemVariationDatas[0].RegionIndexes) != 2 {
		t.Fatal("source store was changed")
	}
}
