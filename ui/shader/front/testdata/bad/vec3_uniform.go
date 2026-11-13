package bad

import "github.com/tangoui/tango/ui/shader/sl"

type In struct {
	P sl.Vec2 `sl:"loc=0"`
}

type Out struct {
	Pos sl.Vec4 `sl:"position"`
}

type Globals struct {
	Tint  sl.Vec3
	Alpha float32
}

//sl:uniform 0
var U Globals

//sl:vertex
func vs(in In, vid uint32) Out {
	return Out{Pos: sl.V4(in.P.X, in.P.Y, 0, U.Alpha)}
}

//sl:fragment
func fs(in Out) sl.Vec4 {
	return in.Pos
}
