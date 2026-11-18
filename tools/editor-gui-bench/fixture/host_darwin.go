package main

import "golang.org/x/sys/unix"

// hostNS is uptime nanoseconds (CLOCK_UPTIME_RAW): the clock of gui-probe's
// input and frame times and of the Metal present trace.
func hostNS() int64 {
	var ts unix.Timespec
	if unix.ClockGettime(unix.CLOCK_UPTIME_RAW, &ts) != nil {
		return 0
	}
	return ts.Nano()
}
