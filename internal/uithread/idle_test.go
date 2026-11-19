package uithread

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdlePumpWakesForPostAndStop(t *testing.T) {
	th := Start("idle pump")
	wake := make(chan struct{}, 1)
	waiting := make(chan struct{}, 1)
	th.Do(func() {
		th.SetIdlePump(func() {
			select {
			case waiting <- struct{}{}:
			default:
			}
			<-wake
		}, func() {
			select {
			case wake <- struct{}{}:
			default:
			}
		})
	})
	until := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("idle pump did not wake")
		}
	}
	until(waiting)
	ran := make(chan struct{})
	th.Post(func() { close(ran) })
	until(ran)
	until(waiting)
	th.Stop()
	until(th.Done())
}

func TestIdlePumpDetachesConcurrentWakers(t *testing.T) {
	th := Start("detach pump")
	defer th.Stop()
	var released atomic.Bool
	wake := make(chan struct{}, 1)
	th.Do(func() {
		th.SetIdlePump(func() { <-wake }, func() {
			if released.Load() {
				t.Error("old wake ran after pump release")
			}
			select {
			case wake <- struct{}{}:
			default:
			}
		})
	})
	var posters sync.WaitGroup
	for range 8 {
		posters.Go(func() {
			for range 100 {
				th.Post(func() {})
			}
		})
	}
	th.Do(func() { th.SetIdlePump(nil, nil); released.Store(true) })
	posters.Wait()
	th.Do(func() {})
}

// A Post made while the thread runs a task (not inside its pump) needs no
// native wake: the loop looks at the queue before it pumps again. A frame
// that asks for the next frame then costs no extra run-loop turn.
func TestPostOutsidePumpSkipsWake(t *testing.T) {
	th := Start("no wake")
	defer th.Stop()
	var wakes atomic.Int32
	wake := make(chan struct{}, 1)
	th.Do(func() {
		th.SetIdlePump(func() { <-wake }, func() {
			wakes.Add(1)
			select {
			case wake <- struct{}{}:
			default:
			}
		})
	})
	ran := make(chan struct{})
	th.Do(func() {
		before := wakes.Load()
		th.Post(func() { close(ran) }) // from a task: not pumping
		if n := wakes.Load() - before; n != 0 {
			t.Errorf("Post from a running task woke the pump %d times", n)
		}
	})
	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("task posted from a task never ran")
	}
	// While the thread waits in its pump, a Post must still wake it.
	deadline := time.Now().Add(time.Second)
	for {
		before := wakes.Load()
		done := make(chan struct{})
		th.Post(func() { close(done) })
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("Post while pumping did not run")
		}
		if wakes.Load() > before {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Post while pumping never woke the pump")
		}
		time.Sleep(10 * time.Millisecond) // let the thread re-enter the pump
	}
}
