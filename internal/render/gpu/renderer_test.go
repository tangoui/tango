package gpu

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"image/draw"
	"io/fs"
	"math"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/atlas"
	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// cpuDevice is cpuexec.Device (internal/render/gpu/cpuexec/device.go) as the
// package's internal tests use it. They cannot import cpuexec, which
// imports gpu, so cpuexec_bridge_test.go (package gpu_test) installs it
// through NewCPUExecDevice; cpuDevice creates it at its first use and
// mirrors its state after every Execute into the fields the tests read.
type cpuDevice struct {
	dev CPUExecDevice

	calls int
	img   *image.RGBA
	mask  *image.Alpha
	color *image.RGBA
	ramp  *image.RGBA64
	// images are the image textures by ID (ImageUploads, ImageFrees).
	images map[int]*image.RGBA
	last   *Frame // shallow copy of the last frame (tests inspect it)
	// batchUploads counts uploads applied between batches (patch page
	// flushes) over all frames.
	batchUploads int
	// liveLayers is the number of layer images still held after the last
	// Execute (each is dropped after its last use: always 0).
	liveLayers int
	// executed counts executed frames; noRetain makes Retained report
	// false (full redraws only).
	executed int
	noRetain bool
}

// CPUExecDevice is the view of cpuexec.Device the internal tests use.
type CPUExecDevice interface {
	Device
	Retainer
	PixelReader
	State() CPUExecState
}

// CPUExecState is the state of a cpuexec.Device (its counters, target,
// textures and last frame).
type CPUExecState struct {
	Calls, Executed, BatchUploads, LiveLayers int
	Img                                       *image.RGBA
	Mask                                      *image.Alpha
	Color                                     *image.RGBA
	Ramp                                      *image.RGBA64
	Images                                    map[int]*image.RGBA
	Last                                      *Frame
}

// NewCPUExecDevice returns a new cpuexec.Device and CPUExecRun is
// cpuexec.Run; cpuexec_bridge_test.go sets both.
var (
	NewCPUExecDevice func() CPUExecDevice
	CPUExecRun       func(insts []shaders.Instance, w, h int, mask, color, ramp, img image.Image, dst *image.RGBA)
)

func (d *cpuDevice) device() CPUExecDevice {
	if d.dev == nil {
		d.dev = NewCPUExecDevice()
	}
	return d.dev
}

func (d *cpuDevice) Execute(f *Frame) error {
	err := d.device().Execute(f)
	s := d.dev.State()
	d.calls, d.executed, d.batchUploads, d.liveLayers = s.Calls, s.Executed, s.BatchUploads, s.LiveLayers
	d.img, d.mask, d.color, d.ramp, d.images, d.last = s.Img, s.Mask, s.Color, s.Ramp, s.Images, s.Last
	return err
}

// Retained implements Retainer: the target persists between frames
// (unless noRetain is set).
func (d *cpuDevice) Retained(size image.Point) bool {
	return !d.noRetain && d.device().Retained(size)
}

// ReadPixels implements PixelReader.
func (d *cpuDevice) ReadPixels() (*image.RGBA, error) { return d.device().ReadPixels() }

// The scenes, colors and comparison are shared with the Metal device's
// tests (internal/render/gpu/internal/gputest).
var (
	white = gputest.White
	blue  = gputest.Blue
	red   = gputest.Red
	green = gputest.Green
	ink   = gputest.Ink

	scenes    = gputest.Scenes
	compare   = gputest.Compare
	shape     = gputest.Shape
	testImage = gputest.TestImage
)

// renderBoth draws l with paint.CPU and with Renderer + cpuexec.
func renderBoth(t *testing.T, l *paint.List, size geom.Size, scale float32) (want, got *image.RGBA, dev *cpuDevice) {
	t.Helper()
	ref := paint.NewCPU()
	if err := ref.Render(l, size, scale); err != nil {
		t.Fatal(err)
	}
	dev = &cpuDevice{}
	if err := NewRenderer(dev).Render(l, size, scale); err != nil {
		t.Fatal(err)
	}
	if dev.img == nil {
		t.Fatal("device not called")
	}
	if dev.img.Rect != ref.Image().Rect {
		t.Fatalf("size %v, want %v", dev.img.Rect, ref.Image().Rect)
	}
	return ref.Image(), dev.img, dev
}

func checkMatch(t *testing.T, name string, l *paint.List, size geom.Size, scale float32, thr int) gputest.Result {
	t.Helper()
	want, got, _ := renderBoth(t, l, size, scale)
	return gputest.Check(t, name, l, scale, want, got, thr)
}

func TestRendererMatchesCPU(t *testing.T) {
	for i, sc := range scenes {
		for j, scale := range gputest.Scales {
			t.Run(fmt.Sprintf("%s@%v", sc.Name, scale), func(t *testing.T) {
				raceSkip(t, j == i%len(gputest.Scales)) // every scene, scales in turn
				checkMatch(t, sc.Name, sc.Make(t), gputest.SceneSize, scale, sc.Thr)
			})
		}
	}
}

// TestRendererScaleChange reuses one Renderer across scales (Retina ↔ 1x).
func TestRendererScaleChange(t *testing.T) {
	l := scenes[3].Make(t) // text: the glyph cache depends on the scale
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	for _, scale := range []float32{2, 1, 2} {
		if err := r.Render(l, geom.Size{W: 96, H: 56}, scale); err != nil {
			t.Fatal(err)
		}
		ref := paint.NewCPU()
		ref.Render(l, geom.Size{W: 96, H: 56}, scale)
		if res := compare(dev.img, ref.Image(), 4); res.Frac > 0.005 {
			t.Fatalf("scale %v: %.3f%% over (max %d)", scale, res.Frac*100, res.MaxDiff)
		}
	}
}

func TestRenderZeroSize(t *testing.T) {
	l := scenes[0].Make(t)
	for _, size := range []geom.Size{{W: 0, H: 100}, {W: 100, H: 0}, {}, {W: -5, H: 10}, {W: float32(math.NaN()), H: 10}} {
		dev := &cpuDevice{}
		if err := NewRenderer(dev).Render(l, size, 2); err != nil {
			t.Fatalf("%v: %v", size, err)
		}
		if dev.calls != 0 {
			t.Fatalf("%v: Execute called %d times", size, dev.calls)
		}
	}
}

func TestUnbalancedPop(t *testing.T) {
	l := &paint.List{Background: white}
	l.Pop() // nothing pushed yet
	// Rectangular clips keep this test about Push/Pop pairing: rounded clip
	// corners are an approximation compared in TestRendererMatchesCPU.
	l.Clip(geom.R(10, 10, 40, 30), geom.Radii{})
	l.Fill(geom.R(0, 0, 96, 56), geom.Radii{}, blue)
	l.Pop()
	l.Pop() // one too many
	l.Fill(geom.R(30, 20, 50, 30), geom.Radii{}, red)
	l.Opacity(0.5) // never closed
	l.Fill(geom.R(5, 30, 30, 20), geom.Rad(4), green)
	l.Clip(geom.R(60, 5, 30, 20), geom.Radii{}) // never closed
	l.Fill(geom.R(0, 0, 96, 56), geom.Radii{}, ink)
	for _, scale := range []float32{1, 2} {
		checkMatch(t, "unbalanced", l, geom.Size{W: 96, H: 56}, scale, 2)
	}
}

func TestDeepClip(t *testing.T) {
	// 40 nested rectangular clips: exact (scissor only).
	l := &paint.List{Background: white}
	for i := range 40 {
		f := float32(i) * 0.5
		l.Clip(geom.R(f, f*0.7, 96-2*f, 56-1.4*f), geom.Radii{})
	}
	l.Fill(geom.R(0, 0, 96, 56), geom.Radii{}, blue)
	for range 40 {
		l.Pop()
	}
	l.Fill(geom.R(40, 0, 10, 56), geom.Radii{}, red)
	checkMatch(t, "deepclip", l, geom.Size{W: 96, H: 56}, 2, 2)

	// 40 nested rounded clips with every op kind inside: no panic.
	m := &paint.List{Background: white}
	for i := range 40 {
		f := float32(i)
		m.Clip(geom.R(f*0.3, f*0.2, 96-f*0.6, 56-f*0.4), geom.Rad(4+f*0.1))
	}
	m.Fill(geom.R(0, 0, 96, 56), geom.Rad(3), blue)
	m.Stroke(geom.R(10, 10, 40, 30), geom.Rad(3), 1, ink)
	m.Opacity(0.5)
	m.Fill(geom.R(5, 5, 50, 30), geom.Radii{}, red)
	m.Pop()
	dev := &cpuDevice{}
	if err := NewRenderer(dev).Render(m, geom.Size{W: 96, H: 56}, 2); err != nil {
		t.Fatal(err)
	}
	if dev.calls != 1 {
		t.Fatalf("Execute calls = %d", dev.calls)
	}
}

// TestScissorForRectClip: a single rectangular clip becomes a batch
// scissor; the instances carry no shader clip.
func TestScissorForRectClip(t *testing.T) {
	l := &paint.List{Background: white}
	l.Fill(geom.R(0, 0, 10, 10), geom.Radii{}, red)
	l.Clip(geom.R(5.2, 5.6, 40, 30), geom.Radii{})
	l.Fill(geom.R(0, 0, 96, 56), geom.Radii{}, blue)
	l.Pop()
	dev := &cpuDevice{}
	if err := NewRenderer(dev).Render(l, geom.Size{W: 96, H: 56}, 2); err != nil {
		t.Fatal(err)
	}
	b := dev.last.Batches
	if len(b) != 2 || !b[0].Scissor.Empty() || b[1].Scissor != image.Rect(10, 11, 90, 71) {
		t.Fatalf("batches %+v", b)
	}
	if in := b[1].Instances[0]; in.Clip.Z != 0 || in.Clip.W != 0 {
		t.Fatalf("rect clip leaked into the shader clip: %+v", in.Clip)
	}
}

// TestHalfTexel: every textured instance carries half a texel of the
// texture it samples in Kind.Y / Kind.Z.
func TestHalfTexel(t *testing.T) {
	l := &paint.List{Background: white}
	l.Glyphs(geom.Pt(4, 4), shape(t, "Tango 你好", 14), ink)
	l.Path(paint.NewPath().RRect(geom.R(50, 10, 30, 20), geom.Rad(3)), green)                                  // a path mask
	l.StrokePath(paint.NewPath().RRect(geom.R(2, 30, 40, 20), geom.Rad(4)), paint.StrokeStyle{Width: 1}, blue) // a path mask
	l.Transform(geom.RotateAround(0.2, geom.Pt(70, 40)))                                                       // a patch
	l.Fill(geom.R(60, 36, 20, 10), geom.Rad(2), red)
	l.Pop()
	l.Opacity(0.5) // a layer composite
	l.Fill(geom.R(50, 2, 20, 10), geom.Rad(2), red)
	l.Fill(geom.R(60, 6, 20, 10), geom.Rad(2), blue)
	l.Pop()
	dev := &cpuDevice{}
	if err := NewRenderer(dev).Render(l, geom.Size{W: 96, H: 56}, 1.5); err != nil {
		t.Fatal(err)
	}
	f := dev.last
	var masks, patches, composites int
	for _, b := range f.Batches {
		for _, in := range b.Instances {
			var tex image.Point
			switch in.Kind.X {
			case shaders.KindMask:
				tex = f.MaskSize
				masks++
			case shaders.KindPatch:
				tex = f.ColorSize
				patches++
				if in.Kind.W > 0.5 {
					tex = f.Layers[b.Layer-1].Size
					patches--
					composites++
				}
			default:
				continue
			}
			if in.Kind.Y != 0.5/float32(tex.X) || in.Kind.Z != 0.5/float32(tex.Y) {
				t.Fatalf("kind %v: half texel %v,%v for texture %v", in.Kind.X, in.Kind.Y, in.Kind.Z, tex)
			}
			// UV stays inside the texture.
			if in.UV.X < 0 || in.UV.Y < 0 || in.UV.X+in.UV.Z > 1 || in.UV.Y+in.UV.W > 1 || in.UV.Z <= 0 || in.UV.W <= 0 {
				t.Fatalf("uv %v", in.UV)
			}
		}
	}
	if masks < 8 || patches < 1 || composites != 1 {
		t.Fatalf("masks %d, patches %d, composites %d", masks, patches, composites)
	}
}

// TestHalfTexelMatters: the half-texel clamp (Kind.Y/Z) keeps every
// sample of a glyph inside its atlas entry, whatever the mask holds. Each
// glyph instance of a real frame is drawn alone, shifted by a fraction of a
// pixel so its edge pixels sample just past the entry, over two copies of
// the mask texture holding the same entry: one zero outside the entry, one
// 255 outside it. With the clamp both draw exactly the same pixels; with
// the clamp zeroed (a deliberately unclamped variant) they differ, so the
// test would catch a missing clamp. The entry content is varied (the real
// glyph mask, fully inked to its edges, a pattern) so the result does not
// depend on the text engine's masks having a transparent border; it runs
// with both text engines.
func TestHalfTexelMatters(t *testing.T) {
	t.Run("go", halfTexelMatters)
	t.Run("system", func(t *testing.T) {
		gputest.SystemText(t)
		halfTexelMatters(t)
	})
}

func halfTexelMatters(t *testing.T) {
	l := &paint.List{Background: white}
	l.Glyphs(geom.Pt(4, 4), shape(t, "Ill 你", 20), ink)
	dev := &cpuDevice{}
	if err := NewRenderer(dev).Render(l, geom.Size{W: 96, H: 40}, 1); err != nil {
		t.Fatal(err)
	}
	f := dev.last
	var glyphs []shaders.Instance
	for _, b := range f.Batches {
		for _, in := range b.Instances {
			if in.Kind.X == shaders.KindMask {
				glyphs = append(glyphs, in)
			}
		}
	}
	if len(glyphs) < 3 {
		t.Fatalf("%d glyph instances", len(glyphs))
	}
	noColor := image.NewRGBA(image.Rect(0, 0, 1, 1)) // KindMask samples only the mask
	// draw1 draws one instance onto white, only around the instance (its
	// pixels plus a margin): nothing else can change.
	draw1 := func(in shaders.Instance, mask *image.Alpha) *image.RGBA {
		r := image.Rect(int(in.Rect.X)-3, int(in.Rect.Y)-3, int(in.Rect.X+in.Rect.Z)+4, int(in.Rect.Y+in.Rect.W)+4).Intersect(image.Rect(0, 0, 96, 40))
		img := image.NewRGBA(image.Rect(0, 0, 96, 40))
		draw.Draw(img, r, image.White, image.Point{}, draw.Src)
		CPUExecRun([]shaders.Instance{in}, 96, 40, mask, noColor, nil, nil, img.SubImage(r).(*image.RGBA))
		return img
	}
	// The entry is copied into a small texture (entry at texel 8,8) so the
	// CPU sampler need not convert the whole 2048² atlas per draw; UV and
	// the half texel are recomputed for that texture as Render does.
	contents := []struct {
		name string
		fill func(src, dst image.Rectangle, tex *image.Alpha)
	}{
		{"glyph", func(src, dst image.Rectangle, tex *image.Alpha) { draw.Draw(tex, dst, dev.mask, src.Min, draw.Src) }},
		{"full", func(_, dst image.Rectangle, tex *image.Alpha) {
			draw.Draw(tex, dst, image.Opaque, image.Point{}, draw.Src)
		}},
		{"pattern", func(_, dst image.Rectangle, tex *image.Alpha) {
			for y := dst.Min.Y; y < dst.Max.Y; y++ {
				for x := dst.Min.X; x < dst.Max.X; x++ {
					tex.SetAlpha(x, y, color.Alpha{uint8(37 + (x*13+y*7)%219)})
				}
			}
		}},
	}
	leaks := 0
	for gi, in := range glyphs {
		src := uvTexels(in.UV, f.MaskSize)
		if src.Empty() || !src.In(dev.mask.Rect) {
			t.Fatalf("glyph %d: entry %v", gi, src)
		}
		n := 16
		for n < max(src.Dx(), src.Dy())+16 {
			n *= 2
		}
		dst := src.Sub(src.Min).Add(image.Pt(8, 8))
		in.UV = sl.V4(float32(dst.Min.X)/float32(n), float32(dst.Min.Y)/float32(n), float32(dst.Dx())/float32(n), float32(dst.Dy())/float32(n))
		in.Kind.Y, in.Kind.Z = 0.5/float32(n), 0.5/float32(n)
		for _, c := range contents {
			clean := image.NewAlpha(image.Rect(0, 0, n, n))
			poisoned := image.NewAlpha(clean.Rect)
			for i := range poisoned.Pix {
				poisoned.Pix[i] = 255
			}
			c.fill(src, dst, clean)
			c.fill(src, dst, poisoned)
			for _, shift := range []float32{0.3, 0.7, -0.3} {
				g := in
				g.Rect.X += shift
				g.Rect.Y += shift
				if !sameImage(draw1(g, clean), draw1(g, poisoned)) {
					t.Fatalf("glyph %d (%s, shift %v): texels outside its entry change the clamped output", gi, c.name, shift)
				}
				g.Kind.Y, g.Kind.Z = 0, 0 // unclamped
				if !sameImage(draw1(g, clean), draw1(g, poisoned)) {
					leaks++
				}
			}
		}
	}
	if leaks == 0 {
		t.Fatal("the unclamped variant never sampled outside its entry: the test cannot detect a missing Kind.Y/Z")
	}
	t.Logf("%d glyphs; unclamped draws that leaked: %d of %d", len(glyphs), leaks, len(glyphs)*len(contents)*3)
}

// uvTexels converts a normalized UV rectangle back to texels of a texture
// of size tex (exact: atlas sizes are powers of two).
func uvTexels(uv sl.Vec4, tex image.Point) image.Rectangle {
	tx, ty := float64(tex.X), float64(tex.Y)
	return image.Rect(int(math.Round(float64(uv.X)*tx)), int(math.Round(float64(uv.Y)*ty)),
		int(math.Round(float64(uv.X+uv.Z)*tx)), int(math.Round(float64(uv.Y+uv.W)*ty)))
}

func sameImage(a, b *image.RGBA) bool {
	return a.Rect == b.Rect && string(a.Pix) == string(b.Pix)
}

// TestRendererAtlasPersistent: glyph masks stay in the atlas texture across
// frames, so redrawing the same text uploads nothing.
func TestRendererAtlasPersistent(t *testing.T) {
	l := scenes[3].Make(t)
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	for i := range 2 {
		if err := r.Render(l, geom.Size{W: 96, H: 56}, 2); err != nil {
			t.Fatal(err)
		}
		masks := 0
		for _, u := range dev.last.Uploads {
			if u.Tex == TexMask {
				masks++
			}
		}
		if i == 0 && masks == 0 || i == 1 && masks != 0 {
			t.Fatalf("frame %d: %d mask uploads", i, masks)
		}
		ref := paint.NewCPU()
		ref.Render(l, geom.Size{W: 96, H: 56}, 2)
		if res := compare(dev.img, ref.Image(), 4); res.Frac > 0.005 {
			t.Fatalf("frame %d: %.3f%% over (max %d)", i, res.Frac*100, res.MaxDiff)
		}
	}
	if dev.last.MaskSize != image.Pt(2048, 2048) {
		t.Fatalf("mask size %v", dev.last.MaskSize)
	}
}

// TestRendererAtlasOverflow: large CJK text through a tiny atlas of two
// pages, each sized from the measured glyph masks so that it holds exactly
// one of them, whichever text engine rasterized them. Frames whose glyphs
// evict the previous frame's pages (the eviction count is checked), and a
// frame with more glyphs than the atlas holds (ErrFull: the rest drawn as
// patches), all match paint.CPU. It runs with both text engines.
func TestRendererAtlasOverflow(t *testing.T) {
	t.Run("go", rendererAtlasOverflow)
	t.Run("system", func(t *testing.T) {
		gputest.SystemText(t)
		rendererAtlasOverflow(t)
	})
}

func rendererAtlasOverflow(t *testing.T) {
	const scale, size = 2, 24
	frames := []string{"你好", "世界", "你好", "大字号中文"}
	// Measure every glyph's mask as the Renderer will get it.
	cpu := paint.NewCPU()
	cpu.BeginFrame(scale)
	minW, minH, maxW, maxH := 1<<30, 1<<30, 0, 0
	lists := make([]*paint.List, len(frames))
	for i, s := range frames {
		l := &paint.List{Background: white}
		gs := shape(t, s, size)
		for _, g := range gs {
			// Glyph 0 is .notdef: without a CJK font every character is
			// the same tofu glyph, so the frames share their masks and
			// nothing is evicted (Linux without fonts-noto-cjk).
			if g.ID == 0 {
				t.Skipf("%q: no font has its glyphs (.notdef); needs a CJK font", s)
			}
		}
		l.Glyphs(geom.Pt(2, 4), gs, ink)
		lists[i] = l
		for _, g := range gs {
			_, _, sub := paint.GlyphPlacement(geom.Pt(2, 4), g, scale)
			m, _ := cpu.GlyphMask(g, g.Size*scale, sub)
			if m == nil || m.Rect.Empty() {
				t.Fatalf("%q: glyph %d has no mask", s, g.ID)
			}
			minW, maxW = min(minW, m.Rect.Dx()), max(maxW, m.Rect.Dx())
			minH, maxH = min(minH, m.Rect.Dy()), max(maxH, m.Rect.Dy())
		}
	}
	// A page of side max(maxW, maxH) takes any glyph; no second glyph fits
	// beside (2*minW+gap > page) or below (2*minH+gap > page) the first.
	page := max(maxW, maxH)
	if 2*minW+1 <= page || 2*minH+1 <= page {
		t.Skipf("glyph masks from %dx%d to %dx%d: too uneven for one glyph per page", minW, minH, maxW, maxH)
	}
	t.Logf("masks %dx%d..%dx%d: page %d", minW, minH, maxW, maxH, page)

	dev := &cpuDevice{}
	r := NewRenderer(dev)
	r.atlas = atlas.New(page, 2)
	want := []struct{ evictions, masks, patches int }{
		{0, 2, 0}, // 你好 fill both pages
		{2, 2, 0}, // 世界 evict both
		{4, 2, 0}, // 你好 again: evicted before, so evict both again
		{6, 2, 3}, // 大字号中文: two evict both pages, the other three do not fit (ErrFull)
	}
	sz := geom.Size{W: 160, H: 40}
	for i, l := range lists {
		if err := r.Render(l, sz, scale); err != nil {
			t.Fatal(err)
		}
		ref := paint.NewCPU()
		ref.Render(l, sz, scale)
		res := compare(dev.img, ref.Image(), 4)
		t.Logf("frame %d %q: max diff %d, over %.3f%%, evictions %d", i, frames[i], res.MaxDiff, res.Frac*100, r.atlas.Evictions())
		if res.Frac > 0.005 {
			t.Fatalf("frame %d: %.3f%% over (max %d)", i, res.Frac*100, res.MaxDiff)
		}
		if dev.last.MaskSize != image.Pt(page, 2*page) {
			t.Fatalf("frame %d: mask size %v, want two %d pages", i, dev.last.MaskSize, page)
		}
		var masks, patches int
		for _, b := range dev.last.Batches {
			for _, in := range b.Instances {
				switch in.Kind.X {
				case shaders.KindMask:
					masks++
				case shaders.KindPatch:
					patches++
				}
			}
		}
		if got := r.atlas.Evictions(); got != want[i].evictions || masks != want[i].masks || patches != want[i].patches {
			t.Fatalf("frame %d: evictions %d, glyph instances %d, patches %d; want %d, %d, %d",
				i, got, masks, patches, want[i].evictions, want[i].masks, want[i].patches)
		}
	}
}

// TestPatchPageOverflow: patches whose total area exceeds one patch page,
// and single patches larger than the page, are drawn over several page
// fills (uploads between batches) and tiles; the texture stays one page.
func TestPatchPageOverflow(t *testing.T) {
	l := &paint.List{Background: white}
	for i := range 12 {
		x, y := float32(i%6)*15+2, float32(i/6)*25+2
		l.StrokePath(paint.NewPath().RRect(geom.R(x, y, 12, 20), geom.Rad(3)), paint.StrokeStyle{Width: 1}, blue) // patches
	}
	l.Shadows(geom.R(20, 10, 50, 30), geom.Rad(6), []paint.Shadow{{Y: 2, Blur: 8, Color: color.NRGBA{0, 0, 0, 60}}})
	rotated := func(x, y float32, c color.NRGBA) { // a patch
		l.Transform(geom.RotateAround(0.3, geom.Pt(x+8, y+6)))
		l.Fill(geom.R(x, y, 16, 12), geom.Rad(2), c)
		l.Pop()
	}
	for i := range 4 {
		rotated(float32(i)*20+4, 2, green)
	}
	// An opacity group (a layer) holding patches that flush the page,
	// between patches drawn into the target that flush it too.
	l.Opacity(0.7)
	l.Fill(geom.R(10, 5, 70, 40), geom.Rad(5), red)
	for i := range 4 {
		rotated(float32(i)*20+6, 20, blue)
	}
	l.Stroke(geom.R(30, 20, 30, 20), geom.Radii{}, 2, ink)
	l.Pop()
	for i := range 4 {
		rotated(float32(i)*20+8, 38, ink)
	}
	l.StrokePath(paint.NewPath().MoveTo(4, 50).LineTo(92, 46), paint.StrokeStyle{Width: 2}, green)
	for _, scale := range []float32{1, 2} {
		dev := &cpuDevice{}
		r := NewRenderer(dev)
		r.patchSize = 64
		if err := r.Render(l, geom.Size{W: 96, H: 56}, scale); err != nil {
			t.Fatal(err)
		}
		ref := paint.NewCPU()
		ref.Render(l, geom.Size{W: 96, H: 56}, scale)
		res := compare(dev.img, ref.Image(), 2)
		t.Logf("scale %v: max diff %d, over %.3f%%, batch uploads %d", scale, res.MaxDiff, res.Frac*100, dev.batchUploads)
		if res.Frac > 0.005 {
			t.Fatalf("scale %v: %.3f%% over (max %d)", scale, res.Frac*100, res.MaxDiff)
		}
		if dev.last.ColorSize != image.Pt(64, 64) {
			t.Fatalf("color size %v", dev.last.ColorSize)
		}
		if dev.batchUploads == 0 {
			t.Fatal("no page flush")
		}
	}

	// The default page: a gradient wider than 2048 device pixels is tiled.
	g := &paint.List{Background: white}
	g.Add(paint.LinearGradient{Rect: geom.R(2, 2, 1096, 20), P0: geom.Pt(2, 0), P1: geom.Pt(1098, 0), From: blue, To: red})
	want, got, dev := renderBoth(t, g, geom.Size{W: 1100, H: 24}, 2)
	if res := compare(got, want, 2); res.Frac > 0.005 {
		t.Fatalf("wide gradient: %.3f%% over (max %d)", res.Frac*100, res.MaxDiff)
	}
	if dev.last.ColorSize != image.Pt(2048, 2048) {
		t.Fatalf("color size %v", dev.last.ColorSize)
	}
}

// fakeGlyphs is a GlyphSource of small squares.
type fakeGlyphs struct{}

func (fakeGlyphs) GlyphPath(id uint32, size float32) *paint.Path {
	return paint.NewPath().Rect(geom.R(0, -4, 3, 4))
}

// TestPatchesKeepGlyphCache: drawing many patches in one frame must not
// age paint.CPU's glyph cache (RenderRegion used to count each patch as a
// frame and evict glyphs drawn earlier in the same frame).
func TestPatchesKeepGlyphCache(t *testing.T) {
	cpu := paint.NewCPU()
	cpu.BeginFrame(1)
	glyph := func(id int) *image.Alpha {
		m, _ := cpu.GlyphMask(paint.Glyph{Src: fakeGlyphs{}, ID: uint32(id), Size: 8}, 8, 0)
		return m
	}
	first := glyph(0)
	for i := 1; i < 4200; i++ {
		glyph(i)
	}
	op := []paint.Op{paint.StrokeRect{Rect: geom.R(1, 1, 5, 5), Width: 1, Color: ink}}
	for range 40 {
		cpu.RenderRegion(op, image.Rect(0, 0, 8, 8), 1)
	}
	if glyph(0) != first {
		t.Fatal("glyph evicted by patches of the same frame")
	}
	// Frames still age the cache.
	for range 32 {
		cpu.BeginFrame(1)
	}
	if glyph(0) == first {
		t.Fatal("BeginFrame does not age the glyph cache")
	}
}

// TestBatchReuse: batch instance slices are reused between frames.
func TestBatchReuse(t *testing.T) {
	l := scenes[2].Make(t)
	r := NewRenderer(&cpuDevice{})
	r.Render(l, geom.Size{W: 96, H: 56}, 1)
	p := &r.frame.Batches[1].Instances[0]
	r.Render(l, geom.Size{W: 96, H: 56}, 1)
	if &r.frame.Batches[1].Instances[0] != p {
		t.Fatal("instances reallocated")
	}
}

// TestMaskUploadsCoalesced: a frame inserting many new glyphs into an
// existing atlas sends a bounded number of mask uploads (row bands of the
// dirty rectangles), and the result still matches paint.CPU.
func TestMaskUploadsCoalesced(t *testing.T) {
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	size := geom.Size{W: 400, H: 60}
	first := &paint.List{Background: white}
	first.Glyphs(geom.Pt(2, 2), shape(t, "Tango", 14), ink)
	if err := r.Render(first, size, 2); err != nil {
		t.Fatal(err)
	}
	l := &paint.List{Background: white}
	l.Glyphs(geom.Pt(2, 2), shape(t, "春眠不觉晓处处闻啼鸟夜来风雨声花落知多少", 14), ink)
	l.Glyphs(geom.Pt(2, 24), shape(t, "abcdefghijklmnopqrstuvwxyz0123456789", 11), blue)
	if err := r.Render(l, size, 2); err != nil {
		t.Fatal(err)
	}
	masks := 0
	for _, u := range dev.last.Uploads {
		if u.Tex == TexMask {
			masks++
		}
	}
	t.Logf("mask uploads: %d", masks)
	if masks == 0 || masks > coalesceAbove {
		t.Fatalf("%d mask uploads, want 1..%d", masks, coalesceAbove)
	}
	ref := paint.NewCPU()
	ref.Render(l, size, 2)
	if res := compare(dev.img, ref.Image(), 4); res.Frac > 0.005 {
		t.Fatalf("%.3f%% over (max %d)", res.Frac*100, res.MaxDiff)
	}
}

func TestCoalesceRects(t *testing.T) {
	var d []image.Rectangle
	for i := range 40 {
		x, y := (i%10)*20, (i/10)*30 // four shelves of ten glyphs
		d = append(d, image.Rect(x, y, x+18, y+10+i%7))
	}
	out := coalesce(d)
	if len(out) != 4 {
		t.Fatalf("%d bands: %v", len(out), out)
	}
	for i, b := range out {
		if b != image.Rect(0, i*30, 198, i*30+16) {
			t.Fatalf("band %d = %v", i, b)
		}
	}
	few := []image.Rectangle{image.Rect(0, 0, 2, 2), image.Rect(5, 0, 7, 2)}
	if got := coalesce(few); len(got) != 2 {
		t.Fatalf("few rects coalesced: %v", got)
	}
	var many []image.Rectangle
	for i := range 40 {
		many = append(many, image.Rect(0, i*10, 5, i*10+5)) // 40 shelves
	}
	if got := coalesce(many); len(got) != 1 || got[0] != image.Rect(0, 0, 5, 395) {
		t.Fatalf("many shelves: %v", got)
	}
}

// TestRendererMatchesCPUSystemText is the parity suite for the system text
// engine (CoreText on macOS, DirectWrite, FreeType): its glyph sources
// rasterize through paint.GlyphRasterizer, and paint.CPU.GlyphMask hands
// the atlas the engine's final R8 masks with the 1px transparent border the
// atlas relies on. Every text scene matches paint.CPU at every scale, and
// every mask the scenes draw keeps that border.
func TestRendererMatchesCPUSystemText(t *testing.T) {
	gputest.SystemText(t)
	for i, sc := range gputest.TextScenes() {
		for j, scale := range gputest.Scales {
			t.Run(fmt.Sprintf("%s@%v", sc.Name, scale), func(t *testing.T) {
				raceSkip(t, j == i%len(gputest.Scales)) // every scene, scales in turn
				l := sc.Make(t)
				checkMatch(t, sc.Name, l, gputest.SceneSize, scale, sc.Thr)
				checkMaskBorders(t, l, scale)
			})
		}
	}
}

// checkMaskBorders checks that every glyph mask of l (as the Renderer gets
// it from paint.CPU.GlyphMask) has an all-zero outermost row and column on
// each side, and that at least one glyph is drawn by the system engine.
func checkMaskBorders(t *testing.T, l *paint.List, scale float32) {
	t.Helper()
	cpu := paint.NewCPU()
	cpu.BeginFrame(scale)
	sys := 0
	for _, op := range l.Ops {
		o, ok := op.(paint.Text)
		if !ok {
			continue
		}
		for _, g := range o.Glyphs {
			if g.Src == nil {
				continue
			}
			if _, ok := g.Src.(paint.GlyphRasterizer); ok {
				sys++
			}
			_, _, sub := paint.GlyphPlacement(o.Origin, g, scale)
			m, _ := cpu.GlyphMask(g, g.Size*scale, sub)
			if m == nil || m.Rect.Empty() {
				continue
			}
			w, h := m.Rect.Dx(), m.Rect.Dy()
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					if (x == 0 || y == 0 || x == w-1 || y == h-1) && m.AlphaAt(m.Rect.Min.X+x, m.Rect.Min.Y+y).A != 0 {
						t.Fatalf("glyph %d at %v px: mask %dx%d inked at its border (%d,%d)", g.ID, g.Size*scale, w, h, x, y)
					}
				}
			}
		}
	}
	if sys == 0 {
		t.Fatal("no glyph source implements paint.GlyphRasterizer: not the system engine")
	}
}

// TestRendererStrokeNative: StrokeRect ops (solid, dashed, rounded, wider
// than half the rectangle, inside a rounded clip) are KindStroke instances,
// not patches, and match paint.CPU.
func TestRendererStrokeNative(t *testing.T) {
	l := gputest.SceneNamed("strokes").Make(t)
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
			strokes := 0
			for _, b := range dev.last.Batches {
				for _, in := range b.Instances {
					switch in.Kind.X {
					case shaders.KindPatch:
						t.Fatalf("patch instance %v in a stroke-only frame", in.Rect)
					case shaders.KindStroke:
						strokes++
					}
				}
			}
			if strokes != 6 {
				t.Fatalf("%d stroke instances, want 6", strokes)
			}
			gputest.Check(t, "strokes", l, scale, want, got, 2)
		})
	}
}

// TestTransformSubtreeNotDropped: PushTransform…Pop subtrees are drawn
// like paint.CPU draws them, nested in opacity and clip groups: a uniform
// scale and a translation natively, the rotation nested in the scale as
// one patch; an identity transform is transparent.
func TestTransformSubtreeNotDropped(t *testing.T) {
	l := &paint.List{Background: white}
	l.Transform(geom.ScaleAround(0.8, 0.8, geom.Pt(30, 20)))
	l.Fill(geom.R(10, 6, 40, 28), geom.Rad(6), blue)
	l.Stroke(geom.R(10, 6, 40, 28), geom.Rad(6), 2, ink)
	l.Transform(geom.RotateAround(0.3, geom.Pt(30, 20)))
	l.Fill(geom.R(24, 14, 12, 12), geom.Radii{}, red)
	l.Pop()
	l.Pop()
	l.Opacity(0.7)
	l.Transform(geom.Translate(2.5, 1.5))
	l.Fill(geom.R(56, 30, 30, 18), geom.Rad(4), green)
	l.Pop()
	l.Pop()
	l.Transform(geom.Identity())
	l.Fill(geom.R(60, 4, 20, 12), geom.Radii{}, red)
	l.Pop()
	l.Fill(geom.R(84, 4, 8, 8), geom.Radii{}, ink) // after the identity group's Pop: no clip popped
	for _, scale := range gputest.Scales {
		want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
		gputest.Check(t, "transform", l, scale, want, got, 2)
		patches := 0
		for _, b := range dev.last.Batches {
			for _, in := range b.Instances {
				if in.Kind.X == shaders.KindPatch {
					patches++
				}
			}
		}
		if patches != 1 { // the rotated square (one tile)
			t.Fatalf("scale %v: %d patches, want the rotated group as one patch", scale, patches)
		}
	}
}

// futureOp stands for a drawing op added to paint after this renderer.
type futureOp struct{ paint.FillRect }

// PushFuture stands for a future group op (matched by a Pop).
type PushFuture struct{ paint.PushOpacity }

// TestUnknownOpsBecomePatches: drawing ops and Push* groups the renderer
// does not know are handed to paint.CPU as patches, not dropped, and an
// unknown group's Pop does not unbalance the clip stack.
func TestUnknownOpsBecomePatches(t *testing.T) {
	l := &paint.List{Background: white}
	l.Clip(geom.R(0, 0, 60, 56), geom.Radii{})
	l.Add(futureOp{paint.FillRect{Rect: geom.R(4, 4, 20, 20), Color: red}})
	l.Add(PushFuture{})
	l.Fill(geom.R(30, 4, 20, 20), geom.Radii{}, blue)
	l.Pop()
	l.Fill(geom.R(40, 30, 40, 20), geom.Radii{}, green) // still clipped at x=60
	l.Pop()
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	var patched []paint.Op
	r.onPatch = func(ops []paint.Op, region image.Rectangle) {
		if region.Empty() {
			t.Errorf("empty patch region for %T", ops[0])
		}
		patched = append(patched, ops[0])
	}
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	var sawOp, sawPush bool
	for _, op := range patched {
		switch op.(type) {
		case futureOp:
			sawOp = true
		case PushFuture:
			sawPush = true
		}
	}
	if !sawOp || !sawPush {
		t.Fatalf("patched %T; want the future op and the future group", patched)
	}
	// The green fill is still clipped by the outer clip.
	if c := dev.img.RGBAAt(70, 40); c != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("pixel outside the clip %v", c)
	}
}

func blankLike(m *image.RGBA, c color.NRGBA) *image.RGBA {
	b := image.NewRGBA(m.Rect)
	draw.Draw(b, b.Rect, image.NewUniform(c), image.Point{}, draw.Src)
	return b
}

// TestRendererHandlesEveryPaintOp: every op type in package paint (every
// type with an isOp method) has an entry in opHandling, so a new op cannot
// be silently dropped or forgotten.
func TestRendererHandlesEveryPaintOp(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "../../../ui/paint", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var ops []string
	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Name.Name != "isOp" || fd.Recv == nil || len(fd.Recv.List) != 1 {
					continue
				}
				typ := fd.Recv.List[0].Type
				if st, ok := typ.(*ast.StarExpr); ok {
					typ = st.X
				}
				if id, ok := typ.(*ast.Ident); ok {
					ops = append(ops, id.Name)
				}
			}
		}
	}
	if len(ops) < 10 {
		t.Fatalf("found only %d op types in ui/paint: %v", len(ops), ops)
	}
	for _, name := range ops {
		if _, ok := opHandling[name]; !ok {
			t.Errorf("paint.%s has no entry in gpu opHandling: classify it as opNative (and dispatch it in Render), opPatch or opHint", name)
		}
	}
	known := map[string]bool{}
	for _, op := range allPaintOps {
		known[opName(op)] = true
	}
	for _, name := range ops {
		if !known[name] {
			t.Errorf("paint.%s is missing from allPaintOps", name)
		}
	}
	// Every op the table calls native is dispatched by Render's switch:
	// one reaching the default case (a misrouted native op) would be
	// counted in misrouted (and drawn as a patch).
	for _, op := range allPaintOps {
		if opHandling[opName(op)] != opNative {
			continue
		}
		l := &paint.List{}
		l.Add(op)
		if isPush(op) {
			l.Fill(geom.R(2, 2, 4, 4), geom.Radii{}, ink)
			l.Pop()
		}
		r := NewRenderer(&cpuDevice{})
		if err := r.Render(l, geom.Size{W: 10, H: 10}, 1); err != nil {
			t.Fatal(err)
		}
		if r.misrouted != 0 {
			t.Errorf("native op paint.%s is not dispatched by Render (reached the default case)", opName(op))
		}
	}
}

// allPaintOps holds a drawable value of every paint op type
// (TestRendererHandlesEveryPaintOp checks that none is missing).
var allPaintOps = []paint.Op{
	paint.FillRect{Rect: geom.R(1, 1, 4, 4), Color: ink},
	paint.StrokeRect{Rect: geom.R(1, 1, 4, 4), Width: 1, Color: ink},
	paint.ShadowRect{Rect: geom.R(2, 2, 4, 4), Shadow: paint.Shadow{Blur: 2, Color: ink}},
	paint.FillPath{Path: paint.NewPath().RRect(geom.R(1, 1, 4, 4), geom.Rad(1)), Color: ink},
	paint.LinearGradient{Rect: geom.R(1, 1, 4, 4), P1: geom.Pt(4, 4), From: red, To: blue},
	paint.Gradient{Rect: geom.R(1, 1, 4, 4), End: geom.Pt(4, 4), Stops: []paint.Stop{{Offset: 0, Color: red}, {Offset: 1, Color: blue}}},
	paint.Checker{Rect: geom.R(1, 1, 4, 4), Cell: 2},
	paint.Text{},
	paint.Image{Rect: geom.R(1, 1, 4, 4), Img: testImage()},
	paint.BackdropBlur{Rect: geom.R(1, 1, 6, 6), Radius: 2},
	paint.PushClip{Rect: geom.R(0, 0, 8, 8)},
	paint.PushOpacity{Alpha: 0.5},
	paint.PushTransform{M: geom.Scale(2, 2)},
	paint.PushTransform{M: geom.Identity()},
	paint.Pop{},
	paint.Damage{Rect: geom.R(0, 0, 4, 4)},
	paint.ScrollRegion{},
}

// TestMisroutedNativeOpIsPatched: an op the table calls native but that
// Render's switch does not handle never panics: it is drawn as a patch (so
// it still matches paint.CPU) and counted in misrouted, which
// TestRendererHandlesEveryPaintOp requires to stay 0 for real ops.
func TestMisroutedNativeOpIsPatched(t *testing.T) {
	opHandling["futureOp"] = opNative
	defer delete(opHandling, "futureOp")
	l := &paint.List{Background: white}
	l.Add(futureOp{paint.FillRect{Rect: geom.R(4, 4, 20, 20), Color: red}})
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	patches := 0
	r.onPatch = func([]paint.Op, image.Rectangle) { patches++ }
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if r.misrouted != 1 || patches != 1 {
		t.Fatalf("misrouted %d, patches %d; want 1, 1", r.misrouted, patches)
	}
	ref := paint.NewCPU()
	ref.Render(l, gputest.SceneSize, 1)
	gputest.Check(t, "misrouted", l, 1, ref.Image(), dev.img, 2)
	if r.Render(l, gputest.SceneSize, 1); r.misrouted != 1 {
		t.Fatalf("misrouted %d after the next frame; it counts per frame", r.misrouted)
	}
}

// TestRendererShadowNative: ShadowRect ops (blurred, offset, spread, no
// blur, wider than the rect, inside a rounded clip) are KindShadow (or,
// without blur, KindSolid) instances, not patches, and match paint.CPU.
func TestRendererShadowNative(t *testing.T) {
	l := gputest.SceneNamed("shadows").Make(t)
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
			shadows, solids := 0, 0
			for _, b := range dev.last.Batches {
				for _, in := range b.Instances {
					switch in.Kind.X {
					case shaders.KindPatch:
						t.Fatalf("patch instance %v in a shadow-only frame", in.Rect)
					case shaders.KindShadow:
						shadows++
					case shaders.KindSolid:
						solids++
					}
				}
			}
			if shadows != 5 || solids != 1 {
				t.Fatalf("%d shadow and %d solid instances, want 5 and 1", shadows, solids)
			}
			gputest.Check(t, "shadows", l, scale, want, got, 2)
		})
	}
}

// TestShadowExtremes: very wide blur, negative spread (also one that
// empties the rect), radii larger than half the size, zero-size and
// inverted rects, NaN and negative blur: the GPU path matches paint.CPU
// and draws no NaN (black) blocks.
func TestShadowExtremes(t *testing.T) {
	nan := float32(math.NaN())
	black := color.NRGBA{0, 0, 0, 200}
	l := &paint.List{Background: white}
	l.Shadows(geom.R(30, 18, 30, 20), geom.Rad(6), []paint.Shadow{{Blur: 40, Color: black}})
	l.Shadows(geom.R(4, 4, 24, 20), geom.Rad(6), []paint.Shadow{{Y: 2, Blur: 6, Spread: -8, Color: black}})
	l.Shadows(geom.R(4, 30, 10, 10), geom.Rad(4), []paint.Shadow{{Blur: 4, Spread: -8, Color: black}}) // spread empties the rect
	l.Shadows(geom.R(70, 4, 20, 12), geom.Rad(50), []paint.Shadow{{Blur: 3, Color: black}})
	l.Shadows(geom.R(70, 30, 20, 12), geom.Radii{TL: 30, TR: 0, BR: 100, BL: 2}, []paint.Shadow{{Blur: 10, Spread: 3, Color: black}})
	l.Shadows(geom.R(40, 4, 0, 0), geom.Rad(4), []paint.Shadow{{Blur: 6, Color: black}})
	l.Shadows(geom.R(40, 4, -10, 8), geom.Rad(4), []paint.Shadow{{Blur: 6, Color: black}})
	l.Shadows(geom.R(20, 44, 20, 8), geom.Rad(4), []paint.Shadow{{Blur: nan, Color: black}})
	l.Shadows(geom.R(50, 44, 20, 8), geom.Rad(4), []paint.Shadow{{Blur: -5, Color: black}})
	l.Shadows(geom.R(4, 20, 8, 8), geom.Radii{TL: nan, BR: float32(math.Inf(1))}, []paint.Shadow{{Blur: 2, Color: black}})
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
			for _, b := range dev.last.Batches {
				for _, in := range b.Instances {
					for _, v := range []sl.Vec4{in.Rect, in.Radii, in.Params} {
						for _, f := range []float32{v.X, v.Y, v.Z, v.W} {
							if f != f {
								t.Fatalf("NaN in instance %+v", in)
							}
						}
					}
					if in.Kind.X == shaders.KindPatch {
						t.Fatalf("patch instance %v", in.Rect)
					}
				}
			}
			gputest.Check(t, "shadowext", l, scale, want, got, 2)
			// The 40-blur shadow is light: no pixel is darker than its
			// peak coverage allows.
			for y := 0; y < got.Rect.Dy(); y++ {
				for x := 0; x < got.Rect.Dx(); x++ {
					if c := got.RGBAAt(x, y); c.R < 20 {
						t.Fatalf("near-black pixel %v at (%d,%d)", c, x, y)
					}
				}
			}
		})
	}
}

// kinds counts the instances of each kind in f.
func kinds(f *Frame) map[float32]int {
	n := map[float32]int{}
	for _, b := range f.Batches {
		for _, in := range b.Instances {
			n[in.Kind.X]++
		}
	}
	return n
}

// rampUploads returns f's uploads to the ramp texture.
func rampUploads(f *Frame) []Upload {
	var out []Upload
	for _, u := range f.Uploads {
		if u.Tex == TexRamp {
			out = append(out, u)
		}
	}
	for _, b := range f.Batches {
		for _, u := range b.Uploads {
			if u.Tex == TexRamp {
				out = append(out, u)
			}
		}
	}
	return out
}

// TestRendererGradientNative: linear, radial and conic gradients (rounded,
// multi-stop, translucent, inside a rounded clip) and a LinearGradient are
// KindGradient instances sampling the ramp texture, not patches, and match
// paint.CPU.
func TestRendererGradientNative(t *testing.T) {
	l := gputest.SceneNamed("gradients").Make(t)
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
			n := kinds(dev.last)
			if n[shaders.KindPatch] != 0 || n[shaders.KindGradient] != 5 {
				t.Fatalf("instances by kind %v; want 5 gradients and no patches", n)
			}
			if dev.last.RampSize != image.Pt(paint.RampWidth, rampRows) {
				t.Fatalf("ramp size %v", dev.last.RampSize)
			}
			gputest.Check(t, "gradients", l, scale, want, got, 2)
		})
	}
}

// TestGradientDegenerateStops: a single stop, two stops at one offset (a
// hard edge), unsorted stops, offsets outside 0..1 and Start == End draw
// natively and match paint.CPU; gradients without stops, with non-finite
// points or an empty rect draw nothing on either side and do not panic.
func TestGradientDegenerateStops(t *testing.T) {
	nan, inf := float32(math.NaN()), float32(math.Inf(1))
	stops := []paint.Stop{{Offset: 0, Color: red}, {Offset: 1, Color: blue}}
	l := gputest.SceneNamed("gradstops").Make(t)
	l.Add(paint.Gradient{Rect: geom.R(10, 10, 20, 20)}) // no stops (List.Gradient drops it; Add does not)
	l.Add(paint.Gradient{Rect: geom.R(10, 10, 20, 20), Start: geom.Pt(nan, 0), End: geom.Pt(5, 5), Stops: stops})
	l.Add(paint.Gradient{Kind: paint.GradientRadial, Rect: geom.R(10, 10, 20, 20), End: geom.Pt(inf, 5), Stops: stops})
	l.Add(paint.Gradient{Kind: paint.GradientConic, Rect: geom.R(10, 10, 0, 20), Stops: stops})
	l.Add(paint.Gradient{Kind: paint.GradientKind(7), Rect: geom.R(40, 24, 10, 2), End: geom.Pt(50, 0), Stops: stops}) // unknown kind: linear
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
			n := kinds(dev.last)
			if n[shaders.KindPatch] != 0 || n[shaders.KindGradient] != 7 {
				t.Fatalf("instances by kind %v; want 7 gradients and no patches", n)
			}
			gputest.Check(t, "gradstops", l, scale, want, got, 2)
			// The hard edge at x = 48 stays hard: red one pixel left of it,
			// blue one pixel right (the ramp's step is 1/255 wide).
			y := int(10 * scale)
			lc, rc := got.RGBAAt(int(47*scale)-1, y), got.RGBAAt(int(49*scale)+1, y)
			if lc != red8 || rc != blue8 {
				t.Fatalf("hard edge not hard: left %v right %v", lc, rc)
			}
		})
	}
}

var (
	red8  = color.RGBA{red.R, red.G, red.B, 255}
	blue8 = color.RGBA{blue.R, blue.G, blue.B, 255}
)

// TestRendererCheckerNative: checkerboards (default colors, translucent,
// fractional cells, rounded, clipped) are KindChecker instances matching
// paint.CPU.
func TestRendererCheckerNative(t *testing.T) {
	l := gputest.SceneNamed("checker").Make(t)
	l.Add(paint.Checker{Rect: geom.R(10, 10, 10, 10), Cell: 0}) // no cells: nothing
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
			n := kinds(dev.last)
			if n[shaders.KindPatch] != 0 || n[shaders.KindChecker] != 4 {
				t.Fatalf("instances by kind %v; want 4 checkers and no patches", n)
			}
			gputest.Check(t, "checker", l, scale, want, got, 2)
		})
	}
}

// TestPathGradientNative: gradients shaped by a Path (linear area fill,
// radial, conic in a rounded clip) are KindGradient instances multiplying
// the ramp color by the path's coverage mask from the R8 atlas, not
// patches, and match paint.CPU. A second frame uploads nothing.
func TestPathGradientNative(t *testing.T) {
	l := gputest.SceneNamed("pathgrad").Make(t)
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			for frame := 1; frame <= 2; frame++ {
				if err := r.Render(l, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				if n := kinds(dev.last); n[shaders.KindPatch] != 0 || n[shaders.KindGradient] != 3 {
					t.Fatalf("frame %d: instances by kind %v; want 3 gradients and no patches", frame, n)
				}
				if frame == 2 && len(allUploads(dev.last)) != 0 {
					t.Fatalf("frame 2: uploads %v; want none", allUploads(dev.last))
				}
				ref := paint.NewCPU()
				ref.Render(l, gputest.SceneSize, scale)
				gputest.Check(t, "pathgrad", l, scale, ref.Image(), dev.img, 2)
			}
		})
	}
}

// gradientStops returns a distinct two-stop set for i.
func gradientStops(i int) []paint.Stop {
	return []paint.Stop{{Offset: 0, Color: color.NRGBA{uint8(i), uint8(i >> 8), 200, 255}}, {Offset: 1, Color: blue}}
}

// TestRampCache: each distinct stop set gets one ramp row, uploaded once;
// identical stops (also across frames) share it; with more distinct sets
// in one frame than rows, the rest are patches; new sets evict the least
// recently used rows; a failed frame re-uploads the whole ramp texture.
func TestRampCache(t *testing.T) {
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	size := geom.Size{W: 400, H: 200}
	frame := func(sets ...int) *paint.List {
		l := &paint.List{Background: white}
		for k, i := range sets {
			x, y := float32(k%40)*10, float32(k/40)*10
			l.LinearStops(geom.R(x, y, 10, 10), geom.Radii{}, geom.Pt(x, 0), geom.Pt(x+10, 0), gradientStops(i)...)
		}
		return l
	}
	render := func(l *paint.List) {
		t.Helper()
		if err := r.Render(l, size, 1); err != nil {
			t.Fatal(err)
		}
		ref := paint.NewCPU()
		ref.Render(l, size, 1)
		gputest.Check(t, "ramps", l, 1, ref.Image(), dev.img, 2)
	}
	rowOf := func() map[float32]bool {
		rows := map[float32]bool{}
		for _, b := range dev.last.Batches {
			for _, in := range b.Instances {
				if in.Kind.X == shaders.KindGradient {
					rows[in.UV.Y] = true
				}
			}
		}
		return rows
	}

	// First frame: the whole texture goes up once (nothing was sent yet).
	render(frame(1, 2, 1, 3, 2))
	if ups := rampUploads(dev.last); len(ups) != 1 || ups[0].Rect != image.Rect(0, 0, paint.RampWidth, rampRows) {
		t.Fatalf("first frame ramp uploads %v; want the whole texture", ups)
	}
	if got := len(rowOf()); got != 3 {
		t.Fatalf("%d rows for 3 distinct stop sets", got)
	}
	// Same stops again: no uploads. One new set: one row.
	render(frame(3, 2, 1))
	if ups := rampUploads(dev.last); len(ups) != 0 {
		t.Fatalf("unchanged stops uploaded %v", ups)
	}
	render(frame(1, 4))
	if ups := rampUploads(dev.last); len(ups) != 1 || ups[0].Rect.Dy() != 1 || ups[0].Rect.Dx() != paint.RampWidth {
		t.Fatalf("one new stop set uploaded %v; want one row", ups)
	}

	// More distinct sets in one frame than rows: the overflow is patched.
	var many []int
	for i := range rampRows + 44 {
		many = append(many, 1000+i)
	}
	render(frame(many...))
	if n := kinds(dev.last); n[shaders.KindGradient] != rampRows || n[shaders.KindPatch] == 0 {
		t.Fatalf("instances by kind %v; want %d gradients and patches for the rest", n, rampRows)
	}

	// LRU: after a frame using sets A, a frame of new sets evicts rows not
	// used by the most recent frames first; sets used last frame keep their
	// rows.
	var a, b []int
	for i := range 200 {
		a = append(a, 5000+i)
	}
	for i := range 56 {
		b = append(b, 6000+i)
	}
	render(frame(a...))
	before := map[float32]bool{}
	for k := range rowOf() {
		before[k] = true
	}
	render(frame(b...)) // 56 new rows: taken from the 256 rows not used by the A frame
	render(frame(a...))
	if ups := rampUploads(dev.last); len(ups) != 0 {
		t.Fatalf("A's rows were evicted (%d uploads) although less recently used rows existed", len(ups))
	}
	if after := rowOf(); len(after) != len(before) {
		t.Fatalf("A uses %d rows, before %d", len(after), len(before))
	} else {
		for k := range after {
			if !before[k] {
				t.Fatalf("A moved to row %v", k)
			}
		}
	}

	// A failed frame: the next one uploads the whole texture again.
	failing := &failDevice{}
	r2 := NewRenderer(failing)
	l := frame(1, 2)
	failing.fail = true
	if err := r2.Render(l, size, 1); err == nil {
		t.Fatal("no error")
	}
	failing.fail = false
	if err := r2.Render(l, size, 1); err != nil {
		t.Fatal(err)
	}
	if ups := rampUploads(failing.last); len(ups) != 1 || ups[0].Rect != image.Rect(0, 0, paint.RampWidth, rampRows) {
		t.Fatalf("after a failed frame ramp uploads %v; want the whole texture", ups)
	}
}

// failDevice fails Execute while fail is set.
type failDevice struct {
	cpuDevice
	fail bool
}

func (d *failDevice) Execute(f *Frame) error {
	if d.fail {
		return fmt.Errorf("device failure")
	}
	return d.cpuDevice.Execute(f)
}

// TestPointerOpsMatchValueOps: ops added by pointer (&paint.FillRect{…},
// &paint.PushClip{…} with its &paint.Pop{}, &paint.Gradient{…}) are drawn
// exactly as the same ops added by value: the Renderer dereferences them
// before dispatch (a nil pointer op draws nothing).
func TestPointerOpsMatchValueOps(t *testing.T) {
	stops := []paint.Stop{{Offset: 0, Color: red}, {Offset: 1, Color: blue}}
	vals := []paint.Op{
		paint.FillRect{Rect: geom.R(4, 4, 20, 20), Radii: geom.Rad(4), Color: red},
		paint.PushClip{Rect: geom.R(30, 2, 40, 30), Radii: geom.Rad(8)},
		paint.Gradient{Kind: paint.GradientConic, Rect: geom.R(26, 0, 50, 40), Start: geom.Pt(50, 20), End: geom.Pt(70, 20), Stops: stops},
		paint.ShadowRect{Rect: geom.R(40, 10, 20, 10), Shadow: paint.Shadow{Blur: 4, Color: ink}},
		paint.Pop{},
		paint.StrokeRect{Rect: geom.R(4, 30, 20, 20), Width: 2, Color: blue},
	}
	ptrs := []paint.Op{
		&paint.FillRect{Rect: geom.R(4, 4, 20, 20), Radii: geom.Rad(4), Color: red},
		&paint.PushClip{Rect: geom.R(30, 2, 40, 30), Radii: geom.Rad(8)},
		&paint.Gradient{Kind: paint.GradientConic, Rect: geom.R(26, 0, 50, 40), Start: geom.Pt(50, 20), End: geom.Pt(70, 20), Stops: stops},
		&paint.ShadowRect{Rect: geom.R(40, 10, 20, 10), Shadow: paint.Shadow{Blur: 4, Color: ink}},
		&paint.Pop{},
		&paint.StrokeRect{Rect: geom.R(4, 30, 20, 20), Width: 2, Color: blue},
		(*paint.FillRect)(nil),
	}
	after := paint.FillRect{Rect: geom.R(60, 36, 30, 14), Color: green} // outside the clip: the pointer Pop popped it
	for _, scale := range gputest.Scales {
		lv := &paint.List{Background: white, Ops: append(append([]paint.Op{}, vals...), after)}
		lp := &paint.List{Background: white, Ops: append(append([]paint.Op{}, ptrs...), after)}
		want, wantGPU, dv := renderBoth(t, lv, gputest.SceneSize, scale)
		dp := &cpuDevice{}
		r := NewRenderer(dp)
		if err := r.Render(lp, gputest.SceneSize, scale); err != nil {
			t.Fatal(err)
		}
		if r.misrouted != 0 {
			t.Fatalf("misrouted %d", r.misrouted)
		}
		if nv, np := kinds(dv.last), kinds(dp.last); fmt.Sprint(nv) != fmt.Sprint(np) {
			t.Fatalf("scale %v: pointer frame instances %v, value frame %v", scale, np, nv)
		}
		if !sameImage(dp.img, wantGPU) {
			t.Fatalf("scale %v: pointer ops draw differently from value ops", scale)
		}
		gputest.Check(t, "pointers", lv, scale, want, dp.img, 2)
	}
	// paint.Op values must not be modified: the list still holds pointers.
	if _, ok := ptrs[0].(*paint.FillRect); !ok {
		t.Fatal("ops changed")
	}
}

// allUploads returns every texture upload of f (frame and batch uploads,
// image textures).
func allUploads(f *Frame) []string {
	var out []string
	for _, u := range f.Uploads {
		out = append(out, fmt.Sprintf("tex%d %v", u.Tex, u.Rect))
	}
	for _, b := range f.Batches {
		for _, u := range b.Uploads {
			out = append(out, fmt.Sprintf("tex%d %v", u.Tex, u.Rect))
		}
	}
	for _, u := range f.ImageUploads {
		out = append(out, fmt.Sprintf("image %d", u.ID))
	}
	return out
}

// countGlyphMask makes r count its paint.CPU.GlyphMask calls in *n.
func countGlyphMask(r *Renderer, n *int) {
	r.glyphMask = func(g paint.Glyph, size float32, sub uint8) (*image.Alpha, image.Point) {
		*n++
		return r.cpu.GlyphMask(g, size, sub)
	}
}

// countingGlyphs is fakeGlyphs counting its GlyphPath calls.
type countingGlyphs struct{ n *int }

func (c countingGlyphs) GlyphPath(id uint32, size float32) *paint.Path {
	*c.n++
	if id == ' ' {
		return nil // empty glyph
	}
	return paint.NewPath().Rect(geom.R(0, -size*0.7, size*0.5, size*0.7))
}

// TestGlyphSingleLookup: a glyph found in the atlas is placed from its
// entry alone: redrawing the same text (spaces included) calls
// paint.CPU.GlyphMask zero times, uploads nothing and still matches
// paint.CPU. Runs with the Go and the system text engine.
func TestGlyphSingleLookup(t *testing.T) {
	t.Run("go", glyphSingleLookup)
	t.Run("system", func(t *testing.T) {
		gputest.SystemText(t)
		glyphSingleLookup(t)
	})
}

func glyphSingleLookup(t *testing.T) {
	l := &paint.List{Background: white}
	l.Glyphs(geom.Pt(3.3, 4.6), shape(t, "Tango 你好 glyphs, twice", 13), ink)
	l.Glyphs(geom.Pt(5, 30), shape(t, "a b  c", 15), blue)
	for _, scale := range gputest.Scales {
		dev := &cpuDevice{}
		r := NewRenderer(dev)
		var calls int
		countGlyphMask(r, &calls)
		for frame := 1; frame <= 2; frame++ {
			calls = 0
			if err := r.Render(l, gputest.SceneSize, scale); err != nil {
				t.Fatal(err)
			}
			if frame == 1 && calls == 0 {
				t.Fatal("frame 1: no GlyphMask calls")
			}
			if frame == 2 {
				if calls != 0 {
					t.Fatalf("@%v frame 2: %d GlyphMask calls; want 0", scale, calls)
				}
				if ups := allUploads(dev.last); len(ups) != 0 {
					t.Fatalf("@%v frame 2: uploads %v", scale, ups)
				}
			}
			ref := paint.NewCPU()
			ref.Render(l, gputest.SceneSize, scale)
			gputest.Check(t, fmt.Sprintf("single lookup frame %d", frame), l, scale, ref.Image(), dev.img, 4)
		}
	}
}

// TestGlyphCulledBeforeRaster: glyphs far outside the current scissor (a
// clip, or the target) are dropped before they are rasterized: neither
// paint.CPU.GlyphMask nor the glyph source is called for them. The visible
// glyphs still draw and match paint.CPU.
func TestGlyphCulledBeforeRaster(t *testing.T) {
	var srcCalls int
	src := countingGlyphs{&srcCalls}
	run := func(origin geom.Point, ids string) paint.Text {
		var gs []paint.Glyph
		for i, c := range ids {
			gs = append(gs, paint.Glyph{Src: src, ID: uint32(c), Size: 12, X: float32(i) * 8})
		}
		return paint.Text{Origin: origin, Glyphs: gs, Color: ink}
	}
	size := geom.Size{W: 96, H: 200}
	l := &paint.List{Background: white}
	l.Add(run(geom.Pt(4, 14), "ab"))    // visible
	l.Add(run(geom.Pt(4, 400), "cdef")) // below the target
	l.Add(run(geom.Pt(-300, 30), "gh")) // left of the target
	l.Clip(geom.R(40, 0, 50, 20), geom.Radii{})
	l.Add(run(geom.Pt(44, 14), "ij"))    // inside the clip
	l.Add(run(geom.Pt(44, 150), "klmn")) // inside the target, far below the clip
	l.Add(run(geom.Pt(44, 14), "op"))    // ...
	l.Pop()
	for _, scale := range gputest.Scales {
		dev := &cpuDevice{}
		r := NewRenderer(dev)
		var calls int
		countGlyphMask(r, &calls)
		srcCalls = 0
		if err := r.Render(l, size, scale); err != nil {
			t.Fatal(err)
		}
		if calls != 6 || srcCalls != 6 { // a b i j o p
			t.Fatalf("@%v: %d GlyphMask calls, %d GlyphPath calls; want 6 each (culled glyphs rasterized)", scale, calls, srcCalls)
		}
		if n := kinds(dev.last); n[shaders.KindMask] != 6 {
			t.Fatalf("@%v: instances %v; want 6 glyphs", scale, n)
		}
		ref := paint.NewCPU()
		ref.Render(l, size, scale)
		gputest.Check(t, "culled", l, scale, ref.Image(), dev.img, 4)
	}
}

// TestRendererFillPathNative: FillPath ops (polygons, curves, a
// self-intersecting star, a stroked path, a path partly outside the
// target, one in a rounded clip, one entirely outside) are KindMask
// instances sampling the path's coverage mask in the R8 atlas, not
// patches, and match paint.CPU. The masks are cached by Path pointer:
// the second frame rasterizes and uploads nothing.
func TestRendererFillPathNative(t *testing.T) {
	l := gputest.SceneNamed("paths").Make(t)
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			var patches int
			r.onPatch = func([]paint.Op, image.Rectangle) { patches++ }
			for frame := 1; frame <= 2; frame++ {
				if err := r.Render(l, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				if n := kinds(dev.last); patches != 0 || n[shaders.KindPatch] != 0 || n[shaders.KindMask] != 6 {
					t.Fatalf("frame %d: instances by kind %v, %d patches; want 6 masks and no patches", frame, n, patches)
				}
				if frame == 2 && len(allUploads(dev.last)) != 0 {
					t.Fatalf("frame 2: uploads %v; want none", allUploads(dev.last))
				}
				ref := paint.NewCPU()
				ref.Render(l, gputest.SceneSize, scale)
				gputest.Check(t, "paths", l, scale, ref.Image(), dev.img, 2)
			}
		})
	}
}

// TestFillPathCacheKey: the cached mask is keyed by Path pointer and
// device transform: a new Path (even with equal content) and a new scale
// rasterize again; the same Path at the same scale does not.
func TestFillPathCacheKey(t *testing.T) {
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	maskUploads := func() int {
		n := 0
		for _, u := range dev.last.Uploads {
			if u.Tex == TexMask {
				n++
			}
		}
		return n
	}
	mk := func() *paint.Path { return paint.NewPath().Circle(30, 20, 10) }
	p := mk()
	frame := func(p *paint.Path, scale float32) *paint.List {
		t.Helper()
		l := &paint.List{Background: white}
		l.Path(p, blue)
		if err := r.Render(l, gputest.SceneSize, scale); err != nil {
			t.Fatal(err)
		}
		return l
	}
	frame(p, 1)
	if n := kinds(dev.last); maskUploads() == 0 || n[shaders.KindMask] != 1 {
		t.Fatalf("first frame: %d mask uploads, instances %v; want one mask instance", maskUploads(), n)
	}
	frame(p, 1)
	if n := maskUploads(); n != 0 {
		t.Fatalf("same Path: %d mask uploads", n)
	}
	frame(mk(), 1)
	if maskUploads() == 0 {
		t.Fatal("a new Path reused the old mask")
	}
	l := frame(p, 2)
	if maskUploads() == 0 {
		t.Fatal("a new scale reused the old mask")
	}
	ref := paint.NewCPU()
	ref.Render(l, gputest.SceneSize, 2)
	gputest.Check(t, "path rescaled", l, 2, ref.Image(), dev.img, 2)
}

// TestFillPathFallbacks: a path whose mask does not fit an atlas page
// (here a tiny atlas), or that the atlas has no room for this frame, is
// drawn as a patch and still matches paint.CPU; a huge path mostly
// outside the target is not rasterized whole.
func TestFillPathFallbacks(t *testing.T) {
	l := &paint.List{Background: white}
	l.Path(paint.NewPath().Circle(20, 20, 6), blue)                       // fits the 32-texel page
	l.Path(paint.NewPath().Circle(60, 28, 20), red)                       // 40+ texels: too large
	l.Path(paint.NewPath().Circle(48, 28, 1e5), color.NRGBA{0, 0, 0, 40}) // huge
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	r.atlas = atlas.New(32, 2) // path masks get one page (maxPages-1)
	var patches int
	r.onPatch = func(_ []paint.Op, region image.Rectangle) {
		patches++
		if region.Dx() > 200 || region.Dy() > 200 {
			t.Errorf("patch region %v not limited to the target", region)
		}
	}
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if n := kinds(dev.last); patches != 2 || n[shaders.KindMask] != 1 {
		t.Fatalf("instances %v, %d patches; want 1 mask and 2 patches", n, patches)
	}
	ref := paint.NewCPU()
	ref.Render(l, gputest.SceneSize, 1)
	gputest.Check(t, "path fallbacks", l, 1, ref.Image(), dev.img, 2)
}

// staleImageDevice reports a failure one frame late like asyncFailDevice,
// but keeps its image textures (with undefined contents, here filled with
// magenta) instead of dropping them: the Renderer's re-upload then targets
// an ID the device still holds, which by the Device contract replaces that
// texture in place.
type staleImageDevice struct {
	cpuDevice
	failNext bool
}

func (d *staleImageDevice) Execute(f *Frame) error {
	if d.failNext {
		d.failNext = false
		for _, m := range d.images {
			draw.Draw(m, m.Rect, image.NewUniform(color.NRGBA{255, 0, 255, 255}), image.Point{}, draw.Src)
		}
		return fmt.Errorf("previous command buffer failed")
	}
	return d.cpuDevice.Execute(f)
}

// TestImageReuploadReplacesHeldTexture: after a failure the Renderer
// uploads an image again under the ID the device already holds (it does
// not free it first); the upload replaces the stale texture, so the frame
// matches paint.CPU.
func TestImageReuploadReplacesHeldTexture(t *testing.T) {
	l := &paint.List{Background: white}
	l.DrawImage(testImage(), geom.R(4, 4, 30, 20), geom.Radii{})
	d := &staleImageDevice{}
	r := NewRenderer(d)
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	held := imageIDs(d.last)
	d.failNext = true
	if err := r.Render(l, gputest.SceneSize, 1); err == nil {
		t.Fatal("no error")
	}
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if len(d.last.ImageUploads) != 1 || len(held) != 1 || d.last.ImageUploads[0].ID != held[0] {
		t.Fatalf("re-upload %v after holding %v; want the held ID uploaded again", imageIDs(d.last), held)
	}
	ref := paint.NewCPU()
	ref.Render(l, gputest.SceneSize, 1)
	gputest.Check(t, "replace in place", l, 1, ref.Image(), d.img, 1)
}

// groupInstances counts f's KindPatch instances: patches (sampling the
// patch page) and layer composites (Kind.W = 1, sampling a layer).
func groupInstances(f *Frame) (patches, composites int) {
	for _, b := range f.Batches {
		for _, in := range b.Instances {
			if in.Kind.X != shaders.KindPatch {
				continue
			}
			if in.Kind.W > 0.5 {
				composites++
			} else {
				patches++
			}
		}
	}
	return patches, composites
}

// TestRendererOpacityGroupNative: a PushOpacity group of several ops is
// drawn natively into an offscreen layer and composited with one KindPatch
// instance sampling the layer: no patch is rasterized by paint.CPU, the
// result matches paint.CPU, and the next frame has a layer again (layers
// last one frame).
func TestRendererOpacityGroupNative(t *testing.T) {
	l := &paint.List{Background: white}
	l.Fill(geom.R(0, 20, 96, 10), geom.Radii{}, red)
	l.Opacity(0.5)
	l.Fill(geom.R(5, 5, 50, 40), geom.Rad(6), blue)
	l.Stroke(geom.R(5, 5, 50, 40), geom.Rad(6), 2, ink)
	l.Glyphs(geom.Pt(10, 10), shape(t, "Tango 半透明", 12), white)
	l.LinearStops(geom.R(30, 25, 60, 20), geom.Rad(4), geom.Pt(30, 0), geom.Pt(90, 0),
		paint.Stop{Offset: 0, Color: green}, paint.Stop{Offset: 1, Color: red})
	l.Shadows(geom.R(60, 4, 20, 12), geom.Rad(3), []paint.Shadow{{Y: 2, Blur: 4, Color: color.NRGBA{0, 0, 0, 90}}})
	l.Pop()
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			npatch := 0
			r.onPatch = func([]paint.Op, image.Rectangle) { npatch++ }
			for frame := 1; frame <= 2; frame++ {
				if err := r.Render(l, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				patches, composites := groupInstances(dev.last)
				if npatch != 0 || patches != 0 || composites != 1 || len(dev.last.Layers) != 1 {
					t.Fatalf("frame %d: %d patches rasterized, %d patch instances, %d composites, %d layers; want 0, 0, 1, 1",
						frame, npatch, patches, composites, len(dev.last.Layers))
				}
				ref := paint.NewCPU()
				ref.Render(l, gputest.SceneSize, scale)
				gputest.Check(t, "group", l, scale, ref.Image(), dev.img, 4)
			}
		})
	}
}

// TestOpacitySingleOpFastPath: a group holding one drawing op (possibly
// inside clips) opens no layer: the op is drawn with its color scaled by
// the quantized group alpha. A group at alpha 1 draws its body directly.
// Both match paint.CPU.
func TestOpacitySingleOpFastPath(t *testing.T) {
	l := &paint.List{Background: white}
	l.Fill(geom.R(0, 20, 96, 10), geom.Radii{}, red)
	group := func(a float32, body func()) {
		l.Opacity(a)
		body()
		l.Pop()
	}
	group(0.5, func() { l.Fill(geom.R(4, 4, 30, 20), geom.Rad(5), blue) })
	group(0.3, func() { l.Stroke(geom.R(36, 4, 26, 20), geom.Rad(4), 2, ink) })
	group(0.6, func() {
		l.Shadows(geom.R(66, 6, 24, 14), geom.Rad(4), []paint.Shadow{{Y: 2, Blur: 5, Color: color.NRGBA{0, 0, 0, 120}}})
	})
	group(0.45, func() { l.Path(paint.NewPath().Circle(14, 40, 9.5), green) })
	group(0.7, func() { l.Checker(geom.R(28, 30, 20, 20), geom.Rad(3), 4, blue, white) })
	group(0.55, func() { l.Glyphs(geom.Pt(52, 30), shape(t, "W", 18), ink) })
	group(0.4, func() {
		l.Clip(geom.R(70, 28, 20, 20), geom.Rad(6))
		l.Fill(geom.R(66, 24, 30, 30), geom.Radii{}, blue)
		l.Pop()
	})
	group(1, func() { // alpha 1: no layer even with several ops
		l.Fill(geom.R(2, 50, 40, 5), geom.Radii{}, green)
		l.Fill(geom.R(20, 48, 40, 5), geom.Radii{}, blue)
	})
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
			patches, composites := groupInstances(dev.last)
			if patches != 0 || composites != 0 || len(dev.last.Layers) != 0 {
				t.Fatalf("%d patches, %d composites, %d layers; want none", patches, composites, len(dev.last.Layers))
			}
			gputest.Check(t, "fastpath", l, scale, want, got, 4)
		})
	}
}

// TestNestedOpacityGroups: three nested 0.5 groups with a rounded clip
// and text inside, in a rounded clip (gputest's opacitynest scene), match
// paint.CPU with three layers and no patches; every layer a frame names is
// drawn before it is sampled, and each frame names only its own layers.
func TestNestedOpacityGroups(t *testing.T) {
	l := gputest.SceneNamed("opacitynest").Make(t)
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			for frame := 1; frame <= 3; frame++ {
				if err := r.Render(l, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				f := dev.last
				patches, composites := groupInstances(f)
				if patches != 0 || composites != 3 || len(f.Layers) != 3 {
					t.Fatalf("frame %d: %d patches, %d composites, %d layers; want 0, 3, 3", frame, patches, composites, len(f.Layers))
				}
				if dev.liveLayers != 0 {
					t.Fatalf("frame %d: %d layers outlive the frame", frame, dev.liveLayers)
				}
				ref := paint.NewCPU()
				ref.Render(l, gputest.SceneSize, scale)
				gputest.Check(t, "opacitynest", l, scale, ref.Image(), dev.img, 4)
			}
		})
	}
}

// TestOpacityGroupGradientDither: a dithered gradient inside a 0.5 group
// whose layer starts at a device position that is not a multiple of 4
// (gputest's opacitygrad scene) is dithered at its absolute device pixels,
// as paint.CPU dithers it (the layer's origin is added back): inside the
// first group's linear gradient (away from its edges and the rectangle
// drawn over it) every pixel equals paint.CPU's; elsewhere pixels differ
// by at most 1 (antialiased edges, where paint.CPU truncates the
// composite's dst*(1-alpha) and the GPU blend rounds it).
func TestOpacityGroupGradientDither(t *testing.T) {
	l := gputest.SceneNamed("opacitygrad").Make(t)
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
			if len(dev.last.Layers) != 2 {
				t.Fatalf("%d layers, want 2", len(dev.last.Layers))
			}
			for _, d := range dev.last.Layers {
				if d.Bounds.Min.X%4 == 0 && d.Bounds.Min.Y%4 == 0 {
					t.Fatalf("layer at %v: the test needs an origin that shifts the dither pattern", d.Bounds.Min)
				}
			}
			if res := compare(got, want, 1); res.Over != 0 {
				t.Fatalf("max diff %d (%d pixels over 1)", res.MaxDiff, res.Over)
			}
			s := float64(scale)
			in := image.Rect(int(math.Ceil(13.25*s))+1, int(math.Ceil(7.5*s))+1, int(83.25*s)-1, int(25.5*s)-1)
			over := image.Rect(int(60*s)-2, 0, int(80*s)+2, 1000)
			n, diff := 0, 0
			for y := in.Min.Y; y < in.Max.Y; y++ {
				for x := in.Min.X; x < in.Max.X; x++ {
					if (image.Point{x, y}).In(over) {
						continue
					}
					n++
					if got.RGBAAt(x, y) != want.RGBAAt(x, y) {
						diff++
					}
				}
			}
			if diff != 0 {
				t.Fatalf("%d of %d gradient pixels differ from paint.CPU", diff, n)
			}
		})
	}
}
