package bad

import "github.com/tangoui/tango/internal/render/shader/sl"

type In struct {
	P sl.Vec2 `sl:"loc=0"`
}

type Out struct {
	Pos sl.Vec4 `sl:"position"`
}

//sl:vertex
func vs(in In, vid uint32) Out {
	return Out{Pos: sl.V4(in.P.X, in.P.Y, 0, 1)}
}

func noop() {}

//sl:fragment
func fs(in Out) sl.Vec4 {
	go noop()
	return in.Pos
}
