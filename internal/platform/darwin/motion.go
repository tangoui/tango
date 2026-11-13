//go:build darwin && !ios

package darwin

// platform.MotionPreference: NSWorkspace.accessibilityDisplayShouldReduceMotion
// (macOS 10.12+), followed through
// NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification on the
// workspace's notification center. Changes are reported as
// AppReduceMotionChanged (docs/specs/a11y-bridge.md). The same observer
// serves platform.ContrastPreference (contrast.go).

import (
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.MotionPreference = (*backend)(nil)

var motionState struct {
	once     sync.Once
	on       atomic.Bool
	observer id
}

// readReduceMotion asks NSWorkspace. Main thread.
func readReduceMotion() bool {
	var on bool
	withPool(func() {
		ws := send(class("NSWorkspace"), "sharedWorkspace")
		if respondsTo(ws, "accessibilityDisplayShouldReduceMotion") {
			on = sendBool(ws, "accessibilityDisplayShouldReduceMotion")
		}
	})
	return on
}

// watchDisplayOptions starts observing the accessibility display options
// once: reduce motion here, increase contrast for contrast.go. Main thread.
func (b *backend) watchDisplayOptions() {
	motionState.once.Do(func() {
		motionState.on.Store(readReduceMotion())
		contrastState.on.Store(readIncreaseContrast())
		classDef("TangoMotionObserver", "NSObject", nil, []objc.MethodDef{
			method("displayOptionsChanged:", func(self id, _ objc.SEL, note id) {
				// Posted on the main thread; runOnMain keeps that true if a
				// future macOS posts it elsewhere.
				runOnMain(func() {
					b.motionMaybeChanged()
					b.contrastMaybeChanged()
				})
			}),
		})
		withPool(func() {
			motionState.observer = alloc("TangoMotionObserver")
			nc := send(send(class("NSWorkspace"), "sharedWorkspace"), "notificationCenter")
			send(nc, "addObserver:selector:name:object:", uintptr(motionState.observer),
				uintptr(sel("displayOptionsChanged:")),
				uintptr(appKitString("NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification")), 0)
		})
	})
}

func (b *backend) motionMaybeChanged() {
	on := readReduceMotion()
	if motionState.on.Swap(on) == on || b == nil || b.h == nil {
		return
	}
	b.appEvent(platform.AppEvent{Kind: platform.AppReduceMotionChanged})
}

// ReduceMotion implements platform.MotionPreference. Any goroutine: off
// the main thread it returns the value last seen there (and starts
// watching if nobody asked on the main thread yet).
func (b *backend) ReduceMotion() bool {
	if isMainThread() {
		b.watchDisplayOptions()
		return motionState.on.Load()
	}
	runOnMain(b.watchDisplayOptions)
	return motionState.on.Load()
}

// appKitString returns the AppKit NSString constant name, or an NSString
// with the same text when the symbol is missing (the constants' values are
// their names).
func appKitString(name string) id {
	if v := constPtr(libAppKit, name); v != 0 {
		return id(v)
	}
	return nsString(name)
}
