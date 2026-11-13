// Uniform struct {A sl.Vec2; B float32; C sl.Vec2}: Go puts C at byte 12,
// where HLSL's cbuffer packing would let it straddle the 16-byte boundary,
// so HLSL moves it to 16. MSL (float2 is 8-byte aligned) also puts it at
// 16, so both backends report the field.
package bad

import "github.com/tangoui/tango/ui/shader/sl"

type In struct {
	P sl.Vec2 `sl:"loc=0"`
}

type Out struct {
	Pos sl.Vec4 `sl:"position"`
}

type Globals struct {
	A sl.Vec2
	B float32
	C sl.Vec2
}

//sl:uniform 0
var U Globals

//sl:vertex
func vs(in In, vid uint32) Out {
	return Out{Pos: sl.V4(in.P.X, U.A.Y, U.B, U.C.X)}
}

//sl:fragment
func fs(in Out) sl.Vec4 {
	return in.Pos
}
