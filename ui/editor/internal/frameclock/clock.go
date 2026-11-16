// Package frameclock supplies one monotonic clock for editor frame deadlines.
// Windows uses QPC: Go's shared interrupt-time clock can remain unchanged for
// longer than the editor's sub-millisecond grace budget. Other systems retain
// time.Now's monotonic clock. These are elapsed wall times, not CPU times.
package frameclock

import (
	"math"
	"math/bits"
	"time"
)

// counterDuration converts an elapsed QPC tick count, not processor cycles.
// Split seconds and use a wide multiply for the fraction: neither long uptime
// nor an unusually high performance-counter frequency may wrap the clock.
func counterDuration(ticks, frequency int64) time.Duration {
	if ticks < 0 || frequency <= 0 {
		panic("frameclock: invalid performance counter")
	}
	seconds, remainder := ticks/frequency, ticks%frequency
	if seconds > math.MaxInt64/int64(time.Second) {
		return time.Duration(math.MaxInt64)
	}
	hi, lo := bits.Mul64(uint64(remainder), uint64(time.Second))
	// remainder < frequency, so the quotient is below one second and
	// the high word is necessarily below frequency: Div64 cannot overflow.
	fraction, _ := bits.Div64(hi, lo, uint64(frequency))
	nanos := seconds * int64(time.Second)
	if fraction > uint64(math.MaxInt64-nanos) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(nanos + int64(fraction))
}

// Until is paired with Now; time.Until would use Windows' coarser clock.
func Until(deadline time.Time) time.Duration { return deadline.Sub(Now()) }

// Since is paired with Now for the same reason.
func Since(start time.Time) time.Duration { return Now().Sub(start) }
