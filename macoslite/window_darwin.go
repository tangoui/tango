//go:build darwin

package macoslite

import (
	"errors"
	"image"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// appkitBackend hosts the App in one NSWindow whose content view is a
// layer-backed MacosLiteView. Frames are drawn on the CPU and handed to the
// layer as a CGImage (updateLayer → App.drawFrame → present).
type appkitBackend struct {
	app *App

	// Main thread only.
	nsApp, window, view, delegate, appDelegate id
	space                                      uintptr // sRGB color space, owned
	looping                                    bool

	qmu   sync.Mutex
	queue []func()
	ready atomic.Bool // dispatcher exists; queued work can be signalled
}

const (
	nsWindowStyleTitled           = 1 << 0
	nsWindowStyleClosable         = 1 << 1
	nsWindowStyleMiniaturizable   = 1 << 2
	nsWindowStyleResizable        = 1 << 3
	nsBackingStoreBuffered        = 2
	nsEventTypeApplicationDefined = 15
	nsTerminateNow                = 1
	// NSViewLayerContentsRedrawDuringViewResize
	redrawDuringResize = 2

	kCGImageAlphaPremultipliedLast = 1
	kCGBitmapByteOrder32Big        = 4 << 12
)

var (
	classOnce  sync.Once
	classErr   error
	dispatcher id // MacosLiteDispatcher singleton, main thread
	// Objective-C object → backend, for method implementations. Main thread.
	backends = map[id]*appkitBackend{}
	// The single backend whose queue the dispatcher drains.
	activeBackend atomic.Pointer[appkitBackend]
)

func registerClasses() error {
	classOnce.Do(func() {
		yes := func(id, objc.SEL) bool { return true }
		_, err := objc.RegisterClass("MacosLiteView", objc.GetClass("NSView"), nil, nil, []objc.MethodDef{
			{Cmd: sel("isFlipped"), Fn: yes},
			{Cmd: sel("wantsUpdateLayer"), Fn: yes},
			{Cmd: sel("acceptsFirstResponder"), Fn: yes},
			{Cmd: sel("acceptsFirstMouse:"), Fn: func(id, objc.SEL, id) bool { return true }},
			{Cmd: sel("updateLayer"), Fn: func(self id, _ objc.SEL) {
				if b := backends[self]; b != nil {
					b.frame()
				}
			}},
			{Cmd: sel("mouseDown:"), Fn: mouseHandler(MouseDown)},
			{Cmd: sel("mouseUp:"), Fn: mouseHandler(MouseUp)},
			{Cmd: sel("mouseDragged:"), Fn: mouseHandler(MouseMove)},
			{Cmd: sel("mouseMoved:"), Fn: mouseHandler(MouseMove)},
			{Cmd: sel("rightMouseDown:"), Fn: mouseHandler(MouseDown)},
			{Cmd: sel("rightMouseUp:"), Fn: mouseHandler(MouseUp)},
			{Cmd: sel("viewDidChangeBackingProperties"), Fn: func(self id, _ objc.SEL) {
				objc.ID(self).SendSuper(sel("viewDidChangeBackingProperties"))
				if b := backends[self]; b != nil {
					b.app.backingChanged()
				}
			}},
			{Cmd: sel("keyDown:"), Fn: keyHandler(true)},
			{Cmd: sel("keyUp:"), Fn: keyHandler(false)},
		})
		if err != nil {
			classErr = err
			return
		}
		_, err = objc.RegisterClass("MacosLiteWindowDelegate", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
			{Cmd: sel("windowWillClose:"), Fn: func(self id, _ objc.SEL, _ id) {
				if b := backends[self]; b != nil {
					b.windowClosed()
				}
			}},
		})
		if err != nil {
			classErr = err
			return
		}
		_, err = objc.RegisterClass("MacosLiteAppDelegate", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
			{Cmd: sel("applicationShouldTerminate:"), Fn: func(self id, _ objc.SEL, _ id) uint {
				if b := backends[self]; b != nil {
					b.app.terminating()
				}
				return nsTerminateNow
			}},
		})
		if err != nil {
			classErr = err
			return
		}
		dcls, err := objc.RegisterClass("MacosLiteDispatcher", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
			{Cmd: sel("drain:"), Fn: func(id, objc.SEL, id) {
				if b := activeBackend.Load(); b != nil {
					b.drain()
				}
			}},
		})
		if err != nil {
			classErr = err
			return
		}
		dispatcher = send(id(dcls), "new")
	})
	return classErr
}

func (b *appkitBackend) open(o Options) error {
	if err := loadNative(); err != nil {
		return err
	}
	if pthreadMainNP() != 1 {
		return errors.New("macoslite: Run must be called on the main goroutine")
	}
	if err := registerClasses(); err != nil {
		return err
	}
	withPool(func() {
		b.nsApp = send(class("NSApplication"), "sharedApplication")
		msgVoidInt(b.nsApp, sel("setActivationPolicy:"), 0) // regular app

		rect := cgRect{Size: cgSize{o.Width, o.Height}}
		style := uint(nsWindowStyleTitled | nsWindowStyleClosable | nsWindowStyleMiniaturizable | nsWindowStyleResizable)
		b.window = msgInitWindow(send(class("NSWindow"), "alloc"), sel("initWithContentRect:styleMask:backing:defer:"),
			rect, style, nsBackingStoreBuffered, false)
		msgVoidBool(b.window, sel("setReleasedWhenClosed:"), false)
		title := cfString(o.Title)
		msgVoidID(b.window, sel("setTitle:"), id(title))
		cfRelease(title)

		b.view = msgInitRect(send(class("MacosLiteView"), "alloc"), sel("initWithFrame:"), rect)
		msgVoidBool(b.view, sel("setWantsLayer:"), true)
		msgVoidInt(b.view, sel("setLayerContentsRedrawPolicy:"), redrawDuringResize)
		msgVoidID(b.window, sel("setContentView:"), b.view)
		msgVoidBool(b.window, sel("setAcceptsMouseMovedEvents:"), true)

		b.delegate = send(class("MacosLiteWindowDelegate"), "new")
		msgVoidID(b.window, sel("setDelegate:"), b.delegate)
		b.appDelegate = send(class("MacosLiteAppDelegate"), "new")
		msgVoidID(b.nsApp, sel("setDelegate:"), b.appDelegate)
		backends[b.view], backends[b.delegate], backends[b.appDelegate] = b, b, b

		b.space = cgColorSpaceCreateWithName(kCGColorSpaceSRGB)
		send(b.window, "center")
		// Keys reach the view before any click (Esc in a prompt window).
		msgBoolID(b.window, sel("makeFirstResponder:"), b.view)
		msgVoidID(b.window, sel("makeKeyAndOrderFront:"), 0)
		msgVoidBool(b.nsApp, sel("activateIgnoringOtherApps:"), true)
		msgVoidBool(b.view, sel("setNeedsDisplay:"), true)
	})
	activeBackend.Store(b)
	b.ready.Store(true)
	b.signal()
	return nil
}

func (b *appkitBackend) loop() error {
	b.looping = true
	msgVoidID(b.nsApp, sel("run"), 0)
	b.looping = false
	b.teardown()
	return nil
}

func (b *appkitBackend) setNeedsDisplay() {
	if b.view != 0 {
		msgVoidBool(b.view, sel("setNeedsDisplay:"), true)
	}
}

func (b *appkitBackend) dispatchMain(fn func()) {
	b.qmu.Lock()
	b.queue = append(b.queue, fn)
	b.qmu.Unlock()
	b.signal()
}

// signal asks the main thread to drain the queue. Safe from any thread:
// performSelectorOnMainThread: is documented as thread-safe.
func (b *appkitBackend) signal() {
	if !b.ready.Load() {
		return
	}
	msgPerformMain(dispatcher, sel("performSelectorOnMainThread:withObject:waitUntilDone:"), sel("drain:"), 0, false)
}

func (b *appkitBackend) drain() {
	b.qmu.Lock()
	q := b.queue
	b.queue = nil
	b.qmu.Unlock()
	for _, fn := range q {
		fn()
	}
}

// close closes the window; windowWillClose: then stops the event loop.
func (b *appkitBackend) close() {
	if b.window != 0 {
		withPool(func() { send(b.window, "close") })
	}
}

// windowClosed handles windowWillClose: for both user and programmatic
// closes: tell the App, then end -[NSApp run].
func (b *appkitBackend) windowClosed() {
	b.app.handleClose()
	if !b.looping {
		return
	}
	withPool(func() {
		msgVoidID(b.nsApp, sel("stop:"), 0)
		// -stop: takes effect after the current event; post one so run
		// returns now rather than on the next user input.
		ev := msgOtherEvent(class("NSEvent"), sel("otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"),
			nsEventTypeApplicationDefined, cgPoint{}, 0, 0, 0, 0, 0, 0, 0)
		msgPostEvent(b.nsApp, sel("postEvent:atStart:"), ev, true)
	})
}

// teardown releases everything open created, after the loop has ended.
func (b *appkitBackend) teardown() {
	activeBackend.CompareAndSwap(b, nil)
	b.ready.Store(false)
	delete(backends, b.view)
	delete(backends, b.delegate)
	delete(backends, b.appDelegate)
	withPool(func() {
		if b.appDelegate != 0 {
			msgVoidID(b.nsApp, sel("setDelegate:"), 0)
			send(b.appDelegate, "release")
		}
		if b.window != 0 {
			msgVoidID(b.window, sel("setDelegate:"), 0)
			msgVoidID(b.window, sel("setContentView:"), 0)
			send(b.window, "release")
		}
		if b.view != 0 {
			send(b.view, "release")
		}
		if b.delegate != 0 {
			send(b.delegate, "release")
		}
	})
	b.window, b.view, b.delegate, b.appDelegate = 0, 0, 0, 0
	if b.space != 0 {
		cgColorSpaceRelease(b.space)
		b.space = 0
	}
}

// frame runs from updateLayer.
func (b *appkitBackend) frame() {
	r := msgRect(b.view, sel("bounds"))
	scale := 1.0
	if b.window != 0 {
		scale = msgFloat(b.window, sel("backingScaleFactor"))
	}
	b.app.drawFrame(r.Size.W, r.Size.H, scale)
}

// present makes img the layer's contents.
func (b *appkitBackend) present(img *image.RGBA) {
	if b.view == 0 {
		return
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	data := cfDataCreate(0, unsafe.Pointer(&img.Pix[0]), len(img.Pix)) // copies
	if data == 0 {
		return
	}
	provider := cgDataProviderCreateWithCFData(data)
	cfRelease(data)
	if provider == 0 {
		return
	}
	cg := cgImageCreate(w, h, 8, 32, img.Stride, b.space,
		kCGImageAlphaPremultipliedLast|kCGBitmapByteOrder32Big, provider, nil, false, 0)
	cgDataProviderRelease(provider)
	if cg == 0 {
		return
	}
	withPool(func() {
		layer := send(b.view, "layer")
		ca := class("CATransaction")
		send(ca, "begin")
		msgVoidBool(ca, sel("setDisableActions:"), true)
		msgVoidID(layer, sel("setContents:"), id(cg))
		msgVoidFloat(layer, sel("setContentsScale:"), float64(w)/msgRect(b.view, sel("bounds")).Size.W)
		send(ca, "commit")
	})
	cgImageRelease(cg)
}

func mouseHandler(kind MouseKind) func(id, objc.SEL, id) {
	return func(self id, _ objc.SEL, ev id) {
		b := backends[self]
		if b == nil {
			return
		}
		p := msgConvert(self, sel("convertPoint:fromView:"), msgPoint(ev, sel("locationInWindow")), 0)
		b.app.view.Mouse(MouseEvent{Kind: kind, Pos: Point{p.X, p.Y}, Button: msgInt(ev, sel("buttonNumber"))})
	}
}

func keyHandler(down bool) func(id, objc.SEL, id) {
	return func(self id, _ objc.SEL, ev id) {
		b := backends[self]
		if b == nil {
			return
		}
		code := msgUint16(ev, sel("keyCode"))
		b.app.view.Key(KeyEvent{Key: keyName(code, nsStringToGo(send(ev, "charactersIgnoringModifiers"))), Code: code, Down: down})
	}
}

// keyName maps the hardware key codes of named keys; other keys report their
// characters.
func keyName(code uint16, chars string) string {
	switch code {
	case 53:
		return "Escape"
	case 36, 76:
		return "Enter"
	case 48:
		return "Tab"
	case 51:
		return "Backspace"
	}
	return chars
}

func nsStringToGo(s id) string {
	if s == 0 {
		return ""
	}
	p := uintptr(send(s, "UTF8String"))
	if p == 0 {
		return ""
	}
	var n int
	for loadByte(p+uintptr(n)) != 0 {
		n++
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = loadByte(p + uintptr(i))
	}
	return string(b)
}

func loadByte(addr uintptr) byte {
	p := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	return *(*byte)(p)
}
