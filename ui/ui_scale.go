package ui

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// SetUIScale requests a window-wide UI scale for the next frame. A value
// of 1 is 100%; nonpositive and nonfinite values reset to 1, and other
// values are clamped to [0.25, 4]. Call on the UI thread.
func (r *Runner) SetUIScale(scale float32) {
	r.checkThread()
	scale = normalizeUIScale(scale)
	if r.requestedScale != scale {
		r.requestedScale = scale
		r.Invalidate()
	}
}

// UIScale returns the requested UI scale (1 is 100%). Call on the UI thread.
func (r *Runner) UIScale() float32 {
	r.checkThread()
	return r.requestedScale
}

func normalizeUIScale(s float32) float32 {
	if !(s > 0) || math.IsInf(float64(s), 0) {
		return 1
	}
	return min(4, max(0.25, s))
}

func scalePoint(p geom.Point, s float32) geom.Point { return geom.Pt(p.X*s, p.Y*s) }

func scaleRect(r geom.Rect, s float32) geom.Rect {
	return geom.R(r.X*s, r.Y*s, r.W*s, r.H*s)
}

func scaleInsets(i geom.Insets, s float32) geom.Insets {
	return geom.Insets{Top: i.Top * s, Right: i.Right * s, Bottom: i.Bottom * s, Left: i.Left * s}
}

func logicalEvent(e input.Event, scale float32) input.Event {
	if scale == 1 {
		return e
	}
	s := 1 / scale
	switch v := e.(type) {
	case input.Pointer:
		v.Pos = scalePoint(v.Pos, s)
		return v
	case input.Scroll:
		v.Pos, v.DX, v.DY = scalePoint(v.Pos, s), v.DX*s, v.DY*s
		return v
	case input.TouchEvent:
		v.Pos = scalePoint(v.Pos, s)
		return v
	default:
		return e
	}
}

// FrameMetrics returns the viewport and effective device-pixel scale to
// use when rendering List again, for example to capture a screenshot.
func (r *Runner) FrameMetrics() (size geom.Size, scale float32) {
	r.checkThread()
	return r.renderSize, r.renderScale
}

// Correct float32 roundoff at the render target boundary. CPU targets
// round up while GPU targets and swap chains round to nearest.
func renderExtent(device, scale float32) float32 {
	logical := device / scale
	actual := logical * scale
	if math.Ceil(float64(actual)) > math.Ceil(float64(device)) ||
		math.Round(float64(actual)) > math.Round(float64(device)) {
		return math.Nextafter32(logical, 0)
	}
	if math.Ceil(float64(actual)) < math.Ceil(float64(device)) ||
		math.Round(float64(actual)) < math.Round(float64(device)) {
		return math.Nextafter32(logical, float32(math.Inf(1)))
	}
	return logical
}
