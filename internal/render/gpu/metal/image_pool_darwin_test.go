//go:build darwin

package metal

import (
	"errors"
	"image"
	"image/color"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

func TestMetalImagePoolVariableSizes(t *testing.T) {
	d := newDevice(t)
	a := imageReuseFrame(1, image.Pt(5, 3), color.RGBA{180, 40, 20, 255})
	if err := d.Execute(&a); err != nil {
		t.Fatal(err)
	}
	held := retain(d.images[1])
	defer release(held)
	b := imageReuseFrame(2, image.Pt(6, 4), color.RGBA{20, 140, 80, 255})
	b.ImageFrees = []int{1}
	if err := d.Execute(&b); err != nil {
		t.Fatal(err)
	}
	c := imageReuseFrame(3, image.Pt(5, 3), color.RGBA{40, 80, 120, 160})
	c.ImageFrees = []int{2}
	// Non-tight source rows still overwrite the entire reused texture.
	u := &c.ImageUploads[0]
	row := u.Stride
	u.Stride += 12
	pix := make([]byte, u.Stride*u.Size.Y)
	for y := range u.Size.Y {
		copy(pix[y*u.Stride:], u.Pix[y*row:][:row])
	}
	u.Pix = pix
	if err := d.Execute(&c); err != nil {
		t.Fatal(err)
	}
	if d.images[3] != held {
		t.Fatal("recurring image size allocated another native texture")
	}
	assertReuseFrameColor(t, d, color.RGBA{40, 80, 120, 160})
}

func TestMetalImagePoolLifetime(t *testing.T) {
	for _, kind := range []string{"no-upload", "expired", "invalid-upload", "sync-failure", "gpu-failure"} {
		t.Run(kind, func(t *testing.T) {
			d := newDevice(t)
			a := imageReuseFrame(1, image.Pt(5, 3), color.RGBA{180, 40, 20, 255})
			if err := d.Execute(&a); err != nil {
				t.Fatal(err)
			}
			held := retain(d.images[1])
			defer release(held)
			b := imageReuseFrame(2, image.Pt(6, 4), color.RGBA{20, 140, 80, 255})
			b.ImageFrees = []int{1}
			if err := d.Execute(&b); err != nil {
				t.Fatal(err)
			}
			b.ImageFrees = nil
			switch kind {
			case "no-upload":
				blank := b
				blank.ImageUploads, blank.Batches = nil, nil
				if err := d.Execute(&blank); err != nil {
					t.Fatal(err)
				}
			case "expired":
				for range 121 {
					if err := d.Execute(&b); err != nil {
						t.Fatal(err)
					}
				}
			case "invalid-upload":
				bad := imageReuseFrame(8, image.Pt(5, 3), color.RGBA{})
				bad.ImageUploads[0].Pix = nil
				if err := d.Execute(&bad); err == nil {
					t.Fatal("invalid upload accepted")
				}
			case "sync-failure":
				boom := errors.New("injected allocation failure")
				if err := d.syncFailed(boom); err != boom {
					t.Fatalf("failure policy changed: %v", err)
				}
			case "gpu-failure":
				if err := d.Wait(); err != nil {
					t.Fatal(err)
				}
				boom := errors.New("injected command failure")
				d.gpuFailed(boom, false)
				if err := d.Execute(&b); err != boom {
					t.Fatalf("failure policy changed: %v", err)
				}
			}
			c := imageReuseFrame(3, image.Pt(5, 3), color.RGBA{40, 80, 120, 255})
			c.ImageFrees = []int{2}
			if err := d.Execute(&c); err != nil {
				t.Fatal(err)
			}
			if d.images[3] == held {
				t.Fatalf("%s retained an unused native texture", kind)
			}
			assertReuseFrameColor(t, d, color.RGBA{40, 80, 120, 255})
		})
	}
}

// Completion failures now arrive through the asynchronous retirement path,
// including presents of an older frame. Neither may retain unused textures.
func TestMetalImagePoolRetiredFailure(t *testing.T) {
	for _, kind := range []string{"frame", "late-present"} {
		t.Run(kind, func(t *testing.T) {
			d := newDevice(t)
			a := imageReuseFrame(1, image.Pt(5, 3), color.RGBA{180, 40, 20, 255})
			if err := d.Execute(&a); err != nil {
				t.Fatal(err)
			}
			b := imageReuseFrame(2, image.Pt(6, 4), color.RGBA{20, 140, 80, 255})
			b.ImageFrees = []int{1}
			if err := d.Execute(&b); err != nil {
				t.Fatal(err)
			}
			if err := d.Wait(); err != nil {
				t.Fatal(err)
			}
			if d.imagePool.n == 0 {
				t.Fatal("failure fixture has no unused image texture")
			}
			s := submission{kind: submitFrame, frame: d.frames}
			if kind == "late-present" {
				s.kind, s.frame = submitPresent, d.frames-1
			}
			boom := errors.New("injected retirement failure")
			func() {
				d.mu.Lock()
				defer d.mu.Unlock()
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				defer autoreleasePool()()
				d.noteRetired(s, boom, false)
			}()
			if d.imagePool.n != 0 || d.imagePool.bytes != 0 {
				t.Fatal("retired GPU failure retained unused image textures")
			}
			if d.failures != 1 || d.pendingErr != boom || d.lost != nil {
				t.Fatal("retirement failure accounting changed")
			}
			if kind == "late-present" && !d.hasFrame {
				t.Fatal("older present invalidated the newer frame")
			}
			b.ImageFrees = nil
			if err := d.Execute(&b); err != boom {
				t.Fatalf("pending completion error changed: %v", err)
			}
			if err := d.Execute(&b); err != nil {
				t.Fatal(err)
			}
			assertReuseFrameColor(t, d, color.RGBA{20, 140, 80, 255})
		})
	}
}

func TestMetalImagePoolBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int
		size image.Point
		want int
	}{{"count", 24, image.Pt(5, 3), 16}, {"bytes", 4, image.Pt(512, 256), 2}} {
		t.Run(tc.name, func(t *testing.T) {
			d := newDevice(t)
			f := imageReuseFrame(1, tc.size, color.RGBA{180, 40, 20, 255})
			for id := 2; id <= tc.n; id++ {
				u := f.ImageUploads[0]
				u.ID = id
				f.ImageUploads = append(f.ImageUploads, u)
			}
			if err := d.Execute(&f); err != nil {
				t.Fatal(err)
			}
			held := map[id]int{}
			for key, tex := range d.images {
				held[tex] = key
				defer release(retain(tex))
			}
			next := imageReuseFrame(100, image.Pt(7, 9), color.RGBA{})
			for i := 1; i <= tc.n; i++ {
				next.ImageFrees = append(next.ImageFrees, i)
			}
			if err := d.Execute(&next); err != nil {
				t.Fatal(err)
			}
			next = imageReuseFrame(101, tc.size, color.RGBA{20, 140, 80, 255})
			for id := 102; id < 101+tc.n; id++ {
				u := next.ImageUploads[0]
				u.ID = id
				next.ImageUploads = append(next.ImageUploads, u)
			}
			if err := d.Execute(&next); err != nil {
				t.Fatal(err)
			}
			reused := 0
			seen := map[id]bool{}
			for _, tex := range d.images {
				if seen[tex] {
					t.Fatal("live image IDs alias a pooled texture")
				}
				seen[tex] = true
				if oldID := held[tex]; oldID != 0 {
					reused++
					if oldID <= tc.n-tc.want {
						t.Fatal("pool evicted a newer spare before an older one")
					}
				}
			}
			if reused != tc.want {
				t.Fatalf("reused %d detached textures, want %d within cap", reused, tc.want)
			}
			assertReuseFrameColor(t, d, color.RGBA{20, 140, 80, 255})
		})
	}
}

func TestMetalImagePoolTrim(t *testing.T) {
	d := newDevice(t)
	r := gpu.NewRenderer(d)
	r.SetVerify(false)
	var prev *image.RGBA
	draw := func(size image.Point, c color.RGBA) id {
		t.Helper()
		if prev != nil {
			r.InvalidateImage(prev)
		}
		img := image.NewRGBA(image.Rectangle{Max: size})
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
		l := &paint.List{}
		l.DrawImage(img, geom.R(0, 0, 24, 24), geom.Radii{})
		if err := r.Render(l, geom.Size{W: 24, H: 24}, 1); err != nil {
			t.Fatal(err)
		}
		assertReuseFrameColor(t, d, c)
		prev = img
		if len(d.images) != 1 {
			t.Fatalf("device holds %d logical images, want 1", len(d.images))
		}
		for _, tex := range d.images {
			return retain(tex)
		}
		panic("unreachable")
	}
	a := draw(image.Pt(5, 3), color.RGBA{180, 40, 20, 255})
	defer release(a)
	b := draw(image.Pt(6, 4), color.RGBA{20, 140, 80, 255})
	defer release(b)
	r.Trim()
	r.Trim()
	if d.imagePool.n != 0 || d.imagePool.bytes != 0 {
		t.Fatal("low-memory trim left unused textures resident")
	}
	c := draw(image.Pt(7, 5), color.RGBA{40, 80, 120, 255})
	defer release(c)
	nextB := draw(image.Pt(6, 4), color.RGBA{30, 70, 100, 255})
	defer release(nextB)
	if nextB == b {
		t.Fatal("first frame after Trim refilled the spare pool")
	}
	nextA := draw(image.Pt(5, 3), color.RGBA{60, 100, 140, 255})
	defer release(nextA)
	if nextA == a {
		t.Fatal("renderer Trim did not release detached image textures")
	}
	d.Close()
	d.Close()
	d.TrimImages()
	if d.imagePool.n != 0 || d.imagePool.bytes != 0 || len(d.images) != 0 || len(d.imageSizes) != 0 {
		t.Fatal("Close retained image resources")
	}
	if err := d.Execute(&gpu.Frame{Size: image.Pt(1, 1)}); err != ErrClosed {
		t.Fatalf("closed device accepted frame: %v", err)
	}
}

// Native nil receivers return nil objects, letting the real allocation paths
// fail without a production allocator hook or a machine-wide memory shortage.
func TestMetalImagePoolAllocationErrors(t *testing.T) {
	for _, kind := range []string{"stage-buffer", "read-buffer", "read-command"} {
		t.Run(kind, func(t *testing.T) {
			d := newDevice(t)
			a := imageReuseFrame(1, image.Pt(5, 3), color.RGBA{180, 40, 20, 255})
			if err := d.Execute(&a); err != nil {
				t.Fatal(err)
			}
			b := imageReuseFrame(2, image.Pt(6, 4), color.RGBA{20, 140, 80, 255})
			b.ImageFrees = []int{1}
			if err := d.Execute(&b); err != nil {
				t.Fatal(err)
			}
			if err := d.Wait(); err != nil {
				t.Fatal(err)
			}
			if d.imagePool.n == 0 {
				t.Fatal("allocation failure fixture has no spare")
			}
			dev, queue, failures := d.dev, d.queue, d.failures
			defer func() { d.dev, d.queue = dev, queue }()
			var err error
			want := "cannot allocate"
			switch kind {
			case "stage-buffer":
				d.dev, d.stagingCap = 0, 0
				b.ImageFrees = nil
				err = d.Execute(&b)
			case "read-buffer":
				d.dev = 0
				_, err = d.ReadPixels()
			case "read-command":
				d.queue = 0
				_, err = d.ReadPixels()
				want = "commandBuffer failed"
			}
			d.dev, d.queue = dev, queue
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s did not reach native failure: %v", kind, err)
			}
			if d.failures != failures || d.pendingErr != nil || d.lost != nil {
				t.Fatal("raw native failure changed the established failure policy")
			}
			if d.imagePool.n != 0 || d.imagePool.bytes != 0 {
				t.Fatal("native failure kept unused textures resident")
			}
			b.ImageFrees = nil
			if err := d.Execute(&b); err != nil {
				t.Fatal(err)
			}
			assertReuseFrameColor(t, d, color.RGBA{20, 140, 80, 255})
		})
	}
}

func BenchmarkMetalImageVariableFrames(b *testing.B) {
	d := newDevice(b)
	var frames [14]gpu.Frame
	for i := range frames {
		frames[i] = imageReuseFrame(1, image.Pt(214+2*i, 64), color.RGBA{80, 40, 20, 255})
		frames[i].ImageFrees = []int{2}
	}
	first := frames[0]
	first.ImageFrees = nil
	if err := d.Execute(&first); err != nil {
		b.Fatal(err)
	}
	if err := d.Wait(); err != nil {
		b.Fatal(err)
	}
	i, old := 1, 1
	var submit, wait time.Duration
	b.ReportAllocs()
	for b.Loop() {
		f := &frames[i%len(frames)]
		f.ImageFrees[0] = old
		f.ImageUploads[0].ID = 3 - old
		f.Batches[0].Image = 3 - old
		start := time.Now()
		if err := d.Execute(f); err != nil {
			b.Fatal(err)
		}
		mid := time.Now()
		if err := d.Wait(); err != nil {
			b.Fatal(err)
		}
		submit += mid.Sub(start)
		wait += time.Since(mid)
		old, i = 3-old, i+1
	}
	b.ReportMetric(float64(submit.Nanoseconds())/float64(b.N)/1e6, "submit-ms")
	b.ReportMetric(float64(wait.Nanoseconds())/float64(b.N)/1e6, "gpuwait-ms")
}
