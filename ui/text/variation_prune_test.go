//go:build !darwin || ios || tango_gotext

package text

import (
	"github.com/go-text/typesetting/font/opentype/tables"
	"math"
	"reflect"
	"testing"
)

func TestPruneZeroVariationRegionsPreservesDeltas(t *testing.T) {
	region := func(start, peak, end int16) tables.VariationRegion {
		return tables.VariationRegion{RegionAxes: []tables.RegionAxisCoordinates{{StartCoord: tables.Coord(start), PeakCoord: tables.Coord(peak), EndCoord: tables.Coord(end)}}}
	}
	original := tables.ItemVarStore{
		VariationRegionList: tables.VariationRegionList{VariationRegions: []tables.VariationRegion{region(-16384, -8192, 0), region(0, 8192, 16384), region(-16384, 0, 16384)}},
		ItemVariationDatas:  []tables.ItemVariationData{{RegionIndexes: []uint16{0, 1, 2}, DeltaSets: [][]int16{{20, 40, 5}, {-7, 11, -3}}}, {RegionIndexes: []uint16{0}, DeltaSets: [][]int16{{1}, {-2}}}},
	}
	before := []uint16{0, 1, 2}
	coords := []tables.Coord{4096}
	pruned, changed := pruneZeroRegions(original, coords)
	if !changed || len(pruned.ItemVariationDatas[0].RegionIndexes) != 2 || len(pruned.ItemVariationDatas[1].RegionIndexes) != 0 {
		t.Fatal("zero region columns were not removed")
	}
	for outer, data := range original.ItemVariationDatas {
		for inner := range data.DeltaSets {
			idx := tables.VariationStoreIndex{DeltaSetOuter: uint16(outer), DeltaSetInner: uint16(inner)}
			if math.Float32bits(original.GetDelta(idx, coords)) != math.Float32bits(pruned.GetDelta(idx, coords)) {
				t.Fatal("variation delta changed")
			}
		}
	}
	if !reflect.DeepEqual(original.ItemVariationDatas[0].RegionIndexes, before) || len(original.ItemVariationDatas[0].DeltaSets[0]) != 3 {
		t.Fatal("shared font tables were mutated")
	}
}
