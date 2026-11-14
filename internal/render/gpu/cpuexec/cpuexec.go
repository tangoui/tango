// Package cpuexec runs the GPU quad shader (internal/render/gpu/shaders) on the CPU, so
// the GPU renderer's pipeline can be tested without a GPU.
//
// It mimics the Metal pipeline: for each instance it runs shaders.VS for the
// four vertices of the triangle strip, maps the clip-space positions back to
// device pixels with the same viewport transform Metal uses, rasterizes the
// two triangles at pixel centers, interpolates the varyings (smooth fields
// linearly in screen space, which is exact because every position has
// w = 1; flat fields from the provoking first vertex of each triangle), and
// calls shaders.FS. Fragment Pos is set as Metal's [[position]] is: the
// pixel-center window coordinates (x+0.5, y+0.5, z, 1/w).
package cpuexec

import (
	"image"
	"image/color"
	"math"
	"reflect"
	"strings"
	"sync"
	"unsafe"

	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
)

// Run draws insts in order into dst, a w×h render target, with mask bound as
// texture 0 (R8 mask atlas, nearest filtering), color as texture 1 (RGBA,
// nearest), ramp as texture 2 (the RGBA gradient ramps, linear
// filtering) and img as texture 3 (the batch's image texture, premultiplied
// RGBA, nearest), as the quad shader declares them; any may be nil. Each
// fragment is blended premultiplied "source over" into dst and
// immediately rounded to 8 bits, so dst is quantized after every instance
// as an 8-bit GPU render target (and paint.CPU) is. Pixels of dst outside
// the w×h target are left alone. It is Draw for the main target (origin
// zero) with no layer bound.
//
// Run must not be called concurrently: it sets the package-level shader
// resources shaders.FrameU, shaders.Mask, shaders.Color, shaders.Ramp,
// shaders.Img and shaders.Layer.
func Run(insts []shaders.Instance, w, h int, mask, color, ramp, img image.Image, dst *image.RGBA) {
	Draw(insts, Target{Size: image.Pt(w, h)}, Textures{Mask: mask, Color: color, Ramp: ramp, Img: img}, dst)
}

// Target is a render target: Size texels whose top-left is the device
// pixel Origin (main-target coordinates): zero for the frame's target, an
// offscreen layer's Bounds.Min for a layer (gpu.Frame.Layers).
type Target struct {
	Size   image.Point
	Origin image.Point
}

// Textures are the textures bound to the quad shader's slots 0..4: Mask
// (R8, nearest), Color (RGBA, nearest), Ramp (RGBA, linear), Img (RGBA,
// nearest) and Layer (an offscreen layer, RGBA, nearest). Any may be nil.
type Textures struct {
	Mask, Color, Ramp, Img, Layer image.Image
}

// Draw is Run for any render target: it draws insts into dst, which holds
// t's texels in target-local coordinates (texel (0, 0) is device pixel
// t.Origin), with shaders.FrameU's Origin set to t.Origin as a device sets
// it for a layer. Instance geometry is in main-target device pixels.
func Draw(insts []shaders.Instance, t Target, tex Textures, dst *image.RGBA) {
	w, h := t.Size.X, t.Size.Y
	if w <= 0 || h <= 0 {
		return
	}
	shaders.FrameU = shaders.Frame{ViewSize: sl.V2(float32(w), float32(h)), Origin: sl.V2(float32(t.Origin.X), float32(t.Origin.Y))}
	shaders.Mask = texture(tex.Mask, sl.Nearest)
	shaders.Color = texture(tex.Color, sl.Nearest)
	shaders.Ramp = texture(tex.Ramp, sl.Linear)
	shaders.Img = texture(tex.Img, sl.Nearest)
	shaders.Layer = texture(tex.Layer, sl.Nearest)
	defer func() {
		shaders.Mask, shaders.Color, shaders.Ramp, shaders.Img, shaders.Layer = sl.Texture2D{}, sl.Texture2D{}, sl.Texture2D{}, sl.Texture2D{}, sl.Texture2D{}
	}()

	// Clip rasterization to the part of the target that dst holds.
	b := image.Rect(0, 0, w, h).Intersect(dst.Bounds())
	if b.Empty() {
		return
	}

	for _, in := range insts {
		var v [4]shaders.Varyings
		var px [4]sl.Vec2
		for vid := range v {
			v[vid] = shaders.VS(in, uint32(vid))
			px[vid] = toPixel(v[vid].Pos, w, h)
		}
		// Triangle strip: (0,1,2) and (1,3,2), both wound like the first;
		// the provoking vertex is each triangle's first.
		drawTriangle(dst, b, [3]shaders.Varyings{v[0], v[1], v[2]}, [3]sl.Vec2{px[0], px[1], px[2]})
		drawTriangle(dst, b, [3]shaders.Varyings{v[1], v[3], v[2]}, [3]sl.Vec2{px[1], px[3], px[2]})
	}
}

func texture(img image.Image, f sl.Filter) sl.Texture2D {
	if img == nil {
		return sl.Texture2D{}
	}
	return sl.NewTexture(img, f)
}

// toPixel applies Metal's viewport transform for a full-target viewport:
// clip (x, y up) to window pixels (y down).
func toPixel(pos sl.Vec4, w, h int) sl.Vec2 {
	x, y := pos.X/pos.W, pos.Y/pos.W
	return sl.V2((x+1)/2*float32(w), (1-y)/2*float32(h))
}

// pixelIndex clamps f to [lo, hi] in float before converting it to int, so
// huge, infinite and NaN coordinates (whose int conversion is
// implementation-defined) stay well defined.
func pixelIndex(f float64, lo, hi int) int {
	if !(f >= float64(lo)) { // also catches NaN
		return lo
	}
	if f > float64(hi) {
		return hi
	}
	return int(f)
}

// edge is the edge function of a→b at p (twice the signed triangle area).
// It is evaluated with the endpoints in a canonical order and negated when
// swapped, so the two triangles sharing an edge get exactly opposite values
// at every pixel center (rounding, or fused multiply-add, would otherwise
// make a center on the shared diagonal land in both triangles or in
// neither). Real GPUs get the same watertightness from fixed-point edges.
func edge(a, b, p sl.Vec2) float32 {
	if b.X < a.X || (b.X == a.X && b.Y < a.Y) {
		return -edgeFn(b, a, p)
	}
	return edgeFn(a, b, p)
}

func edgeFn(a, b, p sl.Vec2) float32 {
	return (b.X-a.X)*(p.Y-a.Y) - (b.Y-a.Y)*(p.X-a.X)
}

// topLeft reports whether a→b is a top or left edge for the triangle
// orientation sign, so pixel centers exactly on an edge shared by the two
// triangles are drawn once (the D3D/Metal fill rule).
func topLeft(a, b sl.Vec2, sign float32) bool {
	dx, dy := (b.X-a.X)*sign, (b.Y-a.Y)*sign
	// With y down and positive orientation (clockwise on screen): a top
	// edge is horizontal and goes right; a left edge goes up.
	return (dy == 0 && dx > 0) || dy < 0
}

func drawTriangle(dst *image.RGBA, b image.Rectangle, v [3]shaders.Varyings, p [3]sl.Vec2) {
	area := edge(p[0], p[1], p[2])
	if area == 0 || math.IsNaN(float64(area)) {
		return
	}
	sign := float32(1)
	if area < 0 {
		sign = -1
	}
	minX := pixelIndex(math.Floor(float64(min(p[0].X, p[1].X, p[2].X))), b.Min.X, b.Max.X-1)
	maxX := pixelIndex(math.Ceil(float64(max(p[0].X, p[1].X, p[2].X))), b.Min.X, b.Max.X-1)
	minY := pixelIndex(math.Floor(float64(min(p[0].Y, p[1].Y, p[2].Y))), b.Min.Y, b.Max.Y-1)
	maxY := pixelIndex(math.Ceil(float64(max(p[0].Y, p[1].Y, p[2].Y))), b.Min.Y, b.Max.Y-1)
	var tl [3]bool
	for i := range 3 {
		tl[i] = topLeft(p[(i+1)%3], p[(i+2)%3], sign)
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			c := sl.V2(float32(x)+0.5, float32(y)+0.5)
			// Barycentric weights: b[i] is opposite vertex i.
			var bw [3]float32
			inside := true
			for i := range 3 {
				e := edge(p[(i+1)%3], p[(i+2)%3], c) * sign
				if e < 0 || (e == 0 && !tl[i]) {
					inside = false
					break
				}
				bw[i] = e / (area * sign)
			}
			if !inside {
				continue
			}
			frag := interpolate(v, bw)
			z := v[0].Pos.Z*bw[0] + v[1].Pos.Z*bw[1] + v[2].Pos.Z*bw[2]
			frag.Pos = sl.V4(c.X, c.Y, z, 1)
			src := shaders.FS(frag)
			blend(dst, x, y, src)
		}
	}
}

// interpolate builds the fragment input from the triangle's vertex outputs:
// fields tagged flat come from v[0] (the provoking vertex), the others are
// weighted sums of their float32 components (integer varyings are flat in
// Metal; they keep the provoking value). Reflection, once (smoothOffsets),
// keeps this in step with whatever fields shaders.Varyings declares.
func interpolate(v [3]shaders.Varyings, bw [3]float32) shaders.Varyings {
	out := v[0]
	for _, off := range smoothOffsets() {
		a := *(*float32)(unsafe.Add(unsafe.Pointer(&v[0]), off))
		b := *(*float32)(unsafe.Add(unsafe.Pointer(&v[1]), off))
		c := *(*float32)(unsafe.Add(unsafe.Pointer(&v[2]), off))
		*(*float32)(unsafe.Add(unsafe.Pointer(&out), off)) = a*bw[0] + b*bw[1] + c*bw[2]
	}
	return out
}

// smoothOffsets are the byte offsets in shaders.Varyings of the float32
// components interpolate interpolates: those of every field not tagged
// position or flat, nested structs included.
var smoothOffsets = sync.OnceValue(func() []uintptr {
	var offs []uintptr
	var walk func(t reflect.Type, base uintptr)
	walk = func(t reflect.Type, base uintptr) {
		switch t.Kind() {
		case reflect.Float32:
			offs = append(offs, base)
		case reflect.Struct:
			for i := range t.NumField() {
				walk(t.Field(i).Type, base+t.Field(i).Offset)
			}
		}
	}
	t := reflect.TypeFor[shaders.Varyings]()
	for i := range t.NumField() {
		f := t.Field(i)
		if tag := f.Tag.Get("sl"); tag == "position" || strings.Contains(tag, "flat") {
			continue
		}
		walk(f.Type, f.Offset)
	}
	return offs
})

// blend composites premultiplied src over dst's pixel (x, y) and rounds the
// result to 8 bits. The two strip triangles never share a pixel, so this
// quantizes once per instance.
func blend(dst *image.RGBA, x, y int, src sl.Vec4) {
	i := dst.PixOffset(x, y)
	p := dst.Pix[i : i+4 : i+4]
	inv := 1 - src.W
	p[0] = to8(src.X + float32(p[0])/255*inv)
	p[1] = to8(src.Y + float32(p[1])/255*inv)
	p[2] = to8(src.Z + float32(p[2])/255*inv)
	p[3] = to8(src.W + float32(p[3])/255*inv)
}

// to8 converts a 0..1 channel to 8 bits, rounding to nearest.
func to8(f float32) uint8 {
	if !(f > 0) {
		return 0
	}
	if f >= 1 {
		return 255
	}
	return uint8(f*255 + 0.5)
}

// CopyRect copies the pixels of src within dst to the rectangle of the same
// size at dp, reading them as they were before the copy (src and the
// destination may overlap), as a device applies a gpu.CopyRegion. The
// rectangles must lie inside dst.
func CopyRect(dst *image.RGBA, src image.Rectangle, dp image.Point) {
	if src.Empty() {
		return
	}
	tmp := make([]byte, 4*src.Dx()*src.Dy())
	row := 4 * src.Dx()
	for y := range src.Dy() {
		copy(tmp[y*row:][:row], dst.Pix[dst.PixOffset(src.Min.X, src.Min.Y+y):][:row])
	}
	for y := range src.Dy() {
		copy(dst.Pix[dst.PixOffset(dp.X, dp.Y+y):][:row], tmp[y*row:][:row])
	}
}

// Fill sets the pixels of r within dst to the premultiplied color c, as a
// device clears a dirty rectangle of a partial frame.
func Fill(dst *image.RGBA, r image.Rectangle, c color.Color) {
	r = r.Intersect(dst.Rect)
	p := color.RGBAModel.Convert(c).(color.RGBA)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := dst.Pix[dst.PixOffset(r.Min.X, y):][:4*r.Dx()]
		for i := 0; i < len(row); i += 4 {
			row[i], row[i+1], row[i+2], row[i+3] = p.R, p.G, p.B, p.A
		}
	}
}
