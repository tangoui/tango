package fake

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/a11y"
)

// A tree read through A11yTree is already in A11yCalls: a test that waits
// for a tree and then checks the calls must find the call that sent it.
func TestA11yTreeRecordedWithCall(t *testing.T) {
	b := New()
	_, pc, err := b.NewCanvasSurface(1, platform.SurfaceOptions{Width: 100, Height: 80}, platform.SurfaceExtras{}, &nopSink{})
	if err != nil {
		t.Fatal(err)
	}
	cv := pc.(*Canvas)
	const n = 2000
	trees := make([]*a11y.Tree, n)
	for i := range trees {
		trees[i] = a11y.NewTree(1)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i, tr := range trees {
			if i%2 == 0 {
				cv.SetAccessibilityTree(tr)
			} else {
				cv.UpdateAccessibilityNodes(tr, nil)
			}
		}
	}()
	for running := true; running; {
		select {
		case <-done:
			running = false
		default:
		}
		tr := cv.A11yTree()
		if tr == nil {
			continue
		}
		found := false
		for _, c := range cv.A11yCalls() {
			if c.Tree == tr {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("A11yTree returned a tree not yet in A11yCalls")
		}
	}
}
