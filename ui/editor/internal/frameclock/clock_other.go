//go:build !windows

package frameclock

import "time"

// Now retains the platform's standard monotonic clock.
func Now() time.Time { return time.Now() }
