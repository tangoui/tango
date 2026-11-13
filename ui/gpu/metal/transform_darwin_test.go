//go:build darwin

package metal

import (
	"fmt"
	"image"
	"slices"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/gpu/shaders"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// iconPage is an icon-heavy page: every icon of package icon, four times
// at different sizes, plus a row of icon buttons and inputs with icons.
// Every icon is drawn by icon.Draw inside a Translate transform.
func iconPage(c *ui.Context) {
	names := icon.Names()
	slices.Sort(names)
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Button(c, "搜索").Icon(icon.Search)
			ui.Button(c, "新建").Icon(icon.Plus).Primary()
			ui.Button(c, "用户").Icon(icon.User)
			ui.Button(c, "首页").Icon(icon.Home).Dashed()
		}, layout.Gap(8))
		for _, size := range []float32{14, 16, 20, 24} {
			ui.Row(c, func() {
				for _, n := range names {
					ui.Icon(c, icon.ByName(n)).Size(size)
				}
			}, layout.Gap(6), layout.WrapItems())
		}
	}, layout.Pad(16), layout.Gap(12))
}

// patchCounter counts the KindPatch instances (not layer composites) of
// the frames it passes on to the Metal device.
type patchCounter struct {
	d       *Device
	patches int
}

func (pc *patchCounter) Execute(f *gpu.Frame) error {
	pc.patches = 0
	for _, b := range f.Batches {
		for _, in := range b.Instances {
			if in.Kind.X == shaders.KindPatch && in.Kind.W < 0.5 {
				pc.patches++
			}
		}
	}
	return pc.d.Execute(f)
}

// TestGalleryNoPatches: the golden gallery page (whose search button
// draws its icon inside a Translate transform) and the icon page are
// drawn without any patch instance, and still match paint.CPU.
func TestGalleryNoPatches(t *testing.T) {
	d := newDevice(t)
	pc := &patchCounter{d: d}
	r := gpu.NewRenderer(pc)
	pages := []struct {
		name string
		page func(*ui.Context)
		size geom.Size
	}{
		{"gallery", galleryPage, geom.Size{W: 640, H: 300}},
		{"icons", iconPage, geom.Size{W: 1200, H: 640}},
	}
	for _, p := range pages {
		for _, th := range goldenThemes {
			for _, scale := range []float32{1, 1.5, 2} {
				drv := ui.NewDriver(p.page, ui.DriverOptions{Width: p.size.W, Height: p.size.H, Scale: scale, Theme: th.th()})
				drv.Settle()
				l := drv.Runner().List()
				transforms := 0
				for _, op := range l.Ops {
					if _, ok := op.(paint.PushTransform); ok {
						transforms++
					}
				}
				if transforms == 0 {
					t.Errorf("%s %s: no transforms on the page (icons are no longer drawn in a transform?)", p.name, th.name)
				}
				if err := r.Render(l, p.size, scale); err != nil {
					t.Fatal(err)
				}
				name := fmt.Sprintf("%s/%s@%v", p.name, th.name, scale)
				if pc.patches != 0 {
					t.Errorf("%s: %d patch instances (%d transforms), want 0", name, pc.patches, transforms)
				}
				ref := paint.NewCPU()
				if err := ref.Render(l, p.size, scale); err != nil {
					t.Fatal(err)
				}
				gputest.Check(t, name, l, scale, ref.Image(), readPixels(t, d), 4)
			}
		}
	}
}

// BenchmarkMetalIconPage renders iconPage (every icon four times, about
// 400 icons, each in a Translate transform) in the reference window.
func BenchmarkMetalIconPage(b *testing.B) {
	for _, scale := range []float32{1, 2} {
		b.Run(fmt.Sprintf("scale%v", scale), func(b *testing.B) {
			d := newDevice(b)
			drv := ui.NewDriver(iconPage, ui.DriverOptions{Width: windowSize.W, Height: windowSize.H, Scale: scale, Theme: theme.AntDesign()})
			drv.Settle()
			benchFrames(b, d, drv.Runner().List(), windowSize, scale, false)
		})
	}
}

// layerSizes records the layer sizes of the frames it passes on.
type layerSizes struct {
	d     *Device
	sizes []image.Point
}

func (ls *layerSizes) Execute(f *gpu.Frame) error {
	ls.sizes = ls.sizes[:0]
	for _, l := range f.Layers {
		ls.sizes = append(ls.sizes, l.Size)
	}
	return ls.d.Execute(f)
}

// TestMetalLayerPoolBounded: an opacity group that grows every frame for
// 60 frames and then shrinks for 60 leaves the layer pool bounded (a
// grown group's smaller textures are released, the pool's bytes stay
// within layerPoolBytes), the shrinking group reuses larger pooled
// textures (up to twice its size; its composite UVs rescaled), and every
// checked frame matches paint.CPU.
func TestMetalLayerPoolBounded(t *testing.T) {
	d := newDevice(t)
	ls := &layerSizes{d: d}
	r := gpu.NewRenderer(ls)
	size := geom.Size{W: 700, H: 420}
	const scale = 2
	larger := 0 // shrinking frames drawn into a larger texture
	maxTex := 0
	for frame := range 120 {
		g := frame
		if frame >= 60 {
			g = 119 - frame
		}
		w, h := float32(40+10*g), float32(20+6*g)
		l := &paint.List{Background: gputest.White}
		l.Fill(geom.R(0, 0, size.W, 10), geom.Radii{}, gputest.Red)
		l.Opacity(0.5)
		l.Fill(geom.R(10.5, 6, w, h), geom.Rad(8), gputest.Blue)
		l.Fill(geom.R(20, 3, w/2, h/2), geom.Radii{}, gputest.Green)
		l.Glyphs(geom.Pt(14, 12), gputest.Shape(t, "层 layer", 12), gputest.Ink)
		l.Pop()
		if err := r.Render(l, size, scale); err != nil {
			t.Fatal(err)
		}
		if len(ls.sizes) != 1 {
			t.Fatalf("frame %d: %d layers, want 1", frame, len(ls.sizes))
		}
		// Growing, each new texture releases the smaller ones; shrinking,
		// a texture is reused down to half its size, so the textures left
		// behind (until idle) are at least twice as large as one another:
		// a geometric series, bounded by the 16× range of this group.
		maxN := 2
		if frame >= 60 {
			maxN = 5
		}
		if n, b := d.layerTextures(), d.layerBytes(); n > maxN || b > layerPoolBytes {
			t.Fatalf("frame %d: %d layer textures, %d bytes; want ≤ %d and ≤ %d", frame, n, b, maxN, layerPoolBytes)
		}
		maxTex = max(maxTex, d.layerTextures())
		if frame >= 60 && d.layerSize[1] != ls.sizes[0] {
			larger++
			if ts := d.layerSize[1]; ts.X < ls.sizes[0].X || ts.Y < ls.sizes[0].Y || ts.X > 2*ls.sizes[0].X || ts.Y > 2*ls.sizes[0].Y {
				t.Fatalf("frame %d: layer of size %v in a texture of %v", frame, ls.sizes[0], ts)
			}
		}
		if frame%10 == 0 || frame == 59 || frame == 119 || frame == 61 {
			ref := paint.NewCPU()
			ref.Render(l, size, scale)
			gputest.Check(t, fmt.Sprintf("frame%d", frame), l, scale, ref.Image(), readPixels(t, d), 4)
		}
	}
	if larger == 0 {
		t.Fatal("no shrinking frame reused a larger texture")
	}
	t.Logf("%d of 60 shrinking frames drawn into a larger pooled texture; at most %d layer textures, %d bytes at the end", larger, maxTex, d.layerBytes())
}

// TestMetalLayerPoolByteCap: a frame whose sibling groups need more layer
// memory at once than layerPoolBytes still renders (the frame's textures
// are all in use), the pool is trimmed back within the cap when the next
// frame starts, and the big frame renders correctly again afterwards.
func TestMetalLayerPoolByteCap(t *testing.T) {
	d := newDevice(t)
	ls := &layerSizes{d: d}
	r := gpu.NewRenderer(ls)
	size := geom.Size{W: 1600, H: 1000}
	const scale = 2
	big := &paint.List{Background: gputest.White}
	for i := range 6 {
		big.Opacity(0.5)
		big.Fill(geom.R(float32(i), float32(i), size.W-float32(2*i), size.H-float32(2*i)), geom.Rad(20), gputest.Blue)
		big.Fill(geom.R(float32(40*i), 40, 30, 30), geom.Radii{}, gputest.Red)
		big.Pop()
	}
	small := &paint.List{Background: gputest.White}
	small.Opacity(0.5)
	small.Fill(geom.R(10, 10, 100, 60), geom.Rad(8), gputest.Blue)
	small.Fill(geom.R(50, 30, 100, 60), geom.Radii{}, gputest.Red)
	small.Pop()
	for frame, l := range []*paint.List{big, small, big} {
		if err := r.Render(l, size, scale); err != nil {
			t.Fatal(err)
		}
		need := 0
		for _, s := range ls.sizes {
			need += s.X * s.Y * 4
		}
		if l == big && need <= layerPoolBytes {
			t.Fatalf("the big frame's layers need %d bytes, want more than the cap %d", need, layerPoolBytes)
		}
		if b := d.layerBytes(); l == small && b > layerPoolBytes {
			t.Fatalf("frame %d: %d bytes of layer textures kept, want ≤ %d", frame, b, layerPoolBytes)
		}
		t.Logf("frame %d: layers need %d bytes; pool %d textures, %d bytes", frame, need, d.layerTextures(), d.layerBytes())
		ref := paint.NewCPU()
		ref.Render(l, size, scale)
		gputest.Check(t, fmt.Sprintf("frame%d", frame), l, scale, ref.Image(), readPixels(t, d), 2)
	}
}
