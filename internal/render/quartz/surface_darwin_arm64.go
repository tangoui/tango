//go:build darwin && !ios && arm64

package quartz

import (
	"fmt"
	"image"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Bulk text is a window-sized raster that changes every frame. Handing Core
// Animation a CGImage makes it convert the whole image (vImage, on the UI
// thread) in every commit. An IOSurface in the layer's native premultiplied
// layout is composited by the window server directly, and Tango rasterizes
// straight into its memory: no Go-heap copy, no CFData copy, no conversion.
//
// A surface the window server may still read (IOSurfaceIsInUse) is never
// written; the renderer alternates between surfaces and allocates at most
// surfaceMax of them, falling back to the CGImage path beyond that.

const surfaceMax = 3

var surfaceOnce sync.Once
var surfaceErr error
var surfaceCreate func(uintptr) uintptr
var surfaceLock, surfaceUnlock func(uintptr, uint32, unsafe.Pointer) int32
var surfaceBase func(uintptr) unsafe.Pointer
var surfaceStride func(uintptr) uintptr
var surfaceInUse func(uintptr) bool
var surfaceSetValue func(uintptr, uintptr, uintptr)
var surfaceKeys [5]uintptr // width, height, bytes per element, pixel format, color space
var surfaceSRGBPlist uintptr
var cgSpacePlist func(uintptr) uintptr

func loadSurface() error {
	surfaceOnce.Do(func() {
		defer func() {
			if e := recover(); e != nil {
				surfaceErr = fmt.Errorf("quartz: load IOSurface: %v", e)
			}
		}()
		h, e := purego.Dlopen("/System/Library/Frameworks/IOSurface.framework/IOSurface", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if e != nil {
			panic(e)
		}
		purego.RegisterLibFunc(&surfaceCreate, h, "IOSurfaceCreate")
		purego.RegisterLibFunc(&surfaceLock, h, "IOSurfaceLock")
		purego.RegisterLibFunc(&surfaceUnlock, h, "IOSurfaceUnlock")
		purego.RegisterLibFunc(&surfaceBase, h, "IOSurfaceGetBaseAddress")
		purego.RegisterLibFunc(&surfaceStride, h, "IOSurfaceGetBytesPerRow")
		purego.RegisterLibFunc(&surfaceInUse, h, "IOSurfaceIsInUse")
		purego.RegisterLibFunc(&surfaceSetValue, h, "IOSurfaceSetValue")
		for i, name := range []string{"kIOSurfaceWidth", "kIOSurfaceHeight", "kIOSurfaceBytesPerElement", "kIOSurfacePixelFormat", "kIOSurfaceColorSpace"} {
			p, e := purego.Dlsym(h, name)
			if e != nil {
				panic(e)
			}
			surfaceKeys[i] = nativeWord(p)
		}
		cg, e := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if e != nil {
			panic(e)
		}
		purego.RegisterLibFunc(&cgSpacePlist, cg, "CGColorSpaceCopyPropertyList")
		surfaceSRGBPlist = cgSpacePlist(nativeSRGB)
	})
	return surfaceErr
}

type surface struct {
	id   uintptr
	size image.Point
}

// surfaces is the bulk raster's IOSurface ring.
type surfaces struct {
	ring  []surface
	next  int
	shown uintptr // the surface the bulk layer displays
}

func newSurface(size image.Point) (surface, error) {
	num := func(v int) objc.ID {
		return objc.ID(objc.GetClass("NSNumber")).Send(sel("numberWithInt:"), int32(v))
	}
	d := objc.ID(objc.GetClass("NSMutableDictionary")).Send(sel("dictionaryWithCapacity:"), uintptr(4))
	d.Send(sel("setObject:forKey:"), num(size.X), surfaceKeys[0])
	d.Send(sel("setObject:forKey:"), num(size.Y), surfaceKeys[1])
	d.Send(sel("setObject:forKey:"), num(4), surfaceKeys[2])
	d.Send(sel("setObject:forKey:"), objc.ID(objc.GetClass("NSNumber")).Send(sel("numberWithUnsignedInt:"), uint32(pixelRGBA)), surfaceKeys[3])
	id := surfaceCreate(uintptr(d))
	if id == 0 {
		return surface{}, fmt.Errorf("quartz: cannot create IOSurface %v", size)
	}
	if surfaceSRGBPlist != 0 {
		surfaceSetValue(id, surfaceKeys[4], surfaceSRGBPlist)
	}
	return surface{id: id, size: size}, nil
}

// take returns a surface of size the window server is not reading, or false.
// It never returns the surface shown now: Core Animation only commits a
// contents change when the contents object changes.
func (r *surfaces) take(size image.Point) (surface, bool) {
	for i := range r.ring {
		if r.ring[i].size != size {
			cfRelease(r.ring[i].id)
			r.ring = append(r.ring[:i], r.ring[i+1:]...)
			return r.take(size)
		}
	}
	for range r.ring {
		s := r.ring[r.next%len(r.ring)]
		r.next++
		if s.id != r.shown && !surfaceInUse(s.id) {
			r.shown = s.id
			return s, true
		}
	}
	if len(r.ring) >= surfaceMax {
		return surface{}, false
	}
	s, err := newSurface(size)
	if err != nil {
		return surface{}, false
	}
	r.ring = append(r.ring, s)
	r.next = len(r.ring)
	r.shown = s.id
	return s, true
}

func (r *surfaces) release() {
	for _, s := range r.ring {
		cfRelease(s.id)
	}
	r.ring = nil
	r.next = 0
	r.shown = 0
}

const pixelRGBA = 0x52474241 // 'RGBA': paint.CPU's premultiplied byte order

// bulkSurface rasterizes the bulk item into a free IOSurface and returns it,
// or 0 when the CGImage path must be used.
func (r *renderer) bulkSurface(it item) uintptr {
	if loadSurface() != nil {
		return 0
	}
	size := it.frame.Size()
	s, ok := r.surfaces.take(size)
	if !ok {
		return 0
	}
	if surfaceLock(s.id, 0, nil) != 0 {
		return 0
	}
	defer surfaceUnlock(s.id, 0, nil)
	stride := int(surfaceStride(s.id))
	base := surfaceBase(s.id)
	if base == nil || stride < 4*size.X {
		return 0
	}
	pix := unsafe.Slice((*byte)(base), stride*(size.Y-1)+4*size.X)
	dst := &image.RGBA{Pix: pix, Stride: stride, Rect: it.frame}
	r.scene.cpu.RenderRegionInto(dst, r.scene.bulkList(it), it.frame, r.scene.scale)
	return s.id
}
