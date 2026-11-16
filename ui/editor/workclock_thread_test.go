//go:build darwin || linux

package editor

import (
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// threadCPUClock: the calling OS thread's CPU time, which a frame's work
// accrues only while the thread runs. The caller holds
// runtime.LockOSThread so the goroutine stays on that thread.
const threadCPUClock = true

func workClock() time.Duration {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_THREAD_CPUTIME_ID, &ts); err != nil {
		panic(err)
	}
	return time.Duration(ts.Nano())
}

// TestWorkClockCountsOnlyRunning: the thread clock leaves out the time the
// thread is off the CPU (here asleep, as when the OS runs other threads)
// and counts the time it computes.
func TestWorkClockCountsOnlyRunning(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	c0 := workClock()
	time.Sleep(50 * time.Millisecond)
	if d := workClock() - c0; d > 10*time.Millisecond {
		t.Errorf("asleep 50ms: thread clock advanced %v", d)
	}
	c0 = workClock()
	for end := time.Now().Add(5 * time.Second); workClock()-c0 < 5*time.Millisecond; {
		if time.Now().After(end) {
			t.Fatal("computing for 5s: thread clock did not reach 5ms")
		}
	}
}
