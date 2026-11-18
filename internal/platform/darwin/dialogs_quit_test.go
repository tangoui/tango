//go:build darwin && !ios

package darwin

import (
	"runtime"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestDialogsAfterQuitDoNotEnterAppKit(t *testing.T) {
	// Trap before any native setup. The late start may be dispatched by the
	// same main-queue drain that set quitting; no native app is needed here.
	origPush := arPoolPush
	arPoolPush = func() uintptr {
		runtime.UnlockOSThread() // withPool has not installed its defer yet
		panic("attempted native dialog setup after quit")
	}
	t.Cleanup(func() { arPoolPush = origPush })
	b := &backend{quitting: true}
	check := func(t *testing.T, start func(func(bool, error))) {
		t.Helper()
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("%v", p)
			}
		}()
		calls := 0
		start(func(empty bool, err error) {
			calls++
			if !empty || err != platform.ErrCanceled {
				t.Errorf("empty=%v error=%v, want empty result and ErrCanceled", empty, err)
			}
		})
		if calls != 1 {
			t.Errorf("completion called %d times, want 1", calls)
		}
	}
	t.Run("message", func(t *testing.T) {
		check(t, func(done func(bool, error)) {
			b.MessageDialog(0, platform.MessageOptions{}, func(button int, err error) { done(button == -1, err) })
		})
	})
	t.Run("open", func(t *testing.T) {
		check(t, func(done func(bool, error)) {
			b.OpenFileDialog(0, platform.FileOptions{}, func(paths []string, err error) { done(paths == nil, err) })
		})
	})
	t.Run("save", func(t *testing.T) {
		check(t, func(done func(bool, error)) {
			b.SaveFileDialog(0, platform.FileOptions{}, func(path string, err error) { done(path == "", err) })
		})
	})
}
