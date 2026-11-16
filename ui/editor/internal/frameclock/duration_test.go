package frameclock

import (
	"math"
	"testing"
	"time"
)

func TestCounterDuration(t *testing.T) {
	for _, tc := range []struct {
		name             string
		ticks, frequency int64
		want             time.Duration
	}{
		{"zero", 0, 10_000_000, 0},
		{"one tick", 1, 10_000_000, 100 * time.Nanosecond},
		{"frame budget", 15_000, 10_000_000, 1500 * time.Microsecond},
		{"third", 1, 3, 333333333 * time.Nanosecond},
		{"long uptime", 1_000_000_000_000, 10_000_000, 100000 * time.Second},
		{"large frequency", math.MaxInt64 - 1, math.MaxInt64, time.Second - time.Nanosecond},
		{"largest duration", math.MaxInt64, 1_000_000_000, time.Duration(math.MaxInt64)},
		{"saturate seconds", math.MaxInt64, 1, time.Duration(math.MaxInt64)},
		{"saturate fraction", 922337203699, 100, time.Duration(math.MaxInt64)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := counterDuration(tc.ticks, tc.frequency); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCounterDurationRejectsInvalidClock(t *testing.T) {
	for _, input := range [][2]int64{{-1, 10}, {1, 0}, {1, -1}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("invalid counter %v silently accepted", input)
				}
			}()
			counterDuration(input[0], input[1])
		}()
	}
}
