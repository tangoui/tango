package geom

import (
	"math"
	"testing"
)

func near(a, b Point) bool {
	return math.Abs(float64(a.X-b.X)) < 1e-4 && math.Abs(float64(a.Y-b.Y)) < 1e-4
}

func TestAffineBasics(t *testing.T) {
	if !Identity().IsIdentity() || (Affine{}).IsIdentity() {
		t.Fatal("identity")
	}
	if p := Rotate(math.Pi / 2).Apply(Pt(1, 0)); p != Pt(0, 1) {
		t.Fatalf("rotate 90 clockwise: %v", p)
	}
	m := Scale(2, 3).Then(Translate(10, 20))
	if p := m.Apply(Pt(1, 1)); p != Pt(12, 23) {
		t.Fatalf("then order: %v", p)
	}
	if p := Translate(10, 20).Mul(Scale(2, 3)).Apply(Pt(1, 1)); p != Pt(12, 23) {
		t.Fatalf("mul order: %v", p)
	}
	c := Pt(50, 40)
	if p := RotateAround(math.Pi/3, c).Apply(c); !near(p, c) {
		t.Fatalf("rotate around keeps centre: %v", p)
	}
	if p := ScaleAround(0.5, 0.5, c).Apply(Pt(60, 40)); !near(p, Pt(55, 40)) {
		t.Fatalf("scale around: %v", p)
	}
	r := RotateAround(0.7, c).Then(Scale(2, 1.5)).Then(Translate(3, -4))
	inv, ok := r.Invert()
	if !ok {
		t.Fatal("invertible")
	}
	if p := inv.Apply(r.Apply(Pt(7, 9))); !near(p, Pt(7, 9)) {
		t.Fatalf("inverse round trip %v", p)
	}
	if _, ok := Scale(0, 1).Invert(); ok {
		t.Fatal("singular should not invert")
	}
	if s := Rotate(1).Then(Scale(3, 2)).MaxScale(); math.Abs(float64(s-3)) > 1e-4 {
		t.Fatalf("max scale %v", s)
	}
	if b := Rotate(math.Pi / 2).Bounds(R(0, 0, 10, 4)); b != R(-4, 0, 4, 10) {
		t.Fatalf("bounds %v", b)
	}
	if !Scale(2, 3).AxisAligned() || Rotate(0.1).AxisAligned() || Scale(-1, 1).AxisAligned() {
		t.Fatal("axis aligned")
	}
}
