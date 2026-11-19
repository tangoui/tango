//go:build darwin && !ios

package darwin

// The display clock uses NSView's CADisplayLink on the window UI thread's
// run loop (macOS 14+), with timer pacing when a display link is unavailable.
// The real-window callback rate and thread affinity must be verified before
// enabling it by default. CVDisplayLink is the alternative if AppKit cannot
// drive the link on the window's run loop.

import (
	"errors"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/uithread"
)

// Requests and native callbacks only queue work. Pacing state and tick are
// owned by the UI thread. The timer only wakes the Go queue, never AppKit.
type threadFrameClock struct {
	th                             *uithread.Thread
	tick                           func()
	pause                          func(bool)
	wanted, running                bool
	idle                           int
	mu                             sync.Mutex
	stopped, queued, refreshQueued bool
	timer                          *time.Timer
	deadline                       time.Time
	generation                     uint64
	lastDraw, refreshAt            time.Time
}

func newThreadFrameClock(th *uithread.Thread, tick func(), pause func(bool)) *threadFrameClock {
	return &threadFrameClock{th: th, tick: tick, pause: pause}
}

func (f *threadFrameClock) request() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queueLocked()
}

func (f *threadFrameClock) queueLocked() {
	if !f.stopped && !f.queued {
		f.queued = f.th.Post(f.requestOnUI)
	}
}

func (f *threadFrameClock) requestOnUI() {
	f.mu.Lock()
	f.queued = false
	stopped := f.stopped
	f.mu.Unlock()
	if stopped {
		return
	}
	atOnce := frameAtOnce(f.idle, f.running, false)
	f.wanted, f.idle = true, 0
	if !f.running {
		f.running = true
		f.pause(false)
	}
	if atOnce {
		f.draw()
	}
}

func (f *threadFrameClock) requestAt(at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped || at.IsZero() || (!f.deadline.IsZero() && !at.Before(f.deadline)) {
		return
	}
	f.clearDeadline()
	f.deadline = at
	gen := f.generation
	f.timer = time.AfterFunc(time.Until(at), func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if gen == f.generation {
			f.deadline, f.timer = time.Time{}, nil
			f.queueLocked()
		}
	})
}

func (f *threadFrameClock) clearDeadline() {
	f.generation++
	if f.timer != nil {
		f.timer.Stop()
	}
	f.deadline, f.timer = time.Time{}, nil
}

func (f *threadFrameClock) queueRefresh() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshAt = time.Now()
	if f.stopped || f.refreshQueued {
		return
	}
	f.refreshQueued = f.th.Post(func() {
		f.mu.Lock()
		f.refreshQueued = false
		at := f.refreshAt
		f.mu.Unlock()
		if at.After(f.lastDraw) {
			f.refresh()
		}
	})
}

func (f *threadFrameClock) refresh() {
	f.mu.Lock()
	stopped := f.stopped
	f.mu.Unlock()
	if stopped || !f.running {
		return
	}
	if f.wanted {
		f.draw()
	} else {
		f.idle++
		if f.idle >= linkIdleTicks {
			f.running = false
			f.pause(true)
		}
	}
}

func (f *threadFrameClock) draw() {
	f.wanted, f.idle = false, 0
	f.mu.Lock()
	f.lastDraw = time.Now()
	if !f.deadline.IsZero() && !f.lastDraw.Before(f.deadline) {
		f.clearDeadline()
	}
	f.mu.Unlock()
	withPool(f.tick)
}

func (f *threadFrameClock) stop() {
	f.mu.Lock()
	f.stopped = true
	f.clearDeadline()
	f.mu.Unlock()
}

type threadedGeometry struct{ w, h, scale float32 }

type threadedCanvas struct {
	*canvas
	useCoreVideo bool
}

var _ platform.CanvasThreaded = (*threadedCanvas)(nil)
var _ platform.CanvasThreadedRelease = (*threadedCanvas)(nil)

// ThreadedCanvas is the internal test adapter. Do not advertise the threaded
// capabilities on ordinary windows until snapshot, embedding and shared
// layout-cache ownership are fixed and multi-window race coverage passes.
func ThreadedCanvas(cv platform.Canvas) platform.Canvas {
	if c, ok := cv.(*canvas); ok {
		tc := &threadedCanvas{canvas: c}
		tc.prepareThreaded()
		return tc
	}
	return cv
}

// MainThreadAccessibility asks uihost to publish all native AX updates
// together with its action-validation snapshot on main.
func (*threadedCanvas) MainThreadAccessibility() {}

func (c *threadedCanvas) MainGeometry() (float32, float32, float32) {
	g := c.geometrySnapshot.Load()
	return g.w, g.h, g.scale
}

var (
	loadThreadLinkOnce sync.Once
	threadLinkClass    id
	currentRunLoop     func() uintptr
	runInMode          func(uintptr, float64, bool) int32
	invalidateSource   func(uintptr)
	// NSRunLoop callbacks are dispatched by its UI thread; no view/global
	// canvas registry lookups from background threads.
	threadLinks sync.Map // target id -> func()
)

func loadThreadLink() {
	loadThreadLinkOnce.Do(func() {
		purego.RegisterLibFunc(&currentRunLoop, libCF, "CFRunLoopGetCurrent")
		purego.RegisterLibFunc(&runInMode, libCF, "CFRunLoopRunInMode")
		purego.RegisterLibFunc(&invalidateSource, libCF, "CFRunLoopSourceInvalidate")
		cls, err := objc.RegisterClass("TangoThreadedDisplayLinkTarget", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{method("tangoThreadTick:", func(self id, _ objc.SEL, _ id) {
			if fn, ok := threadLinks.Load(self); ok {
				fn.(func())()
			}
		})})
		if err != nil {
			panic(err)
		}
		threadLinkClass = id(cls)
	})
}

var idleSourcePerform = purego.NewCallback(func(_ uintptr) {})

// StartFrameClock must not dispatch to main. The main thread creates the
// link and the initial hosted layer before calling this method.
func (c *threadedCanvas) StartFrameClock(tick func()) (func(), func(), error) {
	th := uithread.Current()
	if th == nil {
		return nil, nil, errors.New("tango/darwin: no window UI thread")
	}
	loadThreadLink()
	var f *threadFrameClock
	var stop func()
	if c.threadLink != 0 && !c.useCoreVideo {
		rl := currentRunLoop()
		mode := constPtr(libCF, "kCFRunLoopDefaultMode")
		ctx := cfRunLoopSourceContext{perform: idleSourcePerform}
		source := cfRunLoopSourceCreate(0, 0, unsafe.Pointer(&ctx))
		if source == 0 {
			return c.startCoreVideo(th, tick)
		}
		cfRunLoopAddSource(rl, source, mode)
		f = newThreadFrameClock(th, tick, func(paused bool) {
			var v uintptr
			if paused {
				v = 1
			}
			send(c.threadLink, "setPaused:", v)
		})
		threadLinks.Store(c.threadLinkTarget, f.queueRefresh)
		withPool(func() {
			send(c.threadLink, "addToRunLoop:forMode:", uintptr(send(class("NSRunLoop"), "currentRunLoop")), mode)
		})
		th.SetIdlePump(func() { withPool(func() { runInMode(mode, 3600, true) }) }, func() {
			purego.SyscallN(cfRunLoopSourceSignalFn, source)
			purego.SyscallN(cfRunLoopWakeUpFn, rl)
		})
		stop = func() {
			f.stop()
			threadLinks.Delete(c.threadLinkTarget)
			send(c.threadLink, "invalidate")
			th.SetIdlePump(nil, nil)
			invalidateSource(source)
			cfRelease(source)
		}
	} else if c.useCoreVideo || c.threadLinkTarget != 0 {
		return c.startCoreVideo(th, tick)
	} else {
		return c.startTimerClock(th, tick)
	}
	c.threadClock.Store(f)
	return f.request, stop, nil
}

func (c *threadedCanvas) startTimerClock(th *uithread.Thread, tick func()) (func(), func(), error) {
	var f *threadFrameClock
	var pause func(bool)
	f = newThreadFrameClock(th, tick, func(paused bool) { pause(paused) })
	pause = timerRefresh(f)
	c.threadClock.Store(f)
	return f.request, func() { f.stop(); pause(true) }, nil
}

// timerRefresh returns a pause function driving f at the fallback cadence.
// It is called only on f's UI thread.
func timerRefresh(f *threadFrameClock) func(bool) {
	var quit chan struct{}
	return func(paused bool) {
		if quit != nil {
			close(quit)
			quit = nil
		}
		if paused {
			return
		}
		quit = make(chan struct{})
		go func(done <-chan struct{}) {
			ticker := time.NewTicker(frameInterval)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					f.queueRefresh()
				}
			}
		}(quit)
	}
}

func (c *canvas) publishGeometry() {
	c.geometrySnapshot.Store(&threadedGeometry{float32(c.w), float32(c.h), float32(c.scale)})
}

// prepareThreaded runs before the main thread waits for clock startup.
func (c *threadedCanvas) prepareThreaded() {
	loadThreadLink()
	c.threadPrepared = true
	c.publishGeometry()
	// A display link created from the view refers to its window; keep both
	// alive for frames still in flight after close. Balanced only once the
	// UI thread is Done (ReleaseThreadedOnMain or CancelThreadedOnMain).
	retain(c.view)
	c.threadWindow = retain(c.s.win)
	withPool(func() {
		if c.link != 0 {
			send(c.link, "invalidate")
			release(c.link)
			c.link = 0
		}
		ca := class("CATransaction")
		send(ca, "begin")
		send(ca, "setDisableActions:", 1)
		layer := send(class("CALayer"), "layer")
		send(layer, "setActions:", uintptr(noActions()))
		msgSetFloat(layer, sel("setContentsScale:"), c.scale)
		send(c.view, "setWantsLayer:", 0)
		send(c.view, "setLayer:", uintptr(layer))
		send(c.view, "setWantsLayer:", 1)
		c.threadCPULayer = layer
		c.setGravity()
		send(ca, "commit")
		if respondsTo(c.view, "displayLinkWithTarget:selector:") {
			c.threadLinkTarget = send(send(threadLinkClass, "alloc"), "init")
			c.threadLink = retain(send(c.view, "displayLinkWithTarget:selector:", uintptr(c.threadLinkTarget), uintptr(sel("tangoThreadTick:"))))
			if c.threadLink != 0 {
				send(c.threadLink, "setPaused:", 1)
			}
		}
	})
}

func (c *threadedCanvas) ReleaseThreadedOnMain() {
	withPool(func() {
		c.destroy()
		if !c.threadReleased {
			c.threadReleased = true
			c.nativeLayer, c.metalLayer, c.cpuLayer, c.threadCPULayer = 0, 0, 0, 0
			c.tiles = nil
			if c.threadLink != 0 {
				send(c.threadLink, "invalidate")
			}
			c.releaseThreadRefs()
		}
	})
}

// SetFrameScale records the scale the next presented frame was drawn at
// (UI thread; uihost calls it before Present/PresentDamage).
func (c *canvas) SetFrameScale(scale float32) { c.frameScale = float64(scale) }

// releaseThreadRefs balances prepareThreaded's references. Main thread,
// after the UI thread is Done.
func (c *canvas) releaseThreadRefs() {
	release(c.threadLink)
	release(c.threadLinkTarget)
	if c.threadContent != 0 {
		send(c.threadContent, "removeFromSuperlayer")
		release(c.threadContent)
	}
	c.threadLink, c.threadLinkTarget, c.threadContent = 0, 0, 0
	release(c.view)
	release(c.threadWindow)
	c.threadWindow = 0
}

// CancelThreadedOnMain undoes prepareThreaded after a failed startup, once
// the UI thread is Done: the view goes back to an AppKit-backed layer and
// the main thread's display link, so the same canvas draws on main.
func (c *threadedCanvas) CancelThreadedOnMain() {
	if !c.threadPrepared || c.threadReleased {
		return
	}
	withPool(func() {
		c.threadReleased = true
		if c.threadLink != 0 {
			send(c.threadLink, "invalidate")
		}
		c.threadClock.Store(nil)
		c.threadPrepared = false
		c.frameScale, c.tileScale = 0, 0
		c.releaseThreadRefs()
		if c.closed {
			return
		}
		c.tiles = nil
		if c.nativeLayer == 0 && c.metalLayer == 0 {
			// Back to a layer-backed view: AppKit owns the layer and calls
			// updateLayer, as before preparation.
			send(c.view, "setWantsLayer:", 0)
			send(c.view, "setLayer:", 0)
			send(c.view, "setWantsLayer:", 1)
			send(c.view, "setLayerContentsRedrawPolicy:", nsViewLayerContentsRedrawOnSetNeedsDisplay)
			c.setGravity()
		}
		c.threadCPULayer = 0
		if respondsTo(c.view, "displayLinkWithTarget:selector:") && nsRunLoopCommon != 0 {
			c.link = retain(send(c.view, "displayLinkWithTarget:selector:", uintptr(c.view), uintptr(sel("tangoDisplayTick:"))))
			if c.link != 0 {
				send(c.link, "setPaused:", 1)
				send(c.link, "addToRunLoop:forMode:", uintptr(send(class("NSRunLoop"), "mainRunLoop")), nsRunLoopCommon)
			}
		}
		c.linkRunning, c.idleTicks = false, 0
		c.Invalidate()
	})
}

func (c *canvas) presentationScale() float64 {
	if c.threadClock.Load() != nil {
		if c.frameScale > 0 {
			return c.frameScale
		}
		return float64(c.geometrySnapshot.Load().scale)
	}
	return c.scale
}

func (c *canvas) commitPresentation(transaction id) {
	send(transaction, "commit")
	if c.threadClock.Load() != nil {
		send(transaction, "flush")
	}
}

func (c *canvas) cpuContentLayer() id {
	if c.threadClock.Load() != nil {
		if c.nativeLayer != 0 {
			return c.threadChild(c.nativeLayer)
		}
		return c.threadChild(c.threadCPULayer)
	}
	return send(c.view, "layer")
}

// threadChild returns the UI thread's own CPU content layer under root.
// Only the UI thread sets its frame and scale; the main thread owns root.
func (c *canvas) threadChild(root id) id {
	if c.threadContent != 0 && send(c.threadContent, "superlayer") == root {
		return c.threadContent
	}
	if c.threadContent != 0 {
		send(c.threadContent, "removeFromSuperlayer")
		release(c.threadContent)
		c.tiles = nil
	}
	l := retain(send(class("CALayer"), "layer"))
	send(l, "setActions:", uintptr(noActions()))
	send(l, "setOpaque:", 1)
	send(l, "setGeometryFlipped:", 1)
	send(root, "addSublayer:", uintptr(l))
	c.threadContent = l
	return l
}
