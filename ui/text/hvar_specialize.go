package text

import (
	"encoding/binary"
	"sort"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
)

// encodeVariationStore writes an OpenType ItemVariationStore using signed
// 16-bit deltas. Index order and all delta rows remain unchanged; only zero
// contribution columns have been removed. No font metrics are quantized.
func encodeVariationStore(s tables.ItemVarStore) []byte {
	regions := s.VariationRegionList.VariationRegions
	if len(regions) == 0 || len(regions) > 65535 || len(s.ItemVariationDatas) > 65535 {
		return nil
	}
	axes := len(regions[0].RegionAxes)
	if axes > 65535 {
		return nil
	}
	header := 8 + 4*len(s.ItemVariationDatas)
	out := make([]byte, header)
	binary.BigEndian.PutUint16(out, 1)
	binary.BigEndian.PutUint32(out[2:], uint32(header))
	binary.BigEndian.PutUint16(out[6:], uint16(len(s.ItemVariationDatas)))
	put := func(v uint16) { out = binary.BigEndian.AppendUint16(out, v) }
	put(uint16(axes))
	put(uint16(len(regions)))
	for _, r := range regions {
		if len(r.RegionAxes) != axes {
			return nil
		}
		for _, a := range r.RegionAxes {
			put(uint16(a.StartCoord))
			put(uint16(a.PeakCoord))
			put(uint16(a.EndCoord))
		}
	}
	for i, data := range s.ItemVariationDatas {
		n := len(data.RegionIndexes)
		// wordDeltaCount's top bit selects 32-bit storage; keep it clear.
		if n > 32767 || len(data.DeltaSets) > 65535 {
			return nil
		}
		binary.BigEndian.PutUint32(out[8+4*i:], uint32(len(out)))
		put(uint16(len(data.DeltaSets)))
		put(uint16(n))
		put(uint16(n))
		for _, r := range data.RegionIndexes {
			put(r)
		}
		for _, row := range data.DeltaSets {
			if len(row) != n {
				return nil
			}
			for _, v := range row {
				put(uint16(v))
			}
		}
	}
	return out
}

// specializeHVAR keeps the original glyph-to-delta mappings and substitutes
// a store specialized to the selected normalized coordinates. All offsets
// to mapping data remain valid; the new store is appended at a new offset.
func specializeHVAR(raw []byte, coords []tables.Coord) ([]byte, bool) {
	h, _, err := tables.ParseHVAR(raw)
	if err != nil || len(raw) < 20 {
		return raw, false
	}
	store, changed := pruneZeroRegions(h.ItemVariationStore, coords)
	if !changed {
		return raw, false
	}
	encoded := encodeVariationStore(store)
	if len(encoded) == 0 {
		return raw, false
	}
	out := make([]byte, len(raw), len(raw)+len(encoded))
	copy(out, raw)
	binary.BigEndian.PutUint32(out[4:], uint32(len(raw)))
	out = append(out, encoded...)
	return out, true
}

// specializeMetricFace uses the public font loader to replace only HVAR's
// zero-contribution region columns. The raw-table route is necessary because
// go-text's loaded HVAR is private. No library fork or private-field access.
// A new independent face is returned only when the table can be specialized.
func specializeMetricFace(source *font.Face, loader *ot.Loader) *font.Face {
	if loader == nil || len(source.Coords()) == 0 {
		return source
	}
	raw, err := loader.RawTable(ot.NewTag('H', 'V', 'A', 'R'))
	if err != nil {
		return source
	}
	hvar, changed := specializeHVAR(raw, source.Coords())
	if !changed {
		return source
	}
	var rawTables []ot.Table
	for _, tag := range loader.Tables() {
		data, err := loader.RawTable(tag)
		if err != nil {
			return source
		}
		if tag == ot.NewTag('H', 'V', 'A', 'R') {
			data = hvar
		}
		rawTables = append(rawTables, ot.Table{Tag: tag, Content: data})
	}
	sort.Slice(rawTables, func(i, j int) bool { return rawTables[i].Tag < rawTables[j].Tag })
	data, err := shapingTableResource(rawTables)
	if err != nil {
		return source
	}
	ld, err := ot.NewLoader(data)
	if err != nil {
		return source
	}
	fn, err := font.NewFont(ld)
	if err != nil {
		return source
	}
	f := font.NewFace(fn)
	f.SetCoords(append([]tables.Coord(nil), source.Coords()...))
	x, y := source.Ppem()
	f.SetPpem(x, y)
	return f
}
