package editor

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/internal/frameclock"
)

// Windows has a precise elapsed WALL clock, not a thread CPU clock. Waiting
// remains part of the per-frame measurement and must not be subtracted.
const threadCPUClock = false

var workClockOrigin = frameclock.Now()

func workClock() time.Duration { return frameclock.Since(workClockOrigin) }

func TestWindowsWorkClockIncludesWaiting(t *testing.T) {
	start := workClock()
	time.Sleep(20 * time.Millisecond)
	if elapsed := workClock() - start; elapsed < 10*time.Millisecond {
		t.Fatalf("elapsed wall clock omitted waiting: %v", elapsed)
	}
}
