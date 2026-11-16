//go:build !darwin && !linux && !windows

package editor

import "time"

// Only darwin and linux have a thread CPU clock wired here. On Windows,
// GetThreadTimes' 100 ns units do not guarantee sub-millisecond resolution,
// and QueryThreadCycleTime explicitly cannot be converted to elapsed time.
// Use the monotonic wall clock rather than claim precise CPU timing.
const threadCPUClock = false

var workClockStart = time.Now()

func workClock() time.Duration { return time.Since(workClockStart) }
