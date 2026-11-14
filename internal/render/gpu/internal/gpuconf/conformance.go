// Package gpuconf is the device conformance suite of the GPU renderer:
// Conformance runs the checks every gpu.Device backend (internal/render/gpu/metal,
// internal/render/gpu/d3d11, …) must pass on the same display lists, through
// gpu.Renderer and directly with hand-built gpu.Frames, comparing the
// read-back target with paint.CPU at the thresholds of package gputest.
// Backend-specific checks (presenters, texture counts, pool and scratch
// limits) stay in the backend's own tests. It is imported only by tests.
//
// It is a package of its own, not part of gputest, because it imports
// package gpu, whose in-package tests import gputest (an import cycle).
package gpuconf

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Conformance runs the device conformance suite as subtests of t. open
// returns a new, working device for the subtest it is given (skipping it
// when the platform has none, closing the device in t.Cleanup); each
// subtest opens its own. The device must implement gpu.PixelReader (every
// check reads the target back), or the subtest fails; the partial-frame
// subtests (Partial/…) run only when it implements gpu.Retainer.
//
// Options (Skip) adjust the run; a backend still being written skips the
// subtests of features it does not support yet.
//
// Besides the gpu.Frame contract the subtests check these behaviours,
// documented in package gpu (frame.go):
//   - a frame with a zero or negative Size succeeds and draws nothing;
//   - a zero MaskSize, ColorSize or RampSize is valid for a frame that
//     neither uploads to nor samples that texture;
//   - ReadPixels fails before the first frame with pixels and after a
//     failed Execute that recreated the target;
//   - a batch sampling a layer no earlier batch drew into fails Execute;
//   - a frame with Copies but no Dirty fails Execute;
//   - a partial frame on a target that does not hold the previous frame
//     fails with gpu.ErrNotRetained.
//
// Like every gputest user, the calling package pins the pure-Go text
// engine with gputest.Main in its TestMain; the SystemText subtest
// switches to the platform's engine for itself. Subtests do not run in
// parallel.
func Conformance(t *testing.T, open func(t *testing.T) gpu.Device, opts ...Option) {
	c := &suite{open: open, root: t.Name() + "/"}
	for _, o := range opts {
		o(c)
	}
	c.run(t, "Scenes", c.scenes)
	c.run(t, "TextScenes", c.textScenes)
	c.run(t, "SystemText", c.systemText)
	c.run(t, "ScaleChange", c.scaleChange)
	c.run(t, "RepeatFrames", c.repeatFrames)
	c.run(t, "ZeroSize", c.zeroSize)
	c.run(t, "BatchUploads", c.batchUploads)
	c.run(t, "ManyUploads", c.manyUploads)
	c.run(t, "RecreatedTargetHasNoFrame", c.recreatedTargetHasNoFrame)
	c.run(t, "LayerUploadOrder", c.layerUploadOrder)
	c.run(t, "LayerPoolReuse", c.layerPoolReuse)
	c.run(t, "ImageCache", c.imageCache)
	c.run(t, "GradientRampRows", c.gradientRampRows)
	c.run(t, "Partial", func(t *testing.T) {
		if _, ok := c.device(t).(gpu.Retainer); !ok {
			t.Skip("device is not a gpu.Retainer: no partial frames")
		}
		c.run(t, "Scenes", c.partialScenes)
		c.run(t, "ScrollRegion", c.scrollRegion)
		c.run(t, "VerifyCatchesWrongDamage", c.verifyCatchesWrongDamage)
		c.run(t, "NotRetained", c.notRetained)
		c.run(t, "SubpixelScroll", c.subpixelScroll)
		c.run(t, "VerifyFractionalScroll", c.verifyFractionalScroll)
	})
}

// An Option adjusts a Conformance run.
type Option func(*suite)

// Skip skips every subtest (at any depth) for which skip returns true.
// skip gets the subtest's name relative to the test calling Conformance,
// as its path of subtest names joined with "/" — for example "Scenes",
// "Scenes/images@1.5", "Partial/Scenes/image@2", "LayerUploadOrder" — and
// is asked at the start of every subtest, before it opens a device, so
// skipping a parent skips its children. Several Skip options combine: a
// subtest is skipped when any of them says so.
func Skip(skip func(name string) bool) Option {
	return func(c *suite) { c.skips = append(c.skips, skip) }
}

type suite struct {
	open  func(t *testing.T) gpu.Device
	root  string // the calling test's name and "/"
	skips []func(name string) bool
}

// run runs f as the subtest name of t, skipping it when a Skip option
// asks to.
func (c *suite) run(t *testing.T, name string, f func(t *testing.T)) bool {
	t.Helper()
	return t.Run(name, func(t *testing.T) {
		rel := strings.TrimPrefix(t.Name(), c.root)
		for _, skip := range c.skips {
			if skip(rel) {
				t.Skipf("skipped by gpuconf.Skip: %s", rel)
			}
		}
		f(t)
	})
}

// device opens a device for t and checks it reads its target back.
func (c *suite) device(t *testing.T) gpu.Device {
	t.Helper()
	d := c.open(t)
	if d == nil {
		t.Fatal("open returned a nil device")
	}
	if _, ok := d.(gpu.PixelReader); !ok {
		t.Fatalf("%T does not implement gpu.PixelReader: the conformance suite reads the target back", d)
	}
	return d
}

func readPixels(t *testing.T, d gpu.Device) *image.RGBA {
	t.Helper()
	img, err := d.(gpu.PixelReader).ReadPixels()
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// renderScenes draws scenes at every scale through one Renderer on one
// device (so textures, the glyph atlas and the render target are reused
// across frames as in an app) and compares each frame with paint.CPU.
func (c *suite) renderScenes(t *testing.T, scenes []gputest.Scene) {
	d := c.device(t)
	r := gpu.NewRenderer(d)
	for _, scale := range gputest.Scales {
		for _, sc := range scenes {
			c.run(t, fmt.Sprintf("%s@%v", sc.Name, scale), func(t *testing.T) {
				l := sc.Make(t)
				ref := paint.NewCPU()
				if err := ref.Render(l, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				if err := r.Render(l, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				gputest.Check(t, sc.Name, l, scale, ref.Image(), readPixels(t, d), sc.Thr)
			})
		}
	}
}

// scenes: the shared parity scenes at every scale match paint.CPU, with
// the thresholds of TestRendererMatchesCPU.
func (c *suite) scenes(t *testing.T) { c.renderScenes(t, gputest.Scenes) }

// textScenes: the text scenes (textmix included) with the pure-Go engine.
func (c *suite) textScenes(t *testing.T) { c.renderScenes(t, gputest.TextScenes()) }

// systemText: the text scenes with the system text engine (CoreText,
// DirectWrite, …), whose glyph masks come through paint.GlyphRasterizer
// (1px transparent border included).
func (c *suite) systemText(t *testing.T) {
	gputest.SystemText(t)
	c.renderScenes(t, gputest.TextScenes())
}

// solidAndTextList is the solid parity scene with the text and textmix
// scenes drawn on top.
func solidAndTextList(t *testing.T) *paint.List {
	l := gputest.SceneNamed("solid").Make(t)
	for _, sc := range gputest.TextScenes() {
		if sc.Name == "text" || sc.Name == "textmix" {
			l.Ops = append(l.Ops, sc.Make(t).Ops...)
		}
	}
	return l
}

// scaleChange: one device and one Renderer draw the same scene at scale
// 2, then 1, then 2 again (a window dragged from a Retina screen to a 1x
// screen and back). The render target is recreated at the new size and
// the glyph atlas, rasterized at device size, is rebuilt for the new
// scale (a stale atlas sampled at the other scale would draw wrong
// glyphs).
func (c *suite) scaleChange(t *testing.T) {
	d := c.device(t)
	r := gpu.NewRenderer(d)
	l := solidAndTextList(t)
	for i, scale := range []float32{2, 1, 2} {
		ref := paint.NewCPU()
		if err := ref.Render(l, gputest.SceneSize, scale); err != nil {
			t.Fatal(err)
		}
		if err := r.Render(l, gputest.SceneSize, scale); err != nil {
			t.Fatalf("frame %d (scale %v): %v", i, scale, err)
		}
		got := readPixels(t, d)
		if want := ref.Image().Rect; got.Rect != want {
			t.Fatalf("frame %d (scale %v): target %v, want %v", i, scale, got.Rect, want)
		}
		gputest.Check(t, fmt.Sprintf("frame%d", i), l, scale, ref.Image(), got, 4)
	}
}

// repeatFrames renders the same text twice (the second frame uploads no
// glyphs and must still sample the atlas uploaded by the first) and a
// different scene at another size in between, checking each frame.
func (c *suite) repeatFrames(t *testing.T) {
	d := c.device(t)
	r := gpu.NewRenderer(d)
	text, card := gputest.SceneNamed("text"), gputest.SceneNamed("card")
	for i, step := range []struct {
		sc   gputest.Scene
		size geom.Size
	}{{text, gputest.SceneSize}, {text, gputest.SceneSize}, {card, geom.Size{W: 120, H: 70}}, {text, gputest.SceneSize}} {
		l := step.sc.Make(t)
		ref := paint.NewCPU()
		if err := ref.Render(l, step.size, 2); err != nil {
			t.Fatal(err)
		}
		if err := r.Render(l, step.size, 2); err != nil {
			t.Fatal(err)
		}
		gputest.Check(t, fmt.Sprintf("frame%d-%s", i, step.sc.Name), l, 2, ref.Image(), readPixels(t, d), step.sc.Thr)
	}
}

// zeroSize: frames without pixels draw nothing and succeed, through the
// Renderer and when executed directly; ReadPixels before any frame with
// pixels fails; a real frame afterwards works.
func (c *suite) zeroSize(t *testing.T) {
	d := c.device(t)
	r := gpu.NewRenderer(d)
	l := gputest.SceneNamed("solid").Make(t)
	for _, size := range []geom.Size{{W: 0, H: 100}, {W: 100, H: 0}, {}, {W: -5, H: 10}} {
		if err := r.Render(l, size, 2); err != nil {
			t.Fatalf("%v: %v", size, err)
		}
	}
	for _, sz := range []image.Point{{}, {X: 0, Y: 10}, {X: 10, Y: 0}, {X: -3, Y: 4}} {
		if err := d.Execute(&gpu.Frame{Size: sz, MaskSize: image.Pt(4, 4), ColorSize: image.Pt(4, 4)}); err != nil {
			t.Fatalf("%v: %v", sz, err)
		}
	}
	if _, err := d.(gpu.PixelReader).ReadPixels(); err == nil {
		t.Fatal("ReadPixels before any frame: want an error")
	}
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if img := readPixels(t, d); img.Rect != image.Rect(0, 0, 96, 56) {
		t.Fatalf("size %v", img.Rect)
	}
}

// SolidPatch is an opaque w×h RGBA upload of c to the patch page at its
// origin.
func SolidPatch(w, h int, c color.RGBA) gpu.Upload {
	pix := make([]byte, w*h*4)
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = c.R, c.G, c.B, c.A
	}
	return gpu.Upload{Tex: gpu.TexColor, Rect: image.Rect(0, 0, w, h), Pix: pix, Stride: w * 4}
}

// PatchInstance draws the patch page's top-left w×h texels (of a tex-sized
// page) at x, y.
func PatchInstance(x, y, w, h float32, tex image.Point) shaders.Instance {
	tw, th := float32(tex.X), float32(tex.Y)
	return shaders.Instance{
		Rect:  sl.V4(x, y, w, h),
		Color: sl.V4(1, 1, 1, 1),
		UV:    sl.V4(0, 0, w/tw, h/th),
		Kind:  sl.V4(shaders.KindPatch, 0.5/tw, 0.5/th, 0),
	}
}

// batchUploads: an upload attached to a later batch overwrites texels an
// earlier batch sampled; the earlier batch must still show the old
// contents (uploads and draws in command order), and the scissor limits
// the second batch. The next frame reuses the texture with a smaller
// target and no batches.
func (c *suite) batchUploads(t *testing.T) {
	d := c.device(t)
	tex := image.Pt(8, 8)
	red, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}
	f := &gpu.Frame{
		Size:      image.Pt(32, 16),
		Clear:     color.NRGBA{255, 255, 255, 255},
		MaskSize:  image.Pt(4, 4),
		ColorSize: tex,
		Uploads:   []gpu.Upload{SolidPatch(8, 8, red)},
		Batches: []gpu.Batch{
			{Instances: []shaders.Instance{PatchInstance(0, 0, 8, 8, tex)}},
			{
				Uploads:   []gpu.Upload{SolidPatch(8, 8, blue)},
				Instances: []shaders.Instance{PatchInstance(16, 0, 8, 8, tex)},
				Scissor:   image.Rect(16, 0, 20, 16),
			},
		},
	}
	if err := d.Execute(f); err != nil {
		t.Fatal(err)
	}
	img := readPixels(t, d)
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{{3, 3, red}, {17, 3, blue}, {21, 3, color.RGBA{255, 255, 255, 255}}, {10, 3, color.RGBA{255, 255, 255, 255}}} {
		if got := img.RGBAAt(c.x, c.y); got != c.want {
			t.Errorf("pixel %d,%d = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	f2 := &gpu.Frame{Size: image.Pt(5, 3), Clear: color.NRGBA{10, 20, 30, 255}, MaskSize: f.MaskSize, ColorSize: tex}
	if err := d.Execute(f2); err != nil {
		t.Fatal(err)
	}
	img = readPixels(t, d)
	if img.Rect != image.Rect(0, 0, 5, 3) || img.RGBAAt(4, 2) != (color.RGBA{10, 20, 30, 255}) {
		t.Fatalf("cleared frame %v %v", img.Rect, img.RGBAAt(4, 2))
	}
}

// manyUploads: hundreds of tiny mask uploads in one frame (the glyph
// atlas's dirty rectangles are not coalesced) land where they should.
func (c *suite) manyUploads(t *testing.T) {
	d := c.device(t)
	mask := image.Pt(256, 64)
	atlasPix := make([]byte, mask.X*mask.Y)
	f := &gpu.Frame{Size: image.Pt(256, 64), Clear: color.NRGBA{255, 255, 255, 255}, MaskSize: mask, ColorSize: image.Pt(4, 4)}
	// A new texture's contents are undefined: the whole (empty) atlas
	// first, as the Renderer sends it, then the dirty rectangles.
	f.Uploads = append(f.Uploads, gpu.Upload{Tex: gpu.TexMask, Rect: image.Rectangle{Max: mask}, Pix: make([]byte, len(atlasPix)), Stride: mask.X})
	for i := range 500 {
		x, y := (i%64)*4, (i/64)*8
		r := image.Rect(x, y, x+3, y+5)
		for yy := r.Min.Y; yy < r.Max.Y; yy++ {
			for xx := r.Min.X; xx < r.Max.X; xx++ {
				atlasPix[yy*mask.X+xx] = 255
			}
		}
		f.Uploads = append(f.Uploads, gpu.Upload{Tex: gpu.TexMask, Rect: r, Pix: atlasPix[y*mask.X+x:], Stride: mask.X})
	}
	tw, th := float32(mask.X), float32(mask.Y)
	f.Batches = []gpu.Batch{{Instances: []shaders.Instance{{
		Rect:  sl.V4(0, 0, tw, th),
		Color: sl.V4(0, 0, 0, 1),
		UV:    sl.V4(0, 0, 1, 1),
		Kind:  sl.V4(shaders.KindMask, 0.5/tw, 0.5/th, 0),
	}}}}
	if err := d.Execute(f); err != nil {
		t.Fatal(err)
	}
	img := readPixels(t, d)
	for y := range mask.Y {
		for x := range mask.X {
			want := uint8(255 - atlasPix[y*mask.X+x])
			if got := img.RGBAAt(x, y).R; got != want {
				t.Fatalf("pixel %d,%d = %d, want %d", x, y, got, want)
			}
		}
	}
}

// recreatedTargetHasNoFrame: when Execute recreates the render target and
// then fails, the new texture's contents are undefined, so ReadPixels
// must not return them.
func (c *suite) recreatedTargetHasNoFrame(t *testing.T) {
	d := c.device(t)
	ok := &gpu.Frame{Size: image.Pt(8, 8), Clear: color.NRGBA{1, 2, 3, 255}, MaskSize: image.Pt(4, 4), ColorSize: image.Pt(4, 4)}
	if err := d.Execute(ok); err != nil {
		t.Fatal(err)
	}
	readPixels(t, d)
	bad := &gpu.Frame{Size: image.Pt(16, 8), MaskSize: ok.MaskSize, ColorSize: ok.ColorSize,
		Uploads: []gpu.Upload{{Tex: 7, Rect: image.Rect(0, 0, 1, 1), Pix: []byte{0}, Stride: 1}}}
	if err := d.Execute(bad); err == nil {
		t.Fatal("Execute with an upload to an unknown texture: want an error")
	}
	if img, err := d.(gpu.PixelReader).ReadPixels(); err == nil {
		t.Fatalf("ReadPixels after a failed frame that recreated the target returned a %v image", img.Rect)
	}
}

// LayerComposite is a composite of layer ld (KindPatch, Kind.W = 1)
// covering its bounds with opacity a.
func LayerComposite(ld gpu.LayerDesc, a float32) shaders.Instance {
	b := ld.Bounds
	tw, th := float32(ld.Size.X), float32(ld.Size.Y)
	return shaders.Instance{
		Rect:  sl.V4(float32(b.Min.X), float32(b.Min.Y), float32(b.Dx()), float32(b.Dy())),
		Color: sl.V4(a, a, a, a),
		UV:    sl.V4(0, 0, float32(b.Dx())/tw, float32(b.Dy())/th),
		Kind:  sl.V4(shaders.KindPatch, 0.5/tw, 0.5/th, 1),
	}
}

// layerUploadOrder: layers drawn from the patch page keep the page
// contents of their place in the batch order when a later batch
// re-uploads the page (red into layer 1, then green into layer 2); a
// layer starts transparent, its scissor is in main-target coordinates,
// its instances are placed relative to its origin, and a layer nested in
// another (3 into 2) is composited before its parent. Sampling a layer
// nothing drew into is an error.
func (c *suite) layerUploadOrder(t *testing.T) {
	d := c.device(t)
	tex := image.Pt(8, 8)
	red, green, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}
	white := color.RGBA{255, 255, 255, 255}
	l1 := gpu.LayerDesc{Size: image.Pt(64, 64), Bounds: image.Rect(2, 2, 10, 10)}
	l2 := gpu.LayerDesc{Size: image.Pt(64, 64), Bounds: image.Rect(13, 3, 29, 11)}
	l3 := gpu.LayerDesc{Size: image.Pt(16, 16), Bounds: image.Rect(21, 3, 29, 11)}
	solid := func(x, y, w, h float32, c color.RGBA) shaders.Instance {
		return shaders.Instance{Rect: sl.V4(x, y, w, h), Color: sl.V4(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, 1)}
	}
	f := &gpu.Frame{
		Size:      image.Pt(32, 16),
		Clear:     color.NRGBA{255, 255, 255, 255},
		MaskSize:  image.Pt(4, 4),
		ColorSize: tex,
		Layers:    []gpu.LayerDesc{l1, l2, l3},
		Uploads:   []gpu.Upload{SolidPatch(8, 8, red)},
		Batches: []gpu.Batch{
			{Instances: []shaders.Instance{solid(0, 12, 32, 4, blue)}},
			{Target: 1, Instances: []shaders.Instance{PatchInstance(2, 2, 8, 8, tex)}, Scissor: image.Rect(2, 2, 8, 10)},
			{Layer: 1, Instances: []shaders.Instance{LayerComposite(l1, 1)}},
			{Target: 2, Uploads: []gpu.Upload{SolidPatch(8, 8, green)}, Instances: []shaders.Instance{PatchInstance(13, 3, 8, 8, tex)}},
			{Target: 3, Instances: []shaders.Instance{solid(21, 3, 8, 8, blue)}},
			{Target: 2, Layer: 3, Instances: []shaders.Instance{LayerComposite(l3, 1)}},
			{Layer: 2, Instances: []shaders.Instance{LayerComposite(l2, 1)}},
		},
	}
	for frame := range 2 {
		if err := d.Execute(f); err != nil {
			t.Fatal(err)
		}
		img := readPixels(t, d)
		for _, c := range []struct {
			x, y int
			want color.RGBA
		}{
			{3, 3, red}, {7, 9, red}, {8, 5, white}, // scissor ends at x 8
			{1, 1, white}, {11, 5, white},
			{14, 4, green}, {20, 10, green}, {21, 4, blue}, {28, 10, blue},
			{20, 2, white}, {5, 13, blue},
		} {
			if got := img.RGBAAt(c.x, c.y); got != c.want {
				t.Errorf("frame %d: pixel %d,%d = %v, want %v", frame, c.x, c.y, got, c.want)
			}
		}
		f.Uploads[0] = SolidPatch(8, 8, red)
	}
	bad := *f
	bad.Batches = []gpu.Batch{{Layer: 1, Instances: []shaders.Instance{LayerComposite(l1, 1)}}}
	if err := d.Execute(&bad); err == nil {
		t.Fatal("sampling an undrawn layer: no error")
	}
}

// layerPoolReuse: ten frames each of the nested-groups scene (three
// layers) and of the two-group gradient scene, drawn by one Renderer,
// stay correct while the device reuses its layer textures. (Whether the
// number of layer textures stays bounded is the backend's own test.)
func (c *suite) layerPoolReuse(t *testing.T) {
	d := c.device(t)
	r := gpu.NewRenderer(d)
	for _, name := range []string{"opacitynest", "opacitygrad"} {
		sc := gputest.SceneNamed(name)
		l := sc.Make(t)
		ref := paint.NewCPU()
		if err := ref.Render(l, gputest.SceneSize, 2); err != nil {
			t.Fatal(err)
		}
		for frame := range 10 {
			if err := r.Render(l, gputest.SceneSize, 2); err != nil {
				t.Fatal(err)
			}
			if frame == 0 || frame == 9 {
				gputest.Check(t, fmt.Sprintf("%s-frame%d", name, frame), l, 2, ref.Image(), readPixels(t, d), sc.Thr)
			}
		}
	}
}

// imageCache: image textures live across frames on the device: a frame
// redrawing cached images uploads nothing and still matches paint.CPU;
// images alternate between batches (each binding its own texture); an
// image changed in place and invalidated is drawn from its new pixels.
func (c *suite) imageCache(t *testing.T) {
	d := c.device(t)
	r := gpu.NewRenderer(d)
	a := image.NewRGBA(image.Rect(0, 0, 10, 8))
	for i := range a.Pix {
		a.Pix[i] = uint8(i * 3)
	}
	b := gputest.TestImage()
	size := geom.Size{W: 96, H: 40}
	draw := func(name string, imgs ...image.Image) {
		t.Helper()
		l := &paint.List{Background: gputest.White}
		for i, img := range imgs {
			l.DrawImage(img, geom.R(float32(2+i*18), 4.5, 16, 30), geom.Rad(3))
		}
		ref := paint.NewCPU()
		if err := ref.Render(l, size, 2); err != nil {
			t.Fatal(err)
		}
		if err := r.Render(l, size, 2); err != nil {
			t.Fatal(err)
		}
		gputest.Check(t, name, l, 2, ref.Image(), readPixels(t, d), 2)
	}
	draw("first", a, b, a, b)
	draw("cached", a, b, a, b)
	for i := range a.Pix {
		a.Pix[i] = 255 - uint8(i)
	}
	r.InvalidateImage(a)
	draw("invalidated", b, a)
	draw("again", a, b)
}

// gradientRampRows: gradients over many frames with changing stop sets
// (more distinct sets in all than the ramp texture has rows, so rows are
// evicted and rewritten by row uploads) keep matching paint.CPU: each
// frame samples the rows it uploaded, never stale ones.
func (c *suite) gradientRampRows(t *testing.T) {
	d := c.device(t)
	r := gpu.NewRenderer(d)
	size := geom.Size{W: 200, H: 100}
	for frame := range 6 {
		l := &paint.List{Background: gputest.White}
		for k := range 120 {
			i := frame*60 + k // half of each frame's sets are new
			x, y := float32(k%20)*10, float32(k/20)*10
			l.Gradient(paint.Gradient{Kind: paint.GradientKind(k % 3), Rect: geom.R(x, y, 10, 10), Radii: geom.Rad(2),
				Start: geom.Pt(x+2, y+3), End: geom.Pt(x+9, y+8),
				Stops: []paint.Stop{{Offset: 0, Color: color.NRGBA{uint8(i * 7), uint8(i >> 2), 90, 255}}, {Offset: 1, Color: color.NRGBA{20, uint8(i * 3), 200, uint8(128 + i%128)}}}})
		}
		ref := paint.NewCPU()
		if err := ref.Render(l, size, 1.5); err != nil {
			t.Fatal(err)
		}
		if err := r.Render(l, size, 1.5); err != nil {
			t.Fatal(err)
		}
		gputest.Check(t, fmt.Sprintf("ramps%d", frame), l, 1.5, ref.Image(), readPixels(t, d), 2)
	}
}

// LastFrame forwards to a device (its gpu.Retainer and gpu.PixelReader
// included, so the Renderer redraws partially and verifies) and keeps the
// last frame's number of Dirty rectangles and its Copies.
//
// In verify mode the Renderer follows every partial frame with a full one
// (Renderer.SetVerify), so Dirty and Copies then describe that full frame;
// PartialCopies keeps the Copies of the last partial frame (one with
// Dirty rectangles), which the verification frame does not replace.
type LastFrame struct {
	gpu.Device
	Dirty         int
	Copies        []gpu.CopyRegion
	PartialCopies []gpu.CopyRegion
}

// Execute implements gpu.Device.
func (lf *LastFrame) Execute(f *gpu.Frame) error {
	lf.Dirty, lf.Copies = len(f.Dirty), append(lf.Copies[:0], f.Copies...)
	if len(f.Dirty) > 0 {
		lf.PartialCopies = append(lf.PartialCopies[:0], f.Copies...)
	}
	return lf.Device.Execute(f)
}

// Retained implements gpu.Retainer (false when the device is none).
func (lf *LastFrame) Retained(size image.Point) bool {
	rt, ok := lf.Device.(gpu.Retainer)
	return ok && rt.Retained(size)
}

// ReadPixels implements gpu.PixelReader.
func (lf *LastFrame) ReadPixels() (*image.RGBA, error) {
	pr, ok := lf.Device.(gpu.PixelReader)
	if !ok {
		return nil, fmt.Errorf("gpuconf: %T does not read pixels", lf.Device)
	}
	return pr.ReadPixels()
}

// partialScenes: every parity scene redrawn partially (two damaged
// rectangles) equals paint.CPU within the scene's threshold, and verify
// mode (comparing with the full frame on the device) accepts it.
func (c *suite) partialScenes(t *testing.T) {
	d := c.device(t)
	lf := &LastFrame{Device: d}
	r := gpu.NewRenderer(lf)
	r.SetVerify(true)
	for _, scale := range []float32{1, 2} {
		for _, sc := range gputest.Scenes {
			c.run(t, fmt.Sprintf("%s@%v", sc.Name, scale), func(t *testing.T) {
				l := sc.Make(t)
				if err := r.Render(l, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				l2 := &paint.List{Background: l.Background, Ops: append(append([]paint.Op(nil), l.Ops...),
					paint.Damage{Rect: geom.R(5, 20, 50, 12)}, paint.Damage{Rect: geom.R(60, 3, 20, 50)})}
				r.SetVerify(false)
				if err := r.Render(l2, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				if lf.Dirty != 2 {
					t.Fatalf("%d dirty rectangles, want 2", lf.Dirty)
				}
				ref := paint.NewCPU()
				if err := ref.Render(l, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				gputest.Check(t, sc.Name, l, scale, ref.Image(), readPixels(t, d), sc.Thr)
				r.SetVerify(true)
				if err := r.Render(l2, gputest.SceneSize, scale); err != nil {
					t.Fatal(err) // partial equals the full frame
				}
			})
		}
	}
}

// Scroll scenes: a window-sized list of rows scrolled inside a clipped
// viewport, between a header and a footer.
var (
	ScrollViewport = geom.R(0, 24, 320, 192)
	ScrollSize     = geom.Size{W: 320, H: 480}
)

// ScrollList is the scroll scene with its rows scrolled by off pixels;
// extra, when not nil, adds ops (hints) at the end.
func ScrollList(t testing.TB, off float32, extra func(*paint.List)) *paint.List {
	l := &paint.List{Background: gputest.White}
	l.Fill(geom.R(0, 0, 320, 24), geom.Radii{}, color.NRGBA{0x30, 0x30, 0x30, 255})
	l.Clip(ScrollViewport, geom.Radii{})
	l.Fill(ScrollViewport, geom.Radii{}, color.NRGBA{0xfa, 0xfa, 0xfa, 255})
	for i := range 30 {
		y := ScrollViewport.Y + float32(i)*24 - off
		if i%2 == 1 {
			l.Fill(geom.R(0, y, 320, 24), geom.Radii{}, color.NRGBA{0xe8, 0xf0, 0xff, 255})
		}
		l.Glyphs(geom.Pt(8, y+17), gputest.Shape(t, fmt.Sprintf("第 %d 行 row", i), 13), gputest.Ink)
	}
	l.Pop()
	if extra != nil {
		extra(l)
	}
	return l
}

// scrollRegion: scrolling copies the viewport's pixels within the target
// (source and destination overlap) and redraws the exposed strip; verify
// mode confirms the result equals the full frame, both directions.
func (c *suite) scrollRegion(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		c.run(t, fmt.Sprint(scale), func(t *testing.T) {
			d := c.device(t)
			lf := &LastFrame{Device: d}
			r := gpu.NewRenderer(lf)
			r.SetVerify(true)
			offs := []float32{0, 48, 24, 96, 0}
			if err := r.Render(ScrollList(t, 0, nil), ScrollSize, scale); err != nil {
				t.Fatal(err)
			}
			for k := 1; k < len(offs); k++ {
				dy := offs[k-1] - offs[k]
				l := ScrollList(t, offs[k], func(l *paint.List) { l.Scroll(ScrollViewport, geom.Pt(0, dy)) })
				if err := r.Render(l, ScrollSize, scale); err != nil {
					t.Fatalf("scroll %v: %v", dy, err)
				}
			}
			// The copy itself, without verify mode's full frame after it.
			r.SetVerify(false)
			if err := r.Render(ScrollList(t, 0, nil), ScrollSize, scale); err != nil {
				t.Fatal(err)
			}
			l := ScrollList(t, 24, func(l *paint.List) { l.Scroll(ScrollViewport, geom.Pt(0, -24)) })
			if err := r.Render(l, ScrollSize, scale); err != nil {
				t.Fatal(err)
			}
			if len(lf.Copies) != 1 || lf.Dirty != 1 {
				t.Fatalf("copies %v, %d dirty; want one copy and the strip", lf.Copies, lf.Dirty)
			}
			ref := paint.NewCPU()
			if err := ref.Render(l, ScrollSize, scale); err != nil {
				t.Fatal(err)
			}
			gputest.Check(t, "scrolled", l, scale, ref.Image(), readPixels(t, d), 4)
		})
	}
}

// verifyCatchesWrongDamage: a Damage hint that misses a change is
// reported in verify mode (read back from the device).
func (c *suite) verifyCatchesWrongDamage(t *testing.T) {
	d := c.device(t)
	r := gpu.NewRenderer(d)
	r.SetVerify(true)
	mk := func(c color.NRGBA) *paint.List {
		l := &paint.List{Background: gputest.White}
		l.Fill(geom.R(10, 10, 30, 20), geom.Rad(4), gputest.Blue)
		l.Fill(geom.R(50, 10, 30, 20), geom.Rad(4), c)
		return l
	}
	if err := r.Render(mk(gputest.Blue), gputest.SceneSize, 2); err != nil {
		t.Fatal(err)
	}
	l := mk(gputest.Red)
	l.Damage(geom.R(10, 10, 30, 20)) // the other rectangle changed
	if err := r.Render(l, gputest.SceneSize, 2); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("err = %v, want a verify difference", err)
	}
}

// notRetained: a partial frame on a recreated target is refused with
// gpu.ErrNotRetained; the Renderer, which asks Retained first, draws the
// next hinted frame in full.
func (c *suite) notRetained(t *testing.T) {
	d := c.device(t)
	lf := &LastFrame{Device: d}
	r := gpu.NewRenderer(lf)
	if err := r.Render(ScrollList(t, 0, nil), ScrollSize, 1); err != nil {
		t.Fatal(err)
	}
	// Recreate the target behind the Renderer's back (another size).
	if err := d.Execute(&gpu.Frame{Size: image.Pt(10, 10)}); err != nil {
		t.Fatal(err)
	}
	err := d.Execute(&gpu.Frame{Size: image.Pt(320, 480), Dirty: []image.Rectangle{image.Rect(0, 0, 4, 4)}})
	if !errors.Is(err, gpu.ErrNotRetained) {
		t.Fatalf("partial frame on a new target: err = %v", err)
	}
	l := ScrollList(t, 0, func(l *paint.List) { l.Damage(geom.R(0, 0, 10, 10)) })
	if err := r.Render(l, ScrollSize, 1); err != nil {
		t.Fatal(err)
	}
	if lf.Dirty != 0 {
		t.Fatal("partial frame after the target was recreated")
	}
}

// subpixelScroll: a shift that rounds to no device pixels makes no copy
// (the region is redrawn), and a frame with Copies but no Dirty is
// refused instead of being executed as a full frame (which would clear
// the target and drop the copies).
func (c *suite) subpixelScroll(t *testing.T) {
	d := c.device(t)
	lf := &LastFrame{Device: d}
	r := gpu.NewRenderer(lf)
	r.SetVerify(false)
	if err := r.Render(ScrollList(t, 0, nil), ScrollSize, 1); err != nil {
		t.Fatal(err)
	}
	l := ScrollList(t, 0, func(l *paint.List) {
		l.Add(paint.ScrollRegion{Rect: ScrollViewport, Delta: geom.Pt(0, 0.0004)})
	})
	if err := r.Render(l, ScrollSize, 1); err != nil {
		t.Fatal(err)
	}
	if len(lf.Copies) != 0 || lf.Dirty != 1 {
		t.Fatalf("copies %v, %d dirty; want no copy, the region redrawn", lf.Copies, lf.Dirty)
	}
	ref := paint.NewCPU()
	if err := ref.Render(l, ScrollSize, 1); err != nil {
		t.Fatal(err)
	}
	gputest.Check(t, "sub-pixel", l, 1, ref.Image(), readPixels(t, d), 4)

	err := d.Execute(&gpu.Frame{Size: image.Pt(320, 480), Copies: []gpu.CopyRegion{{Src: image.Rect(0, 0, 4, 4), Dst: image.Rect(4, 4, 8, 8)}}})
	if err == nil {
		t.Fatal("frame with Copies and no Dirty accepted")
	}
}

// verifyFractionalScroll: whole-pixel scrolls of rows at a fractional
// offset, at fractional scales, with a dithered gradient row, pass strict
// verify mode on the device (copies within one level).
func (c *suite) verifyFractionalScroll(t *testing.T) {
	grad := func(off float32) func(*paint.List) {
		return func(l *paint.List) {
			l.Clip(ScrollViewport, geom.Radii{})
			y := ScrollViewport.Y + 40 - off
			l.LinearStops(geom.R(0, y, 320, 24), geom.Radii{}, geom.Pt(0, y), geom.Pt(320, y+24),
				paint.Stop{Offset: 0, Color: color.NRGBA{0x20, 0x40, 0x60, 255}},
				paint.Stop{Offset: 1, Color: color.NRGBA{0x28, 0x44, 0x66, 255}})
			l.Pop()
		}
	}
	for _, sc := range []struct{ scale, step float32 }{{1, 3}, {1.25, 4}, {1.5, 2}} {
		c.run(t, fmt.Sprint(sc.scale), func(t *testing.T) {
			d := c.device(t)
			lf := &LastFrame{Device: d}
			r := gpu.NewRenderer(lf)
			r.SetVerify(true)
			r.SetVerifyStrict(true)
			off := float32(3.3)
			if err := r.Render(ScrollList(t, off, grad(off)), ScrollSize, sc.scale); err != nil {
				t.Fatal(err)
			}
			for k, s := range []float32{1, 2, -1, 3} {
				dy := s * sc.step
				off += dy
				o := off
				l := ScrollList(t, o, func(l *paint.List) {
					grad(o)(l)
					l.Scroll(ScrollViewport, geom.Pt(0, -dy))
				})
				if err := r.Render(l, ScrollSize, sc.scale); err != nil {
					t.Fatalf("frame %d: %v", k, err)
				}
			}
		})
	}
}
