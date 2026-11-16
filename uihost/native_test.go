package uihost

import (
	"errors"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"image"
	"testing"
	"time"
)

type nativeCanvasTest struct {
	platform.Canvas
	presented   *image.RGBA
	nilPresents int
	targets     int
}

func (*nativeCanvasTest) Size() (float32, float32) { return 100, 100 }
func (*nativeCanvasTest) Scale() float32           { return 1 }
func (*nativeCanvasTest) Invalidate()              {}
func (c *nativeCanvasTest) Present(i *image.RGBA) error {
	c.presented = i
	if i == nil {
		c.nilPresents++
	}
	return nil
}
func (c *nativeCanvasTest) UseCoreGraphicsLayer() (uintptr, error) { c.targets++; return 42, nil }

type nativeRenderTest struct {
	closes, invalidations int
	work                  time.Duration
	full                  bool
}

func (*nativeRenderTest) Render(*paint.List, geom.Size, float32) error { return nil }
func (r *nativeRenderTest) Close() error                               { r.closes++; return nil }
func (r *nativeRenderTest) InvalidateImage(image.Image)                { r.invalidations++ }
func (r *nativeRenderTest) FrameMetrics() (time.Duration, bool)        { return r.work, r.full }
func nativeTestHooks(t *testing.T, r *nativeRenderTest) {
	t.Helper()
	old := nativeNewPresenter
	nativeNewPresenter = func(target uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		if target != 42 {
			t.Fatalf("target %d", target)
		}
		return r, func(image.Point, float32) error { return nil }, nil
	}
	t.Cleanup(func() { nativeNewPresenter = old })
}
func TestNativePolicyMeasuresIdleGapsAndSustainedCost(t *testing.T) {
	for _, tc := range []struct {
		name       string
		work, gap  time.Duration
		full, want bool
	}{
		{"slow", 9 * time.Millisecond, 10 * time.Millisecond, false, true},
		{"very slow", 200 * time.Millisecond, 10 * time.Millisecond, false, true},
		{"cold", 9 * time.Millisecond, time.Second, false, false},
		{"fast", time.Millisecond, 10 * time.Millisecond, false, false},
		{"fallback", time.Millisecond, 10 * time.Millisecond, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p nativeRendererState
			now := time.Unix(100, 0)
			for i := 0; i < 6; i++ {
				start := now
				now = now.Add(tc.work)
				got := p.record(tc.work, tc.full, start, now)
				if got != (tc.want && i == 5) {
					t.Fatalf("frame %d promoted=%v", i, got)
				}
				now = now.Add(tc.gap)
			}
		})
	}
}
func TestNativeSelectionAndExplicitModes(t *testing.T) {
	r := &nativeRenderTest{}
	nativeTestHooks(t, r)
	oldTarget, oldNP := gpuTarget, newPresenter
	gpuCalls := 0
	gpuTarget = func(platform.Canvas) (uintptr, error) { gpuCalls++; return 7, nil }
	newPresenter = func(uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		return &nativeRenderTest{}, func(image.Point, float32) error { return nil }, nil
	}
	t.Cleanup(func() { gpuTarget, newPresenter = oldTarget, oldNP })
	for _, mode := range []string{"", "native", "cpu", "gpu", "auto"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("TANGO_RENDERER", mode)
			cv := &nativeCanvasTest{}
			g := setupGPU(cv, ui.RunnerOptions{})
			if mode == "" || mode == "native" {
				if g == nil || !g.nativeMode {
					t.Fatal("native not selected")
				}
				if (g.native != nil) != (mode == "") {
					t.Fatal("force-native promotion policy")
				}
				runner := &ui.Runner{}
				renderers.Store(runner, g)
				defer renderers.Delete(runner)
				if Renderer(runner) != "native" {
					t.Fatal("incorrect backend report")
				}
				g.Close()
			} else if cv.targets != 0 {
				t.Fatal("explicit mode acquired native target")
			}
		})
	}
	t.Setenv("TANGO_RENDERER", "")
	cv := &nativeCanvasTest{}
	if setupGPU(cv, ui.RunnerOptions{Renderer: paint.NewCPU()}) != nil || cv.targets != 0 {
		t.Fatal("caller renderer overridden")
	}
}
func TestNativePromotionFailurePresentsCPUAndTriesOnlyOnce(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	r := &nativeRenderTest{}
	nativeTestHooks(t, r)
	cv := &nativeCanvasTest{}
	g := setupGPU(cv, ui.RunnerOptions{})
	defer g.Close()
	old := gpuTarget
	calls := 0
	gpuTarget = func(platform.Canvas) (uintptr, error) { calls++; return 0, errors.New("no GPU") }
	t.Cleanup(func() { gpuTarget = old })
	g.native.ready = true
	l := &paint.List{}
	for i := 0; i < 3; i++ {
		if err := g.Render(l, geom.Size{W: 100, H: 100}, 1); err != nil {
			t.Fatal(err)
		}
		if err := g.presentFrame(cv, 1); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 || r.closes != 1 || cv.presented == nil || g.nativeMode {
		t.Fatalf("calls=%d closes=%d visible=%v native=%v", calls, r.closes, cv.presented != nil, g.nativeMode)
	}
}
func TestNativeWrapperForwardsImageInvalidation(t *testing.T) {
	r := &nativeRenderTest{}
	g := &gpuRenderer{gpu: r, nativeMode: true}
	g.InvalidateImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	if r.invalidations != 1 {
		t.Fatal("invalidation dropped")
	}
}
