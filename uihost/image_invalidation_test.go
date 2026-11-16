package uihost

import (
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/cpuexec"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

func TestGPUHostImageInvalidationPixels(t *testing.T) {
	for _, mode := range []string{"gpu", "cpu", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			dev := cpuexec.NewDevice()
			dev.SampledOnly = true
			ren := gpu.NewRenderer(dev)
			ren.SetVerify(false)
			host := &gpuRenderer{gpu: ren}
			size := geom.Size{W: 8, H: 8}
			if mode == "cpu" {
				host.cpu = paint.NewCPU()
			}
			if mode == "oversize" {
				size = geom.Size{W: 20000, H: 1}
			}
			img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
			fill := func(c color.NRGBA) {
				for y := 0; y < 2; y++ {
					for x := 0; x < 2; x++ {
						img.SetNRGBA(x, y, c)
					}
				}
			}
			list := &paint.List{}
			list.DrawImage(img, geom.Rect{W: size.W, H: size.H}, geom.Radii{})
			fill(color.NRGBA{180, 40, 20, 255})
			if err := host.Render(list, size, 1); err != nil {
				t.Fatal(err)
			}
			fill(color.NRGBA{20, 140, 80, 255})
			if inv, ok := any(host).(interface{ InvalidateImage(image.Image) }); ok {
				inv.InvalidateImage(img)
			}
			if err := host.Render(list, size, 1); err != nil {
				t.Fatal(err)
			}
			pixels := dev.Target()
			if host.cpu != nil {
				pixels = host.cpu.Image()
			}
			if host.bigFrame {
				pixels = host.big.Image()
			}
			want := color.RGBA{20, 140, 80, 255}
			if got := pixels.RGBAAt(int(size.W/2), int(size.H/2)); got != want {
				t.Fatalf("%s: cached pixels=%v, want %v", mode, got, want)
			}
		})
	}
}
