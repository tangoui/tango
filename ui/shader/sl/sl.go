// Package sl is the shading language that GPU shaders are written in.
//
// Shader functions are ordinary Go code using the types and builtins in this
// package. The shader compiler translates them to Metal; the CPU executor and
// tests run the same code directly through the pure-Go implementations here.
// The exported names in this package are the builtin set the compiler
// recognizes, and the semantics follow GLSL/MSL.
package sl

import "math"

// Vec2 is a two-component float vector.
type Vec2 struct{ X, Y float32 }

// Vec3 is a three-component float vector.
type Vec3 struct{ X, Y, Z float32 }

// Vec4 is a four-component float vector.
type Vec4 struct{ X, Y, Z, W float32 }

// V2 constructs a Vec2.
func V2(x, y float32) Vec2 { return Vec2{x, y} }

// V3 constructs a Vec3.
func V3(x, y, z float32) Vec3 { return Vec3{x, y, z} }

// V4 constructs a Vec4.
func V4(x, y, z, w float32) Vec4 { return Vec4{x, y, z, w} }

// Add returns v+o.
func (v Vec2) Add(o Vec2) Vec2 { return Vec2{v.X + o.X, v.Y + o.Y} }

// Sub returns v-o.
func (v Vec2) Sub(o Vec2) Vec2 { return Vec2{v.X - o.X, v.Y - o.Y} }

// Mul returns the component-wise product v*o.
func (v Vec2) Mul(o Vec2) Vec2 { return Vec2{v.X * o.X, v.Y * o.Y} }

// Div returns the component-wise quotient v/o.
func (v Vec2) Div(o Vec2) Vec2 { return Vec2{v.X / o.X, v.Y / o.Y} }

// Scale returns v*s.
func (v Vec2) Scale(s float32) Vec2 { return Vec2{v.X * s, v.Y * s} }

// Add returns v+o.
func (v Vec3) Add(o Vec3) Vec3 { return Vec3{v.X + o.X, v.Y + o.Y, v.Z + o.Z} }

// Sub returns v-o.
func (v Vec3) Sub(o Vec3) Vec3 { return Vec3{v.X - o.X, v.Y - o.Y, v.Z - o.Z} }

// Mul returns the component-wise product v*o.
func (v Vec3) Mul(o Vec3) Vec3 { return Vec3{v.X * o.X, v.Y * o.Y, v.Z * o.Z} }

// Div returns the component-wise quotient v/o.
func (v Vec3) Div(o Vec3) Vec3 { return Vec3{v.X / o.X, v.Y / o.Y, v.Z / o.Z} }

// Scale returns v*s.
func (v Vec3) Scale(s float32) Vec3 { return Vec3{v.X * s, v.Y * s, v.Z * s} }

// Add returns v+o.
func (v Vec4) Add(o Vec4) Vec4 { return Vec4{v.X + o.X, v.Y + o.Y, v.Z + o.Z, v.W + o.W} }

// Sub returns v-o.
func (v Vec4) Sub(o Vec4) Vec4 { return Vec4{v.X - o.X, v.Y - o.Y, v.Z - o.Z, v.W - o.W} }

// Mul returns the component-wise product v*o.
func (v Vec4) Mul(o Vec4) Vec4 { return Vec4{v.X * o.X, v.Y * o.Y, v.Z * o.Z, v.W * o.W} }

// Div returns the component-wise quotient v/o.
func (v Vec4) Div(o Vec4) Vec4 { return Vec4{v.X / o.X, v.Y / o.Y, v.Z / o.Z, v.W / o.W} }

// Scale returns v*s.
func (v Vec4) Scale(s float32) Vec4 { return Vec4{v.X * s, v.Y * s, v.Z * s, v.W * s} }

// XY returns the swizzle v.xy.
func (v Vec4) XY() Vec2 { return Vec2{v.X, v.Y} }

// ZW returns the swizzle v.zw.
func (v Vec4) ZW() Vec2 { return Vec2{v.Z, v.W} }

// XYZ returns the swizzle v.xyz.
func (v Vec4) XYZ() Vec3 { return Vec3{v.X, v.Y, v.Z} }

// Abs returns |x|.
func Abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// Min returns the smaller of x and y (GLSL min: y < x ? y : x).
func Min(x, y float32) float32 {
	if y < x {
		return y
	}
	return x
}

// Max returns the larger of x and y (GLSL max: x < y ? y : x).
func Max(x, y float32) float32 {
	if x < y {
		return y
	}
	return x
}

// Clamp returns min(max(x, lo), hi).
func Clamp(x, lo, hi float32) float32 { return Min(Max(x, lo), hi) }

// Mix linearly interpolates a*(1-t) + b*t.
func Mix(a, b, t float32) float32 { return a*(1-t) + b*t }

// Step returns 0 if x < edge, else 1.
func Step(edge, x float32) float32 {
	if x < edge {
		return 0
	}
	return 1
}

// Smoothstep returns the Hermite interpolation t*t*(3-2t) with
// t = clamp((x-e0)/(e1-e0), 0, 1).
func Smoothstep(e0, e1, x float32) float32 {
	t := Clamp((x-e0)/(e1-e0), 0, 1)
	return t * t * (3 - 2*t)
}

// Sqrt returns the square root of x.
func Sqrt(x float32) float32 { return float32(math.Sqrt(float64(x))) }

// Floor returns the largest integer value not greater than x.
func Floor(x float32) float32 { return float32(math.Floor(float64(x))) }

// Fract returns x - floor(x), which is in [0, 1) also for negative x. Like
// MSL fract it is capped at the largest float32 below 1, because for tiny
// negative x the subtraction rounds up to exactly 1.
func Fract(x float32) float32 { return Min(x-Floor(x), 0x1.fffffep-1) }

// Pow returns x raised to the power y.
func Pow(x, y float32) float32 { return float32(math.Pow(float64(x), float64(y))) }

// Erf returns the error function of x by Abramowitz–Stegun 7.1.26:
// 1 - t*(a1 + t*(a2 + t*(a3 + t*(a4 + t*a5))))*exp(-x²) with
// t = 1/(1 + p*|x|), negated for x < 0 (absolute error at most 1.5e-7 in
// exact arithmetic). MSL has no erf builtin; the Metal backend emits this
// polynomial as a helper function, so every backend computes the same
// approximation. Erf(±Inf) is ±1.
func Erf(x float32) float32 {
	a := Abs(x)
	t := 1 / (1 + 0.3275911*a)
	y := 1 - t*(0.254829592+t*(-0.284496736+t*(1.421413741+t*(-1.453152027+t*1.061405429))))*float32(math.Exp(float64(-a*a)))
	if x < 0 {
		return -y
	}
	return y
}

// Length returns the Euclidean length of v.
func Length(v Vec2) float32 { return Sqrt(Dot(v, v)) }

// Dot returns the dot product of a and b.
func Dot(a, b Vec2) float32 { return a.X*b.X + a.Y*b.Y }

// AbsV2 returns the component-wise absolute value of v.
func AbsV2(v Vec2) Vec2 { return Vec2{Abs(v.X), Abs(v.Y)} }

// MaxV2 returns the component-wise max(v, s).
func MaxV2(v Vec2, s float32) Vec2 { return Vec2{Max(v.X, s), Max(v.Y, s)} }

// MinV2 returns the component-wise min(v, s).
func MinV2(v Vec2, s float32) Vec2 { return Vec2{Min(v.X, s), Min(v.Y, s)} }

// MixV4 linearly interpolates each component of a and b by t.
func MixV4(a, b Vec4, t float32) Vec4 {
	return Vec4{Mix(a.X, b.X, t), Mix(a.Y, b.Y, t), Mix(a.Z, b.Z, t), Mix(a.W, b.W, t)}
}
