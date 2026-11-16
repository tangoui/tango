//go:build darwin

package metal

import (
	"image"
	"image/color"
	"runtime"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
)

func imageReuseFrame(id int, size image.Point, c color.RGBA) gpu.Frame {
	img := image.NewRGBA(image.Rectangle{Max: size})
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return gpu.Frame{Size: image.Pt(24, 24), MaskSize: image.Pt(1, 1), ColorSize: image.Pt(1, 1), RampSize: image.Pt(1, 1),
		ImageUploads: []gpu.ImageUpload{{ID: id, Size: size, Pix: img.Pix, Stride: img.Stride}},
		Batches:      []gpu.Batch{{Image: id, Instances: []shaders.Instance{{Rect: sl.V4(0, 0, 24, 24), Kind: sl.V4(shaders.KindImage, 0, 0, 0), Params: sl.V4(float32(size.X), float32(size.Y), 0, 0)}}}},
	}
}

func assertReuseFrameColor(t *testing.T, d *Device, want color.RGBA) {
	t.Helper()
	img := readPixels(t, d)
	if got := img.RGBAAt(12, 12); got != want {
		t.Fatalf("image after replacement=%v, want %v", got, want)
	}
}

func TestMetalImageTextureHandoff(t *testing.T) {
	d := newDevice(t)
	a := imageReuseFrame(1, image.Pt(6, 4), color.RGBA{180, 40, 20, 255})
	if err := d.Execute(&a); err != nil {
		t.Fatal(err)
	}
	// Keep the old native object alive: an allocator cannot recycle its
	// address and accidentally make a fresh allocation look like reuse.
	held := retain(d.images[1])
	defer release(held)
	b := imageReuseFrame(2, a.ImageUploads[0].Size, color.RGBA{20, 140, 80, 255})
	b.ImageFrees = []int{1, 1, 999} // duplicate/missing frees remain harmless
	if err := d.Execute(&b); err != nil {
		t.Fatal(err)
	}
	if got := d.images[2]; got != held {
		t.Fatal("same-frame same-size upload allocated a new native texture")
	}
	if d.images[1] != 0 || len(d.images) != 1 {
		t.Fatalf("old image ownership survived: %v", d.images)
	}
	assertReuseFrameColor(t, d, color.RGBA{20, 140, 80, 255})
}

func TestMetalImageSameIDReupload(t *testing.T) {
	d := newDevice(t)
	f := imageReuseFrame(1, image.Pt(5, 3), color.RGBA{150, 20, 30, 255})
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	held := retain(d.images[1])
	defer release(held)
	f = imageReuseFrame(1, image.Pt(5, 3), color.RGBA{40, 80, 120, 255})
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	if d.images[1] != held {
		t.Fatal("same-ID same-size complete upload allocated a new texture")
	}
	assertReuseFrameColor(t, d, color.RGBA{40, 80, 120, 255})
}

func TestMetalImageHandoffSizeAndLifetime(t *testing.T) {
	for _, kind := range []string{"different-size", "next-frame", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			d := newDevice(t)
			sz := image.Pt(5, 3)
			if kind == "oversized" {
				sz = image.Pt(1536, 1536)
			} // exceeds 8 MiB handoff limit
			f := imageReuseFrame(1, sz, color.RGBA{150, 20, 30, 255})
			if err := d.Execute(&f); err != nil {
				t.Fatal(err)
			}
			held := retain(d.images[1])
			defer release(held)
			if kind == "next-frame" {
				blank := gpu.Frame{Size: f.Size, MaskSize: f.MaskSize, ColorSize: f.ColorSize, RampSize: f.RampSize, ImageFrees: []int{1}}
				if err := d.Execute(&blank); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "different-size" {
				sz = image.Pt(6, 3)
			}
			f = imageReuseFrame(2, sz, color.RGBA{10, 120, 80, 255})
			f.ImageFrees = []int{1}
			if err := d.Execute(&f); err != nil {
				t.Fatal(err)
			}
			if d.images[2] == held {
				t.Fatal("reused a mismatched, previous-frame or oversized candidate")
			}
			assertReuseFrameColor(t, d, color.RGBA{10, 120, 80, 255})
		})
	}
}

func TestMetalImageHandoffDistinctAndError(t *testing.T) {
	d := newDevice(t)
	f := imageReuseFrame(1, image.Pt(5, 3), color.RGBA{150, 20, 30, 255})
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	g := imageReuseFrame(2, image.Pt(5, 3), color.RGBA{10, 120, 80, 255})
	g.ImageFrees = []int{1}
	extra := g.ImageUploads[0]
	extra.ID = 3
	g.ImageUploads = append(g.ImageUploads, extra)
	if err := d.Execute(&g); err != nil {
		t.Fatal(err)
	}
	if d.images[2] == d.images[3] {
		t.Fatal("distinct live images share a texture")
	}
	bad := imageReuseFrame(4, image.Pt(5, 3), color.RGBA{10, 120, 80, 255})
	bad.ImageFrees = []int{2, 3}
	bad.ImageUploads[0].Pix = nil
	if err := d.Execute(&bad); err == nil {
		t.Fatal("invalid upload accepted")
	}
	if len(d.images) != 0 {
		t.Fatalf("error retained freed image IDs: %v", d.images)
	}
	valid := imageReuseFrame(5, image.Pt(5, 3), color.RGBA{80, 40, 20, 255})
	if err := d.Execute(&valid); err != nil {
		t.Fatal(err)
	}
	assertReuseFrameColor(t, d, color.RGBA{80, 40, 20, 255})
	d.Close()
	d.Close()
}

func TestMetalImageHandoffBounds(t *testing.T) {
	for _, tc := range []struct {
		name             string
		count, wantReuse int
		size             image.Point
	}{{"count", 40, 32, image.Pt(5, 3)}, {"bytes", 3, 2, image.Pt(1024, 768)}} {
		t.Run(tc.name, func(t *testing.T) {
			d := newDevice(t)
			first := imageReuseFrame(1, tc.size, color.RGBA{180, 40, 20, 255})
			base := first.ImageUploads[0]
			for i := 2; i <= tc.count; i++ {
				u := base
				u.ID = i
				first.ImageUploads = append(first.ImageUploads, u)
			}
			if err := d.Execute(&first); err != nil {
				t.Fatal(err)
			}
			held := map[id]bool{}
			for _, tex := range d.images {
				held[tex] = true
				defer release(retain(tex))
			}
			next := imageReuseFrame(tc.count+1, tc.size, color.RGBA{20, 140, 80, 255})
			base = next.ImageUploads[0]
			for i := 2; i <= tc.wantReuse+1; i++ {
				u := base
				u.ID = tc.count + i
				next.ImageUploads = append(next.ImageUploads, u)
			}
			for i := 1; i <= tc.count; i++ {
				next.ImageFrees = append(next.ImageFrees, i)
			}
			if err := d.Execute(&next); err != nil {
				t.Fatal(err)
			}
			reused := 0
			seen := map[id]bool{}
			for _, tex := range d.images {
				if seen[tex] {
					t.Fatal("two live images share a handed-off texture")
				}
				seen[tex] = true
				if held[tex] {
					reused++
				}
			}
			if reused != tc.wantReuse {
				t.Fatalf("reused %d textures, want %d within handoff bound", reused, tc.wantReuse)
			}
			assertReuseFrameColor(t, d, color.RGBA{20, 140, 80, 255})
		})
	}
}

func TestMetalImageHandoffPaddedAlpha(t *testing.T) {
	d := newDevice(t)
	f := imageReuseFrame(1, image.Pt(5, 3), color.RGBA{180, 40, 20, 255})
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	held := retain(d.images[1])
	defer release(held)
	want := color.RGBA{20, 40, 60, 128}
	f = imageReuseFrame(2, image.Pt(5, 3), want)
	u := &f.ImageUploads[0]
	stride := u.Stride + 12
	pad := make([]byte, stride*u.Size.Y)
	for y := 0; y < u.Size.Y; y++ {
		copy(pad[y*stride:], u.Pix[y*u.Stride:(y+1)*u.Stride])
	}
	u.Pix, u.Stride = pad, stride
	f.ImageFrees = []int{1}
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	if d.images[2] != held {
		t.Fatal("stride/alpha change prevented exact-size reuse")
	}
	assertReuseFrameColor(t, d, want)
}

func BenchmarkMetalImageTextureChurn(b *testing.B) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d := newDevice(b)
	d.mu.Lock()
	defer d.mu.Unlock()
	f := imageReuseFrame(1, image.Pt(240, 128), color.RGBA{80, 40, 20, 255})
	if err := d.applyImages(&f); err != nil {
		b.Fatal(err)
	}
	f.ImageFrees = []int{1}
	b.ReportAllocs()
	for b.Loop() {
		pool := autoreleasePool()
		old := f.ImageUploads[0].ID
		f.ImageFrees[0] = old
		f.ImageUploads[0].ID = 3 - old
		f.Batches[0].Image = 3 - old
		err := d.applyImages(&f)
		pool()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMetalImageFrameChurn(b *testing.B) {
	d := newDevice(b)
	f := imageReuseFrame(1, image.Pt(240, 128), color.RGBA{80, 40, 20, 255})
	if err := d.Execute(&f); err != nil {
		b.Fatal(err)
	}
	if err := d.Wait(); err != nil {
		b.Fatal(err)
	}
	f.ImageFrees = []int{1}
	var submit, wait time.Duration
	b.ReportAllocs()
	for b.Loop() {
		old := f.ImageUploads[0].ID
		f.ImageFrees[0] = old
		f.ImageUploads[0].ID = 3 - old
		f.Batches[0].Image = 3 - old
		start := time.Now()
		if err := d.Execute(&f); err != nil {
			b.Fatal(err)
		}
		mid := time.Now()
		if err := d.Wait(); err != nil {
			b.Fatal(err)
		}
		submit += mid.Sub(start)
		wait += time.Since(mid)
	}
	b.ReportMetric(float64(submit.Nanoseconds())/float64(b.N)/1e6, "submit-ms")
	b.ReportMetric(float64(wait.Nanoseconds())/float64(b.N)/1e6, "gpuwait-ms")
}
