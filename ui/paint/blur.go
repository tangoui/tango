package paint

import (
	"image"
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// Backdrop blur (BackdropBlur): the algorithm both renderers run.
//
// The blur works in device pixels on 8-bit premultiplied RGBA, in passes
// that each round to whole 8-bit levels, so paint.CPU and the GPU quad
// shader (internal/render/gpu/shaders.BlurPass, the same float32 formulas)
// produce the same pixels up to a rare one-level rounding difference:
//
//  1. Plan (PlanBackdropBlur): sigma = min(Radius, MaxBackdropBlur) ×
//     scale device pixels, at most maxBlurSigma. The downsampling factor
//     Down is the smallest power of two (at most maxBlurDown) that brings
//     the reduced sigma Sigma = sigma/Down to 4 or below; the kernel's half
//     width is Taps = ceil(3·Sigma) reduced texels. Out is the op's device
//     rectangle rounded out and clipped; Src, the pixels read, is Out grown
//     by (Taps+2)·Down, its corners moved out to multiples of Down
//     (absolute device coordinates, so the reduced grid never depends on
//     where Out was cut by a clip or a partial redraw) and clipped to the
//     frame.
//  2. Source: the target's pixels of Src. A read beyond Src clamps to its
//     edge (only the frame's edge is ever reached: the margin covers the
//     kernel); inside a group, pixels its layer does not hold are
//     transparent.
//  3. Down (only when Down > 1): reduced texel (i, j) is the box average of
//     the Down×Down source pixels from (i·Down, j·Down), each channel
//     summed in whole levels, divided by Down² (exact) and rounded half up.
//  4. Blur, horizontally then vertically over the reduced image (clamped
//     at its edges): each tap t in -Taps..Taps weighs
//     erf((t+½)k) − erf((t−½)k), k = 1/(Sigma·√2) — the Gaussian's
//     integral over the texel — and the weighted level sum, divided by the
//     weights' sum, is rounded half up.
//  5. Composite: every pixel of Out samples the blurred reduced image
//     bilinearly (in levels, edges clamped) at its center mapped to
//     reduced texels, ((p − Src.Min)/Down − ½), and the result is
//     composited source-over with the rounded rectangle's coverage times
//     the clip's, as a FillRect's color would be.
//
// Cost: a pass touches every pixel of its output once; the downsampling
// pass reads each source pixel once, the two blur passes read 2·Taps+1
// reduced texels per reduced texel (at most 9 below sigma 1.5, 25 when
// Sigma is 4), and the composite reads 4. With Down chosen so that Sigma
// stays at or below 4, the work is about (Out area + margin) plus a
// constant per reduced texel, whatever the radius: a 1280×56 bar at scale 2
// with Radius 12 (sigma 24, Down 8, Taps 12) reads about 0.8 Mpx of
// source and blurs a 336×28 reduced image. The radius is capped
// (MaxBackdropBlur) because the margin, and with it the area read and the
// region a partial redraw must repaint, grows with it.

// MaxBackdropBlur is the largest BackdropBlur.Radius (logical pixels);
// larger radii are clamped to it.
const MaxBackdropBlur = 64

const (
	maxBlurSigma = 256 // device pixels: MaxBackdropBlur at scale 4
	maxBlurDown  = 32  // so Sigma stays <= maxBlurSigma/maxBlurDown = 8, Taps <= 24
	// MaxBlurTaps is the largest BlurPlan.Taps (the GPU shader loops over
	// at most 2·MaxBlurTaps+1 taps).
	MaxBlurTaps = 24
	// MaxBlurDown is the largest BlurPlan.Down.
	MaxBlurDown = maxBlurDown
)

// BlurPlan is the device-pixel plan of one BackdropBlur (see the top of
// blur.go): paint.CPU and the GPU renderer run the same plan.
type BlurPlan struct {
	Out   image.Rectangle // pixels the blur may change
	Src   image.Rectangle // pixels it reads (inside the frame)
	Down  int             // downsampling factor, a power of two
	Sigma float32         // Gaussian sigma in reduced texels
	Taps  int             // kernel half width in reduced texels
	Small image.Point     // reduced image size: Src.Size()/Down rounded up
}

// BlurSigma returns the device sigma of a BackdropBlur of radius at scale:
// min(radius, MaxBackdropBlur)·scale, at most 256; 0 when nothing is
// blurred (radius <= 0 or NaN).
func BlurSigma(radius, scale float32) float32 {
	if !(radius > 0) || !(scale > 0) {
		return 0
	}
	return min(min(radius, MaxBackdropBlur)*scale, maxBlurSigma)
}

// blurKernel returns the downsampling factor, reduced sigma and taps for
// device sigma s > 0.
func blurKernel(s float32) (down int, sigma float32, taps int) {
	down = 1
	for s/float32(down) > 4 && down < maxBlurDown {
		down *= 2
	}
	sigma = s / float32(down)
	taps = max(1, min(MaxBlurTaps, int(math.Ceil(float64(3*sigma)))))
	return down, sigma, taps
}

// BackdropBlurReach returns how far, in logical pixels, a BackdropBlur of
// radius at scale reads beyond its rectangle (its source margin, plus a
// pixel for rounding): a change within that distance changes its pixels.
func BackdropBlurReach(radius, scale float32) float32 {
	s := BlurSigma(radius, scale)
	if s == 0 {
		return 0
	}
	down, _, taps := blurKernel(s)
	return float32((taps+3)*down)/scale + 1
}

// blurOut returns the device pixels a blur over the device rectangle dr
// may change: dr rounded out.
func blurOut(dr geom.Rect) image.Rectangle {
	return image.Rect(clampPx(math.Floor(float64(dr.X))), clampPx(math.Floor(float64(dr.Y))),
		clampPx(math.Ceil(float64(dr.X+dr.W))), clampPx(math.Ceil(float64(dr.Y+dr.H))))
}

// PlanBackdropBlur plans a blur of device sigma sigma (BlurSigma) over the
// device rectangle dr, within the clip rectangle clip, in a frame of
// device pixels frame. ok is false when it changes nothing (no sigma,
// non-finite or empty rectangle, nothing inside the clip and frame).
func PlanBackdropBlur(dr geom.Rect, sigma float32, clip, frame image.Rectangle) (p BlurPlan, ok bool) {
	if !(sigma > 0) || !finiteRect(dr) || !(dr.W > 0) || !(dr.H > 0) {
		return p, false
	}
	p.Out = blurOut(dr).Intersect(clip).Intersect(frame)
	if p.Out.Empty() {
		return p, false
	}
	p.Down, p.Sigma, p.Taps = blurKernel(sigma)
	d := p.Down
	m := (p.Taps + 2) * d
	floorTo := func(v int) int { // to a multiple of d, towards -inf
		if v >= 0 {
			return v / d * d
		}
		return -((-v + d - 1) / d * d)
	}
	ceilTo := func(v int) int { return -floorTo(-v) }
	p.Src = image.Rect(floorTo(p.Out.Min.X-m), floorTo(p.Out.Min.Y-m), ceilTo(p.Out.Max.X+m), ceilTo(p.Out.Max.Y+m)).Intersect(frame)
	p.Small = image.Pt((p.Src.Dx()+d-1)/d, (p.Src.Dy()+d-1)/d)
	return p, true
}

// quantize rounds level v half up to a whole level (the shader's
// blurQuantize, which returns it divided by 255).
func quantize(v float32) uint8 {
	f := float32(math.Floor(float64(v + 0.5)))
	return uint8(min(max(f, 0), 255))
}

// blurImage is a w×h image of 8-bit premultiplied RGBA levels.
type blurImage struct {
	w, h int
	pix  []uint8
}

// blurBackdrop runs steps 2–4 of plan p (see the top of this file) over
// the pixels of src (the target as drawn so far; pixels outside src.Rect
// are transparent) and returns the blurred reduced image, p.Small in
// size. The arithmetic is the shader's (shaders.BlurPass), with the
// per-pixel constants (tap weights, their sum) computed once.
func blurBackdrop(src *image.RGBA, p BlurPlan) *blurImage {
	// The source space: Src, transparent where the target holds nothing
	// (the shader reads the grab layer that way).
	sw, sh := p.Src.Dx(), p.Src.Dy()
	cur := &blurImage{w: sw, h: sh, pix: make([]uint8, 4*sw*sh)}
	g := p.Src.Intersect(src.Rect)
	for y := g.Min.Y; y < g.Max.Y; y++ {
		o := ((y-p.Src.Min.Y)*sw + g.Min.X - p.Src.Min.X) * 4
		copy(cur.pix[o:o+4*g.Dx()], src.Pix[src.PixOffset(g.Min.X, y):])
	}
	if d := p.Down; d > 1 {
		// Box average: whole levels summed exactly (as the shader's float
		// sum is), times 1/d² (exact), rounded half up.
		inv := 1 / float32(d*d)
		out := &blurImage{w: p.Small.X, h: p.Small.Y, pix: make([]uint8, 4*p.Small.X*p.Small.Y)}
		for j := 0; j < out.h; j++ {
			for i := 0; i < out.w; i++ {
				var sum [4]int32
				for v := 0; v < d; v++ {
					row := cur.pix[min(j*d+v, sh-1)*sw*4:]
					for u := 0; u < d; u++ {
						t := row[min(i*d+u, sw-1)*4:]
						sum[0] += int32(t[0])
						sum[1] += int32(t[1])
						sum[2] += int32(t[2])
						sum[3] += int32(t[3])
					}
				}
				o := out.pix[(j*out.w+i)*4:]
				for c := range 4 {
					o[c] = quantize(float32(float32(sum[c]) * inv))
				}
			}
		}
		cur = out
	}
	// Tap weights: the Gaussian's integral over each texel, in the
	// shader's order and precision.
	n := p.Taps
	k := 1 / (p.Sigma * 1.4142135)
	w := make([]float32, 2*n+1)
	ws := float32(0)
	for s := range w {
		t := float32(s - n)
		w[s] = erf32((t+0.5)*k) - erf32((t-0.5)*k)
		ws = ws + w[s]
	}
	iw := 1 / ws
	for pass := range 2 {
		out := &blurImage{w: cur.w, h: cur.h, pix: make([]uint8, len(cur.pix))}
		for j := 0; j < cur.h; j++ {
			for i := 0; i < cur.w; i++ {
				var acc [4]float32
				for s, ws := range w {
					x, y := i, j
					if pass == 0 {
						x = min(max(i+s-n, 0), cur.w-1)
					} else {
						y = min(max(j+s-n, 0), cur.h-1)
					}
					c := cur.pix[(y*cur.w+x)*4:]
					acc[0] = acc[0] + float32(float32(c[0])*ws)
					acc[1] = acc[1] + float32(float32(c[1])*ws)
					acc[2] = acc[2] + float32(float32(c[2])*ws)
					acc[3] = acc[3] + float32(float32(c[3])*ws)
				}
				o := out.pix[(j*cur.w+i)*4:]
				for c := range 4 {
					o[c] = quantize(float32(acc[c] * iw))
				}
			}
		}
		cur = out
	}
	return cur
}

// blurAxis is the bilinear sampling of the composite (step 5) along one
// axis for the pixels from lo: the two reduced texels (clamped) and the
// weight of the second, per pixel.
type blurAxis struct {
	lo     int
	i0, i1 []int
	f      []float32
}

// newBlurAxis computes blurAxis for pixels lo..hi-1 of a plan whose
// source starts at src, downsampled by d, n reduced texels long: the
// pixel center c maps to (c − src)/d − ½.
func newBlurAxis(lo, hi, src, d, n int) blurAxis {
	a := blurAxis{lo: lo, i0: make([]int, hi-lo), i1: make([]int, hi-lo), f: make([]float32, hi-lo)}
	fd := float32(d)
	for x := lo; x < hi; x++ {
		u := (float32(x)+0.5-float32(src))/fd - 0.5
		u0 := float32(math.Floor(float64(u)))
		a.i0[x-lo] = min(max(int(u0), 0), n-1)
		a.i1[x-lo] = min(max(int(u0)+1, 0), n-1)
		a.f[x-lo] = u - u0
	}
	return a
}

// backdropBlur draws o (Rect, Radii and Radius already mapped by a
// uniform transform; see execTransformed).
func (r *CPU) backdropBlur(o BackdropBlur) {
	s := r.scale
	sigma := BlurSigma(o.Radius, s)
	dr := geom.Rect{X: o.Rect.X * s, Y: o.Rect.Y * s, W: o.Rect.W * s, H: o.Rect.H * s}
	p, ok := PlanBackdropBlur(dr, sigma, r.clip, r.frame)
	if !ok {
		return
	}
	small := blurBackdrop(r.cur, p)
	ax := newBlurAxis(p.Out.Min.X, p.Out.Max.X, p.Src.Min.X, p.Down, small.w)
	ay := newBlurAxis(p.Out.Min.Y, p.Out.Max.Y, p.Src.Min.Y, p.Down, small.h)
	r.shadeRect(o.Rect, o.Radii, func(_ geom.Point, x, y int) [4]float32 {
		if x < p.Out.Min.X || y < p.Out.Min.Y || x >= p.Out.Max.X || y >= p.Out.Max.Y {
			return [4]float32{} // not drawn: shadeRect stays within Out
		}
		x0, x1, fx := ax.i0[x-ax.lo], ax.i1[x-ax.lo], ax.f[x-ax.lo]
		y0, y1, fy := ay.i0[y-ay.lo], ay.i1[y-ay.lo], ay.f[y-ay.lo]
		a := small.pix[(y0*small.w+x0)*4:]
		b := small.pix[(y0*small.w+x1)*4:]
		e := small.pix[(y1*small.w+x0)*4:]
		f := small.pix[(y1*small.w+x1)*4:]
		var out [4]float32
		for c := range 4 {
			top := float32(float32(a[c])*(1-fx)) + float32(float32(b[c])*fx)
			bot := float32(float32(e[c])*(1-fx)) + float32(float32(f[c])*fx)
			out[c] = float32(float32(top*(1-fy))+float32(bot*fy)) * (1.0 / 255)
		}
		return out
	})
}
