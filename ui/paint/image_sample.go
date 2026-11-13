package paint

import (
	"image"
	"image/color"
	"math"
)

// This file holds the image filter both renderers share: paint.CPU draws
// images with SampleCatmullRomF and the GPU renderer's KindImage instances
// run shaders.SampleCatmullRom, the same formula operation by operation
// (ui/gpu/shaders TestPaintCatmullRomMatchesShader keeps them equal).

// CatmullRomWeights returns the weights of the four texels around a sample
// point whose fractional position past the second texel's center is f
// (0 ≤ f < 1): the Catmull-Rom cubic (Mitchell–Netravali B = 0, C = 0.5),
//
//	w0 = -0.5f³ + f² - 0.5f
//	w1 =  1.5f³ - 2.5f² + 1
//	w2 = -1.5f³ + 2f² + 0.5f
//	w3 =  0.5f³ - 0.5f²
//
// They sum to 1 and give (0, 1, 0, 0) at f = 0, so texel centers sample
// exactly. shaders.CatmullRomWeights is the same.
func CatmullRomWeights(f float32) [4]float32 {
	f2 := f * f
	f3 := f2 * f
	return [4]float32{
		-0.5*f3 + f2 - 0.5*f,
		1.5*f3 - 2.5*f2 + 1,
		-1.5*f3 + 2*f2 + 0.5*f,
		0.5*f3 - 0.5*f2,
	}
}

// SampleCatmullRomF samples the premultiplied image src at the normalized
// position (u, v) (0..1 spans src's bounds; texel (i, j) is centered at
// ((i+0.5)/w, (j+0.5)/h)) with the Catmull-Rom filter over the 4×4 texels
// around it, edges clamped (texels outside src repeat the border). The
// cubic's negative lobes can overshoot, so the result is clamped to valid
// premultiplied color: alpha to 0..1, then each color channel to 0..alpha.
// The channels are 0..1.
//
// The arithmetic is shaders.SampleCatmullRom's, step by step: the texel
// coordinate x = u*w - 0.5 splits into x0 = floor(x) and the fraction; the
// four rows y0-1 … y0+2 are each filtered horizontally (texels x0-1 …
// x0+2, weighted left to right) and the rows are then weighted top to
// bottom. src's 8-bit channels read as v/255, which is what the GPU's
// RGBA8Unorm texture returns. An empty src samples transparent.
func SampleCatmullRomF(src *image.RGBA, u, v float32) [4]float32 {
	w, h := float32(src.Rect.Dx()), float32(src.Rect.Dy())
	if !(w > 0) || !(h > 0) {
		return [4]float32{}
	}
	x := u*w - 0.5
	y := v*h - 0.5
	x0 := float32(math.Floor(float64(x)))
	y0 := float32(math.Floor(float64(y)))
	wx := CatmullRomWeights(x - x0)
	wy := CatmullRomWeights(y - y0)
	var c [4]float32
	for j := range 4 {
		r := catmullRomRow(src, x0-1, y0-1+float32(j), wx, w, h)
		if j == 0 {
			c = scale4(r, wy[0])
		} else {
			c = add4(c, scale4(r, wy[j]))
		}
	}
	a := clampF(c[3], 0, 1)
	return [4]float32{clampF(c[0], 0, a), clampF(c[1], 0, a), clampF(c[2], 0, a), a}
}

// SampleCatmullRom is SampleCatmullRomF rounded to 8-bit premultiplied
// color.
func SampleCatmullRom(src *image.RGBA, u, v float32) color.RGBA {
	c := SampleCatmullRomF(src, u, v)
	return color.RGBA{unorm8(c[0]), unorm8(c[1]), unorm8(c[2]), unorm8(c[3])}
}

// catmullRomRow filters row y (a texel index, clamped) horizontally:
// texels x0 … x0+3 weighted by wx (shaders.catmullRomRow).
func catmullRomRow(src *image.RGBA, x0, y float32, wx [4]float32, w, h float32) [4]float32 {
	c := scale4(texelF(src, x0, y, w, h), wx[0])
	c = add4(c, scale4(texelF(src, x0+1, y, w, h), wx[1]))
	c = add4(c, scale4(texelF(src, x0+2, y, w, h), wx[2]))
	return add4(c, scale4(texelF(src, x0+3, y, w, h), wx[3]))
}

// texelF returns the texel at the integral position (x, y), clamped to
// src's edges (NaN reads texel 0, as the nearest-filtered texture lookup
// of shaders.imgTexel does), as 0..1 channels.
func texelF(src *image.RGBA, x, y, w, h float32) [4]float32 {
	i := texelIndex(clampF(x, 0, w-1), int(w))
	j := texelIndex(clampF(y, 0, h-1), int(h))
	p := src.Pix[src.PixOffset(src.Rect.Min.X+i, src.Rect.Min.Y+j):]
	return [4]float32{float32(p[0]) / 255, float32(p[1]) / 255, float32(p[2]) / 255, float32(p[3]) / 255}
}

// texelIndex converts a clamped texel coordinate to an index in [0, n-1]
// (NaN is 0).
func texelIndex(f float32, n int) int {
	if !(f > 0) {
		return 0
	}
	return min(int(f), n-1)
}

// clampF is sl.Clamp: min(max(x, lo), hi) with GLSL's min/max (so a NaN x
// stays NaN).
func clampF(x, lo, hi float32) float32 {
	if x < lo {
		x = lo
	}
	if hi < x {
		x = hi
	}
	return x
}

func scale4(c [4]float32, s float32) [4]float32 {
	return [4]float32{c[0] * s, c[1] * s, c[2] * s, c[3] * s}
}

func add4(a, b [4]float32) [4]float32 {
	return [4]float32{a[0] + b[0], a[1] + b[1], a[2] + b[2], a[3] + b[3]}
}

// ImageReduction returns the size an image with bounds b is first reduced
// to (with an area-averaging box filter, PrepareImage) before it is drawn
// devW×devH device pixels large, or 0, 0 when it is drawn from its full
// resolution. Each axis is reduced on its own: one more than twice the
// drawn size becomes ceil(drawn size), the other keeps the source size
// (so 4000×100 drawn 100×100 is reduced to 100×100, 4000×100 drawn
// 100×80 to 100×100). Without it the 4×4 Catmull-Rom filter would skip
// source texels and alias. Both renderers prepare images this way, so
// they sample the same texels.
func ImageReduction(b image.Rectangle, devW, devH float32) (w, h int) {
	axis := func(src int, dev float32) (int, bool) {
		t := math.Ceil(float64(dev))
		if !(t >= 1) || float64(src) <= 2*t {
			return src, false
		}
		return int(t), true
	}
	w, rx := axis(b.Dx(), devW)
	h, ry := axis(b.Dy(), devH)
	if !rx && !ry {
		return 0, 0
	}
	return w, h
}

// PrepareImage returns the premultiplied, origin-anchored pixels an image
// is sampled from: ToRGBA(src) (src itself when it already is such an
// *image.RGBA), reduced to w×h when w > 0 (ImageReduction's result) by
// averaging, per output texel, the source area it covers (a separable box
// filter with fractional edge weights; an axis whose size does not change
// is copied).
func PrepareImage(src image.Image, w, h int) *image.RGBA {
	img := ToRGBA(src)
	if w > 0 && h > 0 {
		img = boxResize(img, w, h)
	}
	return img
}

// boxResize reduces the origin-anchored premultiplied m to w×h (each at
// most m's size) by area averaging: rows first, then columns, in float32,
// rounded to 8 bits once.
func boxResize(m *image.RGBA, w, h int) *image.RGBA {
	sw, sh := m.Rect.Dx(), m.Rect.Dy()
	w, h = max(1, min(w, sw)), max(1, min(h, sh))
	// Horizontal pass: sh rows of w texels.
	tmp := make([]float32, sh*w*4)
	for y := 0; y < sh; y++ {
		row := m.Pix[y*m.Stride:]
		boxAxis(sw, w, func(x, sx int, wt float32) {
			for c := 0; c < 4; c++ {
				tmp[(y*w+x)*4+c] += float32(row[sx*4+c]) * wt
			}
		})
	}
	acc := make([]float32, w*h*4)
	boxAxis(sh, h, func(y, sy int, wt float32) {
		for i := 0; i < w*4; i++ {
			acc[y*w*4+i] += tmp[sy*w*4+i] * wt
		}
	})
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for i := 0; i < w*4; i++ {
			out.Pix[y*out.Stride+i] = uint8(min(255, acc[y*w*4+i]+0.5))
		}
	}
	return out
}

// boxAxis calls add(o, s, weight) for every output index o < n of an axis
// reduced from src texels and every source texel s overlapping o's
// footprint [o*k, (o+1)*k), k = src/n; the weights of each o sum to 1.
func boxAxis(src, n int, add func(o, s int, wt float32)) {
	k := float64(src) / float64(n)
	for o := 0; o < n; o++ {
		lo, hi := float64(o)*k, float64(o+1)*k
		for s := int(lo); s < src && float64(s) < hi; s++ {
			ov := math.Min(hi, float64(s+1)) - math.Max(lo, float64(s))
			if ov > 0 {
				add(o, s, float32(ov/k))
			}
		}
	}
}
