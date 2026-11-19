package tango

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

type uiThreadSink struct {
	platform.CanvasSink
	want bool
}

func (s uiThreadSink) WantsUIThread() bool { return s.want }

// The backend sees the sink through canvasSink; the UI-thread request must
// pass through it unchanged.
func TestCanvasSinkForwardsWantsUIThread(t *testing.T) {
	var plain platform.CanvasSink = canvasSink{sink: uiThreadSink{}}
	if plain.(platform.CanvasThreadedSink).WantsUIThread() {
		t.Fatal("declining sink forwarded as wanting a UI thread")
	}
	if !(canvasSink{sink: uiThreadSink{want: true}}).WantsUIThread() {
		t.Fatal("UI-thread request lost")
	}
	if (canvasSink{sink: struct{ platform.CanvasSink }{}}).WantsUIThread() {
		t.Fatal("sink without the interface forwarded as wanting a UI thread")
	}
}
