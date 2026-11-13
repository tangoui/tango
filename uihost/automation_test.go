//go:build tango_dev

package uihost_test

import (
	"bytes"
	"image/png"
	"path/filepath"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/automation"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/uihost"
)

// pump runs pending frames of cv until the test ends, as a display link
// would.
func pump(t *testing.T, cv *fake.Canvas) {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Millisecond):
				cv.FlushFrame()
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-done })
}

// connectAuto starts an app with TANGO_AUTOMATION set and connects the
// public client to it.
func connectAuto(t *testing.T) (*tango.App, *fake.Backend, func() *automation.App) {
	t.Helper()
	tokenFile := filepath.Join(t.TempDir(), "auto.json")
	t.Setenv("TANGO_AUTOMATION", "127.0.0.1:0")
	t.Setenv("TANGO_AUTOMATION_TOKEN_FILE", tokenFile)
	keep := false // closing the last window must not quit (default on Linux/Windows)
	a, fb := faketest.Run(t, tango.Options{QuitOnLastClose: &keep})
	return a, fb, func() *automation.App {
		t.Helper()
		var addr, token string
		deadline := time.Now().Add(5 * time.Second)
		for {
			if tf, err := uihost.ReadTokenFileForTest(tokenFile); err == nil {
				addr, token = tf.Addr, tf.Token
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("no token file")
			}
			time.Sleep(5 * time.Millisecond)
		}
		app := automation.Connect(t, addr, token)
		app.SetTimeout(3 * time.Second)
		return app
	}
}

func TestAutomationCanvas(t *testing.T) {
	a, fb, conn := connectAuto(t)
	clicks, text := 0, ""
	_, s, cv := open(t, a, fb, 400, 300, func(c *ui.Context) {
		if ui.Button(c, "Add").Clicked() {
			clicks++
		}
		ui.Text(c, "clicks "+string(rune('0'+clicks)))
		ui.Input(c, &text).Placeholder("Name")
	})
	pump(t, cv)
	app := conn()

	ws := app.Windows()
	if len(ws) != 1 || ws[0].ID != uint64(s.ID()) || ws[0].Kind != automation.KindCanvas || ws[0].Title != "UI" {
		t.Fatalf("windows %+v", ws)
	}
	win := app.Window("UI")

	// The tree is forced on although no assistive technology is active,
	// and the platform bridge gets nothing (force only feeds subscribers).
	btn := win.Find(a11y.RoleButton, "Add")
	if n := btn.Node(); n.Bounds.W <= 0 || !n.Actions.Has(a11y.Press) {
		t.Fatalf("button %+v", n)
	}
	if calls := cv.A11yCalls(); len(calls) != 0 {
		t.Fatalf("platform bridge called without an assistive technology: %d calls", len(calls))
	}

	btn.Click() // a11y Press
	win.Find(a11y.RoleText, "clicks 1").Wait()
	btn.ClickPointer() // real pointer events at the center
	win.Find(a11y.RoleText, "clicks 2").Wait()
	b := btn.Bounds()
	win.Click(b.X+2, b.Y+2)
	win.Find(a11y.RoleText, "clicks 3").Wait()

	tb := win.Find(a11y.RoleTextbox, "")
	tb.SetValue("hello")
	tb.WaitValue("hello")
	tb.Type(" world")
	if got := tb.WaitValue("hello world").Value; got != "hello world" {
		t.Fatalf("typed %q", got)
	}
	if got := tango.Await(a, func() string { return text }); got != "hello world" {
		t.Fatalf("app state %q", got)
	}

	if win.FindContains(a11y.RoleText, "CLICKS").Count() != 1 {
		t.Fatal("contains match")
	}
	if win.FindRegexp(a11y.RoleNone, `^clicks \d$`).Name() != "clicks 3" {
		t.Fatal("regexp match")
	}
	if win.Find(a11y.RoleButton, "Missing").Exists() {
		t.Fatal("Exists on a missing node")
	}
	win.Find(a11y.RoleButton, "Missing").Timeout(50 * time.Millisecond).WaitGone()

	img, err := png.Decode(bytes.NewReader(win.Screenshot()))
	if err != nil {
		t.Fatal(err)
	}
	if sz := img.Bounds().Size(); sz.X != 400 || sz.Y != 300 {
		t.Fatalf("screenshot %v", sz)
	}
	if d := win.Dump(); len(d) == 0 || len(win.Nodes()) < 3 {
		t.Fatalf("dump %q", d)
	}

	// Closing the window: further calls report it.
	tango.Await(a, func() struct{} { s.Close(); return struct{}{} })
	deadline := time.Now().Add(3 * time.Second)
	for len(app.Windows()) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("closed window still listed")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAutomationShortcutKeys(t *testing.T) {
	a, fb, conn := connectAuto(t)
	hits := 0
	_, s, cv := open(t, a, fb, 300, 200, func(c *ui.Context) {
		ui.Text(c, "hits "+string(rune('0'+hits)))
	})
	tango.Await(a, func() error {
		_, err := uihost.Shortcut(s, "CmdOrCtrl+K", func() { hits++ })
		return err
	})
	pump(t, cv)
	win := conn().Window("UI")
	win.Find(a11y.RoleText, "hits 0").Wait()
	tango.Await(a, func() struct{} { s.Focus(); return struct{}{} })
	win.Press("CmdOrCtrl+K")
	if got := tango.Await(a, func() int { return hits }); got != 1 {
		t.Fatalf("shortcut hits %d", got)
	}
}
