package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func imgTestGradient(w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.SetRGBA(x, y, color.RGBA{uint8(x * 255 / max(1, w-1)), uint8(y * 255 / max(1, h-1)), 0, 255})
		}
	}
	return m
}

func TestImgPlace(t *testing.T) {
	r := geom.R(0, 0, 100, 100)
	dst, sx, _, sw, _ := imgPlace("cover", r, 200, 100)
	if dst != r || sx != 50 || sw != 100 {
		t.Fatalf("cover: %v %v %v", dst, sx, sw)
	}
	dst, _, _, _, _ = imgPlace("contain", r, 200, 100)
	if dst.H != 50 || dst.Y != 25 {
		t.Fatalf("contain: %v", dst)
	}
}

func TestImagePreviewStateMachine(t *testing.T) {
	src := imgTestGradient(300, 200)
	imgs := []image.Image{src, imgTestGradient(100, 100)}
	var w *ImageW
	d := NewDriver(func(c *Context) {
		w = Image(c, src).Width(120).Alt("thumb").PreviewGroup(imgs, 0)
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	d.Frame()
	if err := d.ClickLabel("thumb"); err != nil {
		t.Fatal(err)
	}
	if !w.PreviewOpen() {
		t.Fatal("preview should open on click")
	}
	st := w.st
	must := func(label string) {
		t.Helper()
		if err := d.ClickLabel(label); err != nil {
			t.Fatal(err)
		}
	}
	must("放大")
	if st.scale <= 1 {
		t.Fatalf("zoom in: %v", st.scale)
	}
	must("缩小")
	must("缩小")
	if st.scale >= 1 {
		t.Fatalf("zoom out: %v", st.scale)
	}
	must("向右旋转")
	if st.angle != 90 || w.rotated(src, 1).Bounds().Dx() != 200 {
		t.Fatalf("rotate: %v", st.angle)
	}
	must("向左旋转")
	must("向左旋转")
	if st.angle != -90 {
		t.Fatalf("rotate left: %v", st.angle)
	}
	must("下一张")
	if st.idx != 1 || st.angle != 0 || st.scale != 1 {
		t.Fatalf("next resets transform: idx=%d angle=%v scale=%v", st.idx, st.angle, st.scale)
	}
	d.Key(input.KeyEscape, 0)
	if w.PreviewOpen() {
		t.Fatal("Esc should close preview")
	}
	// Reopen and close with the close button.
	if err := d.ClickLabel("thumb"); err != nil {
		t.Fatal(err)
	}
	must("关闭预览")
	if w.PreviewOpen() {
		t.Fatal("close button should close")
	}
	if _, ok := d.Find("关闭预览"); ok {
		t.Fatal("overlay should be gone")
	}
}

func TestImagePlaceholderNoPreview(t *testing.T) {
	var w *ImageW
	d := NewDriver(func(c *Context) {
		w = Image(c, nil).Size(100, 80).Loading(true).Preview(true).Alt("ph")
	}, DriverOptions{Width: 300, Height: 200})
	d.Frame()
	d.Click(geom.Pt(50, 40))
	if w.PreviewOpen() {
		t.Fatal("placeholder must not open preview")
	}
}

func TestWatermarkPassesClicks(t *testing.T) {
	clicked := false
	d := NewDriver(func(c *Context) {
		Watermark(c, "Tango", "内部资料").Body(func() {
			if Button(c, "提交").Clicked() {
				clicked = true
			}
			Div(c).Style(layout.H(200))
		})
	}, DriverOptions{Width: 400, Height: 300})
	d.Frame()
	if err := d.ClickLabel("提交"); err != nil {
		t.Fatal(err)
	}
	if !clicked {
		t.Fatal("watermark swallowed the click")
	}
	// The mark must actually be painted: some pixel differs from the bg.
	img := d.Image()
	b := img.Bounds()
	painted := false
	for y := b.Min.Y; y < b.Max.Y && !painted; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			if c := img.RGBAAt(x, y); c.R < 250 && y > 80 {
				painted = true
				break
			}
		}
	}
	if !painted {
		t.Fatal("watermark not painted")
	}
}

func TestImagePreviewWheelZoomAboutPointer(t *testing.T) {
	src := imgTestGradient(300, 200)
	var w *ImageW
	d := NewDriver(func(c *Context) {
		w = Image(c, src).Width(120).Alt("thumb").Preview(true)
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	if err := d.ClickLabel("thumb"); err != nil {
		t.Fatal(err)
	}
	st := w.st
	r0 := st.imgRect
	// A point on the image right of centre stays under the pointer.
	p := geom.Pt(r0.X+r0.W*0.75, r0.Y+r0.H*0.25)
	fx, fy := (p.X-r0.X)/r0.W, (p.Y-r0.Y)/r0.H
	d.Move(p)
	d.Frame()
	d.Scroll(p, -80) // wheel up: zoom in
	d.Frame()
	if st.scale <= 1 {
		t.Fatalf("wheel did not zoom: %v", st.scale)
	}
	r1 := st.imgRect
	if gx, gy := (p.X-r1.X)/r1.W, (p.Y-r1.Y)/r1.H; math.Abs(float64(gx-fx)) > 0.01 || math.Abs(float64(gy-fy)) > 0.01 {
		t.Fatalf("zoom not about pointer: %.3f,%.3f vs %.3f,%.3f", gx, gy, fx, fy)
	}
	for i := 0; i < 40; i++ {
		d.Scroll(p, -200)
	}
	if st.scale != imgMaxScale {
		t.Fatalf("max scale %v", st.scale)
	}
	for i := 0; i < 60; i++ {
		d.Scroll(p, 200)
	}
	if st.scale != imgMinScale || st.pan != (geom.Point{}) {
		t.Fatalf("min scale %v pan %v", st.scale, st.pan)
	}
}

func TestImagePreviewFreeRotation(t *testing.T) {
	src := imgTestGradient(300, 200)
	var w *ImageW
	rotate := 0.0
	d := NewDriver(func(c *Context) {
		w = Image(c, src).Width(120).Alt("thumb").Preview(true)
		if rotate != 0 {
			w.Rotate(rotate)
			rotate = 0
		}
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	if err := d.ClickLabel("thumb"); err != nil {
		t.Fatal(err)
	}
	st := w.st
	rest := st.imgRect
	rotate = 30
	d.Frame()
	d.Frame()
	motionShot(t, d, "ImagePreview-rot30")
	if w.PreviewAngle() != 30 {
		t.Fatalf("api rotate: %v", w.PreviewAngle())
	}
	// The rotated image's bounding box grows (300×200 turned 30°).
	if r := st.imgRect; !(r.W > rest.W && r.H > rest.H) {
		t.Fatalf("rotated bounds %v vs %v", r, rest)
	}
	// The pixel at the old top-left corner is now mask, the centre image.
	img := d.Image()
	cen := st.center
	if c := img.RGBAAt(int(cen.X), int(cen.Y)); c.R == c.G && c.G == c.B {
		t.Fatalf("centre not image: %v", c)
	}
	// Shift-drag a quarter turn around the centre: +90°.
	from := geom.Pt(cen.X+80, cen.Y)
	to := geom.Pt(cen.X, cen.Y+80)
	d.Inject(input.Pointer{Kind: input.Move, Pos: from})
	d.Inject(input.Pointer{Kind: input.Press, Pos: from, Mods: input.ModShift})
	d.Frame()
	for i := 1; i <= 8; i++ {
		a := float64(i) / 8 * math.Pi / 2
		d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(cen.X+80*float32(math.Cos(a)), cen.Y+80*float32(math.Sin(a)))})
		d.Frame()
	}
	d.Inject(input.Pointer{Kind: input.Release, Pos: to})
	d.Settle()
	if a := w.PreviewAngle(); math.Abs(a-120) > 0.5 {
		t.Fatalf("drag rotate: %v", a)
	}
	if st.pan != (geom.Point{}) {
		t.Fatalf("rotate drag panned: %v", st.pan)
	}
}
