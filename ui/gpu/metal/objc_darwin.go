//go:build darwin

package metal

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// A small typed layer over the Objective-C runtime (purego/objc, no cgo).
// Messages returning an object or nothing go through objc.ID.Send; messages
// with float or struct arguments or results go through objc_msgSend
// registered once per Go signature below, so the hot path does not build a
// new reflective function per call (objc.Send[T] does).

type id = objc.ID

// Metal structs (64-bit layouts). How the ones larger than 16 bytes
// travel depends on the architecture; see bigargs_*_darwin.go.
type (
	mtlOrigin      struct{ X, Y, Z uint64 }
	mtlSize        struct{ W, H, D uint64 }
	mtlClearColor  struct{ R, G, B, A float64 } // an HFA: by value in float registers
	mtlScissorRect struct{ X, Y, W, H uint64 }
)

// Metal enum values used here (MTLPixelFormat, MTLStorageMode, …).
const (
	pixelFormatR8Unorm     = 10
	pixelFormatRGBA8Unorm  = 70
	pixelFormatRGBA16Unorm = 110

	storageModeShared = 0
	cpuCacheModeWC    = 1 // MTLResourceCPUCacheModeWriteCombined

	textureStoragePrivate = 2 // MTLStorageModePrivate (texture descriptor property)

	usageShaderRead   = 1
	usageRenderTarget = 4

	loadActionLoad   = 1
	loadActionClear  = 2
	storeActionStore = 1

	primitiveTriangleStrip = 4
	vertexFormatFloat      = 28 // MTLVertexFormatFloat; Float2..4 = 29..31
	vertexFormatInt        = 32 // MTLVertexFormatInt; Int2..4 = 33..35
	vertexFormatUInt       = 36 // MTLVertexFormatUInt; UInt2..4 = 37..39
	stepPerInstance        = 2

	blendOne                 = 1
	blendOneMinusSourceAlpha = 5

	cbStatusCompleted = 4

	mathModeSafe = 0 // MTLMathModeSafe
)

var (
	loadOnce sync.Once
	loadErr  error

	mtlCreateSystemDefaultDevice func() id

	// Typed objc_msgSend variants.
	msgU64         func(o id, s objc.SEL) uint64
	msgF64         func(o id, s objc.SEL) float64
	msgPtr         func(o id, s objc.SEL) unsafe.Pointer
	msgBool1       func(o id, s objc.SEL, a objc.SEL) bool
	msgSetU64      func(o id, s objc.SEL, v uint64)
	msgSetBool     func(o id, s objc.SEL, v bool)
	msgSetID       func(o id, s objc.SEL, v id)
	msgSetIDIdx    func(o id, s objc.SEL, v id, i uint64)
	msgIdx         func(o id, s objc.SEL, i uint64) id
	msgClearColor  func(o id, s objc.SEL, c mtlClearColor)
	msgBufOffIdx   func(o id, s objc.SEL, buf id, off, i uint64)
	msgBytesLenIdx func(o id, s objc.SEL, p unsafe.Pointer, n, i uint64)
	msgDraw        func(o id, s objc.SEL, prim, start, count, instances uint64)
	msgNewBuffer   func(o id, s objc.SEL, n, opts uint64) id
	msgTexDesc     func(c id, s objc.SEL, format, w, h uint64, mip bool) id
	msgWithErr     func(o id, s objc.SEL, a id, err *id) id
	msgWithOptErr  func(o id, s objc.SEL, a, opts id, err *id) id
	msgInitBytes   func(o id, s objc.SEL, p unsafe.Pointer, n, enc uint64) id
	msgU64Arg      func(o id, s objc.SEL, a uint64) uint64
)

func load() error {
	loadOnce.Do(func() {
		lib, err := purego.Dlopen("/System/Library/Frameworks/Metal.framework/Metal", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			loadErr = fmt.Errorf("%w: load Metal.framework: %v", ErrNoDevice, err)
			return
		}
		purego.RegisterLibFunc(&mtlCreateSystemDefaultDevice, lib, "MTLCreateSystemDefaultDevice")
		libobjc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			loadErr = fmt.Errorf("%w: load libobjc: %v", ErrNoDevice, err)
			return
		}
		send, err := purego.Dlsym(libobjc, "objc_msgSend")
		if err != nil {
			loadErr = fmt.Errorf("%w: objc_msgSend: %v", ErrNoDevice, err)
			return
		}
		for _, fn := range []any{&msgU64, &msgF64, &msgPtr, &msgBool1, &msgSetU64, &msgSetBool, &msgSetID, &msgSetIDIdx,
			&msgIdx, &msgClearColor, &msgBufOffIdx, &msgBytesLenIdx, &msgDraw, &msgNewBuffer,
			&msgTexDesc, &msgWithErr, &msgWithOptErr, &msgInitBytes, &msgU64Arg} {
			purego.RegisterFunc(fn, send)
		}
		registerBigArgs(send)
	})
	return loadErr
}

// nameTable memoizes an Objective-C runtime lookup by name. Selectors and
// loaded classes live for the whole process, so entries never expire. A
// zero result is not stored: a class can appear later, once the framework
// that defines it gets loaded.
type nameTable[T comparable] struct {
	mu      sync.RWMutex
	entries map[string]T
	resolve func(string) T
}

func (t *nameTable[T]) get(name string) T {
	t.mu.RLock()
	v, hit := t.entries[name]
	t.mu.RUnlock()
	if hit {
		return v
	}
	var zero T
	if v = t.resolve(name); v == zero {
		return zero
	}
	t.mu.Lock()
	if t.entries == nil {
		t.entries = make(map[string]T)
	}
	t.entries[name] = v
	t.mu.Unlock()
	return v
}

var (
	selectors = nameTable[objc.SEL]{resolve: objc.RegisterName}
	classes   = nameTable[id]{resolve: func(n string) id { return id(objc.GetClass(n)) }}
)

// sel returns the selector registered under name (sel_registerName).
func sel(name string) objc.SEL { return selectors.get(name) }

// class returns the class object named name, ready to receive class
// messages, or 0 when no such class is loaded.
func class(name string) id { return classes.get(name) }

// newObject returns [[cls alloc] init] (+1 retained).
func newObject(cls string) id { return class(cls).Send(sel("new")) }

func release(o id) {
	if o != 0 {
		o.Send(sel("release"))
	}
}

func retain(o id) id {
	if o != 0 {
		o.Send(sel("retain"))
	}
	return o
}

// utf8Encoding is NSUTF8StringEncoding.
const utf8Encoding = 4

// nsString returns an NSString holding the UTF-8 text s, or 0 if s is not
// valid UTF-8.
//
// Ownership: the result is autoreleased, never owned by the caller. Callers
// pass it straight into a message and do not release it, so it must be
// created inside an autorelease pool (see autoreleasePool). Internally the
// string comes from alloc + initWithBytes:length:encoding: (+1) and is then
// handed to the pool with -autorelease. The bytes are copied by the
// initializer and passed with an explicit length, so s needs no terminator.
func nsString(s string) id {
	obj := class("NSString").Send(sel("alloc"))
	str := msgInitBytes(obj, sel("initWithBytes:length:encoding:"),
		unsafe.Pointer(unsafe.StringData(s)), uint64(len(s)), utf8Encoding)
	runtime.KeepAlive(s)
	if str == 0 { // the initializer already released obj
		return 0
	}
	return str.Send(sel("autorelease"))
}

// goString copies the text of an NSString into a Go string.
//
// The byte count comes from -lengthOfBytesUsingEncoding: and exactly that
// many bytes are copied out of -UTF8String, so the copy never depends on
// finding a terminator (and keeps any embedded U+0000).
func goString(s id) string {
	if s == 0 {
		return ""
	}
	size := msgU64Arg(s, sel("lengthOfBytesUsingEncoding:"), utf8Encoding)
	if size == 0 {
		return ""
	}
	buf := msgPtr(s, sel("UTF8String"))
	if buf == nil {
		return ""
	}
	return strings.Clone(unsafe.String((*byte)(buf), int(size)))
}

// nsError turns an NSError into a Go error.
func nsError(what string, e id) error {
	if e == 0 {
		return errors.New(what)
	}
	return fmt.Errorf("%s: %s", what, goString(e.Send(sel("localizedDescription"))))
}

// autoreleasePool pushes an NSAutoreleasePool; call the returned function
// (on the same OS thread) to drain it.
func autoreleasePool() func() {
	p := newObject("NSAutoreleasePool")
	return func() { p.Send(sel("drain")) }
}
