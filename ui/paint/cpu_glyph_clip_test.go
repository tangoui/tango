package paint

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

func glyphClipFixture() (*CPU, Text) {
	r := NewCPU()
	r.prepare(1)
	r.img = image.NewRGBA(image.Rect(0, 0, 32, 24))
	r.cur, r.clip, r.frame, r.ident = r.img, r.img.Rect, r.img.Rect, true
	r.cmask = image.NewAlpha(image.Rect(0, 0, 30, 20))
	r.cmaskAt = image.Pt(2, 1)
	for i := range r.cmask.Pix {
		r.cmask.Pix[i] = 255
	}
	o := Text{Origin: geom.Pt(10, 12), Glyphs: []Glyph{{Src: &borderSrc{}, ID: 1, Size: 10}}, Color: color.NRGBA{R: 190, G: 40, B: 80, A: 137}}
	r.glyph(o.Glyphs[0], 10, 0) // warm the shared glyph mask
	return r, o
}

// Recomputing a mask for a cached glyph wholly inside an opaque clip is
// unnecessary. It should allocate no more than the ordinary unclipped path,
// regardless of how many glyphs the text holds.
func TestCPUTextOpaqueClipReusesGlyphMasks(t *testing.T) {
	r, o := glyphClipFixture()
	o.Glyphs = append(o.Glyphs, o.Glyphs[0], o.Glyphs[0])
	clip := r.cmask
	r.cmask = nil
	plain := testing.AllocsPerRun(25, func() { r.text(o) })
	r.cmask = clip
	allocs := testing.AllocsPerRun(25, func() { r.text(o) })
	if allocs > plain {
		t.Fatalf("cached text in opaque clip allocated %.0f times, unclipped %.0f; want no extra masks", allocs, plain)
	}
}

// Multiplying masks is the existing clipping contract. Derive the expected
// alpha bytes independently, then let image/draw perform the same source-over
// blend, including partial glyph coverage and translucent ink/background.
func TestCPUTextClipMatchesMultipliedMasks(t *testing.T) {
	for _, partial := range []bool{false, true} {
		for _, origin := range []geom.Point{geom.Pt(10, 12), geom.Pt(2, 4), geom.Pt(31, 22)} {
			r, o := glyphClipFixture()
			o.Origin = origin
			if partial {
				for i := range r.cmask.Pix {
					if i%3 == 0 {
						r.cmask.Pix[i] = 127
					}
				}
			}
			draw.Draw(r.cur, r.cur.Rect, image.NewUniform(color.NRGBA{R: 70, G: 120, B: 30, A: 170}), image.Point{}, draw.Src)
			want := image.NewRGBA(r.cur.Rect)
			copy(want.Pix, r.cur.Pix)
			g := o.Glyphs[0]
			ix, iy, sub := GlyphPlacement(o.Origin, g, r.scale)
			gm := r.glyph(g, g.Size*r.scale, sub)
			at := image.Pt(ix+gm.off.X, iy+gm.off.Y)
			dst := image.Rectangle{Min: at, Max: at.Add(gm.mask.Rect.Size())}.Intersect(r.clip)
			mask := image.NewAlpha(image.Rect(0, 0, dst.Dx(), dst.Dy()))
			for y := dst.Min.Y; y < dst.Max.Y; y++ {
				for x := dst.Min.X; x < dst.Max.X; x++ {
					a := uint32(gm.mask.AlphaAt(x-at.X, y-at.Y).A)
					b := uint32(r.cmask.AlphaAt(x-r.cmaskAt.X, y-r.cmaskAt.Y).A)
					mask.SetAlpha(x-dst.Min.X, y-dst.Min.Y, color.Alpha{A: uint8((a*b + 127) / 255)})
				}
			}
			draw.DrawMask(want, dst, image.NewUniform(o.Color), image.Point{}, mask, image.Point{}, draw.Over)
			before := append([]byte(nil), gm.mask.Pix...)
			r.text(o)
			if !bytes.Equal(r.cur.Pix, want.Pix) {
				t.Fatalf("partial %v origin %v: clipped glyph pixels changed", partial, origin)
			}
			if !bytes.Equal(gm.mask.Pix, before) {
				t.Fatal("drawing through a clip modified the cached glyph mask")
			}
		}
	}
}
