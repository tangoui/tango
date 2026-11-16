package paint

import (
	"image"
	"image/color"
	"runtime"
	"testing"
	"weak"

	"github.com/tangoui/tango/ui/geom"
)

// Return from the drawing frame before collecting: only the live renderer,
// rather than a stale local pointer, may keep the old render target alive.
//
//go:noinline
func cpuAfterLargeOpacityFrame(t *testing.T) (*CPU, weak.Pointer[image.RGBA]) {
	t.Helper()
	r := NewCPU()
	l := &List{}
	l.Clip(geom.R(0, 0, 256, 256), geom.Rad(12))
	l.Opacity(0.75)
	l.Opacity(0.5)
	l.Fill(geom.R(0, 0, 256, 256), geom.Radii{}, color.NRGBA{R: 255, A: 255})
	l.Pop()
	l.Pop()
	l.Pop()
	if err := r.Render(l, geom.Size{W: 256, H: 256}, 1); err != nil {
		t.Fatal(err)
	}
	old := weak.Make(r.Image())
	if err := r.Render(&List{}, geom.Size{W: 16, H: 16}, 1); err != nil {
		t.Fatal(err)
	}
	return r, old
}

func TestCPUPoppedLayersReleaseReplacedRenderTarget(t *testing.T) {
	r, old := cpuAfterLargeOpacityFrame(t)
	runtime.GC()
	runtime.GC()
	retained := old.Value() != nil
	runtime.KeepAlive(r)
	if retained {
		t.Fatal("popped opacity/clip layers retain the old large render target after resize")
	}
}
