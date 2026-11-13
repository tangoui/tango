//go:build darwin

package metal

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/paint"
)

func newDevice(tb testing.TB) *Device {
	tb.Helper()
	d, err := NewDevice()
	if errors.Is(err, ErrUnsupported) || errors.Is(err, ErrNoDevice) {
		tb.Skipf("no Metal device: %v", err)
	}
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(d.Close)
	return d
}

func readPixels(tb testing.TB, d *Device) *image.RGBA {
	tb.Helper()
	img, err := d.ReadPixels()
	if err != nil {
		tb.Fatal(err)
	}
	return img
}

// TestMetalZeroSize: frames without pixels submit no command buffer,
// through the Renderer and when executed directly. (That they succeed and
// draw nothing is gpuconf's ZeroSize, run by TestMetalConformance.)
func TestMetalZeroSize(t *testing.T) {
	d := newDevice(t)
	r := gpu.NewRenderer(d)
	l := gputest.Scenes[0].Make(t)
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
	if d.commits != 0 {
		t.Fatalf("%d command buffers committed for empty frames", d.commits)
	}
}

func TestMetalClosed(t *testing.T) {
	d := newDevice(t)
	d.Close()
	d.Close() // idempotent
	if err := d.Execute(&gpu.Frame{Size: image.Pt(4, 4)}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Execute after Close: %v", err)
	}
}

// BenchmarkMetalUploads measures Execute with many small mask uploads (a
// frame that inserts many glyphs) against one upload of the same rows.
func BenchmarkMetalUploads(b *testing.B) {
	mask := image.Pt(2048, 2048)
	atlasPix := make([]byte, mask.X*mask.Y)
	for _, n := range []int{1, 64, 500} {
		b.Run(fmt.Sprintf("uploads%d", n), func(b *testing.B) {
			d := newDevice(b)
			f := &gpu.Frame{Size: image.Pt(64, 64), Clear: color.NRGBA{255, 255, 255, 255}, MaskSize: mask, ColorSize: image.Pt(4, 4)}
			if n == 1 {
				// The rows the 500 glyphs below occupy, in one upload.
				f.Uploads = []gpu.Upload{{Tex: gpu.TexMask, Rect: image.Rect(0, 0, 2048, 8*24), Pix: atlasPix, Stride: mask.X}}
			} else {
				for i := range n {
					x, y := (i%85)*24, (i/85)*24
					f.Uploads = append(f.Uploads, gpu.Upload{Tex: gpu.TexMask, Rect: image.Rect(x, y, x+20, y+22), Pix: atlasPix[y*mask.X+x:], Stride: mask.X})
				}
			}
			for b.Loop() {
				if err := d.Execute(f); err != nil {
					b.Fatal(err)
				}
				if _, err := d.wait(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestMetalTargetTooLarge: a frame larger than Metal's texture limit (a
// window wider than 16384 device pixels) fails with ErrTargetTooLarge
// before anything is allocated or submitted, and does not count as a GPU
// failure: any number of them in a row never loses the device, the
// failure count stays untouched, and the next frame of a valid size draws
// normally. Oversized atlas or patch textures in a Frame fail the same
// way.
func TestMetalTargetTooLarge(t *testing.T) {
	d := newDevice(t)
	r := gpu.NewRenderer(d)
	l := gputest.Scenes[0].Make(t)
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	commits := d.commits
	for i := range 2 * maxFailures {
		size := geom.Size{W: maxTextureSize + 1, H: 10}
		if i%2 == 1 {
			size = geom.Size{W: 10, H: 9000} // 18000 device pixels at 2x
		}
		err := r.Render(l, size, 2)
		if !errors.Is(err, ErrTargetTooLarge) || errors.Is(err, ErrDeviceLost) {
			t.Fatalf("frame %d (%v@2x): %v, want ErrTargetTooLarge", i, size, err)
		}
	}
	for _, f := range []*gpu.Frame{
		{Size: image.Pt(8, 8), MaskSize: image.Pt(4, maxTextureSize+1), ColorSize: image.Pt(4, 4)},
		{Size: image.Pt(8, 8), MaskSize: image.Pt(4, 4), ColorSize: image.Pt(maxTextureSize+1, 4)},
	} {
		if err := d.Execute(f); !errors.Is(err, ErrTargetTooLarge) {
			t.Fatalf("mask %v color %v: %v, want ErrTargetTooLarge", f.MaskSize, f.ColorSize, err)
		}
	}
	d.mu.Lock()
	failures, lost := d.failures, d.lost
	d.mu.Unlock()
	if failures != 0 || lost != nil {
		t.Fatalf("after oversized frames: failures %d, lost %v", failures, lost)
	}
	if d.commits != commits {
		t.Fatalf("%d command buffers committed for oversized frames", d.commits-commits)
	}
	// The largest valid target still works, and so does the usual size.
	if err := r.Render(l, geom.Size{W: maxTextureSize, H: 4}, 1); err != nil {
		t.Fatalf("%d px wide: %v", maxTextureSize, err)
	}
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	ref := paint.NewCPU()
	ref.Render(l, gputest.SceneSize, 1)
	gputest.Check(t, "after", l, 1, ref.Image(), readPixels(t, d), 2)
}

func TestVertexFormat(t *testing.T) {
	for _, c := range []struct {
		kind string
		n    int
		want uint64
	}{{"f32", 1, 28}, {"f32", 2, 29}, {"f32", 4, 31}, {"i32", 1, 32}, {"i32", 3, 34}, {"u32", 2, 37}, {"u32", 4, 39}} {
		if got, err := vertexFormat(c.kind, c.n); err != nil || got != c.want {
			t.Errorf("vertexFormat(%s, %d) = %d, %v; want %d (MTLVertexFormat)", c.kind, c.n, got, err, c.want)
		}
	}
	for _, c := range []struct {
		kind string
		n    int
	}{{"f64", 1}, {"f32", 0}, {"f32", 5}} {
		if _, err := vertexFormat(c.kind, c.n); err == nil {
			t.Errorf("vertexFormat(%s, %d): no error", c.kind, c.n)
		}
	}
}

// TestMetalImageCache: the device holds one texture per cached image
// across frames, and an invalidated image's old texture is released. (That
// the frames match paint.CPU is gpuconf's ImageCache, run by
// TestMetalConformance.)
func TestMetalImageCache(t *testing.T) {
	d := newDevice(t)
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
		if err := r.Render(l, size, 2); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	draw("first", a, b, a, b)
	draw("cached", a, b, a, b)
	if n := len(d.images); n != 2 {
		t.Fatalf("device holds %d image textures; want 2", n)
	}
	for i := range a.Pix {
		a.Pix[i] = 255 - uint8(i)
	}
	r.InvalidateImage(a)
	draw("invalidated", b, a)
	if n := len(d.images); n != 2 {
		t.Fatalf("after invalidation the device holds %d image textures; want 2 (old A freed)", n)
	}
}

// TestMetalLayerPoolReuse: layer textures come from a pool reused by
// size: ten frames of the nested-groups scene (three layers each) and of
// the two-group gradient scene leave the number of layer textures the
// device holds from growing after the first frame. (That the frames stay
// correct is gpuconf's LayerPoolReuse, run by TestMetalConformance.)
func TestMetalLayerPoolReuse(t *testing.T) {
	d := newDevice(t)
	r := gpu.NewRenderer(d)
	for _, name := range []string{"opacitynest", "opacitygrad"} {
		sc := gputest.SceneNamed(name)
		l := sc.Make(t)
		first := -1
		for frame := range 10 {
			if err := r.Render(l, gputest.SceneSize, 2); err != nil {
				t.Fatal(err)
			}
			n := d.layerTextures()
			if first < 0 {
				first = n
			}
			if n == 0 || n > first {
				t.Fatalf("%s frame %d: %d layer textures (first frame %d)", name, frame, n, first)
			}
		}
		t.Logf("%s: %d layer textures", name, first)
	}
}
