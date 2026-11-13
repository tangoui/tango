package linux

import "testing"

func TestCanvasGLTick(t *testing.T) {
	cases := []struct {
		on, changed, wanted, ready bool
		draw, frame                bool
	}{
		// Cairo path: as before (size change, wanted draw or the first
		// frame render inside the draw handler; otherwise render now).
		{false, false, false, true, false, true},
		{false, true, false, true, true, false},
		{false, false, true, true, true, false},
		{false, false, false, false, true, false},
		// GL path: always render now, never through the drawing area.
		{true, false, false, true, false, true},
		{true, true, false, true, false, true},
		{true, false, true, true, false, true},
		{true, false, false, false, false, true},
	}
	for _, c := range cases {
		g := canvasGL{on: c.on}
		d, f := g.tick(c.changed, c.wanted, c.ready)
		if d != c.draw || f != c.frame {
			t.Errorf("%+v: draw %v frame %v", c, d, f)
		}
	}
}

func TestCanvasGLDrawing(t *testing.T) {
	cases := []struct {
		g                                   canvasGL
		resizeFrame, drawFrames, paint, par bool
	}{
		{canvasGL{}, false, true, true, true},
		{canvasGL{on: true}, true, false, false, false},
		{canvasGL{on: true, cpuShown: true}, true, false, true, false},
	}
	for _, c := range cases {
		if c.g.resizeFrame() != c.resizeFrame || c.g.drawFrames() != c.drawFrames ||
			c.g.paintSurface() != c.paint || c.g.partial() != c.par {
			t.Errorf("%+v: resizeFrame %v drawFrames %v paintSurface %v partial %v", c.g,
				c.g.resizeFrame(), c.g.drawFrames(), c.g.paintSurface(), c.g.partial())
		}
	}
}

func TestCanvasGLPresent(t *testing.T) {
	var cairo canvasGL
	for _, img := range []bool{true, false, true} {
		if op := cairo.present(img); op != glAreaNone || cairo.cpuShown {
			t.Fatalf("cairo path: Present(img=%v) = %v", img, op)
		}
	}
	g := canvasGL{on: true}
	steps := []struct {
		img   bool
		want  canvasGLAreaOp
		shown bool
	}{
		{false, glAreaNone, false}, // GPU frame, the GLArea stays shown
		{true, glAreaHide, true},   // presenter failed / oversized frame
		{true, glAreaNone, true},   // still on the CPU
		{false, glAreaShow, false}, // back on the GPU
		{false, glAreaNone, false},
		{true, glAreaHide, true},
	}
	for i, s := range steps {
		if op := g.present(s.img); op != s.want || g.cpuShown != s.shown {
			t.Fatalf("step %d: Present(img=%v) = %v shown %v, want %v %v", i, s.img, op, g.cpuShown, s.want, s.shown)
		}
	}
}

func TestCanvasGLSupport(t *testing.T) {
	if !canvasGLSupport(false, false) || canvasGLSupport(true, false) || canvasGLSupport(false, true) || canvasGLSupport(true, true) {
		t.Fatal("canvasGLSupport")
	}
}

// TestCanvasGLRelease: ReleaseGLArea returns the canvas to the cairo path
// (draw-handler frames, surface painting, partial presents, Present never
// touching a GLArea) and refuses a second switch.
func TestCanvasGLRelease(t *testing.T) {
	var cairo canvasGL
	if cairo.release() || cairo.released {
		t.Fatal("release on the cairo path")
	}
	for _, shown := range []bool{false, true} {
		g := canvasGL{on: true, cpuShown: shown}
		if !g.release() {
			t.Fatalf("cpuShown %v: release reported no GLArea", shown)
		}
		if g.on || g.cpuShown || !g.released {
			t.Fatalf("cpuShown %v: after release %+v", shown, g)
		}
		if !g.drawFrames() || !g.paintSurface() || !g.partial() || g.resizeFrame() {
			t.Fatalf("cpuShown %v: not the cairo path after release", shown)
		}
		if draw, frame := g.tick(true, false, true); !draw || frame {
			t.Fatalf("cpuShown %v: tick after a size change = %v %v", shown, draw, frame)
		}
		if op := g.present(true); op != glAreaNone {
			t.Fatalf("cpuShown %v: Present(img) = %v", shown, op)
		}
		if g.release() {
			t.Fatalf("cpuShown %v: second release reported a GLArea", shown)
		}
	}
}
