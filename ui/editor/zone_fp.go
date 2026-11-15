package editor

import (
	"encoding/binary"
	"hash/maphash"
	"image/color"
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// The fingerprint of a view zone's drawing commands, for the zones without
// a Version (zones.go, damage.go): a small version of the ui package's
// display list fingerprint, for the commands a zone draws with.

var zoneFPSeed = maphash.MakeSeed()

// opsFingerprint returns the fingerprint of ops with their geometry
// relative to org; ok is false when an op is not one it describes (fills,
// strokes, glyphs, paths, clips and their pops), and the fingerprint
// does not tell the drawing.
func opsFingerprint(ops []paint.Op, org geom.Point) (fp uint64, ok bool) {
	var h maphash.Hash
	h.SetSeed(zoneFPSeed)
	var b [8]byte
	num := func(v uint64) { h.Write(binary.LittleEndian.AppendUint64(b[:0], v)) }
	f32 := func(v float32) { num(uint64(math.Float32bits(v))) }
	pt := func(p geom.Point) { f32(p.X - org.X); f32(p.Y - org.Y) }
	rect := func(r geom.Rect) { pt(geom.Pt(r.X, r.Y)); f32(r.W); f32(r.H) }
	radii := func(r geom.Radii) { f32(r.TL); f32(r.TR); f32(r.BR); f32(r.BL) }
	col := func(c color.NRGBA) { num(uint64(c.R) | uint64(c.G)<<8 | uint64(c.B)<<16 | uint64(c.A)<<24) }
	ok = true
	for _, op := range ops {
		switch o := op.(type) {
		case paint.FillRect:
			num(1)
			rect(o.Rect)
			radii(o.Radii)
			col(o.Color)
		case paint.StrokeRect:
			num(2)
			rect(o.Rect)
			radii(o.Radii)
			f32(o.Width)
			col(o.Color)
			for _, d := range o.Dash {
				f32(d)
			}
		case paint.Text:
			num(3)
			pt(o.Origin)
			col(o.Color)
			for _, g := range o.Glyphs {
				num(uint64(g.ID))
				f32(g.Size)
				f32(g.X)
				f32(g.Y)
				f32(g.Skew)
			}
		case paint.FillPath:
			num(4)
			col(o.Color)
			if o.Path != nil {
				for _, sg := range o.Path.Segs {
					num(uint64(sg.Op))
					for _, p := range sg.P {
						pt(p)
					}
				}
			}
		case paint.PushClip:
			num(5)
			rect(o.Rect)
			radii(o.Radii)
		case paint.Pop:
			num(6)
		default:
			ok = false
			num(7)
		}
	}
	return h.Sum64(), ok
}
