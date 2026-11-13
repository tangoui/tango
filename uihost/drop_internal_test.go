package uihost

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
)

// The host forwards platform drag feedback to the runner, where pages read
// it (ui.Context.FileDrag, ui.DropZone).
func TestHostFileDragOver(t *testing.T) {
	r := ui.NewRunner(func(c *ui.Context) { ui.Text(c, "drop") }, ui.RunnerOptions{})
	var h platform.CanvasDropTarget = &host{r: r}
	h.CanvasDragOver(12, 34)
	if p, ok := r.FileDrag(); !ok || p != geom.Pt(12, 34) {
		t.Fatalf("drag over: %v %v", p, ok)
	}
	h.CanvasDragOver(40, 50)
	if p, _ := r.FileDrag(); p != geom.Pt(40, 50) {
		t.Fatalf("drag move: %v", p)
	}
	h.CanvasDragLeave()
	if _, ok := r.FileDrag(); ok {
		t.Fatal("still over after leave")
	}
	// A closed host or one without a runner ignores the callbacks.
	(&host{}).CanvasDragOver(1, 1)
	closed := &host{r: r, closed: true}
	closed.CanvasDragOver(5, 5)
	if _, ok := r.FileDrag(); ok {
		t.Fatal("closed host reported a drag")
	}
}
