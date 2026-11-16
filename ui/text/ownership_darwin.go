//go:build darwin

package text

import (
	"runtime"
	"sync"
)

// ctFontOwner follows one native font from the lazy build closure into its
// raster. Layouts, line caches and GPU glyph sources keep the Face (and hence
// this owner) alive even after the registry drops a font chain.
//
// Keep the cleanup argument separate: it must not reference this owner, a
// Face, a raster, or any closure that captures them, or cleanup could never run.
// The pointer field also keeps the owner out of the pointer-free tiny allocator.
// Cleanup is GC-driven, not a synchronous native-memory bound for cache trims.
type ctFontOwner struct {
	*ctFontResources
}

type ctFontResources struct {
	mu         sync.Mutex
	releaseRef func(uintptr)
	base       uintptr               // owned Create/Copy reference, size 16, no matrix
	desc       uintptr               // pinned variation descriptor, owned
	unit       uintptr               // owned size-upem copy for outlines
	sized      map[ctSizeKey]uintptr // owned copies per (size, skew)
}

func newCTFontOwner(f uintptr, release func(uintptr)) *ctFontOwner {
	s := &ctFontResources{base: f, releaseRef: release, sized: map[ctSizeKey]uintptr{}}
	o := &ctFontOwner{s}
	runtime.AddCleanup(o, (*ctFontResources).release, s)
	return o
}

// release is also used for a failed lazy build: release its native ownership
// immediately rather than waiting for another GC. The eventual cleanup is a
// no-op, since every owned reference has been cleared.
func (o *ctFontOwner) release() {
	defer runtime.KeepAlive(o)
	o.ctFontResources.release()
}

func (s *ctFontResources) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, f := range s.sized {
		if f != 0 {
			s.releaseRef(f)
		}
		delete(s.sized, k)
	}
	for _, p := range []*uintptr{&s.unit, &s.base, &s.desc} {
		if *p != 0 {
			s.releaseRef(*p)
			*p = 0
		}
	}
}
