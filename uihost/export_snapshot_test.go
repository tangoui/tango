package uihost

import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
)

// Snapshot is the per-frame UI state a threaded window last published.
type Snapshot struct {
	EditableFocus, Composing, Capturing bool
	DragRegions                         []geom.Rect
}

// SnapshotOf returns r's latest snapshot; ok is false for a runner without
// its own UI thread. Any goroutine.
func SnapshotOf(r *ui.Runner) (s Snapshot, ok bool) {
	t := threadedOf(r)
	if t == nil {
		return Snapshot{}, false
	}
	st := t.snapshot()
	return Snapshot{st.editableFocus, st.composing, st.capturing, st.dragRegions}, true
}

// SetThreadedNativeViews sets threadedNativeViews and returns a restore
// function.
func SetThreadedNativeViews(on bool) func() {
	old := threadedNativeViews
	threadedNativeViews = on
	return func() { threadedNativeViews = old }
}

// PlacementTasks returns how many placement tasks r's threaded window has
// applied on the main thread. Main thread.
func PlacementTasks(r *ui.Runner) int {
	t := threadedOf(r)
	if t == nil {
		return -1
	}
	return t.embeds.frames
}

// MainFallback reports whether r's window started threaded and went back
// to the main-thread model. Any goroutine.
func MainFallback(r *ui.Runner) bool {
	_, ok := fallbackRunners.Load(r)
	return ok
}
