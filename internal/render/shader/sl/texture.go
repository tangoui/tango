package sl

import "image"

// Filter selects how Texture2D.Sample reconstructs values between texels.
type Filter int

const (
	// Nearest returns the texel containing the sample point.
	Nearest Filter = iota
	// Linear bilinearly interpolates the four nearest texel centers.
	Linear
)

// Texture2D is a sampled 2D texture. On the GPU it maps to a Metal texture
// plus sampler; this CPU implementation holds the texels in memory.
//
// Coordinates are normalized with the origin at the top-left; texel (i, j)
// has its center at ((i+0.5)/w, (j+0.5)/h). Addressing is clamp-to-edge.
// Single-channel (R8) textures sample as (r, 0, 0, 1), matching Metal's
// r8Unorm; color textures sample as premultiplied RGBA in 0..1.
type Texture2D struct {
	w, h   int
	pix    []Vec4 // row-major, w*h texels
	filter Filter
}

// NewTexture copies img into a CPU texture. *image.Gray and *image.Alpha
// become R8 textures (the gray level or alpha in the red channel); any other
// image becomes a premultiplied RGBA texture.
func NewTexture(img image.Image, filter Filter) Texture2D {
	b := img.Bounds()
	t := Texture2D{w: b.Dx(), h: b.Dy(), filter: filter}
	t.pix = make([]Vec4, t.w*t.h)
	switch m := img.(type) {
	case *image.Gray:
		for y := 0; y < t.h; y++ {
			for x := 0; x < t.w; x++ {
				v := m.GrayAt(b.Min.X+x, b.Min.Y+y).Y
				t.pix[y*t.w+x] = Vec4{float32(v) / 255, 0, 0, 1}
			}
		}
	case *image.Alpha:
		for y := 0; y < t.h; y++ {
			for x := 0; x < t.w; x++ {
				v := m.AlphaAt(b.Min.X+x, b.Min.Y+y).A
				t.pix[y*t.w+x] = Vec4{float32(v) / 255, 0, 0, 1}
			}
		}
	default:
		for y := 0; y < t.h; y++ {
			for x := 0; x < t.w; x++ {
				// color.Color.RGBA is alpha-premultiplied, 16 bits per channel.
				r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				t.pix[y*t.w+x] = Vec4{float32(r) / 0xffff, float32(g) / 0xffff, float32(bl) / 0xffff, float32(a) / 0xffff}
			}
		}
	}
	return t
}

// Sample returns the filtered texture value at uv. An empty texture samples
// as transparent black.
func (t Texture2D) Sample(uv Vec2) Vec4 {
	if t.w == 0 || t.h == 0 {
		return Vec4{}
	}
	if t.filter == Nearest {
		return t.texel(index(uv.X*float32(t.w), t.w), index(uv.Y*float32(t.h), t.h))
	}
	// Shift by half a texel so integer coordinates land on texel centers.
	x := linearCoord(uv.X*float32(t.w)-0.5, t.w)
	y := linearCoord(uv.Y*float32(t.h)-0.5, t.h)
	x0, y0 := Floor(x), Floor(y)
	fx, fy := x-x0, y-y0
	i, j := int(x0), int(y0)
	top := MixV4(t.texel(i, j), t.texel(i+1, j), fx)
	bot := MixV4(t.texel(i, j+1), t.texel(i+1, j+1), fx)
	return MixV4(top, bot, fy)
}

// index converts a texel-space coordinate to an integer texel index in
// [-1, n]. Clamping in float first keeps huge, infinite and NaN coordinates
// (whose int conversion is implementation-defined) well behaved; texel then
// clamps to the edge.
func index(f float32, n int) int {
	if !(f > -1) { // also catches NaN
		return -1
	}
	if f > float32(n) {
		return n
	}
	return int(Floor(f))
}

// linearCoord clamps a center-relative texel coordinate to [-0.5, n-0.5].
// Under clamp-to-edge addressing this does not change the filtered result,
// but it keeps infinities and NaN out of the interpolation weights.
func linearCoord(f float32, n int) float32 {
	if !(f > -0.5) { // also catches NaN
		return -0.5
	}
	if hi := float32(n) - 0.5; f > hi {
		return hi
	}
	return f
}

// texel returns the texel at (i, j), clamped to the texture edge.
func (t Texture2D) texel(i, j int) Vec4 {
	i = min(max(i, 0), t.w-1)
	j = min(max(j, 0), t.h-1)
	return t.pix[j*t.w+i]
}
