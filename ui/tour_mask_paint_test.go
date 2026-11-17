package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/cpuexec"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// paintTourMaskOld is the Tour mask as painted up to 9420bb8: one path of
// the whole window with the hole punched out (non-zero rule). It is the
// "before" image for TestTourMaskPixelEquivalence.
func paintTourMaskOld(l *paint.List, r, hole geom.Rect, holeR float32, hasTarget bool, col theme.Color, _ float32) {
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
func renderTourMask(useGPU bool, paintFn func(*paint.List, geom.Rect, geom.Rect, float32, bool, theme.Color, float32), win geom.Size, scale float32, tc tourMaskCase, opacity float32) *image.RGBA {
	var l paint.List
	l.Background = color.NRGBA{255, 255, 255, 255}
	full := geom.Rect{W: win.W, H: win.H}
	l.LinearStops(full, geom.Radii{}, geom.Pt(0, 0), geom.Pt(win.W, win.H),
		paint.Stop{Offset: 0, Color: color.NRGBA{250, 40, 90, 255}}, paint.Stop{Offset: 1, Color: color.NRGBA{30, 200, 240, 255}})
	l.Fill(geom.Rect{X: 20, Y: 30, W: 150, H: 60}, geom.Rad(4), color.NRGBA{255, 255, 255, 255})
	if opacity < 1 {
		l.Opacity(opacity)
	}
	paintFn(&l, full, tc.hole, tc.holeR, tc.hasTarget, color.NRGBA{0, 0, 0, 115}, scale) // antd colorBgMask rgba(0,0,0,.45)
	if opacity < 1 {
		l.Pop()
	}
	var img *image.RGBA
	if useGPU {
		dev := cpuexec.NewDevice()
		if err := gpu.NewRenderer(dev).Render(&l, win, scale); err != nil {
			panic(err)
		}
		img = dev.Target()
	} else {
		cpu := paint.NewCPU()
		if err := cpu.Render(&l, win, scale); err != nil {
			panic(err)
		}
		img = cpu.Image()
	}
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

// tourMaskScales are the device scales the mask is compared at: the
// usual desktop steps and arbitrary ones (Windows 110%, Android 420dpi
// 2.625 and tvdpi 1.33125, and an odd 1.37).
var tourMaskScales = []float32{1, 2, 1.25, 1.5, 1.1, 2.625, 1.33125, 1.37}

// tourMaskWindow is a window of about 240×188 logical px that is a whole
// number of device pixels at scale, as a real window is.
func tourMaskWindow(scale float32) geom.Size {
	return geom.Size{W: float32(math.Ceil(240*float64(scale))) / scale, H: float32(math.Ceil(188*float64(scale))) / scale}
}

// testTourMaskEquivalence renders every case before (the single path) and
// after on one renderer and fails on a per-channel difference above 1.
//
// Tolerance: 1 level. At opacity 1 the whole mask area may differ by one
// level: the renderer blends a FillRect and a FillPath of the same color
// with different rounding (the "no target" case, one rect versus one path
// of the same geometry, shows the same count). Inside an opacity layer a
// few hole-corner pixels may differ by one from float accumulation order.
// A seam between pieces at a fractional device position blends a pixel
// twice and differs by 10 or more levels at alpha .45, so it fails.
func testTourMaskEquivalence(t *testing.T, useGPU bool) {
	for _, scale := range tourMaskScales {
		win := tourMaskWindow(scale)
		for _, tc := range tourMaskCases() {
			for _, op := range []float32{1, 0.6} {
				before := renderTourMask(useGPU, paintTourMaskOld, win, scale, tc, op)
				after := renderTourMask(useGPU, paintTourMask, win, scale, tc, op)
				md, n := imageDiff(before, after)
				t.Logf("scale %-7g %-22s opacity %-3g: max diff %d, %d differing pixels", scale, tc.name, op, md, n)
				if md > 1 {
					t.Errorf("scale %g %s opacity %g: max per-channel diff %d (> 1) over %d pixels", scale, tc.name, op, md, n)
				}
			}
		}
	}
}

// TestTourMaskPixelEquivalence compares on paint.CPU.
func TestTourMaskPixelEquivalence(t *testing.T) { testTourMaskEquivalence(t, false) }

// TestTourMaskPixelEquivalenceGPU compares on the GPU renderer (CPU
// executor), where the old window-sized path is drawn as a CPU patch and
// the new corner paths come from the atlas.
func TestTourMaskPixelEquivalenceGPU(t *testing.T) { testTourMaskEquivalence(t, true) }

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
// paths, each at most 2·holeR + 2/scale (a device pixel of snap on both
// sides) + 2 px.
func TestTourMaskOpShape(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, holeR := range []float32{6, 2, 0} {
			ops := tourMaskOps(t, scale, true, holeR)
			limit := 2*holeR + 2/scale + 2
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

// TestTourMaskOpsCached: while the mask's geometry holds, its ops (and so
// the corner paths the GPU atlas is keyed by) are reused across frames;
// a geometry change rebuilds them.
func TestTourMaskOpsCached(t *testing.T) {
	maskCol := color.NRGBA{1, 2, 3, 115}
	open, cur := true, 0
	d := NewDriver(func(c *Context) {
		var id ID
		Column(c, func() {
			id = Button(c, "Target").ID()
		}, layout.Pad(37.3))
		Tour(c, &open, &cur, []TourStep{{Target: id, Title: "one"}}).MaskColor(maskCol).GapRadius(6).Show()
	}, DriverOptions{Width: 800, Height: 600})
	paths := func() []*paint.Path {
		var ps []*paint.Path
		for _, op := range d.r.List().Ops {
			if fp, ok := op.(paint.FillPath); ok && fp.Color == maskCol {
				ps = append(ps, fp.Path)
			}
		}
		if len(ps) != 4 {
			t.Fatalf("%d corner paths, want 4", len(ps))
		}
		return ps
	}
	d.Settle()
	d.Settle()
	a := paths()
	d.Frame()
	if b := paths(); b[0] != a[0] || b[3] != a[3] {
		t.Fatal("corner paths rebuilt with the same geometry")
	}
	d.Resize(700, 500)
	d.Settle()
	if b := paths(); b[0] == a[0] {
		t.Fatal("corner paths kept after a resize")
	}
}
