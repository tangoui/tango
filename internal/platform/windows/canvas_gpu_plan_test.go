package windows

import "testing"

func TestCanvasGPUPaint(t *testing.T) {
	cases := []struct {
		on, ready, changed, wanted, have bool
		frame, blit                      bool
	}{
		// CPU path: as before (frame when due or no DIB yet, always blit).
		{false, false, false, false, false, false, true},
		{false, true, false, false, true, false, true},
		{false, true, false, false, false, true, true},
		{false, true, true, false, true, true, true},
		{false, true, false, true, true, true, true},
		// GPU path: never blit; frame only when due, whatever the DIB.
		{true, false, true, true, false, false, false},
		{true, true, false, false, false, false, false},
		{true, true, true, false, false, true, false},
		{true, true, false, true, true, true, false},
	}
	for _, c := range cases {
		g := canvasGPU{on: c.on}
		f, b := g.paint(c.ready, c.changed, c.wanted, c.have)
		if f != c.frame || b != c.blit {
			t.Errorf("%+v: frame %v blit %v", c, f, b)
		}
	}
}

func TestCanvasGPUPresentSibling(t *testing.T) {
	var cpu canvasGPU
	for _, img := range []bool{true, false, true} {
		if op := cpu.present(img); op != siblingNone || cpu.cpuShown {
			t.Fatalf("CPU path: Present(img=%v) = %v", img, op)
		}
	}
	g := canvasGPU{on: true}
	steps := []struct {
		img   bool
		want  canvasSiblingOp
		shown bool
	}{
		{false, siblingNone, false}, // GPU frame, sibling never shown
		{true, siblingShow, true},   // presenter failed / oversized frame
		{true, siblingRedraw, true}, // still on the CPU
		{false, siblingHide, false}, // back on the GPU
		{false, siblingNone, false},
		{true, siblingShow, true},
	}
	for i, s := range steps {
		if op := g.present(s.img); op != s.want || g.cpuShown != s.shown {
			t.Fatalf("step %d: Present(img=%v) = %v shown %v, want %v %v", i, s.img, op, g.cpuShown, s.want, s.shown)
		}
	}
}

func TestCanvasGPUSupport(t *testing.T) {
	if !canvasGPUSupport(false, false) || canvasGPUSupport(true, false) || canvasGPUSupport(false, true) || canvasGPUSupport(true, true) {
		t.Fatal("canvasGPUSupport")
	}
}
