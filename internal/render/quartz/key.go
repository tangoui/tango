package quartz

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"reflect"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Canonical byte strings are compared by value, not only by a hash. Resource
// owners keep identity-based sources reachable until all current users retire.
type keyData []byte

func (k *keyData) tag(v byte)         { *k = append(*k, v) }
func (k *keyData) u64(v uint64)       { *k = binary.LittleEndian.AppendUint64(*k, v) }
func (k *keyData) i(v int)            { k.u64(uint64(v)) }
func (k *keyData) f(v float32)        { *k = binary.LittleEndian.AppendUint32(*k, math.Float32bits(v)) }
func (k *keyData) d(v float64)        { *k = binary.LittleEndian.AppendUint64(*k, math.Float64bits(v)) }
func (k *keyData) rgba(c color.NRGBA) { *k = append(*k, c.R, c.G, c.B, c.A) }
func (k *keyData) radii(r geom.Radii) { k.f(r.TL); k.f(r.TR); k.f(r.BR); k.f(r.BL) }
func (k *keyData) point(p geom.Point, s float32, at image.Point) {
	k.d(float64(p.X)*float64(s) - float64(at.X))
	k.d(float64(p.Y)*float64(s) - float64(at.Y))
}
func (k *keyData) rect(r geom.Rect, s float32, at image.Point) {
	k.point(geom.Pt(r.X, r.Y), s, at)
	k.f(r.W)
	k.f(r.H)
}
func (k *keyData) path(p *paint.Path, s float32, at image.Point) {
	if p == nil {
		k.i(-1)
		return
	}
	k.i(len(p.Segs))
	for _, seg := range p.Segs {
		k.tag(byte(seg.Op))
		n := 0
		switch seg.Op {
		case paint.SegMove, paint.SegLine:
			n = 1
		case paint.SegQuad:
			n = 2
		case paint.SegCubic:
			n = 3
		}
		for _, p := range seg.P[:n] {
			k.point(p, s, at)
		}
	}
}
func (k *keyData) identity(v any, serial, nonce uint64) {
	if v == nil {
		k.u64(0)
		return
	}
	typ := reflect.TypeOf(v)
	base := typ
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	t := base.PkgPath() + ":" + typ.String()
	k.i(len(t))
	*k = append(*k, t...)
	value := reflect.ValueOf(v)
	if value.Kind() == reflect.Pointer {
		k.tag(1)
		k.u64(uint64(value.Pointer()))
		return
	}
	// Unusual value-based sources are not safely reusable by address, even
	// with other values of the same type in one frame.
	k.tag(2)
	k.u64(serial)
	k.u64(nonce)
}
func (s *scene) patchKey(op paint.Op, frame, clip image.Rectangle) (string, []any, error) {
	k := keyData{}
	k.tag(byte(itemPatch))
	k.f(s.scale)
	k.i(frame.Dx())
	k.i(frame.Dy())
	// The patch rectangle is already intersected with the integer device clip.
	// All supported leaves compute coverage independently of pixels outside it;
	// including absolute canvas clip coordinates would defeat translated reuse.
	var refs []any
	at := frame.Min
	switch o := op.(type) {
	case paint.FillRect:
		k.tag(1)
		k.rect(o.Rect, s.scale, at)
		k.radii(o.Radii)
		k.rgba(o.Color)
	case paint.StrokeRect:
		k.tag(2)
		k.rect(o.Rect, s.scale, at)
		k.radii(o.Radii)
		k.rgba(o.Color)
		k.f(o.Width)
		k.i(len(o.Dash))
		for _, x := range o.Dash {
			k.f(x)
		}
	case paint.ShadowRect:
		k.tag(3)
		k.rect(o.Rect, s.scale, at)
		k.radii(o.Radii)
		k.f(o.Shadow.X)
		k.f(o.Shadow.Y)
		k.f(o.Shadow.Blur)
		k.f(o.Shadow.Spread)
		k.rgba(o.Shadow.Color)
	case paint.FillPath:
		k.tag(4)
		k.path(o.Path, s.scale, at)
		k.rgba(o.Color)
	case paint.LinearGradient:
		k.tag(5)
		k.tag(byte(frame.Min.X & 3))
		k.tag(byte(frame.Min.Y & 3))
		k.rect(o.Rect, s.scale, at)
		k.radii(o.Radii)
		k.point(o.P0, s.scale, at)
		k.point(o.P1, s.scale, at)
		k.rgba(o.From)
		k.rgba(o.To)
	case paint.Gradient:
		k.tag(6)
		k.tag(byte(frame.Min.X & 3))
		k.tag(byte(frame.Min.Y & 3))
		k.rect(o.Rect, s.scale, at)
		k.radii(o.Radii)
		k.path(o.Path, s.scale, at)
		k.tag(byte(o.Kind))
		k.point(o.Start, s.scale, at)
		k.point(o.End, s.scale, at)
		k.i(len(o.Stops))
		for _, x := range o.Stops {
			k.f(x.Offset)
			k.rgba(x.Color)
		}
	case paint.Checker:
		k.tag(7)
		k.rect(o.Rect, s.scale, at)
		k.radii(o.Radii)
		k.f(o.Cell)
		a, b := paint.CheckerColors(o)
		k.rgba(a)
		k.rgba(b)
	case paint.Image:
		k.tag(8)
		k.rect(o.Rect, s.scale, at)
		k.radii(o.Radii)
		s.identitySeq++
		k.identity(o.Img, s.serial, s.identitySeq)
		k.u64(s.imageEpoch)
		refs = append(refs, o.Img)
	case paint.Text:
		k.tag(9)
		k.rgba(o.Color)
		k.i(len(o.Glyphs))
		for _, g := range o.Glyphs {
			ix, iy, sub := paint.GlyphPlacement(o.Origin, g, s.scale)
			s.identitySeq++
			k.identity(g.Src, s.serial, s.identitySeq)
			k.u64(uint64(g.ID))
			k.f(g.Size)
			k.f(g.Skew)
			k.i(ix - at.X)
			k.i(iy - at.Y)
			k.tag(sub)
			refs = append(refs, g.Src)
		}
	default:
		return "", nil, fmt.Errorf("quartz: unsupported patch %T", op)
	}
	return string(k), refs, nil
}
