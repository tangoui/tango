package gpu

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

var bigPathSize = geom.Size{W: 400, H: 300}

// maskPath is a backdrop larger than an atlas page with a rounded hole (a
// tour/spotlight mask): drawn as a patch. A fresh Path each call, as a
// widget rebuilding it every frame makes.
func maskPath(hole float32) *paint.Path {
	p := paint.NewPath()
	p.MoveTo(0, 0).LineTo(380, 0).LineTo(380, 280).LineTo(0, 280).Close()
	p.MoveTo(100+hole, 100).LineTo(100+hole, 160).QuadTo(100+hole, 180, 120+hole, 180).LineTo(200+hole, 180).LineTo(200+hole, 100).Close()
	return p
}

func bigPathList(p *paint.Path, c color.NRGBA, m *geom.Affine) *paint.List {
	l := &paint.List{Background: white}
	l.Fill(geom.R(20, 20, 60, 40), geom.Rad(4), blue)
	if m != nil {
		l.Transform(*m)
	}
	l.Path(p, c)
	if m != nil {
		l.Pop()
	}
	return l
}

// TestBigPathPatchCached: a path too large for an atlas page is rasterized
// on the CPU once and its patch reused while the same content is drawn at
// the same scale, transform, color and region, with the same pixels; any
// change rasterizes again.
func TestBigPathPatchCached(t *testing.T) {
	half := color.NRGBA{0, 0, 0, 128}
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	rasters := 0
	r.onRaster = func(image.Rectangle) { rasters++ }
	render := func(l *paint.List, scale float32) *image.RGBA {
		t.Helper()
		if err := r.Render(l, bigPathSize, scale); err != nil {
			t.Fatal(err)
		}
		return clonePix(dev.img)
	}
	first := render(bigPathList(maskPath(0), half, nil), 1)
	if rasters != 1 {
		t.Fatalf("first frame: %d rasters, want 1 (the big path is a patch)", rasters)
	}
	second := render(bigPathList(maskPath(0), half, nil), 1) // equal content, new Path
	if rasters != 1 {
		t.Fatalf("second frame re-rasterized the unchanged big path (%d rasters)", rasters)
	}
	if !bytes.Equal(first.Pix, second.Pix) {
		t.Fatal("the cached patch draws different pixels")
	}
	ref := paint.NewCPU()
	l := bigPathList(maskPath(0), half, nil)
	ref.Render(l, bigPathSize, 1)
	gputest.Check(t, "cached big path", l, 1, ref.Image(), second, 2)

	check := func(name string, l *paint.List, scale float32, wantRasters int) *image.RGBA {
		t.Helper()
		before := rasters
		got := render(l, scale)
		if rasters-before != wantRasters {
			t.Fatalf("%s: %d new rasters, want %d", name, rasters-before, wantRasters)
		}
		ref := paint.NewCPU()
		ref.Render(l, bigPathSize, scale)
		gputest.Check(t, name, l, scale, ref.Image(), got, 2)
		return got
	}
	dim := color.NRGBA{0, 0, 0, 100}
	check("moved hole", bigPathList(maskPath(8), half, nil), 1, 1)
	check("color", bigPathList(maskPath(8), dim, nil), 1, 1)
	// Back to the first content: still cached (bounded LRU, not one entry).
	if again := check("first content again", bigPathList(maskPath(0), half, nil), 1, 0); !bytes.Equal(first.Pix, again.Pix) {
		t.Fatal("first content again: different pixels")
	}
	check("transform", bigPathList(maskPath(8), dim, &geom.Affine{A: 1, D: 1, E: 3, F: 2}), 1, 1)
	check("transform again", bigPathList(maskPath(8), dim, &geom.Affine{A: 1, D: 1, E: 3, F: 2}), 1, 0)
	// A scale change drops the cache.
	check("scale", bigPathList(maskPath(8), dim, nil), 2, 1)
	check("scale again", bigPathList(maskPath(8), dim, nil), 2, 0)
	check("first content at the old scale", bigPathList(maskPath(0), half, nil), 1, 1)
	before := rasters
	// Trim (low memory) drops it.
	r.Trim()
	render(bigPathList(maskPath(0), half, nil), 1)
	if rasters != before+1 {
		t.Fatalf("after Trim: %d new rasters, want 1", rasters-before)
	}
}

// TestBigPathCacheBounded: the cache keeps at most bigPathMaxEntries
// entries and bigPathMaxBytes of pixels, evicting the least recently used,
// and forgets entries unused for bigPathIdleFrames frames.
func TestBigPathCacheBounded(t *testing.T) {
	var c bigPathCache
	img := func(w, h int) *image.RGBA { return image.NewRGBA(image.Rect(0, 0, w, h)) }
	p := maskPath(0)
	key := func(i int) bigPathKey { return bigPathKey{raster: image.Rect(i, 0, i+10, 10), scale: 1} }
	for i := 0; i < bigPathMaxEntries+3; i++ {
		c.beginFrame()
		c.put(key(i), p.Segs, img(10, 10), image.Rect(0, 0, 10, 10))
	}
	if len(c.m) != bigPathMaxEntries {
		t.Fatalf("%d entries, want %d", len(c.m), bigPathMaxEntries)
	}
	if c.get(key(0), p.Segs) != nil || c.get(key(bigPathMaxEntries+2), p.Segs) == nil {
		t.Fatal("the oldest entry should be evicted, the newest kept")
	}
	// Bytes: one image over the budget is not kept; big ones evict others.
	var b bigPathCache
	b.beginFrame()
	side := 1
	for side*side*4 <= bigPathMaxBytes {
		side *= 2
	}
	b.put(key(0), p.Segs, img(side, side), image.Rect(0, 0, 1, 1))
	if len(b.m) != 0 || b.bytes != 0 {
		t.Fatalf("an image over the byte budget was kept (%d entries, %d bytes)", len(b.m), b.bytes)
	}
	half := side / 2 // a quarter of the over-budget image: ≤ budget/2
	b.put(key(1), p.Segs, img(half, half), image.Rect(0, 0, 1, 1))
	b.put(key(2), p.Segs, img(half, half), image.Rect(0, 0, 1, 1))
	b.put(key(3), p.Segs, img(half, half), image.Rect(0, 0, 1, 1))
	if b.bytes > bigPathMaxBytes || b.get(key(3), p.Segs) == nil {
		t.Fatalf("bytes %d over budget %d, or newest evicted", b.bytes, bigPathMaxBytes)
	}
	// Content is compared exactly, not just by hash.
	q := maskPath(0)
	q.Segs[1].P[0].X += 1e-3
	if b.get(key(3), q.Segs) != nil {
		t.Fatal("different path content matched")
	}
	// Idle entries are dropped.
	for i := 0; i <= bigPathIdleFrames; i++ {
		b.beginFrame()
	}
	if len(b.m) != 0 || b.bytes != 0 {
		t.Fatalf("idle entries kept: %d entries, %d bytes", len(b.m), b.bytes)
	}
}

func clonePix(img *image.RGBA) *image.RGBA {
	c := *img
	c.Pix = append([]byte(nil), img.Pix...)
	return &c
}

// nopDevice executes nothing (benchmarks of the Renderer's CPU side).
type nopDevice struct{}

func (nopDevice) Execute(*Frame) error { return nil }

// BenchmarkBigPathFrame is the CPU side of an animated frame that draws a
// window-sized mask path rebuilt every frame (equal content), at a Retina
// scale: before bigPathCache every frame rasterized it.
func BenchmarkBigPathFrame(b *testing.B) {
	r := NewRenderer(nopDevice{})
	r.SetVerify(false)
	size := geom.Size{W: 1280, H: 800}
	mk := func() *paint.List {
		p := paint.NewPath()
		p.MoveTo(0, 0).LineTo(1280, 0).LineTo(1280, 800).LineTo(0, 800).Close()
		p.MoveTo(400, 300).LineTo(400, 460).QuadTo(400, 480, 420, 480).LineTo(700, 480).LineTo(700, 300).Close()
		return bigPathList(p, color.NRGBA{0, 0, 0, 115}, nil)
	}
	for b.Loop() {
		if err := r.Render(mk(), size, 2); err != nil {
			b.Fatal(err)
		}
	}
}
