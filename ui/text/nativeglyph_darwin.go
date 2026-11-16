//go:build darwin

package text

import (
	"image"
	"math"
	"runtime"
	"unsafe"

	"github.com/tangoui/tango/internal/nativeglyph"
	"github.com/tangoui/tango/ui/paint"
)

const nativeGlyphRunLimit = 16384
const nativeGlyphMetricLimit = 2048

type nativeGlyphMetricKey struct {
	id         uint16
	size, skew float32
}
type coreGraphicsGlyphs struct{}

func init() { nativeglyph.CoreGraphics = coreGraphicsGlyphs{} }

func nativeRaster(src paint.GlyphSource) (*Face, *ctRaster) {
	f, ok := src.(*Face)
	if !ok || f == nil {
		return nil, nil
	}
	f.ready()
	r, ok := f.ras.(*ctRaster)
	if !ok || r.color {
		return nil, nil
	}
	return f, r
}
func (coreGraphicsGlyphs) Available(src paint.GlyphSource) bool {
	_, r := nativeRaster(src)
	return r != nil
}
func (coreGraphicsGlyphs) Key(src paint.GlyphSource) uintptr {
	f, r := nativeRaster(src)
	if r == nil {
		return 0
	}
	return uintptr(unsafe.Pointer(f))
}
func nativeRunValid(run nativeglyph.Run) bool {
	if len(run.IDs) != len(run.Positions) || len(run.IDs) > nativeGlyphRunLimit || !finiteNative(run.Size) || run.Size <= 0 || !finiteNative(run.Skew) {
		return false
	}
	for _, p := range run.Positions {
		if !finiteNative(p.X) || !finiteNative(p.Y) || math.Abs(float64(p.X)) > 1<<24 || math.Abs(float64(p.Y)) > 1<<24 {
			return false
		}
	}
	return true
}
func finiteNative(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) }

func (coreGraphicsGlyphs) Bounds(run nativeglyph.Run) (image.Rectangle, bool) {
	if !nativeRunValid(run) {
		return image.Rectangle{}, false
	}
	_, r := nativeRaster(run.Source)
	if r == nil {
		return image.Rectangle{}, false
	}
	defer runtime.KeepAlive(run.Source)
	defer runtime.KeepAlive(r)
	r.mu.Lock()
	defer r.mu.Unlock()
	font := r.font(run.Size, run.Skew)
	if font == 0 {
		return image.Rectangle{}, false
	}
	if r.nativeMetrics == nil {
		r.nativeMetrics = make(map[nativeGlyphMetricKey]cgRect)
	}
	var bounds image.Rectangle
	for i, gid := range run.IDs {
		key := nativeGlyphMetricKey{gid, run.Size, run.Skew}
		b, ok := r.nativeMetrics[key]
		if !ok {
			ctFontGetBoundingRectsForGlyphs(font, ctFontOrientationHorz, &gid, &b, 1)
			if len(r.nativeMetrics) >= nativeGlyphMetricLimit {
				clear(r.nativeMetrics)
			}
			r.nativeMetrics[key] = b
		}
		if b.Size.W <= 0 || b.Size.H <= 0 {
			continue
		}
		// Match the rasterizer's allowance for smoothing, skew and quarter-pixel
		// placement. Empty glyphs deliberately contribute no ink rectangle.
		skew := math.Abs(float64(run.Skew)) * math.Max(math.Abs(b.Origin.Y), math.Abs(b.Origin.Y+b.Size.H))
		x, y := float64(run.Positions[i].X), float64(run.Positions[i].Y)
		rect := image.Rect(int(math.Floor(x+b.Origin.X-skew))-4, int(math.Floor(y+b.Origin.Y))-4, int(math.Ceil(x+b.Origin.X+b.Size.W+skew))+4, int(math.Ceil(y+b.Origin.Y+b.Size.H))+4)
		bounds = bounds.Union(rect)
	}
	return bounds, true
}

func (coreGraphicsGlyphs) Draw(ctx uintptr, run nativeglyph.Run) bool {
	if ctx == 0 || !nativeRunValid(run) {
		return false
	}
	_, r := nativeRaster(run.Source)
	if r == nil {
		return false
	}
	defer runtime.KeepAlive(run)
	defer runtime.KeepAlive(r)
	if len(run.IDs) == 0 {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	font := r.font(run.Size, run.Skew)
	if font == 0 {
		return false
	}
	if cap(r.nativePositions) < len(run.IDs) {
		r.nativePositions = make([]cgPoint, len(run.IDs))
	}
	positions := r.nativePositions[:len(run.IDs)]
	for i, p := range run.Positions {
		positions[i] = cgPoint{X: float64(p.X), Y: float64(p.Y)}
	}
	ctFontDrawGlyphs(font, &run.IDs[0], &positions[0], int64(len(run.IDs)), ctx)
	return true
}
