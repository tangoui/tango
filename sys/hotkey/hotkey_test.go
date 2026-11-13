package hotkey

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/corehook"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

func TestRegisterGlobal(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	fired := make(chan bool, 1)
	h, err := Register(a, "CmdOrCtrl+Shift+K", func() { fired <- tango.Await(a, func() bool { return true }) })
	if err != nil {
		t.Fatal(err)
	}
	if got := fb.Hotkeys()[h.id]; got != "CmdOrCtrl+Shift+K" {
		t.Fatalf("backend has %q", got)
	}
	if fb.PressHotkey("CmdOrCtrl+Shift+K") != 1 {
		t.Fatal("not registered")
	}
	select {
	case <-fired:
	case <-time.After(faketest.Timeout):
		t.Fatal("did not fire")
	}
	if err := h.Unregister(); err != nil {
		t.Fatal(err)
	}
	if len(fb.Hotkeys()) != 0 {
		t.Fatal("still registered")
	}
	if err := Unregister(h); !errors.Is(err, ErrUnregistered) {
		t.Fatal(err)
	}
}

func TestRegisterErrors(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	if _, err := Register(a, "Hyper+K", func() {}); err == nil {
		t.Fatal("bad accelerator accepted")
	}
	taken := errors.New("taken")
	fb.SetHotkeyErr(taken)
	if _, err := Register(a, "Ctrl+K", func() {}); err != taken {
		t.Fatal(err)
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := Register(m, "Ctrl+K", func() {}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestBindWindow(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	s, err := a.NewSurface(tango.SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fired := make(chan string, 4)
	b, err := Bind(s, "CmdOrCtrl+K", func() { fired <- "k" })
	if err != nil {
		t.Fatal(err)
	}
	other, _ := a.NewSurface(tango.SurfaceOptions{})
	fs := fb.Surface(s.ID())
	waitEval := func(want string) string {
		deadline := time.Now().Add(faketest.Timeout)
		for time.Now().Before(deadline) {
			ev := fs.Evals()
			if len(ev) > 0 && strings.Contains(ev[len(ev)-1], want) {
				return ev[len(ev)-1]
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("no eval containing %q: %q", want, fs.Evals())
		return ""
	}
	js := waitEval(`"code":"KeyK"`)
	if !strings.Contains(js, `"\u0000tango-int:hotkey:"`) || !strings.Contains(js, "addEventListener") {
		t.Fatalf("script: %s", js)
	}
	msg := corehook.InternalPrefix + "hotkey:" + strconv.Itoa(int(b.id))
	fb.Message(s.ID(), msg)
	select {
	case <-fired:
	case <-time.After(faketest.Timeout):
		t.Fatal("did not fire")
	}
	// The same message from another surface does not fire it.
	fb.Message(other.ID(), msg)
	// After a reload the table is sent again.
	n := len(fs.Evals())
	fb.Load(s.ID())
	if len(fs.Evals()) <= n {
		t.Fatal("not re-injected on load")
	}
	b.Unbind()
	waitEval(`T.b = {};`)
	fb.Message(s.ID(), msg)
	b.Unbind() // twice is fine
	tango.Await(a, func() bool { return true })
	select {
	case <-fired:
		t.Fatal("fired after unbind or from another surface")
	default:
	}
	if _, err := Bind(s, "Ctrl+Nope", func() {}); err == nil {
		t.Fatal("bad accelerator accepted")
	}
}
