//go:build darwin && !ios

package darwin

import (
	"context"
	"fmt"
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
