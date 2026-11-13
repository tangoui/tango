package flyout

import (
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/tray"
)

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(faketest.Timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFlyout(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	tr, err := tray.New(a, tray.Options{Icon: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := New(a, tr, Options{Width: 300, Height: 200, URL: "/panel.html"})
	if err != nil {
		t.Fatal(err)
	}
	fs := fb.Surface(f.Surface().ID())
	ex, _ := fs.Extras()
	if ex.Kind != platform.KindPanel || ex.Appearance.TitleBar != platform.TitleBarNone || !ex.Appearance.AlwaysOnTop {
		t.Fatalf("%+v", ex)
	}
	if fs.Visible() {
		t.Fatal("visible before Open")
	}
	var opened, closed int
	f.OnOpen(func() { opened++ })
	f.OnClose(func() { closed++ })

	// A tray click toggles it open under the icon.
	fb.Trays()[0].Click()
	waitFor(t, "open", f.IsOpen)
	want := platform.Rect{X: fake.TrayBounds.X + fake.TrayBounds.Width/2 - 150, Y: fake.DefaultDisplay.WorkArea.Y + 6, Width: 300, Height: 200}
	if b := f.Surface().Bounds(); b != want || !fs.Visible() {
		t.Fatalf("bounds %+v want %+v visible %v", b, want, fs.Visible())
	}

	// Losing focus hides it; the tray click that caused the blur does not
	// reopen it.
	fb.FocusChange(fs.ID(), false)
	if f.IsOpen() || fs.Visible() {
		t.Fatal("still open after blur")
	}
	fb.Trays()[0].Click()
	if f.IsOpen() {
		t.Fatal("reopened by the click that blurred it")
	}
	// A later click opens it again.
	old := now
	now = func() time.Time { return old().Add(time.Second) }
	defer func() { now = old }()
	fb.Trays()[0].Click()
	waitFor(t, "reopen", f.IsOpen)
	fb.Trays()[0].Click()
	waitFor(t, "toggle closed", func() bool { return !f.IsOpen() })

	// Programmatic.
	if err := f.Open(); err != nil {
		t.Fatal(err)
	}
	if !f.IsOpen() {
		t.Fatal("Open")
	}
	f.Close()
	if f.IsOpen() || fs.Visible() {
		t.Fatal("Close")
	}
	if opened != 3 || closed != 3 {
		t.Fatalf("opened %d closed %d", opened, closed)
	}
	// Closing the window from the system only hides it.
	fb.RequestClose(fs.ID())
	if fs.Closed() {
		t.Fatal("panel destroyed")
	}
}

func TestKeepOnBlurAndNoTray(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	f, err := New(a, nil, Options{KeepOnBlur: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Open(); err != nil {
		t.Fatal(err)
	}
	b := f.Surface().Bounds()
	wa := fake.DefaultDisplay.WorkArea
	if b.Width != 320 || b.Height != 400 || b.X != wa.X+wa.Width-320-6 {
		t.Fatalf("%+v", b)
	}
	fb.FocusChange(f.Surface().ID(), false)
	if !f.IsOpen() {
		t.Fatal("closed on blur with KeepOnBlur")
	}
	f.Surface().Close()
	waitFor(t, "closed", func() bool { return !f.IsOpen() })
	if err := f.Open(); err != ErrClosed {
		t.Fatal(err)
	}
}
