package shaders

import (
	"bytes"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/shader/sl"
)

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

func TestRRectCoverage(t *testing.T) {
	origin, size := sl.V2(10, 10), sl.V2(10, 10)
	var square sl.Vec4
	cases := []struct {
		name         string
		p            sl.Vec2
		origin, size sl.Vec2
		radii        sl.Vec4
		want         float32
	}{
		{"left of edge", sl.V2(9.5, 15), origin, size, square, 0},
		{"inside edge", sl.V2(10.5, 15), origin, size, square, 1},
		{"right outside", sl.V2(20.5, 15), origin, size, square, 0},
		{"right inside", sl.V2(19.5, 15), origin, size, square, 1},
		{"fractional edge", sl.V2(10.5, 15), sl.V2(10.25, 10), sl.V2(9.75, 10), square, 0.75},
		{"fractional edge outside", sl.V2(9.5, 15), sl.V2(10.25, 10), sl.V2(9.75, 10), square, 0},
		{"rounded tl corner outside", sl.V2(10.5, 10.5), origin, size, sl.V4(4, 4, 4, 4), 0},
		{"rounded br corner outside", sl.V2(19.5, 19.5), origin, size, sl.V4(4, 4, 4, 4), 0},
		{"square corner inside", sl.V2(10.5, 10.5), origin, size, square, 1},
		{"only tr rounded keeps tl", sl.V2(10.5, 10.5), origin, size, sl.V4(0, 4, 0, 0), 1},
		{"only tr rounded", sl.V2(19.5, 10.5), origin, size, sl.V4(0, 4, 0, 0), 0},
		{"center of rounded", sl.V2(15, 15), origin, size, sl.V4(4, 4, 4, 4), 1},
		{"0.3px-wide rect", sl.V2(10.5, 15), sl.V2(10.35, 10), sl.V2(0.3, 10), square, 0.3},
		{"square corner at fractional offset", sl.V2(10.5, 10.5), sl.V2(10.25, 10.25), sl.V2(9.75, 9.75), square, 0.5625},
		{"rounded corner capped by box", sl.V2(10.5, 10.5), sl.V2(10.25, 10.25), sl.V2(9.75, 9.75), sl.V4(0.5, 0.5, 0.5, 0.5), 0.5625},
		{"empty rect", sl.V2(10, 10), origin, sl.V2(0, 10), square, 0},
	}
	for _, c := range cases {
		if got := RRectCoverage(c.p, c.origin, c.size, c.radii); !near(got, c.want) {
			t.Errorf("%s: RRectCoverage(%v) = %v, want %v", c.name, c.p, got, c.want)
		}
	}
}

func TestVSClipSpace(t *testing.T) {
	FrameU = Frame{ViewSize: sl.V2(100, 50)}
	in := Instance{Rect: sl.V4(10, 20, 30, 10), Color: sl.V4(1, 0, 0, 1)}
	// The quad is the rect grown by one pixel, as a triangle strip:
	// 0 top-left, 1 top-right, 2 bottom-left, 3 bottom-right.
	want := [4]sl.Vec2{sl.V2(9, 19), sl.V2(41, 19), sl.V2(9, 31), sl.V2(41, 31)}
	for vid := uint32(0); vid < 4; vid++ {
		out := VS(in, vid)
		px := sl.V2((out.Pos.X+1)/2*100, (1-out.Pos.Y)/2*50)
		if !near(px.X, want[vid].X) || !near(px.Y, want[vid].Y) || out.Pos.W != 1 {
			t.Errorf("vid %d: position %v -> pixel %v, want %v", vid, out.Pos, px, want[vid])
		}
		if out.P != want[vid] {
			t.Errorf("vid %d: P = %v, want %v", vid, out.P, want[vid])
		}
	}
}

func TestFSKinds(t *testing.T) {
	mask := sl.NewTexture(grayImage(2, 1, 0, 128), sl.Nearest)
	Mask = mask
	defer func() { Mask = sl.Texture2D{} }()
	base := Varyings{
		Rect:  sl.V4(0, 0, 2, 1),
		Color: sl.V4(0.5, 0.25, 0, 0.5),
		UV:    sl.V4(0, 0, 1, 1),
	}
	// Solid.
	v := base
	v.P = sl.V2(0.5, 0.5)
	if got := FS(v); got != base.Color {
		t.Errorf("solid: FS = %v, want %v", got, base.Color)
	}
	// Mask: texel 1 is 128/255.
	v.Kind = sl.V4(1, 0, 0, 0)
	v.P = sl.V2(1.5, 0.5)
	want := base.Color.Scale(128.0 / 255)
	if got := FS(v); !near(got.X, want.X) || !near(got.W, want.W) {
		t.Errorf("mask: FS = %v, want %v", got, want)
	}
	v.P = sl.V2(0.5, 0.5)
	if got := FS(v); got != (sl.Vec4{}) {
		t.Errorf("mask texel 0: FS = %v, want 0", got)
	}
	// A partially covered pixel past the entry's right edge samples the
	// entry's own edge texel, not the neighbouring entry (texels 2..3).
	Mask = sl.NewTexture(grayImage(4, 1, 255, 255, 100, 100), sl.Nearest)
	v = Varyings{
		P:     sl.V2(2.5, 0.5),
		Rect:  sl.V4(0, 0, 2.25, 1),
		Color: sl.V4(1, 1, 1, 1),
		UV:    sl.V4(0, 0, 0.5, 1),
		Kind:  sl.V4(KindMask, 0.5/4, 0.5, 0),
	}
	if got := FS(v); !near(got.W, 0.25) {
		t.Errorf("atlas edge: FS = %v, want alpha 0.25 (edge texel 255 at coverage 0.25)", got)
	}
	// Clip excludes the pixel.
	v = base
	v.P = sl.V2(1.5, 0.5)
	v.Clip = sl.V4(0, 0, 1, 1)
	if got := FS(v); got != (sl.Vec4{}) {
		t.Errorf("clipped: FS = %v, want 0", got)
	}
}

// TestGeneratedUpToDate regenerates the Metal, HLSL and GLSL sources and
// the Go layout file into a temporary directory and compares them with the
// committed ui/gpu/gen/quad.metal, quad.hlsl, quad.vert, quad.frag and
// quad_layout.go.
func TestGeneratedUpToDate(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go run")
	}
	out := t.TempDir()
	cmd := exec.Command("go", "run", "../../shader/cmd/tanshader", "-pkg", ".", "-out", out, "-package", "gen")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tanshader: %v\n%s", err, b)
	}
	for _, name := range []string{"quad.metal", "quad.hlsl", "quad.vert", "quad.frag", "quad_layout.go"} {
		got, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("..", "gen", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("ui/gpu/gen/%s is stale; run go generate ./ui/gpu/gen/", name)
		}
		// GLSL needs #version first, so the header is the second line there.
		head := string(got)
		if ext := filepath.Ext(name); ext == ".vert" || ext == ".frag" {
			if !strings.HasPrefix(head, "#version 330 core\n") {
				t.Errorf("%s: does not start with #version 330 core:\n%.80s", name, got)
			}
			head = head[strings.IndexByte(head, '\n')+1:]
		}
		if !strings.HasPrefix(head, "// Code generated by tanshader. DO NOT EDIT.\n") {
			t.Errorf("%s: missing generated-file header:\n%.80s", name, got)
		}
	}
}

// TestMetalCompiles compiles the committed quad.metal with the Metal
// compiler; it is skipped where the toolchain is not installed.
func TestMetalCompiles(t *testing.T) {
	if err := exec.Command("xcrun", "-f", "metal").Run(); err != nil {
		t.Skip("xcrun metal not available:", err)
	}
	if out, _ := exec.Command("xcrun", "metal", "--version").CombinedOutput(); strings.Contains(string(out), "missing Metal Toolchain") {
		t.Skip("Metal toolchain not installed")
	}
	air := filepath.Join(t.TempDir(), "quad.air")
	out, err := exec.Command("xcrun", "metal", "-c", "../gen/quad.metal", "-o", air).CombinedOutput()
	if err != nil || len(bytes.TrimSpace(out)) > 0 {
		t.Errorf("xcrun metal -c quad.metal: %v\n%s", err, out)
	}
}
