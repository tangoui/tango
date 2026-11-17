//go:build darwin

package metal

import (
	"errors"
	"image"
	"image/color"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// BenchmarkPresentTrivial measures the process CPU (all threads: Go, the
// Metal driver, Core Animation) per frame of a trivial animated scene on
// an offscreen CAMetalLayer, paced at 60 Hz like the darwin canvas: the
// fixed per-frame cost of Render+present that real-window profiles of
// small component scenes are dominated by. Metric: cpu-us/frame
// (getrusage user+system over the loop / frames), sys-us/frame its
// system part. Arms: idle (pacing only), empty (one empty command buffer
// per frame), render (Render only), full (Render + present). Most of the
// cost is not Go code: each command buffer wakes Metal's submission,
// completion and frame-pacing dispatch queues.
func BenchmarkPresentTrivial(b *testing.B) {
	for _, arm := range []string{"idle", "empty", "render", "full"} {
		b.Run(arm, func(b *testing.B) { presentTrivial(b, arm) })
	}
}

func presentTrivial(b *testing.B, arm string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := loadPresenter(); err != nil {
		b.Skipf("no Metal: %v", err)
	}
	defer autoreleasePool()()
	size := geom.Size{W: 800, H: 600}
	const scale = 2
	px := image.Pt(int(size.W*scale), int(size.H*scale))
	layer := retain(class("CAMetalLayer").Send(sel("layer")))
	if layer == 0 {
		b.Skip("no CAMetalLayer")
	}
	defer release(layer)
	msgSetSize(layer, sel("setDrawableSize:"), cgSize{float64(px.X), float64(px.Y)})
	p, err := newPresenter(uintptr(layer))
	if errors.Is(err, ErrNoDevice) || errors.Is(err, ErrUnsupported) {
		b.Skipf("no Metal device: %v", err)
	}
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close()
	p.r.(*presenterRenderer).SetVerify(false)
	list := func(i int) *paint.List {
		l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}}
		for k := 0; k < 24; k++ {
			x := float32((k*37 + i*3) % 700)
			l.Fill(geom.R(x, float32(k*24), 80, 20), geom.Radii{TL: 4, TR: 4, BL: 4, BR: 4}, color.NRGBA{0x16, 0x77, 0xff, 255})
		}
		return l
	}
	frame := func(i int) {
		if arm == "idle" {
			return
		}
		if arm == "empty" { // one empty command buffer: Metal's fixed submission cost
			defer autoreleasePool()()
			cb := p.dev.queue.Send(sel("commandBuffer"))
			enc := cb.Send(sel("blitCommandEncoder"))
			enc.Send(sel("endEncoding"))
			cb.Send(sel("commit"))
			return
		}
		if err := p.r.Render(list(i), size, scale); err != nil {
			b.Fatal(err)
		}
		if arm == "render" {
			return
		}
		if err := p.present(px, scale); err != nil {
			b.Fatal(err)
		}
	}
	for i := range 10 {
		frame(i)
	}
	if err := p.dev.Wait(); err != nil {
		b.Fatal(err)
	}
	const tick = time.Second / 60
	cpu := func() (time.Duration, time.Duration) {
		var ru syscall.Rusage
		syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
		return time.Duration(ru.Utime.Nano()), time.Duration(ru.Stime.Nano())
	}
	b.ResetTimer()
	u0, s0 := cpu()
	next := time.Now()
	for i := range b.N {
		for now := time.Now(); next.Before(now); {
			next = next.Add(tick)
		}
		time.Sleep(time.Until(next))
		frame(i)
	}
	if err := p.dev.Wait(); err != nil {
		b.Fatal(err)
	}
	u1, s1 := cpu()
	b.ReportMetric(float64(u1-u0+s1-s0)/float64(b.N)/1e3, "cpu-us/frame")
	b.ReportMetric(float64(s1-s0)/float64(b.N)/1e3, "sys-us/frame")
}
