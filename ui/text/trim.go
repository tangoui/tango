package text

// TrimCaches drops the shaped-text caches of Shape and the run shaping
// (they refill as text is shaped again), for a low-memory warning
// (docs/specs/mobile-canvas.md §3.4). LineCache instances belong to their
// owners and are left alone, as are loaded fonts. Any goroutine.
func TrimCaches() { clearCache() }
