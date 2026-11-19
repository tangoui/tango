//go:build darwin && !ios && arm64

package quartz

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/uithread"
)

type nativePoint struct{ X, Y float64 }
type nativeSize struct{ W, H float64 }
type nativeRect struct {
	Origin nativePoint
	Size   nativeSize
}

// sel interns selector names: objc.RegisterName copies the name into a C
// string on every call, and frames send thousands of messages. Renderers
// belong to their UI thread, but tests and multiple windows may share it.
var selCache sync.Map // string -> objc.SEL

func sel(name string) objc.SEL {
	if v, ok := selCache.Load(name); ok {
		return v.(objc.SEL)
	}
	v := objc.RegisterName(name)
	selCache.Store(name, v)
	return v
}

var nativeOnce sync.Once
var nativeErr error
var nativeSRGB uintptr
var nativeDelegate objc.ID
var nativeNearest objc.ID
var nativePoolPush func() uintptr
var nativePoolPop func(uintptr)

// A display callback can be delivered independently of the renderer's
// explicit displayIfNeeded. Never read mutable node state from another thread.
var nativeDrawersMu sync.RWMutex
var nativeDrawers = map[objc.ID]nativeDrawer{}

type nativeDrawer struct {
	node   *node
	thread *uithread.Thread
}

var setNativeRect func(objc.ID, objc.SEL, nativeRect)
var setNativeFloat func(objc.ID, objc.SEL, float64)
var cgSave, cgRestore, cgContextRelease func(uintptr)
var cgScale, cgTranslate func(uintptr, float64, float64)
var cgClip func(uintptr, nativeRect)
var cgFillSpace func(uintptr, uintptr)
var cgFillColor func(uintptr, *float64)
var cgFontFlags [7]func(uintptr, bool)
var cgColorCreate func(uintptr, *float64) uintptr
var cgColorRelease, cgImageRelease, cgProviderRelease, cfRelease func(uintptr)
var cgDataCreate func(uintptr, unsafe.Pointer, int64) uintptr
var cgProviderCreate func(uintptr) uintptr
var cgImageCreate func(int, int, int, int, int, uintptr, uint32, uintptr, unsafe.Pointer, bool, int32) uintptr
var cgBitmapCreate func(unsafe.Pointer, int, int, int, int, uintptr, uint32) uintptr
var nativeSpaceCreate func(uintptr) uintptr

func loadNative() error {
	nativeOnce.Do(func() {
		defer func() {
			if e := recover(); e != nil {
				nativeErr = fmt.Errorf("quartz: load native graphics: %v", e)
			}
		}()
		open := func(path string) uintptr {
			h, e := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
			if e != nil {
				panic(e)
			}
			return h
		}
		cg := open("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
		cf := open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
		qc := open("/System/Library/Frameworks/QuartzCore.framework/QuartzCore")
		ob := open("/usr/lib/libobjc.A.dylib")
		send, e := purego.Dlsym(ob, "objc_msgSend")
		if e != nil {
			panic(e)
		}
		purego.RegisterLibFunc(&nativePoolPush, ob, "objc_autoreleasePoolPush")
		purego.RegisterLibFunc(&nativePoolPop, ob, "objc_autoreleasePoolPop")
		nearest, e := purego.Dlsym(qc, "kCAFilterNearest")
		if e != nil {
			panic(e)
		}
		nativeNearest = objc.ID(nativeWord(nearest))
		purego.RegisterFunc(&setNativeRect, send)
		purego.RegisterFunc(&setNativeFloat, send)
		for _, entry := range []struct {
			fn   any
			name string
		}{
			{&cgSave, "CGContextSaveGState"}, {&cgRestore, "CGContextRestoreGState"}, {&cgContextRelease, "CGContextRelease"}, {&cgScale, "CGContextScaleCTM"}, {&cgTranslate, "CGContextTranslateCTM"},
			{&cgClip, "CGContextClipToRect"}, {&cgFillSpace, "CGContextSetFillColorSpace"}, {&cgFillColor, "CGContextSetFillColor"}, {&cgColorCreate, "CGColorCreate"}, {&cgColorRelease, "CGColorRelease"}, {&cgImageRelease, "CGImageRelease"}, {&cgProviderRelease, "CGDataProviderRelease"},
			{&cgProviderCreate, "CGDataProviderCreateWithCFData"}, {&cgImageCreate, "CGImageCreate"}, {&cgBitmapCreate, "CGBitmapContextCreate"}, {&nativeSpaceCreate, "CGColorSpaceCreateWithName"},
		} {
			purego.RegisterLibFunc(entry.fn, cg, entry.name)
		}
		purego.RegisterLibFunc(&cgDataCreate, cf, "CFDataCreate")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		for i, name := range []string{"CGContextSetShouldAntialias", "CGContextSetAllowsFontSmoothing", "CGContextSetShouldSmoothFonts", "CGContextSetAllowsFontSubpixelPositioning", "CGContextSetShouldSubpixelPositionFonts", "CGContextSetAllowsFontSubpixelQuantization", "CGContextSetShouldSubpixelQuantizeFonts"} {
			purego.RegisterLibFunc(&cgFontFlags[i], cg, name)
		}
		p, e := purego.Dlsym(cg, "kCGColorSpaceSRGB")
		if e != nil {
			panic(e)
		}
		nativeSRGB = nativeSpaceCreate(nativeWord(p))
		if nativeSRGB == 0 {
			panic("no sRGB color space")
		}
		cls, e := objc.RegisterClass("TangoCoreGraphicsLayerDrawer", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{{Cmd: sel("drawLayer:inContext:"), Fn: func(_ objc.ID, _ objc.SEL, layer objc.ID, ctx uintptr) {
			nativeDrawersMu.RLock()
			d := nativeDrawers[layer]
			nativeDrawersMu.RUnlock()
			if d.node != nil && (d.thread == nil || d.thread.On()) {
				d.node.draw(ctx)
			}
		}}})
		if e != nil {
			panic(e)
		}
		nativeDelegate = objc.ID(cls).Send(sel("new"))
	})
	return nativeErr
}
func nativeTransaction() func() {
	ca := objc.ID(objc.GetClass("CATransaction"))
	ca.Send(sel("begin"))
	ca.Send(sel("setDisableActions:"), true)
	return func() { ca.Send(sel("commit")) }
}
func nativeColor(layer objc.ID, name string, c color.NRGBA) {
	values := [4]float64{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, float64(c.A) / 255}
	native := cgColorCreate(nativeSRGB, &values[0])
	layer.Send(sel(name), native)
	if native != 0 {
		cgColorRelease(native)
	}
}
func nativeImage(img *image.RGBA) (uintptr, error) {
	if img == nil || img.Rect.Empty() {
		return 0, fmt.Errorf("quartz: empty image")
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	off := img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y)
	n := img.Stride*(h-1) + w*4
	if off < 0 || n <= 0 || off+n > len(img.Pix) {
		return 0, fmt.Errorf("quartz: invalid image buffer")
	}
	data := cgDataCreate(0, unsafe.Pointer(&img.Pix[off]), int64(n))
	runtime.KeepAlive(img)
	if data == 0 {
		return 0, fmt.Errorf("quartz: cannot copy image pixels")
	}
	defer cfRelease(data)
	provider := cgProviderCreate(data)
	if provider == 0 {
		return 0, fmt.Errorf("quartz: cannot create image provider")
	}
	defer cgProviderRelease(provider)
	image := cgImageCreate(w, h, 8, 32, img.Stride, nativeSRGB, 1|(4<<12), provider, nil, false, 0)
	if image == 0 {
		return 0, fmt.Errorf("quartz: cannot create native image")
	}
	return image, nil
}

// nativeWord reads a pointer-valued exported C constant. The owning system
// framework remains loaded for the process lifetime. Match the platform FFI's
// address-bit conversion; this is never used for a Go-managed allocation.
func nativeWord(addr uintptr) uintptr {
	p := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	return *(*uintptr)(p)
}
