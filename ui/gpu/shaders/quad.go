// Package shaders holds the GPU renderer's shaders, written in Go against
// package sl.
//
// This is an ordinary package (no build tag). The renderer only uses its
// data types (Instance, Frame) to lay out GPU buffers; the shader functions
// are called only by the shader generator (cmd/tanshader, which compiles
// them to ui/gpu/gen), the CPU executor (ui/gpu/cpuexec) and tests, so the
// linker drops them from normal binaries.
package shaders

import "github.com/tangoui/tango/ui/shader/sl"

// Instance kinds (Instance.Kind.X).
const (
	KindSolid = 0 // solid rounded rectangle in Color
	KindMask  = 1 // R8 mask (glyph) tinted with Color
	// KindPatch is a premultiplied RGBA texture multiplied by Color: the
	// patch page (Color), or with Kind.W = 1 an offscreen layer (Layer),
	// which composites an opacity group.
	KindPatch = 2
	// KindStroke is a border inside the rounded rectangle (StrokeCoverage)
	// in Color: Params.X is the width, Params.Y the dash phase, Dash the
	// dash pattern.
	KindStroke = 3
	// KindShadow is a box shadow (ShadowCoverage) of the rounded rectangle
	// in Color: Params.X is the blur's sigma and Params.Y how far the
	// shadow reaches beyond Rect (the quad grows by that much); device
	// pixels.
	KindShadow = 4
	// KindGradient fills the rounded rectangle with a gradient: Params.XY
	// is the start point and Params.ZW the end point (device pixels),
	// Kind.Y the gradient kind (paint.GradientKind: 0 linear, 1 radial,
	// 2 conic), and UV.Y the vertical texture coordinate of the center of
	// the gradient's row in the Ramp texture (premultiplied colors at
	// t = i/255). GradientT gives the parameter t at the pixel center;
	// RampU maps it to the row; the color is dithered (DitherPremul with
	// the pixel's Dither4x4 threshold). Color is unused.
	// With Kind.W = 1 the gradient fills a path instead (paint.Gradient
	// with a Path): Rect is the path's coverage mask in device pixels
	// (Radii zero) and Dash its rectangle in the Mask atlas (x, y, w, h,
	// normalized); the dithered color is multiplied by the mask texel at
	// the pixel center, as paint.CPU multiplies it by the path's coverage.
	KindGradient = 5
	// KindChecker fills the rounded rectangle with a checkerboard: Params.X
	// is the cell size and Params.YZ the anchor (the corner of a cell of
	// the first color; device pixels). The two premultiplied colors reuse
	// the Color field (the anchor cell's) and the Dash field (the other),
	// which KindChecker does not otherwise need.
	KindChecker = 6
	// KindImage draws an image into the rounded rectangle: the Img texture
	// (the image's own premultiplied RGBA8 texture, bound per batch) is
	// sampled with SampleCatmullRom at the pixel center mapped linearly
	// from Rect onto the whole texture; Params.XY is the texture size in
	// texels. Color is unused.
	KindImage = 7
)

// Instance is one quad. All geometry is in device pixels with the origin at
// the top-left of the target.
type Instance struct {
	Rect  sl.Vec4 `sl:"loc=0"` // x, y, w, h
	Radii sl.Vec4 `sl:"loc=1"` // corner radii: top-left, top-right, bottom-right, bottom-left
	// Color is premultiplied RGBA in 0..1. KindPatch multiplies the texel by
	// it component-wise: the host sends (1,1,1,1) for no tint/opacity and
	// (a,a,a,a) for opacity a (a zero Color draws nothing).
	Color sl.Vec4 `sl:"loc=2"`
	UV    sl.Vec4 `sl:"loc=3"` // texture sub-rectangle x, y, w, h, normalized
	// Clip is a rounded clip rectangle (x, y, w, h) with corner radii
	// ClipRadii; a width or height <= 0 means no clip.
	Clip      sl.Vec4 `sl:"loc=4"`
	ClipRadii sl.Vec4 `sl:"loc=5"`
	// Kind.X is the kind (KindSolid … KindImage). For the textured
	// kinds Kind.Y and Kind.Z are half a texel of the sampled texture in
	// normalized units (0.5/width, 0.5/height), filled by the host; FS keeps
	// the sample point that far inside the UV sub-rectangle so nearest
	// sampling never reaches a neighbouring atlas entry. For KindGradient
	// Kind.Y is the gradient kind and Kind.W is 1 for a Path-shaped
	// gradient (0 otherwise); for KindPatch it is 1 for a layer composite
	// (sampling Layer instead of Color). Kind.W is unused by the other kinds.
	Kind sl.Vec4 `sl:"loc=6"`
	// Params.X is KindStroke's stroke width and Params.Y its dash phase;
	// for KindShadow X is sigma and Y the shadow's reach beyond Rect
	// (device pixels), Z and W zero; KindGradient's start and end points;
	// KindChecker's cell size and anchor; KindImage's texture size.
	Params sl.Vec4 `sl:"loc=7"`
	// Dash is KindStroke's dash pattern: up to 4 on/off lengths in device
	// pixels, the first entry <= 0 ending it; all zero is a solid stroke.
	// KindChecker carries its second color here, a Path-shaped
	// KindGradient its mask's atlas rectangle.
	Dash sl.Vec4 `sl:"loc=8"`
}

// Varyings flows from VS to FS. P is the device-pixel position, linearly
// interpolated; the rest is the instance passed through unchanged.
type Varyings struct {
	Pos       sl.Vec4 `sl:"position"`
	P         sl.Vec2 `sl:"loc=0"`
	Rect      sl.Vec4 `sl:"loc=1,flat"`
	Radii     sl.Vec4 `sl:"loc=2,flat"`
	Color     sl.Vec4 `sl:"loc=3,flat"`
	UV        sl.Vec4 `sl:"loc=4,flat"`
	Clip      sl.Vec4 `sl:"loc=5,flat"`
	ClipRadii sl.Vec4 `sl:"loc=6,flat"`
	Kind      sl.Vec4 `sl:"loc=7,flat"`
	Params    sl.Vec4 `sl:"loc=8,flat"`
	Dash      sl.Vec4 `sl:"loc=9,flat"`
}

// Frame is the per-frame uniform block.
type Frame struct {
	ViewSize sl.Vec2 // render target size in device pixels
	// Origin is the device pixel (main-target coordinates) at the render
	// target's top-left: an offscreen layer's position, zero for the main
	// target. Instance geometry and P stay in main-target coordinates.
	Origin sl.Vec2
}

// FrameU is the per-frame uniform.
//
//sl:uniform 0
var FrameU Frame

// Mask is the R8 coverage atlas page sampled by KindMask.
//
//sl:texture 0 filter=nearest
var Mask sl.Texture2D

// Color is the RGBA texture sampled by KindPatch.
//
//sl:texture 1 filter=nearest
var Color sl.Texture2D

// Ramp holds the gradient color ramps sampled by KindGradient: one row of
// paint.RampWidth premultiplied RGBA16Unorm texels per gradient (16 bits,
// so low-contrast gradients keep sub-level steps to dither), sampled at the
// row's center (so the linear filter never mixes rows) and between texel
// centers horizontally. It is a texture of its own, so the half-texel
// clamp of the atlas pages (Kind.YZ) does not apply to it.
//
//sl:texture 2 filter=linear
var Ramp sl.Texture2D

// Img is the image texture sampled by KindImage: one premultiplied RGBA8
// texture per image, bound per batch (gpu.Batch.Image). It is read only at
// texel centers (SampleCatmullRom does the filtering), so nearest.
//
//sl:texture 3 filter=nearest
var Img sl.Texture2D

// Layer is the offscreen layer sampled by a layer composite (KindPatch
// with Kind.W = 1): premultiplied RGBA8, bound per batch (gpu.Batch.Layer).
//
//sl:texture 4 filter=nearest
var Layer sl.Texture2D

// pad grows each quad so that pixels whose centers lie just outside Rect
// (which still get partial coverage) are rasterized.
const pad = 1

// VS expands an instance to a quad (triangle strip, vid 0..3: top-left,
// top-right, bottom-left, bottom-right) covering Rect grown by pad pixels
// (and, for KindShadow, by the shadow's reach Params.Y), and converts
// device pixels (y down, relative to FrameU.Origin) to Metal clip space
// (y up).
//
//sl:vertex
func VS(in Instance, vid uint32) Varyings {
	corner := sl.V2(0, 0)
	if vid == 1 || vid == 3 {
		corner.X = 1
	}
	if vid >= 2 {
		corner.Y = 1
	}
	g := float32(pad)
	if in.Kind.X > KindStroke+0.5 && in.Kind.X < KindShadow+0.5 {
		g = g + sl.Max(in.Params.Y, 0)
	}
	lo := in.Rect.XY().Sub(sl.V2(g, g))
	size := in.Rect.ZW().Add(sl.V2(2*g, 2*g))
	p := lo.Add(size.Mul(corner))
	ndc := p.Sub(FrameU.Origin).Div(FrameU.ViewSize).Scale(2).Sub(sl.V2(1, 1))
	var out Varyings
	out.Pos = sl.V4(ndc.X, -ndc.Y, 0, 1)
	out.P = p
	out.Rect = in.Rect
	out.Radii = in.Radii
	out.Color = in.Color
	out.UV = in.UV
	out.Clip = in.Clip
	out.ClipRadii = in.ClipRadii
	out.Kind = in.Kind
	out.Params = in.Params
	out.Dash = in.Dash
	return out
}

// FS shades one pixel: the instance's paint (Color, the gradient ramp's
// color for KindGradient, the checkerboard's for KindChecker, the
// Catmull-Rom filtered image for KindImage) times its
// coverage (the rounded rect's, the stroke's for KindStroke, the shadow's
// for KindShadow) times the clip coverage, premultiplied.
//
//sl:fragment
func FS(in Varyings) sl.Vec4 {
	// KindStroke and KindShadow paint Color through their own coverage.
	analytic := in.Kind.X > KindPatch+0.5 && in.Kind.X < KindShadow+0.5
	// The dash pattern jumps where the pixel-to-outline mapping switches
	// edges (the diagonal of a square corner), so evaluate it at the exact
	// pixel center paint.CPU uses, not at the interpolated P, which can be
	// off by rounding and land on the other side. Shadows are sampled there
	// too, as paint.CPU's cached shadow mask is, and so are gradients,
	// checkerboards and images (paint.CPU evaluates them at pixel centers).
	pc := sl.V2(sl.Floor(in.P.X)+0.5, sl.Floor(in.P.Y)+0.5)
	var cov float32
	if analytic {
		if in.Kind.X > KindStroke+0.5 {
			cov = ShadowCoverage(pc, in.Rect.XY(), in.Rect.ZW(), in.Radii, in.Params.X)
		} else {
			cov = StrokeCoverage(pc, in.Rect.XY(), in.Rect.ZW(), in.Radii, in.Params.X, in.Dash, in.Params.Y)
		}
	} else {
		cov = RRectCoverage(in.P, in.Rect.XY(), in.Rect.ZW(), in.Radii)
	}
	if in.Clip.Z > 0 && in.Clip.W > 0 {
		cov = cov * RRectCoverage(in.P, in.Clip.XY(), in.Clip.ZW(), in.ClipRadii)
	}
	if cov <= 0 {
		return sl.V4(0, 0, 0, 0)
	}
	if analytic || in.Kind.X < KindSolid+0.5 {
		return in.Color.Scale(cov)
	}
	if in.Kind.X > KindChecker+0.5 {
		// KindImage: Rect maps onto the whole texture.
		t := pc.Sub(in.Rect.XY()).Div(in.Rect.ZW())
		return SampleCatmullRom(t, in.Params.XY()).Scale(cov)
	}
	if in.Kind.X > KindGradient+0.5 {
		// KindChecker: Color for the anchor's cells, Dash for the others.
		sel := CheckerSelect(pc, sl.V2(in.Params.Y, in.Params.Z), in.Params.X)
		if sel > 0.5 {
			return in.Dash.Scale(cov)
		}
		return in.Color.Scale(cov)
	}
	if in.Kind.X > KindShadow+0.5 {
		// KindGradient, ordered-dithered before the 8-bit target write.
		t := GradientT(in.Kind.Y, pc, in.Params.XY(), in.Params.ZW())
		c := Ramp.Sample(sl.V2(RampU(t), in.UV.Y))
		if in.Kind.W > 0.5 {
			// Path-shaped: Rect is the mask, whose texel centers the pixel
			// centers inside Rect map to exactly (pixels outside it have
			// cov 0), so no half-texel clamp is needed.
			mt := pc.Sub(in.Rect.XY()).Div(in.Rect.ZW())
			m := Mask.Sample(in.Dash.XY().Add(in.Dash.ZW().Mul(mt)))
			cov = cov * m.X
		}
		return DitherPremul(c, Dither4x4(pc.X-0.5, pc.Y-0.5)).Scale(cov)
	}
	// Map the pixel to the texture sub-rectangle, then clamp the sample point
	// to the centers of the sub-rectangle's edge texels (Kind.YZ is half a
	// texel): partially covered pixels past Rect's right/bottom edge would
	// otherwise land on the boundary, where nearest sampling picks the
	// first texel of the neighbouring atlas entry.
	t := in.P.Sub(in.Rect.XY()).Div(in.Rect.ZW())
	uv := in.UV.XY().Add(in.UV.ZW().Mul(t))
	lo := in.UV.XY().Add(sl.V2(in.Kind.Y, in.Kind.Z))
	hi := in.UV.XY().Add(in.UV.ZW()).Sub(sl.V2(in.Kind.Y, in.Kind.Z))
	uv = sl.V2(sl.Clamp(uv.X, lo.X, hi.X), sl.Clamp(uv.Y, lo.Y, hi.Y))
	if in.Kind.X < KindMask+0.5 {
		m := Mask.Sample(uv)
		return in.Color.Scale(m.X * cov)
	}
	if in.Kind.W > 0.5 {
		// A layer composite (Rect is whole pixels, so its own coverage is
		// 1 wherever cov > 0): paint.CPU's integer group composite, with
		// the clip coverage at the exact pixel center as paint.CPU's clip
		// mask holds it.
		lc := float32(1)
		if in.Clip.Z > 0 && in.Clip.W > 0 {
			lc = RRectCoverage(pc, in.Clip.XY(), in.Clip.ZW(), in.ClipRadii)
		}
		return LayerComposite(Layer.Sample(uv), in.Color.W, lc)
	}
	c := Color.Sample(uv)
	return c.Mul(in.Color).Scale(cov)
}

// RRectCoverage returns the coverage of the pixel centered at p by the
// rectangle at origin with the given size and corner radii (top-left,
// top-right, bottom-right, bottom-left). Radii are clamped to half the
// shorter side; an empty rectangle covers nothing.
//
// Away from rounded corners the coverage is the exact area overlap of the
// unit pixel square with the axis-aligned rectangle (covX*covY), which
// matches paint.CPU also for sub-pixel-thin rectangles and square corners.
// When p lies in a rounded corner's quadrant (within r of both edges that
// meet there, r > 0) it is Clamp(0.5 - sdf, 0, 1), sdf being the signed
// distance to the rounded boundary (negative inside), capped by the box
// overlap (Min) so it never exceeds the square-corner coverage.
func RRectCoverage(p, origin, size sl.Vec2, radii sl.Vec4) float32 {
	if size.X <= 0 || size.Y <= 0 {
		return 0
	}
	hi := origin.Add(size)
	covX := sl.Clamp(sl.Min(p.X+0.5, hi.X)-sl.Max(p.X-0.5, origin.X), 0, 1)
	covY := sl.Clamp(sl.Min(p.Y+0.5, hi.Y)-sl.Max(p.Y-0.5, origin.Y), 0, 1)
	box := covX * covY
	half := size.Scale(0.5)
	d := p.Sub(origin.Add(half))
	r := radii.W // bottom-left
	if d.X < 0 && d.Y < 0 {
		r = radii.X
	} else if d.Y < 0 {
		r = radii.Y
	} else if d.X >= 0 {
		r = radii.Z
	}
	r = sl.Clamp(r, 0, sl.Min(half.X, half.Y))
	q := sl.AbsV2(d).Sub(half).Add(sl.V2(r, r))
	if r <= 0 || q.X <= 0 || q.Y <= 0 {
		return box // not in a rounded corner's quadrant
	}
	sdf := sl.Length(sl.MaxV2(q, 0)) + sl.Min(sl.Max(q.X, q.Y), 0) - r
	return sl.Min(sl.Clamp(0.5-sdf, 0, 1), box)
}
