package text

// TrimCaches drops the shaped-text caches of Shape and the run shaping
// (they refill as text is shaped again), for a low-memory warning
// (docs/specs/mobile-canvas.md §3.4). LineCache instances belong to their
// owners and are left alone, as are loaded fonts. Any goroutine.
func TrimCaches() { clearCache() }

// TrimFontCaches drops discovered system font chains and all shaped caches.
// The next layout rebuilds only the families it needs. Existing Layout and
// LineCache owners may retain their Face pointers until they are replaced.
func TrimFontCaches() {
	reg.mu.Lock()
	reg.sysBuilt = false
	reg.system, reg.mono, reg.eng = nil, nil, nil
	reg.version++
	reg.mu.Unlock()
	trimFaceTable()
	clearCache()
}
