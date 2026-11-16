//go:build darwin && !ios && arm64

package main

import (
	"bytes"
	"fmt"
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/render/quartz"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"unsafe"
)

var nativeSmokeRenderer paint.Renderer
var nativeSmokePresent func(image.Point, float32) error
var nativeSmokeCanvas platform.Canvas
var nativeSmokeLayer uintptr
var nativeSmokeImage *image.RGBA
var nativeSmokeChecked bool
var nativeSmokeHost *host

func nativeSmokeStart(h *host) error {
	cv, ok := h.cv.(platform.CanvasCoreGraphics)
	if !ok {
		return fmt.Errorf("no CoreGraphics capability")
	}
	var err error
	nativeSmokeLayer, err = cv.UseCoreGraphicsLayer()
	if err != nil {
		return err
	}
	again, err := cv.UseCoreGraphicsLayer()
	if err != nil || again != nativeSmokeLayer {
		return fmt.Errorf("target acquisition not idempotent")
	}
	if len(h.sink.frames) != 0 {
		return fmt.Errorf("target acquisition called frame synchronously")
	}
	if byte(objc.ID(nativeSmokeLayer).Send(objc.RegisterName("isKindOfClass:"), objc.ID(objc.GetClass("CAMetalLayer")))) != 0 {
		return fmt.Errorf("native target aliases Metal layer")
	}
	nativeSmokeRenderer, nativeSmokePresent, err = quartz.NewPresenter(nativeSmokeLayer)
	nativeSmokeCanvas = h.cv
	nativeSmokeHost = h
	if err == nil {
		h.pass("native-hosting", "distinct idempotent retained target, no synchronous frame")
	}
	return err
}
func nativeSmokeFrame(img *image.RGBA, scale float32) error {
	w, h := nativeSmokeCanvas.Size()
	run := text.Shape("TanGo 原生文字 你好", text.Style{Size: 18}, text.Options{})
	// Reuse the immutable source when the smoke's pixels have not changed.
	// This exercises retained image content without rerasterizing a new full
	// frame asset on every scheduling-only invalidation.
	if nativeSmokeImage == nil || nativeSmokeImage.Bounds() != img.Bounds() || !bytes.Equal(nativeSmokeImage.Pix, img.Pix) {
		nativeSmokeImage = img
	}
	l := &paint.List{Ops: []paint.Op{paint.Image{Img: nativeSmokeImage, Rect: geom.Rect{W: w, H: h}}, paint.Text{Origin: geom.Pt(130, 90), Glyphs: run.Glyphs, Color: color.NRGBA{A: 255}}}}
	if err := nativeSmokeRenderer.Render(l, geom.Size{W: w, H: h}, scale); err != nil {
		return err
	}
	if err := nativeSmokePresent(img.Bounds().Size(), scale); err != nil {
		return err
	}
	if !nativeSmokeChecked {
		if err := nativeSmokeSnapshot(img.Bounds().Size(), scale); err != nil {
			return err
		}
		nativeSmokeChecked = true
		nativeSmokeHost.pass("native-pixels", "hosted layer contains full reference gradient and visible Latin/CJK ink")
	}
	return nativeSmokeCanvas.Present(nil)
}
func nativeSmokeFinish(h *host) error {
	if err := nativeSmokeRenderer.(io.Closer).Close(); err != nil {
		return err
	}
	target, err := h.cv.(platform.CanvasMetal).UseMetalLayer()
	if err != nil {
		return err
	}
	if target == nativeSmokeLayer {
		return fmt.Errorf("Metal switch reused native identity")
	}
	// A failed GPU initialization after switching must still accept CPU pixels.
	w, ht := h.cv.Size()
	scale := h.cv.Scale()
	img := image.NewRGBA(image.Rect(0, 0, int(w*scale), int(ht*scale)))
	draw(img, scale, geom.Pt(-100, -100))
	if err := h.cv.Present(img); err != nil {
		return err
	}
	h.pass("native-finish", "closed native renderer, switched to distinct Metal target, CPU recovery visible")
	return nil
}

// Render the owned layer directly: this checks actual hosted pixels without
// requiring system screen-recording permission.
func nativeSmokeSnapshot(size image.Point, scale float32) error {
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	var makeSpace func() uintptr
	var releaseSpace, releaseContext func(uintptr)
	var makeContext func(unsafe.Pointer, int, int, int, int, uintptr, uint32) uintptr
	var translate, scaleCTM func(uintptr, float64, float64)
	purego.RegisterLibFunc(&makeSpace, cg, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&releaseSpace, cg, "CGColorSpaceRelease")
	purego.RegisterLibFunc(&releaseContext, cg, "CGContextRelease")
	purego.RegisterLibFunc(&makeContext, cg, "CGBitmapContextCreate")
	purego.RegisterLibFunc(&translate, cg, "CGContextTranslateCTM")
	purego.RegisterLibFunc(&scaleCTM, cg, "CGContextScaleCTM")
	space := makeSpace()
	defer releaseSpace(space)
	img := image.NewRGBA(image.Rectangle{Max: size})
	ctx := makeContext(unsafe.Pointer(&img.Pix[0]), size.X, size.Y, 8, img.Stride, space, 1|(4<<12))
	if ctx == 0 {
		return fmt.Errorf("snapshot context allocation failed")
	}
	defer releaseContext(ctx)
	translate(ctx, 0, float64(size.Y))
	scaleCTM(ctx, float64(scale), -float64(scale))
	objc.ID(nativeSmokeLayer).Send(objc.RegisterName("renderInContext:"), ctx)
	// The native text rectangle must contain ink, independently of the gradient.
	ink := 0
	for y := int(90 * scale); y < int(118*scale); y++ {
		for x := int(130 * scale); x < int(340*scale); x++ {
			p := img.RGBAAt(x, y)
			if p.R < 50 && p.G < 30 && p.B < 50 && p.A > 200 {
				ink++
			}
		}
	}
	if ink < 100 {
		return fmt.Errorf("hosted native text missing: %d ink pixels", ink)
	}
	for _, pt := range []image.Point{{size.X - 10, 10}, {size.X - 10, size.Y - 10}} {
		got, want := img.RGBAAt(pt.X, pt.Y), nativeSmokeImage.RGBAAt(pt.X, pt.Y)
		if got != want {
			return fmt.Errorf("hosted background differs at %v: %v != %v", pt, got, want)
		}
	}
	if path := os.Getenv("TANGO_NATIVE_SMOKE_IMAGE"); path != "" {
		f, e := os.Create(path)
		if e != nil {
			return e
		}
		e = png.Encode(f, img)
		f.Close()
		return e
	}
	return nil
}
