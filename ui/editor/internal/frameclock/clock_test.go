package frameclock

import (
	"testing"
	"time"
)

func TestClockMonotonicAndDeadline(t *testing.T) {
	previous := Now()
	for range 1000 {
		now := Now()
		if now.Before(previous) {
			t.Fatal("frame clock moved backwards")
		}
		previous = now
	}
	// A deadline constructed and checked with this clock shares one domain;
	// an expired budget cannot become available again between consumers.
	deadline := Now().Add(-time.Second)
	if Until(deadline) >= 0 {
		t.Fatal("expired deadline became available")
	}
	if Since(previous) < 0 {
		t.Fatal("negative elapsed duration")
	}
	// Callers outside this package still construct deadlines with time.Now;
	// Windows must compare those without mixing monotonic clock domains.
	if !Now().After(time.Now().Add(-time.Second)) {
		t.Fatal("frame clock is behind a recently expired time.Now deadline")
	}
}
