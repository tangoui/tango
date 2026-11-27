//go:build darwin && !ios

package darwin

import (
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestOverlayStyle(t *testing.T) {
	ex := platform.SurfaceExtras{Kind: platform.KindOverlay}
	if got := windowStyle(true, &ex); got != 128 {
		t.Fatalf("style = %#x, want a borderless nonactivating panel", got)
	}
}

func TestOverlayCollectionBehavior(t *testing.T) {
	if got := overlayCollectionBehavior(); got != 337 {
		t.Fatalf("collection behavior = %#x, want %#x", got, 337)
	}
}

func TestOverlayLevel(t *testing.T) {
	if got := overlayLevel(); got <= nsFloatingWindowLevel || got < 101 {
		t.Fatalf("level = %d, want at least the pop-up menu level", got)
	}
}
