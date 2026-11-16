package uihost

// Frame latency trace (TANGO_FRAME_TRACE=1): a GPU window logs, every
// frameTraceEvery frames, a summary line of where its frames spend time:
//
//	tango: frame trace surface 1 renderer=gpu n=120 frame p50=1.1ms p95=1.6ms max=2.0ms
//	    render p50=0.8ms p95=1.2ms present p50=0.3ms p95=0.4ms gap p50=16.7ms p95=16.8ms
//	    drawable p50=0.01ms p95=0.02ms max=0.05ms misses=0 gpu p50=1.8ms p95=2.1ms
//
// frame is CanvasFrame from start to end (Runner.Frame: layout, paint,
// Render and Present); render is the renderer's Render, present is
// presentFrame (on macOS: the presenter's present, which includes
// nextDrawable); gap is the time between the starts of consecutive frames
// (the display link's pacing, longer when nothing asked for a frame). The
// drawable and gpu fields come from the presenter's timeline when it has
// one (metal.Device.SetTimeline): drawable is nextDrawable's blocking time,
// misses the presents that got no drawable, gpu the frames' GPU execution
// time. Nothing here is a time on screen: no platform reports when a frame
// was displayed.
//
// The fields count different events, so their sample counts differ from
// n (the frames summarized): render and present only frames that rendered
// or presented (a frame the runner skips adds to frame and gap only); gpu
// every GPU frame the device retired in the period — more than n under
// gpu.Renderer's verify mode (which renders a frame twice), and lagging by
// a frame or two because frames retire asynchronously; drawable and
// misses one per present. renderer is the window's renderer when the
// summarized period ended ("cpu" from the frame that lost the GPU on).
// The drawable, misses and gpu fields appear only for a period in which a
// timeline reported.
//
// Only windows presented by a GPU renderer (uihost.Open with a GPU or
// native presenter, also after a native window promotes to Metal) are
// traced; a native window has no Metal timeline, so only frame, render,
// present and gap. A window drawn by the CPU (TANGO_RENDERER=cpu, no GPU
// presenter as on Linux without usable OpenGL, a renderer chosen in
// ui.RunnerOptions), an attached layer (Attach) and a tan dev window log
// one "not traced" line instead.
//
// Off by default; it times nothing and allocates nothing when off.

import (
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

// Test hooks.
var (
	frameTraceEvery = 120
	frameTraceLogf  = log.Printf
)

// frameTraceOn reports whether TANGO_FRAME_TRACE asks for the trace.
func frameTraceOn() bool {
	v := os.Getenv("TANGO_FRAME_TRACE")
	return v != "" && v != "0"
}

// attachTimeline (gpu_<GOOS>.go) connects a presenter's timeline, when it
// has one, to a trace: Metal on macOS, nothing elsewhere.

// frameTrace accumulates one window's frame intervals. Main thread.
type frameTrace struct {
	id    platform.SurfaceID
	start time.Time // start of the frame in progress
	last  time.Time // start of the previous frame

	frame, render, present, gap []time.Duration
	drawable, gpu               []time.Duration
	misses                      int
	timeline                    bool // drawable/gpu come from a presenter timeline
}

func newFrameTrace(id platform.SurfaceID) *frameTrace { return &frameTrace{id: id} }

// frameTraceSkipped logs, when TANGO_FRAME_TRACE is set, that window id
// gets no trace and why, so the variable is never silently ignored.
func frameTraceSkipped(id platform.SurfaceID, why string) {
	if frameTraceOn() {
		frameTraceLogf("tango: frame trace surface %d not traced: %s", id, why)
	}
}

// begin marks the start of a frame.
func (t *frameTrace) begin() {
	now := time.Now()
	if !t.last.IsZero() {
		t.gap = append(t.gap, now.Sub(t.last))
	}
	t.last, t.start = now, now
}

// end marks the end of the frame begun last and logs a summary every
// frameTraceEvery frames.
func (t *frameTrace) end(renderer string) {
	if t.start.IsZero() {
		return
	}
	t.frame = append(t.frame, time.Since(t.start))
	t.start = time.Time{}
	if len(t.frame) >= frameTraceEvery {
		t.flush(renderer)
	}
}

// observe receives one timeline event from the presenter.
func (t *frameTrace) observe(drawableWait time.Duration, drawable bool, gpu time.Duration, gpuKnown bool, present bool) {
	t.timeline = true
	if present {
		t.drawable = append(t.drawable, drawableWait)
		if !drawable {
			t.misses++
		}
		return
	}
	if gpuKnown {
		t.gpu = append(t.gpu, gpu)
	}
}

func (t *frameTrace) flush(renderer string) {
	var b strings.Builder
	fmt.Fprintf(&b, "tango: frame trace surface %d renderer=%s n=%d", t.id, renderer, len(t.frame))
	stat(&b, "frame", t.frame, true)
	stat(&b, "render", t.render, false)
	stat(&b, "present", t.present, false)
	stat(&b, "gap", t.gap, false)
	if t.timeline {
		stat(&b, "drawable", t.drawable, true)
		fmt.Fprintf(&b, " misses=%d", t.misses)
		stat(&b, "gpu", t.gpu, false)
	}
	frameTraceLogf("%s", b.String())
	t.frame, t.render, t.present, t.gap = t.frame[:0], t.render[:0], t.present[:0], t.gap[:0]
	t.drawable, t.gpu, t.misses = t.drawable[:0], t.gpu[:0], 0
	// Decided again for every summary: after a fallback to the CPU no
	// timeline reports, and a bare misses=0 would read as "no misses".
	t.timeline = false
}

// stat appends " name p50=… p95=…" (and max) for d, sorting it.
func stat(b *strings.Builder, name string, d []time.Duration, withMax bool) {
	if len(d) == 0 {
		return
	}
	slices.Sort(d)
	q := func(f float64) string { return ms(d[int(f*float64(len(d)-1))]) }
	fmt.Fprintf(b, " %s p50=%s p95=%s", name, q(.5), q(.95))
	if withMax {
		fmt.Fprintf(b, " max=%s", ms(d[len(d)-1]))
	}
}

func ms(d time.Duration) string { return fmt.Sprintf("%.2fms", float64(d)/1e6) }

// timed runs fn and appends its duration to *into when t is non-nil.
func (t *frameTrace) timed(into *[]time.Duration, fn func() error) error {
	if t == nil {
		return fn()
	}
	s := time.Now()
	err := fn()
	*into = append(*into, time.Since(s))
	return err
}
