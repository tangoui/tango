//go:build darwin

package macoslite

import (
	"runtime"
	"sync"
)

// nativeOwner holds one owned (+1) Objective-C or CoreFoundation reference
// and releases it exactly once: on Release, or by a GC cleanup if the owner
// becomes unreachable first.
//
// The cleanup argument is the separate nativeRef, which must never point back
// at the owner, a view, an App or a closure capturing them; otherwise the
// cleanup could never run.
type nativeOwner struct {
	ref *nativeRef
}

type nativeRef struct {
	mu      sync.Mutex
	h       uintptr
	release func(uintptr)
}

func (r *nativeRef) drop() {
	r.mu.Lock()
	h := r.h
	r.h = 0
	r.mu.Unlock()
	if h != 0 {
		r.release(h)
	}
}

// newNativeOwner takes ownership of h; release is called once with h.
func newNativeOwner(h uintptr, release func(uintptr)) *nativeOwner {
	r := &nativeRef{h: h, release: release}
	o := &nativeOwner{ref: r}
	runtime.AddCleanup(o, (*nativeRef).drop, r)
	return o
}

// Handle returns the owned reference, or 0 after Release. Callers passing it
// to native code must keep the owner alive across the call (KeepAlive).
func (o *nativeOwner) Handle() uintptr {
	o.ref.mu.Lock()
	defer o.ref.mu.Unlock()
	return o.ref.h
}

// Release drops the reference now. Later calls do nothing.
func (o *nativeOwner) Release() {
	o.ref.drop()
	runtime.KeepAlive(o)
}

// KeepAlive marks o live up to this point, for use after native calls that
// received Handle.
func (o *nativeOwner) KeepAlive() { runtime.KeepAlive(o) }
