package bad

import "github.com/tangoui/tango/internal/render/shader/sl"

type In struct {
	P sl.Vec2 `sl:"loc=0"`
}

type Out struct {
	Pos sl.Vec4 `sl:"position"`
}

//sl:fragment
func fs(in Out) sl.Vec4 {
	return in.Pos
}
