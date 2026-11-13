package mobilecanvas

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// Inset arithmetic (docs/specs/mobile-canvas.md §2.4, §3.2). Android
// reports insets in device pixels; iOS in points, which are already
// logical pixels.

// Logical converts device pixels to logical pixels at density (Android
// DisplayMetrics.density, which may be fractional such as 2.625). A
// density that is not positive counts as 1.
func Logical(px, density float32) float32 {
	if !(density > 0) {
		return px
	}
	return px / density
}

// InsetsPx converts per-edge device-pixel insets to logical pixels,
// dropping negative values.
func InsetsPx(top, right, bottom, left int, density float32) geom.Insets {
	f := func(v int) float32 { return Logical(float32(max(v, 0)), density) }
	return geom.Insets{Top: f(top), Right: f(right), Bottom: f(bottom), Left: f(left)}
}

// Clamp returns in with negative or non-numeric edges replaced by 0.
func Clamp(in geom.Insets) geom.Insets {
	f := func(v float32) float32 {
		if !(v > 0) || math.IsInf(float64(v), 0) {
			return 0
		}
		return v
	}
	return geom.Insets{Top: f(in.Top), Right: f(in.Right), Bottom: f(in.Bottom), Left: f(in.Left)}
}

// Max returns the larger value of each edge: the space to keep clear when
// both the safe area and the keyboard apply (a bottom sheet above the
// keyboard).
func Max(a, b geom.Insets) geom.Insets {
	return geom.Insets{
		Top:    max(a.Top, b.Top),
		Right:  max(a.Right, b.Right),
		Bottom: max(a.Bottom, b.Bottom),
		Left:   max(a.Left, b.Left),
	}
}

// KeyboardBottom returns the keyboard inset of a view whose bottom edge is
// at viewBottom when the keyboard's top edge is at keyboardTop, both in
// the same coordinates (iOS: the keyboard's end frame converted to the
// view). It is 0 when the keyboard is below the view or hidden.
func KeyboardBottom(viewBottom, keyboardTop float32) float32 {
	d := viewBottom - keyboardTop
	if !(d > 0) {
		return 0
	}
	return d
}

// KeyboardInsets is the keyboard inset of a view: only Bottom.
func KeyboardInsets(viewBottom, keyboardTop float32) geom.Insets {
	return geom.Insets{Bottom: KeyboardBottom(viewBottom, keyboardTop)}
}
