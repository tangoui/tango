package shaders

import "github.com/tangoui/tango/internal/render/shader/sl"

// Backdrop blur passes (paint.BackdropBlur; the algorithm is described at
// the top of ui/paint/blur.go, whose float32 code these functions mirror
// operation by operation).
//
// Every pass is a KindPatch instance with Kind.W >= 2 sampling the Layer
// texture bound to its batch, as a layer composite does: UV is the layer's
// valid texels (0, 0, w, h normalized by the texture size) and Kind.Y,
// Kind.Z half a texel, so a device that backs the layer with a larger
// pooled texture rescales them like a composite's. Dash.XY is where the
// layer's texel (0, 0) lies in the pass's source space and Dash.ZW that
// space's size: reads are clamped to the source space, and are transparent
// where the layer holds nothing. Params.ZW is the device position of the
// output's texel (0, 0) (the target's Bounds.Min for the passes into
// reduced layers, the plan's Src.Min for the composite).
//
//	Kind.W = BlurDown:      Params.X the factor (box average Down×Down)
//	Kind.W = BlurH, BlurV:  Params.X the reduced sigma, Params.Y the taps
//	Kind.W = BlurComposite: Params.X the factor; Rect, Radii and the clip
//	                        are the op's rounded rectangle (coverage as
//	                        any instance's)
//
// The reduced passes write whole 8-bit levels (blurQuantize), so the
// target's unorm conversion is exact and every backend stores the same
// bytes paint.CPU does.
const (
	BlurDown      = 2
	BlurH         = 3
	BlurV         = 4
	BlurComposite = 5
)

// Loop bounds: paint.MaxBlurDown and paint.MaxBlurTaps (constant bounds
// with an early break keep every shader compiler happy).
const (
	blurMaxDown = 32
	blurMaxTaps = 24
)

// blurTexel returns the levels (0..255) of texel (x, y) of a pass's source
// space: clamped to the space (dash.ZW), shifted into the layer (dash.XY),
// transparent outside the layer's valid texels (valid), read at the
// texel's center (ht is half a texel of the layer texture).
func blurTexel(x, y float32, ht, valid sl.Vec2, dash sl.Vec4) sl.Vec4 {
	cx := sl.Clamp(x, 0, dash.Z-1) - dash.X
	cy := sl.Clamp(y, 0, dash.W-1) - dash.Y
	if cx < 0 || cy < 0 || cx > valid.X-0.5 || cy > valid.Y-0.5 {
		return sl.V4(0, 0, 0, 0)
	}
	t := Layer.Sample(sl.V2((cx+0.5)*2*ht.X, (cy+0.5)*2*ht.Y))
	return sl.V4(sl.Floor(t.X*255+0.5), sl.Floor(t.Y*255+0.5), sl.Floor(t.Z*255+0.5), sl.Floor(t.W*255+0.5))
}

// blurQuantize rounds levels v half up and returns them as 0..1 values.
func blurQuantize(v sl.Vec4) sl.Vec4 {
	return sl.V4(sl.Floor(v.X+0.5)/255, sl.Floor(v.Y+0.5)/255, sl.Floor(v.Z+0.5)/255, sl.Floor(v.W+0.5)/255)
}

// BlurPass shades pixel center pc of a blur pass instance (see above) with
// coverage cov; uv, kind, params and dash are the instance's fields.
func BlurPass(pc sl.Vec2, uv, kind, params, dash sl.Vec4, cov float32) sl.Vec4 {
	ht := sl.V2(kind.Y, kind.Z)
	valid := sl.V2(uv.Z/(2*kind.Y), uv.W/(2*kind.Z))
	o := params.ZW()
	if kind.W > BlurV+0.5 {
		// Composite: bilinear in levels at the pixel center mapped to
		// reduced texels, edges clamped.
		d := params.X
		u := (pc.X-o.X)/d - 0.5
		v := (pc.Y-o.Y)/d - 0.5
		x0 := sl.Floor(u)
		y0 := sl.Floor(v)
		fx := u - x0
		fy := v - y0
		a := blurTexel(x0, y0, ht, valid, dash)
		b := blurTexel(x0+1, y0, ht, valid, dash)
		e := blurTexel(x0, y0+1, ht, valid, dash)
		f := blurTexel(x0+1, y0+1, ht, valid, dash)
		top := sl.MixV4(a, b, fx)
		bot := sl.MixV4(e, f, fx)
		return sl.MixV4(top, bot, fy).Scale(1.0 / 255).Scale(cov)
	}
	i := pc.X - o.X - 0.5
	j := pc.Y - o.Y - 0.5
	if kind.W < BlurDown+0.5 {
		d := params.X
		sum := sl.V4(0, 0, 0, 0)
		for vy := float32(0); vy < blurMaxDown; vy++ {
			if vy > d-0.5 {
				break
			}
			for ux := float32(0); ux < blurMaxDown; ux++ {
				if ux > d-0.5 {
					break
				}
				sum = sum.Add(blurTexel(i*d+ux, j*d+vy, ht, valid, dash))
			}
		}
		return blurQuantize(sum.Scale(1 / (d * d)))
	}
	dx := float32(1)
	dy := float32(0)
	if kind.W > BlurH+0.5 {
		dx = 0
		dy = 1
	}
	k := 1 / (params.X * 1.4142135)
	n := params.Y
	acc := sl.V4(0, 0, 0, 0)
	ws := float32(0)
	for s := float32(0); s < 2*blurMaxTaps+1; s++ {
		if s > 2*n+0.5 {
			break
		}
		t := s - n
		w := sl.Erf((t+0.5)*k) - sl.Erf((t-0.5)*k)
		c := blurTexel(i+t*dx, j+t*dy, ht, valid, dash)
		acc = acc.Add(c.Scale(w))
		ws = ws + w
	}
	return blurQuantize(acc.Scale(1 / ws))
}
