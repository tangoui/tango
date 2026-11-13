//go:build darwin && !ios

package darwin

import (
	"sync/atomic"
	"testing"
)

// Posting NSCurrentLocaleDidChangeNotification in this process (the
// default center delivers synchronously, no run loop needed) reaches the
// watchers; nothing is changed in the system settings.
func TestWatchLocalesNotification(t *testing.T) {
	var n atomic.Int32
	cancel := watchLocales(func() { n.Add(1) })
	post := func() {
		withPool(func() {
			name := constPtr(libFoundation, "NSCurrentLocaleDidChangeNotification")
			if name == 0 {
				t.Fatal("NSCurrentLocaleDidChangeNotification not found")
			}
			send(send(class("NSNotificationCenter"), "defaultCenter"),
				"postNotificationName:object:", name, 0)
		})
	}
	post()
	if n.Load() != 1 {
		t.Fatalf("watcher called %d times, want 1", n.Load())
	}
	cancel()
	post()
	if n.Load() != 1 {
		t.Fatalf("cancelled watcher called again (%d)", n.Load())
	}
}
