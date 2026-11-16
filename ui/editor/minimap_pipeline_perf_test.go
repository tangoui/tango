package editor

import (
	"bytes"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/paint"
)

func stripFixture(cw int) [minimapStripRows]*miniLine {
	var rows [minimapStripRows]*miniLine
	for _, j := range []int{0, 3, minimapStripRows - 1} {
		b := &miniBitmap{w: 256 - j, h: 2 * cw}
		b.pix = make([]byte, 4*b.w*b.h)
		for k := 0; k < len(b.pix); k += 4 {
			b.pix[k], b.pix[k+1], b.pix[k+2], b.pix[k+3] = byte(k/4), 175, 91, byte(k/4+j)
		}
		rows[j] = &miniLine{bmp: b}
	}
	return rows
}

// Keep the old non-premultiplied composition as an independent pixel oracle.
func legacyMiniStrip(rows *[minimapStripRows]*miniLine, cw int, tall float32) *image.NRGBA {
	h := 2 * cw
	top := func(j int) int { return j * h }
	if tall > 0 {
		pad := max(0, (int(math.Round(float64(tall)))-h)/2)
		top = func(j int) int { return int(math.Round(float64(float32(j)*tall))) + pad }
	}
	w, bottom := 0, 0
	for j, ln := range rows {
		if ln != nil && ln.bmp != nil {
			w, bottom = max(w, ln.bmp.w), top(j)+h
		}
	}
	if w == 0 {
		return nil
	}
	if tall == 0 {
		bottom = minimapStripRows * h
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, bottom))
	for j, ln := range rows {
		if ln == nil || ln.bmp == nil {
			continue
		}
		b := ln.bmp
		for y := range min(h, b.h) {
			copy(img.Pix[(top(j)+y)*img.Stride:], b.pix[y*4*b.w:(y+1)*4*b.w])
		}
	}
	return img
}

func TestMiniStripPremultipliedPixels(t *testing.T) {
	for _, cw := range []int{1, 2} {
		for _, tall := range []float32{0, 7.5, 21.25} {
			rows := stripFixture(cw)
			got := paint.ToRGBA(composeMiniStrip(&rows, cw, tall))
			want := paint.ToRGBA(legacyMiniStrip(&rows, cw, tall))
			if got.Rect != want.Rect || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("cw=%d tall=%g: strip pixels differ", cw, tall)
			}
		}
	}
}

func TestMiniStripNeedsNoRendererConversion(t *testing.T) {
	rows := stripFixture(2)
	img := composeMiniStrip(&rows, 2, 0)
	if ready := paint.PrepareImage(img, 0, 0); image.Image(ready) != image.Image(img) {
		t.Fatal("strip needs a second pixel buffer/conversion before rendering")
	}
}

func TestWarmMinimapPaintAllocationBudget(t *testing.T) {
	goText(t)
	st := NewState(manyLines(400), "a.txt", theme.Base("vs"))
	defer st.Close()
	d, vw := viewDriver(st, 800, 600, nil)
	for range 6 {
		d.Frame()
	}
	w := *vw
	var l paint.List
	w.paintMinimapChars(&l, w.vs.minimapRect)
	images := len(l.Ops)
	if images < 2 {
		t.Fatal("fixture does not exercise multiple strips")
	}
	allocs := testing.AllocsPerRun(30, func() {
		clear(l.Ops)
		l.Ops = l.Ops[:0]
		w.vs.navPaint = w.vs.navPaint[:0]
		w.paintMinimapChars(&l, w.vs.minimapRect)
	})
	if allocs > float64(images+1) {
		t.Fatalf("%.0f allocations for %d cached strip images, want <=%d", allocs, images, images+1)
	}
}

var allocationStrip image.Image

func BenchmarkMiniStripPipeline(b *testing.B) {
	rows := stripFixture(2)
	b.ReportAllocs()
	for b.Loop() {
		allocationStrip = paint.PrepareImage(composeMiniStrip(&rows, 2, 0), 0, 0)
	}
}

// The empty alpha source must not leave color in premultiplied pixels.
func TestMiniStripTransparentRGB(t *testing.T) {
	var rows [minimapStripRows]*miniLine
	rows[0] = &miniLine{bmp: &miniBitmap{w: 1, h: 2, pix: []byte{255, 175, 91, 0, 20, 40, 60, 255}}}
	img := composeMiniStrip(&rows, 1, 0)
	if got := img.At(0, 0); got != (color.RGBA{}) {
		t.Fatalf("transparent pixel = %#v, want zero premultiplied RGB", got)
	}
}
