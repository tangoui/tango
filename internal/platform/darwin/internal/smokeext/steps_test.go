//go:build darwin && !ios

package main

import (
	"fmt"
	"testing"
	"time"
)

func TestStepLogKeepsReloadAfterGeometryBurst(t *testing.T) {
	steps := newStepLog()
	for i := range 80 {
		emit(steps, fmt.Sprintf("resized:%d,1", i))
	}
	emit(steps, "nav:tango://app/")
	emit(steps, "loaded")
	emit(steps, "msg:echo:after-reload")
	for _, want := range []string{"nav:tango://app/", "loaded", "msg:echo:after-reload"} {
		got, ok := waitStep(steps, want, time.Millisecond)
		if !ok || got != want {
			t.Fatalf("%s: got %q %v", want, got, ok)
		}
	}
}

func TestStepLogKeepsEarlierFutureSteps(t *testing.T) {
	steps := newStepLog()
	emit(steps, "loaded")
	emit(steps, "nav:tango://app/")
	if got, ok := waitStep(steps, "nav:", time.Millisecond); !ok || got != "nav:tango://app/" {
		t.Fatalf("nav: got %q %v", got, ok)
	}
	if got, ok := waitStep(steps, "loaded", time.Millisecond); !ok || got != "loaded" {
		t.Fatalf("loaded: got %q %v", got, ok)
	}
}
