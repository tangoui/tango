package uithread

import (
	"sync"
	"testing"
	"time"
)

const timeout = 5 * time.Second

func wait(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestPostRunsInOrderOnOneLockedThread(t *testing.T) {
	th := Start("order")
	defer th.Stop()

	const n = 100
	var (
		got  []int
		tids = map[uint64]bool{}
		ok   = true
	)
	done := make(chan struct{})
	for i := 0; i < n; i++ {
		i := i
		if !th.Post(func() {
			got = append(got, i)
			id, has := osThreadID()
			ok = ok && has
			tids[id] = true
			// Give the scheduler a chance to move an unlocked goroutine.
			time.Sleep(10 * time.Microsecond)
			if i == n-1 {
				close(done)
			}
		}) {
			t.Fatalf("Post %d returned false", i)
		}
	}
	wait(t, done, "posted tasks")

	for i, v := range got {
		if v != i {
			t.Fatalf("task %d ran at position %d: %v", v, i, got)
		}
	}
	if len(got) != n {
		t.Fatalf("ran %d tasks, want %d", len(got), n)
	}
	if !ok {
		t.Skip("OS thread id not available on this platform")
	}
	if len(tids) != 1 {
		t.Fatalf("tasks ran on %d OS threads, want 1: %v", len(tids), tids)
	}
}

func TestPostFromOwnThreadDoesNotReenter(t *testing.T) {
	th := Start("reenter")
	defer th.Stop()

	var order []string
	done := make(chan struct{})
	th.Post(func() {
		th.Post(func() {
			order = append(order, "inner")
			close(done)
		})
		order = append(order, "outer-end")
	})
	wait(t, done, "inner task")
	if len(order) != 2 || order[0] != "outer-end" || order[1] != "inner" {
		t.Fatalf("order = %v, want [outer-end inner]", order)
	}
}

func TestDoWaitsAndRunsInlineOnOwnThread(t *testing.T) {
	th := Start("do")
	defer th.Stop()

	// From another goroutine: Do returns only after fn ran, on the thread.
	var ran, on bool
	if !th.Do(func() {
		time.Sleep(10 * time.Millisecond)
		ran = true
		on = th.On()
	}) {
		t.Fatal("Do returned false")
	}
	if !ran || !on {
		t.Fatalf("after Do: ran=%v on=%v, want both true", ran, on)
	}

	// From the thread itself: Do runs inline (it would deadlock otherwise).
	var order []string
	th.Do(func() {
		order = append(order, "outer-start")
		if !th.Do(func() { order = append(order, "inline") }) {
			order = append(order, "inner Do returned false")
		}
		order = append(order, "outer-end")
	})
	want := []string{"outer-start", "inline", "outer-end"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestStopDrainsThenDropsLaterPosts(t *testing.T) {
	th := Start("stop")

	release := make(chan struct{})
	var ran []int
	th.Post(func() { <-release }) // hold the thread so the next posts queue up
	for i := 0; i < 10; i++ {
		i := i
		if !th.Post(func() { ran = append(ran, i) }) {
			t.Fatalf("Post %d before Stop returned false", i)
		}
	}
	th.Stop()
	th.Stop() // repeated Stop has no effect

	if th.Post(func() { t.Error("task posted after Stop ran") }) {
		t.Fatal("Post after Stop returned true")
	}
	if th.Do(func() { t.Error("Do after Stop ran") }) {
		t.Fatal("Do after Stop returned true")
	}

	select {
	case <-th.Done():
		t.Fatal("Done closed before queued tasks drained")
	default:
	}
	close(release)
	wait(t, th.Done(), "Done")

	if len(ran) != 10 {
		t.Fatalf("drained %d tasks, want 10: %v", len(ran), ran)
	}
	for i, v := range ran {
		if v != i {
			t.Fatalf("drain order = %v", ran)
		}
	}
}

func TestOnReportsOwnThread(t *testing.T) {
	a, b := Start("a"), Start("b")
	defer a.Stop()
	defer b.Stop()

	if a.On() || b.On() {
		t.Fatal("On() true on the test goroutine")
	}
	var onA, onB bool
	a.Do(func() { onA, onB = a.On(), b.On() })
	if !onA || onB {
		t.Fatalf("on a's thread: a.On()=%v b.On()=%v, want true false", onA, onB)
	}

	// A goroutine started from the thread is not the thread.
	var fromChild bool
	a.Do(func() {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); fromChild = a.On() }()
		wg.Wait()
	})
	if fromChild {
		t.Fatal("On() true on a goroutine spawned by the thread")
	}
}

func TestAwaitingMainFlag(t *testing.T) {
	th := Start("await")
	defer th.Stop()

	if th.AwaitingMain() {
		t.Fatal("AwaitingMain initially true")
	}
	th.Do(func() { th.SetAwaitingMain(true) })
	if !th.AwaitingMain() {
		t.Fatal("AwaitingMain false after SetAwaitingMain(true)")
	}
	th.SetAwaitingMain(false)
	if th.AwaitingMain() {
		t.Fatal("AwaitingMain true after SetAwaitingMain(false)")
	}
}

func TestCurrentFindsOwnThread(t *testing.T) {
	th := Start("current")

	if c := Current(); c != nil {
		t.Fatalf("Current() on the test goroutine = %v, want nil", c)
	}
	var got *Thread
	th.Do(func() { got = Current() })
	if got != th {
		t.Fatalf("Current() inside a task = %p, want %p", got, th)
	}

	th.Stop()
	wait(t, th.Done(), "Done")
	ch := make(chan *Thread)
	go func() { ch <- Current() }()
	if c := <-ch; c != nil {
		t.Fatalf("Current() after exit = %v, want nil", c)
	}
}

func TestStartWithRecoversPanics(t *testing.T) {
	got := make(chan any, 1)
	th := StartWith("panic", func(v any) { got <- v })
	defer th.Stop()

	th.Post(func() { panic("boom") })
	var after bool
	th.Do(func() { after = true })
	select {
	case v := <-got:
		if v != "boom" {
			t.Fatalf("onPanic got %v, want boom", v)
		}
	default:
		t.Fatal("onPanic not called")
	}
	if !after {
		t.Fatal("thread did not keep running after a recovered panic")
	}
}

func TestGoroutineID(t *testing.T) {
	a := GoroutineID()
	if a == 0 {
		t.Fatal("GoroutineID() = 0")
	}
	if b := GoroutineID(); b != a {
		t.Fatalf("GoroutineID() changed on the same goroutine: %d then %d", a, b)
	}
	ch := make(chan uint64)
	go func() { ch <- GoroutineID() }()
	if c := <-ch; c == 0 || c == a {
		t.Fatalf("other goroutine id = %d, test goroutine id = %d", c, a)
	}
}
