//go:build linux || darwin

package gl

import (
	"math"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/gen"
	"github.com/tangoui/tango/internal/render/shader/front"
	"github.com/tangoui/tango/internal/render/shader/glsl"
)

// code drops comment and blank lines: the committed sources carry
// tanshader's header, the generator's output its own.
func code(src string) string {
	var b strings.Builder
	for _, l := range strings.Split(src, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "//") {
			continue
		}
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestESSourceMatchesGenerator: esSource of the committed GLSL 330 quad
// shader is internal/render/shader/glsl's ES300 dialect of the same module.
func TestESSourceMatchesGenerator(t *testing.T) {
	m, err := front.Load("../shaders")
	if err != nil {
		t.Fatal(err)
	}
	vert, frag, err := glsl.Generate(m, glsl.ES300)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, got, want string }{
		{"vertex", esSource(gen.QuadGLSLVertex), vert},
		{"fragment", esSource(gen.QuadGLSLFragment), frag},
	} {
		if !strings.HasPrefix(c.got, "#version 300 es\n") {
			t.Errorf("%s: starts %q", c.name, c.got[:min(40, len(c.got))])
		}
		if code(c.got) != code(c.want) {
			t.Errorf("%s: esSource differs from glsl.Generate(ES300):\n--- esSource\n%s\n--- generator\n%s", c.name, code(c.got), code(c.want))
		}
	}
}

func halfToFloat(h uint16) float64 {
	e := int(h >> 10 & 0x1F)
	m := float64(h & 0x3FF)
	if e == 0 {
		return m * math.Pow(2, -24)
	}
	return (1 + m/1024) * math.Pow(2, float64(e-15))
}

// TestUnormToHalf: every 16-bit unorm value maps to the nearest half
// float, 0 and 1 exactly, and the mapping is monotonic.
func TestUnormToHalf(t *testing.T) {
	if unormToHalf(0) != 0 || unormToHalf(0xFFFF) != 0x3C00 {
		t.Fatalf("ends: %#x %#x", unormToHalf(0), unormToHalf(0xFFFF))
	}
	prev := uint16(0)
	for u := 1; u <= 0xFFFF; u++ {
		h := unormToHalf(uint16(u))
		if h < prev {
			t.Fatalf("not monotonic at %d: %#x after %#x", u, h, prev)
		}
		prev = h
		v := float64(u) / 65535
		got := halfToFloat(h)
		// The neighbours of h must not be closer to v.
		for _, n := range []uint16{h - 1, h + 1} {
			if n > 0x3C00 {
				continue
			}
			if d := math.Abs(halfToFloat(n) - v); d < math.Abs(got-v)-1e-12 {
				t.Fatalf("u %d (%.8f): half %#x = %.8f, but %#x = %.8f is closer", u, v, h, got, n, halfToFloat(n))
			}
		}
		if math.Abs(got-v) > 1.0/4096 {
			t.Fatalf("u %d: error %g", u, math.Abs(got-v))
		}
	}
}

func TestHalfRows(t *testing.T) {
	// Two rows of one texel, stride 12 (4 padding bytes).
	pix := []byte{
		0, 0, 0xFF, 0xFF, 0, 0x80, 1, 0, 9, 9, 9, 9,
		0xFF, 0xFF, 0, 0, 0, 0, 0, 0,
	}
	out := halfRows(nil, pix, 1, 2, 12)
	if len(out) != 16 {
		t.Fatalf("len %d", len(out))
	}
	want := []uint16{0, 0x3C00, unormToHalf(0x8000), unormToHalf(1), 0x3C00, 0, 0, 0}
	for i, w := range want {
		if got := uint16(out[2*i]) | uint16(out[2*i+1])<<8; got != w {
			t.Errorf("half %d = %#x, want %#x", i, got, w)
		}
	}
}
