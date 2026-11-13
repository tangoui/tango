//go:build darwin && !ios

package darwin

// Round-2 system integration on macOS: global hotkeys through the Carbon
// Event Manager (RegisterEventHotKey; it needs no Accessibility
// permission) and notifications through the UserNotifications framework.
//
// UNUserNotificationCenter raises an Objective-C exception, which aborts
// the process, when the app is not a bundle with an identifier. Every entry
// point checks the bundle first and returns errNotBundled instead.

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.Hotkeys  = (*backend)(nil)
	_ platform.Notifier = (*backend)(nil)
)

// ---------------------------------------------------------------------------
// Hopping to the main thread
//
// UserNotifications calls completion handlers and delegate methods on its
// own queues. mainHop.post hands that work to runOnMain (mainqueue.go).

type hopQueue struct {
	mu     sync.Mutex
	fns    []func()
	once   sync.Once
	target id
	modes  id
}

var mainHop hopQueue

func (q *hopQueue) post(fn func()) { runOnMain(fn) }

func (q *hopQueue) drain() {
	q.mu.Lock()
	fns := q.fns
	q.fns = nil
	q.mu.Unlock()
	for _, fn := range fns {
		withPool(fn)
	}
}

// addrPtr turns a native address into an unsafe.Pointer (the memory is not
// Go's, so this does not trip the garbage collector).
func addrPtr(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

// invokeBlock calls a block the system handed us. Per the Clang Block ABI
// a block literal starts with isa (pointer), flags and reserved (two
// int32), then the invoke function pointer, whose first argument is the
// block itself.
func invokeBlock(blk uintptr, args ...uintptr) {
	if blk == 0 {
		return
	}
	invoke := *(*uintptr)(unsafe.Add(addrPtr(blk), unsafe.Sizeof(uintptr(0))+8))
	purego.SyscallN(invoke, append([]uintptr{blk}, args...)...)
}

// ---------------------------------------------------------------------------
// Global hotkeys

// Carbon Event Manager constants (CarbonEvents.h, Events.h).
const (
	carbonKeyboardClass   = 'k'<<24 | 'e'<<16 | 'y'<<8 | 'b' // kEventClassKeyboard
	carbonHotKeyPressed   = 5                                // kEventHotKeyPressed
	carbonDirectObject    = '-'<<24 | '-'<<16 | '-'<<8 | '-' // kEventParamDirectObject
	carbonHotKeyIDType    = 'h'<<24 | 'k'<<16 | 'i'<<8 | 'd' // typeEventHotKeyID
	carbonHotKeyExclusive = 1                                // kEventHotKeyExclusive
	tangoHotKeyTag        = 'T'<<24 | 'n'<<16 | 'g'<<8 | 'o' // our EventHotKeyID.signature
)

// carbonEventType mirrors EventTypeSpec; carbonHotKeyID EventHotKeyID.
type carbonEventType struct{ class, kind uint32 }
type carbonHotKeyID struct{ signature, id uint32 }

type carbonAPI struct {
	appTarget  func() uintptr
	install    func(target, proc uintptr, count uint32, types *carbonEventType, data uintptr, out *uintptr) int32
	register   func(code, mods uint32, hkid uint64, target uintptr, options uint32, out *uintptr) int32
	unregister func(ref uintptr) int32
	param      func(event uintptr, name, typ uint32, outType *uint32, size uintptr, outSize *uintptr, data unsafe.Pointer) int32
}

type hotkeyTable struct {
	loadOnce  sync.Once
	loadErr   error
	api       carbonAPI
	installed bool
	refs      map[uint32]uintptr // core id -> EventHotKeyRef
	fire      map[uint32]func()  // core id -> callback
	proc      uintptr
}

var hotkeyTab = hotkeyTable{refs: map[uint32]uintptr{}, fire: map[uint32]func(){}}

func (t *hotkeyTable) load() error {
	t.loadOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/Carbon.framework/Carbon", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			t.loadErr = fmt.Errorf("tango/darwin: Carbon unavailable: %w", err)
			return
		}
		purego.RegisterLibFunc(&t.api.appTarget, lib, "GetApplicationEventTarget")
		purego.RegisterLibFunc(&t.api.install, lib, "InstallEventHandler")
		purego.RegisterLibFunc(&t.api.register, lib, "RegisterEventHotKey")
		purego.RegisterLibFunc(&t.api.unregister, lib, "UnregisterEventHotKey")
		purego.RegisterLibFunc(&t.api.param, lib, "GetEventParameter")
		// EventHandlerProcPtr: OSStatus (EventHandlerCallRef, EventRef, void*).
		// Hot key events are dispatched on the main thread by the run loop.
		t.proc = purego.NewCallback(func(_, event, _ uintptr) uintptr {
			var hk carbonHotKeyID
			if t.api.param(event, carbonDirectObject, carbonHotKeyIDType, nil, unsafe.Sizeof(hk), nil, unsafe.Pointer(&hk)) == 0 &&
				hk.signature == tangoHotKeyTag {
				if fn := t.fire[hk.id]; fn != nil {
					withPool(fn)
				}
			}
			return 0 // noErr
		})
	})
	return t.loadErr
}

func (b *backend) RegisterHotkey(hid uint32, accelerator string, fire func()) error {
	code, mods, err := carbonHotkey(accelerator)
	if err != nil {
		return err
	}
	t := &hotkeyTab
	if err := t.load(); err != nil {
		return err
	}
	target := t.api.appTarget()
	if !t.installed {
		types := carbonEventType{carbonKeyboardClass, carbonHotKeyPressed}
		var handler uintptr
		if st := t.api.install(target, t.proc, 1, &types, 0, &handler); st != 0 {
			return fmt.Errorf("tango/darwin: InstallEventHandler: OSStatus %d", st)
		}
		t.installed = true
	}
	b.UnregisterHotkey(hid)
	// EventHotKeyID is passed by value; its two UInt32 fields travel packed
	// in one 64-bit register (signature in the low half).
	key := uint64(tangoHotKeyTag) | uint64(hid)<<32
	var ref uintptr
	if st := t.api.register(code, mods, key, target, carbonHotKeyExclusive, &ref); st != 0 {
		return fmt.Errorf("tango/darwin: hotkey %q is already registered by this or another app (OSStatus %d)", accelerator, st)
	}
	t.refs[hid], t.fire[hid] = ref, fire
	return nil
}

func (b *backend) UnregisterHotkey(hid uint32) {
	t := &hotkeyTab
	if ref, ok := t.refs[hid]; ok {
		t.api.unregister(ref)
		delete(t.refs, hid)
		delete(t.fire, hid)
	}
}

// ---------------------------------------------------------------------------
// Notifications

var errNotBundled = errors.New("tango/darwin: notifications need the app to run from a .app bundle " +
	"with a CFBundleIdentifier; UNUserNotificationCenter does not accept a bare executable (go run, go build output)")

// UNAuthorizationOptions and UNNotificationPresentationOptions bits
// (UNUserNotificationCenter.h).
const (
	unAuthBadge  = 1 << 0
	unAuthSound  = 1 << 1
	unAuthAlert  = 1 << 2
	unShowSound  = 1 << 1
	unShowAlert  = 1 << 2 // pre-macOS 11
	unShowList   = 1 << 3
	unShowBanner = 1 << 4
)

// authState tracks the authorization request of this process.
type authState int

const (
	authUnknown authState = iota
	authPending
	authDone
)

type notificationCenterState struct {
	setupOnce sync.Once
	ready     bool // framework loaded, delegate installed
	delegate  id

	auth     authState
	granted  bool
	authErr  error
	onAnswer []func() // run on the main thread when auth becomes authDone

	clicks map[string]func() // request identifier -> handler, main thread
}

var unCenter = notificationCenterState{clicks: map[string]func(){}}

// isAppBundle reports whether the process runs from an .app bundle that
// has an identifier.
func isAppBundle() bool {
	var ok bool
	withPool(func() {
		main := send(class("NSBundle"), "mainBundle")
		ok = send(main, "bundleIdentifier") != 0 &&
			strings.HasSuffix(goString(send(main, "bundlePath")), ".app")
	})
	return ok
}

// installNotificationDelegate sets the notification center's delegate
// before the app finishes launching, as UserNotifications requires for the
// click that launched the app to be delivered. Bundled apps only (center
// refuses the others).
func installNotificationDelegate() {
	withPool(func() { _, _ = unCenter.center() })
}

// center returns [UNUserNotificationCenter currentNotificationCenter],
// installing the delegate the first time.
func (c *notificationCenterState) center() (id, error) {
	if !isAppBundle() {
		return 0, errNotBundled
	}
	c.setupOnce.Do(func() {
		if _, err := purego.Dlopen("/System/Library/Frameworks/UserNotifications.framework/UserNotifications",
			purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil || !hasClass("UNUserNotificationCenter") {
			return
		}
		classDef("TangoNotificationDelegate", "NSObject", []string{"UNUserNotificationCenterDelegate"}, []objc.MethodDef{
			// Present notifications while the app is frontmost too.
			method("userNotificationCenter:willPresentNotification:withCompletionHandler:",
				func(_ id, _ objc.SEL, _, _ id, completion uintptr) {
					invokeBlock(completion, unShowSound|unShowAlert|unShowList|unShowBanner)
				}),
			method("userNotificationCenter:didReceiveNotificationResponse:withCompletionHandler:",
				func(_ id, _ objc.SEL, _, response id, completion uintptr) {
					req := send(send(response, "notification"), "request")
					ident := goString(send(req, "identifier"))
					mainHop.post(func() {
						if fn := unCenter.clicks[ident]; fn != nil {
							fn()
							return
						}
						// Sent by an earlier run (possibly the click that
						// launched this one).
						if b := theBackend; b != nil {
							b.appEvent(platform.AppEvent{Kind: platform.AppNotificationOpened, Notification: ident})
						}
					})
					invokeBlock(completion)
				}),
		})
		withPool(func() {
			c.delegate = alloc("TangoNotificationDelegate")
			send(send(class("UNUserNotificationCenter"), "currentNotificationCenter"), "setDelegate:", uintptr(c.delegate))
		})
		c.ready = true
	})
	if !c.ready {
		return 0, platform.ErrUnsupported
	}
	return send(class("UNUserNotificationCenter"), "currentNotificationCenter"), nil
}

// authorize asks for permission (the system prompts once, later calls
// answer from System Settings) and runs then on the main thread with the
// answer recorded.
func (c *notificationCenterState) authorize(center id, then func()) {
	c.onAnswer = append(c.onAnswer, then)
	if c.auth == authPending {
		return
	}
	c.auth = authPending
	handler := objc.NewBlock(func(_ objc.Block, granted bool, nsErr id) {
		var err error
		if nsErr != 0 {
			err = fmt.Errorf("tango/darwin: notification authorization: %s", goString(send(nsErr, "localizedDescription")))
		}
		mainHop.post(func() {
			c.auth, c.granted, c.authErr = authDone, granted, err
			waiting := c.onAnswer
			c.onAnswer = nil
			for _, fn := range waiting {
				fn()
			}
		})
	})
	send(center, "requestAuthorizationWithOptions:completionHandler:", unAuthBadge|unAuthSound|unAuthAlert, uintptr(handler))
	handler.Release()
}

func (b *backend) RequestNotifyPermission(done func(granted bool, err error)) {
	c := &unCenter
	var center id
	var err error
	withPool(func() { center, err = c.center() })
	if err != nil {
		done(false, err)
		return
	}
	if c.auth == authDone {
		c.auth = authUnknown // re-read the current setting
	}
	withPool(func() {
		c.authorize(center, func() { done(c.granted, c.authErr) })
	})
}

func (b *backend) Notify(n platform.Notification, onClick func()) error {
	c := &unCenter
	var center id
	var err error
	withPool(func() { center, err = c.center() })
	if err != nil {
		return err
	}
	deliver := func() {
		if !c.granted {
			return
		}
		c.clicks[n.ID] = onClick
		withPool(func() { c.post(center, n) })
	}
	switch c.auth {
	case authDone:
		if !c.granted {
			if c.authErr != nil {
				return c.authErr
			}
			return errors.New("tango/darwin: the user has not allowed notifications for this app (System Settings > Notifications)")
		}
		deliver()
	default:
		// Requests added before the user answered the first prompt are
		// discarded by the system, so wait for the answer.
		withPool(func() { c.authorize(center, deliver) })
	}
	return nil
}

func (c *notificationCenterState) post(center id, n platform.Notification) {
	content := autorelease(alloc("UNMutableNotificationContent"))
	send(content, "setTitle:", uintptr(nsString(n.Title)))
	send(content, "setSubtitle:", uintptr(nsString(n.Subtitle)))
	send(content, "setBody:", uintptr(nsString(n.Body)))
	if !n.Silent {
		send(content, "setSound:", uintptr(send(class("UNNotificationSound"), "defaultSound")))
	}
	// A nil trigger delivers the notification right away.
	request := send(class("UNNotificationRequest"), "requestWithIdentifier:content:trigger:",
		uintptr(nsString(n.ID)), uintptr(content), 0)
	ident := n.ID
	added := objc.NewBlock(func(_ objc.Block, nsErr id) {
		if nsErr != 0 {
			mainHop.post(func() { delete(c.clicks, ident) })
		}
	})
	send(center, "addNotificationRequest:withCompletionHandler:", uintptr(request), uintptr(added))
	added.Release()
}
