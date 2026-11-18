package uihost

import (
	"errors"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
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

type largeNativeCanvasTest struct{ nativeCanvasTest }

func (*largeNativeCanvasTest) Size() (float32, float32) { return 1000, 800 }
func TestNativeDefaultCoversLargeWindows(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	r := &nativeRenderTest{}
	nativeTestHooks(t, r)
	gpuCalls := metalTestHooks(t, nil)
	cv := &largeNativeCanvasTest{}
	g := setupGPU(cv, ui.RunnerOptions{})
	if g == nil || !g.nativeMode || cv.targets != 1 || *gpuCalls != 0 {
		t.Fatalf("large window: native=%v targets=%d gpu calls=%d, want native layers without Metal", g != nil && g.nativeMode, cv.targets, *gpuCalls)
	}
	if g.native == nil {
		t.Fatal("default native on a large window lost its promotion to Metal")
	}
	g.Close()
	t.Setenv("TANGO_RENDERER", "native")
	forced := setupGPU(cv, ui.RunnerOptions{})
	if forced == nil || !forced.nativeMode || forced.native != nil {
		t.Fatal("explicit native mode should remain available without promotion")
	}
	forced.Close()
}

// metalTestHooks stands in for Metal target acquisition and presenter
// creation; targetErr makes every acquisition fail. It returns the number
// of acquisitions.
func metalTestHooks(t *testing.T, targetErr error) *int {
	t.Helper()
	oldTarget, oldNP := gpuTarget, newPresenter
	calls := 0
	gpuTarget = func(platform.Canvas) (uintptr, error) {
		calls++
		if targetErr != nil {
			return 0, targetErr
		}
		return 7, nil
	}
	newPresenter = func(uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		return &nativeRenderTest{}, func(image.Point, float32) error { return nil }, nil
	}
	t.Cleanup(func() { gpuTarget, newPresenter = oldTarget, oldNP })
	return &calls
}

// nativeKeyboardSetup returns a default (promotable) native renderer on a
// large window with Metal stubbed out.
func nativeKeyboardSetup(t *testing.T, mode string, targetErr error) (*gpuRenderer, *largeNativeCanvasTest, *int) {
	t.Helper()
	return nativeContentSetup(t, mode, ui.ContentGeneral, &nativeRenderTest{}, targetErr)
}

// nativeContentSetup is nativeKeyboardSetup for a window showing content,
// drawn natively by r.
func nativeContentSetup(t *testing.T, mode string, content ui.ContentKind, r *nativeRenderTest, targetErr error) (*gpuRenderer, *largeNativeCanvasTest, *int) {
	t.Helper()
	t.Setenv("TANGO_RENDERER", mode)
	nativeTestHooks(t, r)
	calls := metalTestHooks(t, targetErr)
	cv := &largeNativeCanvasTest{}
	g := setupGPU(cv, ui.RunnerOptions{Content: content})
	if g == nil || !g.nativeMode {
		t.Fatalf("TANGO_RENDERER=%q: native not selected", mode)
	}
	t.Cleanup(func() { g.Close() })
	return g, cv, calls
}

func renderNativeTestFrame(t *testing.T, g *gpuRenderer, cv platform.Canvas) {
	t.Helper()
	if err := g.Render(&paint.List{}, geom.Size{W: 1000, H: 800}, 1); err != nil {
		t.Fatal(err)
	}
	if err := g.presentFrame(cv, 1); err != nil {
		t.Fatal(err)
	}
}

func TestKeyEventPromotesOnNextFrame(t *testing.T) {
	g, cv, calls := nativeKeyboardSetup(t, "", nil)
	renderNativeTestFrame(t, g, cv) // an ordinary native frame first
	for i := 0; i < 3; i++ {        // a held key: press, then two auto-repeats
		ev := input.KeyEvent{Key: input.KeyA, Down: true, Repeat: i > 0}
		if !isKeyboardInput(ev) {
			t.Fatalf("press %d not keyboard input", i)
		}
		g.keyboardInput()
	}
	if !g.nativeMode || *calls != 0 {
		t.Fatal("keyboard input promoted before the next frame")
	}
	renderNativeTestFrame(t, g, cv)
	if g.nativeMode || g.gpu == nil || *calls != 1 {
		t.Fatalf("after a key: native=%v gpu=%v metal targets=%d, want Metal", g.nativeMode, g.gpu != nil, *calls)
	}
	g.keyboardInput()
	renderNativeTestFrame(t, g, cv)
	if *calls != 1 {
		t.Fatalf("promoted %d times, want once", *calls)
	}
}

func TestIsKeyboardInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		ev   input.Event
		want bool
	}{
		{"key down", input.KeyEvent{Key: input.KeyA, Down: true}, true},
		{"key repeat", input.KeyEvent{Key: input.KeyA, Down: true, Repeat: true}, true},
		{"shortcut", input.KeyEvent{Key: input.KeyS, Down: true, Mods: input.ModMeta}, true},
		{"key up", input.KeyEvent{Key: input.KeyA}, false},
		{"shift", input.KeyEvent{Key: input.KeyShiftLeft, Down: true, Mods: input.ModShift}, false},
		{"command", input.KeyEvent{Key: input.KeyMetaLeft, Down: true, Mods: input.ModMeta}, false},
		{"caps lock", input.KeyEvent{Key: input.KeyCapsLock, Down: true}, false},
		{"unknown key", input.KeyEvent{Down: true}, false},
		{"text", input.TextInput{Text: "a"}, true},
		{"empty text", input.TextInput{}, false},
		{"composition", input.Composition{Text: "ni", Cursor: 2}, true},
		{"composition end", input.Composition{}, false},
		{"scroll", input.Scroll{DY: 3}, false},
		{"pointer", input.Pointer{}, false},
		{"touch", input.TouchEvent{}, false},
		{"focus", input.WindowFocus{Focused: true}, false},
	} {
		if got := isKeyboardInput(tc.ev); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestForcedNativeIgnoresKeyboard(t *testing.T) {
	g, cv, calls := nativeKeyboardSetup(t, "native", nil)
	g.keyboardInput()
	renderNativeTestFrame(t, g, cv)
	if !g.nativeMode || *calls != 0 {
		t.Fatalf("forced native: native=%v metal targets=%d, want native", g.nativeMode, *calls)
	}
}

func TestKeyboardPromotesOnlyThatWindow(t *testing.T) {
	typed, cv1, _ := nativeKeyboardSetup(t, "", nil)
	viewed, cv2, _ := nativeKeyboardSetup(t, "", nil)
	typed.keyboardInput()
	renderNativeTestFrame(t, typed, cv1)
	renderNativeTestFrame(t, viewed, cv2)
	if typed.nativeMode || !viewed.nativeMode || viewed.native == nil {
		t.Fatalf("typed native=%v, viewed native=%v: want only the typed window on Metal", typed.nativeMode, viewed.nativeMode)
	}
}

func TestKeyBeforeFirstFrameStillPromotes(t *testing.T) {
	var none *gpuRenderer // a CPU window, or one whose renderer does not exist yet
	none.keyboardInput()
	g, cv, calls := nativeKeyboardSetup(t, "", nil)
	g.keyboardInput()
	renderNativeTestFrame(t, g, cv)
	if g.nativeMode || g.gpu == nil || *calls != 1 {
		t.Fatalf("first frame: native=%v metal targets=%d, want Metal", g.nativeMode, *calls)
	}
}

func TestKeyAfterFailedPromotionDoesNotRetry(t *testing.T) {
	g, cv, calls := nativeKeyboardSetup(t, "", errors.New("no GPU"))
	g.keyboardInput()
	renderNativeTestFrame(t, g, cv)
	if g.cpu == nil || g.nativeMode || *calls != 1 || cv.presented == nil {
		t.Fatalf("failed promotion: cpu=%v native=%v targets=%d visible=%v", g.cpu != nil, g.nativeMode, *calls, cv.presented != nil)
	}
	g.keyboardInput()
	renderNativeTestFrame(t, g, cv)
	if *calls != 1 {
		t.Fatalf("retried Metal after a failed promotion: %d targets", *calls)
	}
}

func TestCanvasInputRoutesKeyboard(t *testing.T) {
	g, _, _ := nativeKeyboardSetup(t, "", nil)
	h := &host{r: ui.NewRunner(func(*ui.Context) {}, ui.RunnerOptions{Renderer: g}), gpu: g}
	h.CanvasInput(input.Scroll{DY: 3})
	h.CanvasInput(input.Pointer{Kind: input.Move})
	h.CanvasInput(input.KeyEvent{Key: input.KeyShiftLeft, Down: true, Mods: input.ModShift})
	if g.native.ready {
		t.Fatal("scroll, pointer or a modifier asked for Metal")
	}
	h.CanvasInput(input.KeyEvent{Key: input.KeyA, Down: true})
	if !g.native.ready {
		t.Fatal("key press did not ask for Metal")
	}
	(&host{}).CanvasInput(input.KeyEvent{Key: input.KeyA, Down: true}) // no surface yet
}

func TestDataWindowsUseNativeLayersAtAnySize(t *testing.T) {
	r := &nativeRenderTest{}
	nativeTestHooks(t, r)
	gpuCalls := metalTestHooks(t, nil)

	t.Setenv("TANGO_RENDERER", "")
	cv := &largeNativeCanvasTest{}
	g := setupGPU(cv, ui.RunnerOptions{Content: ui.ContentData})
	if g == nil || !g.nativeMode || cv.targets != 1 || *gpuCalls != 0 {
		t.Fatalf("large data window: native=%v targets=%d gpu calls=%d, want native layers without Metal", g != nil && g.nativeMode, cv.targets, *gpuCalls)
	}
	if g.native == nil {
		t.Fatal("data window lost the promotion to Metal for expensive frames")
	}
	g.Close()

	general := &largeNativeCanvasTest{}
	g = setupGPU(general, ui.RunnerOptions{})
	if g == nil || !g.nativeMode || general.targets != 1 || *gpuCalls != 0 {
		t.Fatal("large general window did not start on native layers")
	}
	g.Close()

	for _, mode := range []string{"gpu", "cpu", "auto"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("TANGO_RENDERER", mode)
			cv := &largeNativeCanvasTest{}
			g := setupGPU(cv, ui.RunnerOptions{Content: ui.ContentData})
			if g != nil {
				defer g.Close()
			}
			if cv.targets != 0 {
				t.Fatalf("TANGO_RENDERER=%s: data window took native layers", mode)
			}
		})
	}
}

// A fast presented frame between the key and the next Render must not
// cancel the keyboard's request for Metal.
func TestKeyboardRequestSurvivesFrameRecord(t *testing.T) {
	g, cv, calls := nativeKeyboardSetup(t, "", nil)
	g.keyboardInput()
	now := time.Unix(100, 0)
	if !g.native.record(time.Millisecond, false, now, now.Add(time.Millisecond)) || !g.native.ready {
		t.Fatal("a cheap frame cleared the keyboard promotion request")
	}
	renderNativeTestFrame(t, g, cv)
	if g.nativeMode || *calls != 1 {
		t.Fatalf("after record: native=%v metal targets=%d, want Metal", g.nativeMode, *calls)
	}
}

func TestContentGeneralPromotesOnKeyboard(t *testing.T) {
	g, cv, calls := nativeContentSetup(t, "", ui.ContentGeneral, &nativeRenderTest{}, nil)
	g.keyboardInput()
	renderNativeTestFrame(t, g, cv)
	if g.nativeMode || g.gpu == nil || *calls != 1 {
		t.Fatalf("general window after a key: native=%v metal targets=%d, want Metal", g.nativeMode, *calls)
	}
}

// Data windows (tables, quote boards, dashboards) are often browsed with
// the keyboard; only sustained frame cost moves them to Metal.
func TestContentDataIgnoresKeyboard(t *testing.T) {
	r := &nativeRenderTest{}
	g, cv, calls := nativeContentSetup(t, "", ui.ContentData, r, nil)
	for _, ev := range []input.Event{
		input.KeyEvent{Key: input.KeyDown, Down: true},
		input.KeyEvent{Key: input.KeyA, Down: true},
		input.TextInput{Text: "a"},
	} {
		if !isKeyboardInput(ev) {
			t.Fatalf("%#v not keyboard input", ev)
		}
		g.keyboardInput()
		renderNativeTestFrame(t, g, cv)
	}
	if !g.nativeMode || g.native == nil || *calls != 0 {
		t.Fatalf("data window after keys: native=%v metal targets=%d, want native", g.nativeMode, *calls)
	}
	r.work = 9 * time.Millisecond // six expensive frames in a row
	for i := 0; i < 7 && g.nativeMode; i++ {
		renderNativeTestFrame(t, g, cv)
	}
	if g.nativeMode || *calls != 1 {
		t.Fatalf("data window after slow frames: native=%v metal targets=%d, want Metal", g.nativeMode, *calls)
	}
}

// A default window whose native presenter cannot start takes Metal, the
// renderer it would otherwise reach on its first key press; only when Metal
// is unavailable too does it draw on the CPU.
func TestNativePresenterFailureFallsBackToMetal(t *testing.T) {
	t.Setenv("TANGO_RENDERER", "")
	old := nativeNewPresenter
	nativeNewPresenter = func(uintptr) (paint.Renderer, func(image.Point, float32) error, error) {
		return nil, nil, errors.New("no native presenter")
	}
	t.Cleanup(func() { nativeNewPresenter = old })
	calls := metalTestHooks(t, nil)
	cv := &largeNativeCanvasTest{}
	g := setupGPU(cv, ui.RunnerOptions{})
	if g == nil || g.nativeMode || g.gpu == nil || g.cpu != nil || cv.targets != 1 || *calls != 1 {
		t.Fatalf("presenter failure: renderer=%v native=%v gpu=%v cpu=%v targets=%d metal=%d, want Metal",
			g != nil, g != nil && g.nativeMode, g != nil && g.gpu != nil, g != nil && g.cpu != nil, cv.targets, *calls)
	}
	g.Close()

	metalTestHooks(t, errors.New("no GPU"))
	g = setupGPU(&largeNativeCanvasTest{}, ui.RunnerOptions{})
	if g == nil || g.cpu == nil || g.nativeMode {
		t.Fatal("presenter and Metal failure: want a visible CPU renderer")
	}
	g.Close()
}

func TestKeyboardPromotesGeneralNotData(t *testing.T) {
	general, cvG, _ := nativeContentSetup(t, "", ui.ContentGeneral, &nativeRenderTest{}, nil)
	data, cvD, _ := nativeContentSetup(t, "", ui.ContentData, &nativeRenderTest{}, nil)
	general.keyboardInput()
	data.keyboardInput()
	renderNativeTestFrame(t, general, cvG)
	renderNativeTestFrame(t, data, cvD)
	if general.nativeMode || general.gpu == nil {
		t.Fatal("general window stayed native after a key")
	}
	if !data.nativeMode || data.native == nil {
		t.Fatal("data window left native layers on a key")
	}
}
