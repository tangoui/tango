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
