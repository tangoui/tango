package shaders

import "github.com/tangoui/tango/internal/render/shader/sl"

// CatmullRomWeights returns the Catmull-Rom (B = 0, C = 0.5) weights of
// the four texels around a sample point whose fraction past the second
// texel's center is f: paint.CatmullRomWeights, operation by operation.
func CatmullRomWeights(f float32) sl.Vec4 {
	f2 := f * f
	f3 := f2 * f
	return sl.V4(
		-0.5*f3+f2-0.5*f,
		1.5*f3-2.5*f2+1,
		-1.5*f3+2*f2+0.5*f,
		0.5*f3-0.5*f2)
}

// SampleCatmullRom samples the Img texture (size texels, premultiplied)
// at the normalized position uv with the Catmull-Rom filter over the 4×4
// texels around it, edges clamped, and clamps the result to valid
// premultiplied color (alpha to 0..1, color to 0..alpha). It is
// paint.SampleCatmullRomF, step by step; the 16 texels are read with
// nearest filtering at their centers.
func SampleCatmullRom(uv, size sl.Vec2) sl.Vec4 {
	x := uv.X*size.X - 0.5
	y := uv.Y*size.Y - 0.5
	x0 := sl.Floor(x)
	y0 := sl.Floor(y)
	wx := CatmullRomWeights(x - x0)
	wy := CatmullRomWeights(y - y0)
	c := catmullRomRow(x0-1, y0-1, wx, size).Scale(wy.X)
	c = c.Add(catmullRomRow(x0-1, y0, wx, size).Scale(wy.Y))
	c = c.Add(catmullRomRow(x0-1, y0+1, wx, size).Scale(wy.Z))
	c = c.Add(catmullRomRow(x0-1, y0+2, wx, size).Scale(wy.W))
	a := sl.Clamp(c.W, 0, 1)
	return sl.V4(sl.Clamp(c.X, 0, a), sl.Clamp(c.Y, 0, a), sl.Clamp(c.Z, 0, a), a)
}

// catmullRomRow filters texel row y horizontally: texels x0 … x0+3
// weighted by w (paint's catmullRomRow).
func catmullRomRow(x0, y float32, w sl.Vec4, size sl.Vec2) sl.Vec4 {
	c := imgTexel(x0, y, size).Scale(w.X)
	c = c.Add(imgTexel(x0+1, y, size).Scale(w.Y))
	c = c.Add(imgTexel(x0+2, y, size).Scale(w.Z))
	return c.Add(imgTexel(x0+3, y, size).Scale(w.W))
}

// imgTexel reads the Img texel at the integral position (x, y), clamped to
// the texture's edges, by a nearest-filtered sample at its center.
func imgTexel(x, y float32, size sl.Vec2) sl.Vec4 {
	cx := sl.Clamp(x, 0, size.X-1)
	cy := sl.Clamp(y, 0, size.Y-1)
	return Img.Sample(sl.V2((cx+0.5)/size.X, (cy+0.5)/size.Y))
}
