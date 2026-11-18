package quartz

import (
	"image"

	"github.com/tangoui/tango/internal/nativeglyph"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

func (s *scene) textItem(op paint.Text, clip image.Rectangle) error {
	if op.Color.A == 0 {
		return nil
	}
	native := s.provider != nil
	for _, g := range op.Glyphs {
		if g.Src != nil && (g.ID > 65535 || s.provider == nil || !s.provider.Available(g.Src)) {
			native = false
			break
		}
	}
	if !native {
		return s.rasterText(op, clip)
	}
	var runs []nativeglyph.Run
	var bounds image.Rectangle
	for i := 0; i < len(op.Glyphs); {
		first := op.Glyphs[i]
		if first.Src == nil {
			i++
			continue
		}
		j := i + 1
		for j < len(op.Glyphs) {
			g := op.Glyphs[j]
			if g.Src != first.Src || g.Size != first.Size || g.Skew != first.Skew {
				break
			}
			j++
		}
		run := nativeglyph.Run{Source: first.Src, Size: first.Size * s.scale, Skew: first.Skew, IDs: make([]uint16, j-i), Positions: make([]geom.Point, j-i)}
		for n, g := range op.Glyphs[i:j] {
			x, y, sub := paint.GlyphPlacement(op.Origin, g, s.scale)
			run.IDs[n] = uint16(g.ID)
			run.Positions[n] = geom.Pt(float32(x)+float32(sub)/4, -float32(y))
		}
		b, ok := s.provider.Bounds(run)
		if !ok {
			return s.rasterText(op, clip)
		}
		bounds = bounds.Union(b)
		runs = append(runs, run)
		i = j
	}
	frame := image.Rect(bounds.Min.X, -bounds.Max.Y, bounds.Max.X, -bounds.Min.Y).Intersect(clip).Intersect(s.bounds)
	if frame.Empty() {
		return nil
	}
	k := keyData{}
	k.tag(byte(itemText))
	k.rgba(op.Color)
	k.f(s.scale)
	k.i(frame.Dx())
	k.i(frame.Dy())
	k.i(len(runs))
	for _, run := range runs {
		k.u64(uint64(s.provider.Key(run.Source)))
		k.f(run.Size)
		k.f(run.Skew)
		k.i(len(run.IDs))
		for i, id := range run.IDs {
			run.Positions[i].X -= float32(frame.Min.X)
			run.Positions[i].Y += float32(frame.Max.Y)
			k.u64(uint64(id))
			k.f(run.Positions[i].X)
			k.f(run.Positions[i].Y)
		}
	}
	key := string(k)
	s.items = append(s.items, item{kind: itemText, key: key, hash: keyHash(key), frame: frame, color: op.Color, runs: runs, clip: clip, op: op})
	return nil
}
func (s *scene) rasterText(op paint.Text, clip image.Rectangle) error {
	var b image.Rectangle
	for _, g := range op.Glyphs {
		if g.Src == nil {
			continue
		}
		x, y, sub := paint.GlyphPlacement(op.Origin, g, s.scale)
		mask, off := s.cpu.GlyphMask(g, g.Size*s.scale, sub)
		if mask != nil {
			b = b.Union(mask.Rect.Add(image.Pt(x, y).Add(off)))
		}
	}
	b = b.Intersect(clip).Intersect(s.bounds)
	if b.Empty() {
		return nil
	}
	key, refs, e := s.patchKey(op, b, clip)
	if e != nil {
		return e
	}
	// Only the scene's immediate raster call borrows the original glyph slice.
	// Retained resources keep their own source references, not the caller's slice.
	s.items = append(s.items, item{kind: itemPatch, key: key, frame: b, op: op, clip: clip, refs: refs})
	return nil
}
