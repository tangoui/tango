// Package names is a shader whose identifiers collide with Metal/C++
// keywords, Metal library functions and the backend's own generated names.
// The MSL backend must rename them so the output still compiles.
package names

import "github.com/tangoui/tango/internal/render/shader/sl"

type vertex struct {
	half  sl.Vec2 `sl:"loc=0"`
	float float32 `sl:"loc=1"`
}

type fragment struct {
	sample sl.Vec4 `sl:"position"`
	kernel sl.Vec2 `sl:"loc=0"`
	device uint32  `sl:"loc=1,flat"`
}

type thread struct {
	min float32
	pad float32 // max is 8-byte aligned in MSL
	max sl.Vec2
	// SV_ names are legal HLSL identifiers (semantics only follow ":").
	SV_Pos sl.Vec4
}

//sl:uniform 0
var constant thread

//sl:texture 0 filter=nearest
var texture sl.Texture2D

func vs_main(x float32) float32 { return sl.Abs(x) }

func length(half sl.Vec2, sampler float32) float32 {
	return sl.Length(half) * vs_main(sampler) * constant.min
}

// sl_erf takes the name of the backend's generated erf helper.
func sl_erf(x float32) float32 { return sl.Erf(x) * 2 }

func sv_blend(x float32) float32 { return x * constant.SV_Pos.X }

func main(uv sl.Vec2) sl.Vec4 {
	return texture.Sample(uv)
}

//sl:vertex
func vs(in vertex, vid uint32) fragment {
	var out fragment
	using := in.half.Scale(in.float)
	out.sample = sl.V4(using.X, using.Y, 0, 1)
	out.kernel = in.half
	out.device = vid
	return out
}

//sl:fragment
func fs(in fragment) sl.Vec4 {
	namespace := length(in.kernel, 2)
	if in.device == 1 {
		constant := namespace + 1
		texture := main(in.kernel)
		return texture.Scale(constant)
	}
	template := sl.Clamp(namespace+sl_erf(namespace), 0, 1)
	sv_count := sv_blend(template)
	class := sl.Max(template, constant.max.X) + sv_count
	return sl.V4(class, class, class, 1)
}
