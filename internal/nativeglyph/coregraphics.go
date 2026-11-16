// Package nativeglyph connects text-engine-owned glyph resources to optional
// native renderers without adding platform handles to the public paint API.
package nativeglyph

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"image"
)

// Run borrows IDs and Positions for one synchronous call. Size and positions are
// device pixels; positions use CoreGraphics' y-up coordinates. Source must remain
// reachable while a renderer keeps its identity or any run using it.
type Run struct {
	Source     paint.GlyphSource
	Size, Skew float32
	IDs        []uint16
	Positions  []geom.Point
}

// Provider borrows the CGContext and buffers; it never retains either. The
// caller owns the context's color, text matrix, scale and clipping state.
// Bounds returns conservative ink bounds in the run's y-up device coordinates.
// Unsupported sources/runs return false so the normal raster path can draw them.
type Provider interface {
	Available(paint.GlyphSource) bool
	Key(paint.GlyphSource) uintptr
	Bounds(Run) (image.Rectangle, bool)
	Draw(uintptr, Run) bool
}

// CoreGraphics is installed during text package initialization on Darwin and is
// immutable afterward. It is nil where no native text provider is available.
var CoreGraphics Provider
