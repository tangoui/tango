// Package exprs exercises statement and expression forms (arrays, loops
// with break/continue, logical operators, conversions, every builtin) that
// the basic shader does not use.
package exprs

import "github.com/tangoui/tango/internal/render/shader/sl"

type In struct {
	P sl.Vec2 `sl:"loc=0"`
	N int32   `sl:"loc=1"`
}

type Out struct {
	Pos sl.Vec4 `sl:"position"`
	UV  sl.Vec2 `sl:"loc=0"`
	N   uint32  `sl:"loc=1,flat"`
}

// Pair is 16 bytes so that it lays out the same in Go, MSL and HLSL (an
// HLSL cbuffer starts the member after a struct on a 16-byte boundary).
type Pair struct {
	A, B, C, D float32
}

// Params uses an array of sl.Vec4, not of float32: HLSL cbuffers space
// array elements 16 bytes apart.
type Params struct {
	Weights [4]sl.Vec4
	Tint    sl.Vec4
	P       Pair
}

//sl:uniform 2
var P Params

//sl:texture 1 filter=nearest
var Mask sl.Texture2D

//sl:texture 3 filter=linear
var Img sl.Texture2D

func tint(c sl.Vec4) sl.Vec4 {
	return sl.MixV4(c, P.Tint, 0.25)
}

func both(uv sl.Vec2) sl.Vec4 {
	return tint(Img.Sample(uv)).Scale(Mask.Sample(uv).X)
}

func pair(a float32) Pair {
	return Pair{A: a, B: sl.Abs(a)}
}

//sl:vertex
func vs(in In, vid uint32) Out {
	var out Out
	out.Pos = sl.V4(in.P.X, in.P.Y, 0, 1)
	out.UV = sl.MinV2(sl.MaxV2(in.P, 0), 1)
	out.N = uint32(in.N) + vid
	return out
}

//sl:fragment
func fs(in Out) sl.Vec4 {
	var w [4]float32
	sum := float32(0)
	for i := 0; i < 4; i++ {
		if i == 3 && !(sum > 1) {
			break
		}
		if i == int(in.N) || sum < 0 {
			continue
		}
		w[i] = P.Weights[i].X * sl.Step(0.5, in.UV.X)
		sum += w[i]
	}
	p := pair(sum)
	x := sl.Pow(sl.Sqrt(sl.Abs(p.A)), 2) + sl.Floor(p.B) + sl.Fract(p.B)
	y := sl.Dot(in.UV, in.UV) + sl.Mix(x, 1, 0.5) + P.P.A + sl.Erf(x)
	c := both(in.UV)
	return sl.V4(x, y, c.Z, c.W).Add(c)
}
