package text

import (
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// RegisterFont must not hold reg.mu while taking mu (via clearCache):
// layout holds mu and takes reg.mu, so the opposite order can deadlock.
func TestRegisterFontWhileShaping(t *testing.T) {
	data, err := os.ReadFile("/System/Library/Fonts/Supplemental/Arial.ttf")
	if err != nil {
		t.Skip("no test font on this machine")
	}
	// Registered fonts go first in every chain: restore the registry so
	// later tests keep the system fonts.
	reg.mu.Lock()
	saved := append([]*family(nil), reg.user...)
	reg.mu.Unlock()
	t.Cleanup(func() {
		reg.mu.Lock()
		reg.user = saved
		reg.version++
		reg.mu.Unlock()
		clearCache()
	})
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			Shape("lock order 锁顺序", Style{Size: float32(12 + i%5)}, Options{})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			if err := RegisterFont("LockOrderTest", data); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("RegisterFont and Shape deadlocked")
	}
}

// A shaper can retain its chain after dropping the registry lock.
func TestRegisterFontPreservesPublishedFamily(t *testing.T) {
	const name = "RegistrationSnapshotTest"
	reg.mu.Lock()
	saved := append([]*family(nil), reg.user...)
	reg.mu.Unlock()
	t.Cleanup(func() {
		reg.mu.Lock()
		reg.user = saved
		reg.version++
		reg.mu.Unlock()
		clearCache()
	})
	find := func() *family {
		for _, fm := range reg.chain(false) {
			if fm.name == name {
				return fm
			}
		}
		t.Fatal("registered family not found")
		return nil
	}
	if err := RegisterFont(name, goregular.TTF); err != nil {
		t.Fatal(err)
	}
	old := find()
	if err := RegisterFont(name, gobold.TTF); err != nil {
		t.Fatal(err)
	}
	if len(old.faces) != 1 {
		t.Fatalf("published family changed: got %d faces, want 1", len(old.faces))
	}
	if got := len(find().faces); got != 2 {
		t.Fatalf("current family has %d faces, want 2", got)
	}
}
