package gpu

import (
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"math"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// imageUploads counts f's image texture uploads.
func imageIDs(f *Frame) []int {
	var ids []int
	for _, b := range f.Batches {
		for _, in := range b.Instances {
			if in.Kind.X == shaders.KindImage {
				ids = append(ids, b.Image)
			}
		}
	}
	return ids
}

// TestRendererImageNative: images (plain, rounded, inside a rounded clip)
// are KindImage instances sampling their own image texture with the
// shader's Catmull-Rom filter, not patches, and match paint.CPU.
func TestRendererImageNative(t *testing.T) {
	img := testImage()
	l := &paint.List{Background: white}
	l.DrawImage(img, geom.R(4, 4, 48, 32), geom.Radii{})
	l.DrawImage(img, geom.R(55.5, 8.5, 30, 20), geom.Rad(6))
	l.Clip(geom.R(10, 38, 60, 16), geom.Rad(7))
	l.DrawImage(img, geom.R(5.25, 33.5, 70, 22), geom.Radii{TL: 9, BR: 3})
	l.Pop()
	for _, scale := range gputest.Scales {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
			n := kinds(dev.last)
			if n[shaders.KindPatch] != 0 || n[shaders.KindImage] != 3 {
				t.Fatalf("instances by kind %v; want 3 images and no patches", n)
			}
			if len(dev.last.ImageUploads) != 1 {
				t.Fatalf("%d image uploads; want 1 (one source image)", len(dev.last.ImageUploads))
			}
			for _, id := range imageIDs(dev.last) {
				if id != dev.last.ImageUploads[0].ID {
					t.Fatalf("instance in a batch binding image %d, uploaded %d", id, dev.last.ImageUploads[0].ID)
				}
			}
			gputest.Check(t, "images", l, scale, want, got, 1)
		})
	}
}

// TestImageScaleExtremes: an image reduced to 1×1, one enlarged 16×, a
// 1×1 source and non-RGBA sources (*image.Gray, *image.Paletted,
// *image.NRGBA) draw natively and match paint.CPU.
func TestImageScaleExtremes(t *testing.T) {
	big := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			big.SetRGBA(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
		}
	}
	small := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for i := range small.Pix {
		small.Pix[i] = uint8(i*37 + 20)
	}
	one := image.NewRGBA(image.Rect(0, 0, 1, 1))
	one.SetRGBA(0, 0, color.RGBA{10, 120, 200, 220})
	gray := image.NewGray(image.Rect(0, 0, 5, 4))
	for i := range gray.Pix {
		gray.Pix[i] = uint8(i * 13)
	}
	pal := image.NewPaletted(image.Rect(0, 0, 6, 5), palette.Plan9)
	for i := range pal.Pix {
		pal.Pix[i] = uint8(i * 7)
	}
	// A sub-image not anchored at the origin.
	sub := big.SubImage(image.Rect(10, 20, 30, 34))

	cases := []struct {
		name string
		img  image.Image
		rect geom.Rect
		rad  geom.Radii
	}{
		{"1x1-out", big, geom.R(5, 5, 1, 1), geom.Radii{}},
		{"16x-up", small, geom.R(4, 4, 64, 48), geom.Rad(5)},
		{"1x1-src", one, geom.R(10.5, 6.25, 40, 30), geom.Radii{}},
		{"gray", gray, geom.R(3, 3, 50, 40), geom.Radii{TL: 10}},
		{"paletted", pal, geom.R(7.5, 2, 33, 44), geom.Rad(3)},
		{"subimage", sub, geom.R(20, 8, 45, 30), geom.Radii{}},
		{"reduced", big, geom.R(60, 10, 13, 9.5), geom.Radii{}},
	}
	for _, c := range cases {
		for _, scale := range gputest.Scales {
			t.Run(fmt.Sprintf("%s@%v", c.name, scale), func(t *testing.T) {
				l := &paint.List{Background: white}
				l.DrawImage(c.img, c.rect, c.rad)
				want, got, dev := renderBoth(t, l, gputest.SceneSize, scale)
				if n := kinds(dev.last); n[shaders.KindPatch] != 0 || n[shaders.KindImage] != 1 {
					t.Fatalf("instances by kind %v; want 1 image and no patches", n)
				}
				gputest.Check(t, c.name, l, scale, want, got, 1)
			})
		}
	}
}

// TestImageCacheIdentity: image textures are cached by the image.Image
// value: the same image in the next frame is not uploaded again (also when
// drawn twice per frame), InvalidateImage frees the texture and re-uploads
// the image's current pixels, and images unused for a while are freed.
func TestImageCacheIdentity(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for i := range a.Pix {
		a.Pix[i] = 200
	}
	b := testImage()
	list := func(imgs ...image.Image) *paint.List {
		l := &paint.List{Background: white}
		for i, img := range imgs {
			l.DrawImage(img, geom.R(float32(4+i*20), 4, 18, 14), geom.Radii{})
		}
		return l
	}
	size := geom.Size{W: 96, H: 24}
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	render := func(l *paint.List) {
		t.Helper()
		if err := r.Render(l, size, 1); err != nil {
			t.Fatal(err)
		}
		ref := paint.NewCPU()
		ref.Render(l, size, 1)
		gputest.Check(t, "cache", l, 1, ref.Image(), dev.img, 1)
	}

	render(list(a, a))
	if n := len(dev.last.ImageUploads); n != 1 {
		t.Fatalf("first frame: %d uploads, want 1", n)
	}
	idA := dev.last.ImageUploads[0].ID
	render(list(a, a))
	if n := len(dev.last.ImageUploads); n != 0 {
		t.Fatalf("second frame: %d uploads, want 0", n)
	}
	if ids := imageIDs(dev.last); len(ids) != 2 || ids[0] != idA || ids[1] != idA {
		t.Fatalf("second frame binds %v; want [%d %d]", ids, idA, idA)
	}

	// A, B, A: three batches, bound alternately (draw order kept).
	render(list(a, b, a))
	if n := len(dev.last.ImageUploads); n != 1 {
		t.Fatalf("A,B,A: %d uploads, want 1 (B)", n)
	}
	idB := dev.last.ImageUploads[0].ID
	if ids := imageIDs(dev.last); len(ids) != 3 || ids[0] != idA || ids[1] != idB || ids[2] != idA {
		t.Fatalf("A,B,A binds %v", ids)
	}

	// Mutate A in place: without InvalidateImage the cached texture is
	// drawn; after it, A is uploaded again (new ID, old one freed).
	for i := range a.Pix {
		a.Pix[i] = 90
	}
	r.InvalidateImage(a)
	render(list(a))
	if ups := dev.last.ImageUploads; len(ups) != 1 || ups[0].ID == idA {
		t.Fatalf("after InvalidateImage: uploads %v; want one new texture", ups)
	}
	if fr := dev.last.ImageFrees; len(fr) != 1 || fr[0] != idA {
		t.Fatalf("after InvalidateImage: frees %v; want [%d]", fr, idA)
	}

	// B unused for the idle frame count (imageIdleFrames, lowered here) is
	// freed.
	r.images.idle = 3
	for range 5 {
		if err := r.Render(list(a), size, 1); err != nil {
			t.Fatal(err)
		}
		if len(dev.last.ImageFrees) > 0 {
			break
		}
	}
	if fr := dev.last.ImageFrees; len(fr) != 1 || fr[0] != idB {
		t.Fatalf("idle B: frees %v; want [%d]", fr, idB)
	}
	if _, ok := dev.images[idB]; ok {
		t.Fatal("device still holds B")
	}
}

// TestImageLimits: an image larger than maxImageSide after reduction, and
// images beyond the frame's upload budget, are drawn as patches (matching
// paint.CPU); the budget overflow is logged once and the image goes
// native in a later frame.
func TestImageLimits(t *testing.T) {
	wide := image.NewRGBA(image.Rect(0, 0, maxImageSide+1, 2))
	for i := range wide.Pix {
		wide.Pix[i] = uint8(i)
	}
	l := &paint.List{Background: white}
	// Drawn 1:1 (scale 1): no reduction, so the texture would be too wide.
	l.DrawImage(wide, geom.R(-1000, 4, maxImageSide+1, 20), geom.Radii{})
	want, got, dev := renderBoth(t, l, geom.Size{W: 96, H: 40}, 1)
	if n := kinds(dev.last); n[shaders.KindImage] != 0 || n[shaders.KindPatch] == 0 || len(dev.last.ImageUploads) != 0 {
		t.Fatalf("oversized image: kinds %v, %d uploads; want a patch", n, len(dev.last.ImageUploads))
	}
	gputest.Check(t, "oversized", l, 1, want, got, 1)

	a, b := testImage(), testImage() // 24×16: 1536 bytes each
	l = &paint.List{Background: white}
	l.DrawImage(a, geom.R(4, 4, 30, 20), geom.Radii{})
	l.DrawImage(b, geom.R(40, 4, 30, 20), geom.Rad(4))
	dev = &cpuDevice{}
	r := NewRenderer(dev)
	r.imageBudget = 2000
	logs := 0
	r.logf = func(string, ...any) { logs++ }
	size := geom.Size{W: 96, H: 40}
	ref := paint.NewCPU()
	ref.Render(l, size, 1)
	for frame := 1; frame <= 3; frame++ {
		if err := r.Render(l, size, 1); err != nil {
			t.Fatal(err)
		}
		n := kinds(dev.last)
		switch frame {
		case 1:
			if n[shaders.KindImage] != 1 || n[shaders.KindPatch] != 1 || len(dev.last.ImageUploads) != 1 {
				t.Fatalf("frame 1: kinds %v, %d uploads; want A native, B a patch", n, len(dev.last.ImageUploads))
			}
		default:
			if n[shaders.KindImage] != 2 || n[shaders.KindPatch] != 0 {
				t.Fatalf("frame %d: kinds %v; want both native", frame, n)
			}
		}
		gputest.Check(t, fmt.Sprint("budget", frame), l, 1, ref.Image(), dev.img, 1)
	}
	if logs != 1 {
		t.Fatalf("%d log lines; want 1", logs)
	}
}

// TestImageUploadAfterFailure: when Execute fails, the frame's image
// uploads may not have reached the device, so they are sent again.
func TestImageUploadAfterFailure(t *testing.T) {
	l := &paint.List{Background: white}
	l.DrawImage(testImage(), geom.R(4, 4, 30, 20), geom.Radii{})
	d := &failDevice{fail: true}
	r := NewRenderer(d)
	if err := r.Render(l, gputest.SceneSize, 1); err == nil {
		t.Fatal("no error")
	}
	d.fail = false
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if n := len(d.last.ImageUploads); n != 1 {
		t.Fatalf("after a failed frame: %d image uploads; want 1", n)
	}
	ref := paint.NewCPU()
	ref.Render(l, gputest.SceneSize, 1)
	gputest.Check(t, "after failure", l, 1, ref.Image(), d.img, 1)
}

// TestNilOpIgnored: a nil op (l.Add(nil)) draws nothing, as in paint.CPU,
// and does not panic.
func TestNilOpIgnored(t *testing.T) {
	l := &paint.List{Background: white}
	l.Fill(geom.R(4, 4, 20, 20), geom.Radii{}, red)
	l.Add(nil)
	l.Clip(geom.R(0, 0, 50, 50), geom.Rad(4))
	l.Add(nil)
	l.Fill(geom.R(10, 10, 30, 30), geom.Radii{}, blue)
	l.Pop()
	want, got, _ := renderBoth(t, l, gputest.SceneSize, 1)
	gputest.Check(t, "nil op", l, 1, want, got, 1)
}

// TestRampCacheNonFiniteStops: stops with a NaN offset are found in the
// ramp cache again (NaN != NaN must not defeat the equality check), so the
// second frame uploads no ramp row; the gradient matches paint.CPU.
func TestRampCacheNonFiniteStops(t *testing.T) {
	nan, inf := float32(math.NaN()), float32(math.Inf(1))
	l := &paint.List{Background: white}
	l.LinearStops(geom.R(4, 4, 60, 20), geom.Radii{}, geom.Pt(4, 0), geom.Pt(64, 0),
		paint.Stop{Offset: 0, Color: red}, paint.Stop{Offset: nan, Color: green}, paint.Stop{Offset: inf, Color: blue})
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	for frame := 1; frame <= 2; frame++ {
		if err := r.Render(l, gputest.SceneSize, 1); err != nil {
			t.Fatal(err)
		}
		if ups := rampUploads(dev.last); frame == 2 && len(ups) != 0 {
			t.Fatalf("frame 2: %d ramp uploads; want 0", len(ups))
		}
	}
	ref := paint.NewCPU()
	ref.Render(l, gputest.SceneSize, 1)
	gputest.Check(t, "nan stops", l, 1, ref.Image(), dev.img, 2)
}

// asyncFailDevice reports a GPU failure one frame late, as Metal does (the
// previous command buffer's error is returned by the next Execute, before
// that frame's uploads are applied): when failNext is set, the next
// Execute drops every image texture (their blits were in the failed
// command buffer, so their contents are undefined) and fails.
type asyncFailDevice struct {
	cpuDevice
	failNext bool
}

func (d *asyncFailDevice) Execute(f *Frame) error {
	if d.failNext {
		d.failNext = false
		clear(d.images)
		return fmt.Errorf("previous command buffer failed")
	}
	return d.cpuDevice.Execute(f)
}

// TestImageReuploadAfterAsyncFailure: frame N uploads an image and
// "succeeds", frame N+1 reports frame N's GPU failure; frame N+2 must
// upload the image again (the device's copy is undefined) and match
// paint.CPU.
func TestImageReuploadAfterAsyncFailure(t *testing.T) {
	l := &paint.List{Background: white}
	l.DrawImage(testImage(), geom.R(4, 4, 30, 20), geom.Radii{})
	d := &asyncFailDevice{}
	r := NewRenderer(d)
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	d.failNext = true
	if err := r.Render(l, gputest.SceneSize, 1); err == nil {
		t.Fatal("no error")
	}
	if err := r.Render(l, gputest.SceneSize, 1); err != nil {
		t.Fatal(err)
	}
	if n := len(d.last.ImageUploads); n != 1 {
		t.Fatalf("after an asynchronous failure: %d image uploads; want 1", n)
	}
	ref := paint.NewCPU()
	ref.Render(l, gputest.SceneSize, 1)
	gputest.Check(t, "async failure", l, 1, ref.Image(), d.img, 1)
}

// TestImageOverBudgetAlone: an image larger than the whole per-frame
// budget is still uploaded when it is the frame's first upload (otherwise
// it would stay a patch forever).
func TestImageOverBudgetAlone(t *testing.T) {
	l := &paint.List{Background: white}
	l.DrawImage(testImage(), geom.R(4, 4, 30, 20), geom.Radii{}) // 1536 bytes
	l.DrawImage(gputest.TestImage(), geom.R(40, 4, 30, 20), geom.Radii{})
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	r.imageBudget = 1000
	r.logf = func(string, ...any) {}
	for frame := 1; frame <= 2; frame++ {
		if err := r.Render(l, gputest.SceneSize, 1); err != nil {
			t.Fatal(err)
		}
		if n := kinds(dev.last); n[shaders.KindImage] != frame {
			t.Fatalf("frame %d: kinds %v; want %d image instances", frame, n, frame)
		}
	}
}

// valueImage is an image type used by value whose interface field holds
// an incomparable value: reflect calls the type comparable, but hashing
// it as a map key panics.
type valueImage struct {
	image.Image
	tag any
}

// TestImageValueTypeNotCached: such an image draws (as a patch on the
// GPU, uncached on the CPU) without panicking, and matches paint.CPU.
func TestImageValueTypeNotCached(t *testing.T) {
	img := valueImage{testImage(), []int{1}}
	l := &paint.List{Background: white}
	l.DrawImage(img, geom.R(4, 4, 30, 20), geom.Rad(3))
	want, got, _ := renderBoth(t, l, gputest.SceneSize, 1.5)
	gputest.Check(t, "value image", l, 1.5, want, got, 1)
	NewRenderer(&cpuDevice{}).InvalidateImage(img) // no panic
}

// TestPartialFrameKeepsOnScreenImages: a partial frame culls the images
// outside its dirty region (one by one, or a whole layer group), but they
// stay on screen, so the cache keeps their textures (neither the stale
// budget nor the idle limit frees them) and the next full frame uploads
// nothing.
func TestPartialFrameKeepsOnScreenImages(t *testing.T) {
	// Three 2560² textures (26 MB each): the two a partial frame does not
	// draw exceed imageStaleBytes together.
	const side = 2560
	var imgs [3]*image.RGBA
	for i := range imgs {
		imgs[i] = image.NewRGBA(image.Rect(0, 0, side, side))
		imgs[i].Pix[0] = byte(i + 1)
	}
	size := geom.Size{W: 300, H: 100}
	for _, layer := range []bool{false, true} {
		t.Run(fmt.Sprintf("layer=%v", layer), func(t *testing.T) {
			raceSkip(t, layer) // the layer group (images culled with it) under -race
			list := func(dirty bool) *paint.List {
				l := &paint.List{Background: white}
				// Drawn at half size (no reduction), mostly off-frame:
				// image 0 shows at x 0..20, the others from x 100 and 200.
				l.DrawImage(imgs[0], geom.R(20-side/2, 0, side/2, side/2), geom.Radii{})
				if layer {
					l.Opacity(0.5) // two ops: drawn into a layer
				}
				l.DrawImage(imgs[1], geom.R(100, 0, side/2, side/2), geom.Radii{})
				l.DrawImage(imgs[2], geom.R(200, 0, side/2, side/2), geom.Radii{})
				if layer {
					l.Pop()
				}
				if dirty {
					l.Damage(geom.R(2, 2, 10, 10)) // touches image 0 only
				}
				return l
			}
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			r.SetVerify(false) // verify mode's full frame would draw every image
			r.images.idle = 1
			r.imageBudget = 1 << 30 // all three go up in the first frame
			if err := r.Render(list(false), size, 1); err != nil {
				t.Fatal(err)
			}
			if n := len(dev.last.ImageUploads); n != 3 {
				t.Fatalf("first frame uploaded %d images, want 3", n)
			}
			if layer && len(dev.last.Layers) == 0 {
				t.Fatal("group not drawn into a layer")
			}
			for k := range 3 {
				if err := r.Render(list(true), size, 1); err != nil {
					t.Fatal(err)
				}
				if len(dev.last.Dirty) == 0 {
					t.Fatalf("partial frame %d drawn whole", k)
				}
				if f := dev.last.ImageFrees; len(f) != 0 {
					t.Fatalf("partial frame %d freed on-screen images %v", k, f)
				}
				for i, img := range imgs {
					if r.images.byKey[imageKey{img, 0, 0}] == nil {
						t.Fatalf("partial frame %d: image %d evicted", k, i)
					}
				}
			}
			if err := r.Render(list(false), size, 1); err != nil {
				t.Fatal(err)
			}
			if len(dev.last.Dirty) != 0 {
				t.Fatal("hint-free frame partial")
			}
			if n := len(dev.last.ImageUploads); n != 0 {
				t.Fatalf("full frame after partial ones uploaded %d images again", n)
			}
			if n := len(imageIDs(dev.last)); n != 3 {
				t.Fatalf("full frame drew %d images, want 3", n)
			}
		})
	}
}

// TestPartialFrameKeepsImagesUnderTransform: a skipped layer group's images
// are touched under the key image uses, which comes from the rect mapped
// through the native transform (an outer one or one inside the group).
func TestPartialFrameKeepsImagesUnderTransform(t *testing.T) {
	// 4000² shown at 800×800 device pixels: reduced, keyed 800×800.
	img := image.NewRGBA(image.Rect(0, 0, 4000, 4000))
	size := geom.Size{W: 1000, H: 800}
	key := imageKey{img, 800, 800}
	for _, outer := range []bool{true, false} {
		t.Run(fmt.Sprintf("outer=%v", outer), func(t *testing.T) {
			raceSkip(t, !outer) // the transform inside the group under -race
			list := func(dirty bool) *paint.List {
				l := &paint.List{Background: white}
				if outer {
					l.Transform(geom.Scale(2, 2))
				}
				l.Opacity(0.5) // two ops: drawn into a layer
				if !outer {
					l.Transform(geom.Scale(2, 2))
				}
				l.DrawImage(img, geom.R(100, 0, 400, 400), geom.Radii{})
				l.Fill(geom.R(110, 10, 20, 20), geom.Radii{}, blue)
				if !outer {
					l.Pop()
				}
				l.Pop()
				if outer {
					l.Pop()
				}
				if dirty {
					l.Damage(geom.R(2, 2, 10, 10)) // misses the group
				}
				return l
			}
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			r.SetVerify(false)
			r.images.idle = 1
			if err := r.Render(list(false), size, 1); err != nil {
				t.Fatal(err)
			}
			if r.images.byKey[key] == nil || len(dev.last.Layers) == 0 {
				t.Fatalf("setup: texture %v held %v, %d layers", key, r.images.byKey[key] != nil, len(dev.last.Layers))
			}
			for k := range 3 {
				if err := r.Render(list(true), size, 1); err != nil {
					t.Fatal(err)
				}
				if len(dev.last.Dirty) == 0 {
					t.Fatalf("partial frame %d drawn whole", k)
				}
				if r.images.byKey[key] == nil {
					t.Fatalf("partial frame %d evicted the on-screen image", k)
				}
			}
			if err := r.Render(list(false), size, 1); err != nil {
				t.Fatal(err)
			}
			if n := len(dev.last.ImageUploads); n != 0 {
				t.Fatalf("full frame uploaded %d images again", n)
			}
		})
	}
}

// TestPartialFrameEvictsOffScreenImages: touching culled images in partial
// frames is limited to images on screen: one moved fully off-screen (alone
// or inside a skipped layer group) is still freed by the idle limit.
func TestPartialFrameEvictsOffScreenImages(t *testing.T) {
	size := geom.Size{W: 300, H: 100}
	for _, group := range []bool{false, true} {
		t.Run(fmt.Sprintf("group=%v", group), func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, 64, 64))
			key := imageKey{img, 0, 0}
			list := func(x float32, dirty bool) *paint.List {
				l := &paint.List{Background: white}
				if group {
					l.Opacity(0.5)
					l.Fill(geom.R(100, 10, 20, 20), geom.Radii{}, blue) // keeps the group on screen
				}
				l.DrawImage(img, geom.R(x, 10, 64, 64), geom.Radii{})
				if group {
					l.Pop()
				}
				if dirty {
					l.Damage(geom.R(2, 2, 10, 10))
				}
				return l
			}
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			r.SetVerify(false)
			r.images.idle = 1
			if err := r.Render(list(150, false), size, 1); err != nil {
				t.Fatal(err)
			}
			if r.images.byKey[key] == nil {
				t.Fatal("setup: image not uploaded")
			}
			for k := range 3 {
				if err := r.Render(list(1000, true), size, 1); err != nil {
					t.Fatal(err)
				}
				if len(dev.last.Dirty) == 0 {
					t.Fatalf("partial frame %d drawn whole", k)
				}
			}
			if r.images.byKey[key] != nil {
				t.Fatal("off-screen image kept by partial frames")
			}
		})
	}
}
