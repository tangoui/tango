package paint

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"reflect"

	"github.com/tangoui/tango/ui/geom"
)

// skewAffine is the synthetic-oblique shear (y grows downwards, so points
// above the baseline have negative y and move right).
func skewAffine(k float32) geom.Affine { return geom.Affine{A: 1, C: -k, D: 1} }

// device returns the logical→device-pixel transform.
func (r *CPU) device() geom.Affine { return geom.Scale(r.scale, r.scale).Mul(r.m) }

// execTransformed handles op under a non-identity transform. Ops that only
// need their coordinates mapped under a uniform scale+translate are
// rewritten and returned with ok=false so the regular fast paths draw them;
// everything else is drawn here (ok=true).
func (r *CPU) execTransformed(op Op) (Op, bool) {
	m := r.m
	uni := m.AxisAligned() && m.A == m.D
	k := m.A
	mr := func(g geom.Rect) geom.Rect { return m.Bounds(g) }
	mrad := func(rd geom.Radii) geom.Radii {
		return geom.Radii{TL: rd.TL * k, TR: rd.TR * k, BR: rd.BR * k, BL: rd.BL * k}
	}
	switch o := op.(type) {
	case FillRect:
		if uni {
			o.Rect, o.Radii = mr(o.Rect), mrad(o.Radii)
			return o, false
		}
		r.fill(NewPath().RRect(o.Rect, o.Radii).Transform(m), o.Color)
		return op, true
	case StrokeRect:
		if uni {
			o.Rect, o.Radii, o.Width = mr(o.Rect), mrad(o.Radii), o.Width*k
			if len(o.Dash) > 0 {
				d := make([]float32, len(o.Dash))
				for i, v := range o.Dash {
					d[i] = v * k
				}
				o.Dash = d
			}
			return o, false
		}
		r.fill(strokeRectPath(o).Transform(m), o.Color)
		return op, true
	case ShadowRect:
		if uni {
			o.Rect, o.Radii = mr(o.Rect), mrad(o.Radii)
			o.Shadow.X, o.Shadow.Y = o.Shadow.X*k, o.Shadow.Y*k
			o.Shadow.Blur, o.Shadow.Spread = o.Shadow.Blur*k, o.Shadow.Spread*k
			return o, false
		}
		r.shadowTransformed(o)
		return op, true
	case FillPath:
		r.fill(o.Path.Transform(m), o.Color)
		return op, true
	case Text:
		if uni {
			return op, false // text() applies the uniform transform itself
		}
		r.textTransformed(o)
		return op, true
	case Image:
		if uni {
			o.Rect, o.Radii = mr(o.Rect), mrad(o.Radii)
			return o, false
		}
		r.imageTransformed(o)
		return op, true
	case PushClip:
		if uni {
			o.Rect, o.Radii = mr(o.Rect), mrad(o.Radii)
			return o, false
		}
		r.pushPathClip(NewPath().RRect(o.Rect, o.Radii).Transform(m))
		return op, true
	}
	// LinearGradient, Gradient and Checker sample through the inverse
	// transform; group ops and Pop are transform independent.
	return op, false
}

// textTransformed draws glyph outlines through a rotating / skewing /
// non-uniform transform. Masks are not cached (the transform rarely
// repeats exactly); the text gamma still applies.
func (r *CPU) textTransformed(o Text) {
	src := image.NewUniform(o.Color)
	for _, g := range o.Glyphs {
		if g.Src == nil {
			continue
		}
		p := g.Src.GlyphPath(g.ID, g.Size)
		if p == nil || len(p.Segs) == 0 {
			continue
		}
		t := geom.Translate(o.Origin.X+g.X, o.Origin.Y+g.Y)
		if g.Skew != 0 {
			t = t.Mul(skewAffine(g.Skew))
		}
		p = p.Transform(r.m.Mul(t))
		mask, at := r.coverage(p, r.clip)
		if mask.Rect.Empty() {
			continue
		}
		if r.TextGamma != 1 {
			for i, v := range mask.Pix {
				mask.Pix[i] = r.gammaLUT[v]
			}
		}
		dst := image.Rectangle{Min: at, Max: at.Add(mask.Rect.Size())}
		if r.cmask != nil {
			mask = r.clipMask(mask, at, dst)
		}
		draw.DrawMask(r.cur, dst, src, image.Point{}, mask, image.Point{}, draw.Over)
	}
}

// shadowTransformed renders the shadow's mask (ShadowCoverage, cached as
// in shadow) in the shadow's own space at the transform's largest scale and
// resamples it onto the device.
func (r *CPU) shadowTransformed(o ShadowRect) {
	sh := o.Shadow
	rect, rad := shadowShape(o)
	if !(sh.Blur > 0) {
		r.fill(NewPath().RRect(rect, rad).Transform(r.m), sh.Color)
		return
	}
	k := r.scale * r.m.MaxScale()
	if k <= 0 {
		return
	}
	w, h := int(math.Round(float64(rect.W*k))), int(math.Round(float64(rect.H*k)))
	if w <= 0 || h <= 0 {
		return
	}
	g := shadowGeom(image.Rect(0, 0, w, h), rad.Clamp(rect.W, rect.H), k, sh.Blur*k/2)
	m := r.shadowMask(g)
	// mask pixel (u,v) ↔ logical point rect.Min + ((u,v) - margin) / k
	fm := float32(g.Margin)
	toLocal := geom.Translate(rect.X, rect.Y).Mul(geom.Scale(rect.W/float32(w), rect.H/float32(h))).Mul(geom.Translate(-fm, -fm))
	r.drawMaskAffine(m, r.device().Mul(toLocal), sh.Color)
}

// drawMaskAffine blends color c through the alpha mask m placed on the
// device by toDev (mask pixel space → device pixels), sampling bilinearly.
func (r *CPU) drawMaskAffine(m *image.Alpha, toDev geom.Affine, c color.NRGBA) {
	inv, ok := toDev.Invert()
	if !ok {
		return
	}
	mw, mh := m.Rect.Dx(), m.Rect.Dy()
	b := toDev.Bounds(geom.Rect{W: float32(mw), H: float32(mh)})
	db := image.Rect(int(math.Floor(float64(b.X))), int(math.Floor(float64(b.Y))),
		int(math.Ceil(float64(b.Right()))), int(math.Ceil(float64(b.Bottom())))).Intersect(r.clip).Intersect(r.cur.Rect)
	for y := db.Min.Y; y < db.Max.Y; y++ {
		for x := db.Min.X; x < db.Max.X; x++ {
			p := inv.Apply(geom.Pt(float32(x)+0.5, float32(y)+0.5))
			a := sampleAlpha(m, p.X-0.5, p.Y-0.5)
			if a != 0 && r.cmask != nil {
				a = (a*r.clipCov(x, y) + 127) / 255
			}
			if a == 0 {
				continue
			}
			blendPixel(r.cur, x, y, c, a)
		}
	}
}

// sampleAlpha is a bilinear lookup with transparent outside.
func sampleAlpha(m *image.Alpha, fx, fy float32) uint32 {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	if fx <= -1 || fy <= -1 || fx >= float32(w) || fy >= float32(h) {
		return 0
	}
	x0, y0 := int(math.Floor(float64(fx))), int(math.Floor(float64(fy)))
	tx, ty := fx-float32(x0), fy-float32(y0)
	at := func(x, y int) float32 {
		if x < 0 || y < 0 || x >= w || y >= h {
			return 0
		}
		return float32(m.Pix[y*m.Stride+x])
	}
	top := at(x0, y0)*(1-tx) + at(x0+1, y0)*tx
	bot := at(x0, y0+1)*(1-tx) + at(x0+1, y0+1)*tx
	return uint32(top*(1-ty) + bot*ty + 0.5)
}

// imageTransformed draws an image through a rotating / skewing /
// non-uniform transform: each device pixel the transformed rounded rect
// covers is mapped back into the image and sampled with SampleCatmullRomF
// (the untransformed path's filter), blended with the path coverage.
func (r *CPU) imageTransformed(o Image) {
	dev := r.device()
	inv, ok := dev.Invert()
	if !ok || !ImageDrawable(o) {
		return
	}
	// Prefilter large reductions as the untransformed path does.
	k := r.scale * r.m.MaxScale()
	w, h := ImageReduction(o.Img.Bounds(), o.Rect.W*k, o.Rect.H*k)
	src := r.preparedImage(o.Img, w, h)
	mask, at := r.coverage(NewPath().RRect(o.Rect, o.Radii).Transform(r.m), r.clip)
	dst := r.cur
	for y := 0; y < mask.Rect.Dy(); y++ {
		for x := 0; x < mask.Rect.Dx(); x++ {
			cov := uint32(mask.Pix[y*mask.Stride+x])
			dx, dy := at.X+x, at.Y+y
			if cov != 0 && r.cmask != nil {
				cov = (cov*r.clipCov(dx, dy) + 127) / 255
			}
			if cov == 0 {
				continue
			}
			p := inv.Apply(geom.Pt(float32(dx)+0.5, float32(dy)+0.5))
			c := SampleCatmullRomF(src, (p.X-o.Rect.X)/o.Rect.W, (p.Y-o.Rect.Y)/o.Rect.H)
			blendPremulF(dst, dx, dy, c, float32(cov)/255)
		}
	}
}

type imgCacheKey struct {
	src  image.Image
	w, h int
}

type imgCacheEntry struct {
	img *image.RGBA
	gen uint32
}

// preparedImage returns PrepareImage(src, w, h), cached per source and
// size (w, h from ImageReduction; 0, 0 for the full resolution). Only
// pointer image types (every standard one) are cached: a value type can
// pass reflect's Comparable and still panic when hashed (an interface
// field holding a slice), so value images are prepared every time.
func (r *CPU) preparedImage(src image.Image, w, h int) *image.RGBA {
	if reflect.TypeOf(src).Kind() != reflect.Pointer {
		return PrepareImage(src, w, h)
	}
	key := imgCacheKey{src, w, h}
	if r.imgs == nil {
		r.imgs = map[imgCacheKey]*imgCacheEntry{}
	}
	if e, ok := r.imgs[key]; ok {
		e.gen = r.gen
		return e.img
	}
	img := PrepareImage(src, w, h)
	r.imgs[key] = &imgCacheEntry{img, r.gen}
	return img
}
