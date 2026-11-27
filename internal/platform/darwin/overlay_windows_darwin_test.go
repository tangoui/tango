//go:build darwin && !ios

package darwin

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func init() {
	if os.Getenv("TANGO_OVERLAY_WINDOW_TEST_CHILD") != "1" {
		return
	}
	runtime.LockOSThread()
	if err := checkOverlayWindows(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestNativeOverlayWindowBehavior(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "TANGO_OVERLAY_WINDOW_TEST_CHILD=1", "TANGO_LAUNCH_BACKGROUND=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native overlay check: %v\n%s", err, out)
	}
}

func checkOverlayWindows() error {
	b := New().(*backend)
	if err := b.Init(&shortcutTestHost{}, platform.AppOptions{}); err != nil {
		return err
	}
	send(b.app, "setActivationPolicy:", 2) // Prohibited
	send(b.app, "finishLaunching")
	opts := platform.SurfaceOptions{Width: 100, Height: 80}
	parent, _, err := b.NewCanvasSurface(1, opts, platform.SurfaceExtras{Kind: platform.KindOverlay}, &shortcutTestSink{})
	if err != nil {
		return err
	}
	defer parent.Close()
	parent.Show()
	var share id
	withPool(func() { share = b.shareWindow() })
	if share != 0 {
		return fmt.Errorf("share picker selected the overlay")
	}
	embed, err := b.NewEmbeddedWebView(2, 1, opts, platform.WebOptions{})
	if err != nil {
		return err
	}
	defer embed.Close()
	if !sendBool(id(embed.Native()), "acceptsFirstMouse:", 0) {
		return fmt.Errorf("WebView on an overlay rejected the first click")
	}
	ordinary, err := b.NewSurface(3, opts)
	if err != nil {
		return err
	}
	defer ordinary.Close()
	w := ordinary.(*surface).win
	send(w, "orderFrontRegardless")
	withPool(func() { share = b.shareWindow() })
	if share != w {
		return fmt.Errorf("share picker did not select the ordinary window")
	}
	return nil
}
