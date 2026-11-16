package uihost_test

import (
	"strings"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
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
