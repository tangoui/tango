//go:build darwin && !ios

package uihost_test

import (
	"image"
	"sync"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/render/gpu/metal"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/uihost"
)

type threadTimelineGPU struct {
	*fakeGPU
	tlMu sync.Mutex
	tlOn []*uithread.Thread
}

func (g *threadTimelineGPU) SetTimeline(func(metal.Timing)) {
	g.tlMu.Lock()
	g.tlOn = append(g.tlOn, uithread.Current())
	g.tlMu.Unlock()
}

func TestThreadedFrameTraceAttachesOnUIThread(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "gpu")
	t.Setenv("TANGO_FRAME_TRACE", "1")
	g := &threadTimelineGPU{fakeGPU: &fakeGPU{}}
	t.Cleanup(uihost.SetGPUHooks(func(layer uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		_, present, err := g.fakeGPU.newPresenter(layer)
		return g, present, err
	}, nil))
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	_, _, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) { ui.Text(c, "trace") })
	tc.flush(t)
	g.tlMu.Lock()
	defer g.tlMu.Unlock()
	if len(g.tlOn) != 1 || g.tlOn[0] != tc.th {
		t.Fatalf("SetTimeline on %v, want once on the UI thread", g.tlOn)
	}
}
