package uihost_test

import (
	"fmt"
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/cpuexec"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/uihost"
)

// concurrentFrames is how many frames each window draws.
const concurrentFrames = 200

// testImage returns a w×h image filled with c.
func testImage(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{c.R, uint8(x * 255 / w), uint8(y * 255 / h), c.A})
		}
	}
	return img
}

// Two windows on their own UI threads draw at the same time: the shared
// layout, text, glyph, image and shadow caches (and the GPU renderer's)
// must have no data races (run with -race).
func TestTwoWindowsRenderConcurrently(t *testing.T) {
	for _, mode := range []string{"cpu", "gpu"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "gpu" {
				t.Setenv("TANGO_RENDERER", "gpu")
				t.Cleanup(uihost.SetGPUHooks(func(uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
					ren := gpu.NewRenderer(cpuexec.NewDevice())
					ren.SetVerify(false)
					return ren, func(image.Point, float32) error { return nil }, nil
				}, nil))
			}
			a, fb := faketest.Run(t, tango.Options{})
			if mode == "gpu" {
				fb.EnableCanvasMetal(0)
			}
			canvases := useThreadedCanvases(t, false, clockCfg{})
			shared := testImage(24, 16, color.NRGBA{200, 0, 0, 255}) // one image shown by both windows
			shadow := []paint.Shadow{{Y: 2, Blur: 6, Color: color.NRGBA{0, 0, 0, 80}}, {X: 1, Blur: 3, Spread: 1, Color: color.NRGBA{20, 40, 200, 60}}}
			page := func(name string, own image.Image) func(*ui.Context) {
				n := 0
				return func(c *ui.Context) {
					n++ // page state: frames of one window run on its UI thread only
					ui.Text(c, fmt.Sprintf("%s frame %d — 窗口 العربية ✓", name, n))
					ui.Text(c, fmt.Sprintf("%d %s", n*7919, name)).Size(float32(12 + n%5)) // several glyph sizes
					ui.Div(c, layout.Size(float32(40+n%30), 20)).Shadow(shadow).Radius(float32(n % 8))
					ui.Image(c, shared).Size(float32(12+n%12), 8)
					ui.Image(c, own).Size(16, float32(8+n%8))
				}
			}
			var wins []opened
			for i, col := range []color.NRGBA{{0, 200, 0, 255}, {0, 0, 200, 255}} {
				name, own := fmt.Sprintf("w%d", i), testImage(16, 16, col)
				o := tango.Await(a, func() opened {
					r, s, err := uihost.Open(a, uihost.Options{
						Surface: tango.SurfaceOptions{Title: name, Width: 240, Height: 160},
					}, page(name, own), ui.RunnerOptions{})
					return opened{r, s, err}
				})
				if o.err != nil {
					t.Fatal(o.err)
				}
				wins = append(wins, o)
			}
			var tcs []*threadedCanvas
			for _, o := range wins {
				for deadline := time.Now().Add(faketest.Timeout); !tango.Await(a, func() bool { return uihost.ThreadedStarted(o.r) }) && time.Now().Before(deadline); {
					fb.Do(func() {})
				}
				tc := canvases(o.s.ID())
				if tc == nil || !tc.started() {
					t.Fatal("no threaded canvas with a running frame clock")
				}
				closeAtEnd(t, fb, o.s, tc)
				tcs = append(tcs, tc)
			}
			start := make(chan struct{})
			var wg sync.WaitGroup
			errs := make(chan error, len(tcs))
			for _, tc := range tcs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					tc.mu.Lock()
					th, fn := tc.th, tc.tickFn
					tc.mu.Unlock()
					for i := 0; i < concurrentFrames; i++ {
						if !th.Do(fn) {
							errs <- fmt.Errorf("UI thread stopped at frame %d", i)
							return
						}
					}
				}()
			}
			close(start)
			wg.Wait()
			close(errs)
			for err := range errs {
				t.Fatal(err)
			}
			for i, tc := range tcs {
				tc.mu.Lock()
				p := tc.presents
				tc.mu.Unlock()
				if p < concurrentFrames {
					t.Fatalf("window %d presented %d frames, want at least %d", i, p, concurrentFrames)
				}
			}
		})
	}
}
