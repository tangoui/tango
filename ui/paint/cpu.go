package paint

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/tangoui/tango/ui/geom"
	"golang.org/x/image/vector"
)

// CPU is the pure-Go reference renderer. It is not safe for concurrent use.
type CPU struct {
	img   *image.RGBA
	scale float32
	z     vector.Rasterizer

	stack []layer
	cur   *image.RGBA
	clip  image.Rectangle
	// cmask is the coverage of the active rounded clips (their product),
	// origin 0,0, placed at cmaskAt; nil when none is active. It covers
	// clip. Every drawing op multiplies its own coverage by it, exactly as
	// the GPU renderer multiplies clip coverage into each instance.
	cmask   *image.Alpha
	cmaskAt image.Point
	m       geom.Affine // current logical transform (PushTransform)
	ident   bool        // m is the identity: the fast paths apply

	imgs map[imgCacheKey]*imgCacheEntry // sources prepared for transformed images

	glyphs  map[glyphKey]*glyphMask
	shadows map[shadowKey]*image.Alpha
	gen     uint32

	// TextGamma < 1 thickens glyph coverage slightly (closer to native
	// rasterizers). 1 is linear coverage.
	TextGamma float64
	gammaLUT  [256]uint8
	lutFor    float64
}

type layer struct {
	target  *image.RGBA
	clip    image.Rectangle
	alpha   float32
	mask    *image.Alpha // group composite mask anchored at maskAt (enclosing rounded clips)
	maskAt  image.Point
	isGroup bool // composite target onto parent on pop
	// the rounded-clip coverage to restore on pop
	cmask   *image.Alpha
	cmaskAt image.Point
	// the transform to restore on pop
	m     geom.Affine
	ident bool
}

type glyphKey struct {
	src  GlyphSource
	id   uint32
	size float32
	subx uint8
	skew float32
}

type glyphMask struct {
	mask *image.Alpha
	off  image.Point // mask origin relative to the pen
	gen  uint32
}

type shadowKey struct {
	w, h, r0, r1, r2, r3, sigma int32
}

// NewCPU returns a CPU renderer.
func NewCPU() *CPU {
	return &CPU{glyphs: map[glyphKey]*glyphMask{}, shadows: map[shadowKey]*image.Alpha{}, imgs: map[imgCacheKey]*imgCacheEntry{}, TextGamma: 0.85}
}

// Image returns the last rendered frame (device pixels).
func (r *CPU) Image() *image.RGBA { return r.img }

// Render rasterizes l into an internal RGBA of ceil(size*scale) pixels.
func (r *CPU) Render(l *List, size geom.Size, scale float32) error {
	if scale <= 0 {
		scale = 1
	}
	w := int(math.Ceil(float64(size.W * scale)))
	h := int(math.Ceil(float64(size.H * scale)))
	if r.img == nil || r.img.Rect.Dx() != w || r.img.Rect.Dy() != h {
		r.img = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	r.prepare(scale)
	r.gen++
	draw.Draw(r.img, r.img.Rect, image.NewUniform(l.Background), image.Point{}, draw.Src)
	r.cur = r.img
	r.clip = r.img.Rect
	r.cmask = nil
	r.stack = r.stack[:0]
	r.m, r.ident = geom.Identity(), true
	for _, op := range l.Ops {
		r.exec(op)
	}
	for len(r.stack) > 0 {
		r.pop()
	}
	r.trimCaches()
	return nil
}

// trimCaches evicts glyphs unused for a while and resets an oversized shadow
// cache to bound memory.
func (r *CPU) trimCaches() {
	if len(r.glyphs) > 4096 {
		for k, g := range r.glyphs {
			if r.gen-g.gen > 30 {
				delete(r.glyphs, k)
			}
		}
	}
	if len(r.shadows) > 256 {
		r.shadows = map[shadowKey]*image.Alpha{}
	}
	for k, e := range r.imgs {
		if r.gen-e.gen > 30 {
			delete(r.imgs, k)
		}
	}
}

// prepare sets the device scale, dropping the scale-dependent caches when it
// changes, and builds the text gamma table for the current TextGamma.
func (r *CPU) prepare(scale float32) {
	if r.scale != scale {
		r.glyphs = map[glyphKey]*glyphMask{}
		r.shadows = map[shadowKey]*image.Alpha{}
	}
	r.scale = scale
	if r.lutFor != r.TextGamma {
		for i := range r.gammaLUT {
			r.gammaLUT[i] = uint8(math.Round(math.Pow(float64(i)/255, r.TextGamma) * 255))
		}
		r.lutFor = r.TextGamma
	}
}

// BeginFrame starts a frame for a caller that draws through RenderRegion
// and GlyphMask instead of Render (GPU renderers): it sets the device scale
// (dropping the scale-dependent caches when it changes) and counts one frame
// for evicting cached glyphs and images, as each Render call does. Call it
// once per frame, before the frame's RenderRegion and GlyphMask calls.
func (r *CPU) BeginFrame(scale float32) {
	if scale <= 0 {
		scale = 1
	}
	r.prepare(scale)
	r.gen++
	r.trimCaches()
}

// RenderRegion draws only ops onto a transparent image whose Bounds() is
// region (device pixels). Op coordinates stay absolute logical coordinates,
// so the result equals the region of a full Render of the same ops on a
// transparent background. Unbalanced Push ops are closed at the end and
// extra Pops are ignored, as in Render. It does not change Image().
//
// GPU renderers use it to rasterize the commands they do not draw natively
// (strokes, shadows, paths, gradients, images, opacity groups) as patches.
// It does not count as a frame for cache eviction: call BeginFrame once per
// frame, so the patches of one frame do not age each other's glyphs.
func (r *CPU) RenderRegion(ops []Op, region image.Rectangle, scale float32) *image.RGBA {
	if scale <= 0 {
		scale = 1
	}
	r.prepare(scale)
	dst := image.NewRGBA(region)
	saveCur, saveClip, saveStack, saveMask, saveMaskAt := r.cur, r.clip, r.stack, r.cmask, r.cmaskAt
	saveM, saveIdent := r.m, r.ident
	r.cur, r.clip, r.stack, r.cmask = dst, region, nil, nil
	r.m, r.ident = geom.Identity(), true
	for _, op := range ops {
		r.exec(op)
	}
	for len(r.stack) > 0 {
		r.pop()
	}
	r.cur, r.clip, r.stack, r.cmask, r.cmaskAt = saveCur, saveClip, saveStack, saveMask, saveMaskAt
	r.m, r.ident = saveM, saveIdent
	return dst
}

// GlyphMask returns the coverage mask of g rasterized at deviceSize pixels
// with sub-pixel x bucket sub (0..3, quarter pixels; see GlyphPlacement),
// with TextGamma applied (a GlyphRasterizer source supplies its own mask,
// gamma included), exactly as Render draws it. off is the mask's
// top-left relative to the device pen position. A nil mask means the glyph
// is empty (space) or too large. Masks are cached and shared: do not modify.
func (r *CPU) GlyphMask(g Glyph, deviceSize float32, sub uint8) (mask *image.Alpha, off image.Point) {
	if g.Src == nil {
		return nil, image.Point{}
	}
	if r.scale == 0 {
		r.prepare(1)
	} else {
		r.prepare(r.scale)
	}
	gm := r.glyph(g, deviceSize, sub)
	if gm == nil || gm.mask == nil {
		return nil, image.Point{}
	}
	return gm.mask, gm.off
}

// GlyphPlacement returns the device pen position of glyph g of a Text op at
// origin: x is floored to whole pixels with the remainder bucketed into
// quarter pixels (sub, 0..3), y is rounded. Render places glyphs with it.
func GlyphPlacement(origin geom.Point, g Glyph, scale float32) (ix, iy int, sub uint8) {
	px := float64((origin.X + g.X) * scale)
	py := math.Round(float64((origin.Y + g.Y) * scale))
	fx := math.Floor(px)
	return int(fx), int(py), uint8(math.Floor((px - fx) * 4))
}

func (r *CPU) exec(op Op) {
	if !r.ident {
		var ok bool
		if op, ok = r.execTransformed(op); ok {
			return
		}
	}
	switch o := op.(type) {
	case FillRect:
		if o.Color.A == 0 {
			return
		}
		if o.Radii.IsZero() {
			r.fillRect(o.Rect, o.Color)
			return
		}
		r.fillRRect(o.Rect, o.Radii, o.Color)
	case StrokeRect:
		r.strokeRect(o)
	case ShadowRect:
		r.shadow(o)
	case FillPath:
		r.fill(o.Path, o.Color)
	case LinearGradient:
		r.gradient(Gradient{Kind: GradientLinear, Rect: o.Rect, Radii: o.Radii, Start: o.P0, End: o.P1,
			Stops: []Stop{{0, o.From}, {1, o.To}}})
	case Gradient:
		r.gradient(o)
	case Checker:
		r.checker(o)
	case Text:
		r.text(o)
	case Image:
		r.image(o)
	case PushClip:
		// A rectangular clip only narrows clip. A rounded one also
		// multiplies its RRectCoverage into cmask; nested rounded clips
		// multiply their coverages. (Under a uniform scale+translate
		// execTransformed has already mapped Rect/Radii; other transforms
		// are clipped by pushPathClip.)
		r.pushState()
		r.clip = r.clip.Intersect(r.devRect(o.Rect))
		if !o.Radii.IsZero() {
			r.cmask, r.cmaskAt = r.rrectMask(o.Rect, o.Radii, r.clip)
		}
	case PushOpacity:
		// A real group: drawn unclipped by the rounded clips around it, then
		// composited through their coverage.
		r.pushGroup(r.clip, o.Alpha, r.cmask, r.cmaskAt)
	case PushTransform:
		r.pushState()
		r.m = r.m.Mul(o.M)
		r.ident = r.m.IsIdentity()
	case Pop:
		r.pop()
	}
}

// pushState saves the clip, rounded-clip coverage and transform for pop.
func (r *CPU) pushState() {
	r.stack = append(r.stack, layer{target: r.cur, clip: r.clip, cmask: r.cmask, cmaskAt: r.cmaskAt, m: r.m, ident: r.ident})
}

// pushPathClip pushes a clip to the path p (logical coordinates already
// mapped through the current transform; a rounded rect under a rotating /
// skewing / non-uniform transform): clip narrows to its
// bounds and its rasterized coverage is multiplied into cmask, exactly like
// a rounded clip.
func (r *CPU) pushPathClip(p *Path) {
	r.pushState()
	r.clip = r.clip.Intersect(r.devBounds(p))
	m, at := r.coverage(p, r.clip)
	if r.cmask != nil {
		for y := 0; y < m.Rect.Dy(); y++ {
			row := m.Pix[y*m.Stride:]
			for x := 0; x < m.Rect.Dx(); x++ {
				if v := uint32(row[x]); v != 0 {
					row[x] = uint8((v*r.clipCov(at.X+x, at.Y+y) + 127) / 255)
				}
			}
		}
	}
	r.cmask, r.cmaskAt = m, at
}

func (r *CPU) pushGroup(bounds image.Rectangle, alpha float32, mask *image.Alpha, at image.Point) {
	r.stack = append(r.stack, layer{target: r.cur, clip: r.clip, alpha: alpha, mask: mask, maskAt: at, isGroup: true,
		cmask: r.cmask, cmaskAt: r.cmaskAt, m: r.m, ident: r.ident})
	if bounds.Empty() {
		bounds = image.Rect(0, 0, 0, 0)
	}
	r.cur = image.NewRGBA(bounds)
	r.clip = bounds
	r.cmask = nil // mask is applied when the group is composited
}

func (r *CPU) pop() {
	if len(r.stack) == 0 {
		return
	}
	l := r.stack[len(r.stack)-1]
	r.stack = r.stack[:len(r.stack)-1]
	if l.isGroup {
		src := r.cur
		compositeLayer(l.target, src, l.mask, l.maskAt, l.alpha, l.clip)
	}
	r.cur = l.target
	r.clip = l.clip
	r.cmask, r.cmaskAt = l.cmask, l.cmaskAt
	r.m, r.ident = l.m, l.ident
}

// rrectMask returns the coverage (RRectCoverage, device pixels) of the
// logical rounded rect g over bounds, multiplied by the active cmask, as a
// mask with origin 0,0 placed at bounds.Min.
func (r *CPU) rrectMask(g geom.Rect, rad geom.Radii, bounds image.Rectangle) (*image.Alpha, image.Point) {
	if bounds.Empty() {
		return image.NewAlpha(image.Rectangle{}), bounds.Min
	}
	s := r.scale
	dr := geom.Rect{X: g.X * s, Y: g.Y * s, W: g.W * s, H: g.H * s}
	drad := geom.Radii{TL: rad.TL * s, TR: rad.TR * s, BR: rad.BR * s, BL: rad.BL * s}
	m := image.NewAlpha(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		row := m.Pix[(y-bounds.Min.Y)*m.Stride:]
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			cv := float64(RRectCoverage(float32(x)+0.5, float32(y)+0.5, dr, drad))
			if r.cmask != nil {
				cv *= float64(r.clipCov(x, y)) / 255
			}
			row[x-bounds.Min.X] = uint8(cv*255 + 0.5)
		}
	}
	return m, bounds.Min
}

// clipCov returns the rounded-clip coverage (0..255) at device pixel x, y.
func (r *CPU) clipCov(x, y int) uint32 {
	if r.cmask == nil {
		return 255
	}
	mx, my := x-r.cmaskAt.X, y-r.cmaskAt.Y
	if mx < 0 || my < 0 || mx >= r.cmask.Rect.Dx() || my >= r.cmask.Rect.Dy() {
		return 0
	}
	return uint32(r.cmask.Pix[my*r.cmask.Stride+mx])
}

// clipMask returns m (placed at at) restricted to dst and multiplied by the
// rounded-clip coverage, as a new mask with origin 0,0 placed at dst.Min.
// m is not modified (it may be a cached glyph or shadow mask).
func (r *CPU) clipMask(m *image.Alpha, at image.Point, dst image.Rectangle) *image.Alpha {
	out := image.NewAlpha(image.Rect(0, 0, dst.Dx(), dst.Dy()))
	for y := dst.Min.Y; y < dst.Max.Y; y++ {
		src := m.Pix[(y-at.Y)*m.Stride:]
		row := out.Pix[(y-dst.Min.Y)*out.Stride:]
		for x := dst.Min.X; x < dst.Max.X; x++ {
			v := uint32(src[x-at.X])
			if v != 0 {
				v = (v*r.clipCov(x, y) + 127) / 255
			}
			row[x-dst.Min.X] = uint8(v)
		}
	}
	return out
}

// compositeLayer blends premultiplied src over dst, modulated by mask and alpha.
func compositeLayer(dst, src *image.RGBA, mask *image.Alpha, at image.Point, alpha float32, clip image.Rectangle) {
	b := src.Rect.Intersect(clip).Intersect(dst.Rect)
	ia := uint32(alpha*255 + 0.5)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		si := src.PixOffset(b.Min.X, y)
		di := dst.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x, si, di = x+1, si+4, di+4 {
			m := ia
			if mask != nil {
				mx, my := x-at.X, y-at.Y
				if mx < 0 || my < 0 || mx >= mask.Rect.Dx() || my >= mask.Rect.Dy() {
					continue
				}
				m = m * uint32(mask.Pix[my*mask.Stride+mx]) / 255
			}
			if m == 0 {
				continue
			}
			sa := uint32(src.Pix[si+3]) * m / 255
			if sa == 0 {
				continue
			}
			inv := 255 - sa
			for c := 0; c < 3; c++ {
				sc := uint32(src.Pix[si+c]) * m / 255
				dst.Pix[di+c] = uint8((uint32(dst.Pix[di+c])*inv)/255 + sc)
			}
			dst.Pix[di+3] = uint8((uint32(dst.Pix[di+3])*inv)/255 + sa)
		}
	}
}

func (r *CPU) devRect(g geom.Rect) image.Rectangle {
	s := r.scale
	return image.Rect(int(math.Round(float64(g.X*s))), int(math.Round(float64(g.Y*s))),
		int(math.Round(float64(g.Right()*s))), int(math.Round(float64(g.Bottom()*s))))
}

func (r *CPU) devBounds(p *Path) image.Rectangle {
	b := p.Bounds()
	s := r.scale
	return image.Rect(int(math.Floor(float64(b.X*s)))-1, int(math.Floor(float64(b.Y*s)))-1,
		int(math.Ceil(float64(b.Right()*s)))+1, int(math.Ceil(float64(b.Bottom()*s)))+1)
}

// feed adds p (logical coords) to the rasterizer, scaled and offset by -o.
func (r *CPU) feed(p *Path, scale float32, o geom.Point) {
	z := &r.z
	open := false
	var first geom.Point
	tx := func(pt geom.Point) (float32, float32) { return pt.X*scale - o.X, pt.Y*scale - o.Y }
	for _, s := range p.Segs {
		switch s.Op {
		case SegMove:
			if open {
				z.ClosePath()
			}
			x, y := tx(s.P[0])
			z.MoveTo(x, y)
			first = s.P[0]
			open = true
		case SegLine:
			x, y := tx(s.P[0])
			z.LineTo(x, y)
		case SegQuad:
			x0, y0 := tx(s.P[0])
			x1, y1 := tx(s.P[1])
			z.QuadTo(x0, y0, x1, y1)
		case SegCubic:
			x0, y0 := tx(s.P[0])
			x1, y1 := tx(s.P[1])
			x2, y2 := tx(s.P[2])
			z.CubeTo(x0, y0, x1, y1, x2, y2)
		case SegClose:
			if open {
				z.ClosePath()
				x, y := tx(first)
				z.MoveTo(x, y)
				open = false
			}
		}
	}
	if open {
		z.ClosePath()
	}
}

func (r *CPU) fill(p *Path, c color.NRGBA) {
	if c.A == 0 {
		return
	}
	b := r.devBounds(p).Intersect(r.clip)
	if b.Empty() {
		return
	}
	if r.cmask != nil {
		m, at := r.coverage(p, b)
		dst := image.Rectangle{Min: at, Max: at.Add(m.Rect.Size())}
		draw.DrawMask(r.cur, dst, image.NewUniform(c), image.Point{}, r.clipMask(m, at, dst), image.Point{}, draw.Over)
		return
	}
	r.z.Reset(b.Dx(), b.Dy())
	r.feed(p, r.scale, geom.Pt(float32(b.Min.X), float32(b.Min.Y)))
	r.z.DrawOp = draw.Over
	r.z.Draw(r.cur, b, image.NewUniform(c), image.Point{})
}

// coverage rasterizes p into an alpha mask clipped to bounds; returns the
// mask (origin 0,0) and its device position.
func (r *CPU) coverage(p *Path, bounds image.Rectangle) (*image.Alpha, image.Point) {
	b := r.devBounds(p).Intersect(bounds)
	if b.Empty() {
		return image.NewAlpha(image.Rect(0, 0, 0, 0)), b.Min
	}
	m := image.NewAlpha(image.Rect(0, 0, b.Dx(), b.Dy()))
	r.z.Reset(b.Dx(), b.Dy())
	r.feed(p, r.scale, geom.Pt(float32(b.Min.X), float32(b.Min.Y)))
	r.z.DrawOp = draw.Src
	r.z.Draw(m, m.Rect, image.Opaque, image.Point{})
	return m, b.Min
}

// strokeRect draws a StrokeRect with the shared StrokeCoverage definition
// (the GPU shader's) in device pixels, times the rounded-clip coverage.
// Pixels of the inner area, whose coverage is exactly 0, are skipped.
func (r *CPU) strokeRect(o StrokeRect) {
	s := r.scale
	w := o.Width * s
	dr := geom.Rect{X: o.Rect.X * s, Y: o.Rect.Y * s, W: o.Rect.W * s, H: o.Rect.H * s}
	if o.Color.A == 0 || !(w > 0) || dr.W <= 0 || dr.H <= 0 {
		return
	}
	drad := geom.Radii{TL: o.Radii.TL * s, TR: o.Radii.TR * s, BR: o.Radii.BR * s, BL: o.Radii.BL * s}
	var d [4]float32 // the GPU carries 4 dash entries; longer patterns are truncated
	for i := 0; i < len(o.Dash) && i < len(d); i++ {
		d[i] = o.Dash[i] * s
	}
	x0, y0 := float64(dr.X), float64(dr.Y)
	x1, y1 := float64(dr.X+dr.W), float64(dr.Y+dr.H)
	b := r.clip.Intersect(r.cur.Rect).Intersect(image.Rect(
		int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1))))
	if b.Empty() {
		return
	}
	// The hole: pixels at least m+1 inside every edge, m covering the
	// outer corners and the inner rectangle's (inset by w) corners, are
	// fully inside both rounded rectangles (ring coverage 1-1 = 0).
	hx0, hx1, hy0, hy1 := 0, 0, 0, 0
	if iw, ih := dr.W-2*w, dr.H-2*w; iw > 0 && ih > 0 {
		co := drad.Clamp(dr.W, dr.H)
		ci := drad.Grow(-w).Clamp(iw, ih)
		m := float64(max(co.TL, co.TR, co.BR, co.BL, w+max(ci.TL, ci.TR, ci.BR, ci.BL)))
		hx0, hx1 = int(math.Ceil(x0+m))+1, int(math.Floor(x1-m))-1
		hy0, hy1 = int(math.Ceil(y0+m))+1, int(math.Floor(y1-m))-1
	}
	dst := r.cur
	for y := b.Min.Y; y < b.Max.Y; y++ {
		py := float32(y) + 0.5
		hole := y >= hy0 && y < hy1 && hx0 < hx1
		for x := b.Min.X; x < b.Max.X; x++ {
			if hole && x >= hx0 && x < hx1 {
				x = hx1
				if x >= b.Max.X {
					break
				}
			}
			cv := float64(strokeCoverage(float32(x)+0.5, py, dr, drad, w, d, 0))
			if cv <= 0 {
				continue
			}
			if r.cmask != nil {
				cv *= float64(r.clipCov(x, y)) / 255
			}
			blendCov(dst, x, y, o.Color, cv)
		}
	}
}

// strokeRectPath is the fillable outline of a StrokeRect. paint.CPU
// draws a StrokeRect under a non-uniform transform with it; otherwise
// strokeRect's StrokeCoverage replaces it.
func strokeRectPath(o StrokeRect) *Path {
	if len(o.Dash) > 0 {
		hw := o.Width / 2
		center := NewPath().RRect(o.Rect.Expand(-hw), o.Radii.Grow(-hw))
		return StrokeToFill(center, StrokeStyle{Width: o.Width, Dash: o.Dash})
	}
	p := NewPath().RRect(o.Rect, o.Radii)
	inner := o.Rect.Expand(-o.Width)
	if !inner.Empty() {
		p.Append(NewPath().RRect(inner, o.Radii.Grow(-o.Width)).Reversed())
	}
	return p
}

func (r *CPU) shadow(o ShadowRect) {
	g, blurred := ShadowPlacement(o, r.scale)
	if !blurred {
		// No blur: the rounded rectangle itself, drawn as FillRect draws it
		// (the GPU renderer's KindSolid).
		if g.ShapeRadii.IsZero() {
			r.fillRect(g.Shape, o.Shadow.Color)
		} else {
			r.fillRRect(g.Shape, g.ShapeRadii, o.Shadow.Color)
		}
		return
	}
	if g.Rect.Empty() {
		return
	}
	m := r.shadowMask(g)
	at := g.Rect.Min.Sub(image.Pt(g.Margin, g.Margin))
	r.blendMask(m, at, o.Shadow.Color)
}

// shadowMask returns the cached coverage of the blurred shadow g: a mask of
// g.Rect's size plus g.Margin pixels on every side, ShadowCoverage of
// g.Rect (placed at the margin) at each pixel center, rounded to 8 bits.
// The cache key is the device size, radii and sigma, all of which g holds
// already quantized, so the mask depends only on its key.
func (r *CPU) shadowMask(g ShadowGeom) *image.Alpha {
	w, h := g.Rect.Dx(), g.Rect.Dy()
	key := shadowKey{int32(w), int32(h), int32(g.Radii.TL), int32(g.Radii.TR), int32(g.Radii.BR), int32(g.Radii.BL), int32(g.Sigma * 16)}
	m, ok := r.shadows[key]
	if !ok {
		m = shadowMaskImage(w, h, g.Radii, g.Sigma, g.Margin)
		r.shadows[key] = m
	}
	return m
}

func lerpNRGBA(a, b color.NRGBA, t float32) color.NRGBA {
	t = max(0, min(1, t))
	l := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: l(a.A, b.A)}
}

func blendPixel(dst *image.RGBA, x, y int, c color.NRGBA, cov uint32) {
	if !(image.Point{x, y}.In(dst.Rect)) {
		return
	}
	i := dst.PixOffset(x, y)
	a := uint32(c.A) * cov / 255
	inv := 255 - a
	dst.Pix[i+0] = uint8((uint32(dst.Pix[i+0])*inv + uint32(c.R)*a) / 255)
	dst.Pix[i+1] = uint8((uint32(dst.Pix[i+1])*inv + uint32(c.G)*a) / 255)
	dst.Pix[i+2] = uint8((uint32(dst.Pix[i+2])*inv + uint32(c.B)*a) / 255)
	dst.Pix[i+3] = uint8((uint32(dst.Pix[i+3])*inv)/255 + a)
}

func (r *CPU) text(o Text) {
	// The identity and uniform scale+translate transforms keep glyphs
	// upright, so the cached masks apply at size·k. With no transform
	// this is exactly GlyphPlacement (k = scale, e = 0).
	k, ex, ey := r.scale, float32(0), float32(0)
	if !r.ident {
		k, ex, ey = r.scale*r.m.A, r.scale*r.m.E, r.scale*r.m.F
	}
	src := image.NewUniform(o.Color)
	for _, g := range o.Glyphs {
		if g.Src == nil {
			continue
		}
		var ix, iy int
		var sub uint8
		if r.ident {
			ix, iy, sub = GlyphPlacement(o.Origin, g, k)
		} else {
			px := float64((o.Origin.X+g.X)*k + ex)
			py := math.Round(float64((o.Origin.Y+g.Y)*k + ey))
			fx := math.Floor(px)
			ix, iy, sub = int(fx), int(py), uint8(math.Floor((px-fx)*4))
		}
		gm := r.glyph(g, g.Size*k, sub)
		if gm == nil || gm.mask == nil {
			continue
		}
		at := image.Pt(ix+gm.off.X, iy+gm.off.Y)
		dst := image.Rectangle{Min: at, Max: at.Add(gm.mask.Rect.Size())}.Intersect(r.clip)
		if dst.Empty() {
			continue
		}
		if r.cmask != nil {
			draw.DrawMask(r.cur, dst, src, image.Point{}, r.clipMask(gm.mask, at, dst), image.Point{}, draw.Over)
			continue
		}
		draw.DrawMask(r.cur, dst, src, image.Point{}, gm.mask, dst.Min.Sub(at), draw.Over)
	}
}

func (r *CPU) glyph(g Glyph, size float32, sub uint8) *glyphMask {
	k := glyphKey{g.Src, g.ID, size, sub, g.Skew}
	if gm, ok := r.glyphs[k]; ok {
		gm.gen = r.gen
		return gm
	}
	gm := &glyphMask{gen: r.gen}
	r.glyphs[k] = gm
	// A platform text engine rasterizes its own glyphs (gamma included;
	// see GlyphRasterizer), so Render and GlyphMask (the GPU atlas) agree.
	if m, off, ok := rasterGlyph(g, size, sub); ok {
		gm.mask, gm.off = m, off
		return gm
	}
	p := g.Src.GlyphPath(g.ID, size)
	if p == nil || len(p.Segs) == 0 {
		return gm
	}
	if g.Skew != 0 {
		p = p.Skewed(g.Skew)
	}
	b := p.Bounds()
	dx := float32(sub) / 4
	x0 := int(math.Floor(float64(b.X+dx))) - 1
	y0 := int(math.Floor(float64(b.Y))) - 1
	x1 := int(math.Ceil(float64(b.Right()+dx))) + 1
	y1 := int(math.Ceil(float64(b.Bottom()))) + 1
	w, h := x1-x0, y1-y0
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return gm
	}
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	r.z.Reset(w, h)
	r.feed(p, 1, geom.Pt(float32(x0)-dx, float32(y0)))
	r.z.DrawOp = draw.Src
	r.z.Draw(m, m.Rect, image.Opaque, image.Point{})
	if r.TextGamma != 1 {
		for i, v := range m.Pix {
			m.Pix[i] = r.gammaLUT[v]
		}
	}
	gm.mask = m
	gm.off = image.Pt(x0, y0)
	return gm
}

// image draws o like the GPU renderer's KindImage instance: the image's
// prepared pixels (PrepareImage: premultiplied, reduced first when much
// larger than drawn) are sampled with SampleCatmullRomF at every device
// pixel center mapped linearly from the device rectangle onto the whole
// image, times RRectCoverage of the device rounded rectangle and the
// rounded-clip coverage (shadeRect). A nil or empty image, or a
// non-finite or empty rectangle, draws nothing.
func (r *CPU) image(o Image) {
	s := r.scale
	dr := geom.Rect{X: o.Rect.X * s, Y: o.Rect.Y * s, W: o.Rect.W * s, H: o.Rect.H * s}
	if !ImageDrawable(o) || !finiteRect(dr) {
		return
	}
	w, h := ImageReduction(o.Img.Bounds(), dr.W, dr.H)
	src := r.preparedImage(o.Img, w, h)
	r.shadeRect(o.Rect, o.Radii, func(p geom.Point, _, _ int) [4]float32 {
		return SampleCatmullRomF(src, (p.X-dr.X)/dr.W, (p.Y-dr.Y)/dr.H)
	})
}

// ImageDrawable reports whether o draws anything: a non-nil image with
// pixels in a rectangle of positive, finite size at a finite position.
func ImageDrawable(o Image) bool {
	return o.Img != nil && !o.Img.Bounds().Empty() && o.Rect.W > 0 && o.Rect.H > 0 && finiteRect(o.Rect)
}

func finiteRect(g geom.Rect) bool {
	for _, v := range [4]float32{g.X, g.Y, g.W, g.H} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return true
}

// InvalidateImage drops the prepared pixels cached for img (by its
// image.Image value), so the next frame drawing it reads its current
// pixels: call it after changing an image's pixels in place.
func (r *CPU) InvalidateImage(img image.Image) {
	for k := range r.imgs {
		if k.src == img {
			delete(r.imgs, k)
		}
	}
}
