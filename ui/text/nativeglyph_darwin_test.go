//go:build darwin

package text

import (
	"image"
	"math"
	"slices"
	"testing"
	"unsafe"

	"github.com/tangoui/tango/internal/nativeglyph"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

type nativeOutlineOnly struct{}

func (*nativeOutlineOnly) GlyphPath(uint32, float32) *paint.Path { return nil }

func requireNativeGlyphProvider(t *testing.T) nativeglyph.Provider {
	t.Helper()
	if Engine() != "coretext" {
		t.Skip("CoreText unavailable")
	}
	p := nativeglyph.CoreGraphics
	if p == nil {
		t.Fatal("CoreText did not register its native glyph provider")
	}
	return p
}

func TestNativeGlyphProviderDeclinesUnsupported(t *testing.T) {
	p := requireNativeGlyphProvider(t)
	if p.Available(nil) || p.Available(&nativeOutlineOnly{}) {
		t.Fatal("accepted a non-CoreText glyph source")
	}
	if p.Key(&nativeOutlineOnly{}) != 0 {
		t.Fatal("unsupported source has a native key")
	}
	if _, ok := p.Bounds(nativeglyph.Run{}); ok {
		t.Fatal("accepted an invalid run")
	}
	if p.Draw(0, nativeglyph.Run{}) {
		t.Fatal("accepted a nil drawing context")
	}
}

func TestNativeGlyphBoundsContainRasterInk(t *testing.T) {
	p := requireNativeGlyphProvider(t)
	for _, scale := range []float32{1, 1.5, 2} {
		l := Shape("Hello 123 中文 العربية", Style{Size: 17}, Options{})
		for _, g := range l.Glyphs {
			if !p.Available(g.Src) {
				continue
			}
			for _, skew := range []float32{0, 0.2} {
				g.Skew = skew
				ix, iy, sub := paint.GlyphPlacement(geom.Pt(7.25, 11.5), g, scale)
				run := nativeglyph.Run{Source: g.Src, Size: g.Size * scale, Skew: skew, IDs: []uint16{uint16(g.ID)}, Positions: []geom.Point{geom.Pt(float32(ix)+float32(sub)/4, -float32(iy))}}
				bounds, ok := p.Bounds(run)
				if !ok {
					t.Fatal("valid CoreText bounds rejected")
				}
				raster := g.Src.(paint.GlyphRasterizer)
				mask, off, ok := raster.RasterGlyph(g.ID, g.Size*scale, sub, skew)
				if !ok || mask == nil {
					continue
				}
				for y := 0; y < mask.Rect.Dy(); y++ {
					for x := 0; x < mask.Rect.Dx(); x++ {
						if mask.Pix[y*mask.Stride+x] == 0 {
							continue
						}
						// RasterGlyph is y-down; the bridge uses native CoreGraphics y-up.
						px, py := ix+off.X+x, iy+off.Y+y
						if !image.Pt(px, -py-1).In(bounds) {
							t.Fatalf("scale %v skew %v glyph %d ink (%d,%d) outside native bounds %v", scale, skew, g.ID, px, -py-1, bounds)
						}
					}
				}
				if p.Key(g.Src) == 0 || p.Key(g.Src) != p.Key(g.Src) {
					t.Fatal("unstable native source identity")
				}
			}
		}
	}
}

func TestNativeGlyphDrawVisibleAndBorrowsBuffers(t *testing.T) {
	p := requireNativeGlyphProvider(t)
	l := Shape("Hello 123", Style{Size: 16}, Options{})
	if len(l.Glyphs) == 0 {
		t.Fatal("no shaped glyphs")
	}
	src := l.Glyphs[0].Src
	if !p.Available(src) {
		t.Fatal("system glyph source unavailable")
	}
	run := nativeglyph.Run{Source: src, Size: 16}
	for _, g := range l.Glyphs {
		run.IDs = append(run.IDs, uint16(g.ID))
		run.Positions = append(run.Positions, geom.Pt(10+g.X, 35-g.Y))
	}
	ids, positions := slices.Clone(run.IDs), slices.Clone(run.Positions)
	cs := cgColorSpaceCreateDeviceGray()
	ctx := cgBitmapContextCreate(0, 180, 60, 8, 0, cs, cgImageAlphaNone)
	cgColorSpaceRelease(cs)
	if ctx == 0 {
		t.Fatal("no bitmap context")
	}
	defer cgContextRelease(ctx)
	bpr := int(cgBitmapContextGetBytesPerRow(ctx))
	pixels := unsafe.Slice((*byte)(cgBitmapContextGetData(ctx)), bpr*60)
	for i := range pixels {
		pixels[i] = 255
	}
	cgContextSetGrayFillColor(ctx, 0, 1)
	if !p.Draw(ctx, run) {
		t.Fatal("valid native glyph run rejected")
	}
	ink := 0
	for _, v := range pixels {
		if v < 128 {
			ink++
		}
	}
	if ink < 100 {
		t.Fatalf("native glyphs missing: %d dark pixels", ink)
	}
	if !slices.Equal(ids, run.IDs) || !slices.Equal(positions, run.Positions) {
		t.Fatal("drawing mutated borrowed glyph buffers")
	}
	bad := run
	bad.Positions = bad.Positions[:1]
	if p.Draw(ctx, bad) {
		t.Fatal("accepted mismatched ID/position lengths")
	}
	bad = run
	bad.Size = float32(math.NaN())
	if _, ok := p.Bounds(bad); ok {
		t.Fatal("accepted non-finite size")
	}
}
