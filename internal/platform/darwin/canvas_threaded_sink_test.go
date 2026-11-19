//go:build darwin && !ios

package darwin

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

type uiThreadSink struct {
	platform.CanvasSink
	want bool
}

func (s uiThreadSink) WantsUIThread() bool { return s.want }

// NewCanvasSurface returns a threaded canvas only for sinks that ask for a
// UI thread (uihost.Open); the tan dev host's sink and plain sinks do not.
func TestWantsUIThread(t *testing.T) {
	if wantsUIThread(&shortcutTestSink{}) {
		t.Fatal("a plain sink got a UI thread")
	}
	if wantsUIThread(uiThreadSink{want: false}) {
		t.Fatal("a sink that declined got a UI thread")
	}
	if !wantsUIThread(uiThreadSink{want: true}) {
		t.Fatal("a sink that asked got no UI thread")
	}
}

// Attached canvases (uihost.Attach) keep the main thread as their UI thread.
func TestAttachedCanvasNotThreaded(t *testing.T) {
	var cv platform.Canvas = &overlay{canvas: &canvas{}}
	if _, ok := cv.(platform.CanvasThreaded); ok {
		t.Fatal("attached canvas implements platform.CanvasThreaded")
	}
	var plain platform.Canvas = &canvas{}
	if _, ok := plain.(platform.CanvasThreaded); ok {
		t.Fatal("unwrapped canvas implements platform.CanvasThreaded")
	}
}
