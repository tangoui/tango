package uihost_test

import (
	"fmt"
	"image"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/render/gpu/metal"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/uihost"
)

// TestFrameTraceOff: without TANGO_FRAME_TRACE nothing is logged.
func TestFrameTraceOff(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	t.Setenv("TANGO_FRAME_TRACE", "")
	lr := traceHooks(t, 4)
	g := &fakeGPU{}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	_, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	for range 10 {
		cv.Invalidate()
		cv.Frame()
	}
	if l := lr.get(); len(l) != 0 {
		t.Fatalf("frame trace logged without TANGO_FRAME_TRACE: %q", l)
	}
}

// TestFrameTraceSummary: with TANGO_FRAME_TRACE=1 a GPU window logs one
// summary line per window of frames, with the frame, render and present
// intervals and the frame gap; a presenter without a timeline reports no
// drawable or GPU fields.
func TestFrameTraceSummary(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	t.Setenv("TANGO_FRAME_TRACE", "1")
	lr := traceHooks(t, 4)
	g := &fakeGPU{}
	hooks(t, g)
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	_, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	for range 12 {
		cv.Invalidate()
		cv.Frame()
	}
	var lines []string
	for _, l := range lr.get() {
		if strings.HasPrefix(l, "tango: frame trace") {
			lines = append(lines, l)
		}
	}
	if len(lines) < 2 {
		t.Fatalf("want at least 2 summaries of 4 frames, got %q", lr.get())
	}
	l := lines[0]
	for _, want := range []string{"renderer=gpu", "n=4", "frame p50=", "render p50=", "present p50=", "gap p50="} {
		if !strings.Contains(l, want) {
			t.Fatalf("summary %q lacks %q", l, want)
		}
	}
	for _, no := range []string{"drawable", "gpu p50"} {
		if strings.Contains(l, no) {
			t.Fatalf("summary %q reports %q without a presenter timeline", l, no)
		}
	}
}

// traceHooks makes frame-trace summaries cover every n frames and records
// what they log.
func traceHooks(t *testing.T, n int) *logRec {
	t.Helper()
	lr := &logRec{}
	t.Cleanup(uihost.SetFrameTraceHooks(n, lr.logf))
	return lr
}

// timelineGPU is a fakeGPU whose renderer has a Metal-style timeline
// (metal.NewPresenter's SetTimeline): every Render reports a frame with
// gpu GPU time, every present a drawable wait.
type timelineGPU struct {
	*fakeGPU
	tl func(metal.Timing)
}

func (g *timelineGPU) SetTimeline(fn func(metal.Timing)) { g.tl = fn }

var _ metal.TimelineSource = (*timelineGPU)(nil)

func (g *timelineGPU) newPresenter(layer uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
	_, present, err := g.fakeGPU.newPresenter(layer)
	return g, func(size image.Point, scale float32) error {
		if err := present(size, scale); err != nil {
			return err
		}
		if g.tl != nil {
			g.tl(metal.Timing{Kind: "present", Drawable: true, DrawableWait: 2 * time.Millisecond})
		}
		return nil
	}, err
}

func (g *timelineGPU) Render(l *paint.List, size geom.Size, scale float32) error {
	if err := g.fakeGPU.Render(l, size, scale); err != nil {
		return err
	}
	if g.tl != nil {
		g.tl(metal.Timing{Kind: "frame", GPUKnown: true, GPUStart: time.Second, GPUEnd: time.Second + 3*time.Millisecond})
	}
	return nil
}

// needTimeline skips t where no presenter has a timeline: attachTimeline
// is a no-op off macOS (frametrace_other.go).
func needTimeline(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("only the Metal presenter (macOS) has a timeline")
	}
}

// TestFrameTraceTimeline (review M5): a renderer with a timeline adds the
// drawable, misses and gpu fields.
func TestFrameTraceTimeline(t *testing.T) {
	needTimeline(t)
	t.Setenv("TANGO_RENDERER", "")
	t.Setenv("TANGO_FRAME_TRACE", "1")
	lr := traceHooks(t, 4)
	g := &timelineGPU{fakeGPU: &fakeGPU{}}
	t.Cleanup(uihost.SetGPUHooks(g.newPresenter, (&logRec{}).logf))
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	_, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	for range 8 {
		cv.Invalidate()
		cv.Frame()
	}
	l := lastTrace(t, lr)
	for _, want := range []string{"drawable p50=2.00ms", "misses=0", "gpu p50=3.00ms"} {
		if !strings.Contains(l, want) {
			t.Fatalf("summary %q lacks %q", l, want)
		}
	}
}

// TestFrameTraceAfterFallback (review M1, M2): once the GPU is lost the
// summaries say renderer=cpu and carry no timeline fields (a bare misses=0
// would read as "no drawable misses").
func TestFrameTraceAfterFallback(t *testing.T) {
	needTimeline(t)
	t.Setenv("TANGO_RENDERER", "")
	t.Setenv("TANGO_FRAME_TRACE", "1")
	lr := traceHooks(t, 1) // every frame is summarized: the losing frame too
	g := &timelineGPU{fakeGPU: &fakeGPU{presErr: nil}}
	t.Cleanup(uihost.SetGPUHooks(g.newPresenter, (&logRec{}).logf))
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	r, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	cv.Invalidate()
	cv.Frame()
	if l := lastTrace(t, lr); !strings.Contains(l, "renderer=gpu") || !strings.Contains(l, "misses=") {
		t.Fatalf("GPU summary %q", l)
	}
	g.mu.Lock()
	g.presErr = fmt.Errorf("present: %w", metal.ErrDeviceLost)
	g.mu.Unlock()
	r.Invalidate()
	cv.FlushFrame() // the device is lost in this frame's present
	if l := lastTrace(t, lr); !strings.Contains(l, "renderer=cpu") {
		t.Fatalf("summary of the frame that lost the GPU = %q, want renderer=cpu", l)
	}
	r.Invalidate()
	cv.FlushFrame() // a CPU frame
	l := lastTrace(t, lr)
	if !strings.Contains(l, "renderer=cpu") || strings.Contains(l, "misses") || strings.Contains(l, "drawable") {
		t.Fatalf("CPU summary %q: want renderer=cpu without timeline fields", l)
	}
}

// TestFrameTraceNoGPUNotice (review M4): a window that gets no trace
// (here TANGO_RENDERER=cpu) says so once instead of staying silent.
func TestFrameTraceNoGPUNotice(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "cpu")
	t.Setenv("TANGO_FRAME_TRACE", "1")
	lr := traceHooks(t, 4)
	hooks(t, &fakeGPU{})
	a, fb := faketest.Run(t, tango.Options{})
	fb.EnableCanvasMetal(0)
	_, _, cv := open(t, a, fb, 300, 200, func(c *ui.Context) { ui.Text(c, "hello") })
	for range 8 {
		cv.Invalidate()
		cv.Frame()
	}
	var notes []string
	for _, l := range lr.get() {
		if strings.Contains(l, "frame trace") {
			notes = append(notes, l)
		}
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "not traced") || !strings.Contains(notes[0], "CPU") {
		t.Fatalf("want one 'not traced' notice for a CPU window, got %q", notes)
	}
}

// lastTrace returns the last frame-trace summary logged.
func lastTrace(t *testing.T, lr *logRec) string {
	t.Helper()
	l := lr.get()
	for i := len(l) - 1; i >= 0; i-- {
		if strings.HasPrefix(l[i], "tango: frame trace surface") {
			return l[i]
		}
	}
	t.Fatalf("no frame-trace summary in %q", l)
	return ""
}
