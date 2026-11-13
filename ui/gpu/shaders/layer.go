package shaders

import "github.com/tangoui/tango/ui/shader/sl"

// LayerComposite returns the premultiplied color (0..1) an opacity
// group's layer texel t (premultiplied, 8-bit values k/255) contributes
// when the layer is composited at group alpha a (ia/255) through the clip
// coverage cov: paint.CPU's compositeLayer arithmetic, operation by
// operation in exact integers held in floats. The coverage becomes an
// 8-bit mask (round(cov*255), as paint.CPU's rounded-clip mask stores it),
// the effective alpha m = ia*mask/255 truncated, and every channel
// k*m/255 truncated. The fixed-function blend then adds dst*(1-alpha);
// paint.CPU truncates that product where the GPU rounds, so the two
// differ by at most 1 level. TestPaintLayerCompositeMatchesShader keeps
// this equal to paint.CPU.
func LayerComposite(t sl.Vec4, a, cov float32) sl.Vec4 {
	ia := sl.Floor(sl.Clamp(a, 0, 1)*255 + 0.5)
	mask := sl.Floor(sl.Clamp(cov, 0, 1)*255 + 0.5)
	m := sl.Floor(ia * mask / 255)
	return sl.V4(scale8(t.X, m), scale8(t.Y, m), scale8(t.Z, m), scale8(t.W, m))
}

// scale8 is the 8-bit channel c (k/255) times m/255, truncated, as k/255.
func scale8(c, m float32) float32 {
	return sl.Floor(sl.Floor(c*255+0.5)*m/255) / 255
}
