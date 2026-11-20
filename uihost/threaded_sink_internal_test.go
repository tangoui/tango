package uihost

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestHostDoesNotRequestUIThreadByDefault(t *testing.T) {
	var sink platform.CanvasSink = &host{}
	ts, ok := sink.(platform.CanvasThreadedSink)
	if !ok {
		t.Fatal("uihost.Open's sink does not implement platform.CanvasThreadedSink")
	}
	if ts.WantsUIThread() {
		t.Fatal("uihost.Open's sink requests a UI thread by default")
	}
}
