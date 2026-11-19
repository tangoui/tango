//go:build darwin && !ios

package darwin

import (
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"
	"github.com/tangoui/tango/internal/uithread"
)

// CoreVideo calls on a foreign thread. Only a Go queue is touched by its
// callback. The key remains valid as a lookup (and never refers to freed
// native memory) even when a callback races stop.
var (
	cvOnce          sync.Once
	cvCreate        func(*uintptr) int32
	cvOutput        func(link, cb, ctx uintptr) int32
	cvStart, cvStop func(uintptr) int32
	cvRelease       func(uintptr)
	cvClocks        sync.Map // uintptr key -> *threadFrameClock
	cvClockID       atomic.Uintptr
	cvCallback      = purego.NewCallback(func(_, _, _, _, _, key uintptr) int32 {
		if v, ok := cvClocks.Load(key); ok {
			v.(*threadFrameClock).queueRefresh()
		}
		return 0
	})
)

func (c *threadedCanvas) startCoreVideo(th *uithread.Thread, tick func()) (func(), func(), error) {
	cvOnce.Do(func() {
		lib, err := purego.Dlopen(frameworksDir+"CoreVideo.framework/CoreVideo", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return
		}
		for name, fn := range map[string]any{
			"CVDisplayLinkCreateWithActiveCGDisplays": &cvCreate,
			"CVDisplayLinkSetOutputCallback":          &cvOutput,
			"CVDisplayLinkStart":                      &cvStart, "CVDisplayLinkStop": &cvStop, "CVDisplayLinkRelease": &cvRelease,
		} {
			addr, err := purego.Dlsym(lib, name)
			if err != nil {
				cvCreate = nil
				return
			}
			purego.RegisterFunc(fn, addr)
		}
	})
	var link uintptr
	if cvCreate == nil || cvCreate(&link) != 0 || link == 0 {
		return c.startTimerClock(th, tick)
	}
	key := cvClockID.Add(1)
	if cvOutput(link, cvCallback, key) != 0 {
		cvRelease(link)
		return c.startTimerClock(th, tick)
	}
	var f *threadFrameClock
	var timer func(bool) // UI thread; non-nil once CoreVideo stopped working
	usingLink := true
	f = newThreadFrameClock(th, tick, func(paused bool) {
		if timer != nil {
			timer(paused)
			return
		}
		if paused {
			cvStop(link)
			return
		}
		if cvStart(link) != 0 {
			// Keep f (wanted, idle and deadline): only its refresh source changes.
			usingLink = false
			cvClocks.Delete(key)
			timer = timerRefresh(f)
			timer(false)
		}
	})
	cvClocks.Store(key, f)
	if cvStart(link) != 0 {
		cvClocks.Delete(key)
		cvRelease(link)
		return c.startTimerClock(th, tick)
	}
	cvStop(link)
	c.threadClock.Store(f)
	return f.request, func() {
		f.stop()
		cvClocks.Delete(key)
		if timer != nil {
			timer(true)
		}
		if usingLink {
			cvStop(link)
		}
		cvRelease(link)
	}, nil
}
