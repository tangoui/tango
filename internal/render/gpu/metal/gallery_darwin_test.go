//go:build darwin

package metal

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// galleryPage is ui's golden test page (ui/golden_test.go goldenPage,
// which lives in package ui's tests and so cannot be imported), rebuilt
// with the same components through the public ui API.
func galleryPage(c *ui.Context) {
	open := true
	val, empty := "Tango", ""
	ck := true
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Button(c, "主按钮").Primary()
			ui.Button(c, "默认")
			ui.Button(c, "虚线").Dashed()
			ui.Button(c, "危险").Danger()
			ui.Button(c, "搜索").Icon(icon.Search)
			ui.Button(c, "禁用").Disabled(true)
		}, layout.Gap(8))
		ui.Row(c, func() {
			ui.Input(c, &val).Width(160)
			ui.Input(c, &empty).Placeholder("请输入").Status("error").Width(160)
			ui.Checkbox(c, "复选框", &ck)
			ui.Switch(c, &open)
		}, layout.Gap(12), layout.Middle())
		ui.Row(c, func() {
			ui.Tag(c, "默认")
			ui.Tag(c, "成功").Color("success")
			ui.Badge(c).Status("processing", "运行中")
			ui.Badge(c).Count(5).Body(func() { ui.Avatar(c, "U").Square() })
		}, layout.Gap(12), layout.Middle())
		ui.Alert(c, "info", "这是一条提示信息，中文渲染与断行。").Show()
		ui.Progress(c, 60).Layout(layout.W(300))
	}, layout.Pad(16), layout.Gap(16))
}

// TestMetalGallery: the gallery page itself matches paint.CPU (text
// threshold), so the benchmark measures a correct frame.
func TestMetalGallery(t *testing.T) {
	d := newDevice(t)
	size := geom.Size{W: 640, H: 300}
	drv := ui.NewDriver(galleryPage, ui.DriverOptions{Width: size.W, Height: size.H, Scale: 1, Theme: theme.AntDesign()})
	drv.Settle()
	l := drv.Runner().List()
	r := gpu.NewRenderer(d)
	for _, scale := range []float32{1, 2} {
		ref := paint.NewCPU()
		ref.Render(l, size, scale)
		if err := r.Render(l, size, scale); err != nil {
			t.Fatal(err)
		}
		gputest.Check(t, "gallery", l, scale, ref.Image(), readPixels(t, d), 4)
	}
}

// BenchmarkMetalGalleryFrame renders the gallery page (640×300 logical)
// one frame per iteration. gpu-ms is commandBuffer GPUEndTime −
// GPUStartTime of the frame; submit-ms is the CPU time of Renderer.Render
// including Execute up to commit, without waiting for the GPU; exec-ms is
// the Execute part of it (staging, encoding, commit).
func BenchmarkMetalGalleryFrame(b *testing.B) {
	for _, scale := range []float32{1, 2} {
		b.Run(fmt.Sprintf("scale%v", scale), func(b *testing.B) {
			d := newDevice(b)
			size := geom.Size{W: 640, H: 300}
			drv := ui.NewDriver(galleryPage, ui.DriverOptions{Width: size.W, Height: size.H, Scale: scale, Theme: theme.AntDesign()})
			drv.Settle()
			l := drv.Runner().List()
			td := &timedDevice{d: d}
			r := gpu.NewRenderer(td)
			if err := r.Render(l, size, scale); err != nil { // warm the atlas
				b.Fatal(err)
			}
			var gpuT, submitT time.Duration
			td.t = 0
			b.ResetTimer()
			for b.Loop() {
				start := time.Now()
				if err := r.Render(l, size, scale); err != nil {
					b.Fatal(err)
				}
				submitT += time.Since(start)
				g, err := d.wait()
				if err != nil {
					b.Fatal(err)
				}
				gpuT += g
			}
			n := float64(b.N)
			b.ReportMetric(float64(gpuT.Microseconds())/1000/n, "gpu-ms")
			b.ReportMetric(float64(submitT.Microseconds())/1000/n, "submit-ms")
			b.ReportMetric(float64(td.t.Microseconds())/1000/n, "exec-ms")
		})
	}
}

// timedDevice adds up the time spent in Execute.
type timedDevice struct {
	d *Device
	t time.Duration
}

func (td *timedDevice) Execute(f *gpu.Frame) error {
	start := time.Now()
	err := td.d.Execute(f)
	td.t += time.Since(start)
	return err
}
