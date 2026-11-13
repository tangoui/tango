//go:build darwin && !ios

package darwin

// Objective-C runtime layer.
//
// The backend never links against AppKit at build time. Instead it dlopens the
// system images on first use, binds the handful of C entry points it needs by
// name, and reaches every Objective-C method through objc_msgSend as described
// in Apple's "Objective-C Runtime Programming Guide › Messaging". Conversions
// between Go values and Foundation objects live in foundation_values.go.

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type id = objc.ID

// Geometry values passed by value to AppKit. On every 64-bit Apple target
// CGFloat is a double, so CGPoint and CGSize are two doubles each and CGRect is
// an origin followed by a size.
type nsPoint struct {
	X float64
	Y float64
}

type nsSize struct {
	Width  float64
	Height float64
}

type nsRect struct {
	Origin nsPoint
	Size   nsSize
}

// Image handles, filled in by load.
var libObjC, libCF, libAppKit, libWebKit, libFoundation uintptr

// msgSendAddr is the address of objc_msgSend; callers with plain word-sized
// arguments go through send, everything else binds a typed variant to it.
var msgSendAddr uintptr

// Typed objc_msgSend shapes for calls whose arguments or results include
// structs or doubles (those travel in vector registers or memory, which the
// word-only send path cannot express).
var (
	msgRect               func(recv id, op objc.SEL) nsRect
	msgContentRectForRect func(recv id, op objc.SEL, frame nsRect) nsRect
	msgSetSize            func(recv id, op objc.SEL, size nsSize)
	msgInitWindow         func(recv id, op objc.SEL, frame nsRect, styleMask uint, backingStore uint, deferred bool) id
	msgInitRect           func(recv id, op objc.SEL, frame nsRect) id
	msgInitRectID         func(recv id, op objc.SEL, frame nsRect, extra id) id
	msgNextEvent          func(recv id, op objc.SEL, eventMask uint64, deadline id, runMode id, dequeue bool) id
	msgOtherEvent         func(recv id, op objc.SEL, kind uint, where nsPoint, modifiers uint, stamp float64, window int, gfx id, subtype int16, data1, data2 int) id
)

// CoreFoundation run-loop entry points. The signal / wake-up functions are
// kept as raw addresses because they are invoked from arbitrary goroutines via
// purego.SyscallN.
var (
	cfRunLoopGetMain        func() uintptr
	cfRunLoopSourceCreate   func(allocator uintptr, priority int, context unsafe.Pointer) uintptr
	cfRunLoopAddSource      func(loop, source, mode uintptr)
	cfRunLoopSourceSignalFn uintptr
	cfRunLoopWakeUpFn       uintptr
	kCFRunLoopCommonModes   uintptr
)

// Small runtime helpers bound as typed functions.
var (
	arPoolPush        func() uintptr
	arPoolPop         func(token uintptr)
	pthreadMainNP     func() int32
	cfStringFromBytes func(allocator uintptr, bytes *byte, length int, encoding uint32, external bool) uintptr
)

var loadOnce sync.Once

const frameworksDir = "/System/Library/Frameworks/"

// linker opens images and resolves symbols, remembering every failure so that
// a broken environment is reported in one message rather than one at a time.
type linker struct{ failures []error }

func (l *linker) open(path string) uintptr {
	h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		l.failures = append(l.failures, fmt.Errorf("open %s: %w", path, err))
		return 0
	}
	return h
}

func (l *linker) symbol(image uintptr, name string) uintptr {
	addr, err := purego.Dlsym(image, name)
	if err != nil || addr == 0 {
		l.failures = append(l.failures, fmt.Errorf("resolve %s: %v", name, err))
		return 0
	}
	return addr
}

// bind resolves name and attaches it to the Go function pointed to by fnPtr.
func (l *linker) bind(fnPtr any, image uintptr, name string) {
	if addr := l.symbol(image, name); addr != 0 {
		purego.RegisterFunc(fnPtr, addr)
	}
}

// check aborts start-up if anything failed to resolve; the backend cannot run
// with a partial binding.
func (l *linker) check() {
	if len(l.failures) > 0 {
		panic("tango/darwin: dynamic linking failed: " + errors.Join(l.failures...).Error())
	}
}

func mustDlopen(path string) uintptr {
	var l linker
	h := l.open(path)
	l.check()
	return h
}

func mustDlsym(lib uintptr, name string) uintptr {
	var l linker
	addr := l.symbol(lib, name)
	l.check()
	return addr
}

// load links the runtime and the frameworks the backend uses. It is idempotent
// and cheap after the first call.
func load() {
	loadOnce.Do(func() {
		var l linker
		images := []struct {
			handle *uintptr
			path   string
		}{
			{&libObjC, "/usr/lib/libobjc.A.dylib"},
			{&libCF, frameworksDir + "CoreFoundation.framework/CoreFoundation"},
			{&libFoundation, frameworksDir + "Foundation.framework/Foundation"},
			{&libAppKit, frameworksDir + "AppKit.framework/AppKit"},
			{&libWebKit, frameworksDir + "WebKit.framework/WebKit"},
		}
		for _, img := range images {
			*img.handle = l.open(img.path)
		}
		l.check()
		// UniformTypeIdentifiers only exists on macOS 11 and later; dialogs.go
		// falls back to extension lists when UTType is missing.
		purego.Dlopen(frameworksDir+"UniformTypeIdentifiers.framework/UniformTypeIdentifiers", purego.RTLD_NOW|purego.RTLD_GLOBAL)

		msgSendAddr = l.symbol(libObjC, "objc_msgSend")
		// The x86-64 ABI returns structs wider than two words through a hidden
		// pointer, which needs objc_msgSend_stret; arm64 has a single entry.
		bigReturn := msgSendAddr
		if runtime.GOARCH == "amd64" {
			bigReturn = l.symbol(libObjC, "objc_msgSend_stret")
		}
		l.check()
		for _, v := range []struct {
			fn   any
			addr uintptr
		}{
			{&msgRect, bigReturn},
			{&msgContentRectForRect, bigReturn},
			{&msgSetSize, msgSendAddr},
			{&msgInitWindow, msgSendAddr},
			{&msgInitRect, msgSendAddr},
			{&msgInitRectID, msgSendAddr},
			{&msgNextEvent, msgSendAddr},
			{&msgOtherEvent, msgSendAddr},
		} {
			purego.RegisterFunc(v.fn, v.addr)
		}

		l.bind(&arPoolPush, libObjC, "objc_autoreleasePoolPush")
		l.bind(&arPoolPop, libObjC, "objc_autoreleasePoolPop")
		l.bind(&pthreadMainNP, purego.RTLD_DEFAULT, "pthread_main_np")

		l.bind(&cfRunLoopGetMain, libCF, "CFRunLoopGetMain")
		l.bind(&cfRunLoopSourceCreate, libCF, "CFRunLoopSourceCreate")
		l.bind(&cfRunLoopAddSource, libCF, "CFRunLoopAddSource")
		l.bind(&cfStringFromBytes, libCF, "CFStringCreateWithBytes")
		cfRunLoopSourceSignalFn = l.symbol(libCF, "CFRunLoopSourceSignal")
		cfRunLoopWakeUpFn = l.symbol(libCF, "CFRunLoopWakeUp")
		l.check()

		kCFRunLoopCommonModes = constPtr(libCF, "kCFRunLoopCommonModes")
	})
}

// constPtr returns the value of an exported pointer-sized global, such as a
// CFStringRef / NSString * constant. Missing symbols yield 0.
func constPtr(lib uintptr, name string) uintptr {
	addr, err := purego.Dlsym(lib, name)
	if err != nil || addr == 0 {
		return 0
	}
	return loadWord(addr)
}

// loadWord reads the machine word stored at a C address. The double
// indirection converts the integer address into a pointer without the
// uintptr→unsafe.Pointer pattern that vet rejects.
func loadWord(addr uintptr) uintptr {
	p := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	return *(*uintptr)(p)
}

func isMainThread() bool { return pthreadMainNP() != 0 }

// nameCache is a copy-on-write string-keyed map: lookups read an immutable
// snapshot without locking or allocating, and the rare miss rebuilds the
// snapshot under a mutex. Selectors and classes are resolved a few hundred
// times per process, then only read.
type nameCache[V comparable] struct {
	mu   sync.Mutex
	snap atomic.Pointer[map[string]V]
}

func (c *nameCache[V]) lookup(name string) (V, bool) {
	if m := c.snap.Load(); m != nil {
		v, ok := (*m)[name]
		return v, ok
	}
	var zero V
	return zero, false
}

func (c *nameCache[V]) store(name string, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cur := c.snap.Load()
	next := make(map[string]V, 1+lenOf(cur))
	if cur != nil {
		for k, old := range *cur {
			next[k] = old
		}
	}
	next[name] = v
	c.snap.Store(&next)
}

func lenOf[V any](m *map[string]V) int {
	if m == nil {
		return 0
	}
	return len(*m)
}

var (
	selectorCache nameCache[objc.SEL]
	classCache    nameCache[id]
)

// sel interns a selector name (sel_registerName).
func sel(name string) objc.SEL {
	if s, ok := selectorCache.lookup(name); ok {
		return s
	}
	s := objc.RegisterName(name)
	selectorCache.store(name, s)
	return s
}

// findClass looks a class up by name, caching only successful lookups so a
// class that appears later (lazy framework load) is still found.
func findClass(name string) (id, bool) {
	if c, ok := classCache.lookup(name); ok {
		return c, true
	}
	c := id(objc.GetClass(name))
	if c == 0 {
		return 0, false
	}
	classCache.store(name, c)
	return c, true
}

// class returns a class object and treats an unknown name as a programming
// error.
func class(name string) id {
	c, ok := findClass(name)
	if !ok {
		panic("tango/darwin: no Objective-C class named " + name)
	}
	return c
}

// hasClass reports whether the running OS provides the class, for features
// gated on a minimum macOS version.
func hasClass(name string) bool {
	_, ok := findClass(name)
	return ok
}

// send performs objc_msgSend with word-sized arguments (integers, pointers,
// object ids, BOOLs). //go:uintptrescapes keeps any Go memory whose address
// the caller converted to uintptr alive for the duration of the call.
//
//go:uintptrescapes
func send(recv id, op string, args ...uintptr) id {
	self, cmd := uintptr(recv), uintptr(sel(op))
	var ret uintptr
	// The common arities are spelled out so the frame stays on the stack.
	switch len(args) {
	case 0:
		ret, _, _ = purego.SyscallN(msgSendAddr, self, cmd)
	case 1:
		ret, _, _ = purego.SyscallN(msgSendAddr, self, cmd, args[0])
	case 2:
		ret, _, _ = purego.SyscallN(msgSendAddr, self, cmd, args[0], args[1])
	case 3:
		ret, _, _ = purego.SyscallN(msgSendAddr, self, cmd, args[0], args[1], args[2])
	default:
		frame := make([]uintptr, 2, 2+len(args))
		frame[0], frame[1] = self, cmd
		ret, _, _ = purego.SyscallN(msgSendAddr, append(frame, args...)...)
	}
	return id(ret)
}

// sendBool reads a BOOL result. Only the low byte is defined: BOOL is a
// signed char on x86-64 and a C bool on arm64.
//
//go:uintptrescapes
func sendBool(recv id, op string, args ...uintptr) bool {
	return send(recv, op, args...)&0xff != 0
}

// sendInt reads an NSInteger / NSUInteger result.
//
//go:uintptrescapes
func sendInt(recv id, op string, args ...uintptr) int {
	return int(uintptr(send(recv, op, args...)))
}

// respondsTo asks the object itself (respondsToSelector:), which covers
// methods added in newer OS releases and optional protocol methods.
func respondsTo(recv id, op string) bool {
	if recv == 0 {
		return false
	}
	return sendBool(recv, "respondsToSelector:", uintptr(sel(op)))
}

// boolArg encodes a Go bool as an Objective-C BOOL argument word.
func boolArg(v bool) uintptr {
	var w uintptr
	if v {
		w = 1
	}
	return w
}

// sendIfLive forwards a no-argument message unless recv is nil. Messaging nil
// is legal in Objective-C, but skipping it saves a trip through the runtime.
func sendIfLive(recv id, op string) id {
	if recv == 0 {
		return 0
	}
	return send(recv, op)
}

func retain(obj id) id      { return sendIfLive(obj, "retain") }
func release(obj id)        { sendIfLive(obj, "release") }
func autorelease(obj id) id { return sendIfLive(obj, "autorelease") }

// alloc creates an owned (+1) instance; +new is documented as alloc followed
// by init.
func alloc(className string) id { return send(class(className), "new") }

// withPool runs fn with a fresh autorelease pool on a locked OS thread, so
// that objects autoreleased by fn are freed before it returns.
func withPool(fn func()) {
	runtime.LockOSThread()
	token := arPoolPush()
	defer func() {
		arPoolPop(token)
		runtime.UnlockOSThread()
	}()
	fn()
}

// classDef creates and registers a new Objective-C class. Protocols the
// running OS does not know are skipped rather than failing registration.
func classDef(name, super string, protocols []string, methods []objc.MethodDef) id {
	cls, err := objc.RegisterClass(name, objc.GetClass(super), knownProtocols(protocols), nil, methods)
	if err != nil {
		panic(fmt.Sprintf("tango/darwin: registering class %s failed: %v", name, err))
	}
	classCache.store(name, id(cls))
	return id(cls)
}

func knownProtocols(names []string) []*objc.Protocol {
	out := make([]*objc.Protocol, 0, len(names))
	for _, n := range names {
		if p := objc.GetProtocol(n); p != nil {
			out = append(out, p)
		}
	}
	return out
}

// method pairs a selector with its Go implementation for classDef.
func method(selector string, impl any) objc.MethodDef {
	return objc.MethodDef{Fn: impl, Cmd: sel(selector)}
}
