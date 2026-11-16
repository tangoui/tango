package ui

import (
	"runtime"
	"testing"
	"weak"

	"github.com/tangoui/tango/ui/theme"
)

// Returning from this frame ensures the abandoned pointer is no longer on the
// stack when the caller runs GC. The objects contain pointers and are larger
// than the runtime's tiny-allocation batching threshold.
//
//go:noinline
func frameAllocationPeers[T any](alloc func() *T) (weak.Pointer[T], *T) {
	abandoned := alloc()
	retained := alloc()
	return weak.Make(abandoned), retained
}

func checkFrameAllocationPeers[T any](t *testing.T, alloc func() *T) {
	t.Helper()
	abandoned, retained := frameAllocationPeers(alloc)
	runtime.GC()
	runtime.GC()
	stillReachable := abandoned.Value() != nil
	runtime.KeepAlive(retained)
	if stillReachable {
		t.Fatal("keeping one widget alive retained an unrelated abandoned allocation")
	}
}

func TestFrameAllocationsReleaseUnreferencedPeers(t *testing.T) {
	t.Run("node", func(t *testing.T) {
		c := newContext(theme.AntDesign())
		checkFrameAllocationPeers(t, c.allocNode)
	})
	t.Run("label", func(t *testing.T) {
		c := newContext(theme.AntDesign())
		checkFrameAllocationPeers(t, c.allocLabel)
	})
	t.Run("box", func(t *testing.T) {
		c := newContext(theme.AntDesign())
		checkFrameAllocationPeers(t, c.allocBox)
	})
}
