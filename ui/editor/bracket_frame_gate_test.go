package editor

import (
	"testing"
	"time"
)

// bracketFrameTimingLimits returns the immediate-failure and counted-overrun
// thresholds. A zero hard threshold leaves the wall-time median gate enabled.
func bracketFrameTimingLimits(goos string, threadCPU bool, limit time.Duration) (hard, soft time.Duration) {
	if threadCPU || goos == "windows" {
		return 3 * limit, limit
	}
	return 0, 3 * limit
}

// A single Windows frame must not disappear behind the median of many
// fast frames. These inputs also pin the original strict > boundaries;
// non-thread-clock fallback platforms keep their existing far-only count.
func TestBracketFrameTimingOverruns(t *testing.T) {
	for _, tc := range []struct {
		name, goos string
		threadCPU  bool
		work       time.Duration
		wantHard   bool
		wantOver   bool
	}{
		{"windows below", "windows", false, time.Millisecond, false, false},
		{"windows at soft", "windows", false, 2750 * time.Microsecond, false, false},
		{"windows above soft", "windows", false, 2750*time.Microsecond + 1, false, true},
		{"windows at hard", "windows", false, 8250 * time.Microsecond, false, true},
		{"windows single spike", "windows", false, 8250*time.Microsecond + 1, true, true},
		{"darwin CPU", "darwin", true, 8250*time.Microsecond + 1, true, true},
		{"linux CPU", "linux", true, 2750*time.Microsecond + 1, false, true},
		{"fallback modest", "freebsd", false, 3 * time.Millisecond, false, false},
		{"fallback far", "freebsd", false, 9 * time.Millisecond, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hard, soft := bracketFrameTimingLimits(tc.goos, tc.threadCPU, 2750*time.Microsecond)
			if got := hard > 0 && tc.work > hard; got != tc.wantHard {
				t.Errorf("hard overrun of %v: got %v, want %v", tc.work, got, tc.wantHard)
			}
			if got := tc.work > soft; got != tc.wantOver {
				t.Errorf("count overrun of %v: got %v, want %v", tc.work, got, tc.wantOver)
			}
		})
	}
}
