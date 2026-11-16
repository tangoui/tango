//go:build darwin

package uihost

// macOS: the canvas switches to a CAMetalLayer (platform.CanvasMetal) and
// the Metal presenter draws into it.

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/render/gpu/metal"
)

var (
	platformGPUTarget    = metalTarget
	platformNewPresenter = metal.NewPresenter
	platformGPURelease   func(platform.Canvas) // the layer / GPU window stays: Present(img) uses it
)

// metalTimeline is the Metal presenter's renderer (metal.NewPresenter):
// its Device reports drawable waits and GPU times.
type metalTimeline interface {
	SetTimeline(fn func(metal.Timing))
}

// attachTimeline connects r's timeline, when it has one, to t
// (TANGO_FRAME_TRACE, frametrace.go).
func attachTimeline(r any, t *frameTrace) {
	s, ok := r.(metalTimeline)
	if !ok {
		return
	}
	s.SetTimeline(func(m metal.Timing) {
		switch m.Kind {
		case "present":
			t.observe(m.DrawableWait, m.Drawable, 0, false, true)
		case "frame":
			t.observe(0, false, m.GPUEnd-m.GPUStart, m.GPUKnown, false)
		}
	})
}
