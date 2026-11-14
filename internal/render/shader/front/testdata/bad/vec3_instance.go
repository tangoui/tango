package bad

import "github.com/tangoui/tango/internal/render/shader/sl"

type In struct {
	P     sl.Vec2 `sl:"loc=0"`
	Color sl.Vec3 `sl:"loc=1"`
}

type Out struct {
	Pos sl.Vec4 `sl:"position"`
}

//sl:vertex
func vs(in In, vid uint32) Out {
	return Out{Pos: sl.V4(in.P.X, in.P.Y, in.Color.X, 1)}
}

//sl:fragment
func fs(in Out) sl.Vec4 {
	return in.Pos
}
