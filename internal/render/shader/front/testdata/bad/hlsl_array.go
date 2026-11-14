// Uniform struct whose Go layout MSL accepts but HLSL does not: [2]float32
// has a 4-byte stride in Go and MSL, while HLSL cbuffer packing starts the
// array (and every element) on a 16-byte boundary. No vector can straddle
// a 16-byte boundary in a layout MSL accepts (float2 is 8-byte aligned
// there), so the HLSL-only mismatch needs an array or a nested struct.
package bad

import "github.com/tangoui/tango/internal/render/shader/sl"

type In struct {
	P sl.Vec2 `sl:"loc=0"`
}

type Out struct {
	Pos sl.Vec4 `sl:"position"`
}

type Globals struct {
	Scale   float32
	Weights [2]float32
	Pad     float32
}

//sl:uniform 0
var U Globals

//sl:vertex
func vs(in In, vid uint32) Out {
	return Out{Pos: sl.V4(in.P.X, U.Scale, U.Weights[1], U.Pad)}
}

//sl:fragment
func fs(in Out) sl.Vec4 {
	return in.Pos
}
