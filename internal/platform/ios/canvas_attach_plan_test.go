package ios

import (
	"image"
	"math"
	"testing"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func TestPlaceOverlay(t *testing.T) {
	fill := placeOverlay(platform.CanvasLayer{}, 390, 844, 844, 390)
	if fill != (overlayFrame{W: 844, H: 390, Mask: uiResizeFlexWidth | uiResizeFlexHeight}) {
		t.Errorf("fill: %+v", fill)
	}
	b := platform.Rect{X: 20, Y: 30, Width: 100, Height: 50}
	cases := []struct {
		a          platform.CanvasAnchor
		x, y       float64
		mask       int
		rotX, rotY float64 // in a 844×390 page, given for 390×844
	}{
		{platform.AnchorTopLeft, 20, 30, uiResizeFlexRight | uiResizeFlexBottom, 20, 30},
		{platform.AnchorTopRight, 20, 30, uiResizeFlexLeft | uiResizeFlexBottom, 20 + 454, 30},
		{platform.AnchorBottomLeft, 20, 30, uiResizeFlexRight | uiResizeFlexTop, 20, 30 - 454},
		{platform.AnchorBottomRight, 20, 30, uiResizeFlexLeft | uiResizeFlexTop, 20 + 454, 30 - 454},
		{platform.AnchorCenter, 20, 30, uiResizeFlexLeft | uiResizeFlexRight | uiResizeFlexTop | uiResizeFlexBottom, 20 + 227, 30 - 227},
	}
	for _, c := range cases {
		l := platform.CanvasLayer{Bounds: b, Anchor: c.a}
		f := placeOverlay(l, 390, 844, 390, 844)
		if f.X != c.x || f.Y != c.y || f.W != 100 || f.H != 50 || f.Mask != c.mask {
			t.Errorf("anchor %d same size: %+v", c.a, f)
		}
		f = placeOverlay(l, 390, 844, 844, 390)
		if f.X != c.rotX || f.Y != c.rotY || f.W != 100 || f.H != 50 {
			t.Errorf("anchor %d rotated: %+v, want %v,%v", c.a, f, c.rotX, c.rotY)
		}
	}
}

func TestHitMask(t *testing.T) {
	var m hitMask
	if m.opaqueAt(0, 0, 1) || !m.empty() {
		t.Fatal("empty mask is opaque")
	}
	// 4×2 device pixels at scale 2 (2×1 points); a sub-image to test
	// offsets and stride.
	big := image.NewRGBA(image.Rect(0, 0, 6, 3))
	img := big.SubImage(image.Rect(1, 1, 5, 3)).(*image.RGBA)
	img.Pix[img.PixOffset(3, 1)+3] = 255 // device (2,0) → point (1,0)
	img.Pix[img.PixOffset(1, 2)+3] = 20  // device (0,1) → point (0,0.5)
	m.set(img, 2)
	if m.w != 4 || m.h != 2 {
		t.Fatalf("size %dx%d", m.w, m.h)
	}
	for _, c := range []struct {
		x, y float64
		thr  uint8
		want bool
	}{
		{1.0, 0, 1, true},
		{1.4, 0.4, 1, true},
		{0, 0, 1, false},
		{0, 0.5, 1, true},
		{0, 0.5, 32, false}, // soft fringe passes through at HitAlpha 32
		{-0.1, 0, 1, false},
		{2, 0, 1, false}, // outside
		{0, 1, 1, false},
	} {
		if got := m.opaqueAt(c.x, c.y, c.thr); got != c.want {
			t.Errorf("opaqueAt(%v,%v,%d) = %v", c.x, c.y, c.thr, got)
		}
	}
	m.set(image.NewRGBA(image.Rect(0, 0, 0, 0)), 2)
	if !m.empty() {
		t.Error("empty frame keeps the mask")
	}
}

func TestScrollDelta(t *testing.T) {
	// Two fingers moving up (translation -y) reveal what is below: +DY.
	if dx, dy := scrollDelta(3, -12.5, false); dx != -3 || dy != 12.5 {
		t.Errorf("continuous: %v %v", dx, dy)
	}
	if dx, dy := scrollDelta(0, 0, false); dx != 0 || dy != 0 {
		t.Errorf("zero: %v %v", dx, dy)
	}
	if dx, dy := scrollDelta(0, -7, true); dx != 0 || math.Signbit(float64(dx)) || dy != scrollLine {
		t.Errorf("discrete: %v %v", dx, dy)
	}
	if dx, dy := scrollDelta(0.5, 30, true); dx != -scrollLine || dy != -scrollLine {
		t.Errorf("discrete both: %v %v", dx, dy)
	}
}

func TestPointerStyle(t *testing.T) {
	for c, want := range map[input.Cursor]int{
		input.CursorDefault:  pointerSystem,
		input.CursorPointer:  pointerSystem,
		input.CursorText:     pointerBeam,
		input.CursorResizeEW: pointerResizeEW,
		input.CursorResizeNS: pointerResizeNS,
		input.CursorGrab:     pointerSystem,
	} {
		if got := pointerStyle(c); got != want {
			t.Errorf("pointerStyle(%d) = %d, want %d", c, got, want)
		}
	}
}

func TestHoverState(t *testing.T) {
	var s hoverState
	at := func(x float32) input.Pointer { return input.Pointer{Pos: geom.Point{X: x}} }
	steps := []struct {
		phase  int
		opaque bool
		ok     bool
		kind   input.PointerKind
	}{
		{hoverLeave, true, false, 0}, // never entered
		{hoverEnter, true, true, input.Move},
		{hoverMove, true, true, input.Move},
		{hoverMove, false, true, input.Leave}, // onto a transparent pixel
		{hoverMove, false, false, 0},
		{hoverMove, true, true, input.Move}, // back
		{hoverLeave, true, true, input.Leave},
		{hoverLeave, true, false, 0},
	}
	for i, st := range steps {
		ev, ok := s.event(st.phase, at(float32(i)), st.opaque)
		if ok != st.ok || (ok && (ev.Kind != st.kind || ev.Source != input.SourceMouse || ev.Pos.X != float32(i))) {
			t.Errorf("step %d: %+v %v", i, ev, ok)
		}
	}
}
