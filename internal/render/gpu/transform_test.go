package gpu

import (
	"fmt"
	"image"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// renderCounting draws l twice with one Renderer (cold, then warm) and
// returns the second frame's device and the patches rasterized in it.
func renderCounting(t *testing.T, l *paint.List, scale float32) (dev *cpuDevice, patches [][]paint.Op) {
	t.Helper()
	dev = &cpuDevice{}
	r := NewRenderer(dev)
	r.onPatch = func(ops []paint.Op, _ image.Rectangle) { patches = append(patches, ops) }
	for frame := 1; frame <= 2; frame++ {
		patches = nil
		if err := r.Render(l, gputest.SceneSize, scale); err != nil {
			t.Fatal(err)
		}
	}
	return dev, patches
}

// TestTransformAxisAlignedNative: translate and uniform positive scale
// transforms around every op kind (fills, strokes, dashes, shadows, text,
// paths, path gradients, rect gradients, checkers, images, clips, an
// opacity group) are drawn natively, with no patch, and match paint.CPU.
func TestTransformAxisAlignedNative(t *testing.T) {
	sc := gputest.SceneNamed("xform")
	l := sc.Make(t)
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev, patches := renderCounting(t, l, scale)
			if p, _ := groupInstances(dev.last); len(patches) != 0 || p != 0 {
				t.Fatalf("%d patches rasterized (%v), %d patch instances; want none", len(patches), patches, p)
			}
			if n := kinds(dev.last); n[shaders.KindMask] == 0 || n[shaders.KindImage] != 1 || n[shaders.KindShadow] != 1 ||
				n[shaders.KindGradient] != 2 || n[shaders.KindChecker] != 1 || n[shaders.KindStroke] != 2 {
				t.Fatalf("instances by kind %v", n)
			}
			ref := paint.NewCPU()
			ref.Render(l, gputest.SceneSize, scale)
			gputest.Check(t, sc.Name, l, scale, ref.Image(), dev.img, sc.Thr)
		})
	}
}

// TestTransformRotationFallsBack: a rotated subtree (inside a translated
// one) and a mirrored one are each one patch, which carries the
// enclosing transform; the result matches paint.CPU.
func TestTransformRotationFallsBack(t *testing.T) {
	sc := gputest.SceneNamed("xformrot")
	l := sc.Make(t)
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev, patches := renderCounting(t, l, scale)
			if len(patches) != 2 {
				t.Fatalf("%d patches, want 2 (rotated and mirrored subtrees): %v", len(patches), patches)
			}
			// The rotated subtree's patch starts with the enclosing
			// translation, then the rotation.
			if pt, ok := patches[0][0].(paint.PushTransform); !ok || pt.M != geom.Translate(30, 4) {
				t.Fatalf("first patch starts with %#v, want the enclosing Translate(30, 4)", patches[0][0])
			}
			for _, p := range patches {
				for _, op := range p {
					if f, ok := op.(paint.FillRect); ok && f.Color == gputest.Green {
						t.Fatalf("the translated (native) fill went into a patch: %v", p)
					}
				}
			}
			ref := paint.NewCPU()
			ref.Render(l, gputest.SceneSize, scale)
			gputest.Check(t, sc.Name, l, scale, ref.Image(), dev.img, sc.Thr)
		})
	}
}

// TestTransformNested: translate ∘ scale ∘ translate with a rounded clip,
// text, a path and an identity transform inside is native and matches
// paint.CPU; the transforms pop with their Pops.
func TestTransformNested(t *testing.T) {
	sc := gputest.SceneNamed("xformnest")
	l := sc.Make(t)
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev, patches := renderCounting(t, l, scale)
			if p, _ := groupInstances(dev.last); len(patches) != 0 || p != 0 {
				t.Fatalf("%d patches rasterized, %d patch instances; want none", len(patches), p)
			}
			ref := paint.NewCPU()
			ref.Render(l, gputest.SceneSize, scale)
			gputest.Check(t, sc.Name, l, scale, ref.Image(), dev.img, sc.Thr)
		})
	}
}

// TestTransformedPathMaskKey: the mask of a path drawn under a
// translation is keyed by the transform's linear part and the
// translation's subpixel phase (1/4 px), so one icon path at 50 integer
// positions is rasterized once, and at the 4×4 quarter-pixel phases at
// most 16 times; both match paint.CPU.
func TestTransformedPathMaskKey(t *testing.T) {
	icon := paint.StrokeToFill(paint.NewPath().MoveTo(1, 1).LineTo(9, 9).MoveTo(9, 1).LineTo(1, 9).
		MoveTo(5, 1).QuadTo(9, 5, 5, 9), paint.StrokeStyle{Width: 1.5, Cap: paint.CapRound})
	size := geom.Size{W: 200, H: 120}
	cases := []struct {
		name string
		at   []geom.Point
		max  int
	}{
		{"integer", func() (p []geom.Point) {
			for i := range 50 {
				p = append(p, geom.Pt(float32(i%10)*18+2, float32(i/10)*18+3))
			}
			return p
		}(), 1},
		{"phases", func() (p []geom.Point) {
			for i := range 50 {
				p = append(p, geom.Pt(float32(i%10)*18+2+float32(i%4)*0.25, float32(i/10)*18+3+float32(i/4%4)*0.25))
			}
			return p
		}(), 16},
	}
	for _, tc := range cases {
		for _, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("%s@%v", tc.name, scale), func(t *testing.T) {
				l := &paint.List{Background: white}
				for _, p := range tc.at {
					l.Transform(geom.Translate(p.X, p.Y))
					l.Path(icon, ink)
					l.Pop()
				}
				dev := &cpuDevice{}
				r := NewRenderer(dev)
				rasters := 0
				pm := r.pathRaster
				r.pathRaster = func(p *paint.Path) (*image.Alpha, image.Point) { rasters++; return pm(p) }
				var patches int
				r.onPatch = func([]paint.Op, image.Rectangle) { patches++ }
				for frame := 1; frame <= 2; frame++ {
					if err := r.Render(l, size, scale); err != nil {
						t.Fatal(err)
					}
				}
				if rasters < 1 || rasters > tc.max || patches != 0 {
					t.Fatalf("%d path masks rasterized, %d patches; want 1..%d and none", rasters, patches, tc.max)
				}
				t.Logf("%d positions: %d path masks rasterized", len(tc.at), rasters)
				if n := kinds(dev.last); n[shaders.KindMask] != len(tc.at) {
					t.Fatalf("instances %v, want %d masks", n, len(tc.at))
				}
				ref := paint.NewCPU()
				ref.Render(l, size, scale)
				gputest.Check(t, "icons", l, scale, ref.Image(), dev.img, 2)
			})
		}
	}
}

// TestOpaqueGroupNestedRoundedClip: an opacity group at alpha 1 holding a
// rounded clip inside an enclosing rounded clip is drawn through a layer
// (paint.CPU composites the group through the outer clip's coverage, so
// the outer corners must survive); it matches paint.CPU.
func TestOpaqueGroupNestedRoundedClip(t *testing.T) {
	l := &paint.List{Background: white}
	l.Clip(geom.R(4, 4, 60, 44), geom.Rad(14))
	l.Opacity(1)
	l.Clip(geom.R(0, 10, 90, 40), geom.Rad(6))
	l.Fill(geom.R(0, 0, 96, 56), geom.Radii{}, blue)
	l.Pop()
	l.Fill(geom.R(0, 0, 30, 20), geom.Radii{}, red)
	l.Pop()
	l.Pop()
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev, patches := renderCounting(t, l, scale)
			if p, c := groupInstances(dev.last); len(patches) != 0 || p != 0 || c != 1 {
				t.Fatalf("%d patches, %d patch instances, %d composites; want 0, 0, 1", len(patches), p, c)
			}
			ref := paint.NewCPU()
			ref.Render(l, gputest.SceneSize, scale)
			gputest.Check(t, "opaque group", l, scale, ref.Image(), dev.img, 2)
		})
	}
}
