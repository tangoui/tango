package gpu

import (
	"bytes"
	"image"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/paint"
)

// TestTrim: after Trim the atlas is back to one page and the image
// textures are freed, and the next frame draws exactly what the frame
// before Trim drew (re-uploading the atlas and the image).
func TestTrim(t *testing.T) {
	var calls int
	src := countingGlyphs{&calls} // glyph masks stay in paint.CPU's cache: only the atlas is rebuilt
	img := testImage()
	l := &paint.List{Background: white}
	l.DrawImage(img, geom.R(4, 4, 40, 28), geom.Rad(4))
	var gs []paint.Glyph
	for i, c := range "trim me" {
		gs = append(gs, paint.Glyph{Src: src, ID: uint32(c), Size: 14, X: float32(i) * 9})
	}
	l.Add(paint.Text{Origin: geom.Pt(50, 20), Glyphs: gs, Color: ink})
	l.Fill(geom.R(4, 36, 80, 10), geom.Rad(3), blue)
	size := geom.Size{W: 128, H: 56}
	for _, scale := range gputest.Scales {
		dev := &cpuDevice{noRetain: true}
		r := NewRenderer(dev)
		if err := r.Render(l, size, scale); err != nil {
			t.Fatal(err)
		}
		before := trimClone(dev.img)
		idsBefore := len(r.images.byKey)
		if idsBefore != 1 {
			t.Fatalf("scale %v: %d image textures before Trim, want 1", scale, idsBefore)
		}

		r.Trim()
		if n := len(r.images.byKey); n != 0 {
			t.Fatalf("scale %v: %d image textures after Trim", scale, n)
		}
		if px := r.atlas.Pixels().Rect.Size(); px.X != atlasPageSize || px.Y != atlasPageSize {
			t.Fatalf("scale %v: atlas %v after Trim, want one page", scale, px)
		}
		if err := r.Render(l, size, scale); err != nil {
			t.Fatal(err)
		}
		whole := false
		for _, u := range dev.last.Uploads {
			if u.Tex == TexMask && u.Rect == r.atlas.Pixels().Rect {
				whole = true
			}
		}
		if !whole {
			t.Fatalf("scale %v: mask atlas not re-uploaded whole after Trim", scale)
		}
		if len(dev.last.ImageFrees) != 1 || len(dev.last.ImageUploads) != 1 {
			t.Fatalf("scale %v: frees %v uploads %d, want the old texture freed and the image re-uploaded",
				scale, dev.last.ImageFrees, len(dev.last.ImageUploads))
		}
		if len(dev.images) != 1 {
			t.Fatalf("scale %v: device holds %d image textures", scale, len(dev.images))
		}
		if diff := trimDiff(before, dev.img); diff != 0 {
			t.Fatalf("scale %v: %d pixels differ after Trim", scale, diff)
		}
		// Trim twice in a row, and on an idle renderer, is harmless.
		r.Trim()
		r.Trim()
		if err := r.Render(l, size, scale); err != nil {
			t.Fatal(err)
		}
		if diff := trimDiff(before, dev.img); diff != 0 {
			t.Fatalf("scale %v: %d pixels differ after a second Trim", scale, diff)
		}
	}
	NewRenderer(&cpuDevice{}).Trim()
}

func trimClone(m *image.RGBA) *image.RGBA {
	c := image.NewRGBA(m.Rect)
	copy(c.Pix, m.Pix)
	return c
}

// trimDiff counts differing pixels (any channel).
func trimDiff(a, b *image.RGBA) int {
	if a.Rect != b.Rect {
		return a.Rect.Dx() * a.Rect.Dy()
	}
	n := 0
	for i := 0; i+4 <= len(a.Pix); i += 4 {
		if !bytes.Equal(a.Pix[i:i+4], b.Pix[i:i+4]) {
			n++
		}
	}
	return n
}
