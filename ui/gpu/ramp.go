package gpu

import (
	"hash/maphash"
	"image"
	"math"
	"slices"

	"github.com/tangoui/tango/ui/paint"
)

// rampRows is the number of rows of the ramp texture (TexRamp): how many
// distinct gradient stop sets the GPU holds at once. The texture is
// paint.RampWidth × rampRows RGBA16Unorm (512 KiB).
const rampRows = 256

// rampStride is the bytes of one ramp texture row: paint.RampWidth texels
// of 4 little-endian uint16 channels.
const rampStride = paint.RampWidth * 8

// rampCache assigns gradient stop sets to rows of the ramp texture and
// mirrors the texture's pixels. A row is keyed by a hash of the stops'
// content (offsets and colors in order) and verified against a copy of
// them, so equal stops in any op, frame or slice share a row. When every
// row is taken, a new stop set evicts the least recently used row that
// the current frame has not used; if the current frame uses them all, the
// gradient gets no row (the Renderer draws it as a patch).
//
// Rows are only (re)written for stop sets no instance of the current frame
// samples yet, and their uploads go into Frame.Uploads (before any batch):
// a row the frame's earlier batches sample is never overwritten.
type rampCache struct {
	seed  maphash.Seed
	pix   []byte // the texture: rampRows rows of rampStride bytes
	byKey map[uint64]int
	rows  []rampRow
	frame uint64 // current frame number (starts at 1)
	dirty []int  // rows written this frame (each once), uploaded at the end
	sent  bool   // the device holds pix (false: upload all of it)
}

type rampRow struct {
	key   uint64
	stops []paint.Stop
	used  uint64 // last frame that sampled the row
}

func newRampCache() *rampCache {
	return &rampCache{
		seed:  maphash.MakeSeed(),
		pix:   make([]byte, rampRows*rampStride),
		byKey: map[uint64]int{},
	}
}

// beginFrame starts a frame.
func (c *rampCache) beginFrame() {
	c.frame++
	c.dirty = c.dirty[:0]
}

// row returns the ramp row holding stops, assigning (and writing) one when
// needed, or false when every row is in use by this frame.
func (c *rampCache) row(stops []paint.Stop) (int, bool) {
	k := c.hash(stops)
	if i, ok := c.byKey[k]; ok {
		e := &c.rows[i]
		if sameStops(e.stops, stops) {
			e.used = c.frame
			return i, true
		}
		// A hash collision: the row is reassigned below if it is free.
		if e.used == c.frame {
			return 0, false
		}
		delete(c.byKey, k)
		c.write(i, k, stops)
		return i, true
	}
	i := len(c.rows)
	if i < rampRows {
		c.rows = append(c.rows, rampRow{})
	} else {
		i = -1
		for j := range c.rows {
			if u := c.rows[j].used; u != c.frame && (i < 0 || u < c.rows[i].used) {
				i = j
			}
		}
		if i < 0 {
			return 0, false
		}
		delete(c.byKey, c.rows[i].key)
	}
	c.write(i, k, stops)
	return i, true
}

// sameStops reports whether a and b are the same stops, comparing offsets
// by bit pattern (a NaN offset equals itself, so stops holding one still
// find their row).
func sameStops(a, b []paint.Stop) bool {
	return slices.EqualFunc(a, b, func(x, y paint.Stop) bool {
		return math.Float32bits(x.Offset) == math.Float32bits(y.Offset) && x.Color == y.Color
	})
}

// write puts the ramp of stops into row i.
func (c *rampCache) write(i int, k uint64, stops []paint.Stop) {
	ramp := paint.GradientRamp(stops)
	px := paint.RampPremul(&ramp)
	dst := c.pix[i*rampStride:][:rampStride]
	for k, v := range px {
		dst[2*k], dst[2*k+1] = byte(v), byte(v>>8)
	}
	c.rows[i] = rampRow{key: k, stops: append(c.rows[i].stops[:0], stops...), used: c.frame}
	c.byKey[k] = i
	c.dirty = append(c.dirty, i) // a row written this frame is in use, so it is not written again
}

func (c *rampCache) hash(stops []paint.Stop) uint64 {
	var h maphash.Hash
	h.SetSeed(c.seed)
	var b [8]byte
	for _, s := range stops {
		o := math.Float32bits(s.Offset)
		b = [8]byte{byte(o), byte(o >> 8), byte(o >> 16), byte(o >> 24), s.Color.R, s.Color.G, s.Color.B, s.Color.A}
		h.Write(b[:])
	}
	return h.Sum64()
}

// uploads appends this frame's ramp uploads to ups: the whole texture
// when the device may not hold it, else one upload per run of consecutive
// rows written this frame.
func (c *rampCache) uploads(ups []Upload) []Upload {
	const stride = rampStride
	if !c.sent {
		return append(ups, Upload{Tex: TexRamp, Rect: image.Rect(0, 0, paint.RampWidth, rampRows), Pix: c.pix, Stride: stride})
	}
	if len(c.dirty) == 0 {
		return ups
	}
	rows := c.dirty
	slices.Sort(rows)
	for k := 0; k < len(rows); {
		j := k + 1
		for j < len(rows) && rows[j] == rows[j-1]+1 {
			j++
		}
		y0, y1 := rows[k], rows[j-1]+1
		ups = append(ups, Upload{Tex: TexRamp, Rect: image.Rect(0, y0, paint.RampWidth, y1), Pix: c.pix[y0*stride:], Stride: stride})
		k = j
	}
	return ups
}

// size is the ramp texture's size.
func (c *rampCache) size() image.Point { return image.Pt(paint.RampWidth, rampRows) }
