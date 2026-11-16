//go:build darwin && !ios

package darwin

import (
	"fmt"
	"github.com/tangoui/tango/internal/platform"
)

func profileWebViewError() error {
	return fmt.Errorf("tango/darwin: WebView is unavailable in the canvas-only backend profile: %w", platform.ErrUnsupported)
}
func profileCanvasError() error {
	return fmt.Errorf("tango/darwin: canvas is unavailable in the webview-only backend profile: %w", platform.ErrUnsupported)
}
