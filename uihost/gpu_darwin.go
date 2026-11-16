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

// attachTimeline connects r's timeline, when it has one (the Metal
// presenter's renderer, metal.TimelineSource), to t (TANGO_FRAME_TRACE,
// frametrace.go).
func attachTimeline(r any, t *frameTrace) {
	s, ok := r.(metal.TimelineSource)
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
