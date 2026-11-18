//go:build !darwin || ios || !tango_coretext

package text

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/tangoui/tango/ui/paint"
)

// Faces backed by go-text: parsed font tables shape (HarfBuzz port) and,
// for pure-Go faces, supply the outlines paint rasterizes.

// faceImpl is the engine part of Face.
type faceImpl struct {
	face *font.Face
	upem float32
	asc  float32 // font units, positive
	desc float32 // font units, positive

	unitPath func(id uint32) *paint.Path // outline in font units when go-text has none

	// Lazily built engine faces (newLazyEngineFace).
	build func() (gt *font.Face, ras glyphEngine, unitPath func(uint32) *paint.Path)
}

// ready builds a lazy face on first use; a no-op for faces built eagerly.
func (f *Face) ready() {
	f.once.Do(func() {
		build := f.build
		if build == nil {
			return
		}
		f.build = nil
		gt, ras, unitPath := build()
		if gt == nil {
			f.broken = true
			return
		}
		f.setFont(gt)
		f.ras, f.unitPath = ras, unitPath
	})
}

// gt is the go-text face, built on first use.
func (f *Face) gt() *font.Face {
	f.ready()
	return f.face
}

func newFace(family string, w Weight, f *font.Face) *Face {
	fc := &Face{Family: family, Weight: w, paths: map[uint32]*paint.Path{}}
	fc.setFont(f)
	return fc
}

// setFont sets the go-text face and the metrics read from it.
func (fc *Face) setFont(f *font.Face) {
	fc.face, fc.upem = f, float32(f.Upem())
	if ext, ok := f.FontHExtents(); ok {
		fc.asc = ext.Ascender
		fc.desc = -ext.Descender
	} else {
		fc.asc, fc.desc = fc.upem*0.8, fc.upem*0.2
	}
}

// metricsUnits returns the units per em and the ascent and descent (font
// units, positive), building a lazy face first.
func (f *Face) metricsUnits() (upem, asc, desc float32) {
	f.ready()
	return f.upem, f.asc, f.desc
}

// hasRune reports whether the face maps r.
func (f *Face) hasRune(r rune) bool {
	_, ok := f.gt().NominalGlyph(r)
	return ok
}

// unitOutline returns the outline of glyph id in font units, y down (nil
// when the glyph has none).
func (f *Face) unitOutline(id uint32) *paint.Path {
	if f.unitPath != nil {
		return f.unitPath(id)
	}
	o, ok := f.gt().GlyphDataOutline(font.GID(id))
	if !ok || len(o.Segments) == 0 {
		return nil
	}
	p := paint.NewPath()
	open := false
	for _, s := range o.Segments {
		a := s.Args
		switch s.Op {
		case ot.SegmentOpMoveTo:
			if open {
				p.Close()
			}
			p.MoveTo(a[0].X, -a[0].Y)
			open = true
		case ot.SegmentOpLineTo:
			p.LineTo(a[0].X, -a[0].Y)
		case ot.SegmentOpQuadTo:
			p.QuadTo(a[0].X, -a[0].Y, a[1].X, -a[1].Y)
		case ot.SegmentOpCubeTo:
			p.CubeTo(a[0].X, -a[0].Y, a[1].X, -a[1].Y, a[2].X, -a[2].Y)
		}
	}
	if open {
		p.Close()
	}
	return p
}

func (s sysFont) family() *family {
	return &family{name: s.name, load: func() []*Face {
		for _, p := range s.paths {
			if p == "" {
				continue
			}
			f, err := os.Open(p)
			if err != nil {
				continue
			}
			faces, err := parseFaceResource(s.name, f, strings.Join(s.families, "\x00"))
			if err != nil || len(faces) == 0 {
				f.Close()
				continue
			}
			// The file stays open: go-text reads tables lazily via ReadAt.
			for _, x := range s.extra {
				if xf, err := os.Open(x); err == nil {
					if more, err := parseFaceResource(s.name, xf, strings.Join(s.families, "\x00")); err == nil && len(more) > 0 {
						faces = append(faces, more...)
					} else {
						xf.Close()
					}
				}
			}
			return faces
		}
		return nil
	}}
}

// parseFaces parses a font file (TTF, OTF, TTC) held in memory
// (RegisterFont).
func parseFaces(name string, data []byte) ([]*Face, error) {
	return parseFaceResource(name, bytes.NewReader(data), "")
}

// parseFaceResource parses the faces of a font file; keep, when not empty,
// lists the families to keep ("\x00"-separated).
func parseFaceResource(name string, r font.Resource, keep string) ([]*Face, error) {
	lds, err := ot.NewLoaders(r)
	if err != nil {
		return nil, fmt.Errorf("text: parse font %s: %w", name, err)
	}
	var keepSet map[string]bool
	if keep != "" {
		keepSet = map[string]bool{}
		for _, k := range strings.Split(keep, "\x00") {
			keepSet[k] = true
		}
	}
	var out []*Face
	type wk struct {
		w      Weight
		italic bool
	}
	seen := map[wk]bool{}
	for _, ld := range lds {
		d, _ := font.Describe(ld, nil)
		if keepSet != nil && !keepSet[d.Family] {
			continue
		}
		italic := d.Aspect.Style == font.StyleItalic // italic or oblique
		if d.Aspect.Style != font.StyleNormal && d.Aspect.Style != 0 && !italic {
			continue
		}
		fn, err := font.NewFont(ld)
		if err != nil {
			continue
		}
		probe := font.NewFace(fn)
		probe.SetVariations([]font.Variation{{Tag: ot.MustNewTag("wght"), Value: 400}})
		if len(probe.Coords()) > 0 {
			for _, w := range []Weight{Light, Regular, Medium, Semibold, Bold} {
				if seen[wk{w, italic}] {
					continue
				}
				// Each weight needs its own *font.Font: go-text's
				// HarfbuzzShaper caches its shaping font per *font.Font, so
				// faces sharing one would all shape (and measure) with the
				// variation coordinates of whichever was shaped first.
				fw, err := font.NewFont(ld)
				if err != nil {
					continue
				}
				fc := font.NewFace(fw)
				vars := []font.Variation{{Tag: ot.MustNewTag("wght"), Value: float32(w)}, {Tag: ot.MustNewTag("opsz"), Value: 16}}
				fc.SetVariations(vars)
				f := newFace(name, w, fc)
				f.Italic = italic
				out = append(out, f)
				seen[wk{w, italic}] = true
			}
			continue
		}
		w := Weight(d.Aspect.Weight)
		if w == 0 {
			w = Regular
		}
		w = Weight((int(w) + 50) / 100 * 100)
		if seen[wk{w, italic}] {
			continue
		}
		seen[wk{w, italic}] = true
		f := newFace(name, w, font.NewFace(fn))
		f.Italic = italic
		out = append(out, f)
	}
	return out, nil
}

// newEngineFace wraps an engine face: gt shapes (same tables the engine
// renders), ras rasterizes, unitPath (optional) supplies outlines in font
// units when go-text cannot read them.
func newEngineFace(family string, w Weight, italic bool, gt *font.Face, ras glyphEngine, unitPath func(uint32) *paint.Path) *Face {
	f := newFace(family, w, gt)
	f.Italic = italic
	f.ras = ras
	f.unitPath = unitPath
	return f
}

// newLazyEngineFace is newEngineFace for a face built on first use: build
// returns what newEngineFace takes (gt nil when the face cannot be built).
// Loading a family registers every style it has; a UI uses a few of them,
// and parsing a face's tables (a CJK font's character map, a variable
// font's variation data) costs megabytes.
func newLazyEngineFace(family string, w Weight, italic bool, build func() (*font.Face, glyphEngine, func(uint32) *paint.Path)) *Face {
	return &Face{Family: family, Weight: w, Italic: italic, paths: map[uint32]*paint.Path{}, faceImpl: faceImpl{build: build}}
}

// sfntFromTables builds an in-memory OpenType file from tables (tag →
// data), skipping glyph data the shaper does not need (outlines and
// bitmaps: the engine renders those).
func sfntFromTables(tabs map[uint32][]byte) []byte {
	var list []ot.Table
	for tag, data := range tabs {
		if skipTable(tag) {
			continue
		}
		list = append(list, ot.Table{Tag: ot.Tag(tag), Content: data})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Tag < list[j].Tag })
	return ot.WriteTTF(list)
}

// goTextFace parses an in-memory font (one face) for shaping.
func goTextFace(data []byte) (*font.Face, *ot.Loader, error) {
	ld, err := ot.NewLoader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	fn, err := font.NewFont(ld)
	if err != nil {
		return nil, nil, err
	}
	return font.NewFace(fn), ld, nil
}

// fvarAxes returns the variation axes of a font (nil when not variable).
func fvarAxes(ld *ot.Loader) []tables.VariationAxisRecord {
	raw, err := ld.RawTable(ot.MustNewTag("fvar"))
	if err != nil {
		return nil
	}
	fv, _, err := tables.ParseFvar(raw)
	if err != nil {
		return nil
	}
	return fv.Axis
}

// setDesignCoords sets the variation of face to the engine's axis values
// (tag → design value; missing axes keep their default), so shaping uses
// the instance the engine draws.
func setDesignCoords(face *font.Face, axes []tables.VariationAxisRecord, vals map[uint32]float32) {
	if len(axes) == 0 {
		return
	}
	coords := make([]float32, len(axes))
	for i, a := range axes {
		coords[i] = float32(a.Default)
		if v, ok := vals[uint32(a.Tag)]; ok {
			coords[i] = v
		}
	}
	face.SetCoords(face.Font.NormalizeVariations(coords))
}
