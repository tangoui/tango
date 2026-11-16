package uihost

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"time"
)

// Auto is opt-in (TANGO_RENDERER=auto). A small utility starts without a
// native GPU target, and upgrades once on growth, a dense display list, or
// six closely spaced rendered frames. Promotion is sticky to avoid repeatedly
// discarding glyph atlases and GPU pipelines during ordinary interaction.
// These are conservative workload thresholds, not claims that CPU is faster.
const (
	autoLogicalArea = 100000
	autoDenseOps    = 256
	autoRapidFrames = 6
	autoFrameGap    = 100 * time.Millisecond
)

type autoRendererState struct {
	canvas    platform.Canvas
	lastFrame time.Time
	rapid     int
}

func (s *autoRendererState) shouldPromote(l *paint.List, size geom.Size, now time.Time) bool {
	if size.W > 0 && size.H > 0 && size.W*size.H > autoLogicalArea {
		return true
	}
	if len(l.Ops) >= autoDenseOps {
		return true
	}
	if !s.lastFrame.IsZero() && now.Sub(s.lastFrame) > 0 && now.Sub(s.lastFrame) <= autoFrameGap {
		s.rapid++
	} else {
		s.rapid = 1
	}
	s.lastFrame = now
	return s.rapid >= autoRapidFrames
}
