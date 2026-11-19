package uihost

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

// Open windows ask the backend for a threaded canvas (macOS gives one).
func TestHostWantsUIThread(t *testing.T) {
	var sink platform.CanvasSink = &host{}
	ts, ok := sink.(platform.CanvasThreadedSink)
	if !ok || !ts.WantsUIThread() {
		t.Fatal("uihost.Open's sink does not ask for a UI thread")
	}
}
