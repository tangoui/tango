//go:build darwin && !ios

package darwin

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/uithread"
)

func TestFrameAtOnceSemanticsOnUIThread(t *testing.T) {
	th := uithread.Start("pacing")
	defer th.Stop()
	frames := make(chan time.Time, 8)
	clock := newThreadFrameClock(th, func() {
		if !th.On() {
			t.Error("frame off UI thread")
		}
		frames <- time.Now()
	}, func(bool) {}) // manually feed display refreshes
	defer th.Do(clock.stop)
	before := time.Now()
	clock.request()
	th.Do(func() {})
	select {
	case <-frames:
	default:
		t.Fatal("first request waited for refresh")
	}
	clock.request()
	th.Do(func() {})
	select {
	case <-frames:
		t.Fatal("second frame without refresh")
	default:
	}
	th.Do(clock.refresh)
	select {
	case <-frames:
	default:
		t.Fatal("pending frame did not run at refresh")
	}
	th.Do(clock.refresh) // idle refresh
	clock.requestAt(before.Add(25 * time.Millisecond))
	select {
	case at := <-frames:
		if at.Before(before.Add(25 * time.Millisecond)) {
			t.Fatal("deadline ran early")
		}
	case <-time.After(time.Second):
		t.Fatal("deadline needs a main-thread wake")
	}
}

func init() {
	if os.Getenv("TANGO_TASK7_CANCEL_CHILD") != "" {
		runtime.LockOSThread()
		MainThreadTestHook = func() (code int, handled bool) {
			handled = true
			e := &cancelErr{}
			defer func() {
				if v := recover(); v != nil && v != e {
					panic(v)
				}
				if e.err != nil {
					fmt.Fprintln(os.Stderr, e.err)
					code = 1
				}
			}()
			checkCancelThreaded(e)
			return 0, handled
		}
		return
	}
	if os.Getenv("TANGO_TASK7_CLOCK_CHILD") != "" {
		runtime.LockOSThread()
		MainThreadTestHook = func() (int, bool) {
			if err := checkDisplayLink(os.Getenv("TANGO_TASK7_CLOCK_CHILD") == "corevideo"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1, true
			}
			return 0, true
		}
	}
}

func TestDisplayLinkOnUIThreadRunLoop(t *testing.T) {
	if os.Getenv("TANGO_TEST_TASK7_GUI") != "1" || testing.Short() {
		t.Skip("requires coordinated real 60 Hz display")
	}
	for _, name := range []string{"appkit", "corevideo"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
			cmd.Env = append(os.Environ(), "TANGO_TASK7_CLOCK_CHILD="+name)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			} else {
				t.Logf("%s", out)
			}
		})
	}
}

func checkDisplayLink(corevideo bool) error {
	b := New().(*backend)
	if err := b.Init(&shortcutTestHost{}, platform.AppOptions{Name: "Tango UI frame clock test"}); err != nil {
		return err
	}
	send(b.app, "finishLaunching")
	s, cv, err := b.NewCanvasSurface(1, platform.SurfaceOptions{Title: "UI frame clock test", Width: 320, Height: 200, Visible: true}, platform.SurfaceExtras{}, &shortcutTestSink{})
	if err != nil {
		return err
	}
	c := ThreadedCanvas(cv).(*threadedCanvas)
	c.useCoreVideo = corevideo
	defer func() { s.Close(); c.ReleaseThreadedOnMain() }()
	if !corevideo && c.threadLink == 0 {
		return fmt.Errorf("CADisplayLink requires macOS 14+")
	}
	th := uithread.Start("real display link")
	var request, stop func()
	var wrong atomic.Bool
	var count atomic.Int64
	th.Do(func() {
		request, stop, err = c.StartFrameClock(func() {
			if !th.On() || isMainThread() {
				wrong.Store(true)
			}
			count.Add(1)
			request()
		})
		if !corevideo && err == nil {
			// Check the actual ObjC callback, not merely the queued frame.
			var pthreadThreadID func(uintptr, *uint64) int32
			purego.RegisterLibFunc(&pthreadThreadID, purego.RTLD_DEFAULT, "pthread_threadid_np")
			threadID := func() uint64 { var id uint64; pthreadThreadID(0, &id); return id }
			owner := threadID()
			threadLinks.Store(c.threadLinkTarget, func() {
				if threadID() != owner {
					wrong.Store(true)
				}
				c.threadClock.Load().queueRefresh()
			})
		}
	})
	defer func() {
		if stop != nil {
			th.Do(stop)
		}
		th.Stop()
		<-th.Done()
	}()
	if err != nil {
		return err
	}
	if corevideo {
		found := false
		cvClocks.Range(func(_, v any) bool { found = v == c.threadClock.Load(); return !found })
		if !found {
			return fmt.Errorf("CoreVideo fell back to timer")
		}
	}
	request()
	wait := func(d time.Duration) {
		for deadline := time.Now().Add(d); time.Now().Before(deadline); {
			b.Step(false)
			time.Sleep(time.Millisecond)
		}
	}
	wait(250 * time.Millisecond)
	start := count.Load()
	wait(time.Second)
	n := count.Load() - start
	if wrong.Load() || n < 55 || n > 65 {
		return fmt.Errorf("display link: %d frames/s, wrong thread=%v", n, wrong.Load())
	}
	fmt.Printf("display link: %d frames/s, correct UI thread\n", n)
	return nil
}

func TestThreadFrameClockQueuesRefreshAndStopsRequests(t *testing.T) {
	th := uithread.Start("serialized frame clock")
	defer th.Stop()
	var clock *threadFrameClock
	frames := 0 // UI thread only
	clock = newThreadFrameClock(th, func() {
		frames++
		if frames == 1 {
			clock.request()
			clock.queueRefresh() // even a callback in a nested run loop must queue
			if frames != 1 {
				t.Error("refresh reentered the current frame")
			}
		}
	}, func(bool) {})
	clock.request()
	th.Do(func() {})
	th.Do(func() {
		if frames != 2 {
			t.Errorf("queued callback produced %d frames, want 2", frames)
		}
		clock.stop()
	})
	var calls sync.WaitGroup
	for range 10 {
		calls.Go(func() { clock.queueRefresh(); clock.request(); clock.requestAt(time.Now()) })
	}
	calls.Wait()
	th.Do(func() {
		if frames != 2 {
			t.Error("request reached tick after cancellation")
		}
	})
}

func TestThreadClockPreservesFutureDeadline(t *testing.T) {
	th := uithread.Start("deadline")
	defer th.Stop()
	frames := 0
	clock := newThreadFrameClock(th, func() { frames++ }, func(bool) {})
	deadline := time.Now().Add(time.Hour)
	clock.requestAt(deadline)
	clock.request()
	th.Do(func() {
		if frames != 1 || clock.deadline != deadline {
			t.Errorf("ordinary frame consumed independent deadline: %v", clock.deadline)
		}
		clock.stop()
	})
}

func TestThreadClockQueuedOldRefreshCannotRestoreImmediateFrame(t *testing.T) {
	th := uithread.Start("old refresh")
	defer th.Stop()
	frames := 0
	clock := newThreadFrameClock(th, func() { frames++ }, func(bool) {})
	th.Do(func() {
		clock.request()      // queued immediate frame
		clock.queueRefresh() // issued before that frame actually ran
		clock.th.Post(clock.requestOnUI)
	})
	th.Do(func() {})
	th.Do(func() {
		if frames != 1 {
			t.Errorf("old refresh gave a second immediate frame: %d", frames)
		}
		clock.stop()
	})
}

func TestCoreVideoRestartFailureFallsBackToTimer(t *testing.T) {
	cvOnce.Do(func() {})
	oldCreate, oldOutput, oldStart, oldStop, oldRelease := cvCreate, cvOutput, cvStart, cvStop, cvRelease
	defer func() {
		cvCreate, cvOutput, cvStart, cvStop, cvRelease = oldCreate, oldOutput, oldStart, oldStop, oldRelease
	}()
	var starts atomic.Int32
	cvCreate = func(link *uintptr) int32 { *link = 1; return 0 }
	cvOutput = func(_, _, _ uintptr) int32 { return 0 }
	cvStart = func(uintptr) int32 {
		if starts.Add(1) == 1 {
			return 0 // probe at startup
		}
		return -1 // display went away while paused
	}
	cvStop = func(uintptr) int32 { return 0 }
	cvRelease = func(uintptr) {}
	th := uithread.Start("cv restart")
	defer th.Stop()
	frames := make(chan struct{}, 8)
	c := &threadedCanvas{canvas: &canvas{}}
	var request, stop func()
	var err error
	th.Do(func() { request, stop, err = c.startCoreVideo(th, func() { frames <- struct{}{} }) })
	if err != nil {
		t.Fatal(err)
	}
	defer th.Do(stop)
	deadline := time.Now().Add(time.Hour)
	c.threadClock.Load().requestAt(deadline)
	request() // immediate frame; the restart fails
	<-frames
	request() // waits for a refresh, which only the fallback timer can provide
	select {
	case <-frames:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("failed CVDisplayLink restart dropped pending request")
	}
	th.Do(func() {
		if c.threadClock.Load().deadline != deadline {
			t.Error("fallback lost InvalidateAt deadline")
		}
	})
}

// A CPU frame and its partial updates use the scale the pixels were drawn
// at, in a UI-owned child layer; the main-owned root keeps its geometry.
func TestThreadedCPUPresentUsesFrameGeometry(t *testing.T) {
	load()
	loadCanvas()
	withPool(func() {
		root := retain(send(class("CALayer"), "layer"))
		defer release(root)
		rootBounds := nsRect{Size: nsSize{Width: 300, Height: 200}}
		msgSetRect(root, sel("setBounds:"), rootBounds)
		msgSetFloat(root, sel("setContentsScale:"), 1)
		th := uithread.Start("cpu geometry")
		defer th.Stop()
		c := &canvas{threadPrepared: true, threadCPULayer: root, scale: 1}
		c.publishGeometry() // main geometry moved on to scale 1
		c.threadClock.Store(newThreadFrameClock(th, func() {}, func(bool) {}))
		img := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
		for i := range img.Pix {
			img.Pix[i] = 0xff
		}
		c.SetFrameScale(2) // the frame was rendered at scale 2
		if err := c.Present(img); err != nil {
			t.Fatal(err)
		}
		content := c.cpuContentLayer()
		if content == root {
			t.Fatal("UI frame wrote the main-owned root layer")
		}
		if got := msgGetFloat(content, sel("contentsScale")); got != 2 {
			t.Fatalf("contentsScale %v, want the frame's 2", got)
		}
		if got := msgRect(content, sel("bounds")).Size; got != (nsSize{Width: 512, Height: 512}) {
			t.Fatalf("content bounds %v, want 512x512 points", got)
		}
		if msgRect(root, sel("bounds")) != rootBounds || msgGetFloat(root, sel("contentsScale")) != 1 {
			t.Fatal("UI frame changed root geometry")
		}
		if err := c.PresentDamage(img, []image.Rectangle{image.Rect(260, 260, 270, 270)}); err != nil {
			t.Fatal(err)
		}
		if len(c.tiles.shown) != 1 {
			t.Fatalf("shown tiles %v", c.tiles.shown)
		}
		f := msgRect(c.tiles.layers[c.tiles.shown[0]], sel("frame"))
		if f.Origin.X != 128 || f.Size.Width != 128 {
			t.Fatalf("tile frame %v uses another generation's scale", f)
		}
		// A partial frame of another scale is never composed onto this one.
		c.SetFrameScale(1)
		if err := c.PresentDamage(img, []image.Rectangle{image.Rect(0, 0, 4, 4)}); err != nil {
			t.Fatal(err)
		}
		if len(c.tiles.shown) != 0 || msgGetFloat(content, sel("contentsScale")) != 1 {
			t.Fatal("stale-scale partial frame was composed instead of a whole frame")
		}
	})
}

// A failed startup returns the same canvas to main-thread drawing and
// balances every reference preparation took (no window needed: the view
// is never shown).
// AppKit releases a view's display link references on the main thread, so
// the check runs in a child whose main goroutine stays on it (no window).
func TestCancelThreadedRestoresMainDrawing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "TANGO_TASK7_CANCEL_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

type cancelT interface{ Fatal(...any) }

type cancelErr struct{ err error }

func (c *cancelErr) Fatal(v ...any) {
	if c.err == nil {
		c.err = fmt.Errorf("%s", fmt.Sprint(v...))
	}
	panic(c)
}

func checkCancelThreaded(t cancelT) {
	load()
	loadCanvas()
	loadThreadLink()
	var initWeak func(*uintptr, id) id
	var loadWeak func(*uintptr) id
	var destroyWeak func(*uintptr)
	purego.RegisterLibFunc(&initWeak, libObjC, "objc_initWeak")
	purego.RegisterLibFunc(&loadWeak, libObjC, "objc_loadWeak")
	purego.RegisterLibFunc(&destroyWeak, libObjC, "objc_destroyWeak")
	var weak uintptr
	defer destroyWeak(&weak)
	c := &threadedCanvas{canvas: &canvas{s: &surface{}, scale: 1, w: 100, h: 80}}
	withPool(func() {
		view := msgInitRect(send(class("TangoCanvasView"), "alloc"), sel("initWithFrame:"), nsRect{Size: nsSize{Width: 100, Height: 80}})
		initWeak(&weak, view)
		c.view = view
		canvasByView[view] = c.canvas
		send(view, "setWantsLayer:", 1)
		c.prepareThreaded()
		if sendBool(view, "wantsUpdateLayer") {
			t.Fatal("prepared view still asks AppKit for updateLayer")
		}
		hosted := c.threadCPULayer
		c.CancelThreadedOnMain()
		if c.threadPrepared || c.threadClock.Load() != nil {
			t.Fatal("canvas still prepared for a UI thread")
		}
		if !sendBool(view, "wantsUpdateLayer") {
			t.Fatal("view did not return to AppKit drawing")
		}
		if l := send(view, "layer"); l == 0 || l == hosted {
			t.Fatal("view kept the UI thread's hosted layer")
		}
		if c.link != 0 { // the restored main display link targets the view
			send(c.link, "invalidate")
			release(c.link)
			c.link = 0
		}
		delete(canvasByView, view)
		release(view) // the creator's reference: the last one if cancel balanced its own
	})
	withPool(func() {})
	if loadWeak(&weak) != 0 {
		t.Fatal("cancel left an unbalanced reference to the view")
	}
}
