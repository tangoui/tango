//go:build windows && (amd64 || arm64)

package windows

import (
	"net/http"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

type shortcutTestHost struct{}

func (*shortcutTestHost) Drain()                                                   {}
func (*shortcutTestHost) ShouldClose(platform.SurfaceID) bool                      { return true }
func (*shortcutTestHost) OnLifecycle(platform.LifecycleEvent)                      {}
func (*shortcutTestHost) OnSurfaceEvent(platform.SurfaceID, platform.SurfaceEvent) {}
func (*shortcutTestHost) OnMessage(platform.SurfaceID, string, string)             {}
func (*shortcutTestHost) ServeAsset(_ platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

type shortcutTestSink struct{ keys []input.KeyEvent }

func (*shortcutTestSink) CanvasFrame()                            {}
func (*shortcutTestSink) CanvasResized(float32, float32, float32) {}
func (*shortcutTestSink) CanvasFocus(bool)                        {}
func (s *shortcutTestSink) CanvasInput(e input.Event) {
	if k, ok := e.(input.KeyEvent); ok {
		s.keys = append(s.keys, k)
	}
}

func TestNativeShortcutCapture(t *testing.T) {
	if os.Getenv("TANGO_TEST_NATIVE_SHORTCUT") != "1" {
		t.Skip("requires desktop; set TANGO_TEST_NATIVE_SHORTCUT=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	b := New().(*Backend)
	if err := b.Init(&shortcutTestHost{}, platform.AppOptions{Name: "Shortcut routing test"}); err != nil {
		t.Fatal(err)
	}
	sink := &shortcutTestSink{}
	ps, pc, err := b.NewCanvasSurface(1, platform.SurfaceOptions{Width: 320, Height: 200, Visible: true}, platform.SurfaceExtras{}, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	c := pc.(*canvas)
	capture, ok := pc.(interface{ SetShortcutCapture(bool) })
	if !ok {
		t.Fatal("canvas does not implement SetShortcutCapture(bool)")
	}
	clicks := 0
	b.SetAppMenu(1, []platform.MenuItem{{Label: "Test", Submenu: []platform.MenuItem{{ID: 42, Label: "Record", Accelerator: "Ctrl+K"}}}}, func(uint32) { clicks++ })
	canvasSetFocus.Call(c.hwnd)
	focus, _, _ := canvasGetFocus.Call()
	if focus != c.hwnd {
		t.Fatal("canvas failed to get native focus")
	}
	// Control's state is thread-local. No OS-global SendInput/hotkey needed.
	getState := user32.NewProc("GetKeyboardState")
	setState := user32.NewProc("SetKeyboardState")
	var saved [256]byte
	getState.Call(uintptr(unsafe.Pointer(&saved[0])))
	defer setState.Call(uintptr(unsafe.Pointer(&saved[0])))
	keys := saved
	keys[0x11] = 0x80
	setState.Call(uintptr(unsafe.Pointer(&keys[0])))
	m := msg{HWnd: c.hwnd, Message: canvasWmKeyDown, WParam: 'K', LParam: 0x00250001}
	b.dispatch(&m)
	if clicks != 1 {
		t.Fatalf("baseline native menu clicks=%d", clicks)
	}
	capture.SetShortcutCapture(true)
	sink.keys = nil
	b.dispatch(&m)
	if clicks != 1 || len(sink.keys) != 1 || sink.keys[0].Key != input.KeyK {
		t.Fatalf("capture clicks=%d keys=%+v", clicks, sink.keys)
	}
	capture.SetShortcutCapture(false)
	sink.keys = nil
	repeat := m
	repeat.LParam |= 1 << 30
	b.dispatch(&repeat)
	if clicks != 1 || len(sink.keys) != 1 || !sink.keys[0].Repeat {
		t.Fatalf("held repeat escaped: clicks=%d keys=%+v", clicks, sink.keys)
	}
	up := m
	up.Message = canvasWmKeyUp
	up.LParam |= 3 << 30
	b.dispatch(&up)
	if len(c.shortcutHeld) != 0 {
		t.Fatal("keyup did not clear held capture")
	}
	b.dispatch(&m)
	if clicks != 2 {
		t.Fatalf("false did not restore menu: %d", clicks)
	}
	capture.SetShortcutCapture(true)
	// An actual native edit control must keep normal menu accelerators.
	edit, _, createErr := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(u16("EDIT"))), 0, canvasWsChild|canvasWsVisible, 0, 0, 100, 24, c.parent, 0, instance(), 0)
	if edit == 0 {
		t.Fatalf("create native edit: %v", createErr)
	}
	defer procDestroyWindow.Call(edit)
	canvasSetFocus.Call(edit)
	focus, _, _ = canvasGetFocus.Call()
	if focus != edit {
		t.Fatal("native edit failed to take focus")
	}
	// Even a stale queued message naming canvas must not bypass the menu.
	if b.translateAccelerator(&m) != true || clicks != 3 {
		t.Fatalf("other focus affected: clicks=%d", clicks)
	}
	other := m
	other.HWnd = edit
	if !b.translateAccelerator(&other) || clicks != 4 {
		t.Fatalf("native edit menu did not activate: clicks=%d", clicks)
	}
	canvasSetFocus.Call(c.hwnd)
	// DefWindowProc on Alt+F4 closes the window; suppress it in capture mode.
	keys[0x11] = 0
	keys[0x12] = 0x80
	setState.Call(uintptr(unsafe.Pointer(&keys[0])))
	sink.keys = nil
	altF4 := msg{HWnd: c.hwnd, Message: canvasWmSysKeyDown, WParam: 0x73, LParam: 0x203e0001}
	b.dispatch(&altF4)
	if c.closed || len(sink.keys) != 1 || sink.keys[0].Key != input.KeyF4 {
		t.Fatalf("Alt+F4 escaped capture: closed=%v keys=%+v", c.closed, sink.keys)
	}
	altX := msg{HWnd: c.hwnd, Message: canvasWmSysKeyDown, WParam: 'X', LParam: 0x202d0001}
	b.dispatch(&altX)
	var pending msg
	if got, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&pending)), c.hwnd, canvasWmSysChar, canvasWmSysChar, pmRemove); got != 0 {
		t.Fatal("capture synthesized a queued WM_SYSCHAR")
	}
	if _, handled := c.inputMessage(canvasWmSysChar, 'x', 0); !handled {
		t.Fatal("WM_SYSCHAR escaped capture")
	}
	capture.SetShortcutCapture(false)
	c.inputMessage(canvasWmSysKeyUp, 0x73, 0xe03e0001)
	if _, handled := c.inputMessage(canvasWmSysKeyDown, 0x73, 0x203e0001); handled {
		t.Fatal("false must restore system-key default handling")
	}
}
