//go:build darwin && !ios

package darwin

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

// AppKit must run on the process's original thread, not testing's worker.
// Re-exec before the testing runner starts, as the native smoke commands do.
func init() {
	if os.Getenv("TANGO_NATIVE_SHORTCUT_TEST") != "1" {
		return
	}
	runtime.LockOSThread()
	if err := nativeShortcutCaptureTest(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PASS native shortcut capture: application/window/view routing, restore, other responder")
	os.Exit(0)
}

func TestNativeShortcutCapture(t *testing.T) {
	if os.Getenv("TANGO_TEST_NATIVE_SHORTCUT") != "1" || testing.Short() {
		t.Skip("opens a native window; set TANGO_TEST_NATIVE_SHORTCUT=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNativeShortcutCapture$")
	cmd.Env = append(os.Environ(), "TANGO_NATIVE_SHORTCUT_TEST=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("native UI-thread subprocess: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "PASS native shortcut capture") {
		t.Fatalf("missing native result: %s", out)
	}
	t.Log(strings.TrimSpace(string(out)))
}

type shortcutTestHost struct{}

func (*shortcutTestHost) Drain()                                                   {}
func (*shortcutTestHost) ShouldClose(platform.SurfaceID) bool                      { return true }
func (*shortcutTestHost) OnLifecycle(platform.LifecycleEvent)                      {}
func (*shortcutTestHost) OnSurfaceEvent(platform.SurfaceID, platform.SurfaceEvent) {}
func (*shortcutTestHost) OnMessage(platform.SurfaceID, string, string)             {}
func (*shortcutTestHost) ServeAsset(_ platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

type shortcutTestSink struct {
	keys  []input.KeyEvent
	onKey func()
}

func (*shortcutTestSink) CanvasFrame()                            {}
func (*shortcutTestSink) CanvasResized(float32, float32, float32) {}
func (*shortcutTestSink) CanvasFocus(bool)                        {}
func (s *shortcutTestSink) CanvasInput(ev input.Event) {
	if k, ok := ev.(input.KeyEvent); ok {
		s.keys = append(s.keys, k)
		if k.Down && s.onKey != nil {
			s.onKey()
		}
	}
}

func nativeShortcutCaptureTest() error {
	b := New().(*backend)
	if err := b.Init(&shortcutTestHost{}, platform.AppOptions{Name: "Tango shortcut routing test"}); err != nil {
		return err
	}
	send(b.app, "finishLaunching")
	sink := &shortcutTestSink{}
	ps, pc, err := b.NewCanvasSurface(1, platform.SurfaceOptions{Title: "Shortcut routing test", Width: 320, Height: 200, Visible: true}, platform.SurfaceExtras{}, sink)
	if err != nil {
		return err
	}
	c := pc.(*canvas)
	capture, ok := pc.(interface{ SetShortcutCapture(bool) })
	if !ok {
		return fmt.Errorf("canvas does not implement SetShortcutCapture(bool)")
	}
	defer ps.Close()
	var result error
	withPool(func() {
		clicks := 0
		b.SetAppMenu(0, []platform.MenuItem{{Label: "Test", Submenu: []platform.MenuItem{{ID: 42, Label: "Record", Accelerator: "Cmd+K"}}}}, func(uint32) { clicks++ })
		send(b.app, "activateIgnoringOtherApps:", 1)
		send(c.s.win, "makeKeyAndOrderFront:", 0)
		send(c.s.win, "makeFirstResponder:", uintptr(c.view))
		for deadline := time.Now().Add(2 * time.Second); !sendBool(c.s.win, "isKeyWindow") && time.Now().Before(deadline); {
			b.Step(false)
		}
		if !sendBool(c.s.win, "isKeyWindow") || send(c.s.win, "firstResponder") != c.view {
			result = fmt.Errorf("native canvas did not obtain key-window focus")
			return
		}
		var makeKey func(id, objc.SEL, uint, nsPoint, uint, float64, int, id, id, id, bool, uint16) id
		purego.RegisterFunc(&makeKey, msgSendAddr)
		event := func(typ uint, repeat bool) id {
			return makeKey(class("NSEvent"), sel("keyEventWithType:location:modifierFlags:timestamp:windowNumber:context:characters:charactersIgnoringModifiers:isARepeat:keyCode:"), typ, nsPoint{}, nsCommandKeyMask, 0, sendInt(c.s.win, "windowNumber"), 0, nsString("k"), nsString("k"), repeat, 40)
		}
		// Baseline proves this synthetic event really activates the native menu.
		send(b.app, "sendEvent:", uintptr(event(10, false)))
		if clicks != 1 {
			result = fmt.Errorf("baseline native menu clicks=%d, want 1", clicks)
			return
		}
		for _, route := range []string{"application", "window", "view"} {
			capture.SetShortcutCapture(true)
			sink.onKey = func() { capture.SetShortcutCapture(false); sink.onKey = nil }
			sink.keys = nil
			before := clicks
			ev := event(10, false)
			switch route {
			case "application":
				send(b.app, "sendEvent:", uintptr(ev))
			case "window":
				send(c.s.win, "performKeyEquivalent:", uintptr(ev))
			case "view":
				send(c.view, "performKeyEquivalent:", uintptr(ev))
			}
			if c.shortcutCapture || clicks != before || len(sink.keys) != 1 || sink.keys[0].Key != input.KeyK || !sink.keys[0].Down || sink.keys[0].Mods != input.ModMeta {
				result = fmt.Errorf("%s capture: menu delta=%d keys=%+v", route, clicks-before, sink.keys)
				return
			}
			capture.SetShortcutCapture(false)
			before = clicks
			sink.keys = nil
			send(b.app, "sendEvent:", uintptr(event(10, true)))
			if clicks != before || len(sink.keys) != 1 || !sink.keys[0].Repeat {
				result = fmt.Errorf("%s held repeat escaped after capture=false: menu delta=%d keys=%+v", route, clicks-before, sink.keys)
				return
			}
			send(b.app, "sendEvent:", uintptr(event(11, false)))
			if len(c.shortcutHeld) != 0 || len(sink.keys) != 2 || sink.keys[1].Down {
				result = fmt.Errorf("%s keyup did not release capture latch: held=%v keys=%+v", route, c.shortcutHeld, sink.keys)
				return
			}
			send(b.app, "sendEvent:", uintptr(event(10, false)))
			if clicks != before+1 {
				result = fmt.Errorf("%s false did not restore native menu", route)
				return
			}
		}
		capture.SetShortcutCapture(true)
		field := alloc("NSTextField")
		defer release(field)
		send(c.view, "addSubview:", uintptr(field))
		send(c.s.win, "makeFirstResponder:", uintptr(field))
		if send(c.s.win, "firstResponder") == c.view {
			result = fmt.Errorf("other control failed to obtain focus")
			return
		}
		before := clicks
		sink.keys = nil
		send(b.app, "sendEvent:", uintptr(event(10, false)))
		if clicks != before+1 || len(sink.keys) != 0 {
			result = fmt.Errorf("other responder affected: menu delta=%d keys=%+v", clicks-before, sink.keys)
			return
		}
	})
	return result
}
