//go:build linux && !android

package linux

import (
	"net/http"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
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
		t.Skip("requires GTK/WebKit and a desktop; set TANGO_TEST_NATIVE_SHORTCUT=1")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	b := New().(*backend)
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
	if err := loadCanvasSmoke(); err != nil {
		t.Fatal(err)
	}
	k := &canvasSmoke
	var entryNew func() ptr
	var eventsPending func() bool
	var mainIteration func() bool
	purego.RegisterLibFunc(&entryNew, libGTK, "gtk_entry_new")
	purego.RegisterLibFunc(&eventsPending, libGTK, "gtk_events_pending")
	purego.RegisterLibFunc(&mainIteration, libGTK, "gtk_main_iteration")
	canvasFFI.widgetGrabFocus(c.widget)
	for eventsPending() {
		mainIteration()
	}
	if !canvasFFI.widgetHasFocus(c.widget) {
		t.Fatal("canvas has no real keyboard focus")
	}
	// Dispatch to GtkWindow, not the canvas: its default handler activates
	// menu accelerators before it propagates to the focus widget.
	route := func(typ int32) {
		ev := k.eventNew(typ)
		defer k.eventFree(ev)
		*(*ptr)(unsafe.Add(toPtr(ev), 8)) = k.objectRef(canvasFFI.widgetGetWindow(c.owner.win))
		*(*int8)(unsafe.Add(toPtr(ev), 16)) = 1
		*(*uint32)(unsafe.Add(toPtr(ev), canvasEvKeyState)) = gdkControlMask
		*(*uint32)(unsafe.Add(toPtr(ev), canvasEvKeyKeyval)) = 'k'
		*(*uint16)(unsafe.Add(toPtr(ev), canvasEvKeyHWCode)) = 45
		k.widgetEvent(c.owner.win, ev)
	}
	route(canvasGdkKeyPress)
	if clicks != 1 {
		t.Fatalf("baseline native menu clicks=%d", clicks)
	}
	capture.SetShortcutCapture(true)
	sink.keys = nil
	route(canvasGdkKeyPress)
	if clicks != 1 || len(sink.keys) != 1 || sink.keys[0].Key != input.KeyK {
		t.Fatalf("capture clicks=%d keys=%+v", clicks, sink.keys)
	}
	capture.SetShortcutCapture(false)
	sink.keys = nil
	route(canvasGdkKeyPress)
	if clicks != 1 || len(sink.keys) != 1 || !sink.keys[0].Repeat {
		t.Fatalf("held repeat escaped: clicks=%d keys=%+v", clicks, sink.keys)
	}
	route(canvasGdkKeyRelease)
	if len(c.shortcutHeld) != 0 {
		t.Fatal("keyup did not clear held capture")
	}
	route(canvasGdkKeyPress)
	if clicks != 2 {
		t.Fatalf("false did not restore menu: %d", clicks)
	}
	capture.SetShortcutCapture(true)
	entry := entryNew()
	gtkBoxPackStart(c.owner.box, entry, false, false, 0)
	gtkWidgetShowAll(entry)
	canvasFFI.widgetGrabFocus(entry)
	for eventsPending() {
		mainIteration()
	}
	if canvasFFI.widgetHasFocus(c.widget) {
		t.Fatal("canvas retained focus")
	}
	sink.keys = nil
	route(canvasGdkKeyPress)
	if clicks != 3 || len(sink.keys) != 0 {
		t.Fatalf("other focus affected: clicks=%d keys=%+v", clicks, sink.keys)
	}
}
