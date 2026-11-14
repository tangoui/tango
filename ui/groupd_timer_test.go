package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
)

func TestStatisticTimerCountup(t *testing.T) {
	var start time.Time
	finished := 0
	var changes []time.Duration
	d := NewDriver(func(c *Context) {
		if start.IsZero() {
			start = c.Now()
		}
		w := StatisticTimer(c, "Elapsed", TimerCountup, start.Add(-65*time.Second)).Format("mm:ss").
			OnChange(func(d time.Duration) { changes = append(changes, d) })
		if w.Finished() {
			finished++
		}
	}, DriverOptions{Width: 300, Height: 200})
	d.EnableAccessibility()
	d.Frame()
	n, ok := d.A11yFind(a11y.RoleTimer, "Elapsed")
	if !ok || n.Value != "01:05" {
		t.Fatalf("count up value %+v", n)
	}
	if len(changes) != 0 {
		t.Fatalf("OnChange ran on the first frame: %v", changes)
	}
	if w := d.Runner().NextWake(); w.IsZero() || w.Sub(d.Now()) > time.Second {
		t.Fatalf("count up should wake within a second, got %v", w)
	}
	d.Advance(2 * time.Second)
	d.Frame()
	d.Frame()
	n, _ = d.A11yFind(a11y.RoleTimer, "Elapsed")
	if n.Value != "01:07" {
		t.Fatalf("after 2s: %q", n.Value)
	}
	if len(changes) != 1 || changes[0] != 67*time.Second {
		t.Fatalf("OnChange calls %v", changes)
	}
	d.Advance(time.Hour)
	d.Frame()
	if finished != 0 {
		t.Fatal("a count up never finishes")
	}
}

func TestStatisticTimerCountdownOnChange(t *testing.T) {
	var start time.Time
	var changes []time.Duration
	finished := 0
	d := NewDriver(func(c *Context) {
		if start.IsZero() {
			start = c.Now()
		}
		w := StatisticTimer(c, "Left", TimerCountdown, start.Add(3*time.Second)).
			OnChange(func(d time.Duration) { changes = append(changes, d) })
		if w.Finished() {
			finished++
		}
	}, DriverOptions{Width: 300, Height: 200})
	d.Frame()
	for i := 0; i < 4; i++ {
		d.Advance(time.Second)
		d.Frame()
	}
	if len(changes) != 3 || changes[0] != 2*time.Second || changes[2] != 0 {
		t.Fatalf("countdown OnChange %v", changes)
	}
	if finished != 1 {
		t.Fatalf("finished %d", finished)
	}
	// Countdown's own OnChange.
	calls := 0
	var s2 time.Time
	d2 := NewDriver(func(c *Context) {
		if s2.IsZero() {
			s2 = c.Now()
		}
		Countdown(c, "cd", s2.Add(time.Minute+900*time.Millisecond)).OnChange(func(time.Duration) { calls++ })
	}, DriverOptions{})
	d2.Frame()
	d2.Advance(500 * time.Millisecond)
	d2.Frame()
	if calls != 0 {
		t.Fatal("value did not change within the second")
	}
	d2.Advance(600 * time.Millisecond)
	d2.Frame()
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
}
