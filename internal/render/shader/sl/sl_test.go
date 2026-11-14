package sl

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-5 }

func TestScalarBuiltins(t *testing.T) {
	cases := []struct {
		name      string
		got, want float32
	}{
		{"Smoothstep(0,1,0.5)", Smoothstep(0, 1, 0.5), 0.5},
		{"Smoothstep(0,1,-1)", Smoothstep(0, 1, -1), 0},
		{"Smoothstep(0,1,2)", Smoothstep(0, 1, 2), 1},
		{"Clamp(2,0,1)", Clamp(2, 0, 1), 1},
		{"Clamp(-2,0,1)", Clamp(-2, 0, 1), 0},
		{"Mix(0,10,0.25)", Mix(0, 10, 0.25), 2.5},
		{"Fract(-0.25)", Fract(-0.25), 0.75},
		{"Fract(1.75)", Fract(1.75), 0.75},
		{"Step(0.5,0.4)", Step(0.5, 0.4), 0},
		{"Step(0.5,0.5)", Step(0.5, 0.5), 1},
		{"Abs(-3)", Abs(-3), 3},
		{"Min(1,2)", Min(1, 2), 1},
		{"Max(1,2)", Max(1, 2), 2},
		{"Sqrt(9)", Sqrt(9), 3},
		{"Floor(-0.5)", Floor(-0.5), -1},
		{"Pow(2,3)", Pow(2, 3), 8},
	}
	for _, c := range cases {
		if !near(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestVecOps(t *testing.T) {
	if got := Length(V2(3, 4)); !near(got, 5) {
		t.Errorf("Length(V2(3,4)) = %v, want 5", got)
	}
	if got := V4(1, 2, 3, 4).ZW(); got != V2(3, 4) {
		t.Errorf("ZW = %v, want {3 4}", got)
	}
	if got := V4(1, 2, 3, 4).XY(); got != V2(1, 2) {
		t.Errorf("XY = %v", got)
	}
	if got := V4(1, 2, 3, 4).XYZ(); got != V3(1, 2, 3) {
		t.Errorf("XYZ = %v", got)
	}
	if got := V2(1, 2).Add(V2(3, 4)); got != V2(4, 6) {
		t.Errorf("Add = %v", got)
	}
	if got := V3(5, 5, 5).Sub(V3(1, 2, 3)); got != V3(4, 3, 2) {
		t.Errorf("Sub = %v", got)
	}
	if got := V4(1, 2, 3, 4).Mul(V4(2, 2, 2, 2)); got != V4(2, 4, 6, 8) {
		t.Errorf("Mul = %v", got)
	}
	if got := V4(2, 4, 6, 8).Div(V4(2, 2, 2, 2)); got != V4(1, 2, 3, 4) {
		t.Errorf("Div = %v", got)
	}
	if got := V3(1, 2, 3).Scale(2); got != V3(2, 4, 6) {
		t.Errorf("Scale = %v", got)
	}
	if got := Dot(V2(1, 2), V2(3, 4)); !near(got, 11) {
		t.Errorf("Dot = %v", got)
	}
	if got := AbsV2(V2(-1, 2)); got != V2(1, 2) {
		t.Errorf("AbsV2 = %v", got)
	}
	if got := MaxV2(V2(-1, 2), 0); got != V2(0, 2) {
		t.Errorf("MaxV2 = %v", got)
	}
	if got := MinV2(V2(-1, 2), 0); got != V2(-1, 0) {
		t.Errorf("MinV2 = %v", got)
	}
	if got := MixV4(V4(0, 0, 0, 0), V4(4, 8, 12, 16), 0.25); got != V4(1, 2, 3, 4) {
		t.Errorf("MixV4 = %v", got)
	}
}

func TestTextureSample(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 2, 1))
	img.Pix[0], img.Pix[1] = 0, 255

	nt := NewTexture(img, Nearest)
	if got := nt.Sample(V2(0.25, 0.5)); got != V4(0, 0, 0, 1) {
		t.Errorf("Nearest(0.25) = %v, want {0 0 0 1}", got)
	}
	if got := nt.Sample(V2(0.75, 0.5)); got != V4(1, 0, 0, 1) {
		t.Errorf("Nearest(0.75) = %v, want {1 0 0 1}", got)
	}

	lt := NewTexture(img, Linear)
	if got := lt.Sample(V2(0.5, 0.5)); !near(got.X, 0.5) || got.Y != 0 || got.Z != 0 || got.W != 1 {
		t.Errorf("Linear(0.5) = %v, want {0.5 0 0 1}", got)
	}
	// Clamp-to-edge: outside [0,1] reads the border texel.
	if got := lt.Sample(V2(-1, 0.5)); !near(got.X, 0) {
		t.Errorf("Linear(-1) = %v, want X=0", got)
	}
	if got := lt.Sample(V2(2, 0.5)); !near(got.X, 1) {
		t.Errorf("Linear(2) = %v, want X=1", got)
	}
}

func TestTextureSampleRGBA(t *testing.T) {
	// Non-premultiplied half-transparent red becomes premultiplied (0.5,0,0,0.5).
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 128})
	got := NewTexture(img, Nearest).Sample(V2(0.5, 0.5))
	want := V4(128.0/255, 0, 0, 128.0/255)
	if math.Abs(float64(got.X-want.X)) > 1.0/255 || got.Y != 0 || got.Z != 0 || !near(got.W, want.W) {
		t.Errorf("RGBA sample = %v, want ≈%v", got, want)
	}

	// Already-premultiplied RGBA passes through.
	rgba := image.NewRGBA(image.Rect(0, 0, 1, 1))
	rgba.SetRGBA(0, 0, color.RGBA{51, 102, 0, 204})
	got = NewTexture(rgba, Nearest).Sample(V2(0.5, 0.5))
	if !near(got.X, 0.2) || !near(got.Y, 0.4) || got.Z != 0 || !near(got.W, 0.8) {
		t.Errorf("premul RGBA sample = %v", got)
	}
}

func TestTextureSampleDegenerate(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 2, 1))
	img.Pix[0], img.Pix[1] = 0, 255
	inf := float32(math.Inf(1))
	nan := float32(math.NaN())
	for _, f := range []Filter{Nearest, Linear} {
		tex := NewTexture(img, f)
		if got := tex.Sample(V2(inf, 0.5)); !near(got.X, 1) {
			t.Errorf("filter %d: Sample(+Inf) = %v, want X=1", f, got)
		}
		if got := tex.Sample(V2(-inf, 0.5)); !near(got.X, 0) {
			t.Errorf("filter %d: Sample(-Inf) = %v, want X=0", f, got)
		}
		_ = tex.Sample(V2(nan, nan)) // must not panic
	}
	if got := (Texture2D{}).Sample(V2(0.5, 0.5)); got != (Vec4{}) {
		t.Errorf("zero Texture2D sample = %v, want zero", got)
	}
}

// TestFractTinyNegative checks Fract stays below 1 when x - floor(x) rounds
// up to 1 in float32 (MSL fract clamps to the largest float below 1).
func TestFractTinyNegative(t *testing.T) {
	if got := Fract(-1e-9); !(got < 1) {
		t.Errorf("Fract(-1e-9) = %v, want < 1", got)
	}
	if got := Fract(-1e-9); got != 0x1.fffffep-1 {
		t.Errorf("Fract(-1e-9) = %v, want 0x1.fffffep-1", got)
	}
}

// TestErf: Erf is Abramowitz–Stegun 7.1.26 (absolute error at most
// 1.5e-7 in exact arithmetic; float32 rounding adds a few ulps, more where
// the compiler does not fuse multiply-adds: about 4.1e-7 on amd64 against
// under 4e-7 on arm64), odd, and saturates to ±1 without NaN for huge or
// infinite arguments.
func TestErf(t *testing.T) {
	maxErr := 0.0
	for i := -6000; i <= 6000; i++ {
		x := float32(i) / 1000
		got := Erf(x)
		if e := math.Abs(float64(got) - math.Erf(float64(x))); e > maxErr {
			maxErr = e
		}
		if Erf(-x) != -got {
			t.Fatalf("Erf(%v) = %v but Erf(%v) = %v: not odd", x, got, -x, Erf(-x))
		}
	}
	if maxErr > 5e-7 {
		t.Errorf("max |Erf - math.Erf| = %g, want <= 5e-7", maxErr)
	}
	inf := float32(math.Inf(1))
	for _, c := range []struct{ x, want float32 }{{0, 0}, {inf, 1}, {-inf, -1}, {1e30, 1}, {-1e30, -1}} {
		if got := Erf(c.x); got != c.want {
			t.Errorf("Erf(%v) = %v, want %v", c.x, got, c.want)
		}
	}
}
