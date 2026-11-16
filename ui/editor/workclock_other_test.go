//go:build !darwin && !linux

package editor

import "time"

// threadCPUClock is false where workClock has no thread clock (only darwin
// and linux have one wired; Windows' thread times tick at the scheduler's
// 15.6 ms): workClock is then the monotonic wall clock.
const threadCPUClock = false

var workClockStart = time.Now()

func workClock() time.Duration { return time.Since(workClockStart) }
