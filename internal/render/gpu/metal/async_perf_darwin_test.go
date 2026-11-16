//go:build darwin

package metal

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"slices"
	"testing"
	"time"
)

// BenchmarkPresentLoop measures the CPU side of the presenter's frame loop
// as uihost drives it: Render then present, with no Wait in between, on an
// offscreen CAMetalLayer at a Retina window size. The layer has no display,
// so this measures how long the CPU is blocked on command-buffer
// completion (what bounded in-flight changes), not vsync or display
// latency. Metrics: ns/op and the p50/p95/p99 of single iterations; in the
// paced arms the wait for the next tick is excluded from both.
func BenchmarkPresentLoop(b *testing.B) {
	for _, arm := range []struct {
		name  string
		drain bool
	}{{"drain", true}, {"a1", false}} {
		b.Run(arm.name+"/editor", func(b *testing.B) { presentLoop(b, arm.drain, true, 0) })
		b.Run(arm.name+"/editor-nosync", func(b *testing.B) { presentLoop(b, arm.drain, false, 0) })
	}
	// Paced like the app: one frame per display tick (the darwin canvas
	// renders from a CADisplayLink), not back to back. 120 Hz and 60 Hz.
	for _, hz := range []int{120, 60} {
		b.Run(fmt.Sprintf("a1/editor-paced%d", hz), func(b *testing.B) {
			presentLoop(b, false, true, time.Second/time.Duration(hz))
		})
	}
}

// presentLoop runs the loop; with vsync false the layer's displaySyncEnabled
// is turned off, so nextDrawable stops pacing the loop to the display and
// what is left is CPU encode plus command-buffer back-pressure. A non-zero
// tick starts each frame on its own tick of that period (skipping ticks a
// slow frame overran), as a display link would.
func presentLoop(b *testing.B, drain, vsync bool, tick time.Duration) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := loadPresenter(); err != nil {
		b.Skipf("no Metal: %v", err)
	}
	defer autoreleasePool()()
	size := windowSize
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
	msgSetBool(layer, sel("setDisplaySyncEnabled:"), vsync)
	p.r.(*presenterRenderer).SetVerify(false) // test binaries verify every frame against paint.CPU

	p.dev.mu.Lock()
	p.dev.drainOnExecute = drain
	p.dev.mu.Unlock()
	l := editorList(b)
	frame := func() {
		if err := p.r.Render(l, size, scale); err != nil {
			b.Fatal(err)
		}
		if err := p.present(px, scale); err != nil {
			b.Fatal(err)
		}
	}
	for range 10 { // pipelines, textures, atlas
		frame()
	}
	if err := p.dev.Wait(); err != nil {
		b.Fatal(err)
	}
	var frameGPU, presentGPU, drawWait []time.Duration
	p.dev.SetTimeline(func(t Timing) {
		switch {
		case t.Kind == "frame" && t.GPUKnown:
			frameGPU = append(frameGPU, t.GPUEnd-t.GPUStart)
		case t.Kind == "present":
			drawWait = append(drawWait, t.DrawableWait)
			if t.GPUKnown {
				presentGPU = append(presentGPU, t.GPUEnd-t.GPUStart)
			}
		}
	})
	lat := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	next := time.Now()
	for range b.N {
		if tick > 0 {
			// The wait for the tick is not frame work: keep it out of
			// ns/op, which otherwise would just be the tick period.
			b.StopTimer()
			for now := time.Now(); next.Before(now); {
				next = next.Add(tick) // the first tick not yet passed
			}
			time.Sleep(time.Until(next))
			b.StartTimer()
		}
		t := time.Now()
		frame()
		lat = append(lat, time.Since(t))
	}
	b.StopTimer()
	if err := p.dev.Wait(); err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(pct(lat, .50), "p50-ms")
	b.ReportMetric(pct(lat, .95), "p95-ms")
	b.ReportMetric(pct(lat, .99), "p99-ms")
	b.ReportMetric(pct(frameGPU, .50), "frame-gpu-p50-ms")
	b.ReportMetric(pct(presentGPU, .50), "present-gpu-p50-ms")
	b.ReportMetric(pct(drawWait, .50), "drawable-wait-p50-ms")
	b.ReportMetric(pct(drawWait, .95), "drawable-wait-p95-ms")
}

// pct returns the f quantile of d in milliseconds (0 for none); d is sorted.
func pct(d []time.Duration, f float64) float64 {
	if len(d) == 0 {
		return 0
	}
	slices.Sort(d)
	return float64(d[int(f*float64(len(d)-1))]) / 1e6
}
