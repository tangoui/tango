//go:build !darwin || ios || !tango_coretext

package text

import (
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
)

// pruneZeroRegions specializes an immutable variation store for one fixed
// coordinate vector. It removes strictly zero contributions, leaving every
// surviving term in its original order (no rounding, merging or epsilon).
// Tables of other font instances are never modified.
func pruneZeroRegions(store tables.ItemVarStore, coords []tables.Coord) (tables.ItemVarStore, bool) {
	regions := store.VariationRegionList.VariationRegions
	if len(regions) == 0 || len(coords) == 0 {
		return store, false
	}
	zero := make([]bool, len(regions))
	anyZero := false
	for i, r := range regions {
		if len(r.RegionAxes) < len(coords) {
			return store, false
		}
		zero[i] = r.Evaluate(coords) == 0
		anyZero = anyZero || zero[i]
	}
	if !anyZero {
		return store, false
	}
	out := store
	changed := false
	for i, data := range store.ItemVariationDatas {
		var keep []int
		for col, index := range data.RegionIndexes {
			if int(index) >= len(zero) {
				return store, false
			}
			if !zero[index] {
				keep = append(keep, col)
			}
		}
		if len(keep) == len(data.RegionIndexes) {
			continue
		}
		if !changed {
			out.ItemVariationDatas = append([]tables.ItemVariationData(nil), store.ItemVariationDatas...)
			changed = true
		}
		dst := data
		dst.RegionIndexes = make([]uint16, len(keep))
		dst.DeltaSets = make([][]int16, len(data.DeltaSets))
		for j, col := range keep {
			dst.RegionIndexes[j] = data.RegionIndexes[col]
		}
		for row, values := range data.DeltaSets {
			if len(values) < len(data.RegionIndexes) {
				return store, false
			}
			dst.DeltaSets[row] = make([]int16, len(keep))
			for j, col := range keep {
				dst.DeltaSets[row][j] = values[col]
			}
		}
		out.ItemVariationDatas[i] = dst
	}
	return out, changed
}

// fixedShapingFace keeps glyph metrics and variation coordinates unchanged,
// but avoids evaluating GDEF regions that contribute exactly zero to this
// immutable Face. Font and Face are copied; the source and all shared tables
// remain untouched. Cmap/extents caches describe the same glyphs/coordinates.
func fixedShapingFace(source *font.Face, raw ...*ot.Loader) *font.Face {
	if len(raw) > 0 && raw[0] != nil {
		source = specializeMetricFace(source, raw[0])
	}

	store, changed := pruneZeroRegions(source.Font.GDEF.ItemVarStore, source.Coords())
	if !changed {
		return source
	}
	ft := *source.Font
	ft.GDEF.ItemVarStore = store
	face := *source
	face.Font = &ft
	return &face
}
