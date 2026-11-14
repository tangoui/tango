// Package scoping exercises struct ordering and shadowed local names.
package scoping

import "github.com/tangoui/tango/internal/render/shader/sl"

type In struct {
	P sl.Vec2 `sl:"loc=0"`
}

type Out struct {
	Pos sl.Vec4 `sl:"position"`
}

// Globals uses Light, which is declared after it.
type Globals struct {
	L     Light
	Scale float32
	// To MSL's 32-byte size. Separate fields, not [3]float32: HLSL
	// cbuffers space array elements 16 bytes apart.
	Pad0, Pad1, Pad2 float32
}

type Light struct {
	Color sl.Vec4
}

//sl:uniform 0
var U Globals

//sl:vertex
func vs(in In, vid uint32) Out {
	return Out{Pos: sl.V4(in.P.X, in.P.Y, 0, 1)}
}

//sl:fragment
func fs(in Out) sl.Vec4 {
	x := U.Scale
	if x > 0 {
		x := x + 1
		x_1 := x * 2
		x = x_1
	}
	for i := 0; i < 2; i++ {
		x := x + 2
		in := x
		x = in
	}
	return U.L.Color.Scale(x)
}
