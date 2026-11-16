//go:build darwin && !ios

package darwin

import (
	"errors"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestBackendProfileSelection(t *testing.T) {
	if profileWebViewEnabled != profileExpectedWebView || profileCanvasEnabled != profileExpectedCanvas {
		t.Fatalf("profile webview=%v canvas=%v; want webview=%v canvas=%v", profileWebViewEnabled, profileCanvasEnabled, profileExpectedWebView, profileExpectedCanvas)
	}
}

func TestExcludedProfileOperationsReturnUnsupported(t *testing.T) {
	b := New()
	if !profileWebViewEnabled {
		if _, err := b.NewSurface(1, platform.SurfaceOptions{}); !errors.Is(err, platform.ErrUnsupported) {
			t.Fatalf("webview-only operation error = %v, want ErrUnsupported", err)
		}
	}
	if !profileCanvasEnabled {
		cc, ok := b.(platform.CanvasCreator)
		if !ok {
			t.Fatal("Darwin backend must retain the CanvasCreator boundary")
		}
		if _, _, err := cc.NewCanvasSurface(1, platform.SurfaceOptions{}, platform.SurfaceExtras{}, nil); !errors.Is(err, platform.ErrUnsupported) {
			t.Fatalf("canvas-only operation error = %v, want ErrUnsupported", err)
		}
	}
}
