//go:build darwin

package macoslite

// Native bindings. Images are dlopened on first use and every entry point is
// bound by name through purego; Objective-C methods go through objc_msgSend
// with typed function shapes (struct and float arguments cannot use a
// variadic word-only call). Only AppKit, CoreFoundation, CoreGraphics and
// CoreText are opened here, and the package never creates a WebView or a
// Metal device. AppKit itself still maps Metal.framework, and on macOS 15+
// WritingToolsUI maps WebKit.framework once a window becomes key; neither
// starts a WebContent process.

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type id = objc.ID

// CGFloat is a double on every 64-bit Apple target.
type cgPoint struct{ X, Y float64 }
type cgSize struct{ W, H float64 }
type cgRect struct {
	Origin cgPoint
	Size   cgSize
}

const frameworks = "/System/Library/Frameworks/"

var (
	libObjC, libCF, libCG, libCT, libAppKit uintptr

	objcMsgSend uintptr

	// Typed objc_msgSend shapes.
	msgID          func(recv id, op objc.SEL) id
	msgIDID        func(recv id, op objc.SEL, a id) id
	msgVoidID      func(recv id, op objc.SEL, a id)
	msgVoidBool    func(recv id, op objc.SEL, a bool)
	msgVoidInt     func(recv id, op objc.SEL, a int)
	msgBoolID      func(recv id, op objc.SEL, a id) bool
	msgFloat       func(recv id, op objc.SEL) float64
	msgVoidFloat   func(recv id, op objc.SEL, a float64)
	msgRect        func(recv id, op objc.SEL) cgRect
	msgPoint       func(recv id, op objc.SEL) cgPoint
	msgConvert     func(recv id, op objc.SEL, p cgPoint, view id) cgPoint
	msgUint16      func(recv id, op objc.SEL) uint16
	msgInt         func(recv id, op objc.SEL) int
	msgInitWindow  func(recv id, op objc.SEL, r cgRect, style uint, backing uint, deferred bool) id
	msgInitRect    func(recv id, op objc.SEL, r cgRect) id
	msgPerformMain func(recv id, op objc.SEL, sel objc.SEL, arg id, wait bool)
	msgOtherEvent  func(recv id, op objc.SEL, kind uint, where cgPoint, mods uint, stamp float64, window int, ctx id, subtype int16, d1, d2 int) id
	msgPostEvent   func(recv id, op objc.SEL, ev id, atStart bool)

	arPoolPush    func() uintptr
	arPoolPop     func(uintptr)
	pthreadMainNP func() int32

	cfRelease                func(uintptr)
	cfRetain                 func(uintptr) uintptr
	cfStringCreateWithBytes  func(alloc uintptr, b *byte, n int, enc uint32, ext bool) uintptr
	cfDictionaryCreate       func(alloc uintptr, keys, vals *uintptr, n int, kcb, vcb uintptr) uintptr
	cfAttributedStringCreate func(alloc, s, attrs uintptr) uintptr
	cfDataCreate             func(alloc uintptr, b unsafe.Pointer, n int) uintptr

	cgColorSpaceCreateWithName     func(name uintptr) uintptr
	cgColorSpaceRelease            func(uintptr)
	cgBitmapContextCreate          func(data unsafe.Pointer, w, h, bpc, stride int, space uintptr, info uint32) uintptr
	cgContextRelease               func(uintptr)
	cgContextSetRGBFillColor       func(ctx uintptr, r, g, b, a float64)
	cgContextSetTextPosition       func(ctx uintptr, x, y float64)
	cgContextScaleCTM              func(ctx uintptr, sx, sy float64)
	cgContextTranslateCTM          func(ctx uintptr, tx, ty float64)
	cgContextSaveGState            func(ctx uintptr)
	cgContextRestoreGState         func(ctx uintptr)
	cgContextSetShouldSmoothFonts  func(ctx uintptr, b bool)
	cgDataProviderCreateWithCFData func(data uintptr) uintptr
	cgDataProviderRelease          func(uintptr)
	cgImageCreate                  func(w, h, bpc, bpp, stride int, space uintptr, info uint32, provider uintptr, decode unsafe.Pointer, interp bool, intent int) uintptr
	cgImageRelease                 func(uintptr)
	cgColorCreateSRGB              func(r, g, b, a float64) uintptr
	cgColorRelease                 func(uintptr)

	ctFontCreateWithName             func(name uintptr, size float64, matrix unsafe.Pointer) uintptr
	ctFontCreateUIFontForLanguage    func(kind uint32, size float64, lang uintptr) uintptr
	ctLineCreateWithAttributedString func(uintptr) uintptr
	ctLineDraw                       func(line, ctx uintptr)

	kCTFontAttributeName            uintptr
	kCTForegroundColorAttributeName uintptr
	kCFTypeDictionaryKeyCallBacks   uintptr
	kCFTypeDictionaryValueCallBacks uintptr
	kCFBooleanTrue                  uintptr
	kCGColorSpaceSRGB               uintptr

	selCache sync.Map // string -> objc.SEL
)

var (
	loadOnce sync.Once
	loadErr  error
)

type linker struct{ errs []error }

func (l *linker) open(path string) uintptr {
	h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("open %s: %w", path, err))
	}
	return h
}

func (l *linker) sym(img uintptr, name string) uintptr {
	if img == 0 {
		return 0
	}
	a, err := purego.Dlsym(img, name)
	if err != nil || a == 0 {
		l.errs = append(l.errs, fmt.Errorf("resolve %s: %v", name, err))
		return 0
	}
	return a
}

func (l *linker) bind(fn any, img uintptr, name string) {
	if a := l.sym(img, name); a != 0 {
		purego.RegisterFunc(fn, a)
	}
}

// constPtr reads a pointer-valued exported constant (CFStringRef globals).
func (l *linker) constPtr(img uintptr, name string) uintptr {
	if a := l.sym(img, name); a != 0 {
		return loadWord(a)
	}
	return 0
}

// loadWord reads the machine word at a C address; the double indirection
// avoids the uintptr→unsafe.Pointer conversion vet rejects.
func loadWord(addr uintptr) uintptr {
	p := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	return *(*uintptr)(p)
}

// loadNative links the runtime and frameworks once. It is safe to call from
// any goroutine and cheap after the first call.
func loadNative() error {
	loadOnce.Do(func() {
		var l linker
		libObjC = l.open("/usr/lib/libobjc.A.dylib")
		libCF = l.open(frameworks + "CoreFoundation.framework/CoreFoundation")
		libCG = l.open(frameworks + "CoreGraphics.framework/CoreGraphics")
		libCT = l.open(frameworks + "CoreText.framework/CoreText")
		libAppKit = l.open(frameworks + "AppKit.framework/AppKit")
		if len(l.errs) > 0 {
			loadErr = errors.Join(l.errs...)
			return
		}
		objcMsgSend = l.sym(libObjC, "objc_msgSend")
		big := objcMsgSend
		if runtime.GOARCH == "amd64" {
			big = l.sym(libObjC, "objc_msgSend_stret")
		}
		if objcMsgSend != 0 && big != 0 {
			for _, f := range []any{&msgID, &msgIDID, &msgVoidID, &msgVoidBool, &msgVoidInt, &msgBoolID,
				&msgFloat, &msgVoidFloat, &msgPoint, &msgConvert, &msgUint16, &msgInt, &msgInitWindow,
				&msgInitRect, &msgPerformMain, &msgOtherEvent, &msgPostEvent} {
				purego.RegisterFunc(f, objcMsgSend)
			}
			purego.RegisterFunc(&msgRect, big)
		}
		l.bind(&arPoolPush, libObjC, "objc_autoreleasePoolPush")
		l.bind(&arPoolPop, libObjC, "objc_autoreleasePoolPop")
		l.bind(&pthreadMainNP, purego.RTLD_DEFAULT, "pthread_main_np")

		l.bind(&cfRelease, libCF, "CFRelease")
		l.bind(&cfRetain, libCF, "CFRetain")
		l.bind(&cfStringCreateWithBytes, libCF, "CFStringCreateWithBytes")
		l.bind(&cfDictionaryCreate, libCF, "CFDictionaryCreate")
		l.bind(&cfAttributedStringCreate, libCF, "CFAttributedStringCreate")
		l.bind(&cfDataCreate, libCF, "CFDataCreate")
		kCFTypeDictionaryKeyCallBacks = l.sym(libCF, "kCFTypeDictionaryKeyCallBacks")
		kCFTypeDictionaryValueCallBacks = l.sym(libCF, "kCFTypeDictionaryValueCallBacks")
		kCFBooleanTrue = l.constPtr(libCF, "kCFBooleanTrue")

		l.bind(&cgColorSpaceCreateWithName, libCG, "CGColorSpaceCreateWithName")
		kCGColorSpaceSRGB = l.constPtr(libCG, "kCGColorSpaceSRGB")
		l.bind(&cgColorSpaceRelease, libCG, "CGColorSpaceRelease")
		l.bind(&cgBitmapContextCreate, libCG, "CGBitmapContextCreate")
		l.bind(&cgContextRelease, libCG, "CGContextRelease")
		l.bind(&cgContextSetRGBFillColor, libCG, "CGContextSetRGBFillColor")
		l.bind(&cgContextSetTextPosition, libCG, "CGContextSetTextPosition")
		l.bind(&cgContextScaleCTM, libCG, "CGContextScaleCTM")
		l.bind(&cgContextTranslateCTM, libCG, "CGContextTranslateCTM")
		l.bind(&cgContextSaveGState, libCG, "CGContextSaveGState")
		l.bind(&cgContextRestoreGState, libCG, "CGContextRestoreGState")
		l.bind(&cgContextSetShouldSmoothFonts, libCG, "CGContextSetShouldSmoothFonts")
		l.bind(&cgDataProviderCreateWithCFData, libCG, "CGDataProviderCreateWithCFData")
		l.bind(&cgDataProviderRelease, libCG, "CGDataProviderRelease")
		l.bind(&cgImageCreate, libCG, "CGImageCreate")
		l.bind(&cgImageRelease, libCG, "CGImageRelease")
		l.bind(&cgColorCreateSRGB, libCG, "CGColorCreateSRGB")
		l.bind(&cgColorRelease, libCG, "CGColorRelease")

		l.bind(&ctFontCreateWithName, libCT, "CTFontCreateWithName")
		l.bind(&ctFontCreateUIFontForLanguage, libCT, "CTFontCreateUIFontForLanguage")
		l.bind(&ctLineCreateWithAttributedString, libCT, "CTLineCreateWithAttributedString")
		l.bind(&ctLineDraw, libCT, "CTLineDraw")
		kCTFontAttributeName = l.constPtr(libCT, "kCTFontAttributeName")
		kCTForegroundColorAttributeName = l.constPtr(libCT, "kCTForegroundColorAttributeName")

		if len(l.errs) > 0 {
			loadErr = errors.Join(l.errs...)
		}
	})
	return loadErr
}

// sel caches selector registration (sel_registerName takes a global lock).
func sel(name string) objc.SEL {
	if s, ok := selCache.Load(name); ok {
		return s.(objc.SEL)
	}
	s := objc.RegisterName(name)
	selCache.Store(name, s)
	return s
}

func class(name string) id { return id(objc.GetClass(name)) }

// send calls a no-argument method returning an object.
func send(recv id, name string) id { return msgID(recv, sel(name)) }

// cfString returns an owned CFString for s (UTF-8), or 0.
func cfString(s string) uintptr {
	if s == "" {
		return cfStringCreateWithBytes(0, nil, 0, 0x08000100, false)
	}
	b := []byte(s)
	return cfStringCreateWithBytes(0, &b[0], len(b), 0x08000100, false)
}

// withPool runs fn inside an autorelease pool on a locked OS thread.
func withPool(fn func()) {
	runtime.LockOSThread()
	t := arPoolPush()
	defer func() {
		arPoolPop(t)
		runtime.UnlockOSThread()
	}()
	fn()
}
