//go:build darwin && !ios

package darwin

import (
	"errors"
	"strings"
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
		if _, err := b.NewSurface(1, platform.SurfaceOptions{}); !errors.Is(err, platform.ErrUnsupported) || !strings.Contains(err.Error(), "canvas-only") {
			t.Fatalf("NewSurface error = %v, want ErrUnsupported naming the canvas-only profile", err)
		}
		if _, err := b.(platform.CreatorEx).NewSurfaceEx(1, platform.SurfaceOptions{}, platform.SurfaceExtras{}); !errors.Is(err, platform.ErrUnsupported) {
			t.Fatalf("NewSurfaceEx error = %v, want ErrUnsupported", err)
		}
		if _, err := b.(platform.WebCreator).NewSurfaceWeb(1, platform.SurfaceOptions{}, platform.SurfaceExtras{}, platform.WebOptions{}); !errors.Is(err, platform.ErrUnsupported) {
			t.Fatalf("NewSurfaceWeb error = %v, want ErrUnsupported", err)
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
