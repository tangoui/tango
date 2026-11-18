package gpu

import (
	"image"
	"image/color"
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Big paths. A FillPath whose mask is larger than an atlas page is drawn
// as a patch (pathMask returns pathPatch): rasterized with paint.CPU over
// its region and uploaded to the patch page. A window-sized path (a
// spotlight mask, a large shape) costs milliseconds of CPU rasterization,
// and an animating window pays it every frame. bigPathCache keeps the
// rasterized patch of such paths, keyed by everything RenderRegion's
// result depends on: the path's segments (compared exactly; widgets
// usually rebuild the Path every frame, so not its pointer), its color,
// the native transform, the raster region and the scale. A hit draws the
// same pixels (the same image) without rasterizing; the upload still
// happens, the patch page being per frame.
//
// An entry is admitted only when its key was also drawn (and missed) on the
// previous frame: a static path is rasterized on two frames and hits from
// the third, while a path that changes every frame (an animating mask, a
// live resize) stores no image at all and only leaves its key among the
// last frame's sightings.
//
// Only the last frame's working set stays: at the end of every frame
// (endFrame) the entries that frame did not draw are dropped, so a window
// that stops drawing a big path (or goes idle after a frame without one)
// does not keep megabytes of rasters; the next frame that draws it again
// rasterizes it as on a first sighting. The whole cache is also dropped on
// Trim, a scale change and device loss.

// Bounds of bigPathCache.
const (
	bigPathMaxEntries = 8
	bigPathMaxBytes   = 48 << 20 // a 4K window's RGBA mask fits
)

// bigPathKey is what a big path patch's pixels depend on besides the
// segments (hash is theirs; entries compare them exactly).
type bigPathKey struct {
	hash   uint64
	color  color.NRGBA
	m      geom.Affine // the native transform (xf), else zero
	xf     bool
	raster image.Rectangle // device region rasterized
	scale  float32
}

type bigPathEntry struct {
	segs []paint.Seg // a copy
	img  *image.RGBA // RenderRegion's result; never modified
	ink  image.Rectangle
	used uint64 // frame of the last use
}

// bigPathCache is the Renderer's cache of big path patches (see above).
// Main thread, like the Renderer.
type bigPathCache struct {
	m     map[bigPathKey]*bigPathEntry
	bytes int
	frame uint64
	// Keys that missed on the previous frame (prev) and on this one (cur),
	// at most bigPathMaxEntries each: the admission rule (see above).
	prev, cur map[bigPathKey]struct{}
}

// beginFrame advances the frame clock and rotates the sightings.
func (c *bigPathCache) beginFrame() {
	c.frame++
	c.prev, c.cur = c.cur, c.prev
	clear(c.cur)
}

// endFrame drops every entry this frame did not use (get or put): only
// the last frame's working set stays cached.
func (c *bigPathCache) endFrame() {
	for k, e := range c.m {
		if e.used != c.frame {
			c.drop(k, e)
		}
	}
}

// get returns the entry for k with exactly segs, or nil, and marks it
// used.
func (c *bigPathCache) get(k bigPathKey, segs []paint.Seg) *bigPathEntry {
	e := c.m[k]
	if e == nil || !segsEqual(e.segs, segs) {
		return nil
	}
	e.used = c.frame
	return e
}

// sighted records a miss of k on this frame and reports whether k also
// missed on the previous frame, i.e. whether its raster may be stored.
func (c *bigPathCache) sighted(k bigPathKey) bool {
	_, again := c.prev[k]
	if c.cur == nil {
		c.cur = make(map[bigPathKey]struct{})
	}
	if len(c.cur) < bigPathMaxEntries {
		c.cur[k] = struct{}{}
	}
	return again
}

// put stores img (with its ink bounds) for k and segs, evicting the least
// recently used entries to stay within the bounds. An image larger than
// the byte budget is not stored.
func (c *bigPathCache) put(k bigPathKey, segs []paint.Seg, img *image.RGBA, ink image.Rectangle) {
	n := len(img.Pix)
	if n > bigPathMaxBytes {
		return
	}
	if old := c.m[k]; old != nil {
		c.drop(k, old)
	}
	for len(c.m) > 0 && (len(c.m) >= bigPathMaxEntries || c.bytes+n > bigPathMaxBytes) {
		var lk bigPathKey
		var le *bigPathEntry
		for k, e := range c.m {
			if le == nil || e.used < le.used {
				lk, le = k, e
			}
		}
		c.drop(lk, le)
	}
	if c.m == nil {
		c.m = make(map[bigPathKey]*bigPathEntry)
	}
	c.m[k] = &bigPathEntry{segs: append([]paint.Seg(nil), segs...), img: img, ink: ink, used: c.frame}
	c.bytes += n
}

func (c *bigPathCache) drop(k bigPathKey, e *bigPathEntry) {
	c.bytes -= len(e.img.Pix)
	delete(c.m, k)
}

// reset drops every entry and sighting (Trim, scale change, device loss).
func (c *bigPathCache) reset() { c.m, c.bytes, c.prev, c.cur = nil, 0, nil, nil }

// segsHash is FNV-1a over the segments' ops and coordinate bits.
func segsHash(segs []paint.Seg) uint64 {
	const prime = 1099511628211
	h := uint64(14695981039346656037)
	mix := func(v uint32) {
		for i := 0; i < 4; i++ {
			h ^= uint64(byte(v >> (8 * i)))
			h *= prime
		}
	}
	for _, s := range segs {
		mix(uint32(s.Op))
		for _, p := range s.P {
			mix(math.Float32bits(p.X))
			mix(math.Float32bits(p.Y))
		}
	}
	return h
}

// segsEqual compares segments exactly (coordinate bits, so NaN equals
// itself and -0 differs from 0, as the rasterizer may tell them apart).
func segsEqual(a, b []paint.Seg) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Op != b[i].Op {
			return false
		}
		for j := range a[i].P {
			if math.Float32bits(a[i].P[j].X) != math.Float32bits(b[i].P[j].X) ||
				math.Float32bits(a[i].P[j].Y) != math.Float32bits(b[i].P[j].Y) {
				return false
			}
		}
	}
	return true
}
