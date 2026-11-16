//go:build darwin

package macoslite

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

type releaseLog struct {
	mu sync.Mutex
	n  map[uintptr]int
}

func (l *releaseLog) release(h uintptr) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.n == nil {
		l.n = map[uintptr]int{}
	}
	l.n[h]++
}

func (l *releaseLog) count(h uintptr) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.n[h]
}

func waitCount(t *testing.T, l *releaseLog, h uintptr, want int) {
	t.Helper()
	for i := 0; i < 50; i++ {
		runtime.GC()
		if l.count(h) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("handle %#x released %d times, want %d", h, l.count(h), want)
}

func TestNativeOwnerExplicitReleaseOnce(t *testing.T) {
	var log releaseLog
	o := newNativeOwner(0x10, log.release)
	if o.Handle() != 0x10 {
		t.Fatalf("Handle = %#x", o.Handle())
	}
	o.Release()
	o.Release()
	if o.Handle() != 0 {
		t.Fatal("Handle must be zero after Release")
	}
	o = nil
	waitCount(t, &log, 0x10, 1)
}

func TestNativeOwnerCleanupWithoutRelease(t *testing.T) {
	var log releaseLog
	func() { _ = newNativeOwner(0x20, log.release) }()
	waitCount(t, &log, 0x20, 1)
}

func TestNativeOwnerZeroHandle(t *testing.T) {
	var log releaseLog
	o := newNativeOwner(0, log.release)
	o.Release()
	if log.count(0) != 0 {
		t.Fatal("zero handle must never be released")
	}
}

func TestNativeOwnerSameAddressTwoRetains(t *testing.T) {
	var log releaseLog
	a := newNativeOwner(0x30, log.release)
	func() { _ = newNativeOwner(0x30, log.release) }()
	a.Release()
	waitCount(t, &log, 0x30, 2)
}

// The cleanup must not keep the owner reachable, or it could never run even
// when the owner was captured by the release callback's surroundings.
func TestNativeOwnerCleanupDoesNotRetainOwner(t *testing.T) {
	var log releaseLog
	collected := make(chan struct{})
	func() {
		o := newNativeOwner(0x40, log.release)
		runtime.AddCleanup(o, func(ch chan struct{}) { close(ch) }, collected)
	}()
	for i := 0; i < 50; i++ {
		runtime.GC()
		select {
		case <-collected:
			waitCount(t, &log, 0x40, 1)
			return
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	t.Fatal("owner was never collected")
}

func TestNativeLoad(t *testing.T) {
	if err := loadNative(); err != nil {
		t.Fatalf("loadNative: %v", err)
	}
	if objcMsgSend == 0 || cfRelease == nil || ctFontCreateWithName == nil || cgBitmapContextCreate == nil {
		t.Fatal("required symbols not bound")
	}
}
