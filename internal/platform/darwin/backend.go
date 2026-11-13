//go:build darwin && !ios

// Package darwin is Tango's macOS backend: AppKit + WKWebView driven through
// the Objective-C runtime with purego. No cgo.
package darwin

import (
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

// assetOrigin is where Host.ServeAsset is mounted (WKURLSchemeHandler).
const (
	assetScheme = "tango"
	assetOrigin = assetScheme + "://app"
)

// backend implements platform.Backend plus the Clipboard, Dialogs and
// AppMenu capabilities.
type backend struct {
	h    platform.Host
	opts platform.AppOptions

	// Objective-C objects owned for the life of the process.
	app        id // NSApp
	delegate   id // TangoAppDelegate
	menuTarget id // TangoMenuTarget

	// Cross-thread wake-up: a CFRunLoopSource on the main run loop.
	mainLoop       uintptr
	source         uintptr
	ready          atomic.Bool // source exists; Wake may signal it
	wakeBeforeInit atomic.Bool // a Wake arrived while ready was false

	// Loop state, main thread only.
	stepping       int  // nested Step calls
	running        bool // inside -[NSApplication run]
	quitting       bool // Quit was requested
	terminated     bool // LifecycleTerminating was delivered
	modalDepth     int  // runModal sessions in progress
	quitAfterModal bool // Quit arrived during a modal session
	askingQuit     bool // HostEx.ShouldQuit in progress (appevents.go)

	surfaces   map[platform.SurfaceID]*surface
	byDelegate map[id]*surface
	byWebView  map[id]*surface

	menuClick   func(id uint32)
	menuEnabled map[uint32]bool

	// Dialogs still on screen, each with a cancel that reports to its done
	// callback; Run cancels the leftovers so every callback fires once.
	dialogSeq int
	dialogs   map[int]func()
}

var (
	theBackend *backend
	newMu      sync.Mutex
)

// New returns the macOS backend. AppKit has one NSApplication per process,
// so Init accepts only one backend.
func New() platform.Backend {
	load()
	newMu.Lock()
	defer newMu.Unlock()
	return &backend{
		surfaces:    make(map[platform.SurfaceID]*surface),
		byDelegate:  make(map[id]*surface),
		byWebView:   make(map[id]*surface),
		menuEnabled: make(map[uint32]bool),
		dialogs:     make(map[int]func()),
	}
}

var (
	_ platform.Backend   = (*backend)(nil)
	_ platform.Clipboard = (*backend)(nil)
	_ platform.Dialogs   = (*backend)(nil)
	_ platform.AppMenu   = (*backend)(nil)
)

func (b *backend) AssetOrigin() string { return assetOrigin }

// AppKit enumeration values used below (NSApplication.h, NSEvent.h).
const (
	nsApplicationActivationPolicyRegular = 0  // NSApplicationActivationPolicyRegular
	nsTerminateCancel                    = 0  // NSTerminateCancel
	nsEventTypeApplicationDefined        = 15 // NSEventTypeApplicationDefined
)

// wakeSubtype tags the synthetic application-defined event that unblocks a
// waiting Step; it is swallowed there and never dispatched.
const wakeSubtype = 0x7467 // "tg"

var errNotMain = errors.New("tango/darwin: Init must run on the main thread")

func (b *backend) Init(h platform.Host, opts platform.AppOptions) error {
	switch {
	case !isMainThread():
		return errNotMain
	case theBackend != nil && theBackend != b:
		return errors.New("tango/darwin: a backend is already initialised in this process")
	}
	b.h, b.opts = h, opts
	theBackend = b
	registerClasses()
	disableArgumentOpen()
	withPool(func() { b.setUpApplication(opts.Name) })

	b.installSystemObservers()    // integration.go: theme / display change events
	installNotificationDelegate() // notifyhotkey.go: launch clicks on notifications
	installSysx()                 // sysx_dragout.go: mouse-down monitor for file drags

	b.attachDrainSource()
	return nil
}

// setUpApplication creates NSApp and the objects that hang off it.
func (b *backend) setUpApplication(processName string) {
	b.app = send(class("NSApplication"), "sharedApplication")
	send(b.app, "setActivationPolicy:", activationPolicy()) // devrole.go
	// Tango manages its own windows; AppKit's automatic tab grouping would
	// merge them behind the app's back.
	send(class("NSWindow"), "setAllowsAutomaticWindowTabbing:", boolArg(false))
	if processName != "" {
		info := send(class("NSProcessInfo"), "processInfo")
		send(info, "setProcessName:", uintptr(nsString(processName)))
	}
	b.delegate = alloc("TangoAppDelegate")
	send(b.app, "setDelegate:", uintptr(b.delegate))
	b.menuTarget = alloc("TangoMenuTarget")
	b.installDefaultMenu()
}

// cfRunLoopSourceContext is CFRunLoopSourceContext (version 0) from
// CFRunLoop.h: a version field followed by nine pointers.
type cfRunLoopSourceContext struct {
	version         int
	info            uintptr
	retain          uintptr
	release         uintptr
	copyDescription uintptr
	equal           uintptr
	hash            uintptr
	schedule        uintptr
	cancel          uintptr
	perform         uintptr
}

// attachDrainSource installs the source that runs Host.Drain. Being added to
// kCFRunLoopCommonModes it also fires while AppKit spins its own loop in
// event-tracking or modal-panel mode, which the main dispatch queue does not.
// mainqueue.go piggybacks on it for runOnMain.
func (b *backend) attachDrainSource() {
	spec := cfRunLoopSourceContext{perform: drainCallback}
	b.mainLoop = cfRunLoopGetMain()
	// CFRunLoopSourceCreate copies the context, so spec may live on the stack.
	b.source = cfRunLoopSourceCreate(0, 0, unsafe.Pointer(&spec))
	cfRunLoopAddSource(b.mainLoop, b.source, kCFRunLoopCommonModes)

	b.ready.Store(true)
	if b.wakeBeforeInit.Swap(false) {
		b.signalSource()
	}
}

var drainCallback = purego.NewCallback(func(info uintptr) {
	drainMainQueue() // backend work from other threads (mainqueue.go)
	if b := theBackend; b != nil && b.h != nil {
		withPool(b.drainHost)
	}
})

func (b *backend) drainHost() {
	b.h.Drain()
	// Step blocks inside nextEventMatchingMask:, which returns only for an
	// event; post one so the blocked Step observes this Wake.
	if b.stepping > 0 {
		b.postWakeEvent()
	}
}

// Wake may be called from any goroutine.
func (b *backend) Wake() {
	if b.ready.Load() {
		b.signalSource()
		return
	}
	// Not initialised yet: leave a note for attachDrainSource. If Init
	// finished in between, whichever side clears the note does the signal.
	b.wakeBeforeInit.Store(true)
	if b.ready.Load() && b.wakeBeforeInit.CompareAndSwap(true, false) {
		b.signalSource()
	}
}

// signalSource marks the source pending and wakes the main run loop; both CF
// calls are documented as thread-safe.
func (b *backend) signalSource() {
	purego.SyscallN(cfRunLoopSourceSignalFn, b.source)
	purego.SyscallN(cfRunLoopWakeUpFn, b.mainLoop)
}

// postWakeEvent queues an inert NSEventTypeApplicationDefined event at the
// back of the event queue.
func (b *backend) postWakeEvent() {
	withPool(func() {
		const factory = "otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"
		var origin nsPoint
		ev := msgOtherEvent(class("NSEvent"), sel(factory),
			nsEventTypeApplicationDefined, origin, 0, 0, 0, 0, wakeSubtype, 0, 0)
		send(b.app, "postEvent:atStart:", uintptr(ev), boolArg(false))
	})
}

func isWakeEvent(ev id) bool {
	return sendInt(ev, "type") == nsEventTypeApplicationDefined &&
		int16(sendInt(ev, "subtype")) == wakeSubtype
}

func (b *backend) Run() error {
	if !b.quitting {
		b.running = true
		send(b.app, "run")
		b.running = false
	}
	b.cancelDialogs()
	b.terminate()
	return nil
}

func (b *backend) terminate() {
	if !b.terminated {
		b.terminated = true
		b.h.OnLifecycle(platform.LifecycleTerminating)
	}
}

func (b *backend) Quit() {
	b.quitting = true
	if b.modalDepth > 0 {
		// Inside runModal, -stop: would only end the modal session. Abort
		// it; runModalAlert stops the app once the session has unwound.
		b.quitAfterModal = true
		send(b.app, "abortModal")
		return
	}
	send(b.app, "stop:", 0)
	// -stop: is noticed only after the loop processes another event.
	b.postWakeEvent()
}

// Step dispatches at most one event, optionally waiting for it, for hosts
// that drive the loop themselves.
func (b *backend) Step(block bool) bool {
	if b.quitting {
		return false
	}
	b.stepping++
	defer func() { b.stepping-- }()
	withPool(func() {
		if ev := b.nextEvent(block); ev != 0 && !isWakeEvent(ev) {
			send(b.app, "sendEvent:", uintptr(ev))
			send(b.app, "updateWindows")
		}
	})
	return !b.quitting
}

// nextEvent dequeues the next event of any type in the default run loop
// mode, waiting indefinitely when block is set and not at all otherwise.
func (b *backend) nextEvent(block bool) id {
	deadline := "distantPast"
	if block {
		deadline = "distantFuture"
	}
	const anyEvent = ^uint64(0) // NSEventMaskAny
	defaultMode := nsString("kCFRunLoopDefaultMode")
	return msgNextEvent(b.app, sel("nextEventMatchingMask:untilDate:inMode:dequeue:"),
		anyEvent, send(class("NSDate"), deadline), defaultMode, true)
}

var classesOnce sync.Once

func registerClasses() {
	classesOnce.Do(func() {
		registerAppDelegate()
		registerWindowClasses()
		registerSchemeHandler()
		registerMenuTarget()
	})
}

// registerAppDelegate defines TangoAppDelegate (NSApplicationDelegate).
func registerAppDelegate() {
	// Notifications that map one-to-one onto a lifecycle event.
	lifecycle := []struct {
		selector string
		event    platform.LifecycleEvent
		activate bool // bring the app forward first
	}{
		// A bare executable (go run, no bundle) is not activated by
		// LaunchServices, so do it here.
		{"applicationDidFinishLaunching:", platform.LifecycleStarted, true},
		{"applicationDidBecomeActive:", platform.LifecycleForeground, false},
		{"applicationDidResignActive:", platform.LifecycleBackground, false},
	}
	methods := make([]objc.MethodDef, 0, len(lifecycle)+4)
	for _, l := range lifecycle {
		l := l
		methods = append(methods, method(l.selector, func(_ id, _ objc.SEL, _ id) {
			b := theBackend
			if l.activate && !devWorker {
				send(b.app, "activateIgnoringOtherApps:", boolArg(true))
			}
			b.h.OnLifecycle(l.event)
		}))
	}
	methods = append(methods,
		// Cmd+Q, the Quit menu item and the Dock's Quit all ask here first.
		// HostEx.ShouldQuit gets a veto (appevents.go). AppKit is never
		// allowed to terminate on its own (it would exit() underneath Go);
		// instead the loop is stopped so Run returns and main's defers run.
		method("applicationShouldTerminate:", func(_ id, _ objc.SEL, _ id) uint {
			if b := theBackend; b.shouldQuit() {
				b.Quit()
			}
			return nsTerminateCancel
		}),
		method("applicationWillTerminate:", func(_ id, _ objc.SEL, _ id) {
			theBackend.terminate()
		}),
		// Closing the last window does not quit; the core decides.
		method("applicationShouldTerminateAfterLastWindowClosed:", func(_ id, _ objc.SEL, _ id) bool {
			return false
		}),
		// Opt in to secure coding for state restoration (macOS 12+ warns
		// otherwise).
		method("applicationSupportsSecureRestorableState:", func(_ id, _ objc.SEL, _ id) bool {
			return true
		}),
	)
	classDef("TangoAppDelegate", "NSObject", []string{"NSApplicationDelegate"}, append(methods, appEventMethods()...))
}

// cancelDialogs ends every dialog still showing with a cancel, so each done
// callback runs exactly once even when the app quits under it. A cancel may
// open another dialog, hence the outer loop.
func (b *backend) cancelDialogs() {
	for len(b.dialogs) > 0 {
		for key, cancel := range b.dialogs {
			delete(b.dialogs, key)
			cancel()
		}
	}
}
