package cpuexec

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
)

func TestCPUExecSolidRect(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
	Run([]shaders.Instance{{
		Rect:  sl.V4(2, 2, 4, 4),
		Color: sl.V4(1, 0, 0, 1),
	}}, 8, 8, nil, nil, nil, nil, dst)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			want := color.RGBA{}
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				want = color.RGBA{255, 0, 0, 255}
			}
			if got := dst.RGBAAt(x, y); got != want {
				t.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestCPUExecBlendAndRound(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 4, 1))
	for i := range dst.Pix {
		dst.Pix[i] = 255 // opaque white
	}
	// Half-transparent premultiplied blue over white; the left edge at
	// x=0.25 gives pixel 0 coverage 0.75.
	Run([]shaders.Instance{{
		Rect:  sl.V4(0.25, 0, 3.75, 1),
		Color: sl.V4(0, 0, 0.5, 0.5),
	}}, 4, 1, nil, nil, nil, nil, dst)
	// Pixel 1: 0.5*255 + 255*0.5 → (127.5, 127.5, 255) rounds to 128.
	if got, want := dst.RGBAAt(1, 0), (color.RGBA{128, 128, 255, 255}); got != want {
		t.Errorf("pixel 1 = %v, want %v", got, want)
	}
	// Pixel 0: src scaled by 0.75 → a=0.375; r = 255*(1-0.375) = 159.375.
	if got, want := dst.RGBAAt(0, 0), (color.RGBA{159, 159, 255, 255}); got != want {
		t.Errorf("pixel 0 = %v, want %v", got, want)
	}
}

func TestCPUExecMaskAndPatch(t *testing.T) {
	mask := image.NewGray(image.Rect(0, 0, 2, 2))
	mask.Pix = []uint8{255, 0, 0, 255}
	patch := image.NewRGBA(image.Rect(0, 0, 2, 1))
	patch.SetRGBA(0, 0, color.RGBA{0, 255, 0, 255})
	patch.SetRGBA(1, 0, color.RGBA{0, 0, 128, 128})
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	Run([]shaders.Instance{
		{Rect: sl.V4(0, 0, 2, 2), Color: sl.V4(1, 0, 0, 1), UV: sl.V4(0, 0, 1, 1), Kind: sl.V4(1, 0, 0, 0)},
		{Rect: sl.V4(0, 3, 4, 1), Color: sl.V4(1, 1, 1, 1), UV: sl.V4(0, 0, 1, 1), Kind: sl.V4(2, 0, 0, 0)},
	}, 4, 4, mask, patch, nil, nil, dst)
	checks := []struct {
		x, y int
		want color.RGBA
	}{
		{0, 0, color.RGBA{255, 0, 0, 255}},
		{1, 0, color.RGBA{}},
		{0, 1, color.RGBA{}},
		{1, 1, color.RGBA{255, 0, 0, 255}},
		{0, 3, color.RGBA{0, 255, 0, 255}}, // patch texel 0 stretched over x 0..2
		{1, 3, color.RGBA{0, 255, 0, 255}},
		{2, 3, color.RGBA{0, 0, 128, 128}},
		{3, 3, color.RGBA{0, 0, 128, 128}},
		{2, 0, color.RGBA{}},
	}
	for _, c := range checks {
		if got := dst.RGBAAt(c.x, c.y); got != c.want {
			t.Errorf("pixel (%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestCPUExecRoundedClip(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
	Run([]shaders.Instance{{
		Rect:      sl.V4(0, 0, 8, 8),
		Color:     sl.V4(1, 1, 1, 1),
		Clip:      sl.V4(1, 1, 6, 6),
		ClipRadii: sl.V4(3, 3, 3, 3),
	}}, 8, 8, nil, nil, nil, nil, dst)
	if got := dst.RGBAAt(0, 4); got.A != 0 {
		t.Errorf("outside clip = %v, want transparent", got)
	}
	if got := dst.RGBAAt(1, 1); got.A != 0 {
		t.Errorf("rounded clip corner = %v, want transparent", got)
	}
	if got := dst.RGBAAt(3, 3); got.A != 255 {
		t.Errorf("inside clip = %v, want opaque", got)
	}
}

// TestCPUExecQuantizesPerInstance checks that dst is rounded to 8 bits after
// every instance, as a GPU writing an 8-bit target (and paint.CPU) does.
func TestCPUExecQuantizesPerInstance(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 1, 1))
	inst := shaders.Instance{Rect: sl.V4(0, 0, 1, 1), Color: sl.V4(0, 0, 0, 0.06)}
	Run([]shaders.Instance{inst, inst}, 1, 1, nil, nil, nil, nil, dst)
	// 0.06 → 15.3 → 15; then 0.06*255 + 15*0.94 = 29.4 → 29. Rounding only
	// at the end would give 0.1164*255 = 29.68 → 30.
	if got := dst.RGBAAt(0, 0).A; got != 29 {
		t.Errorf("alpha = %d, want 29", got)
	}
}

// TestCPUExecNoAtlasBleed draws the left entry of a two-entry atlas with a
// fractional right edge: the partially covered pixel past the entry must
// sample the entry's own edge texel, not the neighbouring entry.
func TestCPUExecNoAtlasBleed(t *testing.T) {
	mask := image.NewGray(image.Rect(0, 0, 4, 1))
	mask.Pix = []uint8{255, 255, 100, 100}
	patch := image.NewRGBA(image.Rect(0, 0, 4, 1))
	for x := 0; x < 4; x++ {
		c := color.RGBA{255, 0, 0, 255}
		if x >= 2 {
			c = color.RGBA{0, 0, 255, 255}
		}
		patch.SetRGBA(x, 0, c)
	}
	// Half a texel of a 4×1 texture is (0.5/4, 0.5/1).
	half := func(kind float32) sl.Vec4 { return sl.V4(kind, 0.5/4, 0.5, 0) }
	for _, tc := range []struct {
		name string
		kind float32
		want color.RGBA // pixel 2, coverage 0.25 of the entry's edge texel
	}{
		{"mask", shaders.KindMask, color.RGBA{64, 64, 64, 64}},
		{"image", shaders.KindPatch, color.RGBA{64, 0, 0, 64}},
	} {
		dst := image.NewRGBA(image.Rect(0, 0, 4, 1))
		Run([]shaders.Instance{{
			Rect:  sl.V4(0, 0, 2.25, 1),
			Color: sl.V4(1, 1, 1, 1),
			UV:    sl.V4(0, 0, 0.5, 1),
			Kind:  half(tc.kind),
		}}, 4, 1, mask, patch, nil, nil, dst)
		if got := dst.RGBAAt(2, 0); got != tc.want {
			t.Errorf("%s: pixel 2 = %v, want %v (no bleed from the right entry)", tc.name, got, tc.want)
		}
		for x := 0; x < 2; x++ {
			full := color.RGBA{255, 255, 255, 255}
			if tc.kind == shaders.KindPatch {
				full = color.RGBA{255, 0, 0, 255}
			}
			if got := dst.RGBAAt(x, 0); got != full {
				t.Errorf("%s: pixel %d = %v, want %v", tc.name, x, got, full)
			}
		}
	}
}

// TestCPUExecNonFiniteGeometry checks that infinite and NaN geometry does not
// panic or index outside dst.
func TestCPUExecNonFiniteGeometry(t *testing.T) {
	inf := float32(math.Inf(1))
	nan := float32(math.NaN())
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	Run([]shaders.Instance{
		{Rect: sl.V4(0, 0, inf, 2), Color: sl.V4(1, 0, 0, 1)},
		{Rect: sl.V4(-inf, 0, inf, 2), Color: sl.V4(1, 0, 0, 1)},
		{Rect: sl.V4(nan, 0, 2, 2), Color: sl.V4(1, 0, 0, 1)},
		{Rect: sl.V4(1e30, -1e30, 1e30, 1e30), Color: sl.V4(1, 0, 0, 1)},
	}, 4, 4, nil, nil, nil, nil, dst)
}

// TestCPUExecWatertight: the two triangles of a quad must cover every pixel
// center exactly once, including centers on (or within rounding of) the
// shared diagonal. A half-transparent quad shows a missed pixel as the
// background and a doubly drawn one as darker.
func TestCPUExecWatertight(t *testing.T) {
	for _, sz := range []image.Point{{192, 112}, {144, 84}, {96, 56}, {301, 77}} {
		for i := 0; i < 40; i++ {
			fi := float32(i)
			x, y := 1+fi*0.37, 2+fi*0.53
			w, h := float32(sz.X)-x-3-fi*0.11, float32(sz.Y)-y-2-fi*0.07
			dst := image.NewRGBA(image.Rect(0, 0, sz.X, sz.Y))
			Run([]shaders.Instance{{Rect: sl.V4(x, y, w, h), Color: sl.V4(0.5, 0, 0, 0.5)}}, sz.X, sz.Y, nil, nil, nil, nil, dst)
			for py := int(math.Ceil(float64(y))); py+1 <= int(y+h); py++ {
				for px := int(math.Ceil(float64(x))); px+1 <= int(x+w); px++ {
					// ±1 for the shader's float coverage; missed is 0, double ~191.
					if got := dst.RGBAAt(px, py); got.R < 127 || got.R > 128 || got.A < 127 || got.A > 128 {
						t.Fatalf("size %v rect %v,%v %vx%v: pixel (%d,%d) = %v", sz, x, y, w, h, px, py, got)
					}
				}
			}
		}
	}
}

// TestCPUExecLayerTarget: Draw into a target whose top-left is device
// pixel Origin keeps instance geometry in main-target coordinates, and a
// KindPatch instance with Kind.W = 1 samples the Layer texture (not Color).
func TestCPUExecLayerTarget(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
	Draw([]shaders.Instance{{Rect: sl.V4(12, 7, 4, 4), Color: sl.V4(1, 0, 0, 1)}},
		Target{Size: image.Pt(8, 8), Origin: image.Pt(10, 5)}, Textures{}, dst)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			want := color.RGBA{}
			if x >= 2 && x < 6 && y >= 2 && y < 6 {
				want = color.RGBA{255, 0, 0, 255}
			}
			if got := dst.RGBAAt(x, y); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}

	layer := image.NewRGBA(image.Rect(0, 0, 2, 2))
	draw := func(c color.RGBA) {
		for i := 0; i < len(layer.Pix); i += 4 {
			layer.Pix[i], layer.Pix[i+1], layer.Pix[i+2], layer.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	draw(color.RGBA{0, 200, 0, 200})
	page := image.NewRGBA(image.Rect(0, 0, 2, 2)) // transparent patch page
	out := image.NewRGBA(image.Rect(0, 0, 4, 4))
	Run([]shaders.Instance{{Rect: sl.V4(0, 0, 2, 2), Color: sl.V4(0.5, 0.5, 0.5, 0.5), UV: sl.V4(0, 0, 1, 1),
		Kind: sl.V4(shaders.KindPatch, 0.25, 0.25, 1)}}, 4, 4, nil, page, nil, nil, out)
	if got := out.RGBAAt(1, 1); got != (color.RGBA{}) {
		t.Fatalf("composite without a layer bound drew %v", got)
	}
	Draw([]shaders.Instance{{Rect: sl.V4(0, 0, 2, 2), Color: sl.V4(0.5, 0.5, 0.5, 0.5), UV: sl.V4(0, 0, 1, 1),
		Kind: sl.V4(shaders.KindPatch, 0.25, 0.25, 1)}}, Target{Size: image.Pt(4, 4)}, Textures{Color: page, Layer: layer}, out)
	if got, want := out.RGBAAt(1, 1), (color.RGBA{0, 100, 0, 100}); got != want {
		t.Fatalf("composite pixel = %v, want %v", got, want)
	}
}
