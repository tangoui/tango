package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// paintTourMaskOld is the Tour mask as painted up to 9420bb8: one path of
// the whole window with the hole punched out (non-zero rule). It is the
// "before" image for TestTourMaskPixelEquivalence.
func paintTourMaskOld(l *paint.List, r, hole geom.Rect, holeR float32, hasTarget bool, col theme.Color) {
	p := paint.NewPath().Rect(r)
	if hasTarget {
		p.Append(paint.NewPath().RRect(hole, geom.Rad(holeR)).Reversed())
	}
	l.Path(p, col)
}

type tourMaskCase struct {
	name      string
	hole      geom.Rect
	holeR     float32
	hasTarget bool
}

func tourMaskCases() []tourMaskCase {
	return []tourMaskCase{
		{"fractional r6", geom.Rect{X: 37.3, Y: 41.65, W: 120.4, H: 33.7}, 6, true},
		{"fractional r2", geom.Rect{X: 101.1, Y: 63.9, W: 77.55, H: 31.2}, 2, true},
		{"fractional r0", geom.Rect{X: 37.3, Y: 41.65, W: 120.4, H: 33.7}, 0, true},
		{"integer r6", geom.Rect{X: 40, Y: 48, W: 120, H: 32}, 6, true},
		{"integer r0", geom.Rect{X: 40, Y: 48, W: 120, H: 32}, 0, true},
		{"pill (radius clamped)", geom.Rect{X: 60.4, Y: 90.2, W: 30.3, H: 20.7}, 40, true},
		{"tiny hole r6", geom.Rect{X: 70.6, Y: 70.6, W: 3.1, H: 2.2}, 6, true},
		{"empty hole", geom.Rect{X: 70.6, Y: 70.6}, 2, true},
		{"partly off-screen", geom.Rect{X: -12.3, Y: 150.4, W: 90.5, H: 80}, 6, true},
		{"fully off-screen", geom.Rect{X: 400, Y: 20, W: 40, H: 30}, 6, true},
		{"no target", geom.Rect{}, 6, false},
	}
}

// renderTourMask renders a busy background, then the mask (optionally in
// an opacity layer, as during the Tour's fade-in) on the CPU renderer.
func renderTourMask(paintFn func(*paint.List, geom.Rect, geom.Rect, float32, bool, theme.Color), win geom.Size, scale float32, tc tourMaskCase, opacity float32) *image.RGBA {
	var l paint.List
	l.Background = color.NRGBA{255, 255, 255, 255}
	full := geom.Rect{W: win.W, H: win.H}
	l.LinearStops(full, geom.Radii{}, geom.Pt(0, 0), geom.Pt(win.W, win.H),
		paint.Stop{Offset: 0, Color: color.NRGBA{250, 40, 90, 255}}, paint.Stop{Offset: 1, Color: color.NRGBA{30, 200, 240, 255}})
	l.Fill(geom.Rect{X: 20, Y: 30, W: 150, H: 60}, geom.Rad(4), color.NRGBA{255, 255, 255, 255})
	if opacity < 1 {
		l.Opacity(opacity)
	}
	paintFn(&l, full, tc.hole, tc.holeR, tc.hasTarget, color.NRGBA{0, 0, 0, 115}) // antd colorBgMask rgba(0,0,0,.45)
	if opacity < 1 {
		l.Pop()
	}
	cpu := paint.NewCPU()
	if err := cpu.Render(&l, win, scale); err != nil {
		panic(err)
	}
	img := cpu.Image()
	out := image.NewRGBA(img.Rect)
	copy(out.Pix, img.Pix)
	return out
}

func imageDiff(a, b *image.RGBA) (maxDiff, pixels int) {
	for i := 0; i < len(a.Pix); i += 4 {
		px := 0
		for k := 0; k < 4; k++ {
			d := int(a.Pix[i+k]) - int(b.Pix[i+k])
			if d < 0 {
				d = -d
			}
			px = max(px, d)
		}
		if px > 0 {
			pixels++
		}
		maxDiff = max(maxDiff, px)
	}
	return maxDiff, pixels
}

// TestTourMaskPixelEquivalence: the mask paints the same pixels as the old
// single window-sized path. Tolerance: a per-channel difference of at most
// 1, from float accumulation order in the path rasterizer (the hole's
// corners are rasterized relative to a small path's origin instead of the
// window's) and from fillRect's blend rounding versus x/image/vector's.
// Seams between the pieces must not show (they would be ≥ 10 levels at
// alpha .45), so any larger difference fails.
func TestTourMaskPixelEquivalence(t *testing.T) {
	win := geom.Size{W: 240, H: 188} // whole device pixels at every tested scale, as a real window
	for _, scale := range []float32{1, 2, 1.25, 1.5} {
		for _, tc := range tourMaskCases() {
			for _, op := range []float32{1, 0.6} {
				before := renderTourMask(paintTourMaskOld, win, scale, tc, op)
				after := renderTourMask(paintTourMask, win, scale, tc, op)
				md, n := imageDiff(before, after)
				t.Logf("scale %-4g %-22s opacity %-3g: max diff %d, %d differing pixels", scale, tc.name, op, md, n)
				if md > 1 {
					t.Errorf("scale %g %s opacity %g: max per-channel diff %d (> 1) over %d pixels", scale, tc.name, op, md, n)
				}
			}
		}
	}
}

// tourMaskOps returns the paint ops of a shown Tour's mask (identified by
// its unique mask color).
func tourMaskOps(t *testing.T, scale float32, target bool, holeR float32) []paint.Op {
	t.Helper()
	maskCol := color.NRGBA{1, 2, 3, 115}
	open, cur := true, 0
	d := NewDriver(func(c *Context) {
		var id ID
		Column(c, func() {
			Row(c, func() {
				id = Button(c, "Target").ID()
			}, layout.Pad(37.3))
		}, layout.Pad(11.45))
		step := TourStep{Title: "one"}
		if target {
			step.Target = id
		}
		Tour(c, &open, &cur, []TourStep{step}).MaskColor(maskCol).GapRadius(holeR).Gap(6.3).Show()
	}, DriverOptions{Width: 800, Height: 600, Scale: scale})
	d.Settle()
	d.Settle()
	var ops []paint.Op
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Color == maskCol {
				ops = append(ops, op)
			}
		case paint.FillPath:
			if o.Color == maskCol {
				ops = append(ops, op)
			}
		}
	}
	if len(ops) == 0 {
		t.Fatal("no mask ops")
	}
	return ops
}

// TestTourMaskOpShape guards against a window-sized mask path: the GPU
// renderer cannot atlas a path larger than an atlas page and rasterizes
// it on the CPU every frame. Only the hole's small corner pieces may be
// paths, each at most 2·holeR + 2·4 (the seam grid, both sides) + 2 px.
func TestTourMaskOpShape(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, holeR := range []float32{6, 2, 0} {
			ops := tourMaskOps(t, scale, true, holeR)
			limit := 2*holeR + 2*4 + 2
			paths, segs := 0, 0
			var big float32
			for _, op := range ops {
				if fp, ok := op.(paint.FillPath); ok {
					paths++
					b := fp.Path.Bounds()
					segs += len(fp.Path.Segs)
					big = max(big, b.W, b.H)
					if b.W > limit || b.H > limit {
						t.Errorf("scale %g holeR %g: mask path %v exceeds %g px", scale, holeR, b, limit)
					}
				}
			}
			t.Logf("scale %g holeR %g: %d mask ops (%d paths, %d path segments, largest path side %g px)", scale, holeR, len(ops), paths, segs, big)
		}
		ops := tourMaskOps(t, scale, false, 6)
		for _, op := range ops {
			if _, ok := op.(paint.FillPath); ok {
				t.Errorf("scale %g no target: mask is a path, want one rect", scale)
			}
		}
		t.Logf("scale %g no target: %d mask ops", scale, len(ops))
		if len(ops) != 1 {
			t.Errorf("scale %g no target: %d mask ops, want 1", scale, len(ops))
		}
	}
}
