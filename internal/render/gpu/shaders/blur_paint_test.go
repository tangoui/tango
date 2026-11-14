package shaders

import (
	"testing"

	"github.com/tangoui/tango/ui/paint"
)

// TestBlurLoopBounds: the blur passes' constant loop bounds cover every
// plan paint.PlanBackdropBlur makes.
func TestBlurLoopBounds(t *testing.T) {
	if blurMaxDown != paint.MaxBlurDown || blurMaxTaps != paint.MaxBlurTaps {
		t.Fatalf("shader loop bounds %d, %d; paint plans up to %d, %d", blurMaxDown, blurMaxTaps, paint.MaxBlurDown, paint.MaxBlurTaps)
	}
}
