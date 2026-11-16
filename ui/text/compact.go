package text

import (
	"slices"
	"unicode"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/harfbuzz"
	"github.com/go-text/typesetting/shaping"
)

// compactShaper uses HarfBuzz's public buffer API for horizontal layout.
// Layout consumes only glyph ids, clusters, advances and offsets; constructing
// a general shaping.Output (including glyph ink extents, per-cluster counts and
// line bounds) only to copy these few fields causes work and allocations for
// every changing label. Ink extents still come from the renderer as before.
// The font, scale, context and OpenType features are identical to HarfbuzzShaper.
// Caller serializes access, just as for the shared shaping.HarfbuzzShaper.
type compactShaper struct {
	buffer   *harfbuzz.Buffer
	fonts    map[*font.Face]shapingFont
	features []harfbuzz.Feature
}

// The source Face can change coordinates or ppem through go-text's API.
// A specialized shaping font is valid only for that exact instance state.
type shapingFont struct {
	font         *harfbuzz.Font
	coords       []tables.Coord
	xPpem, yPpem uint16
	specialized  bool
	uses         int
}

const hotShapingUses = 64

func (s *compactShaper) append(dst []sg, in shaping.Input, face *Face, run int32) []sg {
	if s.buffer == nil {
		s.buffer = harfbuzz.NewBuffer()
	} else {
		s.buffer.Clear()
	}
	b := s.buffer
	b.AddRunes(in.Text, in.RunStart, in.RunEnd-in.RunStart)
	b.Props.Direction = in.Direction.Harfbuzz()
	b.Props.Script = in.Script
	b.Props.Language = in.Language
	entry, ok := s.fonts[in.Face]
	xPpem, yPpem := in.Face.Ppem()
	coords := in.Face.Coords()
	if !ok || entry.xPpem != xPpem || entry.yPpem != yPpem || !slices.Equal(entry.coords, coords) {
		// Retain only a bounded set of immutable face instances. In particular,
		// distinct variable-font instances must never share a shaping font.
		if s.fonts == nil || len(s.fonts) >= 16 {
			s.fonts = make(map[*font.Face]shapingFont)
		}
		entry = shapingFont{font: harfbuzz.NewFont(fixedShapingFace(in.Face)), coords: append([]tables.Coord(nil), coords...), xPpem: xPpem, yPpem: yPpem}
	}
	// Specializing HVAR re-parses the whole font (a few MiB per instance),
	// so only faces shaped often get it; the glyphs are identical either way.
	if entry.uses++; !entry.specialized && entry.uses > hotShapingUses {
		entry.specialized = true
		var raw *ot.Loader
		if source, ok := face.ras.(interface{ shapingTables() *ot.Loader }); ok {
			raw = source.shapingTables()
		}
		if raw != nil {
			entry.font = harfbuzz.NewFont(fixedShapingFace(in.Face, raw))
		}
	}
	s.fonts[in.Face] = entry
	f := entry.font
	f.XScale = int32(in.Size.Ceil()) * 64
	f.YScale = f.XScale
	s.features = slices.Grow(s.features[:0], len(in.FontFeatures))[:len(in.FontFeatures)]
	for i, feature := range in.FontFeatures {
		s.features[i] = harfbuzz.Feature{Tag: feature.Tag, Value: feature.Value, Start: harfbuzz.FeatureGlobalStart, End: harfbuzz.FeatureGlobalEnd}
	}
	b.Shape(f, s.features)
	dst = slices.Grow(dst, len(b.Info))
	for i, g := range b.Info {
		id := uint32(g.Glyph)
		// Match the general shaper's missing-space handling, without building
		// full cluster metadata for glyphs that do not need it.
		if id == 0 && g.Cluster >= 0 && g.Cluster < len(in.Text) && unicode.Is(unicode.Zs, in.Text[g.Cluster]) {
			single := (i == 0 || b.Info[i-1].Cluster != g.Cluster) && (i+1 == len(b.Info) || b.Info[i+1].Cluster != g.Cluster)
			end := in.RunEnd
			if in.Direction.Progression() == di.FromTopLeft {
				if i+1 < len(b.Info) {
					end = b.Info[i+1].Cluster
				}
			} else if i > 0 {
				end = b.Info[i-1].Cluster
			}
			if single && end-g.Cluster == 1 {
				id = uint32(font.EmptyGlyph)
			}
		}
		p := b.Pos[i]
		dst = append(dst, sg{face: face, gid: id, adv: float32(p.XAdvance) / 64, xoff: float32(p.XOffset) / 64, yoff: float32(p.YOffset) / 64, cluster: g.Cluster, run: run})
	}
	return dst
}
