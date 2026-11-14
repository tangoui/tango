// Package basic is a small shader used as the front-end snapshot test.
package basic

import "github.com/tangoui/tango/internal/render/shader/sl"

const aa = 0.5

// Instance is one rounded rectangle.
type Instance struct {
	Rect   sl.Vec4 `sl:"loc=0"`
	Color  sl.Vec4 `sl:"loc=1"`
	Radius float32 `sl:"loc=2"`
	Kind   uint32  `sl:"loc=3"`
}

// Varyings flows from the vertex to the fragment stage.
type Varyings struct {
	Pos    sl.Vec4 `sl:"position"`
	Local  sl.Vec2 `sl:"loc=0"`
	Half   sl.Vec2 `sl:"loc=1"`
	Color  sl.Vec4 `sl:"loc=2"`
	Radius float32 `sl:"loc=3"`
	Kind   uint32  `sl:"loc=4,flat"`
}

// Globals is the per-frame uniform block.
type Globals struct {
	Viewport sl.Vec2
	Scale    float32
	Pad      float32 // to MSL's 16-byte size (float2 alignment)
}

//sl:uniform 0
var U Globals

//sl:texture 0 filter=linear
var Tex sl.Texture2D

func rrectSDF(p, half sl.Vec2, r float32) float32 {
	q := sl.AbsV2(p).Sub(half).Add(sl.V2(r, r))
	return sl.Length(sl.MaxV2(q, 0)) + sl.Min(sl.Max(q.X, q.Y), 0) - r
}

//sl:vertex
func vs(in Instance, vid uint32) Varyings {
	corner := sl.V2(0, 0)
	if vid == 1 || vid == 3 {
		corner.X = 1
	}
	if vid >= 2 {
		corner.Y = 1
	}
	size := in.Rect.ZW().Sub(in.Rect.XY())
	pos := in.Rect.XY().Add(size.Mul(corner))
	clip := pos.Div(U.Viewport).Scale(2).Sub(sl.V2(1, 1))
	var out Varyings
	out.Pos = sl.V4(clip.X, -clip.Y, 0, 1)
	out.Local = pos.Sub(in.Rect.XY()).Sub(size.Scale(0.5))
	out.Half = size.Scale(0.5)
	out.Color = in.Color
	out.Radius = in.Radius
	out.Kind = in.Kind
	return out
}

//sl:fragment
func fs(in Varyings) sl.Vec4 {
	d := rrectSDF(in.Local, in.Half, in.Radius)
	cov := float32(0)
	for i := 0; i < 4; i++ {
		off := (float32(i) - 1.5) * 0.25
		cov += sl.Clamp(aa-(d+off*U.Scale), 0, 1)
	}
	cov = cov / 4
	if in.Kind == 1 {
		t := Tex.Sample(in.Local.Div(in.Half))
		return t.Scale(cov)
	} else if cov <= 0 {
		return sl.V4(0, 0, 0, 0)
	} else {
		cov = sl.Smoothstep(0, 1, cov)
	}
	return in.Color.Scale(cov)
}
